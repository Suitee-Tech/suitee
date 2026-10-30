package services_test

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/partner_royalty"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCrossRepo struct {
	ownerTenant    string
	ownerLookupErr error
	partnership    *repository.PartnershipFinancials
	partnershipErr error
}

func (f *fakeCrossRepo) PropertyOwnerTenant(_ context.Context, _ string) (string, error) {
	return f.ownerTenant, f.ownerLookupErr
}

func (f *fakeCrossRepo) ActivePartnershipBetween(_ context.Context, _, _ string) (*repository.PartnershipFinancials, error) {
	return f.partnership, f.partnershipErr
}

type mockPartnerRoyaltyRepo struct {
	created     []*partner_royalty.PartnerRoyalty
	createErr   error
	createCalls int
}

type crossCompanyTxRepo struct {
	transaction.RepositoryInterface
	created     []*transaction.Transaction
	createErr   error
	createCalls int
}

func (r *crossCompanyTxRepo) Create(_ context.Context, tx *transaction.Transaction) error {
	r.createCalls++
	if r.createErr != nil {
		return r.createErr
	}
	r.created = append(r.created, tx)
	return nil
}

func (m *mockPartnerRoyaltyRepo) Create(_ context.Context, pr *partner_royalty.PartnerRoyalty) error {
	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}
	m.created = append(m.created, pr)
	return nil
}
func (m *mockPartnerRoyaltyRepo) GetByID(_ context.Context, _, _ string) (*partner_royalty.PartnerRoyalty, error) {
	return nil, nil
}
func (m *mockPartnerRoyaltyRepo) GetBySourceEventID(_ context.Context, _, _ string) (*partner_royalty.PartnerRoyalty, error) {
	return nil, nil
}
func (m *mockPartnerRoyaltyRepo) List(_ context.Context, _ string, _, _ int64) ([]*partner_royalty.PartnerRoyalty, int64, error) {
	return nil, 0, nil
}

func newCrossCompanySaleInput() service.ContractFinancialsInput {
	return service.ContractFinancialsInput{
		EventID: "event-cc",
		Payload: &contracts.ContractSignedEventPayload{
			DealID:        "deal-cc",
			ContractID:    "cont-cc",
			TenantID:      "tenant-closer",
			PropertyID:    "prop-cc",
			Value:         2000.0,
			CommissionPct: 10.0, // commission amount = 200
			Type:          "SALE",
			CustomerID:    "cust-cc",
			OwnerID:       "owner-cc",
		},
		CrossCompany: &service.CrossCompanySettlement{},
	}
}

// Happy path: franchise partnership active, owner share + royalty land.
func TestCrossCompanyHandler_FranchiseSplitsAndRoyalty(t *testing.T) {
	txRepo := &crossCompanyTxRepo{}
	royaltyRepo := &mockPartnerRoyaltyRepo{}
	txService := service.NewTransactionService(txRepo)
	royaltyService := service.NewPartnerRoyaltyService(royaltyRepo)

	cross := &fakeCrossRepo{
		ownerTenant: "tenant-owner",
		partnership: &repository.PartnershipFinancials{
			ID:                "pf-1",
			Type:              "franchise",
			Status:            "active",
			RoyaltyPercentage: 10,
			RoyaltyBase:       "gross_commission",
			OwnerCompanyPct:   30,
			ClosingCompanyPct: 70,
			RequesterTenant:   "tenant-owner",
			TargetTenant:      "tenant-closer",
		},
	}

	h := service.NewCrossCompanyHandler(cross, txService, nil, royaltyService)
	in := newCrossCompanySaleInput()
	err := h.Handle(context.Background(), in)
	assert.NoError(t, err)

	assert.Len(t, txRepo.created, 1, "owner-company share row should be the only ledger write here")
	ownerTx := txRepo.created[0]
	assert.Equal(t, "tenant-owner", ownerTx.TenantID)
	assert.Equal(t, "Comissão Parceria", ownerTx.Category)
	assert.InDelta(t, 60.0, ownerTx.Value, 0.001) // 30% of 200

	assert.Len(t, royaltyRepo.created, 1)
	r := royaltyRepo.created[0]
	assert.Equal(t, "pf-1", r.PartnershipID)
	assert.Equal(t, "tenant-closer", r.PayerTenant)
	assert.Equal(t, "tenant-owner", r.PayeeTenant)
	assert.InDelta(t, 200.0, r.BaseAmount, 0.001)
	assert.InDelta(t, 10.0, r.Percentage, 0.001)

	assert.InDelta(t, 7.0, in.Payload.CommissionPct, 0.001)
	assert.Equal(t, "pf-1", in.CrossCompany.PartnershipID)
	assert.Equal(t, "tenant-owner", in.CrossCompany.RecipientTenant)
	assert.Equal(t, int64(6000), in.CrossCompany.OwnerAmountCents)
	assert.InDelta(t, 30.0, in.CrossCompany.OwnerPercentage, 0.001)
	assert.Equal(t, int64(2000), in.CrossCompany.RoyaltyAmountCents)
	assert.Equal(t, "tenant-owner", in.CrossCompany.RoyaltyPayeeTenant)
}

