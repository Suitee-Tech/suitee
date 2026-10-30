package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
)

// ErrPayoutRecomputeDisabled prevents the legacy charge recompute flow from
// cancelling orders without being able to recreate them. Payout obligations
// are now provisioned from contract.signed, not from charge settlement.
var ErrPayoutRecomputeDisabled = errors.New("payout recompute by charge is disabled; payouts are provisioned from contract.signed")

// PayoutOrchestrationService is invoked when a charge transitions to PAID
// and is responsible for materializing the split into payout_orders. It is
// idempotent on (charge_id, recipient_id): re-running the orchestration
// for the same charge does not create duplicate orders.
type PayoutOrchestrationService struct {
	repo                 payout_order.Repository
	chargeRepo           charge.RepositoryInterface
	dealClient           *clients.DealClient
	partnerRoyaltyRepo   payoutOrchestrationRoyaltyLookup
	commissionService    *CommissionService
	rentalIntermediation *RentalIntermediationService
}

// payoutOrchestrationRoyaltyLookup is the narrow surface this service
// needs from the partner_royalty repository. Defining it locally keeps the
// orchestration testable against fakes without coupling to the full repo.
type payoutOrchestrationRoyaltyLookup interface {
	ListPendingByPayerAndContract(ctx context.Context, payerTenant, dealID, contractID string) ([]map[string]any, error)
}

func NewPayoutOrchestrationService(repo payout_order.Repository, chargeRepo charge.RepositoryInterface, dealClient *clients.DealClient) *PayoutOrchestrationService {
	return &PayoutOrchestrationService{repo: repo, chargeRepo: chargeRepo, dealClient: dealClient}
}

func (s *PayoutOrchestrationService) SetRoyaltyLookup(l payoutOrchestrationRoyaltyLookup) {
	s.partnerRoyaltyRepo = l
}

func (s *PayoutOrchestrationService) SetCommissionService(service *CommissionService) {
	s.commissionService = service
}

func (s *PayoutOrchestrationService) SetRentalIntermediation(service *RentalIntermediationService) {
	s.rentalIntermediation = service
}

// OnChargePaid is the main entry point. Wired from the InvoicePaidEvent
// handler (which fires when ChargeService.MarkPaid settles a charge).
// Best-effort: a partial failure logs and continues. The legacy charge-based
// recompute path is disabled because charge settlement no longer recreates the
// contract.signed payout obligations.
func (s *PayoutOrchestrationService) OnChargePaid(ctx context.Context, tenantID, chargeID string) error {
	if tenantID == "" || chargeID == "" {
		return nil
	}
	c, err := s.chargeRepo.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if c == nil || c.Status != charge.StatusPaid {
		return nil
	}

	splits, err := s.computeSplits(ctx, c)
	if err != nil {
		return err
	}
	for _, sp := range splits {
		order, err := payout_order.New(tenantID, payout_order.SourceCharge, chargeID, sp.RecipientType, sp.RecipientID, sp.RecipientLabel, sp.AmountCents, "system")
		if err != nil {
			logger.FromContext(ctx).Warn("payout: skip invalid split", "error", err, "charge_id", chargeID, "recipient", sp.RecipientID)
			continue
		}
		order.ContractID = c.ContractID
		order.SourceRef = map[string]any{
			"deal_id":         c.DealID,
			"reference_month": c.ReferenceMonth,
			"charge_type":     string(c.Type),
			"amount_paid":     c.PaidAmount,
			"split_pct":       sp.Percentage,
		}
		order.SourceEventID = events.DeriveNotificationID("payout_order", tenantID, chargeID, string(sp.RecipientType), sp.RecipientID, sp.RecipientLabel)
		if err := s.repo.Create(ctx, order); err != nil {
			if errors.Is(err, payout_order.ErrPayoutOrderAlreadyExists) {
				continue
			}
			return fmt.Errorf("payout order persist failed: %w", err)
		}
	}
	return nil
}

