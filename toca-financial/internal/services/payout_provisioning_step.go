package services

import (
	"context"
	"fmt"
)

// PayoutProvisioningStep is the 5th and final step of the contract.signed
// pipeline. It wraps PayoutOrchestrationService.OnContractSigned so that
// payout_orders are created inside the same DB transaction as the ledger
// and commission rows.
//
// Fail-fast policy: a missing payout is a missing monetary obligation. The
// broker delivery must be retried (and, when WithDB is enabled, rolled back)
// instead of being acknowledged with only an audit breadcrumb.
type PayoutProvisioningStep struct {
	orchestration *PayoutOrchestrationService
}

// NewPayoutProvisioningStep creates the chain adapter for payout.
func NewPayoutProvisioningStep(orchestration *PayoutOrchestrationService) *PayoutProvisioningStep {
	return &PayoutProvisioningStep{orchestration: orchestration}
}

// Handle delegates to PayoutOrchestrationService.OnContractSigned.
func (s *PayoutProvisioningStep) Handle(ctx context.Context, in ContractFinancialsInput) error {
	if s.orchestration == nil {
		return fmt.Errorf("payout orchestration service is required")
	}
	if err := s.orchestration.OnContractSigned(ctx, in); err != nil {
		return fmt.Errorf("payout provisioning failed for contract %s: %w", in.Payload.ContractID, err)
	}
	return nil
}
