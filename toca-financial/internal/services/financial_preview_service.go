package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

// FinancialPreviewService projeta — sem persistir e sem chamar o
// gateway — o ciclo financeiro completo que decorreria da assinatura
// do deal: charges (RENT mensal pro-rata + N meses, ou SALE entrada
// + parcelas + saldo), commissions por broker resolvidas via
// CommissionRulesService, e (no RENT) o Transfer mensal projetado ao
// proprietário.
//
// Alimenta a aba "Financeiro (Prévia)" do EsteiraDealDetails antes do
// CONVERTED — operador valida valores e datas antes de assinar.
// Pós-assinatura, a aba consome charges reais via ChargeService.
//
// É puro: reusa BuildChargesPreview do RentalChargeConfigService e do
// SaleBillingPlanService + ResolveCommissionPlan, o mesmo resolver do
// contract.signed. Mesmo input → mesma saída (idempotente em leituras).
type FinancialPreviewService struct {
	dealClient             *clients.DealClient
	rentalConfigService    *RentalChargeConfigService
	saleBillingPlanService *SaleBillingPlanService
	commissionRulesService *CommissionRulesService
	tenantSettings         *TenantFinancialSettingsService
	crossCompany           CrossCompanyLookup
}

func (s *FinancialPreviewService) SetCrossCompanyLookup(lookup CrossCompanyLookup) {
	s.crossCompany = lookup
}

// ResolvePlanForDeal resolves the canonical commission plan for a deal — the
// same resolution used by the preview and contract.signed — so charge payment
// can apply the sale economic composition on the fly when the charge was
// created without line items (e.g. plan/charges created before signing).
func (s *FinancialPreviewService) ResolvePlanForDeal(ctx context.Context, tenantID, dealID string) (*ResolvedCommissionPlan, error) {
	if s == nil || s.dealClient == nil {
		return nil, errors.New("financial preview deal client is not configured")
	}
	d, err := s.dealClient.GetDeal(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, nil
	}
	// A pre-signature receipt cannot synthesize a local-only allocation for a
	// partner deal. Signing freezes both company obligations on the charge.
	if s.crossCompany != nil {
		partnership, source, lookupErr := repositories.ResolveDealPartnership(ctx, s.crossCompany, dealID, d.PropertyID, tenantID)
		if lookupErr != nil {
			return nil, lookupErr
		}
		if partnership != nil || source != "" {
			return nil, errors.New("assine o contrato e confira a composição financeira da parceria antes de registrar este recebimento")
		}
	}
	rules, settings, err := s.loadResolutionInputs(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	bps := make([]contracts.BrokerParticipation, 0, len(d.BrokerParticipations))
	for _, bp := range d.BrokerParticipations {
		bps = append(bps, contracts.BrokerParticipation{
			BrokerID:   bp.BrokerID,
			Role:       bp.Role,
			Percentage: bp.Percentage,
		})
	}
	// Espelha o adapter do contract.signed: CommissionPct == 0 é resolvido
	// pelo default do tenant ANTES do plan hash, para que a composição
	// econômica aplicada na baixa (RuleVersionID) coincida com o hash das
	// comissões congeladas no pipeline (mesmo quando o deal não traz
	// percentual configurado e o default do tenant supre o valor).
	commissionPct := ResolveCommissionPercentage(d.Type, d.CommissionPct, settings)
	plan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             d.Type,
		DealValue:            d.Value,
		CommissionPct:        commissionPct,
		BrokerParticipations: bps,
		Rules:                rules,
		TenantSettings:       settings,
	})
	if err != nil {
		return nil, fmt.Errorf("invalid commission plan: %w", err)
	}
	return &plan, nil
}

