package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// Regressão do incidente de 30/09/2026 (contrato 277b965e no beta): o payload
// v3 da Clicksign carrega só document.key — nunca envelope.id. O handler
// precisa (a) resolver o envelope pelo fallback de document key, (b) aplicar o
// evento e (c) religar a linha de auditoria para o envelope real.
//
// Quando (a) quebrou no banco (jsonb_array_elements sobre metadata->'documents'
// que era JSON null), TODO webhook — signature_started, sign, document_closed —
// terminou em "envelope not found locally" com o envelope existindo na tabela,
// e o contrato ficou preso em PENDING_SIGNATURE com o envelope já fechado na
// Clicksign.
func TestWebhookResolvesEnvelopeByDocumentKeyAndRelinksAudit(t *testing.T) {
	repo := newFakeRepo()
	repo.webhookID = "webhook-1"
	env := &envelope.SignatureEnvelope{
		ID:                 "env-1",
		TenantID:           "tenant-1",
		ContractID:         "contract-1",
		ExternalEnvelopeID: "clicksign-env-1",
		Status:             envelope.StatusPending,
		Signers: []envelope.Signer{
			{SignerID: "signer-1", ClicksignSignerKey: "key-1", Status: envelope.SignerStatusPending},
		},
	}
	// O webhook só traz o document.key: o lookup por id externo não acha nada e
	// a resolução tem de vir do fallback.
	repo.byDocumentKey["doc-key-1"] = env

	svc := service.NewWebhookService(repo, nil)
	payload := clicksign.WebhookPayload{
		Document: &clicksign.WebhookDocument{Key: "doc-key-1"},
		Event:    clicksign.WebhookEvent{Name: "signature_started"},
	}
	if err := svc.Process(context.Background(), payload, []byte(`{"event":{"name":"signature_started"}}`), "delivery-1", "1.2.3.4", "sig"); err != nil {
		t.Fatalf("Process: %v", err)
	}

	if env.Status != envelope.StatusRunning {
		t.Errorf("status do envelope = %q, quero RUNNING", env.Status)
	}
	if len(repo.reassociated) != 1 {
		t.Fatalf("relinks de auditoria = %d, quero 1", len(repo.reassociated))
	}
	got := repo.reassociated[0]
	if got.externalEnvelopeID != "clicksign-env-1" || got.tenantID != "tenant-1" {
		t.Errorf("relink = %+v, quero external=clicksign-env-1 tenant=tenant-1", got)
	}
}

// O relink de auditoria é acessório: se ele falhar, o webhook AINDA precisa ser
// aplicado. Perder estado de assinatura por causa de uma linha de auditoria
// seria trocar um problema pequeno por um grande.
func TestWebhookAppliesEventEvenWhenAuditRelinkFails(t *testing.T) {
	repo := newFakeRepo()
	repo.webhookID = "webhook-1"
	repo.reassociateErr = errors.New("banco indisponível")
	env := &envelope.SignatureEnvelope{
		ID:                 "env-1",
		TenantID:           "tenant-1",
		ContractID:         "contract-1",
		ExternalEnvelopeID: "clicksign-env-1",
		Status:             envelope.StatusPending,
	}
	repo.byDocumentKey["doc-key-1"] = env

	svc := service.NewWebhookService(repo, nil)
	payload := clicksign.WebhookPayload{
		Document: &clicksign.WebhookDocument{Key: "doc-key-1"},
		Event:    clicksign.WebhookEvent{Name: "signature_started"},
	}
	if err := svc.Process(context.Background(), payload, []byte(`{}`), "delivery-2", "1.2.3.4", "sig"); err != nil {
		t.Fatalf("Process devolveu erro por causa do relink de auditoria: %v", err)
	}
	if env.Status != envelope.StatusRunning {
		t.Errorf("status do envelope = %q, quero RUNNING", env.Status)
	}
}
