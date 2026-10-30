package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
)

// SaleCommissionFundingService applies only the broker pool frozen on sale
// charges at signing. It recognizes collected agency revenue and records the
// liquidity backing existing obligations; it never accrues a second payable.
// Its writes join the charge payment transaction. The contract advisory lock
// serializes receipts from different installments before reading their shared
// outstanding targets; lifecycle CAS also protects concurrent approvals.
type SaleCommissionFundingService struct {
	commission       *CommissionService
	payoutRepo       payout_order.Repository
	requirePaymentTx bool
}

func NewSaleCommissionFundingService(commissions *CommissionService, payouts payout_order.Repository) *SaleCommissionFundingService {
	return &SaleCommissionFundingService{commission: commissions, payoutRepo: payouts}
}

func (s *SaleCommissionFundingService) RequirePaymentTransaction() *SaleCommissionFundingService {
	s.requirePaymentTx = true
	return s
}

func hasSaleCommissionPool(c *charge.Charge) bool {
	if c == nil || c.Type != charge.TypeSale {
		return false
	}
	for _, item := range c.LineItems {
		if item.SourceType == "sale_commission_payable" && item.AmountCents > 0 {
			return true
		}
	}
	return false
}

// FundAllocations replaces the frozen broker pool slice with one traceable
// revenue allocation per Commission. Partial receipts carry funding across
// installments, and only fully covered payout orders leave DRAFT.
func (s *SaleCommissionFundingService) FundAllocations(ctx context.Context, c *charge.Charge, paymentID string, fundedAt time.Time, allocations []charge.Allocation) ([]charge.Allocation, error) {
	if !hasSaleCommissionPool(c) {
		return allocations, nil
	}
	if s == nil || s.commission == nil || s.payoutRepo == nil {
		return nil, errors.New("sale commission and payout funding dependencies are required")
	}
	if len(allocations) == 0 {
		return nil, errors.New("signed sale receipt requires its frozen economic allocations")
	}
	var poolItem *charge.LineItem
	for i := range c.LineItems {
		if c.LineItems[i].SourceType != "sale_commission_payable" {
			continue
		}
		if poolItem != nil {
			return nil, errors.New("sale commission pool has duplicate items")
		}
		poolItem = &c.LineItems[i]
	}
	var pool *charge.Allocation
	result := make([]charge.Allocation, 0, len(allocations))
	for i := range allocations {
		allocation := allocations[i]
		if allocation.ChargeItemID != poolItem.ID {
			result = append(result, allocation)
			continue
		}
		if pool != nil || allocation.PaymentID != paymentID || allocation.AmountCents <= 0 || allocation.BeneficiaryPartyID != c.TenantID {
			return nil, errors.New("sale commission pool allocation is invalid")
		}
		pool = &allocations[i]
	}
	// A tiny partial receipt can allocate zero cents to the broker pool.
	if pool == nil {
		return allocations, nil
	}
	if strings.TrimSpace(c.DealID) == "" || strings.TrimSpace(c.ContractID) == "" || strings.TrimSpace(poolItem.RuleVersionID) == "" {
		return nil, errors.New("sale funding requires the frozen contract and commission plan")
	}
	tx := database.TxFromCtx(ctx)
	if s.requirePaymentTx && tx == nil {
		return nil, errors.New("sale commission funding requires the charge payment transaction")
	}
	if tx != nil {
		key := c.TenantID + ":sale-commission-funding:" + c.ContractID
		if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", key); err != nil {
			return nil, fmt.Errorf("lock sale funding contract: %w", err)
		}
	}
	all, err := s.commission.ListByDeal(ctx, c.TenantID, c.DealID)
	if err != nil {
		return nil, err
	}
	obligations := make([]*commission.Commission, 0, len(all))
	for _, obligation := range all {
		if obligation == nil || obligation.ContractID != c.ContractID || obligation.TenantID != c.TenantID || obligation.PlanHash != poolItem.RuleVersionID {
			continue
		}
		if obligation.FundingRequiredCents <= 0 || obligation.PayoutOrderID == "" || obligation.Status == commission.StatusCancelled || obligation.Status == commission.StatusReversed {
			return nil, fmt.Errorf("sale commission %s requires funding reconciliation", obligation.ID)
		}
		obligations = append(obligations, obligation)
	}
	if len(obligations) == 0 {
		return nil, errors.New("no linked commission obligations match the frozen sale plan")
	}
	orders := make(map[string]*payout_order.PayoutOrder, len(obligations))
	for _, obligation := range obligations {
		if _, duplicate := orders[obligation.PayoutOrderID]; duplicate {
			return nil, errors.New("sale commissions cannot share a payout order")
		}
		order, err := s.payoutRepo.GetByID(ctx, c.TenantID, obligation.PayoutOrderID)
		if err != nil {
			return nil, err
		}
		if order == nil || order.TenantID != c.TenantID || order.ContractID != c.ContractID || order.RecipientType != payout_order.RecipientBroker || order.RecipientID != obligation.BrokerID || order.FundingRequiredCents != obligation.FundingRequiredCents {
			return nil, fmt.Errorf("sale commission %s has an inconsistent payout link", obligation.ID)
		}
		if _, err := obligation.EnableFundingGate(obligation.FundingRequiredCents); err != nil {
			return nil, err
		}
		if _, err := order.EnableFundingGate(obligation.FundingRequiredCents); err != nil {
			return nil, err
		}
		for key, expected := range map[string]string{"commission_id": obligation.ID, "deal_id": obligation.DealID, "plan_hash": obligation.PlanHash, "role": obligation.Role} {
			if value, exists := order.SourceRef[key]; exists && value != expected {
				return nil, fmt.Errorf("sale commission %s has a conflicting payout %s", obligation.ID, key)
			}
		}
		if err := validateSaleFundingHistory(obligation, order); err != nil {
			return nil, err
		}
		orders[order.ID] = order
	}
	// A stable tie order ensures deterministic largest-remainder rounding.
	sort.Slice(obligations, func(i, j int) bool { return obligations[i].ID < obligations[j].ID })
	parts := make([]int64, len(obligations))
	var replayCents int64
	for i, obligation := range obligations {
		for _, entry := range obligation.FundingEntries {
			if entry.PaymentID != paymentID {
				continue
			}
			if entry.ChargeID != c.ID || entry.ReferenceMonth != c.ReferenceMonth {
				return nil, errors.New("sale funding payment identity was reused on another receipt")
			}
			parts[i] = entry.AmountCents
			replayCents += entry.AmountCents
		}
	}
	if replayCents > 0 && replayCents != pool.AmountCents {
		return nil, errors.New("sale funding retry differs from its persisted receipt coverage")
	}
	if replayCents == 0 {
		items := make([]charge.LineItem, len(obligations))
		for i, obligation := range obligations {
			items[i] = charge.LineItem{ID: obligation.ID, AmountCents: obligation.FundingOutstandingCents()}
		}
		shares, err := proportionalItemShares(items, nil, pool.AmountCents)
		if err != nil {
			return nil, fmt.Errorf("sale broker funding exceeds outstanding obligations: %w", err)
		}
		for _, share := range shares {
			parts[share.index] = share.cents
		}
	}
	transitionRepo, ok := s.payoutRepo.(payout_order.TransitionRepository)
	if !ok {
		return nil, payout_order.ErrTransitionCompareAndSetMissing
	}
	for i, obligation := range obligations {
		amount := parts[i]
		if amount == 0 {
			continue
		}
		allocation, err := charge.NewAllocationWithPurpose(paymentID, pool.ChargeItemID, c.TenantID,
			charge.AllocationTypeRevenue, amount, pool.RuleVersionID, charge.AllocationPurposeSaleCommissionBroker, obligation.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, *allocation)
		order := orders[obligation.PayoutOrderID]
		if replayCents > 0 {
			matched := false
			for _, entry := range order.FundingEntries {
				if entry.PaymentID == paymentID && entry.ChargeID == c.ID && entry.ReferenceMonth == c.ReferenceMonth && entry.AmountCents == amount {
					matched = true
				}
			}
			if !matched {
				return nil, errors.New("sale commission and payout funding histories disagree")
			}
			continue
		}
		if order.FundedAmountCents != obligation.FundedAmountCents {
			return nil, errors.New("sale commission and payout funding balances disagree")
		}
		expectedStatus, expectedUpdatedAt := order.Status, order.UpdatedAt
		if _, err := order.RecordFunding(payout_order.FundingEntry{PaymentID: paymentID, ChargeID: c.ID, ReferenceMonth: c.ReferenceMonth, AmountCents: amount, FundedAt: fundedAt}); err != nil {
			return nil, err
		}
		if _, err := s.commission.RecordRentalFunding(ctx, c.TenantID, obligation.ID, commission.FundingEntry{PaymentID: paymentID, ChargeID: c.ID, ReferenceMonth: c.ReferenceMonth, AmountCents: amount, FundedAt: fundedAt}); err != nil {
			return nil, err
		}
		if err := transitionRepo.UpdateIfStatus(ctx, order, expectedStatus, expectedUpdatedAt); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// Funding history is the cash evidence behind both obligations. Check all
// links before writing, including retries, so a damaged aggregate cannot be
// repaired by silently crediting one side a second time.
func validateSaleFundingHistory(c *commission.Commission, p *payout_order.PayoutOrder) error {
	if c.FundedAmountCents != p.FundedAmountCents || len(c.FundingEntries) != len(p.FundingEntries) {
		return errors.New("sale commission and payout funding histories disagree")
	}
	entries := make(map[string]payout_order.FundingEntry, len(p.FundingEntries))
	var total int64
	for _, entry := range p.FundingEntries {
		if _, duplicate := entries[entry.PaymentID]; duplicate || entry.AmountCents <= 0 || entry.AmountCents > p.FundedAmountCents-total {
			return errors.New("sale payout funding history is invalid")
		}
		entries[entry.PaymentID] = entry
		total += entry.AmountCents
	}
	if total != c.FundedAmountCents {
		return errors.New("sale funding history does not match its balance")
	}
	for _, entry := range c.FundingEntries {
		other, ok := entries[entry.PaymentID]
		if !ok || other.ChargeID != entry.ChargeID || other.ReferenceMonth != entry.ReferenceMonth || other.AmountCents != entry.AmountCents || !other.FundedAt.Equal(entry.FundedAt) {
			return errors.New("sale commission and payout funding histories disagree")
		}
		delete(entries, entry.PaymentID)
	}
	return nil
}
