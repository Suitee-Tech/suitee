package services

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/partner_royalty"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type royaltyLifecycleRepo struct {
	PartnerRoyaltyRepository
	mu      sync.Mutex
	royalty *partner_royalty.PartnerRoyalty
}

func cloneRoyaltyForTest(royalty *partner_royalty.PartnerRoyalty) *partner_royalty.PartnerRoyalty {
	if royalty == nil {
		return nil
	}
	encoded, _ := json.Marshal(royalty)
	var cloned partner_royalty.PartnerRoyalty
	_ = json.Unmarshal(encoded, &cloned)
	return &cloned
}

func (r *royaltyLifecycleRepo) GetByID(_ context.Context, tenantID, id string) (*partner_royalty.PartnerRoyalty, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.royalty == nil || r.royalty.TenantID != tenantID || r.royalty.ID != id {
		return nil, nil
	}
	return cloneRoyaltyForTest(r.royalty), nil
}

func (r *royaltyLifecycleRepo) UpdateIfStatus(_ context.Context, updated *partner_royalty.PartnerRoyalty, expected partner_royalty.Status, expectedUpdatedAt time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.royalty == nil || r.royalty.Status != expected || !r.royalty.UpdatedAt.Equal(expectedUpdatedAt) {
		return partner_royalty.ErrConcurrentTransition
	}
	r.royalty = cloneRoyaltyForTest(updated)
	return nil
}

func (r *royaltyLifecycleRepo) snapshot() func() {
	r.mu.Lock()
	before := cloneRoyaltyForTest(r.royalty)
	r.mu.Unlock()
	return func() {
		r.mu.Lock()
		r.royalty = before
		r.mu.Unlock()
	}
}

func newRoyaltyPayoutTest(t *testing.T) (*partner_royalty.PartnerRoyalty, *payout_order.PayoutOrder) {
	t.Helper()
	royalty := partner_royalty.New("partnership-royalty", "deal-royalty", "contract-royalty", "payer-tenant", "payee-tenant", 1_000, 10)
	order, err := payout_order.New(
		royalty.PayerTenant, payout_order.SourceRoyaltySettlement, royalty.ID,
		payout_order.RecipientRoyaltyPayee, royalty.PayeeTenant, "Royalty", 10_000, "system",
	)
	require.NoError(t, err)
	order.ContractID = royalty.ContractID
	order.SourceRef = map[string]any{"royalty_id": royalty.ID, "deal_id": royalty.DealID}
	return royalty, order
}

func TestRoyaltyAndPayoutApprovalCommitTogetherByRoyaltyID(t *testing.T) {
	royalty, order := newRoyaltyPayoutTest(t)
	royaltyRepo := &royaltyLifecycleRepo{royalty: royalty}
	payoutRepo := &payoutCASTestRepo{order: order, rollbackOnError: true, externalSnapshot: royaltyRepo.snapshot}
	service := NewPayoutOrderService(payoutRepo)
	service.SetPartnerRoyaltyService(NewPartnerRoyaltyService(royaltyRepo))

	approved, err := service.Approve(context.Background(), order.TenantID, order.ID, "operator")

	require.NoError(t, err)
	assert.Equal(t, payout_order.StatusApproved, approved.Status)
	assert.Equal(t, partner_royalty.StatusApproved, royaltyRepo.royalty.Status)
	assert.Equal(t, order.ID, royaltyRepo.royalty.PayoutOrderID)
	assert.Equal(t, 1, payoutRepo.royaltyTransitionRuns)
	assert.Zero(t, payoutRepo.transitionRuns, "royalty must never use the tenant-scoped transaction runner")
}

type tenantOnlyRoyaltyPayoutRepo struct {
	payout_order.Repository
	inner *payoutCASTestRepo
}

func (r *tenantOnlyRoyaltyPayoutRepo) GetByID(ctx context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	return r.inner.GetByID(ctx, tenantID, id)
}

func (r *tenantOnlyRoyaltyPayoutRepo) UpdateIfStatus(ctx context.Context, order *payout_order.PayoutOrder, expected payout_order.Status, expectedUpdatedAt time.Time) error {
	return r.inner.UpdateIfStatus(ctx, order, expected, expectedUpdatedAt)
}

func (r *tenantOnlyRoyaltyPayoutRepo) RunTransitionTransaction(ctx context.Context, fn func(context.Context) error) error {
	return r.inner.RunTransitionTransaction(ctx, fn)
}

