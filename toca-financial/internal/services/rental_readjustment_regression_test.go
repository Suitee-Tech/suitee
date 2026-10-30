package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

type readjustmentConfigRepo struct {
	cfg         *rental_charge_config.RentalChargeConfig
	updateCalls int
}

func (r *readjustmentConfigRepo) Create(_ context.Context, cfg *rental_charge_config.RentalChargeConfig) error {
	r.cfg = cfg
	return nil
}

func (r *readjustmentConfigRepo) GetByID(_ context.Context, tenantID, id string) (*rental_charge_config.RentalChargeConfig, error) {
	if r.cfg != nil && r.cfg.TenantID == tenantID && r.cfg.ID == id {
		return r.cfg, nil
	}
	return nil, nil
}

func (r *readjustmentConfigRepo) GetByContractID(_ context.Context, tenantID, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	if r.cfg != nil && r.cfg.TenantID == tenantID && r.cfg.ContractID == contractID {
		return r.cfg, nil
	}
	return nil, nil
}

func (r *readjustmentConfigRepo) Update(_ context.Context, cfg *rental_charge_config.RentalChargeConfig) error {
	r.updateCalls++
	r.cfg = cfg
	return nil
}

func (r *readjustmentConfigRepo) Delete(_ context.Context, _, _ string) error { return nil }

type readjustmentChargeRepo struct {
	charges    []*charge.Charge
	updatedIDs []string
}

// repairingReadjustmentChargeRepo behaves like a database repository: List
// returns detached snapshots, and mutations become durable only after a
// successful Update. That distinction is essential to reproduce a mid-loop
// write failure without accidentally changing the fixture through pointers.
type repairingReadjustmentChargeRepo struct {
	charges        []*charge.Charge
	updateAttempts int
	failAttempt    int
}

func cloneReadjustmentCharge(item *charge.Charge) *charge.Charge {
	if item == nil {
		return nil
	}
	cloned := *item
	cloned.LineItems = append([]charge.LineItem(nil), item.LineItems...)
	cloned.Payments = append([]charge.PaymentEntry(nil), item.Payments...)
	if item.PaidAmount != nil {
		paid := *item.PaidAmount
		cloned.PaidAmount = &paid
	}
	if item.PaidAt != nil {
		paidAt := *item.PaidAt
		cloned.PaidAt = &paidAt
	}
	return &cloned
}

func (r *repairingReadjustmentChargeRepo) Create(_ context.Context, item *charge.Charge) error {
	r.charges = append(r.charges, cloneReadjustmentCharge(item))
	return nil
}

func (r *repairingReadjustmentChargeRepo) Update(_ context.Context, item *charge.Charge) error {
	r.updateAttempts++
	if r.failAttempt > 0 && r.updateAttempts == r.failAttempt {
		return errors.New("injected charge update failure")
	}
	for i, stored := range r.charges {
		if stored.ID == item.ID && stored.TenantID == item.TenantID {
			r.charges[i] = cloneReadjustmentCharge(item)
			return nil
		}
	}
	return errors.New("charge not found")
}

