-- Platform plan subscriptions — the agency's (tenant's) subscription to a Toca
-- plan, billed via Abacate Pay. Status is tracked for an in-app warning banner
-- only; it never gates access. JSONB metadata mirrors the other financial
-- aggregates.

CREATE TABLE IF NOT EXISTS financial.platform_subscriptions (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_platform_subscriptions_tenant
    ON financial.platform_subscriptions (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_platform_subscriptions_abacate_id
    ON financial.platform_subscriptions ((metadata->>'abacate_subscription_id'))
    WHERE deleted_at IS NULL;
