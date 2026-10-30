package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type commissionLifecycleRepo struct {
	CommissionRepository
	commission *commission.Commission
	forceErr   error
	casWrites  int
}

type commissionContractLookupStub struct {
	contract *clients.ContractDTO
	err      error
	calls    int
}

func (s *commissionContractLookupStub) GetContract(_ context.Context, _, _ string) (*clients.ContractDTO, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.contract, nil
}

func (r *commissionLifecycleRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	if r.commission == nil || r.commission.TenantID != tenantID || r.commission.ID != id {
		return nil, nil
	}
	return cloneCommissionForTest(r.commission), nil
}

func (r *commissionLifecycleRepo) Update(_ context.Context, updated *commission.Commission) error {
	r.commission = cloneCommissionForTest(updated)
	return nil
}

func (r *commissionLifecycleRepo) UpdateIfStatus(_ context.Context, updated *commission.Commission, expected commission.Status, expectedUpdatedAt time.Time) error {
	if r.forceErr != nil {
		return r.forceErr
	}
	if r.commission.Status != expected || !r.commission.UpdatedAt.Equal(expectedUpdatedAt) {
		return commission.ErrConcurrentTransition
	}
	r.casWrites++
	r.commission = cloneCommissionForTest(updated)
	return nil
}

func TestLinkedCommissionRejectsDirectLifecycleButAllowsOwningPayout(t *testing.T) {
	linked := commission.New("deal-linked-lifecycle", "broker-linked-lifecycle", 100, "tenant-linked-lifecycle")
	linked.PayoutOrderID = "payout-owner"
	repo := &commissionLifecycleRepo{commission: linked}
	service := NewCommissionService(repo)

	_, err := service.Approve(context.Background(), linked.TenantID, linked.ID, "approver-lifecycle")
	require.ErrorIs(t, err, commission.ErrLinkedPayoutLifecycleManaged)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)
	assert.Zero(t, repo.casWrites)

	_, err = service.ApproveFromPayout(context.Background(), linked.TenantID, linked.ID, "different-payout")
	require.ErrorIs(t, err, commission.ErrLinkedPayoutMismatch)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)

	approved, err := service.ApproveFromPayout(context.Background(), linked.TenantID, linked.ID, linked.PayoutOrderID)
	require.NoError(t, err)
	assert.Equal(t, commission.StatusApproved, approved.Status)
	assert.Equal(t, commission.StatusApproved, repo.commission.Status)
	assert.Equal(t, 1, repo.casWrites)
}

func TestStandaloneCommissionCASConflictDoesNotOverwriteNewerState(t *testing.T) {
	standalone := commission.New("deal-cas", "broker-cas", 100, "tenant-cas")
	repo := &commissionLifecycleRepo{commission: standalone, forceErr: commission.ErrConcurrentTransition}
	service := NewCommissionService(repo)

	_, err := service.Approve(context.Background(), standalone.TenantID, standalone.ID, "approver-lifecycle")

	require.ErrorIs(t, err, commission.ErrConcurrentTransition)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)
	assert.Zero(t, repo.casWrites)
}

func TestPaidCommissionCanAttachMissingPaymentEvidence(t *testing.T) {
	paid := commission.New("deal-proof", "broker-proof", 100, "tenant-proof")
	paid.Status = commission.StatusPaid
	paidAt := time.Now().Add(-time.Hour)
	paid.PaidAt = &paidAt
	repo := &commissionLifecycleRepo{commission: paid}
	service := NewCommissionService(repo)

	updated, err := service.MarkPaidWithProof(
		context.Background(),
		paid.TenantID,
		paid.ID,
		"https://files.example/commission-proof.pdf",
		"PIX conferido no extrato",
		"payer-lifecycle",
	)

	require.NoError(t, err)
	assert.Equal(t, commission.StatusPaid, updated.Status)
	assert.True(t, updated.PaidAt.Equal(paidAt))
	assert.Equal(t, "https://files.example/commission-proof.pdf", updated.ProofURL)
	assert.Equal(t, "PIX conferido no extrato", updated.Note)
	assert.Equal(t, 1, repo.casWrites)
}

