package services

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/oginoino/toca/pkg/clients"
)

// ErrSalePlanNotReady marca as recusas do guard de plano financeiro SALE
// (T-01): contrato de venda gerado/enviado sem draft de recebimento ativo e
// compatível com o valor congelado. O handler mapeia para 422 com a mensagem
// operacional — o mesmo contrato que a esteira já bloqueia no frontend.
var ErrSalePlanNotReady = errors.New("sale plan not ready")

// FinancialPlanChecker é a superfície que o ContractService precisa do
// toca-financial para validar contratos de venda ANTES de gerar/enviar.
// *clients.FinancialClient já a implementa; testes injetam um fake.
type FinancialPlanChecker interface {
	GetSalePlanReadiness(ctx context.Context, tenantID, dealID string) (*clients.SalePlanReadiness, error)
}

// WithFinancialPlanChecker substitui o client padrão de readiness do plano
// SALE (usado nos testes e em wiring alternativos).
func (s *ContractService) WithFinancialPlanChecker(checker FinancialPlanChecker) *ContractService {
	s.financialPlanChecker = checker
	return s
}

// requireSalePlanReady valida, no servidor, que um contrato de VENDA só é
// gerado/enviado com o draft de recebimento ativo e com total igual ao valor
// congelado do negócio. Fail-closed: sem o serviço financeiro alcançável o
// envio é recusado (nunca deixa passar uma venda que materializaria cobrança
// errada ou nenhuma).
//
// Espelha o gate do frontend (`saleDraftCurrent` em ContractTab.tsx):
// billingDraft.total_amount_cents === round(deal.value * 100).
func (s *ContractService) requireSalePlanReady(ctx context.Context, tenantID, dealID string, dealValue float64) error {
	if s.financialPlanChecker == nil {
		return fmt.Errorf("%w: validação financeira indisponível (serviço não configurado)", ErrSalePlanNotReady)
	}
	readiness, err := s.financialPlanChecker.GetSalePlanReadiness(ctx, tenantID, dealID)
	if err != nil {
		return fmt.Errorf("não foi possível validar o plano de recebimento no financeiro: %w", err)
	}
	if !readiness.Active {
		return fmt.Errorf("%w: o plano de recebimento da venda ainda não está configurado no financeiro; configure a forma de recebimento antes de continuar", ErrSalePlanNotReady)
	}
	expected := int64(math.Round(dealValue * 100))
	if readiness.TotalAmountCents != expected {
		return fmt.Errorf("%w: o plano de recebimento (%d centavos) não corresponde ao valor do contrato (%d centavos); revise a forma de recebimento", ErrSalePlanNotReady, readiness.TotalAmountCents, expected)
	}
	return nil
}
