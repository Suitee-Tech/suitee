package service

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

// stubMessageRepo é um stub mínimo do MessageRepository: implementa só
// as 3 operações de canal que EnsureChannelForSupport toca
// (GetChannelByReference, CreateChannel, UpdateChannelParticipants).
// O resto vira no-op/panic suficiente p/ satisfazer a interface — assim
// não precisamos do Postgres pra rodar os tests.
type stubMessageRepo struct {
	getByRef                  func(tenantID, refID, refType string) (*message.Channel, error)
	createCh                  func(c *message.Channel) error
	updateParticipantsCalls   int
	updateParticipantsChannel string
	updateParticipantsTenant  string
	updateParticipantsValues  []string
}

func (s *stubMessageRepo) GetChannelByReference(ctx context.Context, tenantID, refID, refType string) (*message.Channel, error) {
	if s.getByRef != nil {
		return s.getByRef(tenantID, refID, refType)
	}
	return nil, errors.New("not found")
}

func (s *stubMessageRepo) CreateChannel(ctx context.Context, c *message.Channel) error {
	if s.createCh != nil {
		return s.createCh(c)
	}
	return nil
}

func (s *stubMessageRepo) UpdateChannelParticipants(ctx context.Context, channelID, tenantID string, participants []string) error {
	s.updateParticipantsCalls++
	s.updateParticipantsChannel = channelID
	s.updateParticipantsTenant = tenantID
	s.updateParticipantsValues = participants
	return nil
}

// ---------- métodos não exercitados ----------

func (s *stubMessageRepo) Create(ctx context.Context, msg *message.Message) error { return nil }
func (s *stubMessageRepo) GetMessage(ctx context.Context, id, tenantID, accountID string) (*message.Message, error) {
	return nil, nil
}
func (s *stubMessageRepo) GetMessages(ctx context.Context, channelID, tenantID string, limit, offset int) ([]*message.Message, int, error) {
	return nil, 0, nil
}
func (s *stubMessageRepo) SearchMessages(ctx context.Context, tenantID, accountID string, channelIDs []string, query, hashtag, mention string, limit, offset int) ([]*message.Message, int, error) {
	return nil, 0, nil
}
func (s *stubMessageRepo) UpdateMessageContent(ctx context.Context, id, tenantID, accountID, content string) error {
	return nil
}
func (s *stubMessageRepo) UpdateMessagePinStatus(ctx context.Context, id, tenantID, accountID string, pinned bool) error {
	return nil
}
func (s *stubMessageRepo) UpdateMessageReactions(ctx context.Context, id, tenantID, accountID string, reactions []message.Reaction) error {
	return nil
}
func (s *stubMessageRepo) DeleteMessage(ctx context.Context, id, tenantID, accountID string) error {
	return nil
}
func (s *stubMessageRepo) SaveMessage(ctx context.Context, msg *message.Message) error { return nil }

