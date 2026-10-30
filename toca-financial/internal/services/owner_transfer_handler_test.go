package services_test

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
)

func newRentInput(_ *testing.T) service.ContractFinancialsInput {
	now := time.Now()
	return service.ContractFinancialsInput{
		Payload: &contracts.ContractSignedEventPayload{
			DealID:        "deal-rent-1",
			ContractID:    "cont-rent-1",
			TenantID:      "tenant-1",
			Value:         3000.0,
			CommissionPct: 10.0,
			Type:          "RENT",
			CustomerID:    "cust-1",
			OwnerID:       "owner-1",
			PropertyID:    "prop-1",
		},
		StartDate: now,
		EndDate:   now.AddDate(1, 0, 0),
	}
}

func newSaleInput(_ *testing.T) service.ContractFinancialsInput {
	now := time.Now()
	return service.ContractFinancialsInput{
		Payload: &contracts.ContractSignedEventPayload{
			DealID:        "deal-sale-1",
			ContractID:    "cont-sale-1",
			TenantID:      "tenant-1",
			Value:         2000.0,
			CommissionPct: 10.0,
			Type:          "SALE",
			CustomerID:    "cust-2",
			OwnerID:       "owner-2",
			PropertyID:    "prop-2",
		},
		StartDate: now,
		EndDate:   now.AddDate(1, 0, 0),
	}
}

func TestOwnerTransferHandler_DoesNotProvisionBeforeRentIsPaid(t *testing.T) {
	tRepo := &mockTransferRepo{}
	tService := service.NewTransferService(tRepo)

	h := service.NewOwnerTransferHandler(tService, nil)
	in := newRentInput(t)
	err := h.Handle(context.Background(), in)

	assert.NoError(t, err)
	assert.Nil(t, tRepo.created, "RENT só deve gerar OwnerTransfer após a charge ficar PAID")
}

func TestOwnerTransferHandler_SkipsForSale(t *testing.T) {
	tRepo := &mockTransferRepo{}
	tService := service.NewTransferService(tRepo)

	h := service.NewOwnerTransferHandler(tService, nil)
	err := h.Handle(context.Background(), newSaleInput(t))

	assert.NoError(t, err)
	assert.Nil(t, tRepo.created, "SALE não provisiona repasse ao proprietário")
}

func TestOwnerTransferHandler_NoOpWithoutService(t *testing.T) {
	h := service.NewOwnerTransferHandler(nil, nil)
	err := h.Handle(context.Background(), newSaleInput(t))
	assert.NoError(t, err, "nil transferService → no-op, sem erro")
}
