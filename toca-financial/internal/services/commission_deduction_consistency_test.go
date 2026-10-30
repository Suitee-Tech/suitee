package services

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommissionDeductionNaturalKeyMatchesMigrationFixture(t *testing.T) {
	deduction := normalizeDeductionIdentity("tenant-1", "commission-1", commission.Deduction{
		Kind: commission.DeductionIRRF, Description: " IRRF retido ", Amount: 10,
	})
	want := events.DeriveNotificationID(
		"commission_deduction", "tenant-1", "commission-1", "IRRF", "IRRF retido", "10",
	)
	assert.Equal(t, "9e96282b-cd70-54a9-b549-22e4a3a1781d", want)
	assert.Equal(t, want, deduction.ID)
}

type deductionConsistencyRepo struct {
	CommissionRepository
	commission  *commission.Commission
	updates     int
	syncUpdates int
	payoutCents int64
	syncErr     error
}

func cloneCommissionForTest(c *commission.Commission) *commission.Commission {
	if c == nil {
		return nil
	}
	encoded, _ := json.Marshal(c)
	var cloned commission.Commission
	_ = json.Unmarshal(encoded, &cloned)
	return &cloned
}

func (r *deductionConsistencyRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	if r.commission == nil || r.commission.TenantID != tenantID || r.commission.ID != id {
		return nil, nil
	}
	return cloneCommissionForTest(r.commission), nil
}

func (r *deductionConsistencyRepo) Update(_ context.Context, c *commission.Commission) error {
	r.updates++
	r.commission = cloneCommissionForTest(c)
	return nil
}

func (r *deductionConsistencyRepo) UpdateWithLinkedPayoutNet(_ context.Context, c *commission.Commission) error {
	if r.syncErr != nil {
		return r.syncErr
	}
	r.syncUpdates++
	r.payoutCents = int64(c.NetAmount*100 + 0.5)
	r.commission = cloneCommissionForTest(c)
	return nil
}

func TestCommissionDeductionRetryIsIdempotent(t *testing.T) {
	comm := commission.New("deal-deduction", "broker-deduction", 100, "tenant-deduction")
	repo := &deductionConsistencyRepo{commission: comm}
	service := NewCommissionService(repo)
	deduction := commission.Deduction{Kind: commission.DeductionIRRF, Description: "IRRF retido", Amount: 10}

	first, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, deduction)
	require.NoError(t, err)
	second, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, deduction)
	require.NoError(t, err)

	require.Len(t, second.Deductions, 1)
	assert.NotEmpty(t, second.Deductions[0].ID)
	assert.Equal(t, first.Deductions[0].ID, second.Deductions[0].ID)
	assert.InDelta(t, 90.0, second.NetAmount, 0.001)
	assert.Equal(t, 1, repo.updates, "retry must not write or append the deduction again")
}

func TestCommissionDeductionSynchronizesLinkedPendingPayoutNet(t *testing.T) {
	comm := commission.New("deal-linked", "broker-linked", 100, "tenant-linked")
	comm.PayoutOrderID = "payout-linked"
	repo := &deductionConsistencyRepo{commission: comm}
	service := NewCommissionService(repo)

	updated, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, commission.Deduction{
		ID: "deduction-linked", Kind: commission.DeductionINSS, Amount: 12.5,
	})
	require.NoError(t, err)
	assert.InDelta(t, 87.5, updated.NetAmount, 0.001)
	assert.Equal(t, int64(8750), repo.payoutCents)
	assert.Equal(t, 1, repo.syncUpdates)
	assert.Zero(t, repo.updates, "linked mutations must not fall back to a split write")

	updated, err = service.RemoveDeduction(context.Background(), comm.TenantID, comm.ID, "deduction-linked")
	require.NoError(t, err)
	assert.InDelta(t, 100.0, updated.NetAmount, 0.001)
	assert.Equal(t, int64(10000), repo.payoutCents)
	assert.Equal(t, 2, repo.syncUpdates)
}

