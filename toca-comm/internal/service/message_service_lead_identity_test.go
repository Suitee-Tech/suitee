package service

import (
	"context"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// Incidente "corretor inicia a conversa com o lead, o lead é convidado para o
// portal e, com acesso, não vê a mensagem enviada" (2026-09-21).
//
// O canal é aberto pelo LEAD (é o id que a busca de participantes do painel
// devolve), mas o portal autentica pela CONTA do cliente
// (`crm.leads.metadata->>'AccountID'`). A igualdade exata de id negava o canal
// ao próprio dono: listagem vazia ("Nenhuma conversa disponível"), 404 ao
// resolver o canal e 403 ao enviar. Estes testes travam as três pontas da
// correção — a escrita grava a conta, a leitura casa o conjunto de identidades e
// o canal antigo é curado para a conta.

const (
	leadDoCliente   = "4de1b561-919b-45b8-ad01-432859a7c57a" // crm.leads.id
	contaDoCliente  = "5b11063c-a7df-4a2e-b1fd-cfe9cdab31e6" // crm.leads.AccountID
	corretorDoCanal = "c7f105dd-3e68-435b-a4cc-ea456ea52551" // crm.brokers.id = conta do corretor
	tenantDoCanal   = "de1c7bb5-b93f-4d4d-86e7-9430f94c404b"
)

// Canal do incidente: corretor + LEAD (sem a conta do cliente).
func canalNomeadoPeloLead() *message.Channel {
	return &message.Channel{
		ID:           "canal-anuncio-marcelo",
		TenantID:     tenantDoCanal,
		Name:         "Anúncio Marcelo",
		Type:         message.ChannelTypeDirect,
		Status:       message.ChannelStatusOpen,
		CreatedBy:    corretorDoCanal,
		Participants: []string{corretorDoCanal, leadDoCliente},
	}
}

func repoDoIncidente(ch *message.Channel) *a9Repo {
	return &a9Repo{
		channel:         ch,
		leadsByAccount:  map[string][]string{contaDoCliente: {leadDoCliente}},
		accountsByLead:  map[string]string{leadDoCliente: contaDoCliente},
		belongsToTenant: map[string]bool{corretorDoCanal: true, contaDoCliente: true},
	}
}

// A listagem leva ao SQL o conjunto de identidades do cliente: só assim o canal
// nomeado pelo lead entra na lista do portal, que consulta pela conta.
func TestListChannelsByParticipantUsesTheClientIdentitySet(t *testing.T) {
	repo := repoDoIncidente(canalNomeadoPeloLead())
	svc := NewMessageService(repo, nil, nil, nil)

	if _, err := svc.ListChannelsByParticipant(context.Background(), tenantDoCanal, contaDoCliente); err != nil {
		t.Fatalf("ListChannelsByParticipant: %v", err)
	}
	if len(repo.listedIdentities) != 1 {
		t.Fatalf("esperava 1 consulta, veio %d", len(repo.listedIdentities))
	}
	got := repo.listedIdentities[0]
	if len(got) != 2 || got[0] != contaDoCliente || got[1] != leadDoCliente {
		t.Fatalf("conjunto de identidades errado: %+v", got)
	}

	// O cliente do portal não tem o tenant da imobiliária no token: sem recorte
	// de tenant, o vínculo do lead continua sendo resolvido.
	if _, err := svc.ListChannelsByParticipant(context.Background(), "", contaDoCliente); err != nil {
		t.Fatalf("ListChannelsByParticipant sem tenant: %v", err)
	}
	if len(repo.listedIdentities) != 2 || len(repo.listedIdentities[1]) != 2 {
		t.Fatalf("sem tenant o vínculo do lead foi perdido: %+v", repo.listedIdentities)
	}
}

// A leitura autoriza o dono do lead e cura o canal: o participante passa a ser a
// conta, que é o id usado pelo portal para listar, enviar e receber em tempo real.
func TestCheckParticipant_AuthorizesLeadOwnerAndHealsChannel(t *testing.T) {
	ch := canalNomeadoPeloLead()
	repo := repoDoIncidente(ch)
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(context.Background(), contaDoCliente)

	if err := svc.checkParticipant(ctx, ch); err != nil {
		t.Fatalf("dono do lead recusado no próprio canal: %v", err)
	}
	if repo.replacedFrom != leadDoCliente || repo.replacedTo != contaDoCliente {
		t.Fatalf("canal não foi curado para a conta: from=%q to=%q", repo.replacedFrom, repo.replacedTo)
	}
	for _, p := range ch.Participants {
		if p == leadDoCliente {
			t.Fatalf("participante ainda nomeia o lead depois da cura: %+v", ch.Participants)
		}
	}
	if len(ch.Participants) != 2 {
		t.Fatalf("cura duplicou/removeu participantes: %+v", ch.Participants)
	}
}

// A identidade por lead NÃO abre canal de outro cliente: a resolução só devolve
// os leads DA conta autenticada.
func TestCheckParticipant_DoesNotAuthorizeOtherClientsLeadChannel(t *testing.T) {
	ch := canalNomeadoPeloLead()
	repo := &a9Repo{
		channel: ch,
		// O intruso não tem lead nenhum vinculado à conta dele.
		leadsByAccount: map[string][]string{},
	}
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(context.Background(), "acc-de-outro-cliente")

	if err := svc.checkParticipant(ctx, ch); err == nil {
		t.Fatal("VAZAMENTO: conta sem vínculo com o lead entrou no canal")
	}
	if repo.replacedFrom != "" {
		t.Fatalf("canal foi curado por quem não é dono: %+v", repo)
	}
}

// O canal de outro tenant continua sendo alcançado pelo fallback por
// participação — agora com o lead reconhecido como o cliente do portal.
func TestResolveChannel_FallbackMatchesTheClientsLeadIdentity(t *testing.T) {
	ch := canalNomeadoPeloLead()
	svc := NewMessageService(repoDoIncidente(ch), nil, nil, nil)

	ctx := pkgcontext.WithAccountID(context.Background(), contaDoCliente)
	got, err := svc.resolveChannel(ctx, ch.ID, contaDoCliente)
	if err != nil {
		t.Fatalf("cliente do portal não alcançou o canal do seu lead: %v", err)
	}
	if got.ID != ch.ID {
		t.Fatalf("canal errado: %+v", got)
	}
}

// A adição de participante grava a CONTA: repetir o fluxo do painel da
// imobiliária (buscar o lead e adicionar) não pode recriar o defeito.
func TestAddParticipant_StoresThePortalAccountInsteadOfTheLead(t *testing.T) {
	ch := &message.Channel{
		ID:           "canal-novo",
		TenantID:     tenantDoCanal,
		Status:       message.ChannelStatusOpen,
		Participants: []string{corretorDoCanal},
	}
	repo := repoDoIncidente(ch)
	svc := NewMessageService(repo, nil, nil, nil)

	ctx := pkgcontext.WithAccountID(context.Background(), corretorDoCanal)
	ctx = pkgcontext.WithTenantID(ctx, tenantDoCanal)
	if err := svc.AddParticipant(ctx, ch.ID, leadDoCliente); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}

	hasAccount := false
	for _, p := range repo.writtenParticipants {
		if p == leadDoCliente {
			t.Fatalf("canal gravou o lead como participante: %+v", repo.writtenParticipants)
		}
		if p == contaDoCliente {
			hasAccount = true
		}
	}
	if !hasAccount {
		t.Fatalf("canal sem a conta do cliente: %+v", repo.writtenParticipants)
	}
}

// Sem conta vinculada (cliente ainda não convidado) o lead é aceito — o convite
// posterior é reconhecido pela autorização, não por uma reescrita do painel.
func TestAddParticipant_KeepsTheLeadWhileThereIsNoPortalAccount(t *testing.T) {
	ch := &message.Channel{
		ID:           "canal-novo",
		TenantID:     tenantDoCanal,
		Status:       message.ChannelStatusOpen,
		Participants: []string{corretorDoCanal},
	}
	repo := &a9Repo{channel: ch, belongsToTenant: map[string]bool{corretorDoCanal: true, leadDoCliente: true}}
	svc := NewMessageService(repo, nil, nil, nil)

	ctx := pkgcontext.WithAccountID(context.Background(), corretorDoCanal)
	ctx = pkgcontext.WithTenantID(ctx, tenantDoCanal)
	if err := svc.AddParticipant(ctx, ch.ID, leadDoCliente); err != nil {
		t.Fatalf("AddParticipant: %v", err)
	}

	found := false
	for _, p := range repo.writtenParticipants {
		if p == leadDoCliente {
			found = true
		}
	}
	if !found {
		t.Fatalf("lead sem conta deveria continuar participante: %+v", repo.writtenParticipants)
	}
}
