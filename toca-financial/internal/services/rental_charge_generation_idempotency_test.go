package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

// idempotentRentalChargeRepo simulates the database unique index on
// (tenant_id, source_event_id): a Create for an already-materialized
// source_event_id returns charge.ErrChargeAlreadyExists, and a matching
// failOn injection reproduces a transient mid-loop write failure that is
// NOT an idempotency conflict (e.g. a deadlock or network blip).
type idempotentRentalChargeRepo struct {
	existing map[string]bool
	created  []string
	failOn   string
}

func (r *idempotentRentalChargeRepo) Create(_ context.Context, c *charge.Charge) error {
	if r.existing[c.SourceEventID] {
		return charge.ErrChargeAlreadyExists
	}
	if r.failOn != "" && c.SourceEventID == r.failOn {
		return errors.New("injected charge create failure")
	}
	r.existing[c.SourceEventID] = true
	r.created = append(r.created, c.SourceEventID)
	return nil
}

func (r *idempotentRentalChargeRepo) Update(context.Context, *charge.Charge) error { return nil }

func (r *idempotentRentalChargeRepo) List(context.Context, string, string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}

func TestCreateAndGenerateChargesSkipsExistingBySourceEventID(t *testing.T) {
	cfg := readjustmentFixtureConfig()
	configRepo := &readjustmentConfigRepo{cfg: cfg}
	// The first charge was already materialized in a previous delivery.
	chargeRepo := &idempotentRentalChargeRepo{existing: map[string]bool{
		"rental.rental-config-readjustment.first.r0": true,
	}}
	svc := NewRentalChargeConfigService(configRepo, chargeRepo, nil)
	ctx := context.Background()

	_, created, err := svc.CreateAndGenerateCharges(
		ctx, cfg.ContractID, cfg.TenantID, cfg.TenantPartyID, cfg.OwnerID, cfg.PropertyID,
		cfg.StartDate, cfg.EndDate, cfg.FirstChargeDate, cfg.FirstChargeType, cfg.DefaultDueDay, cfg.MonthlyAmount,
	)
	if err != nil {
		t.Fatalf("CreateAndGenerateCharges: %v", err)
	}
	// 1 first + 11 monthly = 12 total; only the 11 monthly are new.
	if created != 11 {
		t.Fatalf("created = %d, want 11", created)
	}
	if len(chargeRepo.created) != 11 {
		t.Fatalf("newly materialized rows = %d, want 11", len(chargeRepo.created))
	}
}

func TestCreateAndGenerateChargesRetryRepairsAfterMidLoopFailure(t *testing.T) {
	cfg := readjustmentFixtureConfig()
	configRepo := &readjustmentConfigRepo{cfg: cfg}
	chargeRepo := &idempotentRentalChargeRepo{
		existing: map[string]bool{},
		failOn:   "rental.rental-config-readjustment.month.2026-03.r0",
	}
	svc := NewRentalChargeConfigService(configRepo, chargeRepo, nil)
	ctx := context.Background()

	_, _, err := svc.CreateAndGenerateCharges(
		ctx, cfg.ContractID, cfg.TenantID, cfg.TenantPartyID, cfg.OwnerID, cfg.PropertyID,
		cfg.StartDate, cfg.EndDate, cfg.FirstChargeDate, cfg.FirstChargeType, cfg.DefaultDueDay, cfg.MonthlyAmount,
	)
	if err == nil {
		t.Fatal("expected injected mid-loop create failure")
	}
	// first + Feb committed before the March failure.
	if len(chargeRepo.created) != 2 {
		t.Fatalf("rows before failure = %d, want 2", len(chargeRepo.created))
	}

	chargeRepo.failOn = ""
	_, created, err := svc.CreateAndGenerateCharges(
		ctx, cfg.ContractID, cfg.TenantID, cfg.TenantPartyID, cfg.OwnerID, cfg.PropertyID,
		cfg.StartDate, cfg.EndDate, cfg.FirstChargeDate, cfg.FirstChargeType, cfg.DefaultDueDay, cfg.MonthlyAmount,
	)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	// 12 total - 2 already materialized = 10 repaired rows.
	if created != 10 {
		t.Fatalf("retry created = %d, want 10", created)
	}
	if len(chargeRepo.created) != 12 {
		t.Fatalf("total rows after retry = %d, want 12", len(chargeRepo.created))
	}
}

func TestCreateConfigOnlyDoesNotMaterializeCharges(t *testing.T) {
	configRepo := &readjustmentConfigRepo{}
	chargeRepo := &idempotentRentalChargeRepo{existing: map[string]bool{}}
	svc := NewRentalChargeConfigService(configRepo, chargeRepo, nil)
	ctx := context.Background()

	start := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	_, err := svc.CreateConfigOnly(
		ctx, "contract-x", "tenant-x", "payer-x", "owner-x", "property-x",
		start, time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.January, 5, 0, 0, 0, 0, time.UTC),
		rental_charge_config.SameMonthProRata, 5, 100000,
	)
	if err != nil {
		t.Fatalf("CreateConfigOnly: %v", err)
	}
	if len(chargeRepo.created) != 0 {
		t.Fatalf("CreateConfigOnly materialized %d charges, want 0", len(chargeRepo.created))
	}
}
