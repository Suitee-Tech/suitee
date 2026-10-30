-- Phase 2 (item 2.1) — denormalize deal_id + lead_id on charges so the
-- frontend can answer "all charges of deal X" or "lead's financial summary"
-- without joining contract→deal. Domain is in financial/internal/domain/
-- charge/charge.go; storage is JSONB so we don't need ALTER TABLE — just
-- the indexes.

CREATE INDEX IF NOT EXISTS idx_finance_charges_tenant_deal_id
    ON financial.charges (tenant_id, ((metadata->>'deal_id')))
    WHERE deleted_at IS NULL AND (metadata->>'deal_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_charges_tenant_lead_id
    ON financial.charges (tenant_id, ((metadata->>'lead_id')))
    WHERE deleted_at IS NULL AND (metadata->>'lead_id') IS NOT NULL;

-- Backfill: legacy rows may have deal_id/lead_id only inside contract→deal
-- chain; we can't fully reconstitute without a join. Where the writer
-- already stored it under a different path (`metadata->'metadata'->>'deal_id'`
-- from earlier code paths), surface it. Safe to no-op on fresh installs.
UPDATE financial.charges
   SET metadata = jsonb_set(metadata, '{deal_id}', metadata->'metadata'->'deal_id', true)
 WHERE (metadata->>'deal_id') IS NULL
   AND metadata->'metadata' ? 'deal_id';
