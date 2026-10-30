package rental_charge_config

import (
	"testing"
	"time"
)

func TestIntermediationPlanCarriesUnfundedCommissionAcrossRentPayments(t *testing.T) {
	t.Parallel()

	cfg := &RentalChargeConfig{ContractID: "contract-1"}
	changed, err := cfg.FreezeIntermediationPlan(IntermediationPlan{
		DealID:           "deal-1",
		PlanHash:         "plan-1",
		TotalAmountCents: 500000,
		Shares: []IntermediationShare{
			{BrokerID: "broker-1", Role: "fechamento", TargetAmountCents: 300000},
			{BrokerID: "broker-2", Role: "captador", TargetAmountCents: 100000},
		},
	})
	if err != nil || !changed {
		t.Fatalf("freeze plan = (%v, %v)", changed, err)
	}
	if cfg.IntermediationPlan.AgencyRetainedTargetCents != 100000 {
		t.Fatalf("agency target = %d, want 100000", cfg.IntermediationPlan.AgencyRetainedTargetCents)
	}

	links := []struct {
		brokerID, role, commissionID, payoutID string
	}{
		{"broker-1", "fechamento", "commission-1", "payout-1"},
		{"broker-2", "captador", "commission-2", "payout-2"},
	}
	for _, link := range links {
		if changed, err := cfg.LinkIntermediationCommission(link.brokerID, link.role, link.commissionID); err != nil || !changed {
			t.Fatalf("link commission %s = (%v, %v)", link.commissionID, changed, err)
		}
		if changed, err := cfg.LinkIntermediationPayout(link.brokerID, link.role, link.payoutID); err != nil || !changed {
			t.Fatalf("link payout %s = (%v, %v)", link.payoutID, changed, err)
		}
	}

	firstAt := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	first, changed, err := cfg.AllocateIntermediationFunding(
		"payment-aug", "charge-aug", "2026-08", 225000, firstAt,
	)
	if err != nil || !changed {
		t.Fatalf("first funding = (%+v, %v, %v)", first, changed, err)
	}
	if first.AppliedCents != 225000 || first.OutstandingAfterCents != 275000 || first.AgencyRetainedCents != 0 {
		t.Fatalf("first funding waterfall = %+v", first)
	}
	// Shares are sorted by role, so captador is covered before fechamento.
	if len(first.BrokerFunding) != 2 || first.BrokerFunding[0].CommissionID != "commission-2" || first.BrokerFunding[0].AmountCents != 100000 || first.BrokerFunding[1].CommissionID != "commission-1" || first.BrokerFunding[1].AmountCents != 125000 {
		t.Fatalf("deterministic broker priority = %+v", first.BrokerFunding)
	}

	retry, changed, err := cfg.AllocateIntermediationFunding(
		"payment-aug", "charge-aug", "2026-08", 225000, firstAt,
	)
	if err != nil || changed || retry.AppliedCents != first.AppliedCents || len(cfg.IntermediationPlan.FundingEntries) != 1 {
		t.Fatalf("idempotent retry = (%+v, %v, %v), entries=%d", retry, changed, err, len(cfg.IntermediationPlan.FundingEntries))
	}

	secondAt := time.Date(2026, time.September, 10, 12, 0, 0, 0, time.UTC)
	second, changed, err := cfg.AllocateIntermediationFunding(
		"payment-sep", "charge-sep", "2026-09", 450000, secondAt,
	)
	if err != nil || !changed {
		t.Fatalf("second funding = (%+v, %v, %v)", second, changed, err)
	}
	if second.AppliedCents != 275000 || second.OutstandingAfterCents != 0 || second.AgencyRetainedCents != 100000 {
		t.Fatalf("carry-forward completion = %+v", second)
	}
	if cfg.IntermediationOutstandingCents() != 0 || cfg.IntermediationPlan.Shares[1].FundedAmountCents != 300000 {
		t.Fatalf("final plan state = %+v", cfg.IntermediationPlan)
	}
}

func TestIntermediationPlanRejectsPaymentIdentityConflict(t *testing.T) {
	t.Parallel()

	cfg := &RentalChargeConfig{}
	if _, err := cfg.FreezeIntermediationPlan(IntermediationPlan{
		PlanHash: "plan-1", TotalAmountCents: 100000,
		Shares: []IntermediationShare{{BrokerID: "broker-1", Role: "captador", TargetAmountCents: 100000}},
	}); err != nil {
		t.Fatal(err)
	}
	when := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	if _, _, err := cfg.AllocateIntermediationFunding("payment-1", "charge-1", "2026-08", 50000, when); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cfg.AllocateIntermediationFunding("payment-1", "charge-2", "2026-08", 50000, when); err == nil {
		t.Fatal("expected payment identity conflict")
	}
}
