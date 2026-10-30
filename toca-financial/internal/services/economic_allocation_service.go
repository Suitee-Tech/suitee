package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

type paymentAllocationSettings interface {
	Get(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error)
}

// EconomicAllocationService materializes the economic destination of every
// cent received for an itemized charge. It deliberately has no repository of
// its own: allocations stay append-only inside Charge.Payments and therefore
// commit in the same JSONB update as the payment command.
type EconomicAllocationService struct {
	settings       paymentAllocationSettings
	intermediation rentalIntermediationAllocator
}

type rentalIntermediationAllocator interface {
	FundPayment(
		ctx context.Context,
		tenantID, contractID, paymentID, chargeID, referenceMonth string,
		availableCents int64,
		fundedAt time.Time,
	) (rental_charge_config.IntermediationFundingEntry, error)
}

func NewEconomicAllocationService(settings paymentAllocationSettings) *EconomicAllocationService {
	return &EconomicAllocationService{settings: settings}
}

func (s *EconomicAllocationService) WithRentalIntermediation(intermediation rentalIntermediationAllocator) *EconomicAllocationService {
	s.intermediation = intermediation
	return s
}

type itemPaymentShare struct {
	index     int
	cents     int64
	remainder int64
}

// BuildPaymentAllocations applies proportional_v1 across the unpaid charge
// items, using largest remainders to conserve cents. On RENT, the configured
// administration percentage splits only rental_base between owner_payable and
// revenue; direct-liquidation items remain owner payables for the reserved
// owner-balance bucket.
func (s *EconomicAllocationService) BuildPaymentAllocations(
	ctx context.Context,
	c *charge.Charge,
	paymentID string,
	paymentCents int64,
) (charge.AllocationMethod, []charge.Allocation, error) {
	return s.buildPaymentAllocations(ctx, c, paymentID, paymentCents, false)
}

// RebuildPaymentAllocations is the repair variant used when a charge now
// carries its economic composition but an existing payment was recorded
// before it existed (zero allocations). It tolerates historical unclassified
// cash so the repair can derive the destination split for that same payment.
func (s *EconomicAllocationService) RebuildPaymentAllocations(
	ctx context.Context,
	c *charge.Charge,
	paymentID string,
	paymentCents int64,
) (charge.AllocationMethod, []charge.Allocation, error) {
	return s.buildPaymentAllocations(ctx, c, paymentID, paymentCents, true)
}

