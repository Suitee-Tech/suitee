CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_withdrawals_external_id
    ON financial.withdrawals (tenant_id, ((metadata->>'external_id')))
    WHERE deleted_at IS NULL AND (metadata->>'external_id') IS NOT NULL;
