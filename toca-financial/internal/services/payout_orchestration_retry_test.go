package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type retryPayoutOrderRepo struct {
	*noopPayoutOrderRepo
	createErr      error
	existing       *payout_order.PayoutOrder
	lookupErr      error
	attempted      *payout_order.PayoutOrder
	lookupTenantID string
	lookupSourceID string
}

func (r *retryPayoutOrderRepo) Create(_ context.Context, order *payout_order.PayoutOrder) error {
	r.attempted = order
	return r.createErr
}

func (r *retryPayoutOrderRepo) UpdateIfStatus(_ context.Context, order *payout_order.PayoutOrder, _ payout_order.Status, _ time.Time) error {
	r.existing = order
	return nil
}

func (r *retryPayoutOrderRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*payout_order.PayoutOrder, error) {
	r.lookupTenantID = tenantID
	r.lookupSourceID = sourceEventID
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	if r.existing != nil && r.existing.SourceEventID == sourceEventID {
		return r.existing, nil
	}
	return nil, nil
}

type payoutLinkCommissionRepo struct {
	*mockCommissionRepo
	commissions []*commission.Commission
	updates     int
}

func (r *payoutLinkCommissionRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	for _, c := range r.commissions {
		if c.TenantID == tenantID && c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}

func (r *payoutLinkCommissionRepo) ListByDealID(_ context.Context, tenantID, dealID string) ([]*commission.Commission, error) {
	result := make([]*commission.Commission, 0, len(r.commissions))
	for _, c := range r.commissions {
		if c.TenantID == tenantID && c.DealID == dealID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (r *payoutLinkCommissionRepo) Update(_ context.Context, _ *commission.Commission) error {
	r.updates++
	return nil
}

func TestPayoutOrchestrationContractSignedRetryLinksPersistedOrder(t *testing.T) {
	const (
		tenantID   = "tenant-retry"
		contractID = "contract-retry"
		dealID     = "deal-retry"
		brokerID   = "broker-retry"
		eventID    = "event-retry"
		label      = "Comissão captador (40.0%)"
	)

	sourceEventID := events.DeriveNotificationID(
		eventID, "commission_payout", contractID,
		string(payout_order.RecipientBroker), brokerID, "captador",
	)
	existing, err := payout_order.New(
		tenantID, payout_order.SourceManual, contractID, payout_order.RecipientBroker,
		brokerID, label, 6000, "system",
	)
	require.NoError(t, err)
	existing.ID = "persisted-payout-id"
	existing.SourceEventID = sourceEventID

	payoutRepo := &retryPayoutOrderRepo{
		noopPayoutOrderRepo: &noopPayoutOrderRepo{},
		createErr:           payout_order.ErrPayoutOrderAlreadyExists,
		existing:            existing,
	}
	comm := commission.NewWithRole(dealID, brokerID, contractID, "captador", 40, 60, tenantID)
	commissionRepo := &payoutLinkCommissionRepo{
		mockCommissionRepo: &mockCommissionRepo{},
		commissions:        []*commission.Commission{comm},
	}
	commissionService := service.NewCommissionService(commissionRepo)
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commissionService)

	in := service.ContractFinancialsInput{
		EventID: eventID,
		Payload: &contracts.ContractSignedEventPayload{
			TenantID: tenantID, ContractID: contractID, DealID: dealID,
			Type: "SALE", Value: 2000, CommissionPct: 7.5,
		},
		CommissionPlan: &service.ResolvedCommissionPlan{
			CommissionPct:    7.5,
			TotalAmountCents: 15000,
			Shares: []service.ResolvedCommissionShare{{
				BrokerID: brokerID, Role: "captador", Percentage: 40, AmountCents: 6000,
			}},
		},
	}

	require.NoError(t, payoutService.OnContractSigned(context.Background(), in))
	require.NotNil(t, payoutRepo.attempted)
	assert.NotEqual(t, existing.ID, payoutRepo.attempted.ID, "retry UUID must be discarded")
	assert.Equal(t, tenantID, payoutRepo.lookupTenantID)
	assert.Equal(t, sourceEventID, payoutRepo.lookupSourceID)
	assert.Equal(t, existing.ID, comm.PayoutOrderID)
	assert.Equal(t, 1, commissionRepo.updates)
}

func TestPayoutOrchestrationContractSignedRetryRejectsDivergentFrozenAmount(t *testing.T) {
	const (
		tenantID   = "tenant-retry"
		contractID = "contract-retry"
		dealID     = "deal-retry"
		brokerID   = "broker-retry"
		eventID    = "event-retry"
	)

	sourceEventID := events.DeriveNotificationID(
		eventID, "commission_payout", contractID,
		string(payout_order.RecipientBroker), brokerID, "captador",
	)
	existing, err := payout_order.New(
		tenantID, payout_order.SourceManual, contractID, payout_order.RecipientBroker,
		brokerID, "Comissão captador (40.0%)", 6000, "system",
	)
	require.NoError(t, err)
	existing.ID = "persisted-payout-id"
	existing.SourceEventID = sourceEventID

	payoutRepo := &retryPayoutOrderRepo{
		noopPayoutOrderRepo: &noopPayoutOrderRepo{},
		createErr:           payout_order.ErrPayoutOrderAlreadyExists,
		existing:            existing,
	}
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	comm := commission.NewWithRole(dealID, brokerID, contractID, "captador", 35, 52.5, tenantID)
	commissionRepo := &payoutLinkCommissionRepo{
		mockCommissionRepo: &mockCommissionRepo{},
		commissions:        []*commission.Commission{comm},
	}
	payoutService.SetCommissionService(service.NewCommissionService(commissionRepo))
	in := service.ContractFinancialsInput{
		EventID: eventID,
		Payload: &contracts.ContractSignedEventPayload{
			TenantID: tenantID, ContractID: contractID, DealID: dealID,
			Type: "SALE", Value: 2000, CommissionPct: 7.5,
		},
		// Simula redrive posterior a uma mudança de regra. Label, percentual e
		// valor mudaram, mas a origem econômica e o beneficiário são os mesmos.
		CommissionPlan: &service.ResolvedCommissionPlan{
			CommissionPct:    7.5,
			TotalAmountCents: 15000,
			Shares: []service.ResolvedCommissionShare{{
				BrokerID: brokerID, Role: "captador", Percentage: 35, AmountCents: 5250,
			}},
		},
	}

	require.ErrorIs(t, payoutService.OnContractSigned(context.Background(), in), payout_order.ErrFundingGateConflict)
	require.NotNil(t, payoutRepo.attempted)
	assert.Empty(t, comm.PayoutOrderID, "a conflicting obligation must not be linked")
	assert.Equal(t, int64(6000), existing.AmountCents, "the frozen obligation must remain unchanged")
	assert.Equal(t, "Comissão captador (35.0%)", payoutRepo.attempted.RecipientLabel)
	assert.Equal(t, int64(5250), payoutRepo.attempted.AmountCents)
	assert.Equal(t, sourceEventID, payoutRepo.attempted.SourceEventID)
	assert.Equal(t, sourceEventID, payoutRepo.lookupSourceID)
}

func TestPayoutOrchestrationDoesNotCreateOrphanBrokerPayout(t *testing.T) {
	tests := []struct {
		name              string
		commissionService *service.CommissionService
		wantError         string
	}{
		{name: "missing commission service", wantError: "commission service is required"},
		{
			name: "no matching commission",
			commissionService: service.NewCommissionService(&payoutLinkCommissionRepo{
				mockCommissionRepo: &mockCommissionRepo{},
				commissions: []*commission.Commission{
					commission.NewWithRole("deal-orphan", "different-broker", "contract-orphan", "captador", 100, 100, "tenant-orphan"),
				},
			}),
			wantError: "commission not found for broker payout",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payoutRepo := &retryPayoutOrderRepo{noopPayoutOrderRepo: &noopPayoutOrderRepo{}}
			payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
			payoutService.SetCommissionService(tc.commissionService)
			in := service.ContractFinancialsInput{
				EventID: "event-orphan",
				Payload: &contracts.ContractSignedEventPayload{
					TenantID: "tenant-orphan", ContractID: "contract-orphan", DealID: "deal-orphan", Type: "SALE", Value: 1000,
				},
				CommissionPlan: &service.ResolvedCommissionPlan{
					CommissionPct: 10, TotalAmountCents: 10000,
					Shares: []service.ResolvedCommissionShare{{
						BrokerID: "broker-orphan", Role: "captador", Percentage: 100, AmountCents: 10000,
					}},
				},
			}

			err := payoutService.OnContractSigned(context.Background(), in)

			require.ErrorContains(t, err, tc.wantError)
			assert.Nil(t, payoutRepo.attempted, "broker payout must not be persisted without its Commission link")
		})
	}
}

func TestContractSignedEventHandlerPropagatesPayoutProvisioningFailure(t *testing.T) {
	persistErr := errors.New("payout repository unavailable")
	payoutRepo := &retryPayoutOrderRepo{
		noopPayoutOrderRepo: &noopPayoutOrderRepo{},
		createErr:           persistErr,
	}
	commissionRepo := &mockCommissionRepo{}
	commissionService := service.NewCommissionService(commissionRepo)
	payoutService := service.NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commissionService)

	handler := service.NewContractSignedEventHandler(
		nil,
		service.NewChargeService(&mockChargeRepo{}, nil),
		service.NewTransferService(&mockTransferRepo{}),
		service.NewTransactionService(&mockTxRepo{}),
		nil, nil, commissionService, nil, nil, nil, nil, nil, payoutService,
	)
	event := mockEvent{name: "contract.signed", payload: map[string]interface{}{
		"contract_id":    "contract-failure",
		"deal_id":        "deal-failure",
		"tenant_id":      "tenant-failure",
		"type":           "SALE",
		"value":          2000.0,
		"commission_pct": 10.0,
		"customer_id":    "customer-failure",
		"owner_id":       "owner-failure",
		"property_id":    "property-failure",
		"broker_participations": []interface{}{
			map[string]interface{}{
				"broker_id": "broker-failure", "role": "captador", "percentage": 100.0,
			},
		},
	}}

	err := handler(context.Background(), event)
	// A missing monetary obligation must keep the broker delivery retryable.
	// Idempotent source ids make already-written earlier legs safe to replay.
	require.ErrorIs(t, err, persistErr)
}
