package services

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// commissionLedgerKey identifies a broker share so the ledger transaction and
// the Commission aggregate can be linked deterministically.
func commissionLedgerKey(brokerID, role string) string {
	return strings.TrimSpace(brokerID) + "\x1f" + strings.ToLower(strings.TrimSpace(role))
}

// BrokerCommissionHandler is the broker-distribution step of the
// contract.signed pipeline. It owns the per-broker Transaction ledger
// view (always) and the Commission domain entities (SALE and RENT).
//
// Cross-company partnership financials live in CrossCompanyHandler and
// run as a separate, later step in the chain.
//
// tenantSettings is optional — when set, it is passed to
// ResolveCommissionPlan so the resulting PlanHash matches the preview
// path exactly.
//
// Fail-fast policy:
//   - Transaction ledger writes are fail-fast. O dinheiro precisa
//     estar reconciliado para o relatório financeiro fechar.
//   - Commission entity persistence is fail-fast too: it runs inside the
//     same contract.signed transaction, so a failed entity rollback the
//     already-created ledger row and the delivery is retried instead of
//     leaving a repairable-but-orphaned partial graph.
type BrokerCommissionHandler struct {
	txService              *TransactionService
	chargeService          *ChargeService
	journalService         *JournalService
	commissionService      *CommissionService
	commissionRulesService *CommissionRulesService
	tenantSettings         *TenantFinancialSettingsService
	rentalIntermediation   *RentalIntermediationService
}

func (h *BrokerCommissionHandler) WithRentalIntermediation(s *RentalIntermediationService) *BrokerCommissionHandler {
	h.rentalIntermediation = s
	return h
}

// WithChargeService lets the commission step freeze the SALE waterfall on
// the receivables created immediately before it. The dependency is optional
// for focused tests and legacy deployments without charge persistence.
func (h *BrokerCommissionHandler) WithChargeService(s *ChargeService) *BrokerCommissionHandler {
	h.chargeService = s
	return h
}

// NewBrokerCommissionHandler builds the per-broker step. txService is
// required; commissionService, commissionRulesService and tenantSettings are
// optional and degrade to the historical behaviour when nil.
func NewBrokerCommissionHandler(
	txService *TransactionService,
	commissionService *CommissionService,
	commissionRulesService *CommissionRulesService,
) *BrokerCommissionHandler {
	return &BrokerCommissionHandler{
		txService:              txService,
		commissionService:      commissionService,
		commissionRulesService: commissionRulesService,
	}
}

// WithTenantSettings injects the tenant settings service so the handler can
// pass the same defaults to ResolveCommissionPlan that the preview uses. This
// produces identical PlanHash values between preview and contract.signed.
func (h *BrokerCommissionHandler) WithTenantSettings(s *TenantFinancialSettingsService) *BrokerCommissionHandler {
	h.tenantSettings = s
	return h
}

// WithJournalService injeta o ledger de dupla entrada. Quando nil, o handler
// degrada ao comportamento legado (apenas o espelho single-entry Transaction).
func (h *BrokerCommissionHandler) WithJournalService(s *JournalService) *BrokerCommissionHandler {
	h.journalService = s
	return h
}

