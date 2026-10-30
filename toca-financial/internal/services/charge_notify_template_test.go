package services

import (
	"context"
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
)

func TestRenderChargeTemplate_substitutesAllPlaceholders(t *testing.T) {
	c := newPendingCharge(t)
	c.Description = "Aluguel mar/2026"
	lead := &clients.LeadDTO{Name: "Maria Souza"}

	out := renderChargeTemplate(
		"Olá {tenant_name}, sua cobrança de {amount} vence em {due_date}. Total atualizado: {total_amount}. Ref: {description}.",
		c,
		lead,
	)
	if !strings.Contains(out, "Maria Souza") {
		t.Errorf("expected tenant_name substituted, got: %s", out)
	}
	if !strings.Contains(out, "R$ ") {
		t.Errorf("expected R$ formatted amount, got: %s", out)
	}
	if !strings.Contains(out, "Aluguel mar/2026") {
		t.Errorf("expected description substituted, got: %s", out)
	}
	if strings.Contains(out, "{") {
		t.Errorf("no curly braces should remain when all keys are known, got: %s", out)
	}
}

func TestRenderChargeTemplate_unknownPlaceholdersLeftAlone(t *testing.T) {
	c := newPendingCharge(t)
	lead := &clients.LeadDTO{Name: "X"}
	out := renderChargeTemplate("Olá {foo}, sua {amount}.", c, lead)
	if !strings.Contains(out, "{foo}") {
		t.Errorf("expected unknown placeholder kept verbatim, got: %s", out)
	}
}

func TestRenderChargeTemplate_missingPayerNameFallsBackEmpty(t *testing.T) {
	c := newPendingCharge(t)
	lead := &clients.LeadDTO{}
	out := renderChargeTemplate("Olá {tenant_name}!", c, lead)
	if !strings.Contains(out, "Olá !") {
		t.Errorf("expected empty substitution when lead has no name, got: %q", out)
	}
}

func TestNotifyWithOptions_usesRuleTemplate(t *testing.T) {
	c := newPendingCharge(t)
	c.Description = "Aluguel mar/2026"
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{Name: "Maria", Email: "m@x.com"}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	_, err := svc.NotifyWithOptions(context.Background(), "t-1", c.ID, NotifyOptions{
		Template:    "Custom: {tenant_name} deve {amount}",
		SentBy:      "rule:rule-99",
		SentByLabel: "Régua: 3 dias antes",
		Channels:    []string{"email"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(comm.emailCalls) != 1 {
		t.Fatalf("expected one email call, got %d", len(comm.emailCalls))
	}
	body := comm.emailCalls[0].Content
	if !strings.Contains(body, "Custom: Maria deve R$") {
		t.Fatalf("expected rule template rendered, got: %q", body)
	}
	// Audit recorded both sent_by and the snapshot label
	if len(c.RemindersSent) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(c.RemindersSent))
	}
	entry := c.RemindersSent[0]
	if entry.SentBy != "rule:rule-99" {
		t.Errorf("expected SentBy=rule:rule-99, got %q", entry.SentBy)
	}
	if entry.SentByLabel != "Régua: 3 dias antes" {
		t.Errorf("expected SentByLabel snapshot, got %q", entry.SentByLabel)
	}
}

func TestNotifyWithOptions_channelFilterDropsOthers(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{Email: "x@y.com", AccountID: "acc"}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	// Rule asks for whatsapp only (not yet implemented) — engine should
	// surface a clear skip note and refuse to dispatch via other channels.
	_, err := svc.NotifyWithOptions(context.Background(), "t-1", c.ID, NotifyOptions{
		Channels: []string{"whatsapp"},
	})
	if err == nil {
		t.Fatal("expected error when no channel actually delivered")
	}
	if len(comm.emailCalls) != 0 || len(comm.pushCalls) != 0 {
		t.Fatalf("channel filter should have prevented email/push, got email=%d push=%d", len(comm.emailCalls), len(comm.pushCalls))
	}
}

func TestNotifyWithOptions_channelFilterAllowsOnlyEmail(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	comm := &stubCommClient{}
	crm := &stubCrmClient{lead: &clients.LeadDTO{Email: "x@y.com", AccountID: "acc"}}
	svc.SetCommClient(comm)
	svc.SetCrmClient(crm)

	res, err := svc.NotifyWithOptions(context.Background(), "t-1", c.ID, NotifyOptions{
		Channels: []string{"email"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Channels) != 1 || res.Channels[0] != "email" {
		t.Fatalf("expected only email, got %v", res.Channels)
	}
	if len(comm.pushCalls) != 0 {
		t.Fatalf("in_app should be filtered out, got %d push calls", len(comm.pushCalls))
	}
}

// Backward-compat: the legacy Notify(ctx, t, id) signature must still work
// for the manual button and any existing caller.
func TestNotify_legacySignatureStillWorks(t *testing.T) {
	c := newPendingCharge(t)
	repo := &mockChargeRepo{ch: c}
	svc := NewChargeService(repo, nil)
	svc.SetCommClient(&stubCommClient{})
	svc.SetCrmClient(&stubCrmClient{lead: &clients.LeadDTO{Email: "x@y.com"}})

	if _, err := svc.Notify(context.Background(), "t-1", c.ID); err != nil {
		t.Fatalf("legacy Notify failed: %v", err)
	}
}