func TestCrossCompanyHandler_SplitConservesOwnerAndBrokerCommission(t *testing.T) {
	txRepo := &crossCompanyTxRepo{}
	txService := service.NewTransactionService(txRepo)
	cross := &fakeCrossRepo{
		ownerTenant: "tenant-owner",
		partnership: &repository.PartnershipFinancials{
			ID: "pf-conservation", Type: "partner", OwnerCompanyPct: 30, ClosingCompanyPct: 70,
		},
	}
	chain := service.NewChainedContractHandler(
		service.NewChargeIssuanceHandler(nil, nil, nil, nil),
		service.NewOwnerTransferHandler(nil, nil),
		service.NewBrokerCommissionHandler(txService, nil, nil),
		service.NewCrossCompanyHandler(cross, txService, nil, nil),
	)
	in := newCrossCompanySaleInput()
	in.Payload.BrokerParticipations = []contracts.BrokerParticipation{{
		BrokerID: "broker-closing", Role: "fechamento", Percentage: 100,
	}}

	require.NoError(t, chain.Handle(context.Background(), in))
	require.Len(t, txRepo.created, 2)

	var ownerAmount, brokerAmount float64
	for _, tx := range txRepo.created {
		switch tx.TenantID {
		case "tenant-owner":
			ownerAmount += tx.Value
		case "tenant-closer":
			brokerAmount += tx.Value
		}
	}
	assert.InDelta(t, 60.0, ownerAmount, 0.001)
	assert.InDelta(t, 140.0, brokerAmount, 0.001)
	assert.InDelta(t, 200.0, ownerAmount+brokerAmount, 0.001, "split must conserve the original commission base")
}

// Single-company deal: owner tenant equals deal tenant — no writes.
func TestCrossCompanyHandler_SingleCompanyIsNoOp(t *testing.T) {
	txRepo := &crossCompanyTxRepo{}
	txService := service.NewTransactionService(txRepo)

	cross := &fakeCrossRepo{ownerTenant: "tenant-closer"}
	h := service.NewCrossCompanyHandler(cross, txService, nil, nil)

	err := h.Handle(context.Background(), newCrossCompanySaleInput())
	assert.NoError(t, err)
	assert.Empty(t, txRepo.created)
}

// No partnership in place: cross-company deal, but the closing company
// keeps the full commission — no writes.
func TestCrossCompanyHandler_NoActivePartnership(t *testing.T) {
	txRepo := &crossCompanyTxRepo{}
	txService := service.NewTransactionService(txRepo)

	cross := &fakeCrossRepo{ownerTenant: "tenant-owner", partnership: nil}
	h := service.NewCrossCompanyHandler(cross, txService, nil, nil)

	err := h.Handle(context.Background(), newCrossCompanySaleInput())
	assert.NoError(t, err)
	assert.Empty(t, txRepo.created)
}

// Owner-tenant lookup errors are fail-fast because otherwise the broker step
// would provision an unknown cross-company deal at the full percentage.
func TestCrossCompanyHandler_OwnerLookupError_IsFailFast(t *testing.T) {
	lookupErr := errors.New("boom")
	cross := &fakeCrossRepo{ownerLookupErr: lookupErr}
	h := service.NewCrossCompanyHandler(cross, nil, nil, nil)

	err := h.Handle(context.Background(), newCrossCompanySaleInput())

	require.ErrorIs(t, err, lookupErr)
}

// Partnership lookup errors are also fail-fast: a missing answer must not be
// interpreted as permission to provision the closing company at 100%.
func TestCrossCompanyHandler_PartnershipLookupError_IsFailFast(t *testing.T) {
	lookupErr := errors.New("partnership down")
	cross := &fakeCrossRepo{ownerTenant: "tenant-owner", partnershipErr: lookupErr}
	h := service.NewCrossCompanyHandler(cross, nil, nil, nil)

	err := h.Handle(context.Background(), newCrossCompanySaleInput())

	require.ErrorIs(t, err, lookupErr)
}

