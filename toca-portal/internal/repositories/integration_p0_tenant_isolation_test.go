package repositories_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	"github.com/oginoino/toca/toca-portal/internal/repositories"
)

// P0 / C4 — isolamento de tenant no SQL do repositório de integrações.
//
// Antes da correção:
//
//	GetByID: SELECT metadata FROM portal.integrations WHERE id = $1 AND deleted_at IS NULL
//	Update : UPDATE ... WHERE id = $2 AND tenant_id = $3   -- $3 era o tenant DO REGISTRO
//	Delete : UPDATE ... SET deleted_at = NOW() WHERE id = $1 AND deleted_at IS NULL
//
// Depois: tenant do chamador sempre no WHERE; 0 linhas = ErrNotFound.
//
// O banco de teste é descartável e precisa ter nome começando com
// `toca_review_qa_` (mesmo harness de
// toca-crm/internal/repository/lead_p0_tenant_isolation_test.go).
//
// Rodar:
//
//	createdb toca_review_qa_portal_p0
//	PORTAL_REVIEW_TEST_DATABASE_URL=postgres://.../toca_review_qa_portal_p0 \
//	  go test ./internal/repositories/ -run TestIntegrationP0TenantIsolation -v
func TestIntegrationP0TenantIsolation(t *testing.T) {
	uri := os.Getenv("PORTAL_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set PORTAL_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
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
		t.Fatalf("refusing non-QA database %q", name)
	}

	if _, err := db.Exec(`DROP SCHEMA IF EXISTS portal CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE SCHEMA portal;
		CREATE TABLE portal.integrations (
			id text PRIMARY KEY,
			tenant_id text NOT NULL,
			metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		);
		CREATE TABLE portal.sync_history (
			id text PRIMARY KEY,
			tenant_id text NOT NULL,
			metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		);
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA portal CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	ctx := context.Background()
	repo := repositories.NewIntegrationPostgresRepository(&database.PostgresDB{DB: db})

	const (
		attackerTenant = "tenant-atacante"
		victimTenant   = "tenant-vitima"
		victimID       = "integ-da-vitima"
		ownID          = "integ-do-atacante"
	)

	victim := &integration.PortalIntegration{
		ID:            victimID,
		TenantID:      victimTenant,
		Portal:        integration.PortalZAP,
		Status:        integration.IntegrationActive,
		ContactName:   "Titular Vitima",
		ContactEmail:  "vitima@example.com",
		FeedToken:     "feed-token-da-vitima",
		WebhookSecret: "segredo-da-vitima",
	}
	own := &integration.PortalIntegration{
		ID:            ownID,
		TenantID:      attackerTenant,
		Portal:        integration.PortalZAP,
		Status:        integration.IntegrationActive,
		ContactName:   "Titular Atacante",
		ContactEmail:  "atacante@example.com",
		FeedToken:     "feed-token-do-atacante",
		WebhookSecret: "segredo-do-atacante",
	}
	if err := repo.Create(ctx, victim); err != nil {
		t.Fatalf("create victim integration: %v", err)
	}
	if err := repo.Create(ctx, own); err != nil {
		t.Fatalf("create own integration: %v", err)
	}

	rawMetadata := func(t *testing.T, id string) string {
		t.Helper()
		var meta string
		if err := db.Get(&meta, "SELECT metadata::text FROM portal.integrations WHERE id = $1", id); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		return meta
	}
	rawDeletedAt := func(t *testing.T, id string) *string {
		t.Helper()
		var deletedAt *string
		if err := db.Get(&deletedAt, "SELECT deleted_at::text FROM portal.integrations WHERE id = $1", id); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		return deletedAt
	}

	t.Run("GetByID_recusa_tenant_diferente", func(t *testing.T) {
		got, err := repo.GetByID(ctx, attackerTenant, victimID)
		if err == nil {
			t.Fatalf("VAZAMENTO CROSS-TENANT: leu a integração do tenant %q (retorno=%+v)", victimTenant, got)
		}
		if !errors.Is(err, integration.ErrNotFound) {
			t.Fatalf("esperava integration.ErrNotFound (404), veio %v", err)
		}
	})

	t.Run("GetByID_encontra_no_proprio_tenant", func(t *testing.T) {
		got, err := repo.GetByID(ctx, victimTenant, victimID)
		if err != nil {
			t.Fatalf("leitura legítima falhou: %v", err)
		}
		if got == nil || got.ID != victimID || got.TenantID != victimTenant {
			t.Fatalf("esperava a integração do próprio tenant, veio %+v", got)
		}
		if got.WebhookSecret != "segredo-da-vitima" {
			t.Fatalf("webhook_secret não foi carregado do metadata: %q", got.WebhookSecret)
		}
	})

	t.Run("GetByID_falha_fechado_sem_tenant", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, "", victimID); !errors.Is(err, integration.ErrNotFound) {
			t.Fatalf("tenant vazio deveria ser recusado (fail-closed), veio %v", err)
		}
		if _, err := repo.GetByID(ctx, "   ", victimID); !errors.Is(err, integration.ErrNotFound) {
			t.Fatalf("tenant em branco deveria ser recusado (fail-closed), veio %v", err)
		}
	})

	t.Run("Update_nao_grava_fora_do_tenant_do_chamador", func(t *testing.T) {
		before := rawMetadata(t, victimID)
		// Agregado com o tenant do atacante e o id da vítima: formato que um
		// handler ingênuo produziria a partir do corpo/URL.
		forged := *victim
		forged.TenantID = attackerTenant
		forged.ContactName = "invadido"
		err := repo.Update(ctx, attackerTenant, &forged)
		if err == nil {
			t.Fatal("Update cross-tenant deveria devolver ErrNotFound, devolveu sucesso")
		}
		if !errors.Is(err, integration.ErrNotFound) {
			t.Fatalf("esperava integration.ErrNotFound, veio %v", err)
		}
		if after := rawMetadata(t, victimID); after != before {
			t.Fatalf("ESCRITA CROSS-TENANT: metadata da vítima mudou\nantes: %s\ndepois: %s", before, after)
		}
	})

	t.Run("Update_do_proprio_tenant_grava", func(t *testing.T) {
		mine := *own
		mine.ContactName = "Atualizado pelo dono"
		if err := repo.Update(ctx, attackerTenant, &mine); err != nil {
			t.Fatalf("update do próprio tenant falhou: %v", err)
		}
		var meta string
		if err := db.Get(&meta, "SELECT metadata::text FROM portal.integrations WHERE id = $1", ownID); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		if !strings.Contains(meta, "Atualizado pelo dono") {
			t.Fatalf("update legítimo não foi gravado: %s", meta)
		}
	})

	t.Run("Delete_recusa_tenant_diferente_e_preserva_a_linha", func(t *testing.T) {
		err := repo.Delete(ctx, attackerTenant, victimID)
		if !errors.Is(err, integration.ErrNotFound) {
			t.Fatalf("esperava integration.ErrNotFound (nada afetado), veio %v", err)
		}
		if deletedAt := rawDeletedAt(t, victimID); deletedAt != nil {
			t.Fatalf("EXCLUSÃO CROSS-TENANT: deleted_at=%v", *deletedAt)
		}
	})

	t.Run("Delete_do_proprio_tenant_funciona", func(t *testing.T) {
		if err := repo.Delete(ctx, attackerTenant, ownID); err != nil {
			t.Fatalf("delete do próprio tenant falhou: %v", err)
		}
		if deletedAt := rawDeletedAt(t, ownID); deletedAt == nil {
			t.Fatal("linha do próprio tenant não foi soft-deleted")
		}
	})

	t.Run("GetByIDAnyTenant_e_a_escotilha_do_webhook_publico", func(t *testing.T) {
		got, err := repo.GetByIDAnyTenant(ctx, victimID)
		if err != nil || got == nil {
			t.Fatalf("escotilha documentada (webhook público) deveria funcionar: got=%+v err=%v", got, err)
		}
		if got.TenantID != victimTenant {
			t.Fatalf("tenant derivado do registro veio errado: %q", got.TenantID)
		}
	})

	t.Run("GetByTenantAndPortal_sem_registro_devolve_nil_sem_erro", func(t *testing.T) {
		got, err := repo.GetByTenantAndPortal(ctx, "tenant-novo", integration.PortalOLX)
		if err != nil {
			t.Fatalf("ausência de integração não é erro (Create depende disso): %v", err)
		}
		if got != nil {
			t.Fatalf("esperava nil, veio %+v", got)
		}
	})

	t.Run("GetByFeedToken_continua_publico_por_token", func(t *testing.T) {
		got, err := repo.GetByFeedToken(ctx, "feed-token-da-vitima")
		if err != nil || got == nil || got.ID != victimID {
			t.Fatalf("feed público por token quebrou: got=%+v err=%v", got, err)
		}
		if _, err := repo.GetByFeedToken(ctx, "token-inexistente"); !errors.Is(err, integration.ErrNotFound) {
			t.Fatalf("token inexistente deveria ser ErrNotFound, veio %v", err)
		}
	})
}

// TestSyncP0TenantIsolation prova o mesmo para o histórico de sync
// (portal.sync_history), que também era filtrado só por integration_id.
func TestSyncP0TenantIsolation(t *testing.T) {
	uri := os.Getenv("PORTAL_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set PORTAL_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
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
		t.Fatalf("refusing non-QA database %q", name)
	}

	if _, err := db.Exec(`DROP SCHEMA IF EXISTS portal CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		CREATE SCHEMA portal;
		CREATE TABLE portal.sync_history (
			id text PRIMARY KEY,
			tenant_id text NOT NULL,
			metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		);
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA portal CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	ctx := context.Background()
	syncRepo := repositories.NewSyncPostgresRepository(&database.PostgresDB{DB: db})

	const (
		attackerTenant = "tenant-atacante"
		victimTenant   = "tenant-vitima"
		victimIntegID  = "integ-da-vitima"
	)
	if _, err := db.Exec(`
		INSERT INTO portal.sync_history (id, tenant_id, metadata)
		VALUES ('sync-da-vitima', $1, jsonb_build_object('id','sync-da-vitima','tenant_id',$1::text,'integration_id',$2::text))
	`, victimTenant, victimIntegID); err != nil {
		t.Fatal(err)
	}

	syncs, total, err := syncRepo.ListByIntegrationID(ctx, attackerTenant, victimIntegID, 20, 0)
	if err != nil {
		t.Fatalf("listagem falhou: %v", err)
	}
	if total != 0 || len(syncs) != 0 {
		t.Fatalf("VAZAMENTO CROSS-TENANT: histórico de sync da vítima visível (total=%d, itens=%d)", total, len(syncs))
	}

	syncs, total, err = syncRepo.ListByIntegrationID(ctx, victimTenant, victimIntegID, 20, 0)
	if err != nil {
		t.Fatalf("listagem legítima falhou: %v", err)
	}
	if total != 1 || len(syncs) != 1 {
		t.Fatalf("dono deveria ver o próprio histórico: total=%d itens=%d", total, len(syncs))
	}

	if _, err := syncRepo.GetLatestByIntegrationID(ctx, attackerTenant, victimIntegID); !errors.Is(err, integration.ErrNotFound) {
		t.Fatalf("GetLatest cross-tenant deveria ser ErrNotFound, veio %v", err)
	}
	if got, err := syncRepo.GetLatestByIntegrationID(ctx, victimTenant, victimIntegID); err != nil || got == nil {
		t.Fatalf("GetLatest do dono falhou: got=%+v err=%v", got, err)
	}
}
