package service_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	service "github.com/oginoino/toca/toca-signature/internal/service"
)

func staleEnvelope(id, externalID string) *envelope.SignatureEnvelope {
	return &envelope.SignatureEnvelope{
		ID:                 id,
		TenantID:           "tenant-1",
		ContractID:         "contract-" + id,
		ExternalEnvelopeID: externalID,
		Status:             envelope.StatusRunning,
		Signers:            []envelope.Signer{{SignerID: "sig-1", Email: "a@x.com", Status: envelope.SignerStatusPending}},
		UpdatedAt:          time.Now().Add(-2 * time.Hour),
	}
}

// O sweeper existe porque depender só de webhook não basta: a entrega pode
// atrasar ou se perder, e o envelope fica preso em RUNNING do nosso lado mesmo
// já finalizado no provedor — deixando o contrato em PENDING_SIGNATURE sem
// ninguém perceber. Foi o que aconteceu em produção, com
// `signature.webhook_events` vazio.
func TestReconcileSweeper_ReconcilesStaleEnvelopes(t *testing.T) {
	repo := newFakeRepo()
	env := staleEnvelope("env-1", "ext-1")
	repo.byID[env.ID] = env
	repo.staleActive = []*envelope.SignatureEnvelope{env}

	cs := &fakeClicksign{
		listDocsResponse: []clicksign.DocumentResource{
			{
				ID:   "doc-1",
				Type: "documents",
				Attributes: clicksign.DocumentAttributes{
					Filename:      "Contrato.pdf",
					SignedFileURL: "https://app.clicksign.test/signed/abc.pdf",
				},
			},
		},
	}
	svc := service.NewEnvelopeService(
		repo,
		&reconcileFakeCS{fakeClicksign: cs, status: "closed"},
		&stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, &captureBus{},
	)
	sweeper := service.NewReconcileSweeper(svc, repo, time.Hour)

	if synced := sweeper.Run(context.Background()); synced != 1 {
		t.Fatalf("envelopes sincronizados = %d, want 1", synced)
	}
	// O estado local precisa acompanhar o provedor: era exatamente isso que
	// faltava quando o webhook não chegava.
	if env.Status != envelope.StatusFinished {
		t.Errorf("status = %s, want FINISHED após a varredura", env.Status)
	}
	// A varredura precisa ler pelo pool service-role: sem JWT, a mesma leitura
	// pelo pool de aplicação devolve zero linhas sob RLS (medido no beta:
	// 0 envelopes de app contra 2 de serviço para o mesmo WHERE), e o sweeper
	// fica decorativo.
	if got := atomic.LoadInt32(&repo.serviceReads); got != 1 {
		t.Errorf("leituras service-scope = %d, want 1 (a varredura roda sem JWT)", got)
	}
}

// Um provedor instável não pode impedir a sincronização dos demais: se a
// varredura abortasse no primeiro erro, os outros contratos seguiriam presos.
func TestReconcileSweeper_ContinuesAfterOneFailure(t *testing.T) {
	repo := newFakeRepo()
	ok := staleEnvelope("env-ok", "ext-ok")
	problem := staleEnvelope("env-problem", "ext-problem")
	repo.byID[ok.ID] = ok
	repo.byID[problem.ID] = problem
	// O problemático vem primeiro, de propósito.
	repo.staleActive = []*envelope.SignatureEnvelope{problem, ok}
	// O primeiro Reconcile falha porque o envelope não existe no repositório.
	delete(repo.byID, problem.ID)

	cs := &fakeClicksign{
		listDocsResponse: []clicksign.DocumentResource{
			{
				ID: "doc-1", Type: "documents",
				Attributes: clicksign.DocumentAttributes{
					Filename: "Contrato.pdf", SignedFileURL: "https://app.clicksign.test/signed/abc.pdf",
				},
			},
		},
	}
	svc := service.NewEnvelopeService(
		repo,
		&reconcileFakeCS{fakeClicksign: cs, status: "closed"},
		&stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, &captureBus{},
	)
	sweeper := service.NewReconcileSweeper(svc, repo, time.Hour)

	if synced := sweeper.Run(context.Background()); synced != 1 {
		t.Fatalf("sincronizados = %d, want 1 (a falha de um não pode travar o outro)", synced)
	}
	if ok.Status != envelope.StatusFinished {
		t.Errorf("envelope saudável ficou em %s; a falha do outro não pode impedir sua sincronização", ok.Status)
	}
}

func TestReconcileSweepIntervalFromEnv(t *testing.T) {
	t.Setenv("SIGNATURE_RECONCILE_INTERVAL_MINUTES", "")
	if got := service.ReconcileSweepIntervalFromEnv(); got != service.DefaultReconcileSweepInterval {
		t.Errorf("vazio = %s, want padrão %s", got, service.DefaultReconcileSweepInterval)
	}
	t.Setenv("SIGNATURE_RECONCILE_INTERVAL_MINUTES", "2")
	if got := service.ReconcileSweepIntervalFromEnv(); got != 2*time.Minute {
		t.Errorf("2 = %s, want 2m", got)
	}
	// Valor inválido não pode zerar a varredura em silêncio.
	t.Setenv("SIGNATURE_RECONCILE_INTERVAL_MINUTES", "abc")
	if got := service.ReconcileSweepIntervalFromEnv(); got != service.DefaultReconcileSweepInterval {
		t.Errorf("inválido = %s, want padrão", got)
	}
}
