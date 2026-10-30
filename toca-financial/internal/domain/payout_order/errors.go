package payout_order

import "errors"

var (
	ErrPayoutOrderAlreadyExists         = errors.New("payout order already exists")
	ErrPayoutOrderNotFound              = errors.New("payout order not found")
	ErrConcurrentTransition             = errors.New("payout order changed concurrently")
	ErrTransitionCompareAndSetMissing   = errors.New("payout repository does not support compare-and-set transitions")
	ErrTransitionTransactionMissing     = errors.New("payout repository does not support atomic linked-obligation cascade")
	ErrRoyaltyServiceTransactionMissing = errors.New("payout repository does not support cross-tenant royalty transitions")
)
