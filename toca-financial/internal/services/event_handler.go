package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
)

// getString reads a string field from a map[string]interface{}; returns
// empty when the key is missing or not a string. Used by the event
// payload parsers in this file.
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// newJournalServiceForHandler monta o ledger de dupla entrada quando há banco
// disponível. Testes que passam pgDB=nil degradam ao comportamento legado.
func newJournalServiceForHandler(pgDB *database.PostgresDB) *JournalService {
	if pgDB == nil {
		return nil
	}
	return NewJournalService(repository.NewJournalPostgresRepository(pgDB)).WithDB(pgDB)
}

// newContractSignedChain builds the canonical contract.signed pipeline used
// by the event handler AND by the sale-plan materializer (wizard commit
// path). Sharing the builder keeps both triggers on the exact same step
// composition, transaction policy and idempotency keys.
func NewContractSignedChain(pgDB *database.PostgresDB, chargeService *ChargeService, transferService *TransferService, txService *TransactionService, rentalConfigService *RentalChargeConfigService, rentalService *RentalService, commissionService *CommissionService, crossRepo CrossCompanyLookup, partnerRoyaltyService *PartnerRoyaltyService, saleBillingPlanService *SaleBillingPlanService, commissionRulesService *CommissionRulesService, tenantSettings *TenantFinancialSettingsService, payoutOrchestration *PayoutOrchestrationService) *ChainedContractHandler {
	brokerHandler := NewBrokerCommissionHandler(txService, commissionService, commissionRulesService).
		WithTenantSettings(tenantSettings).
		WithChargeService(chargeService).
		WithJournalService(newJournalServiceForHandler(pgDB))
	if payoutOrchestration != nil && payoutOrchestration.rentalIntermediation != nil {
		brokerHandler.WithRentalIntermediation(payoutOrchestration.rentalIntermediation)
	}
	chain := NewChainedContractHandler(
		NewChargeIssuanceHandler(chargeService, rentalConfigService, rentalService, saleBillingPlanService).WithTenantSettings(tenantSettings),
		NewOwnerTransferHandler(transferService, tenantSettings),
		brokerHandler,
		NewCrossCompanyHandler(crossRepo, txService, commissionService, partnerRoyaltyService),
	).WithDB(pgDB)
	if payoutOrchestration != nil {
		chain.WithPayoutProvisioning(NewPayoutProvisioningStep(payoutOrchestration))
	}
	return chain
}