func (s *stubMessageRepo) GetChannel(ctx context.Context, id, tenantID string) (*message.Channel, error) {
	return nil, nil
}
func (s *stubMessageRepo) GetChannelForParticipant(ctx context.Context, id string, participantIDs []string) (*message.Channel, error) {
	return nil, errors.New("not found")
}
func (s *stubMessageRepo) AccountBelongsToTenant(ctx context.Context, tenantID, accountID string) (bool, error) {
	return false, nil
}
func (s *stubMessageRepo) ClientAccountForLead(ctx context.Context, tenantID, leadID string) (string, error) {
	return "", nil
}
func (s *stubMessageRepo) LeadIDsForAccount(ctx context.Context, tenantID, accountID string) ([]string, error) {
	return nil, nil
}
func (s *stubMessageRepo) ReplaceChannelParticipant(ctx context.Context, channelID, tenantID, fromID, toID string) error {
	return nil
}
func (s *stubMessageRepo) FindDirectChannel(ctx context.Context, tenantID string, participants []string) (*message.Channel, error) {
	return nil, message.ErrChannelNotFound
}
func (s *stubMessageRepo) ListChannels(ctx context.Context, tenantID, accountID, role string) ([]*message.Channel, error) {
	return nil, nil
}
func (s *stubMessageRepo) ListChannelsByParticipant(ctx context.Context, tenantID string, participantIDs []string) ([]*message.Channel, error) {
	return nil, nil
}
func (s *stubMessageRepo) UpdateChannel(ctx context.Context, id, tenantID string, updates map[string]interface{}) (*message.Channel, error) {
	return nil, nil
}
func (s *stubMessageRepo) UpdateChannelLastMessage(ctx context.Context, tenantID, channelID string, msg *message.Message) error {
	return nil
}
func (s *stubMessageRepo) UpdateChannelStatus(ctx context.Context, channelID, tenantID string, status message.ChannelStatus) error {
	return nil
}
func (s *stubMessageRepo) UpdateChannelPinned(ctx context.Context, channelID, tenantID, accountID string, pinned bool) error {
	return nil
}
func (s *stubMessageRepo) UpdateChannelMuted(ctx context.Context, channelID, tenantID, accountID string, muted bool) error {
	return nil
}
func (s *stubMessageRepo) GetMetrics(ctx context.Context, tenantID string) (*message.Metrics, error) {
	return nil, nil
}
func (s *stubMessageRepo) MarkMessagesAsRead(ctx context.Context, channelID, tenantID, participantID string) error {
	return nil
}
func (s *stubMessageRepo) GetTemplateByID(ctx context.Context, tenantID, id string) (*message.Template, error) {
	return nil, nil
}
func (s *stubMessageRepo) GetTemplateByName(ctx context.Context, tenantID, name string) (*message.Template, error) {
	return nil, nil
}
func (s *stubMessageRepo) ListTemplates(ctx context.Context, tenantID, category string) ([]*message.Template, error) {
	return nil, nil
}
func (s *stubMessageRepo) CreateTemplate(ctx context.Context, t *message.Template) error {
	return nil
}
func (s *stubMessageRepo) UpdateTemplate(ctx context.Context, t *message.Template) error {
	return nil
}
func (s *stubMessageRepo) DeleteTemplate(ctx context.Context, tenantID, id string) error { return nil }

// compile-time check
var _ repository.MessageRepository = (*stubMessageRepo)(nil)

// ─────────────────────────────────── tests ───────────────────────────────────

func TestEnsureChannelForSupport_CreatesNewWhenAbsent(t *testing.T) {
	var created *message.Channel
	repo := &stubMessageRepo{
		getByRef: func(tenantID, refID, refType string) (*message.Channel, error) {
			if refType != "contract" {
				t.Errorf("expected refType=contract, got %q", refType)
			}
			if refID != "ctr-1" {
				t.Errorf("expected refID=ctr-1, got %q", refID)
			}
			return nil, errors.New("not found")
		},
		createCh: func(c *message.Channel) error {
			created = c
			return nil
		},
	}

	svc := NewMessageService(repo, nil, nil, nil)
	ch, err := svc.EnsureChannelForSupport(context.Background(), "tenant-1", "ctr-1", []string{"acc-a", "acc-b"})
	if err != nil {
		t.Fatalf("EnsureChannelForSupport: %v", err)
	}
	if ch == nil {
		t.Fatal("expected channel returned")
	}
	if created == nil {
		t.Fatal("expected CreateChannel to be called")
	}
	if created.Type != message.ChannelTypeSupport {
		t.Errorf("expected type=support, got %q", created.Type)
	}
	if created.EntityID != "ctr-1" || created.EntityType != "contract" {
		t.Errorf("entity binding wrong: id=%q type=%q", created.EntityID, created.EntityType)
	}
	if created.CreatedBy != "system" {
		t.Errorf("expected createdBy=system, got %q", created.CreatedBy)
	}
	if len(created.Participants) != 2 {
		t.Errorf("expected 2 participants, got %d", len(created.Participants))
	}
	if created.Status != message.ChannelStatusOpen {
		t.Errorf("expected status=open, got %q", created.Status)
	}
}

