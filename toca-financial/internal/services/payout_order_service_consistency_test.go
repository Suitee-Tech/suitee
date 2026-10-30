package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payoutCASTestRepo struct {
	payout_order.Repository
	mu       sync.Mutex
	order    *payout_order.PayoutOrder
	forceErr error
	ready    chan struct{}
	release  chan struct{}

	mutateAmountBeforeCAS int64
	rollbackOnError       bool
	externalSnapshot      func() func()
	transitionRuns        int
	royaltyTransitionRuns int
}

func clonePayoutOrderForTest(order *payout_order.PayoutOrder) *payout_order.PayoutOrder {
	if order == nil {
		return nil
	}
	encoded, _ := json.Marshal(order)
	var cloned payout_order.PayoutOrder
	_ = json.Unmarshal(encoded, &cloned)
	return &cloned
}

func (r *payoutCASTestRepo) GetByID(_ context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	r.mu.Lock()
	var out *payout_order.PayoutOrder
	if r.order != nil && r.order.TenantID == tenantID && r.order.ID == id {
		out = clonePayoutOrderForTest(r.order)
	}
	ready, release := r.ready, r.release
	r.mu.Unlock()
	if ready != nil {
		ready <- struct{}{}
		<-release
	}
	return out, nil
}

func (r *payoutCASTestRepo) ListByIDs(_ context.Context, tenantID string, ids []string) ([]*payout_order.PayoutOrder, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.order == nil || r.order.TenantID != tenantID {
		return nil, nil
	}
	for _, id := range ids {
		if id == r.order.ID {
			return []*payout_order.PayoutOrder{clonePayoutOrderForTest(r.order)}, nil
		}
	}
	return nil, nil
}

func (r *payoutCASTestRepo) UpdateIfStatus(_ context.Context, order *payout_order.PayoutOrder, expected payout_order.Status, expectedUpdatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.forceErr != nil {
		return r.forceErr
	}
	if r.mutateAmountBeforeCAS > 0 {
		r.order.AmountCents = r.mutateAmountBeforeCAS
		r.order.UpdatedAt = r.order.UpdatedAt.Add(time.Microsecond)
		r.mutateAmountBeforeCAS = 0
	}
	if r.order == nil || r.order.Status != expected || !r.order.UpdatedAt.Equal(expectedUpdatedAt) {
		return payout_order.ErrConcurrentTransition
	}
	r.order = clonePayoutOrderForTest(order)
	return nil
}

func (r *payoutCASTestRepo) RunTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.mu.Lock()
	r.transitionRuns++
	r.mu.Unlock()
	return r.runTestTransitionTransaction(ctx, fn)
}

func (r *payoutCASTestRepo) RunRoyaltyTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	r.mu.Lock()
	r.royaltyTransitionRuns++
	r.mu.Unlock()
	return r.runTestTransitionTransaction(ctx, fn)
}

func (r *payoutCASTestRepo) runTestTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	if !r.rollbackOnError {
		return fn(ctx)
	}
	r.mu.Lock()
	before := clonePayoutOrderForTest(r.order)
	r.mu.Unlock()
	var rollbackExternal func()
	if r.externalSnapshot != nil {
		rollbackExternal = r.externalSnapshot()
	}
	if err := fn(ctx); err != nil {
		r.mu.Lock()
		r.order = before
		r.mu.Unlock()
		if rollbackExternal != nil {
			rollbackExternal()
		}
		return err
	}
	return nil
}

type payoutCascadeCommissionRepo struct {
	CommissionRepository
	listErr error
}

func (r *payoutCascadeCommissionRepo) ListByDealID(_ context.Context, _, _ string) ([]*commission.Commission, error) {
	return nil, r.listErr
}

type payoutAtomicCommissionRepo struct {
	CommissionRepository
	mu         sync.Mutex
	commission *commission.Commission
}

func (r *payoutAtomicCommissionRepo) ListByDealID(_ context.Context, tenantID, dealID string) ([]*commission.Commission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commission == nil || r.commission.TenantID != tenantID || r.commission.DealID != dealID {
		return nil, nil
	}
	return []*commission.Commission{cloneCommissionForTest(r.commission)}, nil
}

func (r *payoutAtomicCommissionRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commission == nil || r.commission.TenantID != tenantID || r.commission.ID != id {
		return nil, nil
	}
	return cloneCommissionForTest(r.commission), nil
}

func (r *payoutAtomicCommissionRepo) Update(_ context.Context, updated *commission.Commission) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commission = cloneCommissionForTest(updated)
	return nil
}

func (r *payoutAtomicCommissionRepo) UpdateIfStatus(_ context.Context, updated *commission.Commission, expected commission.Status, expectedUpdatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commission == nil || r.commission.Status != expected || !r.commission.UpdatedAt.Equal(expectedUpdatedAt) {
		return commission.ErrConcurrentTransition
	}
	r.commission = cloneCommissionForTest(updated)
	return nil
}

