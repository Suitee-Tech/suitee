package services_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
)

func newSALEBrokerInput() service.ContractFinancialsInput {
	return service.ContractFinancialsInput{
		Payload: &contracts.ContractSignedEventPayload{
			DealID:        "deal-1",
			ContractID:    "cont-1",
			TenantID:      "tenant-1",
			Value:         2000.0,
			CommissionPct: 10.0, // commission amount = 200
			Type:          "SALE",
			CustomerID:    "cust-1",
			OwnerID:       "owner-1",
			PropertyID:    "prop-1",
			BrokerParticipations: []contracts.BrokerParticipation{
				{BrokerID: "broker-1", Role: "captador", Percentage: 50},
				{BrokerID: "broker-2", Role: "fechamento", Percentage: 50},
			},
		},
	}
}

func TestBrokerCommissionHandler_SALE_writesLedgerAndCommissionEntities(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	h := service.NewBrokerCommissionHandler(txService, commService, nil)

	err := h.Handle(context.Background(), newSALEBrokerInput())
	assert.NoError(t, err)

	assert.Len(t, txRepo.created, 2)
	assert.Equal(t, "Comissão - captador (50.0%)", txRepo.created[0].Description)
	assert.Equal(t, 100.0, txRepo.created[0].Value)
	assert.Equal(t, "broker-1", txRepo.created[0].BrokerID)
	assert.Equal(t, "captador", txRepo.created[0].Role)
	assert.Equal(t, "Comissão - fechamento (50.0%)", txRepo.created[1].Description)
	assert.Equal(t, 100.0, txRepo.created[1].Value)
	assert.Equal(t, "broker-2", txRepo.created[1].BrokerID)
	assert.Equal(t, "fechamento", txRepo.created[1].Role)

	assert.Len(t, commRepo.created, 2)
	assert.Equal(t, "broker-1", commRepo.created[0].BrokerID)
	assert.Equal(t, 100.0, commRepo.created[0].Amount)
	assert.Equal(t, commission.StatusPending, commRepo.created[0].Status)
}

// RENT now ALSO persists Commission entities (PENDING) per broker in the
// manual model — settled manually via /commissions/:id/pay. Regression guard
// that the entity persistence covers RENT, not just SALE.
func TestBrokerCommissionHandler_RENT_persistsCommissionEntities(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	in := newSALEBrokerInput()
	in.Payload.Type = "RENT"
	in.Payload.BrokerParticipations = []contracts.BrokerParticipation{
		{BrokerID: "broker-rent", Role: "fechamento", Percentage: 100},
	}

	h := service.NewBrokerCommissionHandler(txService, commService, nil)
	err := h.Handle(context.Background(), in)

	assert.NoError(t, err)
	assert.Len(t, txRepo.created, 1)
	// commission amount = 2000 * 10% = 200; broker-rent 100% → 200.
	assert.Len(t, commRepo.created, 1, "RENT deve provisionar Commission por broker")
	assert.Equal(t, "broker-rent", commRepo.created[0].BrokerID)
	assert.Equal(t, 200.0, commRepo.created[0].Amount)
	assert.Equal(t, commission.StatusPending, commRepo.created[0].Status)
}

func TestBrokerCommissionHandler_NoParticipations_FallbackTotal(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	in := newSALEBrokerInput()
	in.Payload.BrokerParticipations = nil
	rulesService := service.NewCommissionRulesService(&staticCommissionRulesRepo{rules: []commission.Rule{
		{TenantID: "tenant-1", Role: commission.RoleCaptador, DefaultPercentage: 30},
		{TenantID: "tenant-1", Role: commission.RoleFechamento, DefaultPercentage: 70},
	}})

	h := service.NewBrokerCommissionHandler(txService, commService, rulesService)
	err := h.Handle(context.Background(), in)

	// FIN-COM-09: sem broker_participations não gera transação sintética.
	// O contrato foi assinado sem alocação de corretores — estado válido
	// que será resolvido posteriormente (audit + deferred commission).
	assert.NoError(t, err)
	assert.Empty(t, txRepo.created, "FIN-COM-09: no synthetic ledger when no participants")
	assert.Empty(t, commRepo.created, "FIN-COM-09: no synthetic commission entities")
	assert.Empty(t, in.Payload.BrokerParticipations, "write-side must not mutate the deal snapshot")
}

func TestBrokerCommissionHandler_NoParticipations_DoesNotDoubleLegacySyntheticLedger(t *testing.T) {
	const eventID = "event-before-fin-com-09"
	in := newSALEBrokerInput()
	in.EventID = eventID
	in.Payload.BrokerParticipations = nil

	legacy := transaction.New(in.Payload.ContractID, transaction.TypeComissao, "Comissão - captador (30.0%)", 60, time.Now(), in.Payload.TenantID)
	legacy.SourceEventID = events.DeriveNotificationID(
		eventID, "commission_tx", in.Payload.ContractID, "default:captador", "captador",
	)
	txRepo := &mockTxRepo{created: []*transaction.Transaction{legacy}}
	h := service.NewBrokerCommissionHandler(service.NewTransactionService(txRepo), nil, nil)

	err := h.Handle(context.Background(), in)

	assert.NoError(t, err)
	assert.Len(t, txRepo.created, 1, "redrive must not add a consolidated row over legacy synthetic rows")
}

// Commission entity persistence failure for SALE is fail-fast: the handler
// returns the error so the surrounding contract.signed transaction rolls back
// the ledger rows and the event is redelivered atomically.
func TestBrokerCommissionHandler_SALE_CommissionEntityError_IsFailFast(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{err: errors.New("repo down")}
	txService := service.NewTransactionService(txRepo)
	commService := service.NewCommissionService(commRepo)

	h := service.NewBrokerCommissionHandler(txService, commService, nil)
	err := h.Handle(context.Background(), newSALEBrokerInput())

	assert.ErrorContains(t, err, "failed to persist commission entities")
	assert.ErrorContains(t, err, "repo down")
	assert.Len(t, txRepo.created, 2, "ledger rows are written before the entity failure, so the transaction must roll them back")
}

func TestBrokerCommissionHandler_FailsWhenServiceMissing(t *testing.T) {
	h := service.NewBrokerCommissionHandler(nil, nil, nil)
	err := h.Handle(context.Background(), newSALEBrokerInput())
	assert.ErrorContains(t, err, "requires a TransactionService")
}
