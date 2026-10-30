package services

import (
	"context"
	"strings"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

// SecurityDepositBalance is the read-model the rescisão modal in the front
// office consumes. Held = original deposit amount (passed in by the
// caller from the Contract). DeductionsPending + DeductionsCharged sum
// every Charge tagged as "Dedução caução" for the contract, split by
// payment status so the operator can pre-populate the form.
type SecurityDepositBalance struct {
	ContractID        string  `json:"contract_id"`
	Held              float64 `json:"held"`
	DeductionsPending float64 `json:"deductions_pending"`
	DeductionsCharged float64 `json:"deductions_charged"`
	ReturnDue         float64 `json:"return_due"`
	DeductionsCount   int     `json:"deductions_count"`
}

// ComputeSecurityDepositBalance walks every charge linked to the contract
// and aggregates the security-deposit-deduction ones. Held is what the
// caller knows from the Contract (passing it in keeps the financial
// service decoupled from the contract repository — the
// /security-deposit-balance handler resolves it via the deal-side internal
// API or accepts an explicit query param).
func (s *ChargeService) ComputeSecurityDepositBalance(ctx context.Context, tenantID, contractID string, held float64) (*SecurityDepositBalance, error) {
	if tenantID == "" || contractID == "" {
		return &SecurityDepositBalance{ContractID: contractID, Held: held, ReturnDue: held}, nil
	}
	balance := &SecurityDepositBalance{ContractID: contractID, Held: held}
	var offset int64
	const limit = int64(200)
	for {
		charges, total, err := s.repo.List(ctx, tenantID, contractID, "", "", limit, offset)
		if err != nil {
			return nil, err
		}
		for _, c := range charges {
			if c == nil {
				continue
			}
			if !isDepositDeduction(c) {
				continue
			}
			amt := c.TotalAmount.Float()
			if amt == 0 {
				amt = c.Amount.Float()
			}
			balance.DeductionsCount++
			switch c.Status {
			case charge.StatusPaid, charge.StatusPartiallyPaid:
				balance.DeductionsCharged += amt
			default:
				balance.DeductionsPending += amt
			}
		}
		offset += int64(len(charges))
		if offset >= total || len(charges) == 0 {
			break
		}
	}
	balance.ReturnDue = held - balance.DeductionsPending - balance.DeductionsCharged
	if balance.ReturnDue < 0 {
		balance.ReturnDue = 0
	}
	return balance, nil
}

// isDepositDeduction recognizes charges created by the inspection
// deductions handler. We rely on the description prefix because that's
// the only stable signal carried through the JSONB metadata today.
// The financial side could later add a dedicated metadata flag if the
// heuristic gets noisy.
func isDepositDeduction(c *charge.Charge) bool {
	if c == nil {
		return false
	}
	if c.Type != charge.TypeOther {
		return false
	}
	return strings.HasPrefix(c.Description, "Dedução caução")
}