func (s *EconomicAllocationService) buildPaymentAllocations(
	ctx context.Context,
	c *charge.Charge,
	paymentID string,
	paymentCents int64,
	tolerateUnallocatedHistory bool,
) (charge.AllocationMethod, []charge.Allocation, error) {
	if c == nil {
		return "", nil, errors.New("charge is nil")
	}
	if strings.TrimSpace(paymentID) == "" {
		return "", nil, errors.New("payment id is required")
	}
	if paymentCents <= 0 {
		return "", nil, errors.New("payment amount must be positive")
	}
	if len(c.LineItems) == 0 {
		return "", nil, nil
	}
	// This also backfills stable IDs/defaults on itemized rows created before
	// these fields became mandatory. The updated charge is persisted together
	// with the payment below.
	if err := c.SetLineItems(c.LineItems); err != nil {
		return "", nil, fmt.Errorf("normalize charge items: %w", err)
	}

	knownItems := make(map[string]struct{}, len(c.LineItems))
	for _, item := range c.LineItems {
		knownItems[item.ID] = struct{}{}
	}
	usedByItem := make(map[string]int64, len(c.LineItems))
	adminRevenueByItem := make(map[string]int64, len(c.LineItems))
	for paymentIndex, payment := range c.Payments {
		if !tolerateUnallocatedHistory && payment.AmountCents > 0 && len(payment.Allocations) == 0 {
			// Do not mix historical unclassified cash with a new allocation
			// policy: a backfill needs an explicit operator decision.
			return "", nil, nil
		}
		if len(payment.Allocations) > 0 && !charge.IsValidAllocationMethod(payment.AllocationMethod) {
			return "", nil, fmt.Errorf("payments[%d] has invalid allocation_method %q", paymentIndex, payment.AllocationMethod)
		}
		if err := charge.ValidateAllocations(payment.Allocations, payment.AmountCents); err != nil {
			return "", nil, fmt.Errorf("payments[%d] has invalid allocations: %w", paymentIndex, err)
		}
		for _, allocation := range payment.Allocations {
			if allocation.PaymentID != payment.ID {
				return "", nil, fmt.Errorf("allocation %s does not belong to payment %s", allocation.ID, payment.ID)
			}
			if _, ok := knownItems[allocation.ChargeItemID]; !ok {
				return "", nil, fmt.Errorf("allocation %s references unknown charge item %s", allocation.ID, allocation.ChargeItemID)
			}
			usedByItem[allocation.ChargeItemID] += allocation.AmountCents
			if allocation.AllocationType == charge.AllocationTypeRevenue &&
				(allocation.Purpose == "" || allocation.Purpose == charge.AllocationPurposeAdminFee) {
				adminRevenueByItem[allocation.ChargeItemID] += allocation.AmountCents
			}
		}
	}

	shares, err := proportionalItemShares(c.LineItems, usedByItem, paymentCents)
	if err != nil {
		return "", nil, err
	}

	adminFeePct := float64(0)
	explicitAdminRevenue := hasExplicitAdminRevenueItem(c)
	if c.Type == charge.TypeRent && !explicitAdminRevenue && hasRentalBaseItem(c) {
		if s.settings == nil {
			return "", nil, errors.New("financial settings are required to allocate rent")
		}
		settings, settingsErr := s.settings.Get(ctx, c.TenantID)
		if settingsErr != nil {
			return "", nil, fmt.Errorf("load tenant financial settings: %w", settingsErr)
		}
		if settings == nil || settings.AdminFeePct < 0 || settings.AdminFeePct > 100 {
			return "", nil, errors.New("tenant admin fee percentage is invalid")
		}
		adminFeePct = settings.AdminFeePct
	}

	allocations := make([]charge.Allocation, 0, len(c.LineItems)+1)
	for _, share := range shares {
		if share.cents == 0 {
			continue
		}
		item := c.LineItems[share.index]
		ruleVersionID := allocationRuleVersionID(item.RuleVersionID, adminFeePct, explicitAdminRevenue)
		if c.Type == charge.TypeRent && item.SourceType == "rental_base" {
			cumulativeItemCents := usedByItem[item.ID] + share.cents
			targetAdminRevenueCents := percentageCents(cumulativeItemCents, adminFeePct)
			adminRevenueCents := targetAdminRevenueCents - adminRevenueByItem[item.ID]
			if adminRevenueCents < 0 || adminRevenueCents > share.cents {
				return "", nil, fmt.Errorf("administration allocation does not close for item %s", item.ID)
			}
			availableAfterAdminCents := share.cents - adminRevenueCents
			funding := rental_charge_config.IntermediationFundingEntry{}
			if s.intermediation != nil && availableAfterAdminCents > 0 {
				funding, err = s.intermediation.FundPayment(
					ctx, c.TenantID, c.ContractID, paymentID, c.ID, c.ReferenceMonth,
					availableAfterAdminCents, time.Now().UTC(),
				)
				if err != nil {
					return "", nil, fmt.Errorf("fund rental intermediation: %w", err)
				}
			}
			if funding.AppliedCents < 0 || funding.AppliedCents > availableAfterAdminCents {
				return "", nil, fmt.Errorf("intermediation allocation does not close for item %s", item.ID)
			}
			ownerCents := availableAfterAdminCents - funding.AppliedCents
			if ownerCents > 0 {
				allocation, allocationErr := charge.NewAllocationWithPurpose(
					paymentID, item.ID, item.EconomicOwnerID, charge.AllocationTypeOwnerPayable,
					ownerCents, ruleVersionID, charge.AllocationPurposeOwnerRent, "",
				)
				if allocationErr != nil {
					return "", nil, allocationErr
				}
				allocations = append(allocations, *allocation)
			}
			if adminRevenueCents > 0 {
				allocation, allocationErr := charge.NewAllocationWithPurpose(
					paymentID, item.ID, c.TenantID, charge.AllocationTypeRevenue,
					adminRevenueCents, ruleVersionID, charge.AllocationPurposeAdminFee, "",
				)
				if allocationErr != nil {
					return "", nil, allocationErr
				}
				allocations = append(allocations, *allocation)
			}
			for _, brokerFunding := range funding.BrokerFunding {
				allocation, allocationErr := charge.NewAllocationWithPurpose(
					paymentID, item.ID, c.TenantID, charge.AllocationTypeRevenue,
					brokerFunding.AmountCents, ruleVersionID,
					charge.AllocationPurposeRentalIntermediationBroker, brokerFunding.CommissionID,
				)
				if allocationErr != nil {
					return "", nil, allocationErr
				}
				allocations = append(allocations, *allocation)
			}
			if funding.AgencyRetainedCents > 0 {
				allocation, allocationErr := charge.NewAllocationWithPurpose(
					paymentID, item.ID, c.TenantID, charge.AllocationTypeRevenue,
					funding.AgencyRetainedCents, ruleVersionID,
					charge.AllocationPurposeRentalIntermediationAgency, "",
				)
				if allocationErr != nil {
					return "", nil, allocationErr
				}
				allocations = append(allocations, *allocation)
			}
			continue
		}

		allocationType := allocationTypeForItem(c, item)
		purpose := charge.AllocationPurpose("")
		if allocationType == charge.AllocationTypeRevenue && strings.EqualFold(strings.TrimSpace(item.SourceID), "admin_fee") {
			purpose = charge.AllocationPurposeAdminFee
		}
		allocation, allocationErr := charge.NewAllocationWithPurpose(paymentID, item.ID, item.EconomicOwnerID, allocationType, share.cents, ruleVersionID, purpose, "")
		if allocationErr != nil {
			return "", nil, allocationErr
		}
		allocations = append(allocations, *allocation)
	}
	if err := charge.ValidateAllocations(allocations, paymentCents); err != nil {
		return "", nil, err
	}
	return charge.AllocationMethodProportionalV1, allocations, nil
}