func (r *repairingReadjustmentChargeRepo) List(_ context.Context, tenantID, contractID, _, _ string, limit, offset int64) ([]*charge.Charge, int64, error) {
	filtered := make([]*charge.Charge, 0, len(r.charges))
	for _, item := range r.charges {
		if item.TenantID == tenantID && item.ContractID == contractID {
			filtered = append(filtered, cloneReadjustmentCharge(item))
		}
	}
	total := int64(len(filtered))
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func (r *repairingReadjustmentChargeRepo) byID(id string) *charge.Charge {
	for _, item := range r.charges {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func (r *readjustmentChargeRepo) Create(_ context.Context, item *charge.Charge) error {
	r.charges = append(r.charges, item)
	return nil
}

func (r *readjustmentChargeRepo) Update(_ context.Context, item *charge.Charge) error {
	r.updatedIDs = append(r.updatedIDs, item.ID)
	return nil
}

func (r *readjustmentChargeRepo) List(_ context.Context, tenantID, contractID, _, _ string, limit, offset int64) ([]*charge.Charge, int64, error) {
	filtered := make([]*charge.Charge, 0, len(r.charges))
	for _, item := range r.charges {
		if item.TenantID == tenantID && item.ContractID == contractID {
			filtered = append(filtered, item)
		}
	}
	total := int64(len(filtered))
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return filtered[offset:end], total, nil
}

func readjustmentFixtureConfig() *rental_charge_config.RentalChargeConfig {
	createdAt := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	return &rental_charge_config.RentalChargeConfig{
		ID:                "rental-config-readjustment",
		TenantID:          "tenant-readjustment",
		PropertyID:        "property-readjustment",
		ContractID:        "contract-readjustment",
		TenantPartyID:     "payer-readjustment",
		OwnerID:           "owner-readjustment",
		StartDate:         createdAt,
		EndDate:           time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
		FirstChargeDate:   time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
		FirstChargeType:   rental_charge_config.SameMonthProRata,
		DefaultDueDay:     5,
		MonthlyAmount:     100000,
		BaseMonthlyAmount: 100000,
		CreatedAt:         createdAt,
		UpdatedAt:         createdAt,
	}
}

func readjustmentFixtureCharge(t *testing.T, cfg *rental_charge_config.RentalChargeConfig, month time.Month, status charge.Status) *charge.Charge {
	t.Helper()
	due := time.Date(2026, month, 5, 0, 0, 0, 0, time.UTC)
	item, err := charge.New(
		cfg.ContractID,
		cfg.TenantPartyID,
		cfg.OwnerID,
		cfg.PropertyID,
		cfg.TenantID,
		charge.TypeRent,
		"Aluguel + Encargos",
		due.Format("2006-01"),
		1000,
		due,
	)
	if err != nil {
		t.Fatalf("create charge fixture: %v", err)
	}
	item.ID = "charge-" + due.Format("2006-01")
	item.Status = status
	if status == charge.StatusPartiallyPaid {
		paid := valueobjects.NewMoney(40000)
		item.PaidAmount = &paid
	}
	if status == charge.StatusPaid {
		paid := valueobjects.NewMoney(100000)
		item.PaidAmount = &paid
	}
	return item
}

func TestApplyReadjustmentHonorsEffectiveDateAndLateOutOfOrderDelivery(t *testing.T) {
	cfg := readjustmentFixtureConfig()
	junePending := readjustmentFixtureCharge(t, cfg, time.June, charge.StatusPending)
	augustPending := readjustmentFixtureCharge(t, cfg, time.August, charge.StatusPending)
	septemberPartial := readjustmentFixtureCharge(t, cfg, time.September, charge.StatusPartiallyPaid)
	octoberPaid := readjustmentFixtureCharge(t, cfg, time.October, charge.StatusPaid)
	configRepo := &readjustmentConfigRepo{cfg: cfg}
	chargeRepo := &readjustmentChargeRepo{charges: []*charge.Charge{
		junePending, augustPending, septemberPartial, octoberPaid,
	}}
	svc := NewRentalChargeConfigService(configRepo, chargeRepo, nil)
	ctx := context.Background()

	updatedCfg, updated, err := svc.ApplyReadjustment(
		ctx,
		cfg.TenantID,
		cfg.ContractID,
		"readjustment-july",
		120000,
		time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("apply July readjustment: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected only August pending charge to update, got %d", updated)
	}
	if junePending.TotalAmount.Cents() != 100000 || augustPending.TotalAmount.Cents() != 120000 {
		t.Fatalf("effective-date gate failed: June=%d August=%d", junePending.TotalAmount.Cents(), augustPending.TotalAmount.Cents())
	}
	if septemberPartial.TotalAmount.Cents() != 100000 || octoberPaid.TotalAmount.Cents() != 100000 {
		t.Fatalf("settled/partial charges must be immutable: September=%d October=%d", septemberPartial.TotalAmount.Cents(), octoberPaid.TotalAmount.Cents())
	}
	if updatedCfg.MonthlyAmount != 120000 {
		t.Fatalf("expected current scheduled amount 120000, got %d", updatedCfg.MonthlyAmount)
	}

	// This older-effective event is delivered after the July event. June must
	// pick the May amount, while August must keep the later July amount.
	_, updated, err = svc.ApplyReadjustment(
		ctx,
		cfg.TenantID,
		cfg.ContractID,
		"readjustment-may-late-delivery",
		110000,
		time.Date(2026, time.May, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.April, 15, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("apply out-of-order May readjustment: %v", err)
	}
	if updated != 2 {
		t.Fatalf("expected both eligible pending rows to be reconciled, got %d", updated)
	}
	if junePending.TotalAmount.Cents() != 110000 {
		t.Fatalf("June must use May-effective amount, got %d", junePending.TotalAmount.Cents())
	}
	if augustPending.TotalAmount.Cents() != 120000 {
		t.Fatalf("August must retain later July-effective amount, got %d", augustPending.TotalAmount.Cents())
	}
	if cfg.MonthlyAmount != 120000 {
		t.Fatalf("late older-effective delivery reverted current amount to %d", cfg.MonthlyAmount)
	}
	if len(cfg.Adjustments) != 2 || cfg.Adjustments[0].EventID != "readjustment-may-late-delivery" || cfg.Adjustments[1].EventID != "readjustment-july" {
		t.Fatalf("expected sorted adjustment schedule, got %+v", cfg.Adjustments)
	}
}

func TestApplyReadjustmentRetryRepairsRowsAfterMidLoopFailure(t *testing.T) {
	cfg := readjustmentFixtureConfig()
	august := readjustmentFixtureCharge(t, cfg, time.August, charge.StatusPending)
	september := readjustmentFixtureCharge(t, cfg, time.September, charge.StatusPending)
	configRepo := &readjustmentConfigRepo{cfg: cfg}
	chargeRepo := &repairingReadjustmentChargeRepo{
		charges:     []*charge.Charge{cloneReadjustmentCharge(august), cloneReadjustmentCharge(september)},
		failAttempt: 2,
	}
	svc := NewRentalChargeConfigService(configRepo, chargeRepo, nil)
	ctx := context.Background()
	effective := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	occurred := time.Date(2026, time.June, 15, 0, 0, 0, 0, time.UTC)

	_, _, err := svc.ApplyReadjustment(
		ctx, cfg.TenantID, cfg.ContractID, "readjustment-repair", 120000, effective, occurred,
	)
	if err == nil {
		t.Fatal("expected injected mid-loop update failure")
	}
	if configRepo.updateCalls != 1 {
		t.Fatalf("schedule should have persisted before charge failure, got %d config updates", configRepo.updateCalls)
	}
	if got := chargeRepo.byID(august.ID).TotalAmount.Cents(); got != 120000 {
		t.Fatalf("first row should have committed before failure, got %d", got)
	}
	if got := chargeRepo.byID(september.ID).TotalAmount.Cents(); got != 100000 {
		t.Fatalf("failed row must remain at old value, got %d", got)
	}

	_, updated, err := svc.ApplyReadjustment(
		ctx, cfg.TenantID, cfg.ContractID, "readjustment-repair", 120000, effective, occurred,
	)
	if err != nil {
		t.Fatalf("same event retry should repair remaining rows: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected retry to update only the stranded row, got %d", updated)
	}
	if configRepo.updateCalls != 1 {
		t.Fatalf("idempotent retry must not rewrite schedule, got %d config updates", configRepo.updateCalls)
	}
	if got := chargeRepo.byID(september.ID).TotalAmount.Cents(); got != 120000 {
		t.Fatalf("retry did not converge stranded row, got %d", got)
	}

	attemptsAfterRepair := chargeRepo.updateAttempts
	_, updated, err = svc.ApplyReadjustment(
		ctx, cfg.TenantID, cfg.ContractID, "readjustment-repair", 120000, effective, occurred,
	)
	if err != nil || updated != 0 {
		t.Fatalf("fully converged retry must be a no-op: updated=%d err=%v", updated, err)
	}
	if chargeRepo.updateAttempts != attemptsAfterRepair {
		t.Fatalf("fully converged retry issued another write: before=%d after=%d", attemptsAfterRepair, chargeRepo.updateAttempts)
	}
}