// OnContractSigned materializa o payout_order de comissão no momento da
// assinatura do contrato, para o modelo manual de provisionamento (sem
// cobrança, sem gateway). Espelha a leg de comissão do OnChargePaid mas
// usa o valor do contrato (payload contract.signed) em vez de uma charge
// paga. Idempotente por SourceEventID. SourceType=manual com o contrato
// como origem — o operador tria como qualquer outro payout e liquida via
// /payout-orders/:id/approve + /mark-paid.
func (s *PayoutOrchestrationService) OnContractSigned(ctx context.Context, in ContractFinancialsInput) error {
	p := in.Payload
	if p == nil || p.TenantID == "" || p.ContractID == "" {
		return nil
	}

	// Use o plano canônico já resolvido pelo BrokerCommissionHandler.
	// O plano é frozen com PlanHash consistente com o preview. Só
	// re-resolve como fallback quando não foi populado (ex: legacy
	// path sem broker handler).
	plan := in.CommissionPlan
	if plan == nil || plan.IsZero() {
		resolved, err := ResolveCommissionPlan(CommissionPlanInput{
			DealType:             p.Type,
			DealValue:            p.Value,
			CommissionPct:        p.CommissionPct,
			BrokerParticipations: p.BrokerParticipations,
		})
		if err != nil {
			return fmt.Errorf("resolve payout commission plan: %w", err)
		}
		plan = &resolved
	}
	commissionCents := plan.TotalAmountCents
	if p.Value > 0 && plan.CommissionPct > 0 && commissionCents > 0 && len(plan.Shares) > 0 {
		// Snapshot já materializado pelo wizard (commit pré-assinatura): os
		// payout_orders de assinatura já foram provisionados com o mesmo
		// plan hash. Uma segunda execução (contract.signed após o wizard, ou
		// re-run do materializer) não pode duplicar os orders por um EventID
		// distinto. Royalties de franquia permanecem fora do guard (idp.).
		payoutSnapshot, err := s.hasSigningPayoutSnapshot(ctx, p.TenantID, p.ContractID, p.DealID, plan.PlanHash)
		if err != nil {
			return err
		}
		if payoutSnapshot {
			log.Printf("[INFO] deal %s contract %s already has the materialized signing payouts (plan_hash=%s); skipping payout provisioning",
				p.DealID, p.ContractID, plan.PlanHash)
		} else {
			for _, share := range plan.Shares {
				if share.BrokerID == "" || share.Percentage <= 0 {
					continue
				}
				amount := share.AmountCents
				if amount <= 0 {
					continue
				}
				label := fmt.Sprintf("Comissão %s (%.1f%%)", share.Role, share.Percentage)
				linkRole := strings.ToLower(strings.TrimSpace(share.Role))
				sourceEventID := events.DeriveNotificationID(
					in.EventID, "commission_payout", p.ContractID,
					string(payout_order.RecipientBroker), share.BrokerID, linkRole,
				)
				if err := s.createSigningPayout(ctx, p.TenantID, p.ContractID, p.DealID, p.Type,
					payout_order.RecipientBroker, share.BrokerID, label, amount, share.Percentage,
					sourceEventID, linkRole, plan.PlanHash); err != nil {
					return err
				}
			}
		}
	}
	if settlement := in.CrossCompany; settlement != nil && settlement.RecipientTenant != "" && settlement.OwnerAmountCents > 0 {
		amount := settlement.OwnerAmountCents
		label := fmt.Sprintf("Comissão parceria (%.1f%%)", settlement.OwnerPercentage)
		sourceEventID := events.DeriveNotificationID(
			"partnership_commission_payout", p.DealID,
			string(payout_order.RecipientPartnerTenant), settlement.RecipientTenant, "cross_company",
		)
		if err := s.createSigningPayout(ctx, p.TenantID, p.ContractID, p.DealID, p.Type,
			payout_order.RecipientPartnerTenant, settlement.RecipientTenant, label, amount, settlement.OwnerPercentage,
			sourceEventID, "", plan.PlanHash); err != nil {
			return err
		}
	}

	// Provision one royalty_settlement payout per franchise royalty recorded
	// for this contract by the cross-company step (which runs before this in the
	// chain). This leg is independent of broker commission: royalties based on
	// deal value remain payable even when the contract commission is zero.
	return s.provisionRoyaltiesAtSigning(ctx, in)
}