func TestEnsureChannelForSupport_IdempotentWhenAlreadyExists(t *testing.T) {
	// Cenário típico de redelivery do RabbitMQ: o consumer roda
	// novamente com o mesmo payload. Não pode duplicar o canal — deve
	// retornar o existente sem chamar CreateChannel.
	existing := &message.Channel{
		ID:           "ch-existing",
		TenantID:     "tenant-1",
		Type:         message.ChannelTypeSupport,
		Status:       message.ChannelStatusOpen,
		Participants: []string{"acc-a"},
		EntityID:     "ctr-1",
		EntityType:   "contract",
	}
	repo := &stubMessageRepo{
		getByRef: func(tenantID, refID, refType string) (*message.Channel, error) {
			return existing, nil
		},
		createCh: func(c *message.Channel) error {
			t.Fatal("CreateChannel should not be called when channel exists")
			return nil
		},
	}

	svc := NewMessageService(repo, nil, nil, nil)
	ch, err := svc.EnsureChannelForSupport(context.Background(), "tenant-1", "ctr-1", []string{"acc-a"})
	if err != nil {
		t.Fatalf("EnsureChannelForSupport: %v", err)
	}
	if ch.ID != "ch-existing" {
		t.Errorf("expected existing channel returned, got %q", ch.ID)
	}
	if repo.updateParticipantsCalls != 0 {
		t.Errorf("expected no participant update (same set), got %d calls", repo.updateParticipantsCalls)
	}
}

func TestEnsureChannelForSupport_AddsMissingParticipantsToExisting(t *testing.T) {
	// Redelivery com participante novo (cenário: fiador adicionado ao
	// contrato depois) — devemos manter o canal mas adicionar o novo.
	existing := &message.Channel{
		ID:           "ch-existing",
		TenantID:     "tenant-1",
		Type:         message.ChannelTypeSupport,
		Participants: []string{"acc-a"},
		EntityID:     "ctr-1",
		EntityType:   "contract",
	}
	repo := &stubMessageRepo{
		getByRef: func(tenantID, refID, refType string) (*message.Channel, error) {
			return existing, nil
		},
	}

	svc := NewMessageService(repo, nil, nil, nil)
	_, err := svc.EnsureChannelForSupport(context.Background(), "tenant-1", "ctr-1", []string{"acc-a", "acc-b"})
	if err != nil {
		t.Fatalf("EnsureChannelForSupport: %v", err)
	}
	if repo.updateParticipantsCalls != 1 {
		t.Fatalf("expected 1 UpdateChannelParticipants call, got %d", repo.updateParticipantsCalls)
	}
	if repo.updateParticipantsChannel != "ch-existing" {
		t.Errorf("expected update on ch-existing, got %q", repo.updateParticipantsChannel)
	}
	if len(repo.updateParticipantsValues) != 2 {
		t.Errorf("expected 2 participants after merge, got %d", len(repo.updateParticipantsValues))
	}
}

func TestEnsureChannelForSupport_DedupesParticipantsOnCreate(t *testing.T) {
	// Payload com IDs repetidos (mesmo cliente listado 2x — bug benigno
	// no upstream) não deve materializar duplicatas no canal.
	var created *message.Channel
	repo := &stubMessageRepo{
		getByRef: func(tenantID, refID, refType string) (*message.Channel, error) {
			return nil, errors.New("not found")
		},
		createCh: func(c *message.Channel) error {
			created = c
			return nil
		},
	}

	svc := NewMessageService(repo, nil, nil, nil)
	_, err := svc.EnsureChannelForSupport(context.Background(), "tenant-1", "ctr-1", []string{"acc-a", "acc-a", "", "acc-b", "acc-a"})
	if err != nil {
		t.Fatalf("EnsureChannelForSupport: %v", err)
	}
	if len(created.Participants) != 2 {
		t.Errorf("expected dedup to 2 unique non-empty ids, got %v", created.Participants)
	}
}

func TestEnsureChannelForSupport_RejectsMissingTenantOrContract(t *testing.T) {
	repo := &stubMessageRepo{}
	svc := NewMessageService(repo, nil, nil, nil)

	if _, err := svc.EnsureChannelForSupport(context.Background(), "", "ctr-1", nil); err == nil {
		t.Error("expected error when tenantID empty")
	}
	if _, err := svc.EnsureChannelForSupport(context.Background(), "tenant-1", "", nil); err == nil {
		t.Error("expected error when contractID empty")
	}
}
