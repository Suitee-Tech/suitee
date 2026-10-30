package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
)

func saleRepairFixturePlan() *sale_billing_plan.SaleBillingPlan {
	plan, err := sale_billing_plan.New(
		"tenant-1", "contract-1", "deal-1", "buyer-1", "owner-1", "property-1",
		sale_billing_plan.ModeInstallments,
		70000, 10000, 20000, 3,
		time.Date(2026, time.January, 10, 0, 0, 0, 0, time.UTC),
		30,
		sale_billing_plan.CompositeInput{},
	)
	if err != nil {
		panic(err)
	}
	plan.ID = "sale-plan-repair"
	return plan
}

func TestSaleBillingPlanRetryRepairsMissingInstallments(t *testing.T) {
	plan := saleRepairFixturePlan()
	planRepo := &canonicalSalePlanRepo{canonical: cloneSaleBillingPlan(plan)}
	chargeRepo := newConcurrentSaleChargeRepo()
	chargeRepo.failOn = "sale.sale-plan-repair.installment.2"
	chargeRepo.failLeft = 1

	service := NewSaleBillingPlanService(planRepo, NewChargeService(chargeRepo, nil))
	ctx := context.Background()

	_, _, err := service.CreateAndGenerateCharges(
		ctx, plan.TenantID, plan.ContractID, plan.DealID, plan.BuyerPartyID,
		plan.OwnerID, plan.PropertyID, plan.Mode, plan.TotalAmountCents,
		plan.DownPaymentCents, plan.InstallmentAmountCents, plan.InstallmentCount,
		plan.FirstDueDate, plan.RecurrenceIntervalDays, sale_billing_plan.CompositeInput{},
	)
	if err == nil {
		t.Fatal("expected injected mid-loop create failure")
	}
	// downpayment + installment.1 committed before installment.2 failed.
	if got := len(chargeRepo.charges()); got != 2 {
		t.Fatalf("charges before failure = %d, want 2", got)
	}

	chargeRepo.failOn = ""
	_, created, err := service.CreateAndGenerateCharges(
		ctx, plan.TenantID, plan.ContractID, plan.DealID, plan.BuyerPartyID,
		plan.OwnerID, plan.PropertyID, plan.Mode, plan.TotalAmountCents,
		plan.DownPaymentCents, plan.InstallmentAmountCents, plan.InstallmentCount,
		plan.FirstDueDate, plan.RecurrenceIntervalDays, sale_billing_plan.CompositeInput{},
	)
	if err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	// 4 total - 2 already materialized = 2 repaired legs.
	if created != 2 {
		t.Fatalf("retry created = %d, want 2", created)
	}
	if got := len(chargeRepo.charges()); got != 4 {
		t.Fatalf("charges after retry = %d, want 4", got)
	}
	if chargeRepo.charges()["sale.sale-plan-repair.installment.2"] == nil {
		t.Fatal("installment.2 was not repaired on retry")
	}
}