func (s *FinancialPreviewService) loadResolutionInputs(ctx context.Context, tenantID string) ([]commission.Rule, *tenant_financial_settings.Settings, error) {
	var rules []commission.Rule
	var settings *tenant_financial_settings.Settings
	if s.tenantSettings != nil {
		ts, err := s.tenantSettings.Get(ctx, tenantID)
		if err != nil {
			return nil, nil, fmt.Errorf("load tenant financial settings: %w", err)
		}
		settings = ts
	}
	if s.commissionRulesService != nil {
		r, err := s.commissionRulesService.Get(ctx, tenantID)
		if err != nil {
			return nil, nil, fmt.Errorf("load commission rules: %w", err)
		}
		rules = r
	}
	return rules, settings, nil
}

func NewFinancialPreviewService(
	dealClient *clients.DealClient,
	rentalConfigService *RentalChargeConfigService,
	saleBillingPlanService *SaleBillingPlanService,
	commissionRulesService *CommissionRulesService,
	tenantSettings *TenantFinancialSettingsService,
) *FinancialPreviewService {
	return &FinancialPreviewService{
		dealClient:             dealClient,
		rentalConfigService:    rentalConfigService,
		saleBillingPlanService: saleBillingPlanService,
		commissionRulesService: commissionRulesService,
		tenantSettings:         tenantSettings,
	}
}

// ChargePreview é a projeção pública de uma charge — somente leitura,
// não persiste.
type ChargePreview struct {
	SourceEventID  string    `json:"source_event_id"`
	Type           string    `json:"type"`
	Description    string    `json:"description"`
	AmountCents    int64     `json:"amount_cents"`
	DueDate        time.Time `json:"due_date"`
	ReferenceMonth string    `json:"reference_month"`
}

// CommissionPreview é a projeção pública de uma commission por broker.
type CommissionPreview struct {
	BrokerID    string  `json:"broker_id"`
	Role        string  `json:"role"`
	Percentage  float64 `json:"percentage"`
	AmountCents int64   `json:"amount_cents"`
	Retained    bool    `json:"retained,omitempty"`
}

// TransferPreview é a projeção pública de um Transfer mensal ao
// proprietário (RENT). Para SALE, o repasse depende do pagamento de
// cada parcela e é projetado quando aplicável.
type TransferPreview struct {
	ReferenceMonth            string  `json:"reference_month"`
	GrossCents                int64   `json:"gross_cents"`
	AdminFeePct               float64 `json:"admin_fee_pct"`
	AdminFeeCents             int64   `json:"admin_fee_cents"`
	IntermediationCents       int64   `json:"intermediation_cents"`
	CommissionFundedCents     int64   `json:"commission_funded_cents"`
	AgencyIntermediationCents int64   `json:"agency_intermediation_cents"`
	IntermediationCarryCents  int64   `json:"intermediation_carry_cents"`
	NetCents                  int64   `json:"net_cents"`
}

// EconomicCompositionPreview is the waterfall that answers the operator's
// central question: after receiving the money, what belongs to third parties,
// what is revenue retained by the agency, and what remains payable to the
// property owner. Values are cents to keep the preview aligned with payment
// allocations and settlement records.
type EconomicCompositionPreview struct {
	Flow                   string `json:"flow"`
	GrossCents             int64  `json:"gross_cents"`
	CommissionPoolCents    int64  `json:"commission_pool_cents"`
	CommissionPayableCents int64  `json:"commission_payable_cents"`
	PartnerPayableCents    int64  `json:"partner_payable_cents"`
	AgencyRevenueCents     int64  `json:"agency_revenue_cents"`
	AdminFeeCents          int64  `json:"admin_fee_cents"`
	IntermediationCents    int64  `json:"intermediation_cents"`
	CommissionFundedCents  int64  `json:"commission_funded_cents"`
	AgencyRetainedCents    int64  `json:"agency_retained_cents"`
	OwnerPayableCents      int64  `json:"owner_payable_cents"`
	ReservedCents          int64  `json:"reserved_cents"`
	AvailableCents         int64  `json:"available_cents"`
	DeductionsCents        int64  `json:"deductions_cents"`
}

