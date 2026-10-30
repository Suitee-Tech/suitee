-- Add partial unique index on (tenant_id, source_event_id) for collections
-- to prevent duplicate collection creation via idempotency key.
-- Matches the pattern used by charges, commissions, transactions,
-- invoices, and payout_orders.
CREATE UNIQUE INDEX IF NOT EXISTS uq_financial_collections_source_event_id
  ON financial.collections (tenant_id, ((metadata->>'source_event_id')))
  WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;