// hasSigningPayoutSnapshot reports whether signing payout orders for the
// same deal + frozen plan hash already exist. Both the wizard materializer
// (pre-signature plan commit) and the deal.contract.signed pipeline call
// this guard so neither trigger can duplicate the broker payout orders.
//
// A consulta é escopada no NEGÓCIO, não no contrato. O plan hash é a identidade
// econômica do plano e não contém o contrato; escopar a consulta por contrato
// fazia as ordens já materializadas pelo contrato original ficarem invisíveis
// quando o mesmo negócio era assinado de novo por um aditivo, e cada aditivo
// provisionava uma segunda ordem de pagamento ao mesmo corretor — dobrando um
// valor a pagar. O mesmo defeito existia na guarda de comissões.
func (s *PayoutOrchestrationService) hasSigningPayoutSnapshot(ctx context.Context, tenantID, contractID, dealID, planHash string) (bool, error) {
	if s == nil || s.repo == nil || strings.TrimSpace(dealID) == "" || strings.TrimSpace(planHash) == "" {
		return false, nil
	}
	orders, _, err := s.repo.List(ctx, tenantID, payout_order.ListFilter{
		DealID:        dealID,
		RecipientType: string(payout_order.RecipientBroker),
		Limit:         1000,
		Offset:        0,
	})
	if err != nil {
		return false, fmt.Errorf("list signing payouts for materialization guard: %w", err)
	}
	for _, order := range orders {
		if order == nil {
			continue
		}
		if ref, ok := order.SourceRef["deal_id"].(string); !ok || ref != dealID {
			continue
		}
		if ref, ok := order.SourceRef["trigger"].(string); !ok || ref != "contract.signed" {
			continue
		}
		if ref, ok := order.SourceRef["plan_hash"].(string); !ok || ref != planHash {
			continue
		}
		if order.ContractID != contractID {
			log.Printf("[INFO] signing payouts for plan_hash=%s already materialized by contract %s; contract %s (deal %s) reuses them",
				planHash, order.ContractID, contractID, dealID)
		}
		return true, nil
	}
	return false, nil
}

func (s *PayoutOrchestrationService) createSigningPayout(
	ctx context.Context,
	tenantID, contractID, dealID, dealType string,
	recipientType payout_order.RecipientType,
	recipientID, label string,
	amountCents int64,
	splitPct float64,
	sourceEventID, linkRole, planHash string,
) error {
	if recipientType == payout_order.RecipientPartnerTenant {
		orders, total, err := s.repo.List(ctx, tenantID, payout_order.ListFilter{DealID: dealID, RecipientType: string(recipientType), RecipientID: recipientID, Limit: 1000})
		if err != nil {
			return fmt.Errorf("consultar comissão de parceria existente: %w", err)
		}
		if total > 1000 {
			return fmt.Errorf("histórico de comissões de parceria excede o limite de revisão")
		}
		matched := 0
		for _, old := range orders {
			if old == nil || old.SourceRef["trigger"] != "contract.signed" {
				continue
			}
			if old.AmountCents != amountCents {
				return fmt.Errorf("comissão de parceria divergente: reconcilie o histórico antes de provisionar")
			}
			matched++
		}
		if matched > 1 {
			return fmt.Errorf("comissões de parceria duplicadas: reconcilie o histórico antes de provisionar")
		}
		if matched == 1 {
			return nil
		}
	}
	linkedCommissionID := ""
	if recipientType == payout_order.RecipientBroker {
		if s.commissionService == nil {
			return fmt.Errorf("commission service is required for broker payout")
		}
		if dealID == "" || recipientID == "" {
			return fmt.Errorf("deal and broker identities are required for broker payout")
		}
		commissions, err := s.commissionService.ListByDeal(ctx, tenantID, dealID)
		if err != nil {
			return fmt.Errorf("commission lookup for payout link failed: %w", err)
		}
		for _, linkedCommission := range commissions {
			commissionRole := strings.ToLower(strings.TrimSpace(linkedCommission.Role))
			if linkedCommission.ContractID == contractID && linkedCommission.BrokerID == recipientID && (linkRole == "" || commissionRole == linkRole) {
				linkedCommissionID = linkedCommission.ID
				break
			}
		}
		if linkedCommissionID == "" {
			return fmt.Errorf("commission not found for broker payout contract=%s broker=%s role=%s", contractID, recipientID, linkRole)
		}
	}

	order, err := payout_order.New(tenantID, payout_order.SourceManual, contractID, recipientType, recipientID, label, amountCents, "system")
	if err != nil {
		return err
	}
	order.ContractID = contractID
	if recipientType == payout_order.RecipientBroker && (isRentalDealType(dealType) || isSaleDealType(dealType)) {
		if _, err := order.EnableFundingGate(amountCents); err != nil {
			return fmt.Errorf("configure payout funding gate: %w", err)
		}
	}
	order.SourceRef = map[string]any{
		"deal_id": dealID, "charge_type": dealType, "split_pct": splitPct, "trigger": "contract.signed",
	}
	if strings.TrimSpace(planHash) != "" {
		order.SourceRef["plan_hash"] = planHash
	}
	// The identity of this obligation comes from the immutable delivery and
	// beneficiary, never from label/percentage/amount. A retry after rules
	// change must resolve the original order instead of creating a new debt.
	order.SourceEventID = sourceEventID

	// Compatibility bridge: releases before FIN-COM-07 keyed signing payouts
	// with the mutable label. Reuse that row when present so a redrive across
	// the deployment boundary cannot create a second obligation.
	legacySourceEventID := events.DeriveNotificationID(
		"payout_order", tenantID, contractID, string(recipientType), recipientID, label,
	)
	persistedOrder, err := s.repo.GetBySourceEventID(ctx, tenantID, legacySourceEventID)
	if err != nil {
		return fmt.Errorf("legacy payout order lookup failed: %w", err)
	}
	if persistedOrder == nil {
		persistedOrder = order
		if err := s.repo.Create(ctx, order); err != nil {
			if !errors.Is(err, payout_order.ErrPayoutOrderAlreadyExists) {
				return fmt.Errorf("payout order persist failed: %w", err)
			}

			// Create generated a fresh UUID before the unique source_event_id
			// constraint detected this delivery as a retry. Resolve the aggregate
			// that actually won the first insert so Commission never links to the
			// discarded in-memory UUID.
			persistedOrder, err = s.repo.GetBySourceEventID(ctx, tenantID, order.SourceEventID)
			if err != nil {
				return fmt.Errorf("existing payout order lookup failed: %w", err)
			}
			if persistedOrder == nil || persistedOrder.ID == "" {
				return fmt.Errorf("existing payout order not found for source_event_id %s", order.SourceEventID)
			}
		}
	}
	if recipientType == payout_order.RecipientBroker && (isRentalDealType(dealType) || isSaleDealType(dealType)) {
		persistedOrder, err = s.ensurePayoutFundingGate(ctx, persistedOrder, amountCents)
		if err != nil {
			return err
		}
	}
	if linkedCommissionID != "" {
		if _, err := s.commissionService.LinkPayoutOrder(ctx, tenantID, linkedCommissionID, persistedOrder.ID); err != nil {
			return fmt.Errorf("commission payout link failed: %w", err)
		}
		if isRentalDealType(dealType) {
			if s.rentalIntermediation == nil {
				return fmt.Errorf("rental intermediation service is required for broker payout")
			}
			if err := s.rentalIntermediation.LinkPayout(ctx, tenantID, contractID, recipientID, linkRole, persistedOrder.ID); err != nil {
				return fmt.Errorf("rental payout funding link failed: %w", err)
			}
		}
	}
	return nil
}