// FinancialPreviewDTO é o snapshot devolvido pelo handler GET
// /financial-preview/deals/:id. Totais agregam por categoria.
type FinancialPreviewDTO struct {
	PartnershipFees      []PartnershipFeePreview     `json:"partnership_fees,omitempty"`
	DealID               string                      `json:"deal_id"`
	ContractID           string                      `json:"contract_id,omitempty"`
	DealType             string                      `json:"deal_type"`
	Charges              []ChargePreview             `json:"charges"`
	Commissions          []CommissionPreview         `json:"commissions"`
	Transfers            []TransferPreview           `json:"transfers,omitempty"`
	Composition          *EconomicCompositionPreview `json:"composition,omitempty"`
	TotalChargesCents    int64                       `json:"total_charges_cents"`
	TotalCommissionCents int64                       `json:"total_commission_cents"`
	TotalTransfersCents  int64                       `json:"total_transfers_cents"`
	GatewayDisabled      bool                        `json:"gateway_disabled"`
	Notes                []string                    `json:"notes,omitempty"`
}

const previewDefaultDueDay = 7
const previewDefaultLeaseTermMonths = 30

// PreviewByDeal projeta o ciclo financeiro a partir do deal. Não
// persiste nada e não chama o gateway. Pode ser invocado em qualquer
// stage da esteira.
func (s *FinancialPreviewService) PreviewByDeal(ctx context.Context, tenantID, dealID string) (*FinancialPreviewDTO, error) {
	return s.PreviewByDealWithDates(ctx, tenantID, dealID, nil, nil)
}

// PreviewByDealWithDates projeta o ciclo financeiro do deal usando as datas
// informadas quando elas existem. Serve ao plano pré-contrato da locação: o
// negócio ainda não guarda início/término, então sem o override a projeção
// cairia no prazo padrão de 30 meses e mostraria um calendário diferente do
// que o operador acabou de configurar (prazo, pró-rata do primeiro mês e
// residual do último).
//
// O override vale só para ESTA projeção — nada é persistido; a fonte de verdade
// continua sendo o contrato gerado.
func (s *FinancialPreviewService) PreviewByDealWithDates(ctx context.Context, tenantID, dealID string, startOverride, endOverride *time.Time) (*FinancialPreviewDTO, error) {
	if s.dealClient == nil {
		return nil, fmt.Errorf("deal client not configured")
	}
	d, err := s.dealClient.GetDeal(ctx, tenantID, dealID)
	if err != nil {
		return nil, fmt.Errorf("failed to load deal: %w", err)
	}
	if d == nil {
		return nil, fmt.Errorf("deal not found")
	}
	if startOverride != nil && !startOverride.IsZero() {
		start := *startOverride
		d.StartDate = &start
	}
	if endOverride != nil && !endOverride.IsZero() {
		end := *endOverride
		d.EndDate = &end
	}
	return s.previewFromDeal(ctx, tenantID, d, "")
}

// PreviewByContract resolve o deal a partir do contract e devolve a
// mesma projeção, garantindo que startDate/endDate consultados venham
// do contract (mais autoritativo quando já assinado).
func (s *FinancialPreviewService) PreviewByContract(ctx context.Context, tenantID, contractID string) (*FinancialPreviewDTO, error) {
	if s.dealClient == nil {
		return nil, fmt.Errorf("deal client not configured")
	}
	contract, err := s.dealClient.GetContract(ctx, tenantID, contractID)
	if err != nil {
		return nil, fmt.Errorf("failed to load contract: %w", err)
	}
	if contract == nil {
		return nil, fmt.Errorf("contract not found")
	}
	if contract.DealID == "" {
		return nil, fmt.Errorf("contract has no deal_id reference")
	}
	d, err := s.dealClient.GetDeal(ctx, tenantID, contract.DealID)
	if err != nil {
		return nil, fmt.Errorf("failed to load deal from contract: %w", err)
	}
	// Quando o contrato traz datas mais precisas, sobrescrevemos o
	// snapshot do deal para a projeção. Mantém o preview pré-assinatura
	// consistente com o que o contract.signed handler vai materializar.
	startDate := contract.StartDate
	endDate := contract.EndDate
	if !startDate.IsZero() {
		d.StartDate = &startDate
	}
	if endDate != nil {
		d.EndDate = endDate
	}
	// Sobrescreve CommissionPct do deal com o snapshot do contrato
	// quando presente. O contrato é a fonte autoritativa pós-assinatura
	// — o deal pode ter sido alterado após a assinatura, mas o que vale
	// para o ciclo financeiro é o percentual congelado no contrato.
	if contract.CommissionPct > 0 {
		d.CommissionPct = contract.CommissionPct
	}
	if contract.Value > 0 {
		d.Value = contract.Value
	}
	return s.previewFromDeal(ctx, tenantID, d, contractID)
}

