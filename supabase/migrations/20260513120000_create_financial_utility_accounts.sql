-- Phase 5 (rental ops) — utility/service accounts (água, energia, gás,
-- condomínio, IPTU, internet, seguro) attached to a rental contract. The
-- recurring bills themselves still flow through financial.charges; this
-- table is the configuration catalog used by the admin UI in Rentals →
-- Contas de Consumo.
--
-- See domain in toca-financial/internal/domain/utility_account for the
-- full struct shape. JSONB metadata mirrors the same pattern as
-- financial.rentals / financial.rental_charge_configs.

CREATE TABLE IF NOT EXISTS financial.utility_accounts (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE INDEX IF NOT EXISTS idx_finance_utility_accounts_tenant
    ON financial.utility_accounts (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_utility_accounts_tenant_contract
    ON financial.utility_accounts (tenant_id, ((metadata->>'contract_id')))
    WHERE deleted_at IS NULL AND (metadata->>'contract_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_utility_accounts_tenant_property
    ON financial.utility_accounts (tenant_id, ((metadata->>'property_id')))
    WHERE deleted_at IS NULL AND (metadata->>'property_id') IS NOT NULL;
