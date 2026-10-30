package commission

import "errors"

var (
	ErrCommissionAlreadyExists      = errors.New("commission already exists")
	ErrDeductionConflict            = errors.New("deduction id already exists with different data")
	ErrDeductionIDRequired          = errors.New("deduction id is required")
	ErrLinkedPayoutLifecycleManaged = errors.New("commission lifecycle is managed by its linked payout order")
	ErrLinkedPayoutMismatch         = errors.New("commission is not linked to this payout order")
	ErrConcurrentTransition         = errors.New("commission changed concurrently")
	ErrLinkedPayoutSyncUnsupported  = errors.New("commission repository cannot synchronize linked payout")
	ErrLinkedPayoutNotAdjustable    = errors.New("linked payout is not pending and cannot be adjusted")
	ErrLinkedPayoutAmountNotPayable = errors.New("linked payout net amount must be greater than zero")
	ErrDeductionFrozen              = errors.New("commission is no longer pending and its deductions cannot be adjusted")
	ErrFundingIncomplete            = errors.New("commission is not fully funded by collected receipts")
	ErrFundingGateConflict          = errors.New("commission funding gate conflicts with existing commission state")
)

// ErrInvalidDeductionRate: alíquota/base/valor do desconto não fecham entre si.
var ErrInvalidDeductionRate = errors.New("deduction rate is inconsistent with its amount")