func TestCrossCompanyHandler_RoyaltyFailureLeavesNoOwnerOrSettlementProvisioning(t *testing.T) {
	royaltyErr := errors.New("royalty repository unavailable")
	txRepo := &crossCompanyTxRepo{}
	royaltyRepo := &mockPartnerRoyaltyRepo{createErr: royaltyErr}
	cross := &fakeCrossRepo{
		ownerTenant: "tenant-owner",
		partnership: &repository.PartnershipFinancials{
			ID:                "pf-failure",
			Type:              "franchise",
			RoyaltyPercentage: 10,
			RoyaltyBase:       "gross_commission",
			OwnerCompanyPct:   30,
			ClosingCompanyPct: 70,
			RequesterTenant:   "tenant-owner",
			TargetTenant:      "tenant-closer",
		},
	}
	in := newCrossCompanySaleInput()
	h := service.NewCrossCompanyHandler(
		cross,
		service.NewTransactionService(txRepo),
		nil,
		service.NewPartnerRoyaltyService(royaltyRepo),
	)

	err := h.Handle(context.Background(), in)

	require.ErrorIs(t, err, royaltyErr)
	assert.Equal(t, 1, royaltyRepo.createCalls)
	assert.Empty(t, txRepo.created, "owner share must not run after a royalty failure")
	assert.InDelta(t, 10.0, in.Payload.CommissionPct, 0.001, "broker base must remain untouched on failure")
	assert.Empty(t, *in.CrossCompany, "failed monetary writes must not become payout-visible")
}

func TestCrossCompanyHandler_RoyaltyFailureAbortsChainBeforeBrokerProvisioning(t *testing.T) {
	royaltyErr := errors.New("royalty repository unavailable")
	txRepo := &crossCompanyTxRepo{}
	royaltyRepo := &mockPartnerRoyaltyRepo{createErr: royaltyErr}
	cross := &fakeCrossRepo{
		ownerTenant: "tenant-owner",
		partnership: &repository.PartnershipFinancials{
			ID:                "pf-chain-failure",
			Type:              "franchise",
			RoyaltyPercentage: 10,
			RoyaltyBase:       "gross_commission",
			OwnerCompanyPct:   30,
			ClosingCompanyPct: 70,
			RequesterTenant:   "tenant-owner",
			TargetTenant:      "tenant-closer",
		},
	}
	txService := service.NewTransactionService(txRepo)
	chain := service.NewChainedContractHandler(
		service.NewChargeIssuanceHandler(nil, nil, nil, nil),
		service.NewOwnerTransferHandler(nil, nil),
		service.NewBrokerCommissionHandler(txService, nil, nil),
		service.NewCrossCompanyHandler(cross, txService, nil, service.NewPartnerRoyaltyService(royaltyRepo)),
	)
	in := newCrossCompanySaleInput()
	in.Payload.BrokerParticipations = []contracts.BrokerParticipation{{
		BrokerID: "broker-cc", Role: "captador", Percentage: 100,
	}}

	err := chain.Handle(context.Background(), in)

	require.ErrorIs(t, err, royaltyErr)
	assert.Empty(t, txRepo.created, "neither owner nor broker ledger legs may be provisioned")
	assert.Empty(t, *in.CrossCompany)
}

func TestCrossCompanyHandler_OwnerFailureAbortsChainBeforeBrokerProvisioning(t *testing.T) {
	ownerErr := errors.New("owner ledger unavailable")
	txRepo := &crossCompanyTxRepo{createErr: ownerErr}
	cross := &fakeCrossRepo{
		ownerTenant: "tenant-owner",
		partnership: &repository.PartnershipFinancials{
			ID: "pf-owner-failure", Type: "partner", OwnerCompanyPct: 30, ClosingCompanyPct: 70,
		},
	}
	txService := service.NewTransactionService(txRepo)
	chain := service.NewChainedContractHandler(
		service.NewChargeIssuanceHandler(nil, nil, nil, nil),
		service.NewOwnerTransferHandler(nil, nil),
		service.NewBrokerCommissionHandler(txService, nil, nil),
		service.NewCrossCompanyHandler(cross, txService, nil, nil),
	)
	in := newCrossCompanySaleInput()
	in.Payload.BrokerParticipations = []contracts.BrokerParticipation{{
		BrokerID: "broker-cc", Role: "captador", Percentage: 100,
	}}

	err := chain.Handle(context.Background(), in)

	require.ErrorIs(t, err, ownerErr)
	assert.Equal(t, 1, txRepo.createCalls, "only the failed owner write may be attempted; broker must not run")
	assert.InDelta(t, 10.0, in.Payload.CommissionPct, 0.001)
	assert.Empty(t, *in.CrossCompany)
}