func (r *payoutAtomicCommissionRepo) snapshot() func() {
	r.mu.Lock()
	before := cloneCommissionForTest(r.commission)
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		r.commission = before
		r.mu.Unlock()
	}
}

// fullStackPayoutRepo predates repository CAS and is local to
// financial_parity_test.go. Giving that in-memory test double the transition
// method keeps the existing lifecycle fixture focused on parity; production
// safety is exercised against payoutCASTestRepo and the Postgres repository.
func (m *fullStackPayoutRepo) UpdateIfStatus(ctx context.Context, order *payout_order.PayoutOrder, _ payout_order.Status, _ time.Time) error {
	return m.Update(ctx, order)
}

func (m *fullStackPayoutRepo) RunTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func newPayoutForTransitionTest(t *testing.T, status payout_order.Status, recipient payout_order.RecipientType) *payout_order.PayoutOrder {
	t.Helper()
	order, err := payout_order.New(
		"tenant-cas", payout_order.SourceManual, "contract-cas", recipient,
		"recipient-cas", "Recipient", 10_000, "requester",
	)
	require.NoError(t, err)
	order.Status = status
	order.SourceRef = map[string]any{"deal_id": "deal-cas", "trigger": "contract.signed"}
	return order
}

func TestPayoutApproveCancelRaceOnlyOneTransitionWins(t *testing.T) {
	order := newPayoutForTransitionTest(t, payout_order.StatusPendingReview, payout_order.RecipientExternal)
	repo := &payoutCASTestRepo{
		order:   order,
		ready:   make(chan struct{}, 2),
		release: make(chan struct{}),
	}
	service := NewPayoutOrderService(repo)
	errs := make(chan error, 2)

	go func() {
		_, err := service.Approve(context.Background(), order.TenantID, order.ID, "approver")
		errs <- err
	}()
	go func() {
		_, err := service.Cancel(context.Background(), order.TenantID, order.ID, "canceller", "race")
		errs <- err
	}()

	<-repo.ready
	<-repo.ready
	close(repo.release)
	firstErr, secondErr := <-errs, <-errs

	oneSucceeded := (firstErr == nil) != (secondErr == nil)
	assert.True(t, oneSucceeded, "exactly one competing transition must commit")
	if firstErr != nil {
		assert.ErrorIs(t, firstErr, payout_order.ErrConcurrentTransition)
	}
	if secondErr != nil {
		assert.ErrorIs(t, secondErr, payout_order.ErrConcurrentTransition)
	}
	repo.mu.Lock()
	finalStatus := repo.order.Status
	repo.mu.Unlock()
	assert.Contains(t, []payout_order.Status{payout_order.StatusApproved, payout_order.StatusCancelled}, finalStatus)
}

func TestPayoutGroupRejectsConcurrentAmountChange(t *testing.T) {
	order := newPayoutForTransitionTest(t, payout_order.StatusPendingReview, payout_order.RecipientExternal)
	repo := &payoutCASTestRepo{
		order:                 order,
		mutateAmountBeforeCAS: 9_000,
	}
	service := NewPayoutOrderService(repo)

	_, _, err := service.GroupWithID(context.Background(), order.TenantID, []string{order.ID}, "operator", "group-1")

	require.ErrorIs(t, err, payout_order.ErrConcurrentTransition)
	repo.mu.Lock()
	defer repo.mu.Unlock()
	assert.Equal(t, int64(9_000), repo.order.AmountCents)
	assert.Empty(t, repo.order.GroupID)
	assert.Equal(t, payout_order.StatusPendingReview, repo.order.Status)
}

func TestPayoutTransitionsRejectCompareAndSetConflict(t *testing.T) {
	tests := []struct {
		name   string
		status payout_order.Status
		run    func(*PayoutOrderService, *payout_order.PayoutOrder) error
	}{
		{
			name: "approve", status: payout_order.StatusPendingReview,
			run: func(s *PayoutOrderService, o *payout_order.PayoutOrder) error {
				_, err := s.Approve(context.Background(), o.TenantID, o.ID, "actor")
				return err
			},
		},
		{
			name: "pay", status: payout_order.StatusApproved,
			run: func(s *PayoutOrderService, o *payout_order.PayoutOrder) error {
				_, err := s.MarkPaid(context.Background(), o.TenantID, o.ID, "actor", MarkPaidInput{
					Method: string(payout_order.SettlementManualPIX), ProofURL: "https://proof",
				})
				return err
			},
		},
		{
			name: "cancel", status: payout_order.StatusPendingReview,
			run: func(s *PayoutOrderService, o *payout_order.PayoutOrder) error {
				_, err := s.Cancel(context.Background(), o.TenantID, o.ID, "actor", "reason")
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			order := newPayoutForTransitionTest(t, tc.status, payout_order.RecipientExternal)
			repo := &payoutCASTestRepo{order: order, forceErr: payout_order.ErrConcurrentTransition}

			err := tc.run(NewPayoutOrderService(repo), order)

			require.ErrorIs(t, err, payout_order.ErrConcurrentTransition)
			assert.Equal(t, tc.status, repo.order.Status)
		})
	}
}