func TestRoyaltyPayoutRequiresCrossTenantServiceRunner(t *testing.T) {
	royalty, order := newRoyaltyPayoutTest(t)
	royaltyRepo := &royaltyLifecycleRepo{royalty: royalty}
	inner := &payoutCASTestRepo{order: order, rollbackOnError: true, externalSnapshot: royaltyRepo.snapshot}
	service := NewPayoutOrderService(&tenantOnlyRoyaltyPayoutRepo{inner: inner})
	service.SetPartnerRoyaltyService(NewPartnerRoyaltyService(royaltyRepo))

	_, err := service.Approve(context.Background(), order.TenantID, order.ID, "operator")

	require.ErrorIs(t, err, payout_order.ErrRoyaltyServiceTransactionMissing)
	assert.Equal(t, payout_order.StatusPendingReview, inner.order.Status)
	assert.Equal(t, partner_royalty.StatusPending, royaltyRepo.royalty.Status)
	assert.Zero(t, inner.transitionRuns)
}

func TestRoyaltyAndPayoutTerminalTransitionsStayAligned(t *testing.T) {
	tests := []struct {
		name          string
		payoutStatus  payout_order.Status
		royaltyStatus partner_royalty.Status
		wantPayout    payout_order.Status
		wantRoyalty   partner_royalty.Status
		run           func(*PayoutOrderService, *payout_order.PayoutOrder) error
	}{
		{
			name: "paid", payoutStatus: payout_order.StatusApproved, royaltyStatus: partner_royalty.StatusApproved,
			wantPayout: payout_order.StatusPaid, wantRoyalty: partner_royalty.StatusPaid,
			run: func(service *PayoutOrderService, order *payout_order.PayoutOrder) error {
				_, err := service.MarkPaid(context.Background(), order.TenantID, order.ID, "operator", MarkPaidInput{
					Method: string(payout_order.SettlementManualPIX), ProofURL: "https://proof.example/royalty",
				})
				return err
			},
		},
		{
			name: "cancelled", payoutStatus: payout_order.StatusPendingReview, royaltyStatus: partner_royalty.StatusPending,
			wantPayout: payout_order.StatusCancelled, wantRoyalty: partner_royalty.StatusCancelled,
			run: func(service *PayoutOrderService, order *payout_order.PayoutOrder) error {
				_, err := service.Cancel(context.Background(), order.TenantID, order.ID, "operator", "cancelled")
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			royalty, order := newRoyaltyPayoutTest(t)
			order.Status = tc.payoutStatus
			royalty.Status = tc.royaltyStatus
			royalty.PayoutOrderID = order.ID
			royaltyRepo := &royaltyLifecycleRepo{royalty: royalty}
			payoutRepo := &payoutCASTestRepo{order: order, rollbackOnError: true, externalSnapshot: royaltyRepo.snapshot}
			service := NewPayoutOrderService(payoutRepo)
			service.SetPartnerRoyaltyService(NewPartnerRoyaltyService(royaltyRepo))

			require.NoError(t, tc.run(service, order))
			assert.Equal(t, tc.wantPayout, payoutRepo.order.Status)
			assert.Equal(t, tc.wantRoyalty, royaltyRepo.royalty.Status)
		})
	}
}

func TestRoyaltyCascadeRollsBackWhenPayoutCASLosesRace(t *testing.T) {
	royalty, order := newRoyaltyPayoutTest(t)
	royaltyRepo := &royaltyLifecycleRepo{royalty: royalty}
	payoutRepo := &payoutCASTestRepo{
		order: order, forceErr: payout_order.ErrConcurrentTransition,
		rollbackOnError: true, externalSnapshot: royaltyRepo.snapshot,
	}
	service := NewPayoutOrderService(payoutRepo)
	service.SetPartnerRoyaltyService(NewPartnerRoyaltyService(royaltyRepo))

	_, err := service.Approve(context.Background(), order.TenantID, order.ID, "operator")

	require.ErrorIs(t, err, payout_order.ErrConcurrentTransition)
	assert.Equal(t, payout_order.StatusPendingReview, payoutRepo.order.Status)
	assert.Equal(t, partner_royalty.StatusPending, royaltyRepo.royalty.Status)
	assert.Empty(t, royaltyRepo.royalty.PayoutOrderID)
}

func TestRoyaltyPayoutRejectsMismatchedRoyaltyIdentity(t *testing.T) {
	royalty, order := newRoyaltyPayoutTest(t)
	order.SourceRef["royalty_id"] = "different-royalty"
	royaltyRepo := &royaltyLifecycleRepo{royalty: royalty}
	payoutRepo := &payoutCASTestRepo{order: order, rollbackOnError: true, externalSnapshot: royaltyRepo.snapshot}
	service := NewPayoutOrderService(payoutRepo)
	service.SetPartnerRoyaltyService(NewPartnerRoyaltyService(royaltyRepo))

	_, err := service.Approve(context.Background(), order.TenantID, order.ID, "operator")

	require.ErrorContains(t, err, "invalid royalty identity")
	assert.Equal(t, payout_order.StatusPendingReview, payoutRepo.order.Status)
	assert.Equal(t, partner_royalty.StatusPending, royaltyRepo.royalty.Status)
}
