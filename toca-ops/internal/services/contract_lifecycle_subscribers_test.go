package services_test

import (
	"context"
	"testing"

	pkgevents "github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

func TestContractSignedInspectionHandler_CreatesEntryInspection(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)
	handler := services.NewContractSignedInspectionHandler(svc)

	evt := contracts.ContractSignedEvent{
		EventIDVal: "evt-1",
		Data: contracts.ContractSignedEventPayload{
			DealID:     "deal-1",
			ContractID: "contract-1",
			TenantID:   "tenant-1",
			SignedAt:   "2026-06-01T10:00:00Z",
			StartDate:  "2026-06-15T00:00:00Z",
			Type:       "locacao",
			CustomerID: "cust-1",
			OwnerID:    "owner-1",
			PropertyID: "prop-1",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))

	derivedID := pkgevents.DeriveNotificationID("contract-1", "inspection.entry")
	stored, err := repo.GetByID(context.Background(), "tenant-1", derivedID)
	assert.NoError(t, err)
	if assert.NotNil(t, stored) {
		assert.Equal(t, inspection.TypeEntry, stored.Type)
		assert.Equal(t, inspection.StatusAgendada, stored.Status)
		assert.Equal(t, "contract-1", stored.ContractID)
		assert.Equal(t, "prop-1", stored.PropertyID)
	}
	assert.Len(t, bus.byName("inspection.scheduled"), 1)
}

func TestContractSignedInspectionHandler_Idempotent(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)
	handler := services.NewContractSignedInspectionHandler(svc)

	evt := contracts.ContractSignedEvent{
		EventIDVal: "evt-1",
		Data: contracts.ContractSignedEventPayload{
			ContractID: "contract-9",
			TenantID:   "tenant-1",
			PropertyID: "prop-1",
			Type:       "locacao",
			StartDate:  "2026-06-15T00:00:00Z",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.NoError(t, handler(context.Background(), evt))
	assert.Len(t, bus.byName("inspection.scheduled"), 1, "second delivery must no-op")
}

func TestContractSignedInspectionHandler_SkipsSaleContracts(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)
	handler := services.NewContractSignedInspectionHandler(svc)

	evt := contracts.ContractSignedEvent{
		Data: contracts.ContractSignedEventPayload{
			ContractID: "contract-sale-1",
			TenantID:   "tenant-1",
			PropertyID: "prop-1",
			Type:       "venda",
			StartDate:  "2026-06-15T00:00:00Z",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
	assert.Empty(t, bus.byName("inspection.scheduled"))
}

func TestContractSignedInspectionHandler_AcceptsMapPayload(t *testing.T) {
	repo := newInMemoryInspectionRepo()
	bus := &captureEventBus{}
	svc := services.NewInspectionService(repo, bus)
	handler := services.NewContractSignedInspectionHandler(svc)

	rawPayload := map[string]interface{}{
		"contract_id": "contract-raw",
		"tenant_id":   "tenant-1",
		"property_id": "prop-1",
		"type":        "locacao",
		"start_date":  "2026-07-01",
	}
	evt := mockEvent{name: "deal.contract.signed", payload: rawPayload}
	assert.NoError(t, handler(context.Background(), evt))

	derivedID := pkgevents.DeriveNotificationID("contract-raw", "inspection.entry")
	stored, _ := repo.GetByID(context.Background(), "tenant-1", derivedID)
	assert.NotNil(t, stored)
}

type mockEvent struct {
	name    string
	payload interface{}
}

func (m mockEvent) EventID() string      { return "" }
func (m mockEvent) Name() string         { return m.name }
func (m mockEvent) Payload() interface{} { return m.payload }

var _ pkgevents.Event = mockEvent{}
