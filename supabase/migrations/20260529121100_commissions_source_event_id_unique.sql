CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_commissions_source_event_id
    ON financial.commissions (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

