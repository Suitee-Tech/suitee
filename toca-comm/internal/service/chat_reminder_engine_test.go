package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/toca-comm/internal/repository"
)

type fakeChatReminderRepo struct {
	candidates []repository.ChatReminderCandidate
}

func (f *fakeChatReminderRepo) ListUnreadCandidates(_ context.Context, _, _ time.Duration, _ int) ([]repository.ChatReminderCandidate, error) {
	return f.candidates, nil
}

type fakeEmailResolver struct {
	emails map[string]string // participantID -> email
}

func (f *fakeEmailResolver) ParticipantEmailByID(_ context.Context, _, participantID string) (string, error) {
	return f.emails[participantID], nil
}

func (f *fakeEmailResolver) LeadEmailByID(_ context.Context, _, leadID string) (string, error) {
	return f.emails[leadID], nil
}

func newReminderHarness(t *testing.T) (*ChatReminderEngine, *notifierHarness, *fakeChatReminderRepo, *fakeEmailResolver) {
	t.Helper()
	h := newNotifierHarness(t, enabledAllConfig())
	repo := &fakeChatReminderRepo{}
	resolver := &fakeEmailResolver{emails: map[string]string{}}
	engine := NewChatReminderEngine(repo, resolver, h.notifier, h.notifier.hub, 30*time.Minute)
	return engine, h, repo, resolver
}

func reminderCandidate(participant string) repository.ChatReminderCandidate {
	return repository.ChatReminderCandidate{
		TenantID:      "tenant-1",
		ChannelID:     "ch-1",
		ChannelName:   "Negócio: Apto Centro",
		MessageID:     "m-1",
		SenderID:      "broker-1",
		SenderName:    "Carlos",
		Snippet:       "Olá! Separei uma proposta para você.",
		SentAt:        time.Now().Add(-40 * time.Minute),
		ParticipantID: participant,
	}
}

func TestChatReminderEngine_SendsForUnreadParticipant(t *testing.T) {
	engine, h, repo, resolver := newReminderHarness(t)
	resolver.emails["client-1"] = "cliente@example.com"
	repo.candidates = []repository.ChatReminderCandidate{reminderCandidate("client-1")}

	engine.runTick()
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1", got)
	}
	// O dedup sintético ficou registrado no ledger.
	h.sentExternals.mu.Lock()
	found := false
	for key := range h.sentExternals.ledger {
		if strings.HasPrefix(key, "chat_reminder:ch-1:client-1:") {
			found = true
		}
	}
	h.sentExternals.mu.Unlock()
	if !found {
		t.Fatal("synthetic reminder event id missing from ledger")
	}
}

func TestChatReminderEngine_SkipsOnlineParticipant(t *testing.T) {
	engine, h, repo, resolver := newReminderHarness(t)
	resolver.emails["client-1"] = "cliente@example.com"
	h.markOnline("tenant-1", "client-1")
	repo.candidates = []repository.ChatReminderCandidate{reminderCandidate("client-1")}

	engine.runTick()
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (online)", got)
	}
}

func TestChatReminderEngine_SkipsUnresolvableEmail(t *testing.T) {
	engine, h, repo, _ := newReminderHarness(t)
	repo.candidates = []repository.ChatReminderCandidate{reminderCandidate("ghost-1")}

	engine.runTick()
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (no email)", got)
	}
}

func TestChatReminderEngine_DedupLatestMessagePerConversation(t *testing.T) {
	engine, h, repo, resolver := newReminderHarness(t)
	resolver.emails["client-1"] = "cliente@example.com"
	older := reminderCandidate("client-1")
	newer := reminderCandidate("client-1")
	newer.MessageID = "m-2"
	newer.SentAt = older.SentAt.Add(10 * time.Minute)
	repo.candidates = []repository.ChatReminderCandidate{older, newer}

	engine.runTick()
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1 (one per conversation)", got)
	}
}

// O lembrete sai UMA única vez por grupo de mensagens não lidas: ticks
// seguintes com o mesmo topo de não lido não geram novo e-mail (antes o dedup
// era por hora e repetia enquanto a mensagem continuasse sem leitura).
func TestChatReminderEngine_SameUnreadGroupDoesNotRepeat(t *testing.T) {
	engine, h, repo, resolver := newReminderHarness(t)
	resolver.emails["client-1"] = "cliente@example.com"
	repo.candidates = []repository.ChatReminderCandidate{reminderCandidate("client-1")}

	engine.runTick()
	engine.runTick()
	engine.runTick()
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1 (um lembrete por grupo de não lidas)", got)
	}
}

// Mensagem NOVA depois do lembrete forma um grupo novo (chave diferente) e
// pode ser lembrada de novo — aqui com o cooldown desligado para isolar a
// semântica de grupo.
func TestChatReminderEngine_NewMessageFormsNewGroup(t *testing.T) {
	cfg := enabledAllConfig()
	cfg.EmailCooldownMinutes = 0
	h := newNotifierHarness(t, cfg)
	repo := &fakeChatReminderRepo{}
	resolver := &fakeEmailResolver{emails: map[string]string{"client-1": "cliente@example.com"}}
	engine := NewChatReminderEngine(repo, resolver, h.notifier, h.notifier.hub, 30*time.Minute)

	first := reminderCandidate("client-1")
	repo.candidates = []repository.ChatReminderCandidate{first}
	engine.runTick()

	// Nova mensagem não lida no mesmo canal → grupo novo.
	newer := reminderCandidate("client-1")
	newer.MessageID = "m-2"
	newer.SentAt = first.SentAt.Add(5 * time.Minute)
	repo.candidates = []repository.ChatReminderCandidate{newer}
	engine.runTick()

	if got := h.sendCount.Load(); got != 2 {
		t.Fatalf("sends = %d, want 2 (mensagem nova = grupo novo)", got)
	}
}

func TestChatReminderGroupKey(t *testing.T) {
	// Mesmo topo de não lido = mesmo grupo (não repete).
	if chatReminderGroupKey("ch-1", "u-1", "m-1") != chatReminderGroupKey("ch-1", "u-1", "m-1") {
		t.Fatal("mesma mensagem deveria produzir a mesma chave")
	}
	// Mensagem nova ou outro destinatário = grupo diferente.
	if chatReminderGroupKey("ch-1", "u-1", "m-1") == chatReminderGroupKey("ch-1", "u-1", "m-2") {
		t.Fatal("mensagem nova deveria mudar a chave")
	}
	if chatReminderGroupKey("ch-1", "u-1", "m-1") == chatReminderGroupKey("ch-1", "u-2", "m-1") {
		t.Fatal("outro participante deveria ter chave própria")
	}
}

func TestChatReminderEngine_OptOutSuppresses(t *testing.T) {
	engine, h, repo, resolver := newReminderHarness(t)
	resolver.emails["client-1"] = "cliente@example.com"
	_ = h.prefs.Set(context.Background(), "client-1", "tenant-1", string(CategoryChatReminders), false)
	repo.candidates = []repository.ChatReminderCandidate{reminderCandidate("client-1")}

	engine.runTick()
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (opt-out)", got)
	}
}

// cfg precisa expor a flag de chat reminder ligada — enabledAllConfig já liga.
var _ = config.EmailConfig{}
