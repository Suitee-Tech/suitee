-- Régua de Comunicação — declarative rules consumed by the daily reminder
-- engine in toca-financial. Domain lives at
-- toca-financial/internal/domain/reminder_rule/reminder_rule.go.
--
-- Why JSONB metadata: matches the rest of the financial schema and lets us
-- evolve the rule shape (channels, template variables, scoping) without
-- migrations. Composite indexes pull tenant + enabled + trigger filters
-- without table scans.

CREATE TABLE IF NOT EXISTS financial.reminder_rules (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_reminder_rules_tenant
    ON financial.reminder_rules (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_reminder_rules_tenant_enabled
    ON financial.reminder_rules (tenant_id, ((metadata->>'enabled')))
    WHERE deleted_at IS NULL;
