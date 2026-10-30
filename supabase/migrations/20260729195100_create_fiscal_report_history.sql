-- Durable audit trail for generated DIMOB, NFS-e and income reports exposed
-- by GET /api/v1/fiscal/reports.

CREATE TABLE IF NOT EXISTS financial.report_history (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_report_history_tenant
    ON financial.report_history (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_report_history_tenant_generated_at
    ON financial.report_history (tenant_id, ((metadata->>'generated_at')) DESC)
    WHERE deleted_at IS NULL AND (metadata->>'generated_at') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_report_history_tenant_type
    ON financial.report_history (tenant_id, ((metadata->>'type')))
    WHERE deleted_at IS NULL AND (metadata->>'type') IS NOT NULL;

ALTER TABLE financial.report_history ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON financial.report_history;
CREATE POLICY tenant_isolation ON financial.report_history
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );
