package services

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan_draft"
)

type controlledSaleDraftRepo struct {
	draft        *sale_billing_plan_draft.Draft
	consumeErr   error
	consumeCalls int
}

func (r *controlledSaleDraftRepo) Upsert(_ context.Context, d *sale_billing_plan_draft.Draft) error {
	r.draft = d
	return nil
}

func (r *controlledSaleDraftRepo) GetActiveByDealForService(_ context.Context, _, _ string) (*sale_billing_plan_draft.Draft, error) {
	return r.draft, nil
}
func (r *controlledSaleDraftRepo) GetActiveByDeal(_ context.Context, _, _ string) (*sale_billing_plan_draft.Draft, error) {
	return r.draft, nil
}

func (r *controlledSaleDraftRepo) MarkConsumed(_ context.Context, _, _ string) error {
	r.consumeCalls++
	return r.consumeErr
}

func newSingleSaleDraftForTest(t *testing.T, total int64) *sale_billing_plan_draft.Draft {
	t.Helper()
	draft, err := sale_billing_plan_draft.New(
		"tenant-1", "deal-1", sale_billing_plan.ModeSingle,
		total, 0, total, 1,
		time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC),
		30, sale_billing_plan.CompositeInput{}, "operator-1",
	)
	if err != nil {
		t.Fatalf("build draft: %v", err)
	}
	return draft
}

func TestCreateFromDraftFailsWhenConsumptionCannotJoinMaterialization(t *testing.T) {
	draftRepo := &controlledSaleDraftRepo{
		draft:      newSingleSaleDraftForTest(t, 100000),
		consumeErr: errors.New("consume write failed"),
	}
	service := NewSaleBillingPlanService(
		&frozenValueSalePlanRepo{},
		NewChargeService(&mockChargeRepo{}, nil),
	).WithDraftRepo(draftRepo)

	plan, created, err := service.CreateFromDraft(
		context.Background(), "tenant-1", "contract-1", "deal-1",
		"buyer-1", "owner-1", "property-1", 100000,
	)
	if err == nil || !strings.Contains(err.Error(), "mark sale draft consumed") {
		t.Fatalf("CreateFromDraft() error = %v, want consumption failure", err)
	}
	if plan != nil || created != 0 {
		t.Fatalf("failed materialization returned success data: plan=%v created=%d", plan, created)
	}
	if draftRepo.consumeCalls != 1 {
		t.Fatalf("consume calls = %d, want 1", draftRepo.consumeCalls)
	}
}

func TestCreateFromDraftRepairsConsumptionForExistingPlan(t *testing.T) {
	due := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	plan, err := sale_billing_plan.New(
		"tenant-1", "contract-1", "deal-1", "buyer-1", "owner-1", "property-1",
		sale_billing_plan.ModeSingle, 100000, 0, 100000, 1, due, 30,
		sale_billing_plan.CompositeInput{},
	)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	draftRepo := &controlledSaleDraftRepo{draft: newSingleSaleDraftForTest(t, 100000)}
	service := NewSaleBillingPlanService(
		&frozenValueSalePlanRepo{plan: plan},
		NewChargeService(&mockChargeRepo{}, nil),
	).WithDraftRepo(draftRepo)

	got, _, err := service.CreateFromDraft(
		context.Background(), "tenant-1", "contract-1", "deal-1",
		"buyer-1", "owner-1", "property-1", 100000,
	)
	if err != nil {
		t.Fatalf("CreateFromDraft() repair error = %v", err)
	}
	if got != plan {
		t.Fatalf("CreateFromDraft() plan = %p, want existing %p", got, plan)
	}
	if draftRepo.consumeCalls != 1 {
		t.Fatalf("consume repair calls = %d, want 1", draftRepo.consumeCalls)
	}
}
