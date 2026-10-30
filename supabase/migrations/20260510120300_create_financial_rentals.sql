-- Phase 2 (item 2.6) — consolidated Rental aggregate. See domain in
-- toca-financial/internal/domain/rental/rental.go for the full struct
-- shape; this migration only creates the storage table + indexes.
--
-- Why this isn't `rental_charge_configs`: that table holds the *recurring
-- billing config* (cadence, late fees, dia de vencimento, valor base). The
-- new `rentals` table holds the *aggregate* — owner/tenant leads, deal+
-- contract refs, lifecycle status. The two compose: a Rental references a
-- ChargeConfig via contract_id.

CREATE TABLE IF NOT EXISTS financial.rentals (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   text NOT NULL,
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

-- Most queries are tenant-scoped lookups by deal_id, contract_id, property_id,
-- tenant_lead_id or owner_lead_id. Composite indexes on the metadata expressions.

CREATE INDEX IF NOT EXISTS idx_finance_rentals_tenant
    ON financial.rentals (tenant_id)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_finance_rentals_tenant_deal
    ON financial.rentals (tenant_id, ((metadata->>'deal_id')))
    WHERE deleted_at IS NULL AND (metadata->>'deal_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_rentals_tenant_contract
    ON financial.rentals (tenant_id, ((metadata->>'contract_id')))
    WHERE deleted_at IS NULL AND (metadata->>'contract_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_rentals_tenant_property
    ON financial.rentals (tenant_id, ((metadata->>'property_id')))
    WHERE deleted_at IS NULL AND (metadata->>'property_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_rentals_tenant_tenant_lead
    ON financial.rentals (tenant_id, ((metadata->>'tenant_lead_id')))
    WHERE deleted_at IS NULL AND (metadata->>'tenant_lead_id') IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_finance_rentals_tenant_owner_lead
    ON financial.rentals (tenant_id, ((metadata->>'owner_lead_id')))
    WHERE deleted_at IS NULL AND (metadata->>'owner_lead_id') IS NOT NULL;