func TestCommissionDeductionIDConflictDoesNotOverwriteExistingLine(t *testing.T) {
	comm := commission.New("deal-conflict", "broker-conflict", 100, "tenant-conflict")
	comm.Deductions = []commission.Deduction{{
		ID: "deduction-conflict", Kind: commission.DeductionISS, Amount: 5,
	}}
	comm.RecomputeNet()
	repo := &deductionConsistencyRepo{commission: comm}
	service := NewCommissionService(repo)

	_, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, commission.Deduction{
		ID: "deduction-conflict", Kind: commission.DeductionISS, Amount: 7,
	})

	require.ErrorIs(t, err, commission.ErrDeductionConflict)
	assert.Equal(t, 0, repo.updates)
	assert.Equal(t, 0, repo.syncUpdates)
	assert.InDelta(t, 95.0, repo.commission.NetAmount, 0.001)
}

type deductionRepoWithoutPayoutSync struct {
	CommissionRepository
	commission *commission.Commission
}

type serializedDeductionRepo struct {
	CommissionRepository
	mu          sync.Mutex
	commission  *commission.Commission
	payoutCents int64
}

func (r *serializedDeductionRepo) AddDeductionAtomic(_ context.Context, _, _ string, d commission.Deduction) (*commission.Commission, float64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previousNet := r.commission.NetAmount
	if existing, ok := r.commission.DeductionByID(d.ID); ok {
		if !sameDeductionData(existing, d) {
			return nil, previousNet, false, commission.ErrDeductionConflict
		}
		return cloneCommissionForTest(r.commission), previousNet, false, nil
	}
	if err := r.commission.AddDeduction(d); err != nil {
		return nil, previousNet, false, err
	}
	r.payoutCents = int64(r.commission.NetAmount*100 + 0.5)
	return cloneCommissionForTest(r.commission), previousNet, true, nil
}

func (r *serializedDeductionRepo) RemoveDeductionAtomic(_ context.Context, _, _, deductionID string) (*commission.Commission, float64, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	previousNet := r.commission.NetAmount
	changed, err := r.commission.RemoveDeductionByID(deductionID)
	if err != nil {
		return nil, previousNet, false, err
	}
	if changed {
		r.payoutCents = int64(r.commission.NetAmount*100 + 0.5)
	}
	return cloneCommissionForTest(r.commission), previousNet, changed, nil
}

func (r *deductionRepoWithoutPayoutSync) GetByID(_ context.Context, _, _ string) (*commission.Commission, error) {
	return cloneCommissionForTest(r.commission), nil
}

func TestCommissionDeductionRefusesUnsynchronizedLinkedPayout(t *testing.T) {
	comm := commission.New("deal-no-sync", "broker-no-sync", 100, "tenant-no-sync")
	comm.PayoutOrderID = "payout-no-sync"
	service := NewCommissionService(&deductionRepoWithoutPayoutSync{commission: comm})

	_, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, commission.Deduction{
		Kind: commission.DeductionOutros, Amount: 5,
	})

	require.ErrorIs(t, err, commission.ErrLinkedPayoutSyncUnsupported)
}

func TestCommissionConcurrentDistinctDeductionsPreserveBothAndPayoutNet(t *testing.T) {
	comm := commission.New("deal-serialized", "broker-serialized", 100, "tenant-serialized")
	comm.PayoutOrderID = "payout-serialized"
	repo := &serializedDeductionRepo{commission: comm, payoutCents: 10_000}
	service := NewCommissionService(repo)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	for _, deduction := range []commission.Deduction{
		{ID: "deduction-irrf", Kind: commission.DeductionIRRF, Amount: 10},
		{ID: "deduction-inss", Kind: commission.DeductionINSS, Amount: 5},
	} {
		deduction := deduction
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, deduction)
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.commission.Deductions, 2)
	assert.InDelta(t, 85.0, repo.commission.NetAmount, 0.001)
	assert.Equal(t, int64(8_500), repo.payoutCents)
}

