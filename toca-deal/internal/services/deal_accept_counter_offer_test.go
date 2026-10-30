package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func dealWithCounterOffer(counterValue float64) *deal.Deal {
	v := counterValue
	return &deal.Deal{
		ID:       "deal-counter-1",
		TenantID: "tenant-A",
		Type:     deal.TypeSale,
		Status:   deal.StatusNegotiation,
		Value:    1_000_000,
		Proposals: []deal.Proposal{
			{
				ID:           "prop-1",
				Value:        1_000_000,
				CounterValue: &v,
				Status:       deal.ProposalStatusCounter,
				CreatedAt:    time.Now(),
			},
		},
	}
}

func TestAcceptCounterOffer_Success(t *testing.T) {
	ctx := context.Background()
	d := dealWithCounterOffer(850_000)
	svc := testService(d)

	got, err := svc.AcceptCounterOffer(ctx, d.TenantID, d.ID, "prop-1", "actor-1", "Admin User")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got.Value != 850_000 {
		t.Errorf("Deal.Value should be updated to counter value, got %f", got.Value)
	}
	if got.Proposals[0].Status != deal.ProposalStatusAccepted {
		t.Errorf("proposal status should be ACCEPTED, got %q", got.Proposals[0].Status)
	}
}

func TestAcceptCounterOffer_WrongStatus(t *testing.T) {
	ctx := context.Background()
	d := dealWithCounterOffer(850_000)
	d.Proposals[0].Status = deal.ProposalStatusAccepted // já aceito
	svc := testService(d)

	_, err := svc.AcceptCounterOffer(ctx, d.TenantID, d.ID, "prop-1", "actor-1", "")
	if !errors.Is(err, ErrProposalNotInCounterOffer) {
		t.Errorf("expected ErrProposalNotInCounterOffer, got %v", err)
	}
}

func TestAcceptCounterOffer_RejectedStatus(t *testing.T) {
	ctx := context.Background()
	d := dealWithCounterOffer(850_000)
	d.Proposals[0].Status = deal.ProposalStatusRejected
	svc := testService(d)

	_, err := svc.AcceptCounterOffer(ctx, d.TenantID, d.ID, "prop-1", "actor-1", "")
	if !errors.Is(err, ErrProposalNotInCounterOffer) {
		t.Errorf("expected ErrProposalNotInCounterOffer for REJECTED, got %v", err)
	}
}

func TestAcceptCounterOffer_DealNotFound(t *testing.T) {
	// Repo com algum deal mas pedindo ID diferente — `deal not found`.
	ctx := context.Background()
	svc := testService(dealWithCounterOffer(850_000))
	_, err := svc.AcceptCounterOffer(ctx, "tenant-A", "missing-deal", "prop-1", "actor-1", "")
	if err == nil || err.Error() != "deal not found" {
		t.Errorf("expected 'deal not found', got %v", err)
	}
}

func TestAcceptCounterOffer_ProposalNotFound(t *testing.T) {
	ctx := context.Background()
	d := dealWithCounterOffer(850_000)
	svc := testService(d)

	_, err := svc.AcceptCounterOffer(ctx, d.TenantID, d.ID, "missing-prop", "actor-1", "")
	if err == nil || err.Error() != "proposal not found" {
		t.Errorf("expected 'proposal not found', got %v", err)
	}
}

func TestAcceptCounterOffer_NilCounterValue_KeepsDealValue(t *testing.T) {
	// Edge case: proposal está em COUNTER_OFFER mas sem CounterValue (dirty
	// data). Aceitar mantém Deal.Value original — não zera nem regride.
	ctx := context.Background()
	d := dealWithCounterOffer(850_000)
	d.Proposals[0].CounterValue = nil
	svc := testService(d)

	got, err := svc.AcceptCounterOffer(ctx, d.TenantID, d.ID, "prop-1", "actor-1", "")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got.Value != 1_000_000 {
		t.Errorf("Deal.Value should remain when CounterValue is nil, got %f", got.Value)
	}
	if got.Proposals[0].Status != deal.ProposalStatusAccepted {
		t.Errorf("status should still be ACCEPTED")
	}
}
