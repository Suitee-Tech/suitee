-- Add partial unique index on (tenant_id, source_event_id) for invoices
-- to prevent duplicate invoice creation via idempotency key.
-- Matches the pattern used by charges, commissions, transactions, and payout_orders.
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_invoices_source_event_id
  ON financial.invoices (tenant_id, ((metadata->>'source_event_id')))
  WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;