// NewContractSignedEventHandler wires the contract.signed pipeline as a
// thin adapter on top of ChainedContractHandler. The chain composes
// ChargeIssuanceHandler → OwnerTransferHandler → BrokerCommissionHandler
// with a per-step failure policy (see chained_contract_handler.go).
//
// Optional dependencies (all but charge / transfer / tx services) keep
// the historical contract: when nil, the corresponding step degrades to
// the legacy behaviour documented in each sub-handler.
//
// `rentalService` and `commissionService` are optional. `crossRepo` and
// `partnerRoyaltyService` are optional cross-company hooks. When both
// are set, the broker step detects a cross-company deal and, on an
// active partnership, splits the commission per the partnership
// CommissionModel and records a franchise royalty.
//
// `saleBillingPlanService` is optional — when nil the SALE branch does not
// fabricate a payment plan that was never approved by the operator.
//
// `commissionRulesService` is optional — when nil the commission share
// resolution falls back to commission.DefaultPercentages.
func NewContractSignedEventHandler(pgDB *database.PostgresDB, chargeService *ChargeService, transferService *TransferService, txService *TransactionService, rentalConfigService *RentalChargeConfigService, rentalService *RentalService, commissionService *CommissionService, crossRepo CrossCompanyLookup, partnerRoyaltyService *PartnerRoyaltyService, saleBillingPlanService *SaleBillingPlanService, commissionRulesService *CommissionRulesService, tenantSettings *TenantFinancialSettingsService, payoutOrchestration *PayoutOrchestrationService) events.EventHandler {
	chain := NewContractSignedChain(pgDB, chargeService, transferService, txService, rentalConfigService, rentalService, commissionService, crossRepo, partnerRoyaltyService, saleBillingPlanService, commissionRulesService, tenantSettings, payoutOrchestration)

	return func(ctx context.Context, event events.Event) error {
		in, err := parseContractSignedEvent(event)
		if err != nil {
			return err
		}

		p := in.Payload
		// Reject malformed business payloads before resolving tenant-scoped
		// defaults or opening the privileged cross-tenant transaction. Besides
		// avoiding needless database work, this preserves deterministic validation
		// errors for producers that submit more than one invalid field.
		if p.Value <= 0 {
			return fmt.Errorf("invalid contract value: must be greater than 0")
		}
		if p.CommissionPct < 0 || p.CommissionPct > 100 {
			return fmt.Errorf("invalid commission percentage: must be between 0 and 100")
		}
		if p.CustomerID == "" || p.OwnerID == "" {
			return fmt.Errorf("invalid contract parties: customer and owner are required")
		}
		if p.DealID == "" || p.ContractID == "" {
			return fmt.Errorf("invalid contract payload: deal_id and contract_id are required")
		}
		if p.TenantID == "" {
			return fmt.Errorf("invalid contract payload: tenant_id is required")
		}
		// Service-role execution is deliberately limited to authenticated
		// internal deliveries whose AMQP tenant header exactly matches the
		// immutable payload tenant. This prevents a forged/misrouted event from
		// using the BYPASSRLS transaction to write another company's graph.
		if pgDB != nil {
			eventTenantID, tenantErr := pkgctx.GetTenantID(ctx)
			if tenantErr != nil {
				return fmt.Errorf("contract.signed requires tenant context: %w", tenantErr)
			}
			if eventTenantID != p.TenantID {
				return fmt.Errorf("contract.signed tenant mismatch: header=%s payload=%s", eventTenantID, p.TenantID)
			}
		}

		execute := func(execCtx context.Context) error {
			// Resolve o percentual-base uma vez com a mesma regra pura usada pelo
			// preview. O plano por beneficiário será preenchido pelo broker step
			// depois do eventual ajuste cross-company e compartilhado com o payout.
			if p.CommissionPct == 0 && tenantSettings != nil {
				settings, settingsErr := tenantSettings.Get(execCtx, p.TenantID)
				if settingsErr != nil {
					return fmt.Errorf("load tenant financial settings: %w", settingsErr)
				}
				p.CommissionPct = ResolveCommissionPercentage(p.Type, p.CommissionPct, settings)
				log.Printf("[INFO] commission_pct auto-filled from tenant defaults for deal %s: %.1f%%",
					p.DealID, p.CommissionPct)
			}
			in.CommissionPlan = &ResolvedCommissionPlan{}

			log.Printf("[AUDIT] Generating financials for contract %s (type: %s, value: %.2f, commission: %.1f%%, tenant_id: %s)\n",
				p.ContractID, p.Type, p.Value, p.CommissionPct, p.TenantID)

			if err := chain.Handle(execCtx, in); err != nil {
				return err
			}
			// Audit: plan hash is now deterministic and shared between preview and
			// contract.signed. Recording it here lets reconciliation tooling compare
			// preview-vs-execution hashes per contract.
			if in.CommissionPlan != nil && in.CommissionPlan.PlanHash != "" {
				log.Printf("[INFO] commission plan resolved for contract %s: plan_hash=%s total_amount=%.2f shares=%d retained_pct=%.1f%%",
					p.ContractID, in.CommissionPlan.PlanHash, float64(in.CommissionPlan.TotalAmountCents)/100.0,
					len(in.CommissionPlan.Shares), in.CommissionPlan.RetainedPercentage)
			}
			return nil
		}

		if pgDB != nil {
			return pgDB.RunInServiceContext(ctx, execute)
		}
		return execute(ctx)
	}
}

