package repositories_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca-core/internal/repositories"
	"github.com/oginoino/toca/pkg/database"
)

// P0/C6 — isolamento de tenant no SQL (não só na camada de serviço).
//
// Prova que `core.webhooks.deleted_at` e `core.api_keys.metadata.is_active`
// não podem ser alterados por um chamador de OUTRO tenant, que 0 linhas
// afetadas viram sql.ErrNoRows (=> 404, não "sucesso" falso) e que o signing
// secret continua sendo PERSISTIDO mesmo com `json:"-"` no modelo.
//
// Banco descartável, com nome obrigatoriamente `toca_review_qa_*` — mesmo
// harness de toca-crm/internal/repository/lead_p0_tenant_isolation_test.go.
//
//	createdb toca_review_qa_core_p0
//	CORE_REVIEW_TEST_DATABASE_URL=postgres://postgres:postgres@localhost:54322/toca_review_qa_core_p0?sslmode=disable \
//	  go test ./internal/repositories/ -run TestSettingsP0TenantIsolation -v
func TestSettingsP0TenantIsolation(t *testing.T) {
	uri := os.Getenv("CORE_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set CORE_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
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

	if _, err := db.Exec(`
		CREATE SCHEMA core;
		CREATE TABLE core.webhooks (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id uuid NOT NULL,
			metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		);
		CREATE TABLE core.api_keys (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id uuid NOT NULL,
			metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			deleted_at timestamptz
		);
	`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA core CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	ctx := context.Background()
	webhookRepo := repositories.NewWebhookPostgresRepository(&database.PostgresDB{DB: db})
	apiKeyRepo := repositories.NewAPIKeyPostgresRepository(&database.PostgresDB{DB: db})

	const (
		tenantAtacante = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
		tenantVitima   = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	)

	vitima := &settings.WebhookEndpoint{
		TenantID: tenantVitima,
		URL:      "https://vitima.example/hook",
		Events:   []string{"lead.created"},
		Active:   true,
		Secret:   "whsec_segredo_da_vitima",
	}
	if err := webhookRepo.Create(ctx, vitima); err != nil {
		t.Fatalf("create victim webhook: %v", err)
	}
	vitimaID, ok := vitima.ID.(string)
	if !ok || vitimaID == "" {
		t.Fatalf("Create deveria devolver o id gerado pelo RETURNING, veio %#v", vitima.ID)
	}

	chave := &settings.APIKey{TenantID: tenantVitima, Name: "chave da vítima", KeyPrefix: "toca_abcdef", IsActive: true}
	if err := apiKeyRepo.Create(ctx, chave); err != nil {
		t.Fatalf("create victim api key: %v", err)
	}
	chaveID, ok := chave.ID.(string)
	if !ok || chaveID == "" {
		t.Fatalf("Create deveria devolver o id gerado pelo RETURNING, veio %#v", chave.ID)
	}

	webhookDeletedAt := func(t *testing.T) *string {
		t.Helper()
		var deletedAt *string
		if err := db.Get(&deletedAt, "SELECT deleted_at::text FROM core.webhooks WHERE id = $1", vitimaID); err != nil {
			t.Fatalf("raw read webhook: %v", err)
		}
		return deletedAt
	}
	chaveAtiva := func(t *testing.T) bool {
		t.Helper()
		// Lê a MESMA chave JSON que a API devolve (APIKey.IsActive => isActive).
		// Sem COALESCE: uma chave ausente é falha, não "false".
		var ativa bool
		if err := db.Get(&ativa, "SELECT (metadata->>'isActive')::boolean FROM core.api_keys WHERE id = $1", chaveID); err != nil {
			t.Fatalf("raw read api key: %v", err)
		}
		return ativa
	}
	chavePelaAPI := func(t *testing.T) *settings.APIKey {
		t.Helper()
		lista, err := apiKeyRepo.ListByTenantID(ctx, tenantVitima)
		if err != nil {
			t.Fatalf("list api keys: %v", err)
		}
		for i := range lista {
			if lista[i].ID == chaveID {
				return &lista[i]
			}
		}
		t.Fatalf("chave não apareceu na listagem do próprio tenant: %+v", lista)
		return nil
	}

	t.Run("Delete_webhook_recusa_tenant_diferente", func(t *testing.T) {
		err := webhookRepo.Delete(ctx, tenantAtacante, vitimaID)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("esperava sql.ErrNoRows (0 linhas), veio %v", err)
		}
		if deletedAt := webhookDeletedAt(t); deletedAt != nil {
			t.Fatalf("EXCLUSÃO CROSS-TENANT: deleted_at=%v", *deletedAt)
		}
	})

	t.Run("Revoke_api_key_recusa_tenant_diferente", func(t *testing.T) {
		err := apiKeyRepo.Revoke(ctx, tenantAtacante, chaveID)
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("esperava sql.ErrNoRows (0 linhas), veio %v", err)
		}
		if !chaveAtiva(t) {
			t.Fatal("REVOGAÇÃO CROSS-TENANT: a chave da vítima foi desativada")
		}
	})

	t.Run("Delete_webhook_funciona_no_proprio_tenant", func(t *testing.T) {
		if err := webhookRepo.Delete(ctx, tenantVitima, vitimaID); err != nil {
			t.Fatalf("exclusão legítima falhou: %v", err)
		}
		if deletedAt := webhookDeletedAt(t); deletedAt == nil {
			t.Fatal("o webhook do próprio tenant deveria ter sido soft-deleted")
		}
	})

	t.Run("Delete_webhook_repetido_devolve_ErrNoRows", func(t *testing.T) {
		if err := webhookRepo.Delete(ctx, tenantVitima, vitimaID); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("segundo delete deveria devolver sql.ErrNoRows, veio %v", err)
		}
	})

	t.Run("Revoke_api_key_funciona_no_proprio_tenant", func(t *testing.T) {
		if err := apiKeyRepo.Revoke(ctx, tenantVitima, chaveID); err != nil {
			t.Fatalf("revogação legítima falhou: %v", err)
		}
		if chaveAtiva(t) {
			t.Fatal("a chave do próprio tenant deveria estar inativa em metadata")
		}
		// O que importa é o que a API devolve: IsActive=false na listagem.
		if ativa := chavePelaAPI(t).IsActive; ativa {
			t.Fatal("a revogação não chegou ao read model: a listagem continua mostrando isActive=true")
		}
	})

	// Regressão do `json:"-"`: o secret é write-only na API, mas precisa
	// continuar gravado em core.webhooks.metadata (é o que assina as entregas).
	t.Run("secret_e_persistido_mas_nunca_serializado", func(t *testing.T) {
		outro := &settings.WebhookEndpoint{TenantID: tenantVitima, URL: "https://vitima.example/outro", Active: true, Secret: "whsec_round_trip"}
		if err := webhookRepo.Create(ctx, outro); err != nil {
			t.Fatalf("create: %v", err)
		}

		var bruto string
		if err := db.Get(&bruto, "SELECT metadata::text FROM core.webhooks WHERE id = $1", outro.ID); err != nil {
			t.Fatalf("raw read: %v", err)
		}
		if !strings.Contains(bruto, "whsec_round_trip") {
			t.Fatalf("o signing secret não foi persistido em core.webhooks.metadata: %s", bruto)
		}

		lista, err := webhookRepo.ListByTenantID(ctx, tenantVitima)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		var encontrado *settings.WebhookEndpoint
		for i := range lista {
			if lista[i].ID == outro.ID {
				encontrado = &lista[i]
			}
		}
		if encontrado == nil {
			t.Fatalf("webhook recém-criado não apareceu na listagem do tenant: %+v", lista)
		}
		if encontrado.Secret != "whsec_round_trip" {
			t.Fatalf("o secret não sobreviveu ao round-trip: %q", encontrado.Secret)
		}
		if encontrado.ID != outro.ID {
			t.Fatalf("a listagem deve devolver o id da linha: %#v != %#v", encontrado.ID, outro.ID)
		}

		serializado, err := json.Marshal(encontrado)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if strings.Contains(string(serializado), "whsec_round_trip") || strings.Contains(string(serializado), `"secret"`) {
			t.Fatalf("o secret foi serializado para a API: %s", serializado)
		}
	})
}
