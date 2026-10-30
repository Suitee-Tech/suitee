package partner_royalty

import "errors"

var (
	ErrPartnerRoyaltyAlreadyExists = errors.New("partner royalty already exists")
	ErrConcurrentTransition        = errors.New("partner royalty changed concurrently")
	ErrPayoutLinkMismatch          = errors.New("partner royalty is linked to a different payout order")
	ErrPayoutSourceMismatch        = errors.New("payout order does not match partner royalty")
)