func TestPayoutTransitionRejectsSameStatusMonetaryRevision(t *testing.T) {
	order := newPayoutForTransitionTest(t, payout_order.StatusPendingReview, payout_order.RecipientExternal)
	repo := &payoutCASTestRepo{
		order:                 order,
		mutateAmountBeforeCAS: 8_000,
	}

	_, err := NewPayoutOrderService(repo).Approve(context.Background(), order.TenantID, order.ID, "actor")

	require.ErrorIs(t, err, payout_order.ErrConcurrentTransition)
	assert.Equal(t, payout_order.StatusPendingReview, repo.order.Status)
	assert.EqualValues(t, 8_000, repo.order.AmountCents, "the newer net amount must not be overwritten")
}

func TestPayoutTransitionsPropagateCommissionCascadeFailure(t *testing.T) {
	cascadeErr := errors.New("commission repository unavailable")
	tests := []struct {
		name   string
		status payout_order.Status
		run    func(*PayoutOrderService, *payout_order.PayoutOrder) error
	}{
		{
			name: "approve", status: payout_order.StatusPendingReview,
			run: func(s *PayoutOrderService, o *payout_order.PayoutOrder) error {
				_, err := s.Approve(context.Background(), o.TenantID, o.ID, "actor")
				return err
			},
		},
		{
			name: "pay", status: payout_order.StatusApproved,
			run: func(s *PayoutOrderService, o *payout_order.PayoutOrder) error {
				_, err := s.MarkPaid(context.Background(), o.TenantID, o.ID, "actor", MarkPaidInput{
					Method: string(payout_order.SettlementManualPIX), ProofURL: "https://proof",
				})
				return err
			},
		},
		{
			name: "cancel", status: payout_order.StatusPendingReview,
			run: func(s *PayoutOrderService, o *payout_order.PayoutOrder) error {
				_, err := s.Cancel(context.Background(), o.TenantID, o.ID, "actor", "reason")
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			order := newPayoutForTransitionTest(t, tc.status, payout_order.RecipientBroker)
			repo := &payoutCASTestRepo{order: order, rollbackOnError: true}
			service := NewPayoutOrderService(repo)
			service.SetCommissionService(NewCommissionService(&payoutCascadeCommissionRepo{listErr: cascadeErr}))

			err := tc.run(service, order)

			require.ErrorIs(t, err, cascadeErr)
			assert.Equal(t, tc.status, repo.order.Status, "payout transition must roll back with its cascade")
		})
	}
}

func TestPayoutCASFailureRollsBackSuccessfulCommissionCascade(t *testing.T) {
	order := newPayoutForTransitionTest(t, payout_order.StatusPendingReview, payout_order.RecipientBroker)
	linkedCommission := commission.New(order.SourceRef["deal_id"].(string), order.RecipientID, 100, order.TenantID)
	linkedCommission.PayoutOrderID = order.ID
	commissionRepo := &payoutAtomicCommissionRepo{commission: linkedCommission}
	payoutRepo := &payoutCASTestRepo{
		order:            order,
		forceErr:         payout_order.ErrConcurrentTransition,
		rollbackOnError:  true,
		externalSnapshot: commissionRepo.snapshot,
	}
	service := NewPayoutOrderService(payoutRepo)
	service.SetCommissionService(NewCommissionService(commissionRepo))

	_, err := service.Approve(context.Background(), order.TenantID, order.ID, "actor")

	require.ErrorIs(t, err, payout_order.ErrConcurrentTransition)
	assert.Equal(t, payout_order.StatusPendingReview, payoutRepo.order.Status)
	assert.Equal(t, commission.StatusPending, commissionRepo.commission.Status,
		"commission must roll back when the payout CAS loses the race")
}

func TestBrokerPayoutTransitionRequiresLinkedCommissionCascade(t *testing.T) {
	tests := []struct {
		name       string
		commission *CommissionService
		wantError  string
	}{
		{name: "missing service", wantError: "commission service is required"},
		{
			name:       "missing link",
			commission: NewCommissionService(&payoutCascadeCommissionRepo{}),
			wantError:  "linked commission not found",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			order := newPayoutForTransitionTest(t, payout_order.StatusPendingReview, payout_order.RecipientBroker)
			repo := &payoutCASTestRepo{order: order, rollbackOnError: true}
			service := NewPayoutOrderService(repo)
			service.SetCommissionService(tc.commission)

			_, err := service.Approve(context.Background(), order.TenantID, order.ID, "actor")

			require.ErrorContains(t, err, tc.wantError)
			assert.Equal(t, payout_order.StatusPendingReview, repo.order.Status)
		})
	}
}
