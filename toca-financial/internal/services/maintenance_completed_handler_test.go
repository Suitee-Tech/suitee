package services_test

import (
	"context"
	"testing"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	pkgevents "github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
)

// reuses mockChargeRepo + mockTxRepo + mockEvent from event_handler_test.go

func TestMaintenanceCompletedHandler_NoDiff_NoOp(t *testing.T) {
	// Cobrança já emitida na requisição: diff zero não gera ajuste nenhum.
	existing := &charge.Charge{
		ID:            "ch-original",
		TenantID:      "tenant-1",
		ContractID:    "contract-1",
		TenantPartyID: "lead-tenant",
		OwnerID:       "lead-owner",
		SourceEventID: pkgevents.DeriveNotificationID("ticket-1", "maintenance.requested"),
		Status:        charge.StatusPending,
	}
	cRepo := &mockChargeRepo{created: existing}
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, nil)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-1",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:      "ticket-1",
			TenantID:      "tenant-1",
			ContractID:    "contract-1",
			PayerRole:     "tenant",
			EstimatedCost: 500.0,
			ActualCost:    500.0,
		},
	}
	err := handler(context.Background(), evt)
	assert.NoError(t, err)
	assert.Equal(t, "ch-original", cRepo.created.ID, "nenhuma cobrança nova quando o diff é zero")
	assert.Empty(t, txRepo.created, "no transaction should be created when diff is zero")
}

// A política financeira pode ser definida só na conclusão do chamado (tela
// "Integração Financeira"). Nesse caso o evento `maintenance.requested` saiu
// sem payer_role e nada foi criado — a conclusão precisa emitir a cobrança,
// senão o lançamento se perde.
func TestMaintenanceCompletedHandler_CreatesBillingWhenNoneExists(t *testing.T) {
	cRepo := &billingChargeRepo{}
	billing := service.NewMaintenanceBillingService(service.NewChargeService(cRepo, nil), cRepo, nil, nil)
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, billing)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-1",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:        "ticket-late-policy",
			TenantID:        "tenant-1",
			ContractID:      "contract-1",
			PropertyID:      "property-1",
			RequesterLeadID: "lead-tenant",
			Category:        "Elétrica",
			PayerRole:       "tenant",
			EstimatedCost:   130.0,
			ActualCost:      130.0,
		},
	}

	err := handler(context.Background(), evt)
	assert.NoError(t, err)

	created := cRepo.lastCreated()
	assert.NotNil(t, created, "a cobrança deve nascer na conclusão quando ainda não existe")
	assert.Equal(t, charge.TypeMaintenance, created.Type)
	assert.Equal(t, "contract-1", created.ContractID)
	assert.Equal(t, "property-1", created.PropertyID)
	assert.Equal(t, "lead-tenant", created.TenantPartyID)
	assert.Equal(t, pkgevents.DeriveNotificationID("ticket-late-policy", "maintenance.requested"), created.SourceEventID)
	assert.InDelta(t, 130.0, created.Amount.Float(), 0.001)
	assert.Len(t, created.LineItems, 1, "a cobrança avulsa precisa nascer com composição")
	assert.Equal(t, "ticket-late-policy", created.LineItems[0].SourceID)
}

// Sem valor informado não há lançamento — a tela exige o valor do serviço.
func TestMaintenanceCompletedHandler_NoAmount_NoBilling(t *testing.T) {
	cRepo := &mockChargeRepo{}
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, nil)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-1",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:   "ticket-1",
			TenantID:   "tenant-1",
			ContractID: "contract-1",
			PayerRole:  "tenant",
			ActualCost: 0,
		},
	}

	err := handler(context.Background(), evt)
	assert.NoError(t, err)
	assert.Nil(t, cRepo.created)
}

func TestMaintenanceCompletedHandler_NoPayerRole_NoOp(t *testing.T) {
	cRepo := &mockChargeRepo{}
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, nil)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-1",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:      "ticket-1",
			TenantID:      "tenant-1",
			PayerRole:     "",
			EstimatedCost: 500.0,
			ActualCost:    600.0,
		},
	}
	err := handler(context.Background(), evt)
	assert.NoError(t, err)
	assert.Nil(t, cRepo.created, "no charge should be created when payer_role is empty")
}