// Handle emits per-broker commissions for the deal. A Commission-entity
// persistence failure is fatal: inside the contract.signed transaction it
// rolls back the ledger rows so the event can be redelivered atomically.
func (h *BrokerCommissionHandler) Handle(ctx context.Context, in ContractFinancialsInput) error {
	if h.txService == nil {
		return fmt.Errorf("broker commission handler requires a TransactionService")
	}
	p := in.Payload
	rules, err := h.loadRules(ctx, p.TenantID)
	if err != nil {
		return err
	}
	settings, err := h.loadSettings(ctx, p.TenantID)
	if err != nil {
		return err
	}

	// Audit: CommissionPct == 0 is a data-quality issue. The contract was
	// signed but no commission percentage was defined — this means zero
	// commission is distributed to brokers. The audit entry surfaces the
	// gap without blocking the pipeline.
	if p.CommissionPct == 0 {
		audit.Record(ctx, audit.Entry{
			Action:     "commission.zero_pct",
			EntityType: "Deal",
			EntityID:   p.DealID,
			Metadata: map[string]any{
				"contract_id": p.ContractID,
				"tenant_id":   p.TenantID,
				"value":       p.Value,
			},
		})
	}

	if len(p.BrokerParticipations) == 0 {
		// FIN-COM-09: sem beneficiário real não geramos transação sintética
		// de comissão. A distribuição só é materializada quando o deal
		// informa participantes reais. O contrato foi assinado sem alocação
		// de corretores — estado válido que será resolvido posteriormente.
		//
		// A comissão não fica sem destino: sem corretor, o plano retém 100% na
		// imobiliária (ResolveCommissionPlan) e a composição da venda nasce com
		// o pool inteiro como receita da imobiliária. Os valores ficam no audit
		// para a operação enxergar o que foi retido por falta de rateio.
		plan, err := ResolveCommissionPlan(h.commissionPlanInput(in, rules, settings))
		if err != nil {
			return fmt.Errorf("resolve consolidated commission plan: %w", err)
		}
		h.storePlan(in, plan)
		if err := h.freezeSaleComposition(ctx, in, plan); err != nil {
			return err
		}
		audit.Record(ctx, audit.Entry{
			Action:     "commission.no_participants",
			EntityType: "Deal",
			EntityID:   p.DealID,
			Metadata: map[string]any{
				"contract_id":     p.ContractID,
				"tenant_id":       p.TenantID,
				"value":           p.Value,
				"plan_hash":       plan.PlanHash,
				"retained_pct":    plan.RetainedPercentage,
				"retained_amount": float64(plan.RetainedAmountCents) / 100.0,
			},
		})
		log.Printf("[INFO] contract %s signed without broker_participations — commission deferred until beneficiaries are configured (plan_hash=%s)",
			p.ContractID, plan.PlanHash)
		return nil
	}

	plan, err := ResolveCommissionPlan(h.commissionPlanInput(in, rules, settings))
	if err != nil {
		return fmt.Errorf("resolve commission plan: %w", err)
	}
	h.storePlan(in, plan)
	// Snapshot já materializado (ex.: wizard commit materializou o plano
	// antes da assinatura): a redelivery do contract.signed ou uma re-exec
	// do materializer não pode duplicar ledger/comissões/payouts do mesmo
	// plan hash. A composição econômica da charge já foi congelada e é
	// idempotente (ApplySaleEconomicComposition valida match).
	materialized, err := h.hasMaterializedSnapshot(ctx, p.TenantID, p.ContractID, p.DealID, plan.PlanHash)
	if err != nil {
		return err
	}
	if materialized {
		log.Printf("[INFO] deal %s contract %s already has the materialized commission snapshot (plan_hash=%s); skipping broker step",
			p.DealID, p.ContractID, plan.PlanHash)
		return nil
	}
	if err := h.freezeSaleComposition(ctx, in, plan); err != nil {
		return err
	}
	if err := h.freezeRentalIntermediation(ctx, in, plan); err != nil {
		return err
	}
	shares := plan.Shares
	commissionAmount := float64(plan.TotalAmountCents) / 100.0

	// A distribuição sempre fecha 100%: o resíduo sem corretor vira retenção da
	// imobiliária (ResolveCommissionPlan). Quando o rateio INFORMADO não fechava,
	// a operação precisa enxergar isso — o negócio foi assinado com distribuição
	// incompleta e a diferença ficou retida.
	var informedPct float64
	for _, bp := range p.BrokerParticipations {
		informedPct += bp.Percentage
	}
	filledPct := 100 - informedPct
	if len(p.BrokerParticipations) > 0 && filledPct > 0.02 {
		log.Printf("[WARN] commission distribution for deal %s was incomplete (informed %.2f%%); %.2f%% stayed with the agency as retention",
			p.DealID, informedPct, filledPct)
		audit.Record(ctx, audit.Entry{
			Action:     "commission.split_filled_by_retention",
			EntityType: "Deal",
			EntityID:   p.DealID,
			Metadata: map[string]any{
				"contract_id":       p.ContractID,
				"tenant_id":         p.TenantID,
				"informed_pct":      informedPct,
				"filled_pct":        filledPct,
				"retained_pct":      plan.RetainedPercentage,
				"retained_amount":   float64(plan.RetainedAmountCents) / 100.0,
				"commission_pct":    p.CommissionPct,
				"commission_amount": commissionAmount,
			},
		})
	}

	ledgerIDs, err := h.emitLedgerForShares(ctx, in.EventID, p.ContractID, p.TenantID, shares, plan.PlanHash)
	if err != nil {
		return err
	}
	if h.commissionService != nil {
		if err := h.persistCommissionEntities(ctx, in, shares, plan.PlanHash, ledgerIDs); err != nil {
			return err
		}
	}
	// FIN-COM-10: Party.SharePct representa participação societária
	// (proprietário, comprador, cônjuge), não comissão de corretagem.
	// Comissões são geradas exclusivamente via broker_participations.
	// Participações societárias são tratadas no fluxo de repasse (transfer),
	// não no fluxo de comissão.
	return nil
}

