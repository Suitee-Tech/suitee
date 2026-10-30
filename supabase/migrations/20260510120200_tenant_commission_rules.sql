-- Phase 2 (item 2.5) — replace the hardcoded 50% commission split with a
-- per-tenant rules table. Existing event_handler test fixture uses 50% for
-- captador but admins should be able to tune captador/visitador/fechamento
-- shares without a code change.
--
-- Storage uses the same JSONB-metadata pattern as the rest of toca-financial
-- so the table looks like every other one in the schema.

CREATE TABLE IF NOT EXISTS financial.tenant_commission_rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    role        text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz,
    UNIQUE (tenant_id, role)
);

CREATE INDEX IF NOT EXISTS idx_finance_commission_rules_tenant
    ON financial.tenant_commission_rules (tenant_id)
    WHERE deleted_at IS NULL;

-- The Rule shape stored under metadata:
--   {
--     "default_percentage": 30,
--     "updated_at":         "<rfc3339>"
--   }
-- We split tenant_id and role into real columns because they're the natural
-- composite key; the rest stays in metadata for forward-compat.

-- Note: defaults (captador=30, visitador=20, fechamento=50) are seeded at
-- runtime by the service when a tenant doesn't have rules yet (see
-- domain/commission/commission_rules.go DefaultPercentages). We don't seed
-- here because tenants are not enumerable from this migration.