// parseContractSignedEvent turns the raw events.Event payload into the
// resolved ContractFinancialsInput consumed by the chained handler.
// Date parsing falls back to the legacy defaults documented in the
// previous executeContractLogic implementation.
func parseContractSignedEvent(event events.Event) (ContractFinancialsInput, error) {
	payloadMap, ok := event.Payload().(map[string]interface{})
	if !ok {
		return ContractFinancialsInput{}, fmt.Errorf("invalid payload type: %T", event.Payload())
	}

	value := 0.0
	if v, ok := payloadMap["value"].(float64); ok {
		value = v
	}
	commissionPct := 0.0
	if c, ok := payloadMap["commission_pct"].(float64); ok {
		commissionPct = c
	}

	dealType := getString(payloadMap, "type")

	// CommissionPct == 0 means the deal has no commission configured.
	// This is a silent data-quality issue — subsequent commission steps
	// produce zero-amount transactions and entities. The warning surfaces
	// it in logs so operators can identify deals with missing commission
	// configuration from the admin dashboard.
	if commissionPct == 0 {
		log.Printf("[WARN] commission_pct is 0 for contract_signed event (deal=%s, contract=%s): no commission will be calculated",
			getString(payloadMap, "deal_id"), getString(payloadMap, "contract_id"))
	}

	startDate, err := parseEventTime(payloadMap, "start_date")
	if err != nil {
		startDate = time.Now()
	}
	endDate, err := parseEventTime(payloadMap, "end_date")
	if err != nil {
		if isRentalDealType(dealType) {
			endDate = startDate.AddDate(0, 30, 0)
		} else {
			endDate = startDate.AddDate(1, 0, 0)
		}
	}
	if endDate.Before(startDate) {
		if isRentalDealType(dealType) {
			endDate = startDate.AddDate(0, 30, 0)
		} else {
			endDate = startDate
		}
	}

	payload := &contracts.ContractSignedEventPayload{
		DealID:        getString(payloadMap, "deal_id"),
		ContractID:    getString(payloadMap, "contract_id"),
		TenantID:      getString(payloadMap, "tenant_id"),
		Value:         value,
		CommissionPct: commissionPct,
		Type:          dealType,
		CustomerID:    getString(payloadMap, "customer_id"),
		OwnerID:       getString(payloadMap, "owner_id"),
		PropertyID:    getString(payloadMap, "property_id"),
		StartDate:     startDate.Format(time.RFC3339),
		EndDate:       endDate.Format(time.RFC3339),
	}

	if bpsRaw, ok := payloadMap["broker_participations"].([]interface{}); ok && len(bpsRaw) > 0 {
		for _, bpRaw := range bpsRaw {
			if bpMap, ok := bpRaw.(map[string]interface{}); ok {
				bp := contracts.BrokerParticipation{}
				if v, ok := bpMap["broker_id"].(string); ok {
					bp.BrokerID = v
				}
				if v, ok := bpMap["role"].(string); ok {
					bp.Role = v
				}
				if v, ok := bpMap["percentage"].(float64); ok {
					bp.Percentage = v
				}
				payload.BrokerParticipations = append(payload.BrokerParticipations, bp)
			}
		}
	}

	// Parties do contrato (comprador, proprietário, cônjuge, etc.)
	// Cada parte pode ter SharePct > 0 — neste caso, a parte é
	// beneficiária de uma fatia da comissão além dos brokers.
	if partiesRaw, ok := payloadMap["parties"].([]interface{}); ok && len(partiesRaw) > 0 {
		for _, pRaw := range partiesRaw {
			if pMap, ok := pRaw.(map[string]interface{}); ok {
				pp := contracts.PartyPayload{}
				if v, ok := pMap["party_id"].(string); ok {
					pp.PartyID = v
				}
				if v, ok := pMap["lead_id"].(string); ok {
					pp.LeadID = v
				}
				if v, ok := pMap["account_id"].(string); ok {
					pp.AccountID = v
				}
				if v, ok := pMap["email"].(string); ok {
					pp.Email = v
				}
				if v, ok := pMap["role"].(string); ok {
					pp.Role = v
				}
				if v, ok := pMap["is_primary"].(bool); ok {
					pp.IsPrimary = v
				}
				if v, ok := pMap["share_pct"].(float64); ok {
					pp.SharePct = v
				}
				payload.Parties = append(payload.Parties, pp)
			}
		}
	}

	return ContractFinancialsInput{
		EventID:      event.EventID(),
		Payload:      payload,
		StartDate:    startDate,
		EndDate:      endDate,
		CrossCompany: &CrossCompanySettlement{},
	}, nil
}

