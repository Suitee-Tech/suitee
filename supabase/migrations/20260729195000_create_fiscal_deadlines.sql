-- Calendar entries presented by GET /api/v1/fiscal/calendar.
-- The aggregate is stored in metadata JSONB to follow the established
-- financial schema pattern while tenant ownership remains a typed column.

CREATE TABLE IF NOT EXISTS financial.fiscal_deadlines (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_fiscal_deadlines_tenant
    ON financial.fiscal_deadlines (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_fiscal_deadlines_tenant_due_date
    ON financial.fiscal_deadlines (tenant_id, ((metadata->>'due_date')))
    WHERE deleted_at IS NULL AND (metadata->>'due_date') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_fiscal_deadlines_tenant_status
    ON financial.fiscal_deadlines (tenant_id, ((metadata->>'status')))
    WHERE deleted_at IS NULL AND (metadata->>'status') IS NOT NULL;

ALTER TABLE financial.fiscal_deadlines ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation ON financial.fiscal_deadlines;
CREATE POLICY tenant_isolation ON financial.fiscal_deadlines
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