func (s *FinancialPreviewService) previewFromDeal(ctx context.Context, tenantID string, d *clients.DealDTO, contractID string) (*FinancialPreviewDTO, error) {
	out := &FinancialPreviewDTO{
		DealID:     d.ID,
		ContractID: contractID,
		DealType:   d.Type,
		// Initialize the projection lists so an empty projection serializes as
		// [] instead of null. A nil Go slice marshals to JSON null, and the
		// frontend (FinancialPreviewCard) reads .length/.map on charges and
		// commissions without a null guard — null there throws
		// "Cannot read properties of null (reading 'length')".
		Charges:     []ChargePreview{},
		Commissions: []CommissionPreview{},
		Transfers:   []TransferPreview{},
		// No payment gateway exists anymore — settlement is always manual.
		GatewayDisabled: true,
	}

	startDate, endDate := s.normalizeDates(d)

	// Tenant defaults and commission rules are authoritative inputs to the
	// projection. A lookup failure must not produce a plausible but incorrect
	// preview that can later diverge from contract.signed.
	defaultDueDay := previewDefaultDueDay
	adminFeePct := 0.0
	var rules []commission.Rule
	var settings *tenant_financial_settings.Settings
	if s.tenantSettings != nil {
		ts, err := s.tenantSettings.Get(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("load tenant financial settings: %w", err)
		}
		if ts != nil {
			settings = ts
			if ts.DefaultDueDay > 0 {
				defaultDueDay = ts.DefaultDueDay
			}
			adminFeePct = ts.AdminFeePct
		}
	}
	if s.commissionRulesService != nil {
		r, err := s.commissionRulesService.Get(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("load commission rules: %w", err)
		}
		rules = r
	}

	if isRentalDealType(d.Type) {
		if err := s.appendRentPreview(ctx, out, d, startDate, endDate, defaultDueDay); err != nil {
			return nil, err
		}
	} else if isSaleDealType(d.Type) {
		if err := s.appendSalePreview(ctx, out, d); err != nil {
			return nil, err
		}
	} else {
		out.Notes = append(out.Notes, fmt.Sprintf("Tipo de deal '%s' não reconhecido — projeção financeira indisponível", d.Type))
	}

	bps := make([]contracts.BrokerParticipation, 0, len(d.BrokerParticipations))
	for _, bp := range d.BrokerParticipations {
		bps = append(bps, contracts.BrokerParticipation{
			BrokerID:   bp.BrokerID,
			Role:       bp.Role,
			Percentage: bp.Percentage,
		})
	}
	planInput := CommissionPlanInput{
		DealType:             d.Type,
		DealValue:            d.Value,
		CommissionPct:        d.CommissionPct,
		BrokerParticipations: bps,
		Rules:                rules,
		TenantSettings:       settings,
	}
	basePlan, err := ResolveCommissionPlan(planInput)
	if err != nil {
		return nil, fmt.Errorf("invalid commission plan: %w", err)
	}
	plan := basePlan
	if s.crossCompany != nil {
		partnership, sourceTenant, err := repositories.ResolveDealPartnership(ctx, s.crossCompany, d.ID, d.PropertyID, tenantID)
		if err != nil {
			return nil, fmt.Errorf("condições financeiras da parceria: %w", err)
		}
		if partnership != nil {
			fees, err := partnershipFees(partnership, sourceTenant, tenantID)
			if err != nil {
				return nil, err
			}
			out.PartnershipFees = fees
			ownerPct, closingPct := partnership.EffectiveCommissionModel()
			if ownerPct < 0 || closingPct < 0 || math.Abs(ownerPct+closingPct-100) > 0.02 {
				return nil, fmt.Errorf("invalid cross-company commission split %.2f/%.2f", ownerPct, closingPct)
			}
			ownerAmountCents := valueobjects.NewMoney(basePlan.TotalAmountCents).Percentage(ownerPct).Cents()
			if ownerAmountCents > 0 {
				out.Commissions = append(out.Commissions, CommissionPreview{
					Role: "empresa_captadora", Percentage: ownerPct,
					AmountCents: ownerAmountCents,
				})
			}
			planInput.CrossCompany = &CrossCompanyPlanInput{
				OwnerPercentage:   ownerPct,
				ClosingPercentage: closingPct,
				RoyaltyPercentage: partnership.RoyaltyPercentage,
				RoyaltyBaseMode:   partnership.EffectiveRoyaltyBase(),
			}
			planInput.CommissionPct = basePlan.CommissionPct * closingPct / 100
			plan, err = ResolveCommissionPlan(planInput)
			if err != nil {
				return nil, fmt.Errorf("invalid closing-company commission plan: %w", err)
			}
			if partnership.IsFranchise() && partnership.RoyaltyPercentage > 0 {
				base := royaltyBaseAmount(partnership.EffectiveRoyaltyBase(), d.Value, float64(basePlan.TotalAmountCents)/100.0, 0)
				royalty := base * partnership.RoyaltyPercentage / 100
				out.Commissions = append(out.Commissions, CommissionPreview{
					Role: "royalty_franquia", Percentage: partnership.RoyaltyPercentage,
					AmountCents: int64(math.Round(royalty * 100)),
				})
			}
			out.Notes = append(out.Notes, fmt.Sprintf("Parceria cross-company aplicada: %.2f%% parceira de origem / %.2f%% fechamento.", ownerPct, closingPct))
		}
	}
	if isRentalDealType(d.Type) {
		s.appendRentTransferPreview(out, d, startDate, endDate, adminFeePct, plan)
	}
	s.appendCommissionPreview(out, d.Value, plan)
	s.computeTotals(out)
	s.buildEconomicComposition(out, d.Value)
	out.Notes = append(out.Notes, "Sem gateway de pagamento — cobranças são criadas em PENDING e quitadas manualmente.")
	return out, nil
}

