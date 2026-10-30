package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

type transferLifecycleRaceRepo struct {
	*chargeIdentityTransferRepo
	injectDeduction bool
	casCalls        int
}

func (r *transferLifecycleRaceRepo) UpdateIfVersion(ctx context.Context, item *transfer.OwnerTransfer, expected time.Time) (bool, error) {
	r.casCalls++
	if r.injectDeduction {
		r.injectDeduction = false
		for index, stored := range r.items {
			if stored.ID != item.ID || stored.TenantID != item.TenantID {
				continue
			}
			concurrent := cloneOwnerTransfer(stored)
			applyTransferDeduction(concurrent, transfer.Deduction{
				Description: "concurrent owner expense",
				Amount:      5,
				Type:        "UTILITY",
				ReferenceID: fmt.Sprintf("race.ref-%d", r.casCalls),
			})
			concurrent.UpdatedAt = expected.Add(time.Second)
			r.items[index] = concurrent
			return false, nil
		}
	}
	return r.chargeIdentityTransferRepo.UpdateIfVersion(ctx, item, expected)
}

func TestTransferLifecycleCASPreservesConcurrentDeductions(t *testing.T) {
	item := transfer.New("contract-race", "owner-race", "property-race", "tenant-race", "2026-08", 100, 0, 0, 0, nil, "")
	item.ID = "transfer-race"
	base := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{cloneOwnerTransfer(item)}}
	repo := &transferLifecycleRaceRepo{chargeIdentityTransferRepo: base, injectDeduction: true}
	svc := NewTransferService(repo)
	ctx := context.Background()

	err := svc.Approve(ctx, item.TenantID, item.ID, "operator", "Operator")
	if !errors.Is(err, transfer.ErrConcurrentModification) {
		t.Fatalf("Approve() error=%v, want ErrConcurrentModification", err)
	}
	stored, _ := repo.GetByID(ctx, item.TenantID, item.ID)
	if stored.Status != transfer.StatusPending || stored.OtherDeductions != 5 || len(stored.DeductionDetails) != 1 {
		t.Fatalf("stale approve overwrote concurrent deduction: %+v", stored)
	}
	if err := svc.Approve(ctx, item.TenantID, item.ID, "operator", "Operator"); err != nil {
		t.Fatalf("Approve() retry: %v", err)
	}
	stored, _ = repo.GetByID(ctx, item.TenantID, item.ID)
	if stored.Status != transfer.StatusApproved || stored.OtherDeductions != 5 || len(stored.DeductionDetails) != 1 {
		t.Fatalf("approve retry did not preserve deduction: %+v", stored)
	}

	repo.injectDeduction = true
	err = svc.Process(ctx, item.TenantID, item.ID, "operator-2", "Operator 2")
	if !errors.Is(err, transfer.ErrConcurrentModification) {
		t.Fatalf("Process() error=%v, want ErrConcurrentModification", err)
	}
	stored, _ = repo.GetByID(ctx, item.TenantID, item.ID)
	if stored.Status != transfer.StatusApproved || stored.OtherDeductions != 10 || len(stored.DeductionDetails) != 2 {
		t.Fatalf("stale process overwrote concurrent deduction: %+v", stored)
	}
	if err := svc.Process(ctx, item.TenantID, item.ID, "operator-2", "Operator 2"); err != nil {
		t.Fatalf("Process() retry: %v", err)
	}
	stored, _ = repo.GetByID(ctx, item.TenantID, item.ID)
	if stored.Status != transfer.StatusProcessed || stored.OtherDeductions != 10 || len(stored.DeductionDetails) != 2 {
		t.Fatalf("process retry did not preserve deductions: %+v", stored)
	}
}
