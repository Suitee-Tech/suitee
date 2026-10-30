package services

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
)

// SalePlanMaterializationService materializes the frozen commission snapshot
// (ledger + journal + Commission entities + payout orders + sale economic
// composition on charges) for a SALE billing plan created by the operator
// wizard (POST /sale-billing-plans/contracts/:id) BEFORE the contract gets
// signed through the esteira pipeline.
//
// The wizard is the supported "manual lançamento" trigger: the operator
// configures the payment shape and the charge can be paid right away, so the
// broker commission must already exist by the time the payment funding step
// runs (SaleCommissionFundingService.FundAllocations rejects a payment whose
// pool has no linked frozen obligations).
//
// It reuses the exact same ChainedContractHandler used by deal.contract.signed
// (idempotent deterministic source-event IDs, single SQL transaction,
// fail-fast steps), synthesized from the deal snapshot. Cross-company deals
// are deliberately skipped here: the partnership split must be resolved by
// the signed pipeline (CrossCompanyHandler), not by the wizard.
type SalePlanMaterializationService struct {
	chain          *ChainedContractHandler
	dealClient     *clients.DealClient
	crossRepo      CrossCompanyLookup
	tenantSettings *TenantFinancialSettingsService
}

// NewSalePlanMaterializationService builds the materializer over the same
// chain the event pipeline uses. Nil-safe: when chain or dealClient are nil
// the EnsureMaterialized call degrades to a no-op so tests and environments
// without the pipeline keep the historical behaviour.
func NewSalePlanMaterializationService(chain *ChainedContractHandler, dealClient *clients.DealClient, crossRepo CrossCompanyLookup, tenantSettings *TenantFinancialSettingsService) *SalePlanMaterializationService {
	return &SalePlanMaterializationService{chain: chain, dealClient: dealClient, crossRepo: crossRepo, tenantSettings: tenantSettings}
}

// EnsureMaterialized converges the frozen commission snapshot for a SALE
// deal/contract pair. It is safe to call repeatedly: the broker handler and
// payout provisioning skip the writes once the snapshot (same plan hash)
// exists, and the deterministic source-event IDs keep every write idempotent.
//
// Returns nil (no-op) when:
//   - the service is not configured (chain/deal client missing);
//   - dealID is empty (legacy plan created before the wizard sent deal_id);
//   - the deal is not a SALE (rent follows the signed pipeline/charge flow);
//   - the deal resolves to a cross-company partnership (signed pipeline owns it).
func (s *SalePlanMaterializationService) EnsureMaterialized(ctx context.Context, tenantID, contractID, dealID string) error {
	if s == nil || s.chain == nil || s.dealClient == nil {
		return nil
	}
	if strings.TrimSpace(dealID) == "" || strings.TrimSpace(contractID) == "" {
		return nil
	}
	deal, err := s.dealClient.GetDeal(ctx, tenantID, dealID)
	if err != nil {
		return fmt.Errorf("load deal for plan materialization: %w", err)
	}
	if deal == nil {
		return nil
	}
	if !isSaleDealType(deal.Type) {
		return nil
	}
	// Cross-company deals are resolved by the signed pipeline with the
	// partnership split; the wizard must not freeze a wrong commission base.
	if s.crossRepo != nil {
		partnership, source, lookupErr := repositories.ResolveDealPartnership(ctx, s.crossRepo, dealID, deal.PropertyID, tenantID)
		if lookupErr != nil {
			return fmt.Errorf("partnership lookup for plan materialization: %w", lookupErr)
		}
		if partnership != nil || source != "" {
			log.Printf("[INFO] sale plan materialization skipped for partner deal %s: the signed pipeline owns the split", dealID)
			return nil
		}
	}

	eventID := events.DeriveNotificationID("charges.plan.wizard", tenantID, contractID, dealID)
	// Espelha o adapter do deal.contract.signed: o percentual base é resolvido
	// ANTES do plan hash (default do tenant quando o deal traz 0), para que o
	// snapshot congelado aqui tenha o MESMO PlanHash da composição aplicada na
	// baixa (ResolvePlanForDeal) e do pipeline de assinatura.
	commissionPct := deal.CommissionPct
	if s.tenantSettings != nil {
		settings, settingsErr := s.tenantSettings.Get(ctx, tenantID)
		if settingsErr != nil {
			return fmt.Errorf("load tenant financial settings for plan materialization: %w", settingsErr)
		}
		commissionPct = ResolveCommissionPercentage(deal.Type, deal.CommissionPct, settings)
	}
	payload := &contracts.ContractSignedEventPayload{
		DealID:        dealID,
		ContractID:    contractID,
		TenantID:      tenantID,
		Value:         deal.Value,
		CommissionPct: commissionPct,
		Type:          deal.Type,
		CustomerID:    deal.CustomerID,
		OwnerID:       deal.OwnerID,
		PropertyID:    deal.PropertyID,
	}
	for _, bp := range deal.BrokerParticipations {
		payload.BrokerParticipations = append(payload.BrokerParticipations, contracts.BrokerParticipation{
			BrokerID:   bp.BrokerID,
			Role:       bp.Role,
			Percentage: bp.Percentage,
		})
	}
	in := ContractFinancialsInput{
		EventID:        eventID,
		Payload:        payload,
		CommissionPlan: &ResolvedCommissionPlan{},
	}
	if err := s.chain.Handle(ctx, in); err != nil {
		return fmt.Errorf("materialize sale commission snapshot for plan %s: %w", contractID, err)
	}
	return nil
}
