package postgres

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/models/key"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
)

// P0/C8 + A8/ops — isolamento de tenant no SQL (não só na camada de serviço):
// o `SELECT ... WHERE id = $1` de ops.inspections/ops.maintenance/ops.keys não
// pode devolver a linha de outro tenant, e tenant vazio não pode virar busca
// sem escopo. Banco descartável, com nome obrigatoriamente `toca_review_qa_*`
// (mesmo harness de toca-core/toca-crm), então o teste é skip por padrão:
//
//	createdb toca_review_qa_ops_p0
//	OPS_REVIEW_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:54322/toca_review_qa_ops_p0?sslmode=disable' \
//	  go test ./internal/repositories/postgres/ -run TestOpsP0TenantIsolation -v
func TestOpsP0TenantIsolation(t *testing.T) {
	uri := os.Getenv("OPS_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set OPS_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
	}

	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri, MaxPoolSize: 2, MinPoolSize: 1})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var dbName string
	if err := db.QueryRowContext(ctx, "SELECT current_database()").Scan(&dbName); err != nil {
		t.Fatalf("current_database: %v", err)
	}
	if !strings.HasPrefix(dbName, "toca_review_qa_") {
		t.Fatalf("refusing to run against non-QA database %q", dbName)
	}

	if _, err := db.ExecContext(ctx, `
		CREATE SCHEMA IF NOT EXISTS events;
 CREATE TABLE IF NOT EXISTS events.outbox_events(id uuid DEFAULT gen_random_uuid() PRIMARY KEY, tenant_id uuid, event_name text NOT NULL, payload jsonb NOT NULL, status text NOT NULL);
 CREATE SCHEMA IF NOT EXISTS ops;
		CREATE TABLE IF NOT EXISTS ops.inspections (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		CREATE TABLE IF NOT EXISTS ops.maintenance (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		CREATE TABLE IF NOT EXISTS ops.keys (
			id UUID PRIMARY KEY, tenant_id UUID NOT NULL, metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ, updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
	`); err != nil {
		t.Fatalf("create ops schema: %v", err)
	}

	tenantA := uuid.NewString() // chamador
	tenantB := uuid.NewString() // dono dos registros

	inspRepo := NewInspectionRepository(db)
	insp := &inspection.Inspection{
		ID:         uuid.NewString(),
		TenantID:   tenantB,
		PropertyID: "prop-tenant-B",
		ContractID: "contract-tenant-B",
		Type:       inspection.TypeExit,
		Status:     inspection.StatusConcluida,
		ReportURL:  "https://docs.example/tenant-B/laudo.pdf",
	}
	if err := inspRepo.Create(ctx, insp); err != nil {
		t.Fatalf("seed inspection: %v", err)
	}

	mtRepo := NewMaintenanceRepository(db)
	mt := maintenance.New("hidraulica", "Reparo", "prop-tenant-B", "contract-tenant-B", "Imóvel", "Cliente", "Inquilino", "Alta", "vazamento", tenantB)
	mt.Attachments = []string{"tenant-B/maintenance/photo.jpg"}
	if err := mtRepo.Create(ctx, mt); err != nil {
		t.Fatalf("seed maintenance: %v", err)
	}

	keyRepo := NewKeyRepository(db)
	k := key.New("prop-tenant-B", "K-01", "Portaria", "", tenantB)
	if err := keyRepo.Create(ctx, k); err != nil {
		t.Fatalf("seed key: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM ops.inspections WHERE id = $1", insp.ID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM ops.maintenance WHERE id = $1", mt.ID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM ops.keys WHERE id = $1", k.ID)
	})

	// notFound exige o erro de "não encontrado" — um erro genérico de SQL (bind
	// errado, tabela ausente) NÃO pode ser confundido com isolamento correto.
	// `loaded` é o resultado de `got != nil` calculado no call site: comparar o
	// ponteiro tipado (e não um `any`) evita o falso positivo do nil-boxed.
	notFound := func(t *testing.T, label string, loaded bool, err error) {
		t.Helper()
		if loaded {
			t.Errorf("%s: registro de outro tenant foi devolvido", label)
			return
		}
		if err == nil || !strings.Contains(err.Error(), "not found") {
			t.Errorf("%s: esperado erro 'not found' (e não erro genérico), got=%v", label, err)
		}
	}
	failsClosed := func(t *testing.T, label string, loaded bool, err error) {
		t.Helper()
		if loaded {
			t.Errorf("%s: tenant vazio devolveu registro", label)
			return
		}
		if err == nil || !strings.Contains(err.Error(), "tenant context required") {
			t.Errorf("%s: esperado fail-closed ('tenant context required'), got=%v", label, err)
		}
	}

	// 1) chamador de OUTRO tenant: nada é devolvido (a linha existe no banco).
	{
		g, e := inspRepo.GetByID(ctx, tenantA, insp.ID)
		notFound(t, "C8 inspection", g != nil, e)
	}
	{
		g, e := mtRepo.GetByID(ctx, tenantA, mt.ID)
		notFound(t, "A8 maintenance", g != nil, e)
	}
	{
		g, e := keyRepo.GetByID(ctx, tenantA, k.ID)
		notFound(t, "A8 key", g != nil, e)
	}

	// 2) fail closed sem tenant: erro, sem cair em busca sem escopo.
	{
		g, e := inspRepo.GetByID(ctx, "", insp.ID)
		failsClosed(t, "C8 inspection", g != nil, e)
	}
	{
		g, e := mtRepo.GetByID(ctx, "   ", mt.ID)
		failsClosed(t, "A8 maintenance", g != nil, e)
	}
	{
		g, e := keyRepo.GetByID(ctx, "", k.ID)
		failsClosed(t, "A8 key", g != nil, e)
	}

	// 3) o dono continua lendo (sem regressão funcional).
	gotInsp, err := inspRepo.GetByID(ctx, tenantB, insp.ID)
	if err != nil || gotInsp == nil {
		t.Fatalf("dono deveria ler a própria vistoria: got=%+v err=%v", gotInsp, err)
	}
	if gotInsp.ReportURL != insp.ReportURL {
		t.Errorf("report_url = %q, want %q", gotInsp.ReportURL, insp.ReportURL)
	}
	gotMt, err := mtRepo.GetByID(ctx, tenantB, mt.ID)
	if err != nil || gotMt == nil {
		t.Fatalf("dono deveria ler o próprio chamado: got=%+v err=%v", gotMt, err)
	}
	gotKey, err := keyRepo.GetByID(ctx, tenantB, k.ID)
	if err != nil || gotKey == nil {
		t.Fatalf("dono deveria ler a própria chave: got=%+v err=%v", gotKey, err)
	}

	// 4) ESCRITA (item 36 da revisão): o soft delete carrega o tenant na própria
	// sentença. Antes, o repositório apagava por id e a segurança dependia do
	// handler ter feito um GetByID escopado antes — uma regressão de wiring (ou
	// um call site novo) apagava o registro de outro tenant.
	softDeleted := func(t *testing.T, label, table, id string) bool {
		t.Helper()
		var deleted bool
		if err := db.QueryRowContext(ctx,
			"SELECT deleted_at IS NOT NULL FROM "+table+" WHERE id = $1", id).Scan(&deleted); err != nil {
			t.Fatalf("%s: reler %s: %v", label, table, err)
		}
		return deleted
	}

	for _, tc := range []struct {
		label string
		table string
		id    string
		del   func(tenantID, id string) error
	}{
		{"C8 inspection", "ops.inspections", insp.ID, func(tid, id string) error { return inspRepo.Delete(ctx, tid, id) }},
		{"A8 maintenance", "ops.maintenance", mt.ID, func(tid, id string) error { return mtRepo.Delete(ctx, tid, id) }},
		{"A8 key", "ops.keys", k.ID, func(tid, id string) error { return keyRepo.Delete(ctx, tid, id) }},
	} {
		t.Run("delete cross-tenant recusado: "+tc.label, func(t *testing.T) {
			if err := tc.del(tenantA, tc.id); err != nil {
				t.Fatalf("delete com tenant alheio devolveu erro: %v", err)
			}
			if softDeleted(t, tc.label, tc.table, tc.id) {
				t.Fatalf("%s: registro de OUTRO tenant foi apagado (falta predicado de tenant na escrita)", tc.label)
			}
		})
		t.Run("delete sem tenant recusado: "+tc.label, func(t *testing.T) {
			// Fail-closed: o repositório recusa explicitamente ("tenant context
			// required") — antes da correção, o tenant vazio ia para o SQL e o
			// erro era do banco (uuid inválido). O que não pode acontecer, em
			// nenhum dos casos, é o registro ser apagado.
			if err := tc.del("", tc.id); err != nil && !strings.Contains(err.Error(), "tenant context required") {
				t.Fatalf("%s: fail-closed com erro inesperado: %v", tc.label, err)
			}
			if softDeleted(t, tc.label, tc.table, tc.id) {
				t.Fatalf("%s: delete sem tenant apagou o registro (fail-open)", tc.label)
			}
		})
		t.Run("delete do dono funciona: "+tc.label, func(t *testing.T) {
			if err := tc.del(tenantB, tc.id); err != nil {
				t.Fatalf("delete do dono falhou: %v", err)
			}
			if !softDeleted(t, tc.label, tc.table, tc.id) {
				t.Fatalf("%s: dono não conseguiu apagar o próprio registro", tc.label)
			}
		})
	}
}