func (h *BrokerCommissionHandler) freezeRentalIntermediation(ctx context.Context, in ContractFinancialsInput, plan ResolvedCommissionPlan) error {
	if !isRentalDealType(in.Payload.Type) || plan.TotalAmountCents <= 0 {
		return nil
	}
	if h.rentalIntermediation == nil {
		return nil
	}
	if err := h.rentalIntermediation.FreezePlan(
		ctx, in.Payload.TenantID, in.Payload.ContractID, in.Payload.DealID, plan,
	); err != nil {
		return fmt.Errorf("freeze rental intermediation plan: %w", err)
	}
	return nil
}

// loadRules reads the tenant snapshot once so ledger, Commission and payout all
// receive values produced from the same rules. A lookup failure aborts the
// financial chain: silently freezing a fallback plan would make preview,
// ledger and payout disagree after a transient database outage.
func (h *BrokerCommissionHandler) loadRules(ctx context.Context, tenantID string) ([]commission.Rule, error) {
	if h.commissionRulesService != nil {
		rules, err := h.commissionRulesService.Get(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("load commission rules for tenant %s: %w", tenantID, err)
		}
		return rules, nil
	}
	return nil, nil
}

// loadSettings reads the tenant financial settings once so the plan hash
// matches the preview path. A configured service must be authoritative;
// fallback defaults are only valid when the dependency is intentionally nil.
func (h *BrokerCommissionHandler) loadSettings(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error) {
	if h.tenantSettings != nil {
		settings, err := h.tenantSettings.Get(ctx, tenantID)
		if err != nil {
			return nil, fmt.Errorf("load financial settings for tenant %s: %w", tenantID, err)
		}
		return settings, nil
	}
	return nil, nil
}

func (h *BrokerCommissionHandler) storePlan(in ContractFinancialsInput, plan ResolvedCommissionPlan) {
	if in.CommissionPlan != nil {
		*in.CommissionPlan = plan
	}
}

func (h *BrokerCommissionHandler) freezeSaleComposition(ctx context.Context, in ContractFinancialsInput, plan ResolvedCommissionPlan) error {
	if h.chargeService == nil || !isSaleDealType(in.Payload.Type) {
		return nil
	}
	partnerCents := int64(0)
	partnerID := ""
	royaltyCents := int64(0)
	royaltyPayeeID := ""
	if in.CrossCompany != nil {
		partnerCents = in.CrossCompany.OwnerAmountCents
		partnerID = in.CrossCompany.RecipientTenant
		royaltyCents = in.CrossCompany.RoyaltyAmountCents
		royaltyPayeeID = in.CrossCompany.RoyaltyPayeeTenant
	}
	if err := h.chargeService.ApplySaleEconomicComposition(ctx, in.Payload.TenantID, in.Payload.ContractID, plan, partnerCents, partnerID, royaltyCents, royaltyPayeeID); err != nil {
		return fmt.Errorf("freeze SALE economic composition: %w", err)
	}
	return nil
}

