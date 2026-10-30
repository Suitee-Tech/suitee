package services

import (
	"context"
	"time"

	"github.com/oginoino/toca/pkg/events/contracts"
)

// ContractFinancialsInput is the resolved input passed across the
// chained contract.signed sub-handlers. The raw event payload is parsed
// once at the entry point and the dates are normalised so each handler
// can stay focused on its own write side-effects.
type ContractFinancialsInput struct {
	EventID      string
	Payload      *contracts.ContractSignedEventPayload
	StartDate    time.Time
	EndDate      time.Time
	CrossCompany *CrossCompanySettlement
	// CommissionPlan is allocated by the event adapter and populated by the
	// broker step after cross-company adjustments. The pointer lets the payout
	// step consume the exact same resolved rules despite this input being passed
	// by value through the chain.
	CommissionPlan *ResolvedCommissionPlan
}

// CrossCompanySettlement is populated by the cross-company chain step and
// consumed by payout orchestration in the same contract.signed execution.
// Keeping it on a shared pointer preserves the information across handlers
// even though ContractFinancialsInput itself is passed by value.
//
// OwnerAmountCents is the owner-company commission share in integer cents.
// OwnerPercentage/ClosingPercentage/RoyaltyPercentage/RoyaltyBaseMode carry
// the partnership context so the broker step can fold it into the PlanHash
// and keep preview and materialisation fingerprints identical.
type CrossCompanySettlement struct {
	FeePayerTenant     string
	PartnershipID      string
	RecipientTenant    string
	OwnerPercentage    float64
	ClosingPercentage  float64
	OwnerAmountCents   int64
	RoyaltyPercentage  float64
	RoyaltyBaseMode    string
	RoyaltyAmountCents int64
	RoyaltyPayeeTenant string
}

// ContractFinancialsHandler is the per-step interface the chained
// contract.signed pipeline composes. Each implementation MUST be
// self-contained on its own dependency surface so a unit test can mock
// just the relevant repositories.
type ContractFinancialsHandler interface {
	Handle(ctx context.Context, in ContractFinancialsInput) error
}

// The contract.signed pipeline has no best-effort step: production wraps
// the whole chain in a single SQL transaction (see ChainedContractHandler
// and WithDB). A failed write therefore rolls back every obligation and the
// delivery is retried, instead of leaving a partial graph for operators to
// backfill by hand. There is no typed "best-effort" error anymore.
