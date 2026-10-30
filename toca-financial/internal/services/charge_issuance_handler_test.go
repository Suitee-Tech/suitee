package services_test

import (
	"context"
	"testing"

	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
)

func TestChargeIssuanceHandler_SALE_noChargesIssued(t *testing.T) {
	cRepo := &mockChargeRepo{}
	cService := service.NewChargeService(cRepo, nil)

	// Modelo manual: sem SaleBillingPlanService e sem gateway, nenhuma
	// cobrança é emitida na assinatura. O provisionamento de comissão /
	// payout acontece nos passos seguintes da chain.
	h := service.NewChargeIssuanceHandler(cService, nil, nil, nil)
	err := h.Handle(context.Background(), newSaleInput(t))

	assert.NoError(t, err)
	assert.Nil(t, cRepo.created, "nenhuma cobrança deve ser emitida no modelo manual")
}

// A ausência do serviço de cronograma não pode quebrar o handler.
//
// RENT COM serviço emite as cobranças na assinatura (ver
// charge_issuance_on_signing_test.go). Sem o serviço — montagem parcial, testes
// de outros passos da chain — o provisionamento é no-op e o handler segue.
func TestChargeIssuanceHandler_RENT_NoConfigServiceIsNoop(t *testing.T) {
	cRepo := &mockChargeRepo{}
	cService := service.NewChargeService(cRepo, nil)

	h := service.NewChargeIssuanceHandler(cService, nil, nil, nil)
	err := h.Handle(context.Background(), newRentInput(t))

	assert.NoError(t, err)
	assert.Nil(t, cRepo.created, "sem serviço de cronograma não há cobrança a emitir")
}

func TestChargeIssuanceHandler_NilRentalServiceSkipsAggregate(t *testing.T) {
	cRepo := &mockChargeRepo{}
	cService := service.NewChargeService(cRepo, nil)

	// rentalService nil — aggregate creation is skipped without error.
	h := service.NewChargeIssuanceHandler(cService, nil, nil, nil)
	in := newRentInput(t)
	err := h.Handle(context.Background(), in)
	assert.NoError(t, err)
}
