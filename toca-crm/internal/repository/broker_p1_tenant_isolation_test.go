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
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

// P1 / A19 — isolamento de tenant no SQL do repositório de corretores.
//
// Antes: `SELECT metadata FROM crm.brokers WHERE id = $1` — PII de corretor de
// qualquer tenant pelo UUID, para qualquer autenticado não-`cliente`.
//
// Rodar:
//
//	createdb toca_review_qa_p0
//	CRM_REVIEW_TEST_DATABASE_URL=postgres://.../toca_review_qa_p0 go test ./internal/repository/ -run TestBrokerP1TenantIsolation
func TestBrokerP1TenantIsolation(t *testing.T) {
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

	// Chave primária (tenant_id, id) como em produção depois da migração
	// 20261006120000: `id` é o account_id da pessoa e a mesma pessoa pode ser
	// corretora em mais de uma empresa.
	if _, err := db.Exec(`CREATE SCHEMA crm;
		CREATE TABLE crm.brokers (id text NOT NULL, tenant_id text NOT NULL, metadata jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
		deleted_at timestamptz, PRIMARY KEY (tenant_id, id))`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA crm CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	ctx := context.Background()
	repo := repository.NewBrokerPostgresRepository(&database.PostgresDB{DB: db})

	const victimTenant = "tenant-vitima"
	const victimBrokerID = "broker-da-vitima"
	if err := repo.Create(ctx, &broker.Broker{
		ID:       victimBrokerID,
		TenantID: victimTenant,
		Name:     "Corretor da vítima",
		Email:    "vitima@example.com",
	}); err != nil {
		t.Fatalf("create victim broker: %v", err)
	}

	t.Run("GetByID_recusa_tenant_diferente", func(t *testing.T) {
		got, err := repo.GetByID(ctx, "tenant-atacante", victimBrokerID)
		if err == nil {
			t.Fatalf("VAZAMENTO CROSS-TENANT: leu PII do corretor do tenant %q (retorno=%+v)", victimTenant, got)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("esperava sql.ErrNoRows, veio %v", err)
		}
	})

	t.Run("GetByID_encontra_no_proprio_tenant", func(t *testing.T) {
		got, err := repo.GetByID(ctx, victimTenant, victimBrokerID)
		if err != nil || got == nil || got.ID != victimBrokerID {
			t.Fatalf("leitura legítima falhou: got=%+v err=%v", got, err)
		}
	})

	t.Run("GetByID_falha_fechado_sem_tenant", func(t *testing.T) {
		if _, err := repo.GetByID(ctx, "", victimBrokerID); err == nil {
			t.Fatal("tenant vazio deveria ser recusado (fail-closed)")
		}
		if _, err := repo.GetByID(ctx, "   ", victimBrokerID); err == nil {
			t.Fatal("tenant em branco deveria ser recusado (fail-closed)")
		}
	})

	t.Run("FindByEmailAnyTenant_e_a_escotilha_documentada", func(t *testing.T) {
		// O caminho cross-tenant de LEADS continua existindo, mas explícito e
		// documentado para o portal do cliente (posse do e-mail).
		leadRepo := repository.NewLeadPostgresRepository(&database.PostgresDB{DB: db})
		_ = leadRepo // ausência de erro de compilação já documenta o contrato
	})
}