// commissionPlanInput builds the resolver input from the (already cross-company
// adjusted) payload plus the partnership context published by the cross-company
// step. Folding the owner/closing/royalty context into the input keeps the
// resulting PlanHash identical to the preview path.
func (h *BrokerCommissionHandler) commissionPlanInput(in ContractFinancialsInput, rules []commission.Rule, settings *tenant_financial_settings.Settings) CommissionPlanInput {
	p := in.Payload
	planInput := CommissionPlanInput{
		DealType:             p.Type,
		DealValue:            p.Value,
		CommissionPct:        p.CommissionPct,
		BrokerParticipations: p.BrokerParticipations,
		Rules:                rules,
		TenantSettings:       settings,
	}
	// The event always carries an empty settlement container. Only a resolved
	// partnership belongs in the fingerprint, as in the preview path.
	if in.CrossCompany != nil && in.CrossCompany.PartnershipID != "" {
		planInput.CrossCompany = &CrossCompanyPlanInput{
			OwnerPercentage:   in.CrossCompany.OwnerPercentage,
			ClosingPercentage: in.CrossCompany.ClosingPercentage,
			RoyaltyPercentage: in.CrossCompany.RoyaltyPercentage,
			RoyaltyBaseMode:   in.CrossCompany.RoyaltyBaseMode,
		}
	}
	return planInput
}

// hasMaterializedSnapshot reports whether a live commission snapshot for the
// deal's frozen plan already exists. Both the wizard materializer
// (pre-signature plan commit) and the deal.contract.signed pipeline call this
// guard so neither trigger can duplicate the broker ledger, journal entries,
// Commission entities or payout orders for the same frozen plan.
//
// A guarda é escopada no DEAL e no plan hash, e não no contrato. O plan hash é
// a identidade econômica do negócio: deriva de tipo, valor, percentual de
// comissão, participações e regras — nada dele vem do contrato. Escopar por
// contrato deixava a comissão já materializada invisível quando o mesmo
// negócio era assinado de novo por um aditivo (contrato novo, plano idêntico),
// e cada assinatura de aditivo criava uma segunda comissão para o mesmo
// corretor: em produção, um negócio com comissões PAID em 09/09 recebeu o mesmo
// par de comissões PENDING em 14/09, ao assinar o aditivo, dobrando o valor a
// pagar. Duas parcelas do mesmo plano (papéis distintos) continuam sendo
// materializadas: cada uma tem seu próprio plan hash.
func (h *BrokerCommissionHandler) hasMaterializedSnapshot(ctx context.Context, tenantID, contractID, dealID, planHash string) (bool, error) {
	if h.commissionService == nil || strings.TrimSpace(dealID) == "" || strings.TrimSpace(planHash) == "" {
		return false, nil
	}
	all, err := h.commissionService.ListByDeal(ctx, tenantID, dealID)
	if err != nil {
		return false, fmt.Errorf("list commissions for materialization guard: %w", err)
	}
	for _, c := range all {
		if c == nil || c.PlanHash != planHash {
			continue
		}
		if c.Status == commission.StatusCancelled || c.Status == commission.StatusReversed {
			continue
		}
		if c.ContractID != contractID {
			// O plano já foi materializado por outro contrato do mesmo negócio
			// (aditivo). Registra o contrato que materializou para a operação
			// conseguir distinguir isso de um plano que nunca foi pago.
			log.Printf("[INFO] commission snapshot for plan_hash=%s already materialized by contract %s; contract %s (deal %s) reuses it",
				planHash, c.ContractID, contractID, dealID)
		}
		return true, nil
	}
	return false, nil
}

// hasLegacySyntheticLedger bridges contract.signed deliveries created before
// FIN-COM-09. Those releases expanded missing participants into
// default:<role> rows. A redrive after removing synthetic beneficiaries must
// not add a consolidated fallback on top of the old rows.
func (h *BrokerCommissionHandler) hasLegacySyntheticLedger(ctx context.Context, eventID, contractID, tenantID string) (bool, error) {
	roles := []commission.Role{
		commission.RoleCaptador,
		commission.RoleCoCaptador,
		commission.RoleVisitador,
		commission.RoleAtendente,
		commission.RoleFechamento,
		commission.RoleImobiliaria,
	}
	for _, role := range roles {
		sourceEventID := events.DeriveNotificationID(
			eventID, "commission_tx", contractID, "default:"+string(role), string(role),
		)
		existing, err := h.txService.GetBySourceEventID(ctx, tenantID, sourceEventID)
		if err != nil {
			return false, err
		}
		if existing != nil {
			return true, nil
		}
	}
	return false, nil
}

