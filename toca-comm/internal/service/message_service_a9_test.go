package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

// P1/A9 — autorização de canal fail-closed.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A9) mostrou três furos no mesmo caminho de leitura:
//
//  1. `checkParticipant` devolvia `nil` (autorizado) quando não havia account id
//     no contexto — "system call or no user context";
//  2. `resolveChannel` caía para uma busca de canal **sem tenant** em QUALQUER
//     erro de `GetChannel`, inclusive falha de banco, e a partir daí a única
//     trava era a lista de participantes;
//  3. a busca sem tenant (`GetChannelByID`) não exigia participação nenhuma.
//
// Estes testes são unitários de propósito: cada trava é verificada isolada, para
// que a remoção de uma delas falhe um teste específico (ver
// docs/reviews/evidence/p1/A9-mutation.txt).

type a9Repo struct {
	// ErrChannelNotFound é o que o repositório real devolve quando o canal não
	// existe para aquele tenant.
	channelErr error
	// byID simula `GetChannelForParticipant`: devolve o canal só se ALGUMA das
	// identidades informadas estiver na lista de participantes.
	channel *message.Channel
	// leadsByAccount simula `LeadIDsForAccount` (as outras identidades do
	// cliente) e accountsByLead simula `ClientAccountForLead` (lead → conta).
	leadsByAccount map[string][]string
	accountsByLead map[string]string
	// belongsToTenant simula `AccountBelongsToTenant` (quem tem vínculo com o
	// tenant: lead por id/AccountID ou corretor).
	belongsToTenant map[string]bool
	// writtenParticipants registra a última escrita de participantes.
	writtenParticipants []string
	// listedIdentities registra os conjuntos de identidades levados ao SQL de
	// listagem por participante.
	listedIdentities [][]string
	// Registro da cura de identidade.
	replacedFrom string
	replacedTo   string
	replaceErr   error
}

func (r *a9Repo) GetChannel(_ context.Context, id, tenantID string) (*message.Channel, error) {
	if r.channelErr != nil {
		return nil, r.channelErr
	}
	if r.channel != nil && r.channel.ID == id && r.channel.TenantID == tenantID {
		return r.channel, nil
	}
	return nil, fmt.Errorf("%w", message.ErrChannelNotFound)
}

func (r *a9Repo) GetChannelForParticipant(_ context.Context, id string, participantIDs []string) (*message.Channel, error) {
	if r.channel == nil || r.channel.ID != id || len(participantIDs) == 0 {
		return nil, fmt.Errorf("%w", message.ErrChannelNotFound)
	}
	for _, participantID := range participantIDs {
		if participantID == "" {
			continue
		}
		for _, p := range r.channel.Participants {
			if p == participantID {
				return r.channel, nil
			}
		}
	}
	return nil, fmt.Errorf("%w", message.ErrChannelNotFound)
}

func (r *a9Repo) LeadIDsForAccount(_ context.Context, _, accountID string) ([]string, error) {
	return r.leadsByAccount[accountID], nil
}

func (r *a9Repo) ClientAccountForLead(_ context.Context, _, leadID string) (string, error) {
	return r.accountsByLead[leadID], nil
}

