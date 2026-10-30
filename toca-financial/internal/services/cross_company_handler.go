package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
)

// CrossCompanyHandler records the owner-company's commission share and,
// for franchise partnerships, the franchisor royalty. Runs BEFORE the
// broker step in the contract.signed chain so it can reduce
// p.CommissionPct before BrokerCommissionHandler distributes the remaining
// share to individual brokers.
//
// Once an active cross-company partnership is found, every monetary leg is
// fail-fast. The handler runs before broker provisioning, so returning a plain
// error prevents the chain from over-provisioning the closing company when the
// owner share or royalty could not be recorded.
type CrossCompanyHandler struct {
	crossRepo             CrossCompanyLookup
	txService             *TransactionService
	commissionService     *CommissionService
	partnerRoyaltyService *PartnerRoyaltyService
}

// NewCrossCompanyHandler builds the cross-company step. A nil crossRepo keeps
// the historical no-op behaviour. Once a partnership exposes a monetary leg,
// the dependencies required by that leg are validated by Handle.
func NewCrossCompanyHandler(
	crossRepo CrossCompanyLookup,
	txService *TransactionService,
	commissionService *CommissionService,
	partnerRoyaltyService *PartnerRoyaltyService,
) *CrossCompanyHandler {
	return &CrossCompanyHandler{
		crossRepo:             crossRepo,
		txService:             txService,
		commissionService:     commissionService,
		partnerRoyaltyService: partnerRoyaltyService,
	}
}

// Handle splits the commission per the partnership model and records the
// franchise royalty when applicable. Cross-company lookup and write failures
// are fail-fast so the chained handler does not continue to broker payouts.
func (h *CrossCompanyHandler) Handle(ctx context.Context, in ContractFinancialsInput) error {
	if h.crossRepo == nil {
		return nil
	}
	p := in.Payload
	pf, ownerTenant, err := repository.ResolveDealPartnership(ctx, h.crossRepo, p.DealID, p.PropertyID, p.TenantID)
	if err != nil {
		return fmt.Errorf("condições financeiras da parceria: %w", err)
	}
	if pf == nil {
		log.Printf("[INFO] cross-company deal %s (owner=%s, closer=%s) has no active partnership — no split applied", p.DealID, ownerTenant, p.TenantID)
		return nil
	}

	fees, err := partnershipFees(pf, ownerTenant, p.TenantID)
	if err != nil {
		return err
	}
	if len(fees) > 0 && (h.partnerRoyaltyService == nil || in.CrossCompany == nil) {
		return fmt.Errorf("serviço de obrigações de parceria obrigatório para as taxas")
	}
	dealValueCents := valueobjects.NewMoneyFromFloat(p.Value).Cents()
	commissionCents := valueobjects.NewMoney(dealValueCents).Percentage(p.CommissionPct).Cents()
	ownerPct, closingPct := pf.EffectiveCommissionModel()
	if ownerPct < 0 || closingPct < 0 || ownerPct+closingPct < 99.99 || ownerPct+closingPct > 100.01 {
		return fmt.Errorf("cross-company partnership %s commission split must sum to 100, got %.2f", pf.ID, ownerPct+closingPct)
	}

	ownerShareCents := valueobjects.NewMoney(commissionCents).Percentage(ownerPct).Cents()
	if ownerShareCents > 0 {
		if h.txService == nil {
			return fmt.Errorf("cross-company owner-share transaction service is required for partnership %s", pf.ID)
		}
		if in.CrossCompany == nil {
			return fmt.Errorf("cross-company settlement carrier is required for partnership %s", pf.ID)
		}
	}

	// Write the royalty first. A failure stops the owner and broker legs. If the
	// owner write then fails, retrying is safe because both writes use stable
	// source-event identifiers.
	royaltyAmountCents := int64(0)
	royaltyPayeeTenant := ""
	if pf.IsFranchise() {
		royaltyAmountCents, royaltyPayeeTenant, err = h.writeFranchiseRoyalty(ctx, in, pf, float64(commissionCents)/100.0)
		if err != nil {
			return fmt.Errorf("cross-company franchise royalty for partnership %s: %w", pf.ID, err)
		}
	}

	for _, fee := range fees {
		sourceID := events.DeriveNotificationID("partnership_fixed_fee", pf.ID, p.DealID, fee.Kind)
		obligation, err := h.partnerRoyaltyService.CreateFromEvent(ctx, sourceID, pf.ID, p.DealID, p.ContractID, fee.PayerTenant, fee.PayeeTenant, float64(fee.AmountCents)/100, 100, fee.Kind)
		if err != nil {
			return fmt.Errorf("provisionar %s: %w", fee.Label, err)
		}
		if obligation == nil {
			return fmt.Errorf("taxa de parceria não persistida")
		}
	}
	if ownerShareCents > 0 {
		if err := h.writeOwnerShare(ctx, in.EventID, p.ContractID, p.DealID, ownerTenant, commissionCents, ownerPct); err != nil {
			return fmt.Errorf("cross-company owner share for partnership %s: %w", pf.ID, err)
		}
	}

	// Publish the settlement only after every monetary write succeeded. The
	// payout step must never observe a share that failed to reach the ledger.
	if in.CrossCompany != nil {
		if len(fees) > 0 {
			in.CrossCompany.FeePayerTenant = fees[0].PayerTenant
		}
		in.CrossCompany.PartnershipID = pf.ID
		in.CrossCompany.RecipientTenant = ownerTenant
		in.CrossCompany.OwnerPercentage = ownerPct
		in.CrossCompany.ClosingPercentage = closingPct
		in.CrossCompany.OwnerAmountCents = ownerShareCents
		in.CrossCompany.RoyaltyPercentage = pf.RoyaltyPercentage
		in.CrossCompany.RoyaltyBaseMode = pf.EffectiveRoyaltyBase()
		in.CrossCompany.RoyaltyAmountCents = royaltyAmountCents
		in.CrossCompany.RoyaltyPayeeTenant = royaltyPayeeTenant
	}

	// The broker step runs next and distributes only the closing company's
	// slice. The owner-company share above plus this reduced slice conserve
	// exactly 100% of the original commission base.
	p.CommissionPct = p.CommissionPct * closingPct / 100
	return nil
}

