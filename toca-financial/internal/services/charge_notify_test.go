package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

type stubCommClient struct {
	emailCalls []clients.SendNotificationRequest
	pushCalls  []clients.PushNotificationRequest
	emailErr   error
	pushErr    error
}

func (s *stubCommClient) SendNotification(_ context.Context, req clients.SendNotificationRequest) error {
	s.emailCalls = append(s.emailCalls, req)
	return s.emailErr
}

func (s *stubCommClient) PushInAppNotification(_ context.Context, accountID string, req clients.PushNotificationRequest) error {
	s.pushCalls = append(s.pushCalls, req)
	return s.pushErr
}

type stubCrmClient struct {
	lead *clients.LeadDTO
	err  error
}

func (s *stubCrmClient) GetLead(_ context.Context, _, _ string) (*clients.LeadDTO, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.lead, nil
}

func newPendingCharge(t *testing.T) *charge.Charge {
	t.Helper()
	c, err := charge.New("contract-1", "tenant-party-1", "owner-1", "prop-1", "tenant-1", charge.TypeRent, "Aluguel março", "2026-03", 1500.0, time.Now().AddDate(0, 0, 5))
	if err != nil {
		t.Fatalf("failed to build charge: %v", err)
	}
	return c
}

func TestChargeService_Notify_emailAndInApp(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{ID: "tenant-party-1", Name: "Maria Souza", Email: "maria@example.com", AccountID: "account-99"}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	res, err := svc.Notify(context.Background(), "tenant-1", c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Channels) != 2 {
		t.Fatalf("expected 2 channels, got %v", res.Channels)
	}
	if len(comm.emailCalls) != 1 || comm.emailCalls[0].Recipient != "maria@example.com" {
		t.Fatalf("unexpected email calls: %+v", comm.emailCalls)
	}
	if len(comm.pushCalls) != 1 {
		t.Fatalf("expected in-app push, got %d", len(comm.pushCalls))
	}
	// Audit trail recorded
	if len(c.RemindersSent) != 1 {
		t.Fatalf("expected 1 reminder entry, got %d", len(c.RemindersSent))
	}
	entry := c.RemindersSent[0]
	if entry.Recipient != "maria@example.com" {
		t.Fatalf("expected recipient maria@example.com, got %q", entry.Recipient)
	}
	if len(entry.Channels) != 2 {
		t.Fatalf("expected 2 channels in audit, got %v", entry.Channels)
	}
}

func TestChargeService_Notify_appendsOnEachCall(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	svc.SetCommClient(&stubCommClient{})
	svc.SetCrmClient(&stubCrmClient{lead: &clients.LeadDTO{Email: "x@y.com"}})

	for i := 0; i < 3; i++ {
		if _, err := svc.Notify(context.Background(), "tenant-1", c.ID); err != nil {
			t.Fatalf("call %d failed: %v", i, err)
		}
	}
	if len(c.RemindersSent) != 3 {
		t.Fatalf("expected 3 reminder entries after 3 calls, got %d", len(c.RemindersSent))
	}
}

func TestChargeService_Notify_skipsEmailWhenMissing(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	svc.SetCommClient(comm)
	svc.SetCrmClient(&stubCrmClient{lead: &clients.LeadDTO{ID: "tenant-party-1", Name: "WhatsApp Only"}})

	res, err := svc.Notify(context.Background(), "tenant-1", c.ID)
	if err == nil {
		t.Fatal("expected error when no channels delivered")
	}
	if len(comm.emailCalls) != 0 {
		t.Fatalf("email shouldn't have been called: %+v", comm.emailCalls)
	}
	if res == nil {
		t.Fatal("expected partial result")
	}
}

func TestChargeService_Notify_partialFailure(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{pushErr: errors.New("upstream timeout")}
	svc.SetCommClient(comm)
	svc.SetCrmClient(&stubCrmClient{lead: &clients.LeadDTO{ID: "tenant-party-1", Email: "maria@example.com", AccountID: "account-99"}})

	res, err := svc.Notify(context.Background(), "tenant-1", c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Channels) != 1 || res.Channels[0] != "email" {
		t.Fatalf("expected email only, got %v", res.Channels)
	}
	if len(res.SkippedNotes) == 0 {
		t.Fatal("expected skipped note for in_app")
	}
}

