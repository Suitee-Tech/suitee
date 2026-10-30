package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// captureBus implements pkgevents.EventBus and stashes every published
// event so tests can assert on the wire shape the deal-side
// subscribers will see. Only Publish/PublishReliable are exercised —
// Subscribe* return nil since Reconcile never subscribes.
type captureBus struct {
	mu       sync.Mutex
	reliable []events.Event
	fired    []events.Event
}

func (b *captureBus) Publish(_ context.Context, e events.Event) error {
	b.mu.Lock()
	b.fired = append(b.fired, e)
	b.mu.Unlock()
	return nil
}
func (b *captureBus) PublishReliable(_ context.Context, e events.Event) error {
	b.mu.Lock()
	b.reliable = append(b.reliable, e)
	b.mu.Unlock()
	return nil
}
func (b *captureBus) Subscribe(_ string, _ events.EventHandler) error { return nil }
func (b *captureBus) SubscribeWithQueue(_, _ string, _ events.EventHandler) error {
	return nil
}

func (b *captureBus) findFinishedEvent(t *testing.T) pkgevents.SignatureEnvelopeFinishedEvent {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, e := range append(append([]events.Event(nil), b.reliable...), b.fired...) {
		if fin, ok := e.(pkgevents.SignatureEnvelopeFinishedEvent); ok {
			return fin
		}
	}
	t.Fatalf("expected an envelope_finished event in the bus, got reliable=%d fired=%d",
		len(b.reliable), len(b.fired))
	return pkgevents.SignatureEnvelopeFinishedEvent{}
}