func TestPaidCommissionEvidenceRetryIsNoOp(t *testing.T) {
	paid := commission.New("deal-proof-retry", "broker-proof-retry", 100, "tenant-proof-retry")
	paid.Status = commission.StatusPaid
	paid.ProofURL = "https://files.example/commission-proof.pdf"
	paid.Note = "PIX conferido no extrato"
	repo := &commissionLifecycleRepo{commission: paid}
	service := NewCommissionService(repo)

	updated, err := service.MarkPaidWithProof(
		context.Background(),
		paid.TenantID,
		paid.ID,
		paid.ProofURL,
		paid.Note,
		"payer-lifecycle",
	)

	require.NoError(t, err)
	assert.Equal(t, paid.ProofURL, updated.ProofURL)
	assert.Zero(t, repo.casWrites)
}

func TestLegacyRentalCommissionRequiresFundingReconciliationBeforeApprovalOrPayment(t *testing.T) {
	legacy := commission.NewWithRole(
		"deal-legacy-rent",
		"broker-legacy-rent",
		"contract-legacy-rent",
		"broker",
		100,
		3200,
		"tenant-legacy-rent",
	)
	repo := &commissionLifecycleRepo{commission: legacy}
	lookup := &commissionContractLookupStub{contract: &clients.ContractDTO{
		ID:       legacy.ContractID,
		TenantID: legacy.TenantID,
		Type:     "RENT",
	}}
	service := NewCommissionService(repo).WithContractLookup(lookup)

	_, err := service.Approve(context.Background(), legacy.TenantID, legacy.ID, "approver-lifecycle")
	require.ErrorIs(t, err, commission.ErrFundingGateConflict)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)
	assert.Zero(t, repo.casWrites)

	_, err = service.MarkPaidWithProof(
		context.Background(),
		legacy.TenantID,
		legacy.ID,
		"https://files.example/legacy-rent-proof.pdf",
		"tentativa sem conciliação",
		"payer-lifecycle",
	)
	require.ErrorIs(t, err, commission.ErrFundingGateConflict)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)
	assert.Zero(t, repo.casWrites)
	assert.Equal(t, 2, lookup.calls)
}

func TestLegacySaleCommissionKeepsExistingLifecycle(t *testing.T) {
	legacy := commission.NewWithRole(
		"deal-legacy-sale",
		"broker-legacy-sale",
		"contract-legacy-sale",
		"broker",
		100,
		3200,
		"tenant-legacy-sale",
	)
	repo := &commissionLifecycleRepo{commission: legacy}
	lookup := &commissionContractLookupStub{contract: &clients.ContractDTO{
		ID:       legacy.ContractID,
		TenantID: legacy.TenantID,
		Type:     "SALE",
	}}
	service := NewCommissionService(repo).WithContractLookup(lookup)

	approved, err := service.Approve(context.Background(), legacy.TenantID, legacy.ID, "approver-lifecycle")
	require.NoError(t, err)
	assert.Equal(t, commission.StatusApproved, approved.Status)
	assert.Equal(t, 1, repo.casWrites)
	assert.Equal(t, 1, lookup.calls)
}

func TestContractLookupFailureCannotPartiallyApproveLegacyCommission(t *testing.T) {
	legacy := commission.NewWithRole(
		"deal-lookup-failure",
		"broker-lookup-failure",
		"contract-lookup-failure",
		"broker",
		100,
		3200,
		"tenant-lookup-failure",
	)
	repo := &commissionLifecycleRepo{commission: legacy}
	lookupErr := errors.New("deal service unavailable")
	lookup := &commissionContractLookupStub{err: lookupErr}
	service := NewCommissionService(repo).WithContractLookup(lookup)

	_, err := service.Approve(context.Background(), legacy.TenantID, legacy.ID, "approver-lifecycle")
	require.ErrorIs(t, err, lookupErr)
	assert.Equal(t, commission.StatusPending, repo.commission.Status)
	assert.Zero(t, repo.casWrites)
}
