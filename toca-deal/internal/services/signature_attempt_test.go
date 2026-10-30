package services_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

type attemptSignatureClient struct {
	fakeSignatureClient
	current *clients.SignatureEnvelopeDTO
	readErr error
}

func (f *attemptSignatureClient) GetByContract(context.Context, string, string) (*clients.SignatureEnvelopeDTO, error) {
	return f.current, f.readErr
}

type attemptDocumentSpy struct {
	service.DocumentStatusSyncer
	calls int
}

func (s *attemptDocumentSpy) SyncFromContractStatus(context.Context, string, string, string) (int, error) {
	s.calls++
	return 0, nil
}

type attemptDealSpy struct {
	*mockDealRepo
	writes   int
	writeErr error
}

func (s *attemptDealSpy) Update(ctx context.Context, d *deal.Deal) error {
	s.writes++
	if s.writeErr != nil {
		return s.writeErr
	}
	return s.mockDealRepo.Update(ctx, d)
}

func TestSignatureFailureEventsDoNotRegressOtherAttemptsOrFinalStates(t *testing.T) {
	for _, eventName := range []string{"signature.refused", "signature.expired"} {
		for _, tc := range []struct {
			name                 string
			status               contract.Status
			externalID, signerID string
		}{
			{name: "previous attempt", status: contract.StatusPending, externalID: "old-envelope", signerID: "sig-1"},
			{name: "missing correlation", status: contract.StatusPending, signerID: "sig-1"},
			{name: "unknown signer", status: contract.StatusPending, externalID: "current-envelope", signerID: "other-signer"},
			{name: "signed", status: contract.StatusSigned, externalID: "current-envelope", signerID: "sig-1"},
			{name: "active", status: contract.StatusActive, externalID: "current-envelope", signerID: "sig-1"},
			{name: "terminated", status: contract.StatusTerminated, externalID: "current-envelope", signerID: "sig-1"},
			{name: "expired contract", status: contract.StatusExpired, externalID: "current-envelope", signerID: "sig-1"},
		} {
			t.Run(eventName+"/"+tc.name, func(t *testing.T) {
				c := sampleSignedContract()
				c.Status, c.ExternalEnvelopeID = tc.status, "current-envelope"
				now := time.Now()
				c.SignatureLockedAt, c.UpdatedAt = &now, now
				before := *c
				before.Signatories = append([]contract.Signatory(nil), c.Signatories...)
				repo := &mockContractRepo{c: c}
				d, _ := deal.New(deal.TypeRent, "property", "owner", "client", "broker", 1000, "tenant-1")
				deals := &attemptDealSpy{mockDealRepo: &mockDealRepo{d: d}}
				documents := &attemptDocumentSpy{}
				svc := service.NewContractService(repo, deals, &recordingBus{}, nil).WithDocumentSyncer(documents)
				handler := service.NewSignatureRefusedHandler(svc)
				if eventName == "signature.expired" {
					handler = service.NewSignatureExpiredHandler(svc)
				}
				// Expiration is envelope-level; an unknown signer only applies to refusal.
				if eventName == "signature.expired" && tc.name == "unknown signer" {
					return
				}
				err := handler(context.Background(), stubEvent{name: eventName, payload: map[string]interface{}{
					"tenant_id": "tenant-1", "contract_id": c.ID, "external_envelope_id": tc.externalID, "signer_id": tc.signerID,
				}})
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(*repo.c, before) || documents.calls != 0 || deals.writes != 0 {
					t.Fatalf("ignored event changed contract/projections: contract=%+v documents=%d deals=%d", repo.c, documents.calls, deals.writes)
				}
			})
		}
	}
}

func TestSignatureFailureBeforeStartedCorrelatesLatestEnvelopeAndSendTime(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		age                  time.Duration
		externalID           string
		readErr              error
		wantDraft, wantError bool
	}{
		{name: "current attempt", age: time.Second, externalID: "current-envelope", wantDraft: true},
		{name: "old event", age: time.Second, externalID: "old-envelope"},
		{name: "old envelope before worker created new one", age: -time.Second, externalID: "current-envelope"},
		{name: "signature outage must retry", externalID: "current-envelope", readErr: errors.New("unavailable"), wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := sampleSignedContract()
			now := time.Now()
			c.Status, c.SentForSignatureAt, c.SignatureLockedAt = contract.StatusPartiallySigned, &now, &now
			signature := &attemptSignatureClient{current: &clients.SignatureEnvelopeDTO{ContractID: c.ID, ExternalEnvelopeID: tc.externalID, CreatedAt: now.Add(tc.age)}, readErr: tc.readErr}
			if tc.name == "old event" {
				signature.current.ExternalEnvelopeID = "current-envelope"
			}
			repo := &mockContractRepo{c: c}
			svc := service.NewContractService(repo, &mockDealRepo{}, nil, nil, signature)
			err := svc.HandleSignatureRefused(context.Background(), c.TenantID, c.ID, tc.externalID, "sig-1", "refused")
			if (err != nil) != tc.wantError {
				t.Fatalf("unexpected error: %v", err)
			}
			if (repo.c.Status == contract.StatusDraft) != tc.wantDraft {
				t.Fatalf("unexpected status: %s", repo.c.Status)
			}
		})
	}
}

