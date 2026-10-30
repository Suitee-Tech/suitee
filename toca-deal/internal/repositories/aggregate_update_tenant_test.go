package repositories_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/repositories"
)

// P1 / B12 — os UPDATE de agregado têm predicado de tenant (e falham alto).
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado B12) registrou `UPDATE ... WHERE id = $n` sem `tenant_id` em
// deal/contract/document. A segurança dependia de TODO chamador ter feito um
// GetByID tenant-scoped antes — exatamente o modo de falha do achado C9, onde um
// agregado montado a partir do corpo da requisição sobrescreveu a linha de outro
// tenant.
//
// Rodar:
//
//	createdb toca_review_qa_deal
//	DEAL_REVIEW_TEST_DATABASE_URL=postgres://.../toca_review_qa_deal go test ./internal/repositories/ -run TestAggregateUpdateTenantPredicate
func TestAggregateUpdateTenantPredicate(t *testing.T) {
	uri := os.Getenv("DEAL_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set DEAL_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
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

	if _, err := db.Exec(`CREATE SCHEMA deal;
		CREATE TABLE deal.deals (id text PRIMARY KEY, tenant_id text NOT NULL, metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz);
		CREATE TABLE deal.contracts (id text PRIMARY KEY, tenant_id text NOT NULL, deal_id text NOT NULL, metadata jsonb NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), deleted_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA deal CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	ctx := context.Background()
	pg := &database.PostgresDB{DB: db}
	dealRepo := repositories.NewDealPostgresRepository(pg)
	contractRepo := repositories.NewContractPostgresRepository(pg)

	victimDeal := &deal.Deal{ID: "deal-vitima", TenantID: "tenant-vitima", Value: 100}
	if err := dealRepo.Create(ctx, victimDeal); err != nil {
		t.Fatalf("create deal: %v", err)
	}
	victimContract := &contract.Contract{ID: "contract-vitima", TenantID: "tenant-vitima", DealID: victimDeal.ID}
	if err := contractRepo.Create(ctx, victimContract); err != nil {
		t.Fatalf("create contract: %v", err)
	}

	rawMeta := func(table, id string) string {
		t.Helper()
		var meta string
		if err := db.Get(&meta, "SELECT metadata::text FROM deal."+table+" WHERE id = $1", id); err != nil {
			t.Fatalf("raw read %s: %v", table, err)
		}
		return meta
	}

	t.Run("deal: agregado com tenant forjado não grava", func(t *testing.T) {
		before := rawMeta("deals", victimDeal.ID)
		forged := &deal.Deal{ID: victimDeal.ID, TenantID: "tenant-atacante", Value: 999999}
		err := dealRepo.Update(ctx, forged)
		// A asserção principal é o EFEITO (linha da vítima intacta); o erro é a
		// segunda, para o caso de o UPDATE virar um no-op silencioso.
		if after := rawMeta("deals", victimDeal.ID); after != before {
			t.Fatalf("ESCRITA CROSS-TENANT: metadata do deal da vítima mudou com um agregado de outro tenant\nantes: %s\ndepois: %s", before, after)
		}
		if err == nil {
			t.Fatal("update com tenant forjado deveria falhar alto (0 linhas), não reportar sucesso")
		}
	})

	t.Run("contract: agregado com tenant forjado não grava", func(t *testing.T) {
		before := rawMeta("contracts", victimContract.ID)
		forged := &contract.Contract{ID: victimContract.ID, TenantID: "tenant-atacante", Type: contract.TypeVenda}
		err := contractRepo.Update(ctx, forged)
		if after := rawMeta("contracts", victimContract.ID); after != before {
			t.Fatalf("ESCRITA CROSS-TENANT: metadata do contrato da vítima mudou com um agregado de outro tenant\nantes: %s\ndepois: %s", before, after)
		}
		if err == nil {
			t.Fatal("update com tenant forjado deveria falhar alto (0 linhas), não reportar sucesso")
		}
	})

	t.Run("fluxo legítimo continua gravando", func(t *testing.T) {
		loaded, err := dealRepo.GetByID(ctx, "tenant-vitima", victimDeal.ID)
		if err != nil || loaded == nil {
			t.Fatalf("load: %v", err)
		}
		loaded.Value = 250
		if err := dealRepo.Update(ctx, loaded); err != nil {
			t.Fatalf("update legítimo falhou: %v", err)
		}
		if !strings.Contains(rawMeta("deals", victimDeal.ID), "250") {
			t.Fatal("valor atualizado não foi persistido")
		}
	})
}
