package services_test

import (
	"context"
	"testing"

	"github.com/oginoino/toca/pkg/events/contracts"
	service "github.com/oginoino/toca/toca-deal/internal/services"
	"github.com/stretchr/testify/assert"
)

func TestCollectionEscalatedHandler_TypedPayload(t *testing.T) {
	handler := service.NewCollectionEscalatedHandler()

	evt := contracts.CollectionEscalatedEvent{
		Data: contracts.CollectionEscalatedEventPayload{
			CollectionID:   "coll-1",
			ContractID:     "contract-1",
			TenantID:       "tenant-1",
			PreviousStatus: "ACTIVE",
			NewStatus:      "NEGOTIATED",
			TotalDebt:      3200.0,
			EscalatedBy:    "operator-X",
		},
	}
	assert.NoError(t, handler(context.Background(), evt))
}

func TestCollectionEscalatedHandler_AcceptsMapPayload(t *testing.T) {
	handler := service.NewCollectionEscalatedHandler()

	raw := map[string]interface{}{
		"collection_id":   "coll-2",
		"contract_id":     "contract-2",
		"tenant_id":       "tenant-1",
		"previous_status": "NEGOTIATED",
		"new_status":      "JUDICIAL",
		"total_debt":      9999.0,
	}
	evt := mockInspectionEvent{payload: raw} // reuses lightweight wrapper
	assert.NoError(t, handler(context.Background(), evt))
}

func TestCollectionEscalatedHandler_SkipsMissingFields(t *testing.T) {
	handler := service.NewCollectionEscalatedHandler()
	evt := contracts.CollectionEscalatedEvent{
		Data: contracts.CollectionEscalatedEventPayload{
			NewStatus: "JUDICIAL",
		},
	}
	// Should ack without error even with missing contract id.
	assert.NoError(t, handler(context.Background(), evt))
}