func (h *CrossCompanyHandler) writeOwnerShare(ctx context.Context, eventID, contractID, dealID, ownerTenant string, commissionCents int64, ownerPct float64) error {
	if h.txService == nil {
		return fmt.Errorf("transaction service is required")
	}
	ownerShareCents := valueobjects.NewMoney(commissionCents).Percentage(ownerPct).Cents()
	if ownerShareCents <= 0 {
		return nil
	}
	desc := fmt.Sprintf("Comissão parceria (empresa parceira de origem — %.1f%%)", ownerPct)
	tx := transaction.New(contractID, transaction.TypeComissao, desc, float64(ownerShareCents)/100.0, time.Now(), ownerTenant)
	tx.Category = "Comissão Parceria"
	tx.DealID = dealID
	if err := h.txService.createPartnershipShare(ctx, tx); err != nil {
		log.Printf("[WARN] cross-company: failed to record owner-company commission share for deal %s tenant %s: %v", dealID, ownerTenant, err)
		return err
	}
	return nil
}

func (h *CrossCompanyHandler) writeFranchiseRoyalty(ctx context.Context, in ContractFinancialsInput, pf *repository.PartnershipFinancials, commissionAmount float64) (int64, string, error) {
	p := in.Payload
	if pf.RoyaltyPercentage <= 0 {
		return 0, "", nil
	}

	var totalDeductions float64
	if pf.EffectiveRoyaltyBase() == "net_commission" {
		if h.commissionService == nil {
			return 0, "", fmt.Errorf("commission service is required for net-commission royalty")
		}
		commissions, err := h.commissionService.ListByDeal(ctx, p.TenantID, p.DealID)
		if err != nil {
			log.Printf("[WARN] cross-company: failed to load commissions for deal %s to compute net deductions: %v", p.DealID, err)
			return 0, "", fmt.Errorf("load commissions for net-commission royalty: %w", err)
		}
		for _, c := range commissions {
			totalDeductions += c.SumDeductions()
		}
	}
	base := royaltyBaseAmount(pf.EffectiveRoyaltyBase(), p.Value, commissionAmount, totalDeductions)
	if base <= 0 {
		return 0, "", nil
	}
	if h.partnerRoyaltyService == nil {
		return 0, "", fmt.Errorf("partner royalty service is required")
	}
	payerTenant := pf.TargetTenant
	payeeTenant := pf.RequesterTenant
	if payerTenant == "" || payeeTenant == "" || payerTenant == payeeTenant {
		return 0, "", fmt.Errorf("invalid franchise direction for partnership %s", pf.ID)
	}
	sourceEventID := events.DeriveNotificationID(in.EventID, "partner_royalty", pf.ID, p.DealID, p.ContractID, payerTenant, payeeTenant)
	royalty, err := h.partnerRoyaltyService.CreateFromEvent(ctx, sourceEventID, pf.ID, p.DealID, p.ContractID, payerTenant, payeeTenant, base, pf.RoyaltyPercentage)
	if err != nil {
		log.Printf("[WARN] cross-company: failed to record franchise royalty for deal %s partnership %s: %v", p.DealID, pf.ID, err)
		return 0, "", err
	}
	if royalty == nil {
		return 0, "", fmt.Errorf("franchise royalty persistence returned no obligation")
	}
	return valueobjects.NewMoneyFromFloat(royalty.Amount).Cents(), royalty.PayeeTenant, nil
}
