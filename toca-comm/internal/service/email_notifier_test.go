package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/config"
)

// --- fakes ---

type fakePrefsRepo struct {
	mu       sync.Mutex
	disabled map[string]bool // key: userID|tenantID|category
}

func (f *fakePrefsRepo) IsEnabled(_ context.Context, userID, tenantID, category string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.disabled[userID+"|"+tenantID+"|"+category], nil
}

func (f *fakePrefsRepo) Set(_ context.Context, userID, tenantID, category string, enabled bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := userID + "|" + tenantID + "|" + category
	if !enabled {
		f.disabled[key] = true
	} else {
		delete(f.disabled, key)
	}
	return nil
}

type fakeSentExternals struct {
	mu     sync.Mutex
	ledger map[string]bool // key: eventID|channel|recipient
	recent map[string]bool // key: channel|recipient|category
}

func (f *fakeSentExternals) RegisterIfFirst(_ context.Context, eventID, channel, recipientID, _, category string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := eventID + "|" + channel + "|" + recipientID
	if f.ledger[key] {
		return false, nil
	}
	f.ledger[key] = true
	// O cooldown consulta dispatches recentes POR CATEGORIA: registra a
	// categoria no índice "recent" como o repositório real faria via coluna.
	f.recent[channel+"|"+recipientID+"|"+category] = true
	return true, nil
}

func (f *fakeSentExternals) HasRecentDispatch(_ context.Context, _, channel, recipientID, category string, _ time.Duration) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recent[channel+"|"+recipientID+"|"+category], nil
}

// --- test harness ---

type notifierHarness struct {
	notifier      *EmailNotifier
	prefs         *fakePrefsRepo
	sentExternals *fakeSentExternals
	sendCount     atomic.Int64
}