// TestReconcile_FinishedHydratesSignedDocURL pins the Phase 3 reconcile
// gap fix: when Clicksign reports the envelope as closed/finished but
// the local envelope is still RUNNING, Reconcile must fetch the
// signed-file URL via ListDocuments and include it in the
// envelope_finished event. Without that hydration, the toca-deal
// persist subscriber stays in no-op and operators see "Assinado /
// 404".
func TestReconcile_FinishedHydratesSignedDocURL(t *testing.T) {
	repo := newFakeRepo()
	// Seed an envelope that's still RUNNING locally (we missed the
	// finish webhook) and has a Clicksign ID we can reconcile against.
	signed := time.Date(2026, 5, 17, 10, 0, 0, 0, time.UTC)
	env := &envelope.SignatureEnvelope{
		ID:                 "env-1",
		TenantID:           "tenant-1",
		ContractID:         "contract-1",
		ExternalEnvelopeID: "ext-1",
		Status:             envelope.StatusRunning,
		CreatedAt:          signed,
		UpdatedAt:          signed,
		Signers: []envelope.Signer{
			{SignerID: "sig-1", Email: "a@x.com", Status: envelope.SignerStatusPending},
		},
	}
	repo.byID[env.ID] = env
	repo.byContract[env.ContractID] = env

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
	// Override GetEnvelope to report closed so Reconcile takes the
	// finished branch (default fake returns "running").
	bus := &captureBus{}
	svc := service.NewEnvelopeService(repo, &reconcileFakeCS{fakeClicksign: cs, status: "closed"}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	out, err := svc.Reconcile(context.Background(), "tenant-1", env.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != envelope.StatusFinished {
		t.Errorf("envelope status = %s, want FINISHED", out.Status)
	}
	if cs.listDocsCalls != 1 {
		t.Errorf("ListDocuments calls = %d, want 1 (must query Clicksign to hydrate URL)", cs.listDocsCalls)
	}
	finished := bus.findFinishedEvent(t)
	if finished.Data.SignedDocumentURL != "https://app.clicksign.test/signed/abc.pdf" {
		t.Errorf("envelope_finished payload SignedDocumentURL = %q; want the URL ListDocuments returned",
			finished.Data.SignedDocumentURL)
	}
}

// TestReconcile_FinishedPicksContractSignedURL covers the Clicksign v3 shape:
// the signed-PDF URL lives under links.files.signed, and a multi-document
// envelope lists attachments alongside the contract (Clicksign does not
// guarantee the contract is first). Reconcile must put the CONTRACT
// document's signed URL on the envelope_finished event — never the
// attachment's.
func TestReconcile_FinishedPicksContractSignedURL(t *testing.T) {
	repo := newFakeRepo()
	env := &envelope.SignatureEnvelope{
		ID: "env-2", TenantID: "tenant-1", ContractID: "contract-2",
		ExternalEnvelopeID: "ext-2", Status: envelope.StatusRunning,
	}
	repo.byID[env.ID] = env
	repo.byContract[env.ContractID] = env

	cs := &fakeClicksign{
		listDocsResponse: []clicksign.DocumentResource{
			{ // attachment — Clicksign lists it first
				ID:         "doc-anexo",
				Attributes: clicksign.DocumentAttributes{Filename: "anexo-123.pdf"},
				Links:      clicksign.DocumentLinks{Files: clicksign.DocumentLinkFiles{Signed: "https://app.clicksign.test/signed/anexo.pdf"}},
			},
			{ // the contract — toca uploads it as contrato-<contractID>.pdf
				ID:         "doc-contrato",
				Attributes: clicksign.DocumentAttributes{Filename: "contrato-contract-2.pdf"},
				Links:      clicksign.DocumentLinks{Files: clicksign.DocumentLinkFiles{Signed: "https://app.clicksign.test/signed/contrato.pdf"}},
			},
		},
	}
	bus := &captureBus{}
	svc := service.NewEnvelopeService(repo, &reconcileFakeCS{fakeClicksign: cs, status: "finished"}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	if _, err := svc.Reconcile(context.Background(), "tenant-1", env.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	finished := bus.findFinishedEvent(t)
	if finished.Data.SignedDocumentURL != "https://app.clicksign.test/signed/contrato.pdf" {
		t.Errorf("envelope_finished SignedDocumentURL = %q; want the CONTRACT document's signed URL from links.files.signed",
			finished.Data.SignedDocumentURL)
	}
}

// TestReconcile_FinishedToleratesEmptyDocList covers the edge case
// where Clicksign reports closed but ListDocuments returns nothing
// (the file is genuinely not ready yet). Reconcile should still
// finish the envelope and publish the event with an empty URL — the
// backfill job picks it up on the next sweep.
func TestReconcile_FinishedToleratesEmptyDocList(t *testing.T) {
	repo := newFakeRepo()
	env := &envelope.SignatureEnvelope{
		ID: "env-3", TenantID: "tenant-1", ContractID: "contract-3",
		ExternalEnvelopeID: "ext-3", Status: envelope.StatusRunning,
	}
	repo.byID[env.ID] = env
	repo.byContract[env.ContractID] = env

	cs := &fakeClicksign{listDocsResponse: nil} // empty
	bus := &captureBus{}
	svc := service.NewEnvelopeService(repo, &reconcileFakeCS{fakeClicksign: cs, status: "closed"}, &stubCreds{}, &fakeDeal{contract: sampleContract()}, &stubPDF{}, bus)

	out, err := svc.Reconcile(context.Background(), "tenant-1", env.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != envelope.StatusFinished {
		t.Errorf("envelope must still flip to FINISHED even when URL not ready")
	}
	finished := bus.findFinishedEvent(t)
	if finished.Data.SignedDocumentURL != "" {
		t.Errorf("expected empty SignedDocumentURL, got %q", finished.Data.SignedDocumentURL)
	}
}

// reconcileFakeCS wraps fakeClicksign overriding only GetEnvelope so
// tests can pin the Clicksign-reported status without changing the
// default test behaviour ("running") that other suites rely on.
type reconcileFakeCS struct {
	*fakeClicksign
	status string
}

func (r *reconcileFakeCS) GetEnvelope(_ context.Context, _, envelopeID string) (*clicksign.EnvelopeResource, error) {
	r.fakeClicksign.getCalls++
	return &clicksign.EnvelopeResource{
		ID:         envelopeID,
		Type:       "envelopes",
		Attributes: clicksign.EnvelopeAttributes{Status: r.status},
	}, nil
}