// emitLedgerForShares writes one Transaction.TypeComissao row per
// broker share. Fail-fast: a missing ledger row corrupts the report.
// Shares com amount <= 0 (ex: imobiliária com 0% default) são puladas
// para não poluir a tabela financial.transactions com registros zerados.
//
// planHash links the transaction to its resolved commission plan so the
// inbox view can reconcile Transaction ↔ Commission rows unambiguously.
//
// Returns the ledger transaction ID per broker share so the Commission
// aggregate can persist its LedgerTransactionID.
func (h *BrokerCommissionHandler) emitLedgerForShares(ctx context.Context, eventID, contractID, tenantID string, shares []ResolvedCommissionShare, planHash string) (map[string]string, error) {
	ledgerIDs := make(map[string]string)
	for _, share := range shares {
		if share.AmountCents <= 0 {
			continue
		}
		desc := fmt.Sprintf("Comissão - %s (%.1f%%)", share.Role, share.Percentage)
		tx := transaction.New(contractID, transaction.TypeComissao, desc, float64(share.AmountCents)/100.0, time.Now(), tenantID)
		tx.Category = "Comissão"
		tx.PlanHash = planHash
		tx.BrokerID = share.BrokerID
		tx.Role = string(share.Role)
		sourceEventID := events.DeriveNotificationID(eventID, "commission_tx", contractID, share.BrokerID, share.Role)
		// Deterministic primary key so the Commission aggregate can reference
		// its ledger row (LedgerTransactionID) across idempotent redeliveries.
		tx.ID = events.DeriveNotificationID(sourceEventID, "posting")
		if err := h.txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
			return nil, fmt.Errorf("failed to create commission tx for broker %s: %v", share.BrokerID, err)
		}
		ledgerIDs[commissionLedgerKey(share.BrokerID, share.Role)] = tx.ID
	}
	if err := h.emitJournalForShares(ctx, eventID, contractID, tenantID, shares); err != nil {
		return nil, err
	}
	return ledgerIDs, nil
}

// emitJournalForShares materializa a perna de dupla entrada de cada comissão no
// ledger contábil: débito em commission_expense (a imobiliária incorre a
// despesa) e crédito em commission_payable (a imobiliária deve ao corretor).
// Cada lançamento é balanceado, imutável e idempotente por source_event_id.
func (h *BrokerCommissionHandler) emitJournalForShares(ctx context.Context, eventID, contractID, tenantID string, shares []ResolvedCommissionShare) error {
	if h.journalService == nil {
		return nil
	}
	for _, share := range shares {
		if share.AmountCents <= 0 {
			continue
		}
		entry, err := journal.New(tenantID, contractID,
			fmt.Sprintf("Comissão - %s (%.1f%%)", share.Role, share.Percentage),
			time.Now().UTC(),
			[]journal.Leg{
				{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: share.AmountCents},
				{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: share.AmountCents},
			},
		)
		if err != nil {
			return fmt.Errorf("build commission journal entry for broker %s: %w", share.BrokerID, err)
		}
		sourceEventID := events.DeriveNotificationID(eventID, "journal_commission", contractID, share.BrokerID, share.Role)
		if err := h.journalService.CreateWithSourceEventID(ctx, sourceEventID, entry); err != nil {
			return fmt.Errorf("failed to create commission journal entry for broker %s: %w", share.BrokerID, err)
		}
	}
	return nil
}

