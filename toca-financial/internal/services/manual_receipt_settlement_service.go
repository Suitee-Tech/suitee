package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

type manualReceiptDealLookup interface {
	GetDeal(ctx context.Context, tenantID, dealID string) (*clients.DealDTO, error)
}

// ManualReceiptSettlementService resolves the authoritative owner/property of
// a manual SALE receipt and delegates idempotent transfer creation. It runs in
// TransactionService's settlement transaction, so the cash baixa and owner
// obligation commit or roll back together. For RENT receipts it also funds the
// frozen rental intermediation plan (funding-gated commissions + payouts).
type ManualReceiptSettlementService struct {
	transfers            *TransferService
	deals                manualReceiptDealLookup
	journal              *JournalService
	rentalIntermediation *RentalIntermediationService
}

func NewManualReceiptSettlementService(transfers *TransferService, deals manualReceiptDealLookup) *ManualReceiptSettlementService {
	return &ManualReceiptSettlementService{transfers: transfers, deals: deals}
}

func (s *ManualReceiptSettlementService) WithJournalService(journalService *JournalService) *ManualReceiptSettlementService {
	s.journal = journalService
	return s
}

// WithRentalIntermediation funds the frozen first-rent plan when a manual RENT
// receipt is settled. When nil, rent settlement degrades to the Phase A shape
// (administration fee + owner) without funding the signing-time commissions.
func (s *ManualReceiptSettlementService) WithRentalIntermediation(intermediation *RentalIntermediationService) *ManualReceiptSettlementService {
	s.rentalIntermediation = intermediation
	return s
}

func (s *ManualReceiptSettlementService) EnsureForSettledReceipt(ctx context.Context, receipt *transaction.Transaction) error {
	if receipt == nil || receipt.EconomicBreakdown == nil {
		return nil
	}
	flow := receipt.EconomicBreakdown.Flow
	if flow != "sale" && flow != "rent" {
		return nil
	}
	if s == nil || s.transfers == nil || s.deals == nil || s.journal == nil {
		return errors.New("manual receipt settlement dependencies are not configured")
	}
	dealID := strings.TrimSpace(receipt.DealID)
	if dealID == "" {
		return errors.New("manual receipt has no source deal")
	}
	deal, err := s.deals.GetDeal(ctx, receipt.TenantID, dealID)
	if err != nil {
		return fmt.Errorf("load source deal %s: %w", dealID, err)
	}
	if deal == nil {
		return errors.New("manual receipt source deal not found")
	}
	if flow == "sale" && !isSaleDealType(deal.Type) {
		return errors.New("manual receipt source deal is not a sale")
	}
	if flow == "rent" && !isRentalDealType(deal.Type) {
		return errors.New("manual receipt source deal is not a rental")
	}
	if deal.TenantID != "" && deal.TenantID != receipt.TenantID {
		return errors.New("manual receipt source deal belongs to another tenant")
	}
	if flow == "rent" {
		if err := s.fundRentIntermediation(ctx, receipt); err != nil {
			return err
		}
	}
	entry, err := newManualReceiptJournalEntry(receipt, deal.OwnerID)
	if err != nil {
		return err
	}
	if _, err := s.journal.CreateCanonicalWithSourceEventID(ctx, manualReceiptJournalSourceID(receipt), entry); err != nil {
		return fmt.Errorf("post manual receipt journal: %w", err)
	}
	_, _, err = s.transfers.EnsureForManualReceipt(ctx, receipt, deal.OwnerID, deal.PropertyID)
	return err
}