func (r *a9Repo) ReplaceChannelParticipant(_ context.Context, _, _, fromID, toID string) error {
	if r.replaceErr != nil {
		return r.replaceErr
	}
	r.replacedFrom, r.replacedTo = fromID, toID
	if r.channel == nil {
		return nil
	}
	healed := make([]string, 0, len(r.channel.Participants))
	seen := map[string]bool{}
	for _, p := range r.channel.Participants {
		if p == fromID {
			p = toID
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		healed = append(healed, p)
	}
	r.channel.Participants = healed
	return nil
}

// Demais métodos da interface não são exercitados aqui.
func (r *a9Repo) Create(context.Context, *message.Message) error { return nil }
func (r *a9Repo) GetMessage(context.Context, string, string, string) (*message.Message, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) GetMessages(context.Context, string, string, int, int) ([]*message.Message, int, error) {
	return nil, 0, nil
}
func (r *a9Repo) SearchMessages(context.Context, string, string, []string, string, string, string, int, int) ([]*message.Message, int, error) {
	return nil, 0, nil
}
func (r *a9Repo) UpdateMessageContent(context.Context, string, string, string, string) error {
	return nil
}
func (r *a9Repo) UpdateMessagePinStatus(context.Context, string, string, string, bool) error {
	return nil
}
func (r *a9Repo) UpdateMessageReactions(context.Context, string, string, string, []message.Reaction) error {
	return nil
}
func (r *a9Repo) DeleteMessage(context.Context, string, string, string) error { return nil }
func (r *a9Repo) SaveMessage(context.Context, *message.Message) error         { return nil }
func (r *a9Repo) FindDirectChannel(context.Context, string, []string) (*message.Channel, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) GetChannelByReference(context.Context, string, string, string) (*message.Channel, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) ListChannels(context.Context, string, string, string) ([]*message.Channel, error) {
	return nil, nil
}
func (r *a9Repo) ListChannelsByParticipant(_ context.Context, _ string, participantIDs []string) ([]*message.Channel, error) {
	r.listedIdentities = append(r.listedIdentities, participantIDs)
	return nil, nil
}
func (r *a9Repo) CreateChannel(context.Context, *message.Channel) error { return nil }
func (r *a9Repo) UpdateChannel(context.Context, string, string, map[string]interface{}) (*message.Channel, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) UpdateChannelLastMessage(context.Context, string, string, *message.Message) error {
	return nil
}
func (r *a9Repo) UpdateChannelStatus(context.Context, string, string, message.ChannelStatus) error {
	return nil
}
func (r *a9Repo) UpdateChannelParticipants(_ context.Context, _, _ string, participants []string) error {
	r.writtenParticipants = participants
	return nil
}
func (r *a9Repo) UpdateChannelPinned(context.Context, string, string, string, bool) error {
	return nil
}
func (r *a9Repo) UpdateChannelMuted(context.Context, string, string, string, bool) error {
	return nil
}
func (r *a9Repo) AccountBelongsToTenant(_ context.Context, _, accountID string) (bool, error) {
	return r.belongsToTenant[accountID], nil
}
func (r *a9Repo) GetMetrics(context.Context, string) (*message.Metrics, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) MarkMessagesAsRead(context.Context, string, string, string) error { return nil }
func (r *a9Repo) GetTemplateByID(context.Context, string, string) (*message.Template, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) GetTemplateByName(context.Context, string, string) (*message.Template, error) {
	return nil, errors.New("not implemented")
}
func (r *a9Repo) ListTemplates(context.Context, string, string) ([]*message.Template, error) {
	return nil, nil
}
func (r *a9Repo) CreateTemplate(context.Context, *message.Template) error { return nil }
func (r *a9Repo) UpdateTemplate(context.Context, *message.Template) error { return nil }
func (r *a9Repo) DeleteTemplate(context.Context, string, string) error    { return nil }

func a9Channel() *message.Channel {
	return &message.Channel{
		ID:           "canal-1",
		TenantID:     "tenant-do-canal",
		Status:       message.ChannelStatusOpen,
		Participants: []string{"acc-corretor", "acc-cliente"},
	}
}

// Sem account id no contexto NÃO é "chamada de sistema": é anomalia. Antes
// devolvia nil (autorizado) e qualquer rota que perdesse o account id liberava
// o canal inteiro.
func TestCheckParticipant_FailsClosedWithoutAccountInContext(t *testing.T) {
	svc := NewMessageService(&a9Repo{channel: a9Channel()}, nil, nil, nil)

	err := svc.checkParticipant(context.Background(), a9Channel())
	if err == nil {
		t.Fatal("AUTORIZAÇÃO VAZIA: canal liberado sem account id no contexto")
	}
}

// Account que não está na lista de participantes é recusado.
func TestCheckParticipant_RejectsNonParticipant(t *testing.T) {
	svc := NewMessageService(&a9Repo{channel: a9Channel()}, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(context.Background(), "acc-intruso")

	if err := svc.checkParticipant(ctx, a9Channel()); err == nil {
		t.Fatal("account fora da lista de participantes foi autorizado")
	}
}

// Falha de infraestrutura NÃO pode virar "busca por id": o fallback silencioso
// escondia erro de banco e trocava a autorização por tenant pela participação.
func TestResolveChannel_DoesNotFallBackOnInfrastructureError(t *testing.T) {
	infra := errors.New("connection refused")
	repo := &a9Repo{channelErr: infra, channel: a9Channel()}
	svc := NewMessageService(repo, nil, nil, nil)
	ctx := pkgcontext.WithAccountID(context.Background(), "acc-cliente")

	if _, err := svc.resolveChannel(ctx, "canal-1", "tenant-do-canal"); !errors.Is(err, infra) {
		t.Fatalf("erro de infraestrutura foi mascarado pelo fallback: %v", err)
	}
}

// Canal de outro tenant: o participante entra (é o cliente do portal, cujo
// tenant no token é a própria conta) e o não-participante não entra.
func TestResolveChannel_FallbackRequiresParticipation(t *testing.T) {
	repo := &a9Repo{channel: a9Channel()}
	svc := NewMessageService(repo, nil, nil, nil)

	participante := pkgcontext.WithAccountID(context.Background(), "acc-cliente")
	ch, err := svc.resolveChannel(participante, "canal-1", "tenant-do-cliente")
	if err != nil {
		t.Fatalf("participante deveria alcançar o canal de outro tenant: %v", err)
	}
	if ch.ID != "canal-1" {
		t.Fatalf("canal errado: %+v", ch)
	}

	intruso := pkgcontext.WithAccountID(context.Background(), "acc-intruso")
	if _, err := svc.resolveChannel(intruso, "canal-1", "tenant-do-intruso"); err == nil {
		t.Fatal("VAZAMENTO: não-participante alcançou o canal pelo id")
	}

	semConta := context.Background()
	if _, err := svc.resolveChannel(semConta, "canal-1", "tenant-x"); err == nil {
		t.Fatal("VAZAMENTO: canal resolvido sem account id no contexto")
	}
}
