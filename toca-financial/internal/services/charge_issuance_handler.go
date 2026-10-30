package services

import (
	"context"
	"fmt"
	"math"

	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

// ChargeIssuanceHandler is the first step of the contract.signed pipeline.
// RENT persists its recurring configuration/aggregate; SALE materializes the
// operator-approved draft into a billing plan plus deterministic manual
// receivable Charges. No gateway charge is created.
//
// chargeService é preservado no struct para compatibilidade de wiring
// (NewChargeIssuanceHandler) mas não é mais usado aqui.
type ChargeIssuanceHandler struct {
	chargeService          *ChargeService
	rentalConfigService    *RentalChargeConfigService
	rentalService          *RentalService
	saleBillingPlanService *SaleBillingPlanService
	tenantSettings         *TenantFinancialSettingsService
}

func (h *ChargeIssuanceHandler) WithTenantSettings(settings *TenantFinancialSettingsService) *ChargeIssuanceHandler {
	h.tenantSettings = settings
	return h
}

// NewChargeIssuanceHandler builds the first step of the chained
// contract.signed pipeline.
func NewChargeIssuanceHandler(
	chargeService *ChargeService,
	rentalConfigService *RentalChargeConfigService,
	rentalService *RentalService,
	saleBillingPlanService *SaleBillingPlanService,
) *ChargeIssuanceHandler {
	return &ChargeIssuanceHandler{
		chargeService:          chargeService,
		rentalConfigService:    rentalConfigService,
		rentalService:          rentalService,
		saleBillingPlanService: saleBillingPlanService,
	}
}

// Handle persists the deal's billing basis: RENT config + aggregate, or the
// SALE plan and its manual receivable schedule. Commission, transfer and
// payout obligations are handled by the later steps.
func (h *ChargeIssuanceHandler) Handle(ctx context.Context, in ContractFinancialsInput) error {
	p := in.Payload
	if isRentalDealType(p.Type) {
		if err := h.provisionRentalConfig(ctx, in); err != nil {
			return err
		}
		if err := h.createRentalAggregate(ctx, in); err != nil {
			return err
		}
		return nil
	}
	return h.provisionSalePlan(ctx, in)
}

// provisionRentalConfig persiste o rental_charge_config (cronograma do
// aluguel) SEM gerar charges. É a base de controle do que será devido
// mensalmente; o repasse provisionado nasce no OwnerTransferHandler.
func (h *ChargeIssuanceHandler) provisionRentalConfig(ctx context.Context, in ContractFinancialsInput) error {
	if h.rentalConfigService == nil {
		return nil
	}
	p := in.Payload
	defaultDueDay := 7
	if h.tenantSettings != nil {
		settings, err := h.tenantSettings.Get(ctx, p.TenantID)
		if err != nil {
			return fmt.Errorf("load tenant financial settings: %w", err)
		}
		if settings != nil && settings.DefaultDueDay > 0 {
			defaultDueDay = settings.DefaultDueDay
		}
	}
	firstChargeType := rental_charge_config.SameMonthProRata
	firstChargeDate := defaultFirstChargeDate(in.StartDate, defaultDueDay)
	monthlyCents := int64(math.Round(p.Value * 100))

	// Emite as cobranças JUNTO com a config, e não como etapa manual separada.
	//
	// Antes o contrato assinado ficava com o cronograma provisionado e nenhuma
	// cobrança emitida — a emissão dependia do operador clicar "Emitir cobranças
	// previstas" na tela de Locação. Um contrato assinado sem nenhuma cobrança é
	// um contrato que não cobra, e nada na tela de contrato sinalizava isso.
	//
	// A emissão é idempotente: cada cobrança carrega um `source_event_id`
	// determinístico por competência, então uma reentrega do evento pula as
	// linhas já existentes em vez de duplicar. Isso também torna a mudança
	// segura para contratos antigos que ainda não têm cobranças.
	_, created, err := h.rentalConfigService.CreateAndGenerateCharges(
		ctx, p.ContractID, p.TenantID, p.CustomerID, p.OwnerID, p.PropertyID,
		in.StartDate, in.EndDate, firstChargeDate, firstChargeType,
		defaultDueDay, monthlyCents,
	)
	if err != nil {
		return fmt.Errorf("failed to create rental charge config: %w", err)
	}
	if created > 0 {
		logger.FromContext(ctx).Info("cobranças emitidas na assinatura",
			"contract_id", p.ContractID, "created", created)
	}
	return nil
}

// createRentalAggregate persiste a row Rental consolidada para que a UX
// admin e o portal naveguem sem percorrer contract→deal manualmente.
// Fail-fast: roda dentro da mesma transação do contrato assinado, então uma
// falha aqui reverte a config já escrita e mantém a entrega recuperável via
// redelivery do evento.
func (h *ChargeIssuanceHandler) createRentalAggregate(ctx context.Context, in ContractFinancialsInput) error {
	if h.rentalService == nil {
		return nil
	}
	p := in.Payload
	if p.CustomerID == "" || p.OwnerID == "" {
		return nil
	}
	brokerID := ""
	if len(p.BrokerParticipations) > 0 {
		brokerID = p.BrokerParticipations[0].BrokerID
	}
	monthlyCents := int64(math.Round(p.Value * 100))
	if _, err := h.rentalService.CreateFromDeal(ctx, CreateFromDealInput{
		TenantID:          p.TenantID,
		DealID:            p.DealID,
		ContractID:        p.ContractID,
		PropertyID:        p.PropertyID,
		TenantLeadID:      p.CustomerID,
		OwnerLeadID:       p.OwnerID,
		BrokerID:          brokerID,
		StartDate:         in.StartDate,
		EndDate:           in.EndDate,
		MonthlyValueCents: monthlyCents,
	}); err != nil {
		return fmt.Errorf("failed to create rental aggregate: %w", err)
	}
	return nil
}

// provisionSalePlan materializes the operator's active draft and its manual
// receivable charges. No gateway is involved: rows start PENDING and are
// settled through the same idempotent payment flow as rent.
func (h *ChargeIssuanceHandler) provisionSalePlan(ctx context.Context, in ContractFinancialsInput) error {
	if h.saleBillingPlanService == nil {
		return nil
	}
	p := in.Payload
	if !isSaleDealType(p.Type) {
		return nil
	}
	// The frozen contract value must equal the draft total; stale drafts are
	// rejected before any monetary row is written.
	if _, _, err := h.saleBillingPlanService.CreateFromDraft(
		ctx, p.TenantID, p.ContractID, p.DealID, p.CustomerID, p.OwnerID, p.PropertyID,
		int64(math.Round(p.Value*100)),
	); err != nil {
		return fmt.Errorf("failed to materialize SALE billing plan: %v", err)
	}
	return nil
}
