package repository_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

// A mesma pessoa pode ser corretora em duas imobiliárias: `id` é o account_id
// dela e a identidade do registro é (tenant_id, id). Com a chave primária só em
// `id`, o sync de membros da segunda empresa batia na linha da primeira, a RLS
// recusava a atualização e a rota devolvia 500 a cada carregamento.
//
// O teste aplica a migração de verdade sobre a tabela antiga e exercita o
// repositório nas duas empresas.
//
// Rodar:
//
//	createdb toca_review_qa_brokers
//	CRM_REVIEW_TEST_DATABASE_URL=postgres://.../toca_review_qa_brokers go test ./internal/repository/ -run TestBrokerPerTenantIdentity
func TestBrokerPerTenantIdentity(t *testing.T) {
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

	// Estado ANTERIOR à migração: chave primária só em `id`.
	if _, err := db.Exec(`CREATE SCHEMA crm;
		CREATE TABLE crm.brokers (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA crm CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})

	migration, err := os.ReadFile("../../../supabase/migrations/20261006120000_crm_brokers_per_tenant_identity.sql")
	if err != nil {
		t.Fatalf("ler migração: %v", err)
	}
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("aplicar migração: %v", err)
	}
	var definition string
	if err := db.Get(&definition, `SELECT pg_get_constraintdef(c.oid)
		FROM pg_constraint c WHERE c.conrelid = 'crm.brokers'::regclass AND c.contype = 'p'`); err != nil {
		t.Fatal(err)
	}
	if definition != "PRIMARY KEY (tenant_id, id)" {
		t.Fatalf("chave primária = %q, esperado PRIMARY KEY (tenant_id, id)", definition)
	}

	// Reexecutar a migração é inerte (deploy repetido não quebra).
	if _, err := db.Exec(string(migration)); err != nil {
		t.Fatalf("migração deveria ser idempotente: %v", err)
	}

	const (
		companyA = "11111111-1111-1111-1111-111111111111"
		companyB = "22222222-2222-2222-2222-222222222222"
		person   = "33333333-3333-3333-3333-333333333333"
	)
	ctx := context.Background()
	repo := repository.NewBrokerPostgresRepository(&database.PostgresDB{DB: db})

	// Mesma pessoa, mesma id, duas empresas: era exatamente aqui que a segunda
	// gravação estourava a política de RLS.
	first := broker.New("Bruno Exemplo", "corretor", "bruno@example.com", companyA)
	first.ID = person
	if err := repo.Create(ctx, first); err != nil {
		t.Fatalf("criar corretor na empresa A: %v", err)
	}
	second := broker.New("Bruno Exemplo", "corretor", "bruno@example.com", companyB)
	second.ID = person
	if err := repo.Create(ctx, second); err != nil {
		t.Fatalf("criar o mesmo corretor na empresa B: %v", err)
	}

	var rows int
	if err := db.Get(&rows, "SELECT COUNT(*) FROM crm.brokers WHERE id = $1", person); err != nil {
		t.Fatal(err)
	}
	if rows != 2 {
		t.Fatalf("esperava um registro por empresa, veio %d", rows)
	}

	// Cada empresa enxerga o seu, e o upsert continua atualizando o próprio.
	found, err := repo.GetByID(ctx, companyB, person)
	if err != nil || found == nil {
		t.Fatalf("empresa B deveria enxergar o corretor: found=%v err=%v", found, err)
	}
	updated := broker.New("Bruno Exemplo", "gestor", "bruno@example.com", companyB)
	updated.ID = person
	if err := repo.Create(ctx, updated); err != nil {
		t.Fatalf("upsert na empresa B: %v", err)
	}
	found, err = repo.GetByID(ctx, companyB, person)
	if err != nil || found == nil || found.Role != "gestor" {
		t.Fatalf("upsert deveria atualizar a empresa B: found=%+v err=%v", found, err)
	}
	untouched, err := repo.GetByID(ctx, companyA, person)
	if err != nil || untouched == nil || untouched.Role != "corretor" {
		t.Fatalf("empresa A não pode ser afetada: found=%+v err=%v", untouched, err)
	}
}