func (s *FinancialPreviewService) normalizeDates(d *clients.DealDTO) (time.Time, time.Time) {
	var start, end time.Time
	if d.StartDate != nil {
		start = *d.StartDate
	} else {
		start = time.Now()
	}
	if d.EndDate != nil {
		end = *d.EndDate
	} else if isRentalDealType(d.Type) {
		end = start.AddDate(0, previewDefaultLeaseTermMonths, 0)
	} else {
		end = start.AddDate(1, 0, 0)
	}
	if end.Before(start) {
		end = start.AddDate(0, 1, 0)
	}
	return start, end
}

func (s *FinancialPreviewService) appendRentPreview(ctx context.Context, out *FinancialPreviewDTO, d *clients.DealDTO, startDate, endDate time.Time, defaultDueDay int) error {
	firstChargeDate := defaultFirstChargeDate(startDate, defaultDueDay)
	monthlyCents := int64(math.Round(d.Value * 100))
	if monthlyCents <= 0 {
		out.Notes = append(out.Notes, "Valor mensal do deal é zero — nenhuma cobrança será projetada.")
		return nil
	}
	charges, err := s.rentalConfigService.BuildChargesPreview(
		ctx,
		"preview-"+d.ID,
		d.TenantID,
		d.CustomerID,
		d.OwnerID,
		d.PropertyID,
		startDate, endDate,
		firstChargeDate,
		rental_charge_config.SameMonthProRata,
		defaultDueDay,
		monthlyCents,
		nil, // bundled items: o draft de RENT ainda não é editável na esteira — fica para fase futura
	)
	if err != nil {
		return fmt.Errorf("rent preview build failed: %w", err)
	}
	for _, ch := range charges {
		out.Charges = append(out.Charges, chargeToPreview(ch))
	}
	return nil
}

