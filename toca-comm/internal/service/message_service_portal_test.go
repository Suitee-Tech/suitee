package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
)

type portalMessageMutationRepo struct {
	*stubMessageRepo
	channel *message.Channel
	msg     *message.Message

	updatedMessageID      string
	updatedTenantID       string
	updatedAccountID      string
	updatedContent        string
	deletedMessageID      string
	deletedTenantID       string
	deletedAccountID      string
	pinnedMessageID       string
	pinnedTenantID        string
	pinnedAccountID       string
	searchedTenantID      string
	searchedAccountID     string
	searchedChannelIDs    []string
	searchedQuery         string
	searchedHashtag       string
	searchedMention       string
	markReadChannelID     string
	markReadTenantID      string
	markReadParticipantID string
}

func newPortalMessageMutationRepo() *portalMessageMutationRepo {
	return &portalMessageMutationRepo{
		stubMessageRepo: &stubMessageRepo{},
		channel: &message.Channel{
			ID:           "channel-1",
			TenantID:     "company-tenant",
			Status:       message.ChannelStatusOpen,
			Participants: []string{"portal-client", "operator-1"},
		},
		msg: &message.Message{
			ID:        "message-1",
			ChannelID: "channel-1",
			TenantID:  "company-tenant",
			SenderID:  "portal-client",
			Content:   "Mensagem original",
			ReadBy:    []string{"portal-client"},
		},
	}
}

func (r *portalMessageMutationRepo) GetChannel(_ context.Context, id, tenantID string) (*message.Channel, error) {
	if id == r.channel.ID && tenantID == r.channel.TenantID {
		return r.channel, nil
	}
	// Cinto do contrato do repositório real (achado A9): "não existe para este
	// tenant" é o sentinel `message.ErrChannelNotFound`; qualquer outro erro é
	// falha de infraestrutura e NÃO autoriza o fallback por participação.
	return nil, fmt.Errorf("%w", message.ErrChannelNotFound)
}

// GetChannelForParticipant reproduz o WHERE com participação do repositório
// real: só devolve o canal se ALGUMA identidade do chamador estiver na lista.
func (r *portalMessageMutationRepo) GetChannelForParticipant(_ context.Context, id string, participantIDs []string) (*message.Channel, error) {
	if id != r.channel.ID || len(participantIDs) == 0 {
		return nil, fmt.Errorf("%w", message.ErrChannelNotFound)
	}
	for _, participantID := range participantIDs {
		for _, p := range r.channel.Participants {
			if p == participantID {
				return r.channel, nil
			}
		}
	}
	return nil, fmt.Errorf("%w", message.ErrChannelNotFound)
}

func (r *portalMessageMutationRepo) GetMessage(_ context.Context, id, tenantID, _ string) (*message.Message, error) {
	if id == r.msg.ID && tenantID == r.msg.TenantID {
		copy := *r.msg
		return &copy, nil
	}
	return nil, errors.New("message not found")
}

func (r *portalMessageMutationRepo) UpdateMessageContent(_ context.Context, id, tenantID, accountID, content string) error {
	r.updatedMessageID = id
	r.updatedTenantID = tenantID
	r.updatedAccountID = accountID
	r.updatedContent = content
	return nil
}

func (r *portalMessageMutationRepo) DeleteMessage(_ context.Context, id, tenantID, accountID string) error {
	r.deletedMessageID = id
	r.deletedTenantID = tenantID
	r.deletedAccountID = accountID
	return nil
}

func (r *portalMessageMutationRepo) UpdateMessagePinStatus(_ context.Context, id, tenantID, accountID string, _ bool) error {
	r.pinnedMessageID = id
	r.pinnedTenantID = tenantID
	r.pinnedAccountID = accountID
	return nil
}

func (r *portalMessageMutationRepo) SearchMessages(
	_ context.Context,
	tenantID, accountID string,
	channelIDs []string,
	query, hashtag, mention string,
	_, _ int,
) ([]*message.Message, int, error) {
	r.searchedTenantID = tenantID
	r.searchedAccountID = accountID
	r.searchedChannelIDs = append([]string(nil), channelIDs...)
	r.searchedQuery = query
	r.searchedHashtag = hashtag
	r.searchedMention = mention
	return []*message.Message{r.msg}, 1, nil
}

func (r *portalMessageMutationRepo) MarkMessagesAsRead(_ context.Context, channelID, tenantID, participantID string) error {
	r.markReadChannelID = channelID
	r.markReadTenantID = tenantID
	r.markReadParticipantID = participantID
	return nil
}

