-- Migration: Add deleted_at column to all financial.* tables.
-- The JSONB-pilot tables (20231026000000) were created without deleted_at,
-- but every repository soft-deletes via `deleted_at IS NULL` and `SET deleted_at = NOW()`.
-- This was causing runtime errors on every CRUD operation.
--
-- Tables covered:
--   financial.transactions   financial.commissions         financial.charges
--   financial.transfers      financial.collections         financial.payments
--   financial.invoices       financial.rental_charge_configs
--   financial.fiscal_deadlines (20260729195000 - already has deleted_at)
--   financial.sale_billing_plan (20260526120000 - needs check)
--   financial.sale_billing_plan_drafts (20260528120000 - needs check)
--   financial.utility_accounts (20260513120000 - needs check)
--   financial.reminder_rules (20260513120100 - needs check)
--   financial.payout_orders (20260526120100 - needs check)
--   financial.withdrawals (20260522120300 - needs check)

-- ============================================================
-- Step 1: Add deleted_at to core financial tables (from init)
-- ============================================================
ALTER TABLE IF EXISTS financial.transactions
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.commissions
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.charges
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.transfers
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.collections
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.invoices
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.payments
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.rental_charge_configs
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

-- ============================================================
-- Step 2: Add deleted_at to tables created in later migrations
-- ============================================================
ALTER TABLE IF EXISTS financial.sale_billing_plan
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.sale_billing_plan_drafts
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.utility_accounts
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.reminder_rules
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.payout_orders
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.withdrawals
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.partner_royalties
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.tenant_commission_rules
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

-- ============================================================
-- Step 3: Recreate unique indexes that depended on deleted_at
-- (These were created in migrations 20260529* but the column
--  didn't exist yet, so they likely failed silently)
-- ============================================================

-- financial.transactions: source_event_id uniqueness
DROP INDEX IF EXISTS financial.uq_finance_transactions_source_event_id;
CREATE UNIQUE INDEX uq_finance_transactions_source_event_id
    ON financial.transactions (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- financial.commissions: source_event_id uniqueness
DROP INDEX IF EXISTS financial.uq_finance_commissions_source_event_id;
CREATE UNIQUE INDEX uq_finance_commissions_source_event_id
    ON financial.commissions (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- financial.charges: source_event_id uniqueness (from 20260518120000)
DROP INDEX IF EXISTS financial.uq_finance_charges_source_event_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_charges_source_event_id
    ON financial.charges (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- financial.payout_orders: source_event_id uniqueness (from 20260529121200)
DROP INDEX IF EXISTS financial.uq_finance_payout_orders_source_event_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_payout_orders_source_event_id
    ON financial.payout_orders (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- financial.partner_royalties: source_event_id uniqueness (from 20260529121300)
DROP INDEX IF EXISTS financial.uq_finance_partner_royalties_source_event_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_partner_royalties_source_event_id
    ON financial.partner_royalties (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- financial.invoices: source_event_id uniqueness (from 20260731000000)
DROP INDEX IF EXISTS financial.uq_finance_invoices_source_event_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_invoices_source_event_id
    ON financial.invoices (tenant_id, ((metadata->>'source_event_id')))
    WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

-- financial.withdrawals: external_id uniqueness (from 20260529160000)
DROP INDEX IF EXISTS financial.uq_finance_withdrawals_external_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_withdrawals_external_id
    ON financial.withdrawals (tenant_id, ((metadata->>'external_id')))
    WHERE deleted_at IS NULL AND (metadata->>'external_id') IS NOT NULL;

-- ============================================================
-- Step 4: Partial index on deleted_at for common filtered queries
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_financial_transactions_active
    ON financial.transactions (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_financial_commissions_active
    ON financial.commissions (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_financial_charges_active
    ON financial.charges (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_financial_transfers_active
    ON financial.transfers (tenant_id, created_at DESC)
    WHERE deleted_at IS NULL;