func TestCommissionConcurrentAddAndRemoveDoNotLoseTheSurvivingDeduction(t *testing.T) {
	comm := commission.New("deal-add-remove", "broker-add-remove", 100, "tenant-add-remove")
	comm.PayoutOrderID = "payout-add-remove"
	require.NoError(t, comm.AddDeduction(commission.Deduction{ID: "deduction-old", Kind: commission.DeductionIRRF, Amount: 10}))
	repo := &serializedDeductionRepo{commission: comm, payoutCents: 9_000}
	service := NewCommissionService(repo)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		_, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, commission.Deduction{
			ID: "deduction-new", Kind: commission.DeductionINSS, Amount: 5,
		})
		errs <- err
	}()
	go func() {
		defer wg.Done()
		<-start
		_, err := service.RemoveDeduction(context.Background(), comm.TenantID, comm.ID, "deduction-old")
		errs <- err
	}()
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.commission.Deductions, 1)
	assert.Equal(t, "deduction-new", repo.commission.Deductions[0].ID)
	assert.InDelta(t, 95.0, repo.commission.NetAmount, 0.001)
	assert.Equal(t, int64(9_500), repo.payoutCents)
}

func TestCommissionDeductionRemovalRetryDoesNotDeleteNextLine(t *testing.T) {
	comm := commission.New("deal-remove-retry", "broker-remove-retry", 100, "tenant-remove-retry")
	require.NoError(t, comm.AddDeduction(commission.Deduction{ID: "deduction-a", Kind: commission.DeductionIRRF, Amount: 10}))
	require.NoError(t, comm.AddDeduction(commission.Deduction{ID: "deduction-b", Kind: commission.DeductionINSS, Amount: 5}))
	repo := &serializedDeductionRepo{commission: comm, payoutCents: 8_500}
	service := NewCommissionService(repo)

	first, err := service.RemoveDeduction(context.Background(), comm.TenantID, comm.ID, "deduction-a")
	require.NoError(t, err)
	retry, err := service.RemoveDeduction(context.Background(), comm.TenantID, comm.ID, "deduction-a")
	require.NoError(t, err)

	require.Len(t, first.Deductions, 1)
	require.Len(t, retry.Deductions, 1)
	assert.Equal(t, "deduction-b", retry.Deductions[0].ID)
	assert.InDelta(t, 95.0, retry.NetAmount, 0.001)
}

func TestCommissionDeductionFrozenOnceLifecycleLeavesPending(t *testing.T) {
	for _, status := range []commission.Status{
		commission.StatusApproved,
		commission.StatusPaid,
		commission.StatusCancelled,
		commission.StatusReversed,
	} {
		t.Run(string(status), func(t *testing.T) {
			comm := commission.New("deal-frozen", "broker-frozen", 100, "tenant-frozen")
			comm.Status = status
			repo := &deductionConsistencyRepo{commission: comm}
			service := NewCommissionService(repo)

			_, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, commission.Deduction{
				Kind: commission.DeductionIRRF, Amount: 10,
			})

			require.ErrorIs(t, err, commission.ErrDeductionFrozen)
			assert.Zero(t, repo.updates)
			assert.Zero(t, repo.syncUpdates)
		})
	}
}

func TestCommissionDeductionRemovalFrozenAfterApproval(t *testing.T) {
	comm := commission.New("deal-frozen-remove", "broker-frozen-remove", 100, "tenant-frozen-remove")
	require.NoError(t, comm.AddDeduction(commission.Deduction{ID: "deduction-frozen", Kind: commission.DeductionIRRF, Amount: 10}))
	comm.Status = commission.StatusApproved
	repo := &deductionConsistencyRepo{commission: comm}
	service := NewCommissionService(repo)

	_, err := service.RemoveDeduction(context.Background(), comm.TenantID, comm.ID, "deduction-frozen")

	require.ErrorIs(t, err, commission.ErrDeductionFrozen)
	assert.Zero(t, repo.updates)
	assert.Zero(t, repo.syncUpdates)
}

func TestCommissionDeductionFrozenAtomicPath(t *testing.T) {
	comm := commission.New("deal-frozen-atomic", "broker-frozen-atomic", 100, "tenant-frozen-atomic")
	comm.Status = commission.StatusApproved
	repo := &serializedDeductionRepo{commission: comm, payoutCents: 10_000}
	service := NewCommissionService(repo)

	_, err := service.AddDeduction(context.Background(), comm.TenantID, comm.ID, commission.Deduction{
		ID: "deduction-frozen-atomic", Kind: commission.DeductionINSS, Amount: 5,
	})

	require.ErrorIs(t, err, commission.ErrDeductionFrozen)
	require.Empty(t, repo.commission.Deductions)
	assert.InDelta(t, 100.0, repo.commission.NetAmount, 0.001)
}