func (s *FinancialPreviewService) appendRentTransferPreview(out *FinancialPreviewDTO, d *clients.DealDTO, startDate, endDate time.Time, adminFeePct float64, plan ResolvedCommissionPlan) {
	monthlyCents := int64(math.Round(d.Value * 100))
	if monthlyCents <= 0 {
		return
	}
	// Itera os mesmos meses que appendRentPreview emite charges para,
	// para que a projeção de Transfer fique 1:1 com o calendário.
	policy := rental_charge_config.DefaultProRataPolicy
	startMonth := time.Date(startDate.Year(), startDate.Month(), 1, 0, 0, 0, 0, startDate.Location())
	endMonth := time.Date(endDate.Year(), endDate.Month(), 1, 0, 0, 0, 0, endDate.Location())
	brokerOutstandingCents := int64(0)
	for _, share := range plan.Shares {
		brokerOutstandingCents += share.AmountCents
	}
	intermediationOutstandingCents := int64(0)
	if len(plan.Shares) > 0 || plan.RetainedAmountCents > 0 {
		intermediationOutstandingCents = plan.TotalAmountCents
	}
	agencyOutstandingCents := intermediationOutstandingCents - brokerOutstandingCents
	if agencyOutstandingCents < 0 {
		agencyOutstandingCents = 0
	}
	carryWasExplained := false
	for m := startMonth; !m.After(endMonth); m = m.AddDate(0, 1, 0) {
		gross := monthlyCents
		// Primeiro e último mês são pro-rata — preserva o ajuste mesmo na
		// projeção do Transfer, para que o operador veja "abril R$ 1.000
		// (gross) → R$ 900 (net) já com taxa adm".
		if sameMonth(m, startMonth) {
			gross = rental_charge_config.CalculateProRataAmount(startDate, monthlyCents, policy)
		} else if sameMonth(m, endMonth) {
			gross = rental_charge_config.CalculateLastMonthAmount(endDate, monthlyCents, policy)
		}
		if gross <= 0 {
			continue
		}
		feeCents := int64(math.Round(float64(gross) * adminFeePct / 100))
		availableAfterAdminCents := gross - feeCents
		brokerFundedCents := minInt64(availableAfterAdminCents, brokerOutstandingCents)
		brokerOutstandingCents -= brokerFundedCents
		remainingCents := availableAfterAdminCents - brokerFundedCents
		agencyIntermediationCents := minInt64(remainingCents, agencyOutstandingCents)
		agencyOutstandingCents -= agencyIntermediationCents
		intermediationCents := brokerFundedCents + agencyIntermediationCents
		net := availableAfterAdminCents - intermediationCents
		if net < 0 {
			net = 0
		}
		carryCents := brokerOutstandingCents + agencyOutstandingCents
		out.Transfers = append(out.Transfers, TransferPreview{
			ReferenceMonth:            m.Format("2006-01"),
			GrossCents:                gross,
			AdminFeePct:               adminFeePct,
			AdminFeeCents:             feeCents,
			IntermediationCents:       intermediationCents,
			CommissionFundedCents:     brokerFundedCents,
			AgencyIntermediationCents: agencyIntermediationCents,
			IntermediationCarryCents:  carryCents,
			NetCents:                  net,
		})
		if carryCents > 0 && !carryWasExplained {
			out.Notes = append(out.Notes, "O primeiro aluguel proporcional não cobre toda a intermediação; o saldo será apropriado automaticamente nas competências seguintes, após a taxa de administração.")
			carryWasExplained = true
		}
	}
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}