// CrossCompanyLookup is the subset of repository.CrossTenantRepository
// the contract.signed pipeline needs to resolve cross-company deals.
// Declared as an interface so tests can pass a fake; the production
// implementation is repository.CrossTenantRepository.
type CrossCompanyLookup interface {
	PropertyOwnerTenant(ctx context.Context, propertyID string) (string, error)
	ActivePartnershipBetween(ctx context.Context, companyA, companyB string) (*repository.PartnershipFinancials, error)
}

// royaltyBaseAmount resolves the amount the royalty percentage is applied to.
//   - "revenue" → deal value (bruto da venda/locação)
//   - "gross_commission" → total da comissão antes de deductions
//   - "net_commission" → total da comissão menos a soma das deductions
//     registradas nas Commissions do deal. Quando não há deductions ainda
//     registradas (caminho normal no momento do contract.signed), net ==
//     gross — sem regressão vs. comportamento anterior.
func royaltyBaseAmount(base string, dealValue, commissionAmount, totalDeductions float64) float64 {
	switch base {
	case "revenue":
		return dealValue
	case "net_commission":
		net := commissionAmount - totalDeductions
		if net < 0 {
			net = 0
		}
		return net
	case "gross_commission":
		return commissionAmount
	default:
		return commissionAmount
	}
}

// isSaleDealType accepts both English ("SALE") and Portuguese ("venda")
// variants emitted by upstream services.
func isSaleDealType(t string) bool {
	v := strings.TrimSpace(strings.ToLower(t))
	return v == "sale" || v == "venda"
}

func isRentalDealType(t string) bool {
	v := strings.TrimSpace(strings.ToLower(t))
	return v == "rent" || v == "rental" || v == "locacao" || v == "locação"
}

func parseEventTime(m map[string]interface{}, key string) (time.Time, error) {
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case string:
			if t == "" {
				return time.Time{}, fmt.Errorf("empty date")
			}
			if parsed, err := time.Parse(time.RFC3339, t); err == nil {
				return parsed, nil
			}
			if parsed, err := time.Parse("2006-01-02", t); err == nil {
				return parsed, nil
			}
			return time.Time{}, fmt.Errorf("invalid date format")
		case time.Time:
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("missing date")
}

func defaultFirstChargeDate(startDate time.Time, defaultDueDay int) time.Time {
	loc := startDate.Location()
	dueThisMonth := time.Date(startDate.Year(), startDate.Month(), min(defaultDueDay, daysInMonth(startDate.Year(), startDate.Month(), loc)), 0, 0, 0, 0, loc)
	if startDate.Day() <= dueThisMonth.Day() {
		return dueThisMonth
	}
	next := startDate.AddDate(0, 1, 0)
	return time.Date(next.Year(), next.Month(), min(defaultDueDay, daysInMonth(next.Year(), next.Month(), loc)), 0, 0, 0, 0, loc)
}

func daysInMonth(year int, month time.Month, loc *time.Location) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func NewDealEventHandler(commService *CommissionService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		// Payload is expected to be map[string]interface{} from JSON unmarshal
		payload, ok := event.Payload().(map[string]interface{})
		if !ok {
			return fmt.Errorf("invalid payload type: %T", event.Payload())
		}

		dealID, _ := payload["deal_id"].(string)
		brokerID, _ := payload["broker_id"].(string)
		value, _ := payload["value"].(float64)
		tenantID, _ := payload["tenant_id"].(string)

		if dealID == "" || brokerID == "" {
			return nil
		}

		// Default rule: 6% of deal value
		commissionAmount := value * 0.06

		fmt.Printf("Generating commission for deal %s: %.2f\n", dealID, commissionAmount)

		// Worker por evento: sem autor humano (RequestedBy vazio ⇒ sem conflito de SoD).
		_, err := commService.Create(ctx, dealID, brokerID, commissionAmount, tenantID, "")
		return err
	}
}