func newNotifierHarness(t *testing.T, cfg config.EmailConfig) *notifierHarness {
	t.Helper()
	h := &notifierHarness{
		prefs:         &fakePrefsRepo{disabled: map[string]bool{}},
		sentExternals: &fakeSentExternals{ledger: map[string]bool{}, recent: map[string]bool{}},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.sendCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"email_test_1"}`))
	}))
	t.Cleanup(server.Close)

	emailService := newEmailService(config.EmailConfig{
		ResendAPIKey: "sandbox-key",
		FromAddress:  "Suitee <noreply@sandbox.example>",
	}, server.URL, time.Second)

	h.notifier = NewEmailNotifier(emailService, h.prefs, h.sentExternals, NewHub(), cfg)
	return h
}

func (h *notifierHarness) markOnline(tenantID, userID string) {
	// O hub do harness roda sem o loop Run(); registrar direto no mapa é o
	// caminho determinístico para o teste de presença.
	h.notifier.hub.mu.Lock()
	defer h.notifier.hub.mu.Unlock()
	if h.notifier.hub.clients[tenantID] == nil {
		h.notifier.hub.clients[tenantID] = map[string]map[*Client]bool{}
	}
	h.notifier.hub.clients[tenantID][userID] = map[*Client]bool{{}: true}
}

func enabledAllConfig() config.EmailConfig {
	return config.EmailConfig{
		TransactionalEmailEnabled: true,
		EmailLeadAssigned:         true,
		EmailDealUpdates:          true,
		EmailProposals:            true,
		EmailAppointments:         true,
		EmailRecommendations:      true,
		EmailChatReminders:        true,
		EmailCooldownMinutes:      60,
	}
}

func baseOpts() NotifyOptions {
	return NotifyOptions{
		Category:     CategoryDealUpdates,
		TenantID:     "tenant-1",
		UserID:       "user-1",
		Recipient:    "cliente@example.com",
		Subject:      "Atualização do seu negócio",
		BodyHTML:     "<p>Status: Em Negociação</p>",
		EventID:      "evt-1",
		SkipIfOnline: true,
	}
}

// --- tests ---

func TestEmailNotifier_GlobalKillSwitch(t *testing.T) {
	cfg := enabledAllConfig()
	cfg.TransactionalEmailEnabled = false
	h := newNotifierHarness(t, cfg)

	if _, err := h.notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (kill-switch off)", got)
	}
}

func TestEmailNotifier_CategoryFlag(t *testing.T) {
	cfg := enabledAllConfig()
	cfg.EmailDealUpdates = false
	h := newNotifierHarness(t, cfg)

	if _, err := h.notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (category flag off)", got)
	}
}

func TestEmailNotifier_OptOut(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())
	_ = h.prefs.Set(context.Background(), "user-1", "tenant-1", string(CategoryDealUpdates), false)

	if _, err := h.notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (user opted out)", got)
	}
}

func TestEmailNotifier_OnlineSuppression(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())
	h.markOnline("tenant-1", "user-1")

	if _, err := h.notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (user online)", got)
	}

	// Force ignora presença — marco crítico deve sair mesmo online.
	opts := baseOpts()
	opts.Force = true
	if _, err := h.notifier.Notify(context.Background(), opts); err != nil {
		t.Fatalf("Notify(force): %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1 (force bypasses presence)", got)
	}
}

func TestEmailNotifier_Cooldown(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())
	h.sentExternals.mu.Lock()
	h.sentExternals.recent["email|user-1|deal_updates"] = true
	h.sentExternals.mu.Unlock()

	if _, err := h.notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (cooldown)", got)
	}

	opts := baseOpts()
	opts.Force = true
	if _, err := h.notifier.Notify(context.Background(), opts); err != nil {
		t.Fatalf("Notify(force): %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1 (force bypasses cooldown)", got)
	}
}

func TestEmailNotifier_CooldownIsPerCategory(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())

	// Categoria deal_updates em cooldown…
	h.sentExternals.mu.Lock()
	h.sentExternals.recent["email|user-1|deal_updates"] = true
	h.sentExternals.mu.Unlock()

	// …não suprime uma categoria diferente (proposals).
	opts := baseOpts()
	opts.Category = CategoryProposals
	opts.EventID = "evt-9"
	if _, err := h.notifier.Notify(context.Background(), opts); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1 (cooldown is per category)", got)
	}
}

func TestEmailNotifier_RedeliveryDedup(t *testing.T) {
	// Cooldown 0: o teste isola apenas o gate de idempotência.
	cfg := enabledAllConfig()
	cfg.EmailCooldownMinutes = 0
	h := newNotifierHarness(t, cfg)

	opts := baseOpts()
	if _, err := h.notifier.Notify(context.Background(), opts); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	// Redelivery do mesmo evento para o mesmo destinatário: skip.
	if _, err := h.notifier.Notify(context.Background(), opts); err != nil {
		t.Fatalf("Notify (redelivery): %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1 (dedup)", got)
	}

	// Evento diferente para o mesmo destinatário: sai (sem cooldown recente).
	opts2 := baseOpts()
	opts2.EventID = "evt-2"
	if _, err := h.notifier.Notify(context.Background(), opts2); err != nil {
		t.Fatalf("Notify (new event): %v", err)
	}
	if got := h.sendCount.Load(); got != 2 {
		t.Fatalf("sends = %d, want 2 (new event passes)", got)
	}
}

func TestEmailNotifier_NoRecipientEmail(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())
	opts := baseOpts()
	opts.Recipient = ""

	if _, err := h.notifier.Notify(context.Background(), opts); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0 (no email resolved)", got)
	}
}

func TestEmailNotifier_MissingIdentity(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())
	opts := baseOpts()
	opts.UserID = ""

	if _, err := h.notifier.Notify(context.Background(), opts); err == nil {
		t.Fatal("Notify with empty UserID should return error")
	}
	if got := h.sendCount.Load(); got != 0 {
		t.Fatalf("sends = %d, want 0", got)
	}
}

func TestEmailNotifier_HappyPath(t *testing.T) {
	h := newNotifierHarness(t, enabledAllConfig())

	if _, err := h.notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if got := h.sendCount.Load(); got != 1 {
		t.Fatalf("sends = %d, want 1", got)
	}
	// O ledger registrou o dispatch para o par (evento, canal, destinatário).
	h.sentExternals.mu.Lock()
	registered := h.sentExternals.ledger["evt-1|email|user-1"]
	h.sentExternals.mu.Unlock()
	if !registered {
		t.Fatal("sent_externals ledger missing dispatch record")
	}
}

func TestEmailNotifier_NilEmailService(t *testing.T) {
	prefs := &fakePrefsRepo{disabled: map[string]bool{}}
	notifier := NewEmailNotifier(nil, prefs, nil, NewHub(), enabledAllConfig())
	// Sem EmailService o notifier é inerte, nunca devolve erro.
	if _, err := notifier.Notify(context.Background(), baseOpts()); err != nil {
		t.Fatalf("Notify: %v", err)
	}
}

// --- copy helpers ---

func TestLeadAssignedCopy(t *testing.T) {
	if got := leadAssignedSubject(""); got != "Novo lead atribuído" {
		t.Fatalf("subject without name = %q", got)
	}
	if got := leadAssignedSubject("Maria Souza"); got != "Novo lead atribuído: Maria Souza" {
		t.Fatalf("subject with name = %q", got)
	}

	body := leadAssignedBody(leadAssignedPayload{
		LeadID: "lead-1", LeadName: "Maria Souza", Stage: "NEW", Method: "sequential",
		Tags: []string{"MANUAL", "BUYER", "COMPRA_VENDA", "COMPRA E VENDA", "OTHER", "HIGH_PRICE", "INTERESSE GERAL"},
	})
	for _, want := range []string{"Maria Souza", "Etapa:", "Novo", "Distribuição:", "Roleta sequencial", "Comprador"} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
	for _, unwanted := range []string{"NEW", "sequential", "COMPRA_VENDA", "HIGH_PRICE", "BUYER"} {
		if strings.Contains(body, unwanted) {
			t.Fatalf("body leaked raw enum %q: %s", unwanted, body)
		}
	}
}

func TestHubIsUserOnline(t *testing.T) {
	hub := NewHub()
	if hub.IsUserOnline("t1", "u1") {
		t.Fatal("empty hub must report offline")
	}
	hub.mu.Lock()
	hub.clients["t1"] = map[string]map[*Client]bool{"u1": {&Client{}: true}}
	hub.mu.Unlock()
	if !hub.IsUserOnline("t1", "u1") {
		t.Fatal("registered client must report online")
	}
	if hub.IsUserOnline("t1", "u2") {
		t.Fatal("unknown user must report offline")
	}
}