func (s *PayoutOrchestrationService) ensurePayoutFundingGate(ctx context.Context, order *payout_order.PayoutOrder, requiredCents int64) (*payout_order.PayoutOrder, error) {
	expectedStatus := order.Status
	expectedUpdatedAt := order.UpdatedAt
	changed, err := order.EnableFundingGate(requiredCents)
	if err != nil {
		return nil, fmt.Errorf("payout %s requires historical reconciliation: %w", order.ID, err)
	}
	if !changed {
		return order, nil
	}
	repo, ok := s.repo.(payout_order.TransitionRepository)
	if !ok {
		return nil, payout_order.ErrTransitionCompareAndSetMissing
	}
	if err := repo.UpdateIfStatus(ctx, order, expectedStatus, expectedUpdatedAt); err != nil {
		return nil, err
	}
	return order, nil
}

// provisionRoyaltiesAtSigning emits one royalty_settlement payout per
// partner_royalty recorded for the deal, using the one-time accrued amount
// (base * pct). Idempotent by SourceEventID. No-op when the royalty lookup is
// not wired or there are no royalties. Once wired, lookup and persistence
// failures are fail-fast so the enclosing contract transaction can roll back
// every payout leg together. Using the accrued amount (not a per-charge slice)
// keeps the royalty a single correct obligation, settleable via the payout
// rails and visible in the inbox's royalty bucket.
func (s *PayoutOrchestrationService) provisionRoyaltiesAtSigning(ctx context.Context, in ContractFinancialsInput) error {
	p := in.Payload
	if s.partnerRoyaltyRepo == nil && in.CrossCompany != nil && in.CrossCompany.FeePayerTenant != "" {
		return fmt.Errorf("consulta de obrigações de parceria obrigatória para provisionar as taxas")
	}
	if s.partnerRoyaltyRepo == nil || p == nil || p.TenantID == "" || p.DealID == "" || p.ContractID == "" {
		return nil
	}
	payers := []string{p.TenantID}
	if in.CrossCompany != nil && in.CrossCompany.FeePayerTenant != "" && in.CrossCompany.FeePayerTenant != p.TenantID {
		payers = append(payers, in.CrossCompany.FeePayerTenant)
	}
	for _, payerTenant := range payers {
		royalties, err := s.partnerRoyaltyRepo.ListPendingByPayerAndContract(ctx, payerTenant, p.DealID, p.ContractID)
		if err != nil {
			return fmt.Errorf("royalty payout lookup failed: %w", err)
		}
		for _, r := range royalties {
			royaltyID := asString(r["id"])
			payee := asString(r["payee_tenant"])
			amount := asFloat(r["amount"])
			if royaltyID == "" || payee == "" || amount <= 0 {
				return fmt.Errorf("invalid pending royalty payout row for contract %s", p.ContractID)
			}
			cents := int64(math.Round(amount * 100))
			if cents <= 0 {
				return fmt.Errorf("pending royalty %s rounds to a non-payable amount", royaltyID)
			}
			label := fmt.Sprintf("Royalty %s", payee)
			kind := asString(r["kind"])
			if kind == "admin_fee" {
				label = "Taxa administrativa da parceria"
			} else if kind == "transaction_fee" {
				label = "Taxa por negócio da parceria"
			}
			order, err := payout_order.New(payerTenant, payout_order.SourceRoyaltySettlement, royaltyID, payout_order.RecipientRoyaltyPayee, payee, label, cents, "system")
			if err != nil {
				return fmt.Errorf("build royalty payout %s: %w", royaltyID, err)
			}
			order.ContractID = p.ContractID
			order.SourceRef = map[string]any{
				"obligation_kind": kind,
				"royalty_id":      royaltyID,
				"deal_id":         p.DealID,
				"percentage":      asFloat(r["percentage"]),
				"trigger":         "contract.signed",
			}
			order.SourceEventID = events.DeriveNotificationID(
				"payout_order", payerTenant, p.ContractID,
				string(payout_order.RecipientRoyaltyPayee), payee, royaltyID,
			)
			if err := s.repo.Create(ctx, order); err != nil {
				if errors.Is(err, payout_order.ErrPayoutOrderAlreadyExists) {
					continue
				}
				return fmt.Errorf("persist royalty payout %s: %w", royaltyID, err)
			}
		}
	}
	return nil
}