func TestChargeService_Notify_paidChargeRejected(t *testing.T) {
	c := newPendingCharge(t)
	c.Status = charge.StatusPaid
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	svc.SetCommClient(&stubCommClient{})
	svc.SetCrmClient(&stubCrmClient{lead: &clients.LeadDTO{Email: "x@y"}})

	if _, err := svc.Notify(context.Background(), "tenant-1", c.ID); err == nil {
		t.Fatal("expected error for paid charge")
	}
}

func TestChargeService_Notify_missingClients(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)

	if _, err := svc.Notify(context.Background(), "tenant-1", c.ID); err == nil {
		t.Fatal("expected error when comm client is not configured")
	}
}

// ─────────────────────────── WhatsApp dispatch (#443) ───────────────────────

func TestChargeService_Notify_dispatchesWhatsAppWhenLeadHasPhone(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{
		ID:        "tenant-party-1",
		Name:      "João Silva",
		Email:     "joao@example.com",
		Phone:     "+55 11 99999-9999",
		AccountID: "account-99",
	}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	res, err := svc.Notify(context.Background(), "tenant-1", c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Há 3 chamadas em SendNotification (email + whatsapp) e 1 em
	// PushInAppNotification (in_app). O switch fica no Type da request.
	var emailReqs, whatsappReqs int
	for _, req := range comm.emailCalls {
		switch req.Type {
		case "email":
			emailReqs++
		case "whatsapp":
			whatsappReqs++
		}
	}
	if emailReqs != 1 {
		t.Errorf("expected 1 email send, got %d", emailReqs)
	}
	if whatsappReqs != 1 {
		t.Errorf("expected 1 whatsapp send, got %d", whatsappReqs)
	}

	// O recipient da WhatsApp call deve ser o telefone como veio do CRM —
	// normalização para E.164 acontece dentro do toca-comm (não duplica).
	var whatsappRecipient string
	for _, req := range comm.emailCalls {
		if req.Type == "whatsapp" {
			whatsappRecipient = req.Recipient
		}
	}
	if whatsappRecipient != "+55 11 99999-9999" {
		t.Errorf("expected whatsapp recipient to be lead.Phone raw, got %q", whatsappRecipient)
	}

	// Resultado deve incluir "whatsapp" na lista de canais entregues.
	hasWhatsApp := false
	for _, ch := range res.Channels {
		if ch == "whatsapp" {
			hasWhatsApp = true
		}
	}
	if !hasWhatsApp {
		t.Errorf("expected whatsapp in delivered channels, got %v", res.Channels)
	}

	// Audit do charge deve registrar whatsapp também.
	if len(c.RemindersSent) != 1 {
		t.Fatalf("expected 1 reminder entry, got %d", len(c.RemindersSent))
	}
	foundWhatsApp := false
	for _, ch := range c.RemindersSent[0].Channels {
		if ch == "whatsapp" {
			foundWhatsApp = true
		}
	}
	if !foundWhatsApp {
		t.Errorf("expected whatsapp in audit, got %v", c.RemindersSent[0].Channels)
	}
}

func TestChargeService_Notify_skipsWhatsAppWhenLeadHasNoPhone(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{
		ID:    "tenant-party-1",
		Name:  "Sem telefone",
		Email: "x@y.com",
		// Phone is empty
	}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	res, err := svc.Notify(context.Background(), "tenant-1", c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, req := range comm.emailCalls {
		if req.Type == "whatsapp" {
			t.Errorf("whatsapp shouldn't have been called: %+v", req)
		}
	}

	// SkippedNotes deve mencionar whatsapp ausente.
	foundSkip := false
	for _, note := range res.SkippedNotes {
		if note == "whatsapp: lead has no phone on file" {
			foundSkip = true
		}
	}
	if !foundSkip {
		t.Errorf("expected SkippedNotes to mention missing phone, got %v", res.SkippedNotes)
	}
}

func TestChargeService_Notify_whatsappFailureFallsBackToSkippedNote(t *testing.T) {
	// Meta API devolveu erro (token inválido, rate limit, etc.) — não
	// deve quebrar o pipeline; email + in_app ainda devem sair.
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{emailErr: errors.New("whatsapp meta api error: status=401")}
	crm := &stubCrmClient{lead: &clients.LeadDTO{
		ID:        "tenant-party-1",
		Name:      "João",
		Email:     "joao@example.com",
		Phone:     "5511999999999",
		AccountID: "account-99",
	}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	res, err := svc.Notify(context.Background(), "tenant-1", c.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// emailErr está setado no stub e afeta TODAS as SendNotification
	// calls — então email E whatsapp falham aqui. Mas in_app (push)
	// não usa SendNotification, então ele entrega normalmente.
	foundInApp := false
	for _, ch := range res.Channels {
		if ch == "in_app" {
			foundInApp = true
		}
	}
	if !foundInApp {
		t.Errorf("expected in_app to be delivered when comm SendNotification fails, got channels %v", res.Channels)
	}
	if len(res.SkippedNotes) < 2 {
		t.Errorf("expected SkippedNotes for both email and whatsapp, got %v", res.SkippedNotes)
	}
}

func TestChargeService_Notify_whatsappChannelExplicit(t *testing.T) {
	// Régua de cobrança pode passar opts.Channels=["whatsapp"] para
	// pular email/in_app e mandar SÓ WhatsApp. Garante que o filtro
	// allowChannel respeita isso.
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{
		ID:        "tenant-party-1",
		Email:     "joao@example.com",
		Phone:     "5511999999999",
		AccountID: "account-99",
	}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	res, err := svc.NotifyWithOptions(context.Background(), "tenant-1", c.ID, NotifyOptions{
		Channels: []string{"whatsapp"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Channels) != 1 || res.Channels[0] != "whatsapp" {
		t.Errorf("expected only whatsapp channel, got %v", res.Channels)
	}
	for _, req := range comm.emailCalls {
		if req.Type == "email" {
			t.Errorf("email should NOT be called when channels=[whatsapp], got %+v", req)
		}
	}
	if len(comm.pushCalls) != 0 {
		t.Errorf("in_app should NOT be called when channels=[whatsapp], got %d calls", len(comm.pushCalls))
	}
}

// ───────────────── P1/A3: tenant explícito no push in-app ────────────────────
//
// A rota interna do toca-comm (`POST /comm/internal/notifications/:accountId`)
// resolve o tenant pelo chamador: com JWT de usuário vale o tenant do token;
// chamada de serviço (worker de cobrança, sem JWT) precisa do tenant no corpo —
// sem ele o resolver responde 400 e a notificação in-app nunca chega.
//
// Este teste prova que o lembrete de cobrança originado por worker carrega o
// tenant do charge no payload do push.
func TestChargeService_Notify_inAppPushCarriesTenant(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{
		ID:        "tenant-party-1",
		Name:      "Maria Souza",
		Email:     "maria@example.com",
		AccountID: "account-99",
	}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	if _, err := svc.Notify(context.Background(), "tenant-1", c.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(comm.pushCalls) != 1 {
		t.Fatalf("expected 1 in-app push, got %d", len(comm.pushCalls))
	}
	data, ok := comm.pushCalls[0].Data.(map[string]string)
	if !ok {
		t.Fatalf("expected push Data to be map[string]string, got %T", comm.pushCalls[0].Data)
	}
	if got := data["tenant_id"]; got != "tenant-1" {
		t.Fatalf("push in-app sem tenant: data[tenant_id]=%q (quer %q) — o resolver interno do toca-comm devolve 400 e a notificação não é criada", got, "tenant-1")
	}
}
