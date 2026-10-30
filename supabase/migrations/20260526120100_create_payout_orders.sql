-- financial.payout_orders — manual split / commission payment orders.
--
-- Abacate Pay v2 has no native split. When a charge is paid we still need to
-- pay commissioned brokers, partner agencies and royalty payees. The
-- PayoutOrchestrationService inspects the contract (CommissionPct, broker
-- participations) and pending partner_royalties to materialize one payout
-- order per recipient, in status PENDING_REVIEW.
--
-- Operators review, optionally group several orders into a single
-- consolidated payment (via group_id), approve, then settle off-platform
-- (PIX, TED). After settling they upload a proof and mark the order PAID.
--
-- State machine (enforced in domain):
--   DRAFT -> PENDING_REVIEW -> APPROVED -> PAID
--                                       \-> FAILED
--   any non-PAID -> CANCELLED
--
-- Domain: toca-financial/internal/domain/payout_order/payout_order.go
-- Service: toca-financial/internal/service/payout_order_service.go
-- Orchestration: toca-financial/internal/service/payout_orchestration_service.go

CREATE TABLE IF NOT EXISTS financial.payout_orders (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_tenant
    ON financial.payout_orders (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_tenant_status
    ON financial.payout_orders (tenant_id, ((metadata->>'status')))
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_source
    ON financial.payout_orders (tenant_id,
                                 ((metadata->>'source_type')),
                                 ((metadata->>'source_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_recipient
    ON financial.payout_orders (tenant_id,
                                 ((metadata->>'recipient_type')),
                                 ((metadata->>'recipient_id')))
    WHERE deleted_at IS NULL AND (metadata->>'recipient_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_group
    ON financial.payout_orders (tenant_id, ((metadata->>'group_id')))
    WHERE deleted_at IS NULL AND (metadata->>'group_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_external_id
    ON financial.payout_orders ((metadata->>'external_id'))
    WHERE deleted_at IS NULL AND (metadata->>'external_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_payout_orders_contract
    ON financial.payout_orders (tenant_id, ((metadata->>'contract_id')))
    WHERE deleted_at IS NULL AND (metadata->>'contract_id') IS NOT NULL;
