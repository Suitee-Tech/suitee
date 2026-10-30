package services_test

import (
	"context"
	"errors"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	service "github.com/oginoino/toca/toca-deal/internal/services"
	"reflect"
	"testing"
	"time"
)

func TestSignatureSuccessEventsIgnoreOldAttemptsAndFinalStates(t *testing.T) {
	for _, event := range []string{"signature.signer_signed", "signature.envelope_finished"} {
		for _, tc := range []struct {
			name     string
			status   contract.Status
			envelope string
		}{
			{"old attempt", contract.StatusPending, "old"}, {"missing attempt", contract.StatusPending, ""},
			{"draft after refusal", contract.StatusDraft, "current"}, {"active", contract.StatusActive, "current"},
			{"terminated", contract.StatusTerminated, "current"}, {"expired", contract.StatusExpired, "current"},
		} {
			t.Run(event+"/"+tc.name, func(t *testing.T) {
				c := sampleSignedContract()
				c.Status = tc.status
				c.ExternalEnvelopeID = "current"
				c.UpdatedAt = time.Now()
				before := *c
				before.Signatories = append([]contract.Signatory(nil), c.Signatories...)
				bus := &recordingBus{}
				svc, repo := newSvc(t, c, bus)
				handler := service.NewSignerSignedHandler(svc)
				if event == "signature.envelope_finished" {
					handler = service.NewEnvelopeFinishedHandler(svc)
				}
				err := handler(context.Background(), stubEvent{name: event, payload: map[string]interface{}{"tenant_id": c.TenantID, "contract_id": c.ID, "signer_id": "sig-1", "external_envelope_id": tc.envelope}})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*repo.c, before) || len(bus.published) != 0 {
					t.Fatal("obsolete success changed contract or emitted event")
				}
			})
		}
	}
}

func TestSignatureSuccessCASDoesNotPublishOnConcurrentResend(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "current"
	repo := &conflictingContractRepo{mockContractRepo: &mockContractRepo{c: c}}
	bus := &recordingBus{}
	svc := service.NewContractService(repo, nil, bus, nil)
	// Individual signer path must not mutate the repository-owned slice on CAS failure.
	err := svc.MarkSignerSignedExternally(context.Background(), c.TenantID, c.ID, "sig-1", time.Now(), "", "current")
	if !errors.Is(err, contract.ErrConcurrentModification) {
		t.Fatalf("expected revision conflict: %v", err)
	}
	if c.Signatories[0].SignedAt != nil || len(bus.published) != 0 {
		t.Fatal("failed CAS leaked signer changes")
	}
}

func TestSignerSuccessRedeliveryPreservesOriginalEvidenceAndRevision(t *testing.T) {
	c := sampleSignedContract()
	c.Status = contract.StatusPending
	c.ExternalEnvelopeID = "current"
	svc, repo := newSvc(t, c, &recordingBus{})
	first := time.Now().Add(-time.Hour)
	if err := svc.MarkSignerSignedExternally(context.Background(), c.TenantID, c.ID, "sig-1", first, "https://evidence/first", "current"); err != nil {
		t.Fatal(err)
	}
	revision := repo.c.UpdatedAt
	if err := svc.MarkSignerSignedExternally(context.Background(), c.TenantID, c.ID, "sig-1", time.Now(), "https://evidence/redelivery", "current"); err != nil {
		t.Fatal(err)
	}
	if !repo.c.Signatories[0].SignedAt.Equal(first) || repo.c.Signatories[0].EvidenceURL != "https://evidence/first" || !repo.c.UpdatedAt.Equal(revision) {
		t.Fatal("redelivery rewrote signature evidence")
	}
}
