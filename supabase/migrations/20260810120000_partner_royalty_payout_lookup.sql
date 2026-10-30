-- Scope the contract.signed royalty payout lookup to the paying tenant, exact
-- contract and lifecycle state. This prevents a reused deal_id (or a settled
-- royalty) from being materialized into another payout order.
BEGIN;

CREATE INDEX IF NOT EXISTS idx_finance_partner_royalties_payout_lookup
    ON financial.partner_royalties (payer_tenant, contract_id, status, deal_id)
    WHERE deleted_at IS NULL;

-- Keep the physical unique index identical to payout_order_postgres.Create's
-- ON CONFLICT inference predicate. The deleted_at backfill migration recreated
-- this index without the CANCELLED predicate, which makes PostgreSQL reject the
-- targeted ON CONFLICT clause and also prevents a cancelled obligation from
-- being intentionally reprovisioned.
DROP INDEX IF EXISTS financial.uq_finance_payout_orders_source_event_id;
CREATE UNIQUE INDEX uq_finance_payout_orders_source_event_id
    ON financial.payout_orders (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL
      AND (metadata->>'source_event_id') IS NOT NULL
      AND COALESCE((metadata->>'status'), '') <> 'CANCELLED';

COMMIT;
