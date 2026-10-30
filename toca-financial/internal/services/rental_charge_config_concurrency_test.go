package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

type conflictingRentalConfigRepo struct {
	canonical *rental_charge_config.RentalChargeConfig
	lookups   int
	creates   int
}

func (r *conflictingRentalConfigRepo) Create(_ context.Context, _ *rental_charge_config.RentalChargeConfig) error {
	r.creates++
	return rental_charge_config.ErrAlreadyExists
}

func (r *conflictingRentalConfigRepo) GetByID(context.Context, string, string) (*rental_charge_config.RentalChargeConfig, error) {
	return nil, nil
}

func (r *conflictingRentalConfigRepo) GetByContractID(context.Context, string, string) (*rental_charge_config.RentalChargeConfig, error) {
	r.lookups++
	if r.lookups == 1 {
		return nil, nil
	}
	return r.canonical, nil
}

func (r *conflictingRentalConfigRepo) Update(context.Context, *rental_charge_config.RentalChargeConfig) error {
	return nil
}

func (r *conflictingRentalConfigRepo) Delete(context.Context, string, string) error { return nil }

func TestCreateConfigOnlyReturnsCanonicalWinnerAfterUniqueConflict(t *testing.T) {
	start := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	canonical, err := rental_charge_config.New(
		"tenant-1", "property-1", "contract-1", "payer-1", "owner-1",
		start, start.AddDate(1, 0, 0), start, rental_charge_config.SameMonthProRata, 10, 100_000,
	)
	if err != nil {
		t.Fatalf("create canonical config: %v", err)
	}
	canonical.ID = "canonical-config"
	repo := &conflictingRentalConfigRepo{canonical: canonical}
	svc := NewRentalChargeConfigService(repo, nil, nil)

	got, err := svc.CreateConfigOnly(
		context.Background(), "contract-1", "tenant-1", "payer-1", "owner-1", "property-1",
		start, start.AddDate(1, 0, 0), start, rental_charge_config.SameMonthProRata, 10, 100_000,
	)
	if err != nil {
		t.Fatalf("CreateConfigOnly() error = %v", err)
	}
	if got != canonical || got.ID != "canonical-config" {
		t.Fatalf("CreateConfigOnly() = %+v, want canonical winner", got)
	}
	if repo.creates != 1 || repo.lookups != 2 {
		t.Fatalf("unexpected repository calls: creates=%d lookups=%d", repo.creates, repo.lookups)
	}
}

func TestCreateConfigOnlyFailsClosedWhenConflictWinnerCannotBeRead(t *testing.T) {
	start := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	repo := &conflictingRentalConfigRepo{}
	svc := NewRentalChargeConfigService(repo, nil, nil)

	_, err := svc.CreateConfigOnly(
		context.Background(), "contract-1", "tenant-1", "payer-1", "owner-1", "property-1",
		start, start.AddDate(1, 0, 0), start, rental_charge_config.SameMonthProRata, 10, 100_000,
	)
	if err == nil || errors.Is(err, rental_charge_config.ErrAlreadyExists) {
		t.Fatalf("CreateConfigOnly() error = %v, want canonical-row consistency error", err)
	}
}
