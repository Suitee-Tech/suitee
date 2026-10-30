CREATE EXTENSION IF NOT EXISTS pgcrypto;

ALTER TABLE IF EXISTS core.properties
  ALTER COLUMN title SET DEFAULT '',
  ALTER COLUMN status SET DEFAULT '',
  ALTER COLUMN type SET DEFAULT '';

ALTER TABLE IF EXISTS crm.leads
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS crm.roulette_history
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS crm.roulette_configs
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS deal.deals
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS deal.documents
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS deal.templates
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS deal.contracts
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.charges
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.collections
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.transfers
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.invoices
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.transactions
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS financial.commissions
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS ops.keys
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS ops.inspections
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS ops.maintenance
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

ALTER TABLE IF EXISTS ops.tickets
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_crm_leads_tenant_deleted_at ON crm.leads(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_crm_roulette_history_tenant_deleted_at ON crm.roulette_history(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_deal_deals_tenant_deleted_at ON deal.deals(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_financial_charges_tenant_deleted_at ON financial.charges(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_financial_collections_tenant_deleted_at ON financial.collections(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_financial_transfers_tenant_deleted_at ON financial.transfers(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_financial_invoices_tenant_deleted_at ON financial.invoices(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_financial_transactions_tenant_deleted_at ON financial.transactions(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_financial_commissions_tenant_deleted_at ON financial.commissions(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_ops_keys_tenant_deleted_at ON ops.keys(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_ops_inspections_tenant_deleted_at ON ops.inspections(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_ops_maintenance_tenant_deleted_at ON ops.maintenance(tenant_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_ops_tickets_tenant_deleted_at ON ops.tickets(tenant_id, deleted_at);