// emitFallbackLedger writes the catch-all "Comissão Total" Transaction
// when the payload omits broker_participations. No Commission entity is
// created (no broker_id to attach a status to).
func (h *BrokerCommissionHandler) emitFallbackLedger(ctx context.Context, eventID, contractID, tenantID string, commissionPct, commissionAmount float64, planHash string) error {
	desc := fmt.Sprintf("Comissão Total (%.1f%%)", commissionPct)
	tx := transaction.New(contractID, transaction.TypeComissao, desc, commissionAmount, time.Now(), tenantID)
	tx.Category = "Comissão"
	tx.PlanHash = planHash
	sourceEventID := events.DeriveNotificationID(eventID, "commission_tx", contractID, "fallback")
	if err := h.txService.CreateWithSourceEventID(ctx, sourceEventID, tx); err != nil {
		return fmt.Errorf("failed to create fallback commission tx: %v", err)
	}
	return nil
}

// persistCommissionEntities writes one Commission domain entity per
// broker share for SALE deals. Fail-fast: it runs inside the
// contract.signed transaction, so the first persistence failure aborts the
// chain and rolls back the ledger rows written above.
//
// planHash links the Commission entity to its resolved plan so the
// inbox view can reconcile Transaction ↔ Commission rows unambiguously.
func (h *BrokerCommissionHandler) persistCommissionEntities(ctx context.Context, in ContractFinancialsInput, shares []ResolvedCommissionShare, planHash string, ledgerIDs map[string]string) error {
	p := in.Payload
	var firstErr error
	for _, share := range shares {
		if share.BrokerID == "" {
			log.Printf("[WARN] SALE commission skipped (empty broker_id) for deal %s role=%s", p.DealID, share.Role)
			continue
		}
		// Pula shares com amount <= 0 (ex: imobiliária com 0% default)
		// para não criar Commission entities com valor zerado.
		if share.AmountCents <= 0 {
			continue
		}
		sourceEventID := events.DeriveNotificationID(in.EventID, "commission", p.ContractID, share.BrokerID, share.Role)
		ledgerTransactionID := ledgerIDs[commissionLedgerKey(share.BrokerID, share.Role)]
		var c *commission.Commission
		var err error
		if isRentalDealType(p.Type) || isSaleDealType(p.Type) {
			c, err = h.commissionService.createWithRoleFromEventAndLedger(
				ctx, sourceEventID, p.DealID, share.BrokerID, p.ContractID, share.Role,
				share.Percentage, float64(share.AmountCents)/100.0, p.TenantID,
				p.OwnerID, p.CustomerID, p.PropertyID, planHash, ledgerTransactionID, share.AmountCents, "",
			)
		} else {
			c, err = h.commissionService.createWithRoleFromEventAndLedger(
				ctx, sourceEventID, p.DealID, share.BrokerID, p.ContractID, share.Role,
				share.Percentage, float64(share.AmountCents)/100.0, p.TenantID,
				p.OwnerID, p.CustomerID, p.PropertyID, planHash, ledgerTransactionID, 0, "",
			)
		}
		if err != nil {
			log.Printf("[WARN] failed to persist Commission for broker %s deal %s: %v", share.BrokerID, p.DealID, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if isRentalDealType(p.Type) && h.rentalIntermediation != nil {
			if err := h.rentalIntermediation.LinkCommission(ctx, p.TenantID, p.ContractID, share.BrokerID, share.Role, c.ID); err != nil {
				return fmt.Errorf("link rental commission %s: %w", c.ID, err)
			}
		}
		audit.Record(ctx, audit.Entry{
			Action:     "commission.created",
			EntityType: "Commission",
			EntityID:   c.ID,
			After: map[string]any{
				"deal_id":    p.DealID,
				"broker_id":  share.BrokerID,
				"role":       share.Role,
				"percentage": share.Percentage,
				"amount":     float64(share.AmountCents) / 100.0,
				"status":     string(c.Status),
			},
			Metadata: map[string]any{
				"contract_id":    p.ContractID,
				"deal_type":      p.Type,
				"contract_value": p.Value,
				"commission_pct": p.CommissionPct,
				"source_trigger": "contract.signed",
				"tenant_id":      p.TenantID,
			},
		})
	}
	if firstErr != nil {
		return fmt.Errorf("failed to persist commission entities: %w", firstErr)
	}
	return nil
}