func TestCrossCompanyHandler_RequiredDependenciesFailBeforeMonetaryWrites(t *testing.T) {
	t.Run("owner share requires transaction service", func(t *testing.T) {
		cross := &fakeCrossRepo{
			ownerTenant: "tenant-owner",
			partnership: &repository.PartnershipFinancials{
				ID: "pf-owner", Type: "partner", OwnerCompanyPct: 30, ClosingCompanyPct: 70,
			},
		}
		in := newCrossCompanySaleInput()
		h := service.NewCrossCompanyHandler(cross, nil, nil, nil)

		err := h.Handle(context.Background(), in)

		require.ErrorContains(t, err, "transaction service is required")
		assert.InDelta(t, 10.0, in.Payload.CommissionPct, 0.001)
		assert.Empty(t, *in.CrossCompany)
	})

	t.Run("owner share requires settlement carrier", func(t *testing.T) {
		txRepo := &crossCompanyTxRepo{}
		cross := &fakeCrossRepo{
			ownerTenant: "tenant-owner",
			partnership: &repository.PartnershipFinancials{
				ID: "pf-settlement", Type: "partner", OwnerCompanyPct: 30, ClosingCompanyPct: 70,
			},
		}
		in := newCrossCompanySaleInput()
		in.CrossCompany = nil
		h := service.NewCrossCompanyHandler(cross, service.NewTransactionService(txRepo), nil, nil)

		err := h.Handle(context.Background(), in)

		require.ErrorContains(t, err, "settlement carrier is required")
		assert.Empty(t, txRepo.created)
		assert.InDelta(t, 10.0, in.Payload.CommissionPct, 0.001)
	})

	t.Run("royalty requires partner royalty service", func(t *testing.T) {
		txRepo := &crossCompanyTxRepo{}
		cross := &fakeCrossRepo{
			ownerTenant: "tenant-owner",
			partnership: &repository.PartnershipFinancials{
				ID: "pf-royalty", Type: "franchise", RoyaltyPercentage: 10,
				RoyaltyBase: "gross_commission", OwnerCompanyPct: 30, ClosingCompanyPct: 70,
				RequesterTenant: "tenant-owner", TargetTenant: "tenant-closer",
			},
		}
		in := newCrossCompanySaleInput()
		h := service.NewCrossCompanyHandler(cross, service.NewTransactionService(txRepo), nil, nil)

		err := h.Handle(context.Background(), in)

		require.ErrorContains(t, err, "partner royalty service is required")
		assert.Empty(t, txRepo.created)
		assert.InDelta(t, 10.0, in.Payload.CommissionPct, 0.001)
		assert.Empty(t, *in.CrossCompany)
	})

	t.Run("net royalty requires commission service", func(t *testing.T) {
		txRepo := &crossCompanyTxRepo{}
		royaltyRepo := &mockPartnerRoyaltyRepo{}
		cross := &fakeCrossRepo{
			ownerTenant: "tenant-owner",
			partnership: &repository.PartnershipFinancials{
				ID: "pf-net", Type: "franchise", RoyaltyPercentage: 10,
				RoyaltyBase: "net_commission", OwnerCompanyPct: 30, ClosingCompanyPct: 70,
				RequesterTenant: "tenant-owner", TargetTenant: "tenant-closer",
			},
		}
		in := newCrossCompanySaleInput()
		h := service.NewCrossCompanyHandler(
			cross,
			service.NewTransactionService(txRepo),
			nil,
			service.NewPartnerRoyaltyService(royaltyRepo),
		)

		err := h.Handle(context.Background(), in)

		require.ErrorContains(t, err, "commission service is required")
		assert.Empty(t, txRepo.created)
		assert.Empty(t, royaltyRepo.created)
		assert.InDelta(t, 10.0, in.Payload.CommissionPct, 0.001)
		assert.Empty(t, *in.CrossCompany)
	})
}

// Nil cross-repo is a hard no-op (matches the legacy "hooks not wired"
// behaviour).
func TestCrossCompanyHandler_NilCrossRepo_IsNoOp(t *testing.T) {
	h := service.NewCrossCompanyHandler(nil, nil, nil, nil)
	err := h.Handle(context.Background(), newCrossCompanySaleInput())
	assert.NoError(t, err)
}
