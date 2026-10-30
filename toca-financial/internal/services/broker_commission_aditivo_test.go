package services_test

import (
	"context"
	"testing"

	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/assert"
)

// Assinar um ADITIVO não pode gerar uma segunda comissão para o mesmo negócio.
//
// O plan hash é a identidade econômica do negócio — deriva de tipo, valor,
// percentual, participações e regras, nada dele vem do contrato. Enquanto a
// guarda de materialização filtrava por contract_id, a comissão já materializada
// pelo contrato original ficava invisível para o aditivo (contrato novo, plano
// idêntico) e a comissão era criada de novo.
//
// Em produção isso dobrou o valor a pagar: um negócio com comissões PAID em
// 09/09 recebeu o MESMO par de comissões PENDING em 14/09, ao assinar o aditivo
// — mesmo deal, mesmo plan_hash, mesmos corretores e percentuais.
func TestBrokerCommissionHandler_AditivoDoesNotDuplicateCommission(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	h := service.NewBrokerCommissionHandler(
		service.NewTransactionService(txRepo),
		service.NewCommissionService(commRepo),
		nil,
	)

	// Assinatura do contrato original: materializa a comissão.
	original := newSALEBrokerInput()
	original.Payload.ContractID = "contrato-original"
	assert.NoError(t, h.Handle(context.Background(), original))
	assert.NotEmpty(t, commRepo.created, "a assinatura do primeiro contrato precisa materializar a comissão")
	primeiraRodada := len(commRepo.created)

	// Assinatura do ADITIVO: mesmo negócio e mesmo plano, contrato novo.
	aditivo := newSALEBrokerInput()
	aditivo.Payload.ContractID = "contrato-aditivo"

	assert.NoError(t, h.Handle(context.Background(), aditivo))
	assert.Equal(t, primeiraRodada, len(commRepo.created),
		"assinar o aditivo não pode criar uma segunda comissão para o mesmo plano do negócio")
}

// A guarda não pode virar "uma comissão por negócio" e engolir parcelas
// legítimas: papéis distintos produzem plan hashes distintos e continuam sendo
// materializados.
func TestBrokerCommissionHandler_DifferentPlansStillMaterialize(t *testing.T) {
	txRepo := &mockTxRepo{}
	commRepo := &mockCommissionRepo{}
	h := service.NewBrokerCommissionHandler(
		service.NewTransactionService(txRepo),
		service.NewCommissionService(commRepo),
		nil,
	)

	primeiro := newSALEBrokerInput()
	assert.NoError(t, h.Handle(context.Background(), primeiro))
	primeiraRodada := len(commRepo.created)
	assert.NotEmpty(t, primeiraRodada)

	// Mesmo negócio, participação diferente: é outro plano.
	outro := newSALEBrokerInput()
	outro.Payload.BrokerParticipations[0].Percentage = 30
	outro.Payload.BrokerParticipations[1].Percentage = 70

	assert.NoError(t, h.Handle(context.Background(), outro))
	assert.Greater(t, len(commRepo.created), primeiraRodada,
		"um plano diferente para o mesmo negócio precisa ser materializado")
}
