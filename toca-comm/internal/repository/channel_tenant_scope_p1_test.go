package repository

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// P1/A3 + A9 — isolamento de tenant no SQL de canais (não só no handler).
//
// O dublê em memória dos testes de handler aplica o filtro de tenant sozinho e,
// por isso, NÃO prova que o `WHERE` do repositório tem o predicado: remover
// `tenant_id = $1` do SQL passaria despercebido. Este teste roda contra um
// Postgres descartável com nome obrigatoriamente `toca_review_qa_*` (mesmo
// harness de toca-core/toca-crm/toca-ops) e é skip por padrão:
//
//	createdb toca_review_qa_comm_p1
//	COMM_REVIEW_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:54322/toca_review_qa_comm_p1?sslmode=disable' \
//	  go test ./internal/repository/ -run 'TestCommP1' -v
func TestCommP1ChannelTenantScope(t *testing.T) {
	uri := os.Getenv("COMM_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set COMM_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
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

	// Os tipos precisam ESPELHAR a produção: `tenant_id` é UUID nas migrations.
	// A versão anterior deste harness criava as colunas como TEXT e, com isso,
	// escondia um bug real de tipo — o cliente do portal recebia
	// `operator does not exist: uuid = text (SQLSTATE 42883)` na listagem de
	// canais quando o tenant ia vazio (o parâmetro texto não tem cast implícito
	// para a coluna uuid). Recriar as tabelas garante o tipo certo mesmo em QA
	// reaproveitado.
	if _, err := db.ExecContext(ctx, `
		DROP TABLE IF EXISTS comm.channels, crm.leads, crm.brokers;
		CREATE SCHEMA IF NOT EXISTS comm;
		CREATE TABLE IF NOT EXISTS comm.channels (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ);
		CREATE SCHEMA IF NOT EXISTS crm;
		CREATE SCHEMA IF NOT EXISTS toca_auth;
		CREATE TABLE IF NOT EXISTS toca_auth.accounts (
			id UUID PRIMARY KEY,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		CREATE TABLE IF NOT EXISTS crm.brokers (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ);
		CREATE TABLE IF NOT EXISTS crm.leads (
			id UUID PRIMARY KEY,
			tenant_id UUID NOT NULL,
			metadata JSONB NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			deleted_at TIMESTAMPTZ);
	`); err != nil {
		t.Fatalf("create comm schema: %v", err)
	}

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()
	participante := uuid.NewString()

	repo := NewMessagePostgresRepository(db)
	canais := []*message.Channel{
		message.NewChannel(tenantA, "canal do tenant A", message.ChannelTypeDeal, "acc-1", []string{participante}),
		message.NewChannel(tenantB, "canal do tenant B", message.ChannelTypeDeal, "acc-1", []string{participante}),
	}
	for _, ch := range canais {
		if err := repo.CreateChannel(ctx, ch); err != nil {
			t.Fatalf("seed channel: %v", err)
		}
	}

	t.Run("listagem por participante com tenant não vaza outro tenant", func(t *testing.T) {
		got, err := repo.ListChannelsByParticipant(ctx, tenantA, []string{participante})
		if err != nil {
			t.Fatalf("ListChannelsByParticipant: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("esperava 1 canal de %s, veio %d: %+v", tenantA, len(got), got)
		}
		if got[0].TenantID != tenantA {
			t.Fatalf("VAZAMENTO CROSS-TENANT no SQL: %s listou canal de %s", tenantA, got[0].TenantID)
		}
	})

	t.Run("listagem por participante sem tenant é o caminho do cliente do portal", func(t *testing.T) {
		got, err := repo.ListChannelsByParticipant(ctx, "", []string{participante})
		if err != nil {
			t.Fatalf("ListChannelsByParticipant: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("sem recorte de tenant a listagem é por participação (2 canais), veio %d", len(got))
		}
	})

	t.Run("GetChannel exige o tenant do canal", func(t *testing.T) {
		if _, err := repo.GetChannel(ctx, canais[0].ID, tenantB); err == nil {
			t.Fatal("VAZAMENTO: GetChannel devolveu o canal de outro tenant")
		}
	})

	t.Run("GetChannelForParticipant exige participação", func(t *testing.T) {
		if _, err := repo.GetChannelForParticipant(ctx, canais[0].ID, []string{uuid.NewString()}); err == nil {
			t.Fatal("VAZAMENTO: não-participante alcançou o canal pelo id")
		}
		ch, err := repo.GetChannelForParticipant(ctx, canais[0].ID, []string{participante})
		if err != nil {
			t.Fatalf("participante deveria alcançar o canal: %v", err)
		}
		if ch.ID != canais[0].ID {
			t.Fatalf("canal errado: %s", ch.ID)
		}
	})

	t.Run("escrita de participantes não toca canal de outro tenant", func(t *testing.T) {
		err := repo.UpdateChannelParticipants(ctx, canais[0].ID, tenantB, []string{participante, uuid.NewString()})
		if err == nil {
			t.Fatal("escrita de participantes com tenant errado deveria falhar (0 linhas)")
		}
		atual, err := repo.GetChannel(ctx, canais[0].ID, tenantA)
		if err != nil {
			t.Fatalf("GetChannel: %v", err)
		}
		if len(atual.Participants) != 1 {
			t.Fatalf("os participantes do canal foram alterados por um tenant estrangeiro: %v", atual.Participants)
		}
	})

	t.Run("validação de participante usa as fontes do tenant", func(t *testing.T) {
		leadID := uuid.NewString()
		accountID := uuid.NewString()
		if _, err := db.ExecContext(ctx, `
			INSERT INTO crm.leads (id, tenant_id, metadata) VALUES ($1, $2, $3::jsonb)
		`, leadID, tenantA, `{"AccountID":"`+accountID+`"}`); err != nil {
			t.Fatalf("seed lead: %v", err)
		}

		ok, err := repo.AccountBelongsToTenant(ctx, tenantA, accountID)
		if err != nil {
			t.Fatalf("AccountBelongsToTenant: %v", err)
		}
		if !ok {
			t.Fatal("lead do tenant (por AccountID) deveria ser aceito")
		}

		ok, err = repo.AccountBelongsToTenant(ctx, tenantB, accountID)
		if err != nil {
			t.Fatalf("AccountBelongsToTenant: %v", err)
		}
		if ok {
			t.Fatal("VAZAMENTO CROSS-TENANT: conta do tenant A aceita como participante do tenant B")
		}
	})
}
