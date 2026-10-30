package repository

import (
	"context"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// Incidente "corretor inicia a conversa com o lead, o cliente é convidado para o
// portal e não vê a mensagem" (2026-09-21) — prova do SQL, não do serviço.
//
// O dublê em memória dos testes de serviço implementa a resolução de identidade
// sozinho e, por isso, não prova que o `WHERE` casa o CONJUNTO de identidades
// (`?|`), nem que a cura troca o lead pela conta sem perder os demais
// participantes. Este teste roda contra um Postgres descartável com nome
// obrigatoriamente `toca_review_qa_*` (mesmo harness do teste de tenant) e é skip
// por padrão:
//
//	createdb toca_review_qa_comm_ident
//	COMM_REVIEW_TEST_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:54322/toca_review_qa_comm_ident?sslmode=disable' \
//	  go test ./internal/repository/ -run 'TestCommP1ParticipantIdentity' -v
func TestCommP1ParticipantIdentity(t *testing.T) {
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

	// Tipos espelhando a produção (tenant_id UUID, metadata JSONB).
	if _, err := db.ExecContext(ctx, `
		DROP TABLE IF EXISTS comm.channels, crm.leads;
		CREATE SCHEMA IF NOT EXISTS comm;
		CREATE SCHEMA IF NOT EXISTS crm;
		CREATE TABLE IF NOT EXISTS comm.channels (
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
		t.Fatalf("create schema: %v", err)
	}

	tenantDaImobiliaria := uuid.NewString()
	outraImobiliaria := uuid.NewString()
	corretor := uuid.NewString() // crm.brokers.id = conta do corretor
	leadDoCliente := uuid.NewString()
	contaDoPortal := uuid.NewString()
	leadDeOutroCliente := uuid.NewString()
	contaDeOutroCliente := uuid.NewString()

	for _, lead := range []struct{ id, tenant, account string }{
		{leadDoCliente, tenantDaImobiliaria, contaDoPortal},
		{leadDeOutroCliente, tenantDaImobiliaria, contaDeOutroCliente},
	} {
		if _, err := db.ExecContext(ctx, `
			INSERT INTO crm.leads (id, tenant_id, metadata) VALUES ($1, $2, $3::jsonb)
		`, lead.id, lead.tenant, `{"AccountID":"`+lead.account+`"}`); err != nil {
			t.Fatalf("seed lead: %v", err)
		}
	}

	repo := NewMessagePostgresRepository(db)

	// O canal do incidente: corretor + LEAD (o painel escolhe o lead na busca de
	// participantes; a conta do portal não estava na lista).
	canalDoIncidente := message.NewChannel(tenantDaImobiliaria, "Anúncio Marcelo", message.ChannelTypeDirect, corretor, []string{corretor, leadDoCliente})
	if err := repo.CreateChannel(ctx, canalDoIncidente); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	// Canal de outro cliente da mesma imobiliária: não pode entrar na lista.
	canalDeOutroCliente := message.NewChannel(tenantDaImobiliaria, "Outro atendimento", message.ChannelTypeDirect, corretor, []string{corretor, leadDeOutroCliente})
	if err := repo.CreateChannel(ctx, canalDeOutroCliente); err != nil {
		t.Fatalf("seed channel: %v", err)
	}
	// Canal de OUTRA imobiliária com o mesmo cliente: entra na lista sem tenant.
	canalDaOutraImobiliaria := message.NewChannel(outraImobiliaria, "Outra imobiliária", message.ChannelTypeDirect, corretor, []string{corretor, contaDoPortal})
	if err := repo.CreateChannel(ctx, canalDaOutraImobiliaria); err != nil {
		t.Fatalf("seed channel: %v", err)
	}

	t.Run("direct contact reuses legacy channel within the agency", func(t *testing.T) {
		ch, err := repo.FindDirectChannel(ctx, tenantDaImobiliaria, []string{corretor, contaDoPortal})
		if err != nil || ch.ID != canalDoIncidente.ID {
			t.Fatalf("legacy match: %+v %v", ch, err)
		}
		ch, err = repo.FindDirectChannel(ctx, outraImobiliaria, []string{corretor, contaDoPortal})
		if err != nil || ch.ID != canalDaOutraImobiliaria.ID {
			t.Fatalf("tenant match: %+v %v", ch, err)
		}
		if _, err = repo.FindDirectChannel(ctx, uuid.NewString(), []string{corretor, contaDoPortal}); err == nil {
			t.Fatal("matched another agency")
		}
	})

	t.Run("ClientAccountForLead traduz lead → conta e respeita o tenant", func(t *testing.T) {
		got, err := repo.ClientAccountForLead(ctx, tenantDaImobiliaria, leadDoCliente)
		if err != nil {
			t.Fatalf("ClientAccountForLead: %v", err)
		}
		if got != contaDoPortal {
			t.Fatalf("conta do lead = %q, esperado %q", got, contaDoPortal)
		}

		got, err = repo.ClientAccountForLead(ctx, outraImobiliaria, leadDoCliente)
		if err != nil {
			t.Fatalf("ClientAccountForLead (outro tenant): %v", err)
		}
		if got != "" {
			t.Fatalf("VAZAMENTO CROSS-TENANT: lead do tenant A traduzido a partir do tenant B (%q)", got)
		}

		got, err = repo.ClientAccountForLead(ctx, tenantDaImobiliaria, uuid.NewString())
		if err != nil {
			t.Fatalf("ClientAccountForLead (lead inexistente): %v", err)
		}
		if got != "" {
			t.Fatalf("lead inexistente devolveu conta: %q", got)
		}
	})

	t.Run("LeadIDsForAccount devolve as outras identidades do cliente", func(t *testing.T) {
		got, err := repo.LeadIDsForAccount(ctx, tenantDaImobiliaria, contaDoPortal)
		if err != nil {
			t.Fatalf("LeadIDsForAccount: %v", err)
		}
		if len(got) != 1 || got[0] != leadDoCliente {
			t.Fatalf("leads da conta = %v, esperado [%s]", got, leadDoCliente)
		}

		// Sem tenant (token do cliente do portal): busca em qualquer imobiliária.
		got, err = repo.LeadIDsForAccount(ctx, "", contaDoPortal)
		if err != nil {
			t.Fatalf("LeadIDsForAccount sem tenant: %v", err)
		}
		if len(got) != 1 || got[0] != leadDoCliente {
			t.Fatalf("leads da conta sem tenant = %v, esperado [%s]", got, leadDoCliente)
		}
	})

	t.Run("listagem por conjunto de identidades acha o canal nomeado pelo lead", func(t *testing.T) {
		identidades := []string{contaDoPortal, leadDoCliente}
		got, err := repo.ListChannelsByParticipant(ctx, tenantDaImobiliaria, identidades)
		if err != nil {
			t.Fatalf("ListChannelsByParticipant: %v", err)
		}
		ids := map[string]bool{}
		for _, ch := range got {
			ids[ch.ID] = true
		}
		if !ids[canalDoIncidente.ID] {
			t.Fatalf("canal nomeado pelo lead ficou de fora: %+v", ids)
		}
		if ids[canalDeOutroCliente.ID] {
			t.Fatal("VAZAMENTO: canal de OUTRO cliente entrou na lista")
		}

		// Só a conta (sem o vínculo do lead) não deveria achar o canal — é o
		// defeito original; a identidade do lead é o que o corrige.
		got, err = repo.ListChannelsByParticipant(ctx, tenantDaImobiliaria, []string{contaDoPortal})
		if err != nil {
			t.Fatalf("ListChannelsByParticipant (só conta): %v", err)
		}
		if len(got) != 0 {
			t.Fatalf("o canal ainda é achado sem o vínculo do lead: %+v", got)
		}
	})

	t.Run("GetChannelForParticipant aceita o vínculo do lead e recusa estranhos", func(t *testing.T) {
		ch, err := repo.GetChannelForParticipant(ctx, canalDoIncidente.ID, []string{contaDoPortal, leadDoCliente})
		if err != nil {
			t.Fatalf("dono do lead não alcançou o canal: %v", err)
		}
		if ch.ID != canalDoIncidente.ID {
			t.Fatalf("canal errado: %s", ch.ID)
		}

		if _, err := repo.GetChannelForParticipant(ctx, canalDoIncidente.ID, []string{contaDeOutroCliente, leadDeOutroCliente}); err == nil {
			t.Fatal("VAZAMENTO: outro cliente alcançou o canal pelo id")
		}
	})

	t.Run("cura troca o lead pela conta sem perder os demais participantes", func(t *testing.T) {
		if err := repo.ReplaceChannelParticipant(ctx, canalDoIncidente.ID, tenantDaImobiliaria, leadDoCliente, contaDoPortal); err != nil {
			t.Fatalf("ReplaceChannelParticipant: %v", err)
		}

		atual, err := repo.GetChannel(ctx, canalDoIncidente.ID, tenantDaImobiliaria)
		if err != nil {
			t.Fatalf("GetChannel: %v", err)
		}
		hasCorretor, hasConta, hasLead := false, false, false
		for _, p := range atual.Participants {
			switch p {
			case corretor:
				hasCorretor = true
			case contaDoPortal:
				hasConta = true
			case leadDoCliente:
				hasLead = true
			}
		}
		if !hasCorretor || !hasConta || hasLead {
			t.Fatalf("cura incorreta: %+v", atual.Participants)
		}
		if len(atual.Participants) != 2 {
			t.Fatalf("cura duplicou participantes: %+v", atual.Participants)
		}

		// Idempotente: repetir não altera nem cria linhas fantasmas.
		if err := repo.ReplaceChannelParticipant(ctx, canalDoIncidente.ID, tenantDaImobiliaria, leadDoCliente, contaDoPortal); err == nil {
			t.Fatal("cura repetida deveria ser no-op (0 linhas) para o lead já ausente")
		}
		repetido, err := repo.GetChannel(ctx, canalDoIncidente.ID, tenantDaImobiliaria)
		if err != nil {
			t.Fatalf("GetChannel: %v", err)
		}
		if len(repetido.Participants) != 2 {
			t.Fatalf("cura repetida alterou participantes: %+v", repetido.Participants)
		}
	})

	t.Run("cura não atravessa o tenant do canal", func(t *testing.T) {
		err := repo.ReplaceChannelParticipant(ctx, canalDeOutroCliente.ID, outraImobiliaria, leadDeOutroCliente, contaDeOutroCliente)
		if err == nil {
			t.Fatal("cura com tenant errado deveria falhar (0 linhas)")
		}
		atual, err := repo.GetChannel(ctx, canalDeOutroCliente.ID, tenantDaImobiliaria)
		if err != nil {
			t.Fatalf("GetChannel: %v", err)
		}
		if len(atual.Participants) != 2 || atual.Participants[1] != leadDeOutroCliente {
			t.Fatalf("canal de outro tenant foi alterado: %+v", atual.Participants)
		}
	})
	t.Run("portal participation works with tenant RLS enabled", func(t *testing.T) {
		role := "contact_qa_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		password := uuid.NewString()
		if _, err := db.ExecContext(ctx, "CREATE ROLE "+role+" LOGIN NOBYPASSRLS PASSWORD '"+password+"'"); err != nil {
			t.Fatal(err)
		}
		defer func() {
			// This role owns no objects. Revoke its explicit grants without
			// granting the test runner membership in the temporary role.
			_, err := db.ExecContext(ctx, "REVOKE ALL ON comm.channels, crm.leads FROM "+role+"; REVOKE ALL ON SCHEMA comm, crm FROM "+role+"; DROP ROLE "+role)
			if err != nil {
				t.Errorf("clean up temporary RLS role: %v", err)
			}
		}()
		if _, err := db.ExecContext(ctx, "GRANT USAGE ON SCHEMA comm, crm TO "+role+"; GRANT SELECT, UPDATE ON comm.channels TO "+role+"; GRANT SELECT ON crm.leads TO "+role); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, `ALTER TABLE comm.channels ENABLE ROW LEVEL SECURITY;
   CREATE POLICY contact_tenant_scope ON comm.channels USING
    (tenant_id::text = current_setting('request.jwt.claims', true)::jsonb->'app_metadata'->>'tenant_id');`); err != nil {
			t.Fatal(err)
		}
		appURL, err := url.Parse(uri)
		if err != nil {
			t.Fatal(err)
		}
		appURL.User = url.UserPassword(role, password)
		scopedDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURL.String(), ServiceURI: uri, MaxPoolSize: 2, MinPoolSize: 1})
		if err != nil {
			t.Fatal(err)
		}
		defer scopedDB.Close()
		scopedRepo := NewMessagePostgresRepository(scopedDB)
		portalCtx := pkgcontext.WithTenantID(pkgcontext.WithAccountID(ctx, contaDoPortal), contaDoPortal)
		// The application pool is genuinely blocked by the portal token's tenant.
		if _, err := scopedRepo.GetChannel(portalCtx, canalDoIncidente.ID, tenantDaImobiliaria); err == nil {
			t.Fatal("fixture bypasses RLS")
		}
		channels, err := scopedRepo.ListChannelsByParticipant(portalCtx, "", []string{contaDoPortal, leadDoCliente})
		if err != nil || len(channels) != 2 {
			t.Fatalf("participant discovery under RLS: %v %v", channels, err)
		}
		ch, err := scopedRepo.GetChannelForParticipant(portalCtx, canalDoIncidente.ID, []string{contaDoPortal, leadDoCliente})
		if err != nil || ch.ID != canalDoIncidente.ID {
			t.Fatalf("participant lookup under RLS: %+v %v", ch, err)
		}
		if _, err := scopedRepo.GetChannelForParticipant(portalCtx, canalDeOutroCliente.ID, []string{contaDoPortal, leadDoCliente}); err == nil {
			t.Fatal("other client's channel disclosed")
		}
		channelCtx := pkgcontext.WithTenantID(portalCtx, tenantDaImobiliaria)
		if _, err := scopedRepo.GetChannel(channelCtx, canalDoIncidente.ID, tenantDaImobiliaria); err != nil {
			t.Fatalf("authorized channel scope: %v", err)
		}
	})

}
