package repository_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

// P0 / C9 — isolamento de tenant no SQL do repositório de leads.
//
// Complementa lead_p0_tenant_isolation_test.go (nível de serviço): aqui a prova
// é a query. O banco de teste é descartável e precisa ter nome começando com
// `toca_review_qa_` (mesmo harness de lead_postgres_integration_test.go).
//
// Rodar:
//
//	createdb toca_review_qa_p0
//	CRM_REVIEW_TEST_DATABASE_URL=postgres://.../toca_review_qa_p0 go test ./internal/repository/ -run TestLeadP0TenantIsolation
func TestLeadP0TenantIsolation(t *testing.T) {
	uri := os.Getenv("CRM_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set CRM_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "toca_review_qa_") {
		t.Fatal("refusing non-QA database")
	}

	if _, err := db.Exec(`CREATE SCHEMA crm;
		CREATE TABLE crm.leads (id text PRIMARY KEY, tenant_id text NOT NULL, metadata jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
		updated_seq bigint NOT NULL DEFAULT 0, deleted_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA crm CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	ctx := context.Background()
	repo := repository.NewLeadPostgresRepository(&database.PostgresDB{DB: db})

	const (
		attackerTenant = "tenant-atacante"
		victimTenant   = "tenant-vitima"
		victimLeadID   = "lead-da-vitima"
	)

	victim := &lead.Lead{
		ID:       victimLeadID,
		TenantID: victimTenant,
		Name:     "Titular de outro tenant",
		Email:    "vitima@example.com",
		Phone:    "11999999999",
		Status:   lead.LeadStatusNew,
	}
	if err := repo.Create(ctx, victim); err != nil {
		t.Fatalf("create victim lead: %v", err)
	}

	// SQL helper: lê a linha direto, sem passar pelo repositório.
	rawMetadata := func(t *testing.T) string {
		t.Helper()
		var meta string
		if err := db.Get(&meta, "SELECT metadata::text FROM crm.leads WHERE id = $1", victimLeadID); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		return meta
	}

	t.Run("GetByID_recusa_tenant_diferente", func(t *testing.T) {
		got, err := repo.GetByID(ctx, attackerTenant, victimLeadID)
		if err == nil {
			t.Fatalf("VAZAMENTO CROSS-TENANT: leu o lead do tenant %q (retorno=%+v)", victimTenant, got)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("esperava sql.ErrNoRows (não encontrado), veio %v", err)
		}
	})

	t.Run("GetByID_encontra_no_proprio_tenant", func(t *testing.T) {
		got, err := repo.GetByID(ctx, victimTenant, victimLeadID)
		if err != nil {
			t.Fatalf("leitura legítima falhou: %v", err)
		}
		if got == nil || got.ID != victimLeadID {
			t.Fatalf("esperava o lead do próprio tenant, veio %+v", got)
		}
	})

	t.Run("GetByID_falha_fechado_sem_tenant", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, "", victimLeadID); err == nil {
			t.Fatal("tenant vazio deveria ser recusado (fail-closed), não buscar globalmente")
		}
		if _, err := repo.GetByID(ctx, "   ", victimLeadID); err == nil {
			t.Fatal("tenant em branco deveria ser recusado (fail-closed)")
		}
	})

	t.Run("GetByIDAnyTenant_e_o_escape_hatch_explicito", func(t *testing.T) {
		got, err := repo.GetByIDAnyTenant(ctx, victimLeadID)
		if err != nil || got == nil {
			t.Fatalf("escape hatch documentado deveria funcionar (portal do cliente): got=%+v err=%v", got, err)
		}
	})

	t.Run("Update_nao_grava_fora_do_tenant_do_agregado", func(t *testing.T) {
		before := rawMetadata(t)
		// Agregado construído com o id da vítima mas tenant do atacante: é o
		// formato que um handler ingênuo produziria a partir do corpo/URL.
		forged := &lead.Lead{ID: victimLeadID, TenantID: attackerTenant, Name: "invadido"}
		_ = repo.Update(ctx, forged)
		if after := rawMetadata(t); after != before {
			t.Fatalf("ESCRITA CROSS-TENANT: metadata da vítima mudou\nantes: %s\ndepois: %s", before, after)
		}
	})

	t.Run("Delete_recusa_tenant_diferente_e_preserva_a_linha", func(t *testing.T) {
		err := repo.Delete(ctx, attackerTenant, victimLeadID)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("esperava sql.ErrNoRows (nada afetado), veio %v", err)
		}
		var deletedAt *string
		if err := db.Get(&deletedAt, "SELECT deleted_at::text FROM crm.leads WHERE id = $1", victimLeadID); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		if deletedAt != nil {
			t.Fatalf("EXCLUSÃO CROSS-TENANT: deleted_at=%v", *deletedAt)
		}
	})

	t.Run("Patch_nao_afeta_outro_tenant", func(t *testing.T) {
		before := rawMetadata(t)
		_, ok, err := repo.Patch(ctx, victimLeadID, attackerTenant, nil, map[string]interface{}{"Name": "invadido"})
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if ok {
			t.Fatal("Patch reportou sucesso para tenant diferente do dono")
		}
		if after := rawMetadata(t); after != before {
			t.Fatal("ESCRITA CROSS-TENANT via Patch")
		}
	})
}