func TestSignatureRefusalConcurrentWritePreservesContractAndProjections(t *testing.T) {
	c := sampleSignedContract()
	c.Status, c.ExternalEnvelopeID = contract.StatusPending, "current-envelope"
	now := time.Now()
	c.SignatureLockedAt = &now
	base := &mockContractRepo{c: c}
	documents := &attemptDocumentSpy{}
	svc := service.NewContractService(&conflictingContractRepo{base}, &mockDealRepo{}, nil, nil).WithDocumentSyncer(documents)
	err := svc.HandleSignatureRefused(context.Background(), c.TenantID, c.ID, c.ExternalEnvelopeID, "sig-1", "refused")
	if !errors.Is(err, contract.ErrConcurrentModification) || base.c.Status != contract.StatusPending || base.c.SignatureLockedAt == nil || documents.calls != 0 {
		t.Fatalf("failed CAS modified contract/projections: %+v error=%v", base.c, err)
	}
}

func TestEnvelopeStartedRecordsPendingAttemptWithoutReopeningDraft(t *testing.T) {
	c := sampleSignedContract()
	now := time.Now()
	c.Status, c.SentForSignatureAt = contract.StatusPending, &now
	signature := &attemptSignatureClient{current: &clients.SignatureEnvelopeDTO{ContractID: c.ID, ExternalEnvelopeID: "current-envelope", CreatedAt: now.Add(time.Second)}}
	repo := &mockContractRepo{c: c}
	svc := service.NewContractService(repo, &mockDealRepo{}, nil, nil, signature)
	if err := svc.MarkEnvelopeStartedExternally(context.Background(), c.TenantID, c.ID, "current-envelope"); err != nil {
		t.Fatal(err)
	}
	if repo.c.ExternalEnvelopeID != "current-envelope" {
		t.Fatal("pending attempt was not recorded")
	}
	repo.c.Status = contract.StatusDraft
	if err := svc.MarkEnvelopeStartedExternally(context.Background(), c.TenantID, c.ID, "current-envelope"); err != nil {
		t.Fatal(err)
	}
	if repo.c.Status != contract.StatusDraft {
		t.Fatal("late start reopened refused draft")
	}
}

func TestSendForSignatureClearsPreviousEnvelopeCorrelation(t *testing.T) {
	c := sampleSignedContract()
	c.ExternalEnvelopeID = "old-envelope"
	svc, repo := newSvc(t, c, &recordingBus{})
	if _, err := svc.SendForSignature(context.Background(), c.TenantID, c.ID, service.SendForSignatureRequest{}); err != nil {
		t.Fatal(err)
	}
	if repo.c.ExternalEnvelopeID != "" {
		t.Fatal("new send inherited previous envelope")
	}
}

func TestSignatureFailureRedeliveryRecoversProjectionWithoutRewritingDraft(t *testing.T) {
	for _, signerID := range []string{"sig-1", ""} {
		c := sampleSignedContract()
		c.Status, c.ExternalEnvelopeID = contract.StatusPending, "current-envelope"
		repo := &mockContractRepo{c: c}
		d, _ := deal.New(deal.TypeRent, "property", "owner", "client", "broker", 1000, "tenant-1")
		deals := &attemptDealSpy{mockDealRepo: &mockDealRepo{d: d}, writeErr: errors.New("projection unavailable")}
		svc := service.NewContractService(repo, deals, nil, nil)
		if err := svc.HandleSignatureRefused(context.Background(), c.TenantID, c.ID, c.ExternalEnvelopeID, signerID, "refused"); err == nil {
			t.Fatal("expected projection error")
		}
		if repo.c.Status != contract.StatusDraft {
			t.Fatal("refusal was not persisted")
		}
		committedAt := repo.c.UpdatedAt
		deals.writeErr = nil
		if err := svc.HandleSignatureRefused(context.Background(), c.TenantID, c.ID, c.ExternalEnvelopeID, signerID, "refused"); err != nil {
			t.Fatal(err)
		}
		if deals.writes != 2 || !repo.c.UpdatedAt.Equal(committedAt) {
			t.Fatal("redelivery did not recover projection or rewrote contract")
		}
	}
}