func portalMessageContext() context.Context {
	ctx := pkgcontext.WithTenantID(context.Background(), "portal-client")
	return pkgcontext.WithAccountID(ctx, "portal-client")
}

func TestMessageServicePortalMutationsUseChannelTenant(t *testing.T) {
	t.Run("edit", func(t *testing.T) {
		repo := newPortalMessageMutationRepo()
		svc := NewMessageService(repo, nil, nil, nil)
		if _, err := svc.EditMessage(portalMessageContext(), "channel-1", "message-1", "Atualizada"); err != nil {
			t.Fatalf("EditMessage: %v", err)
		}
		if repo.updatedMessageID != "message-1" || repo.updatedTenantID != "company-tenant" || repo.updatedAccountID != "portal-client" || repo.updatedContent != "Atualizada" {
			t.Fatalf("unexpected edit scope: id=%q tenant=%q account=%q content=%q", repo.updatedMessageID, repo.updatedTenantID, repo.updatedAccountID, repo.updatedContent)
		}
	})

	t.Run("delete", func(t *testing.T) {
		repo := newPortalMessageMutationRepo()
		svc := NewMessageService(repo, nil, nil, nil)
		if err := svc.DeleteMessage(portalMessageContext(), "channel-1", "message-1"); err != nil {
			t.Fatalf("DeleteMessage: %v", err)
		}
		if repo.deletedMessageID != "message-1" || repo.deletedTenantID != "company-tenant" || repo.deletedAccountID != "portal-client" {
			t.Fatalf("unexpected delete scope: id=%q tenant=%q account=%q", repo.deletedMessageID, repo.deletedTenantID, repo.deletedAccountID)
		}
	})

	t.Run("pin", func(t *testing.T) {
		repo := newPortalMessageMutationRepo()
		svc := NewMessageService(repo, nil, nil, nil)
		if err := svc.TogglePinMessage(portalMessageContext(), "channel-1", "message-1", true); err != nil {
			t.Fatalf("TogglePinMessage: %v", err)
		}
		if repo.pinnedMessageID != "message-1" || repo.pinnedTenantID != "company-tenant" || repo.pinnedAccountID != "portal-client" {
			t.Fatalf("unexpected pin scope: id=%q tenant=%q account=%q", repo.pinnedMessageID, repo.pinnedTenantID, repo.pinnedAccountID)
		}
	})

	t.Run("search", func(t *testing.T) {
		repo := newPortalMessageMutationRepo()
		svc := NewMessageService(repo, nil, nil, nil)
		if _, _, err := svc.SearchMessages(portalMessageContext(), "contrato", "channel-1", "urgente", "operator-1", 20, 0); err != nil {
			t.Fatalf("SearchMessages: %v", err)
		}
		if repo.searchedTenantID != "company-tenant" || repo.searchedAccountID != "portal-client" || len(repo.searchedChannelIDs) != 1 || repo.searchedChannelIDs[0] != "channel-1" {
			t.Fatalf("unexpected search scope: tenant=%q account=%q channels=%v", repo.searchedTenantID, repo.searchedAccountID, repo.searchedChannelIDs)
		}
		if repo.searchedQuery != "contrato" || repo.searchedHashtag != "urgente" || repo.searchedMention != "operator-1" {
			t.Fatalf("unexpected search filters: query=%q hashtag=%q mention=%q", repo.searchedQuery, repo.searchedHashtag, repo.searchedMention)
		}
	})
}

func TestMessageServiceMarkReadBroadcastsReceiptAcrossPortalBoundary(t *testing.T) {
	repo := newPortalMessageMutationRepo()
	hub := NewHub()
	operator := &Client{
		Hub:      hub,
		Send:     make(chan []byte, 1),
		TenantID: "company-tenant",
		UserID:   "operator-1",
	}
	hub.clients["company-tenant"] = map[string]map[*Client]bool{
		"operator-1": {operator: true},
	}

	svc := NewMessageService(repo, hub, nil, nil)
	if err := svc.MarkRead(portalMessageContext(), "channel-1"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if repo.markReadChannelID != "channel-1" || repo.markReadTenantID != "company-tenant" || repo.markReadParticipantID != "portal-client" {
		t.Fatalf("unexpected read scope: channel=%q tenant=%q participant=%q", repo.markReadChannelID, repo.markReadTenantID, repo.markReadParticipantID)
	}

	select {
	case raw := <-operator.Send:
		var event WSEvent
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatalf("invalid websocket event: %v", err)
		}
		if event.Type != "messages_read" {
			t.Fatalf("event type = %q, want messages_read", event.Type)
		}
	default:
		t.Fatal("operator did not receive read receipt")
	}
}