// CancelForReceipt cancela o OwnerTransfer liberado por um recebimento manual,
// usado quando o recebimento é revertido/cancelado. Repasse já processado
// (caixa movido ao proprietário) falha fechado — exige estorno explícito.
func (s *ManualReceiptSettlementService) CancelForReceipt(ctx context.Context, receipt *transaction.Transaction) error {
	if s == nil || s.transfers == nil || receipt == nil {
		return nil
	}
	identityRepo, ok := s.transfers.repo.(transferTransactionIdentityRepository)
	if !ok {
		return errors.New("transfer repository cannot identify manual receipt transfers")
	}
	existing, err := identityRepo.GetBySourceTransactionID(ctx, receipt.TenantID, receipt.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	if existing.Status == transfer.StatusProcessed {
		return errors.New("owner transfer already processed; estorno of the owner settlement is required before reversing this receipt")
	}
	return s.transfers.Cancel(ctx, existing.TenantID, existing.ID, "system:receipt-cancel", "Sistema", "Recebimento manual revertido")
}

// fundRentIntermediation funds the frozen first-rent plan with the rent left
// after the administration fee, then rewrites the receipt's economic breakdown
// so the owner obligation reflects the funded slice. Broker funding is already
// accrued as commission_payable at contract signing, so the manual receipt
// recognizes gross intermediation revenue while retaining the broker/agency
// split for obligations and available-margin views.
func (s *ManualReceiptSettlementService) fundRentIntermediation(ctx context.Context, receipt *transaction.Transaction) error {
	b := receipt.EconomicBreakdown
	if b == nil || b.Flow != "rent" {
		return nil
	}
	availableCents := b.GrossCents - b.DiscountCents - b.AdminFeeCents
	if availableCents < 0 {
		return errors.New("manual rent receipt fees exceed collected cash")
	}
	appliedCents := int64(0)
	brokerCents := int64(0)
	if s.rentalIntermediation != nil && availableCents > 0 {
		referenceDate := receipt.Date
		if receipt.PaymentDate != nil && !receipt.PaymentDate.IsZero() {
			referenceDate = *receipt.PaymentDate
		}
		funding, err := s.rentalIntermediation.FundPayment(
			ctx, receipt.TenantID, receipt.ContractID, receipt.ID, "", referenceDate.Format("2006-01"), availableCents, referenceDate.UTC(),
		)
		if err != nil {
			return fmt.Errorf("fund rental intermediation from manual receipt: %w", err)
		}
		appliedCents = funding.AppliedCents
		for _, broker := range funding.BrokerFunding {
			brokerCents += broker.AmountCents
		}
	}
	if appliedCents < 0 || appliedCents > availableCents || brokerCents < 0 || brokerCents > appliedCents {
		return errors.New("manual rent intermediation exceeds collected cash")
	}
	receipt.EconomicBreakdown = &transaction.EconomicBreakdown{
		Flow:                   "rent",
		GrossCents:             b.GrossCents,
		CommissionPoolCents:    appliedCents,
		CommissionPayableCents: brokerCents,
		CommissionFundedCents:  brokerCents,
		CommissionFundingKnown: true,
		AgencyRetainedCents:    appliedCents - brokerCents,
		OwnerPayableCents:      availableCents - appliedCents,
		AdminFeeCents:          b.AdminFeeCents,
		DiscountCents:          b.DiscountCents,
	}
	return nil
}

func manualSaleReceiptJournalSourceID(receipt *transaction.Transaction) string {
	if receipt == nil {
		return ""
	}
	return events.DeriveNotificationID("manual_sale_receipt.payment", receipt.TenantID, receipt.ID)
}

// manualReceiptJournalSourceID namespaces the settlement journal by economic
// flow so sale and rent receipts never collide. For sale receipts it matches
// manualSaleReceiptJournalSourceID exactly.
func manualReceiptJournalSourceID(receipt *transaction.Transaction) string {
	if receipt == nil || receipt.EconomicBreakdown == nil {
		return ""
	}
	return events.DeriveNotificationID("manual_"+receipt.EconomicBreakdown.Flow+"_receipt.payment", receipt.TenantID, receipt.ID)
}

// newManualReceiptJournalEntry builds the double-entry settlement of a manual
// receipt (sale or rent). The agency's revenue credit is the administration fee
// (rent only) plus any commission retention.
func newManualReceiptJournalEntry(receipt *transaction.Transaction, ownerID string) (*journal.Entry, error) {
	if receipt == nil || receipt.EconomicBreakdown == nil {
		return nil, errors.New("manual receipt economic breakdown is required")
	}
	if err := validateManualReceiptBreakdown(receipt); err != nil {
		return nil, err
	}
	b := receipt.EconomicBreakdown
	cashCents := b.GrossCents - b.DiscountCents
	if cashCents <= 0 {
		return nil, errors.New("manual receipt cash amount must be positive")
	}
	legs := []journal.Leg{{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: cashCents, Flow: b.Flow}}
	appendCredit := func(account journal.Account, amount int64, beneficiary string) {
		if amount <= 0 {
			return
		}
		legs = append(legs, journal.Leg{
			Account: account, Direction: journal.Credit, AmountCents: amount, BeneficiaryPartyID: strings.TrimSpace(beneficiary), Flow: b.Flow,
		})
	}
	appendCredit(journal.AccountOwnerPayable, b.OwnerPayableCents, ownerID)
	// GROSS revenue: the agency recognizes the full commission (broker share +
	// retention) plus the administration fee as revenue. The broker share is
	// expensed separately by the commission accrual entry, so it is no longer
	// credited to commission_payable here.
	appendCredit(journal.AccountRevenue, b.CommissionPayableCents+b.AgencyRetainedCents+b.AdminFeeCents, receipt.TenantID)
	appendCredit(journal.AccountPartnerPayable, b.PartnerPayableCents, "")
	settledAt := receipt.Date
	if receipt.PaymentDate != nil && !receipt.PaymentDate.IsZero() {
		settledAt = *receipt.PaymentDate
	}
	settledAt = settledAt.UTC().Truncate(time.Microsecond)
	description := "Recebimento manual da venda "
	if b.Flow == "rent" {
		description = "Recebimento manual da locação "
	}
	entry, err := journal.New(receipt.TenantID, receipt.ContractID, description+receipt.ID, settledAt, legs)
	if err != nil {
		return nil, err
	}
	entry.OperationID = receipt.ID
	return entry, nil
}

// newManualSaleReceiptJournalEntry is the sale-specific alias kept for tests
// and callers that predate the rent manual receipt.
func newManualSaleReceiptJournalEntry(receipt *transaction.Transaction, ownerID string) (*journal.Entry, error) {
	return newManualReceiptJournalEntry(receipt, ownerID)
}