func (s *FinancialPreviewService) appendSalePreview(ctx context.Context, out *FinancialPreviewDTO, d *clients.DealDTO) error {
	totalCents := int64(math.Round(d.Value * 100))
	if totalCents <= 0 {
		out.Notes = append(out.Notes, "Valor do deal é zero — nenhuma cobrança será projetada.")
		return nil
	}
	if s.saleBillingPlanService == nil {
		return errors.New("sale billing plan service not configured")
	}

	// Pós-assinatura, o plano materializado é o snapshot autoritativo.
	if out.ContractID != "" {
		plan, err := s.saleBillingPlanService.GetByContractID(ctx, d.TenantID, out.ContractID)
		if err != nil {
			return fmt.Errorf("sale plan lookup failed: %w", err)
		}
		if plan != nil {
			if plan.TotalAmountCents != totalCents {
				return fmt.Errorf("sale plan total %d does not match frozen deal total %d cents", plan.TotalAmountCents, totalCents)
			}
			for _, ch := range s.saleBillingPlanService.BuildChargesPreview(plan) {
				out.Charges = append(out.Charges, chargeToPreview(ch))
			}
			out.Notes = append(out.Notes, fmt.Sprintf("Plano de pagamento materializado (%s).", plan.Mode))
			return nil
		}
	}

	draft, err := s.saleBillingPlanService.GetActiveDraft(ctx, d.TenantID, d.ID)
	if err != nil {
		return fmt.Errorf("sale draft lookup failed: %w", err)
	}
	if draft == nil {
		out.Notes = append(out.Notes, "Nenhum plano de pagamento SALE ativo; a assinatura não criará cobrança automaticamente.")
		return nil
	}
	if draft.TotalAmountCents != totalCents {
		return fmt.Errorf("sale draft total %d does not match frozen deal total %d cents", draft.TotalAmountCents, totalCents)
	}
	plan, err := sale_billing_plan.New(
		d.TenantID, "preview-"+d.ID, d.ID, d.CustomerID, d.OwnerID, d.PropertyID,
		draft.Mode, draft.TotalAmountCents, draft.DownPaymentCents,
		draft.InstallmentAmountCents, draft.InstallmentCount, draft.FirstDueDate,
		draft.RecurrenceIntervalDays, draft.Composite,
	)
	if err != nil {
		return fmt.Errorf("sale preview draft build failed: %w", err)
	}
	for _, ch := range s.saleBillingPlanService.BuildChargesPreview(plan) {
		out.Charges = append(out.Charges, chargeToPreview(ch))
	}
	out.Notes = append(out.Notes, fmt.Sprintf("Prévia baseada no draft ativo (%s).", draft.Mode))
	return nil
}

func (s *FinancialPreviewService) appendCommissionPreview(out *FinancialPreviewDTO, dealValue float64, plan ResolvedCommissionPlan) {
	if dealValue <= 0 {
		return
	}
	// CommissionPct == 0 ainda gera a estrutura de distribuição com
	// valores zerados — o operador vê quais papéis estão configurados
	// e pode ajustar o percentual no painel financeiro.
	if len(plan.Shares) == 0 && plan.RetainedAmountCents <= 0 {
		// Quando o deal não tem broker_participations, projetamos a
		// comissão consolidada (sem quebrar por broker) — espelha o
		// fallback do contract.signed.
		out.Commissions = append(out.Commissions, CommissionPreview{
			BrokerID:    "",
			Role:        fmt.Sprintf("Total %.1f%%", plan.CommissionPct),
			Percentage:  plan.CommissionPct,
			AmountCents: plan.TotalAmountCents,
		})
		return
	}
	for _, sh := range plan.Shares {
		out.Commissions = append(out.Commissions, CommissionPreview{
			BrokerID:    sh.BrokerID,
			Role:        sh.Role,
			Percentage:  sh.Percentage,
			AmountCents: sh.AmountCents,
		})
	}
	if plan.RetainedAmountCents > 0 {
		out.Commissions = append(out.Commissions, CommissionPreview{
			BrokerID:    "",
			Role:        "imobiliaria",
			Percentage:  plan.RetainedPercentage,
			AmountCents: plan.RetainedAmountCents,
			Retained:    true,
		})
	}
}