// NewChargePaidEventHandler reacts only to full settlement of the canonical
// Charge. Invoice payments, including partial allocations, have their own
// invoice.paid contract and must never release charge-level payout work.
func NewChargePaidEventHandler(payoutOrchestration *PayoutOrchestrationService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		var tenantID, chargeID string
		switch payload := event.Payload().(type) {
		case contracts.ChargePaidEventPayload:
			tenantID, chargeID = payload.TenantID, payload.ChargeID
		case *contracts.ChargePaidEventPayload:
			if payload == nil {
				return errors.New("invalid nil charge.paid payload")
			}
			tenantID, chargeID = payload.TenantID, payload.ChargeID
		case map[string]interface{}:
			tenantID = getString(payload, "tenant_id")
			chargeID = getString(payload, "charge_id")
		default:
			return fmt.Errorf("invalid charge.paid payload type: %T", event.Payload())
		}
		if tenantID == "" || chargeID == "" {
			return errors.New("invalid charge.paid payload: tenant_id and charge_id are required")
		}
		if payoutOrchestration == nil {
			return nil
		}
		if err := payoutOrchestration.OnChargePaid(ctx, tenantID, chargeID); err != nil {
			return fmt.Errorf("orchestrate charge.paid for charge %s: %w", chargeID, err)
		}
		return nil
	}
}

// NewLegacyInvoicePaidEventHandler is a rolling-deployment compatibility
// adapter for schema_version=0 messages historically emitted by ChargeService.
// Modern schema_version=2 invoice events are deliberately ignored here; their
// corresponding full-settlement signal, when any, arrives as charge.paid.
func NewLegacyInvoicePaidEventHandler(payoutOrchestration *PayoutOrchestrationService) events.EventHandler {
	return func(ctx context.Context, event events.Event) error {
		var tenantID, invoiceID, chargeID string
		schemaVersion := 0
		switch payload := event.Payload().(type) {
		case contracts.InvoicePaidEventPayload:
			tenantID, invoiceID, chargeID, schemaVersion = payload.TenantID, payload.InvoiceID, payload.ChargeID, payload.SchemaVersion
		case *contracts.InvoicePaidEventPayload:
			if payload == nil {
				return errors.New("invalid nil invoice.paid payload")
			}
			tenantID, invoiceID, chargeID, schemaVersion = payload.TenantID, payload.InvoiceID, payload.ChargeID, payload.SchemaVersion
		case map[string]interface{}:
			tenantID = getString(payload, "tenant_id")
			invoiceID = getString(payload, "invoice_id")
			chargeID = getString(payload, "charge_id")
			if rawVersion, ok := payload["schema_version"].(float64); ok {
				schemaVersion = int(rawVersion)
			}
		default:
			return fmt.Errorf("invalid invoice.paid payload type: %T", event.Payload())
		}
		if schemaVersion >= 2 {
			return nil
		}
		if chargeID == "" {
			chargeID = invoiceID
		}
		if tenantID == "" || chargeID == "" {
			return errors.New("invalid legacy invoice.paid payload: tenant_id and charge identity are required")
		}
		if payoutOrchestration == nil {
			return nil
		}
		if err := payoutOrchestration.OnChargePaid(ctx, tenantID, chargeID); err != nil {
			return fmt.Errorf("orchestrate legacy invoice.paid for charge %s: %w", chargeID, err)
		}
		return nil
	}
}

// NewInvoicePaidEventHandler is kept as a source-compatible alias for older
// wiring. New code should name the legacy intent explicitly above.
func NewInvoicePaidEventHandler(_ *CommissionService, payoutOrchestration *PayoutOrchestrationService) events.EventHandler {
	return NewLegacyInvoicePaidEventHandler(payoutOrchestration)
}