func proportionalItemShares(items []charge.LineItem, usedByItem map[string]int64, paymentCents int64) ([]itemPaymentShare, error) {
	remainingTotal := int64(0)
	shares := make([]itemPaymentShare, len(items))
	for i, item := range items {
		used := usedByItem[item.ID]
		if used < 0 || used > item.AmountCents {
			return nil, fmt.Errorf("charge item %s has allocations above its amount", item.ID)
		}
		remainingTotal += item.AmountCents - used
		shares[i].index = i
	}
	if paymentCents > remainingTotal {
		return nil, fmt.Errorf("payment has %d cents but only %d itemized cents remain; an explicit adjustment rule is required", paymentCents, remainingTotal)
	}
	if remainingTotal <= 0 {
		return nil, errors.New("itemized charge has no allocatable balance")
	}

	allocated := int64(0)
	for i, item := range items {
		outstanding := item.AmountCents - usedByItem[item.ID]
		if paymentCents != 0 && outstanding > math.MaxInt64/paymentCents {
			return nil, errors.New("payment allocation exceeds supported integer range")
		}
		numerator := paymentCents * outstanding
		shares[i].cents = numerator / remainingTotal
		shares[i].remainder = numerator % remainingTotal
		allocated += shares[i].cents
	}

	order := make([]int, len(shares))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(i, j int) bool {
		return shares[order[i]].remainder > shares[order[j]].remainder
	})
	for residual := paymentCents - allocated; residual > 0; residual-- {
		shares[order[int(paymentCents-allocated-residual)%len(order)]].cents++
	}
	return shares, nil
}

func percentageCents(baseCents int64, percentage float64) int64 {
	return int64(math.Round(float64(baseCents) * percentage / 100))
}

func allocationTypeForItem(c *charge.Charge, item charge.LineItem) charge.AllocationType {
	if item.AllocationType != "" {
		return item.AllocationType
	}
	switch item.EconomicOwnerID {
	case c.TenantID:
		return charge.AllocationTypeRevenue
	case c.OwnerID:
		return charge.AllocationTypeOwnerPayable
	default:
		return charge.AllocationTypePartnerPayable
	}
}

func hasRentalBaseItem(c *charge.Charge) bool {
	for _, item := range c.LineItems {
		if item.SourceType == "rental_base" {
			return true
		}
	}
	return false
}

func hasExplicitAdminRevenueItem(c *charge.Charge) bool {
	for _, item := range c.LineItems {
		if item.SourceID == "admin_fee" && item.EconomicOwnerID == c.TenantID {
			return true
		}
	}
	return false
}

func allocationRuleVersionID(itemRuleVersionID string, adminFeePct float64, explicitAdminRevenue bool) string {
	key := strings.Join([]string{
		"toca/payment-allocation-rule/v2-rental-intermediation-carry-forward",
		strings.TrimSpace(itemRuleVersionID),
		string(charge.AllocationMethodProportionalV1),
		strconv.FormatFloat(adminFeePct, 'f', 6, 64),
		strconv.FormatBool(explicitAdminRevenue),
	}, "|")
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}
