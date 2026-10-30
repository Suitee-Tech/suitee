package services

import (
	"context"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
)

// ChainedContractHandler composes the contract.signed pipeline as a
// sequence of independent sub-handlers. Each sub-handler owns its own
// dependency surface (see charge_issuance_handler.go,
// owner_transfer_handler.go, broker_commission_handler.go,
// cross_company_handler.go) so each one can be unit-tested in isolation.
//
// Sequence + failure policy:
//
//  1. ChargeIssuance — fail-fast for every persisted rental/sale row.
//  2. OwnerTransfer — fail-fast. Without the transfer row the owner-side
//     ledger is incomplete and downstream financial reporting breaks.
//  3. CrossCompany — fail-fast for every monetary leg. Runs BEFORE broker
//     commission and reduces p.CommissionPct only after its own writes.
//  4. BrokerCommission — fail-fast for Transaction and Commission writes.
//     Receives the already-reduced p.CommissionPct from CrossCompany.
//  5. PayoutProvisioning — fail-fast. Creates payout_orders linked to
//     commissions; a failed obligation keeps the broker message retryable.
//
// No monetary sub-step is best-effort: production wraps this chain in one SQL
// transaction, so a delivery either materializes every obligation or none.
type ChainedContractHandler struct {
	chargeIssuance     *ChargeIssuanceHandler
	ownerTransfer      *OwnerTransferHandler
	brokerCommission   *BrokerCommissionHandler
	crossCompany       *CrossCompanyHandler
	payoutProvisioning *PayoutProvisioningStep
	db                 *database.PostgresDB
}

// NewChainedContractHandler composes the sub-handlers. crossCompany
// may be nil — production wiring builds one even when the cross-repo is
// missing (the handler is a no-op in that case).
func NewChainedContractHandler(
	chargeIssuance *ChargeIssuanceHandler,
	ownerTransfer *OwnerTransferHandler,
	brokerCommission *BrokerCommissionHandler,
	crossCompany *CrossCompanyHandler,
) *ChainedContractHandler {
	return &ChainedContractHandler{
		chargeIssuance:   chargeIssuance,
		ownerTransfer:    ownerTransfer,
		brokerCommission: brokerCommission,
		crossCompany:     crossCompany,
	}
}

// WithDB enables transactional execution. When set, Handle() wraps all
// pipeline steps in a single SQL transaction: if any step fails, every
// preceding write is rolled back atomically. Without it, steps commit
// independently (legacy behaviour).
func (h *ChainedContractHandler) WithDB(db *database.PostgresDB) *ChainedContractHandler {
	h.db = db
	return h
}

// WithPayoutProvisioning injects the payout step into the chain. When set,
// payout_orders are created inside the same transaction as the rest of the
// pipeline, removing the split-brain risk between ledger and payout.
func (h *ChainedContractHandler) WithPayoutProvisioning(step *PayoutProvisioningStep) *ChainedContractHandler {
	h.payoutProvisioning = step
	return h
}

// Handle runs the pipeline inside a single SQL transaction when WithDB
// was called. Returns the first error from any sub-handler.
func (h *ChainedContractHandler) Handle(ctx context.Context, in ContractFinancialsInput) error {
	if h.db == nil {
		return h.handleSteps(ctx, in)
	}
	return h.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		return h.handleSteps(database.WithTx(ctx, tx), in)
	})
}

// handleSteps executes the 4 pipeline steps sequentially.
func (h *ChainedContractHandler) handleSteps(ctx context.Context, in ContractFinancialsInput) error {
	// Trace ID = event ID para correlação de todos os logs do pipeline
	traceFields := map[string]interface{}{
		"trace_id":       in.EventID,
		"deal_id":        in.Payload.DealID,
		"contract_id":    in.Payload.ContractID,
		"deal_type":      in.Payload.Type,
		"deal_value":     in.Payload.Value,
		"commission_pct": in.Payload.CommissionPct,
	}
	logger.Info("Commission pipeline started", traceFields)

	if err := h.runStep(ctx, in, "charge_issuance", h.chargeIssuance.Handle); err != nil {
		logger.Error("Commission pipeline aborted at charge_issuance", mergeFields(traceFields, "error", err.Error()))
		return err
	}
	if err := h.runStep(ctx, in, "owner_transfer", h.ownerTransfer.Handle); err != nil {
		logger.Error("Commission pipeline aborted at owner_transfer", mergeFields(traceFields, "error", err.Error()))
		return err
	}
	if h.crossCompany != nil {
		if err := h.runStep(ctx, in, "cross_company", h.crossCompany.Handle); err != nil {
			logger.Error("Commission pipeline aborted at cross_company", mergeFields(traceFields, "error", err.Error()))
			return err
		}
	}
	if err := h.runStep(ctx, in, "broker_commission", h.brokerCommission.Handle); err != nil {
		logger.Error("Commission pipeline aborted at broker_commission", mergeFields(traceFields, "error", err.Error()))
		return err
	}
	if h.payoutProvisioning != nil {
		if err := h.runStep(ctx, in, "payout_provisioning", h.payoutProvisioning.Handle); err != nil {
			logger.Error("Commission pipeline aborted at payout_provisioning", mergeFields(traceFields, "error", err.Error()))
			return err
		}
	}

	logger.Info("Commission pipeline completed successfully", traceFields)
	return nil
}

// mergeFields creates a shallow copy of fields and adds a key-value pair.
func mergeFields(fields map[string]interface{}, k string, v interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(fields)+1)
	for key, val := range fields {
		merged[key] = val
	}
	merged[k] = v
	return merged
}

// runStep treats every error as fatal. There is no best-effort step left in
// the chain: swallowing a failure would commit a partial financial graph, so
// the surrounding SQL transaction rolls back and the event is redelivered.
func (h *ChainedContractHandler) runStep(ctx context.Context, in ContractFinancialsInput, _ string, run func(context.Context, ContractFinancialsInput) error) error {
	return run(ctx, in)
}
