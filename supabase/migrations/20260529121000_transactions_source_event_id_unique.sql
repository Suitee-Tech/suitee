CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transactions_source_event_id
    ON financial.transactions (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