func TestMaintenanceCompletedHandler_Overrun_CreatesDiffCharge(t *testing.T) {
	// Seed the existing maintenance.requested charge so the diff path can
	// inherit owner/tenant ids and deal id from it.
	existing := &charge.Charge{
		ID:            "ch-original",
		TenantID:      "tenant-1",
		ContractID:    "contract-1",
		DealID:        "deal-99",
		TenantPartyID: "lead-tenant",
		OwnerID:       "lead-owner",
		SourceEventID: pkgevents.DeriveNotificationID("ticket-1", "maintenance.requested"),
		Status:        charge.StatusPending,
	}
	cRepo := &mockChargeRepo{created: existing}
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, nil)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-1",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:      "ticket-1",
			TenantID:      "tenant-1",
			ContractID:    "contract-1",
			PropertyID:    "prop-1",
			Category:      "hidráulica",
			PayerRole:     "tenant",
			EstimatedCost: 500.0,
			ActualCost:    750.0,
		},
	}
	err := handler(context.Background(), evt)
	assert.NoError(t, err)
	assert.NotNil(t, cRepo.created)
	// The repo records only the last Created charge; verify it carries the
	// expected derived source_event_id for the diff path.
	expected := pkgevents.DeriveNotificationID("ticket-1", "maintenance.completed.diff")
	assert.Equal(t, expected, cRepo.created.SourceEventID)
	assert.Equal(t, charge.TypeOther, cRepo.created.Type)
	assert.InDelta(t, 250.0, cRepo.created.Amount.Float(), 0.001)
}

func TestMaintenanceCompletedHandler_Underrun_PendingChargeGetsDiscount(t *testing.T) {
	existing := &charge.Charge{
		ID:            "ch-original",
		TenantID:      "tenant-1",
		ContractID:    "contract-1",
		Amount:        valueobjects.NewMoneyFromFloat(500.0),
		TotalAmount:   valueobjects.NewMoneyFromFloat(500.0),
		Status:        charge.StatusPending,
		SourceEventID: pkgevents.DeriveNotificationID("ticket-2", "maintenance.requested"),
	}
	cRepo := &mockChargeRepo{created: existing}
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, nil)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-2",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:      "ticket-2",
			TenantID:      "tenant-1",
			ContractID:    "contract-1",
			PayerRole:     "tenant",
			EstimatedCost: 500.0,
			ActualCost:    380.0,
		},
	}
	err := handler(context.Background(), evt)
	assert.NoError(t, err)
	assert.InDelta(t, 120.0, existing.Discount.Float(), 0.001)
	assert.InDelta(t, 380.0, existing.TotalAmount.Float(), 0.001)
	assert.Empty(t, txRepo.created, "no transaction needed when the original charge is still PENDING")
}

func TestMaintenanceCompletedHandler_Underrun_PaidChargeGetsCreditTransaction(t *testing.T) {
	existing := &charge.Charge{
		ID:            "ch-original",
		TenantID:      "tenant-1",
		ContractID:    "contract-1",
		Amount:        valueobjects.NewMoneyFromFloat(500.0),
		TotalAmount:   valueobjects.NewMoneyFromFloat(500.0),
		Status:        charge.StatusPaid,
		SourceEventID: pkgevents.DeriveNotificationID("ticket-3", "maintenance.requested"),
	}
	cRepo := &mockChargeRepo{created: existing}
	txRepo := &mockTxRepo{}
	cService := service.NewChargeService(cRepo, nil)
	txService := service.NewTransactionService(txRepo)
	handler := service.NewMaintenanceCompletedEventHandler(cService, cRepo, txService, nil)

	evt := contracts.MaintenanceCompletedEvent{
		EventIDVal: "evt-3",
		Data: contracts.MaintenanceCompletedEventPayload{
			TicketID:      "ticket-3",
			TenantID:      "tenant-1",
			ContractID:    "contract-1",
			PayerRole:     "tenant",
			Category:      "elétrica",
			EstimatedCost: 500.0,
			ActualCost:    320.0,
		},
	}
	err := handler(context.Background(), evt)
	assert.NoError(t, err)
	if assert.Len(t, txRepo.created, 1, "should create exactly one credit transaction") {
		tx := txRepo.created[0]
		// Nota de crédito: valor negativo e tipo de estorno — a devolução não é
		// despesa nem receita da imobiliária.
		assert.Equal(t, transaction.TypeEstorno, tx.Type)
		assert.InDelta(t, -180.0, tx.Value, 0.001)
		assert.Equal(t, "Manutenção", tx.Category)
	}
	assert.InDelta(t, 0.0, existing.Discount.Float(), 0.001, "PAID charges must not be discounted")
}
