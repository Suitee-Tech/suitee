// Package sale_billing_plan_draft armazena a escolha de forma de
// pagamento de venda gravada pelo operador na esteira ANTES da
// assinatura do contrato. Quando o contract.signed dispara, o branch
// SALE do NewContractSignedEventHandler chama
// SaleBillingPlanService.CreateFromDraft, que lê o draft via
// (tenant_id, deal_id), materializa o SaleBillingPlan real e marca o
// draft como consumed. Sem draft ativo, nenhuma cobrança é fabricada: o
// contrato precisa de uma escolha explícita do operador.
//
// O Draft mora numa tabela separada de SaleBillingPlan (migration
// 20260528120000) porque seu ciclo de vida é independente: pode
// existir antes do contrato, pode ser sobrescrito várias vezes, e
// fica histórico depois do consumo. Sem coupling reverso financeiro
// → deal.
package sale_billing_plan_draft

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
)

// Draft é o snapshot in-progress da escolha do operador. Reaproveita
// os mesmos campos do SaleBillingPlan + Composite porque o objetivo
// é hidratar exatamente o input de sale_billing_plan.New no momento
// do consumo.
type Draft struct {
	ID                     string                           `json:"id"`
	TenantID               string                           `json:"tenant_id"`
	DealID                 string                           `json:"deal_id"`
	Mode                   sale_billing_plan.Mode           `json:"mode"`
	TotalAmountCents       int64                            `json:"total_amount_cents"`
	DownPaymentCents       int64                            `json:"down_payment_cents"`
	InstallmentAmountCents int64                            `json:"installment_amount_cents"`
	InstallmentCount       int                              `json:"installment_count"`
	FirstDueDate           time.Time                        `json:"first_due_date"`
	RecurrenceIntervalDays int                              `json:"recurrence_interval_days"`
	Composite              sale_billing_plan.CompositeInput `json:"composite"`
	CreatedBy              string                           `json:"created_by,omitempty"`
	ConsumedAt             *time.Time                       `json:"consumed_at,omitempty"`
	CreatedAt              time.Time                        `json:"created_at"`
	UpdatedAt              time.Time                        `json:"updated_at"`
}

// New valida e constrói o draft. Reaproveita as mesmas regras de
// sale_billing_plan.New (Mode SINGLE/INSTALLMENTS, saldo + due date,
// somatório dos legs) para falhar cedo se o operador enviar input
// inconsistente — ao chegar no handler contract.signed, a
// materialização não precisa revalidar.
func New(tenantID, dealID string, mode sale_billing_plan.Mode, totalCents, downPaymentCents, installmentAmountCents int64, installmentCount int, firstDueDate time.Time, recurrenceIntervalDays int, composite sale_billing_plan.CompositeInput, createdBy string) (*Draft, error) {
	if tenantID == "" || dealID == "" {
		return nil, errors.New("tenant_id and deal_id are required")
	}
	// Reusa a validação canônica do plano para garantir que o draft
	// represente exatamente um plano valido (incluindo invariante do
	// somatório dos legs).
	probe, err := sale_billing_plan.New(
		tenantID, "draft-probe-"+dealID, dealID, "", "", "",
		mode, totalCents, downPaymentCents, installmentAmountCents,
		installmentCount, firstDueDate, recurrenceIntervalDays, composite,
	)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	return &Draft{
		ID:                     uuid.New().String(),
		TenantID:               tenantID,
		DealID:                 dealID,
		Mode:                   probe.Mode,
		TotalAmountCents:       probe.TotalAmountCents,
		DownPaymentCents:       probe.DownPaymentCents,
		InstallmentAmountCents: probe.InstallmentAmountCents,
		InstallmentCount:       probe.InstallmentCount,
		FirstDueDate:           probe.FirstDueDate,
		RecurrenceIntervalDays: probe.RecurrenceIntervalDays,
		Composite: sale_billing_plan.CompositeInput{
			DacaoAmountCents: probe.DacaoAmountCents,
			DacaoDescription: probe.DacaoDescription,
			TrocaAmountCents: probe.TrocaAmountCents,
			TrocaDescription: probe.TrocaDescription,
			SaldoAmountCents: probe.SaldoAmountCents,
			SaldoDueDate:     probe.SaldoDueDate,
			SaldoDescription: probe.SaldoDescription,
		},
		CreatedBy: createdBy,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// IsConsumed reporta se o draft já foi convertido em SaleBillingPlan
// real pelo contract.signed.
func (d *Draft) IsConsumed() bool {
	return d.ConsumedAt != nil
}