func (s *FinancialPreviewService) computeTotals(out *FinancialPreviewDTO) {
	for _, c := range out.Charges {
		out.TotalChargesCents += c.AmountCents
	}
	for _, c := range out.Commissions {
		out.TotalCommissionCents += c.AmountCents
	}
	for _, t := range out.Transfers {
		out.TotalTransfersCents += t.NetCents
	}
}

func (s *FinancialPreviewService) buildEconomicComposition(out *FinancialPreviewDTO, dealValue float64) {
	if out == nil {
		return
	}
	if isSaleDealType(out.DealType) {
		grossCents := int64(math.Round(dealValue * 100))
		var commissionPool, brokerCents, partnerCents int64
		for _, commission := range out.Commissions {
			// Royalty is a downstream obligation of the agency's commission,
			// not another deduction from the seller's sale proceeds.
			if commission.Role == "royalty_franquia" {
				partnerCents += commission.AmountCents
				continue
			}
			commissionPool += commission.AmountCents
			if commission.Role == "empresa_captadora" {
				partnerCents += commission.AmountCents
				continue
			}
			if commission.Retained || commission.BrokerID == "" {
				continue
			}
			brokerCents += commission.AmountCents
		}
		thirdParty := brokerCents + partnerCents
		agencyRetained := commissionPool - thirdParty
		if agencyRetained < 0 {
			agencyRetained = 0
		}
		ownerCents := grossCents - commissionPool
		if ownerCents < 0 {
			ownerCents = 0
		}
		out.Composition = &EconomicCompositionPreview{
			Flow:                   "sale",
			GrossCents:             grossCents,
			CommissionPoolCents:    commissionPool,
			CommissionPayableCents: brokerCents,
			PartnerPayableCents:    partnerCents,
			AgencyRevenueCents:     agencyRetained,
			AgencyRetainedCents:    agencyRetained,
			OwnerPayableCents:      ownerCents,
			AvailableCents:         ownerCents,
		}
		return
	}
	if !isRentalDealType(out.DealType) {
		return
	}
	var grossCents, adminFeeCents, intermediationCents, commissionFundedCents, agencyIntermediationCents, ownerCents int64
	for _, transfer := range out.Transfers {
		grossCents += transfer.GrossCents
		adminFeeCents += transfer.AdminFeeCents
		intermediationCents += transfer.IntermediationCents
		commissionFundedCents += transfer.CommissionFundedCents
		agencyIntermediationCents += transfer.AgencyIntermediationCents
		ownerCents += transfer.NetCents
	}
	if grossCents == 0 && ownerCents == 0 {
		return
	}
	otherDeductions := grossCents - adminFeeCents - intermediationCents - ownerCents
	if otherDeductions < 0 {
		otherDeductions = 0
	}
	out.Composition = &EconomicCompositionPreview{
		Flow:                   "rent",
		GrossCents:             grossCents,
		CommissionPayableCents: commissionFundedCents,
		AgencyRevenueCents:     adminFeeCents + intermediationCents,
		AdminFeeCents:          adminFeeCents,
		IntermediationCents:    intermediationCents,
		CommissionFundedCents:  commissionFundedCents,
		AgencyRetainedCents:    adminFeeCents + agencyIntermediationCents,
		OwnerPayableCents:      ownerCents,
		AvailableCents:         ownerCents,
		DeductionsCents:        adminFeeCents + intermediationCents + otherDeductions,
	}
}

func chargeToPreview(c *charge.Charge) ChargePreview {
	desc := c.Description
	if desc == "" {
		desc = string(c.Type)
	}
	return ChargePreview{
		SourceEventID:  c.SourceEventID,
		Type:           string(c.Type),
		Description:    strings.TrimSpace(desc),
		AmountCents:    c.TotalAmount.Cents(),
		DueDate:        c.DueDate,
		ReferenceMonth: c.ReferenceMonth,
	}
}