// RecomputeForCharge is deliberately disabled. The former implementation
// cancelled every non-terminal charge payout and then called OnChargePaid,
// whose split calculation is intentionally empty now that payouts originate
// from contract.signed. Returning before repository access preserves existing
// obligations and surfaces an explicit error to the admin endpoint.
func (s *PayoutOrchestrationService) RecomputeForCharge(ctx context.Context, tenantID, chargeID, actor string) (int, error) {
	if tenantID == "" || chargeID == "" {
		return 0, nil
	}
	return 0, ErrPayoutRecomputeDisabled
}

// splitLine is the in-memory representation of one computed split. Kept
// local so the public surface only exposes the orchestration verbs and not the
// calculation internals.
type splitLine struct {
	RecipientType  payout_order.RecipientType
	RecipientID    string
	RecipientLabel string
	Percentage     float64
	AmountCents    int64
}

// computeSplits is where the business rules of "who gets paid when this
// charge clears" live. Today it covers:
//
//  1. Broker commissions: read from the contract's broker_participations
//     via the DealClient. Each broker gets contract.commission_pct *
//     participation_pct of the paid amount.
//  2. Partner royalties: any PENDING partner_royalty row tied to this
//     deal contributes one line to the royalty payee.
//
// Both legs are guarded by nil-checks so a partial wiring (e.g. royalty
// lookup not injected) still produces the broker commissions. The
// rounding residual is tracked so the operator can audit ", restante R$
// 0,02 não distribuído".
func (s *PayoutOrchestrationService) computeSplits(ctx context.Context, c *charge.Charge) ([]splitLine, error) {
	out := make([]splitLine, 0, 4)
	paidCents := c.TotalAmount.Cents()
	if c.PaidAmount != nil {
		paidCents = c.PaidAmount.Cents()
	}
	if paidCents <= 0 {
		return out, nil
	}

	// Comissões são provisionadas uma única vez em contract.signed.
	// Uma parcela paga não deve recriar a mesma obrigação.
	_ = ctx
	_ = paidCents

	// Royalties are intentionally NOT split per-charge here. A franchise
	// royalty is a one-time obligation provisioned at contract.signed by
	// provisionRoyaltiesAtSigning as a royalty_settlement payout with the
	// accrued amount (base * pct). Slicing pct of every paid charge would both
	// double-count and use the wrong base, so the charge path emits only the
	// broker commission line above.
	return out, nil
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	}
	return 0
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
