CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS toca_auth;
CREATE SCHEMA IF NOT EXISTS calendar;
CREATE SCHEMA IF NOT EXISTS comm;
CREATE SCHEMA IF NOT EXISTS core;
CREATE SCHEMA IF NOT EXISTS crm;
CREATE SCHEMA IF NOT EXISTS deal;
CREATE SCHEMA IF NOT EXISTS events;
CREATE SCHEMA IF NOT EXISTS financial;
CREATE SCHEMA IF NOT EXISTS ops;
CREATE SCHEMA IF NOT EXISTS portal;

CREATE TABLE IF NOT EXISTS comm.channels (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_channels_tenant_id ON comm.channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comm_channels_created_at ON comm.channels(created_at);
CREATE INDEX IF NOT EXISTS idx_comm_channels_deleted_at ON comm.channels(deleted_at);

CREATE TABLE IF NOT EXISTS comm.messages (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_messages_tenant_id ON comm.messages(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comm_messages_created_at ON comm.messages(created_at);
CREATE INDEX IF NOT EXISTS idx_comm_messages_deleted_at ON comm.messages(deleted_at);

CREATE TABLE IF NOT EXISTS comm.templates (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_templates_tenant_id ON comm.templates(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comm_templates_created_at ON comm.templates(created_at);
CREATE INDEX IF NOT EXISTS idx_comm_templates_deleted_at ON comm.templates(deleted_at);

CREATE TABLE IF NOT EXISTS comm.inapp_notifications (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_inapp_notifications_tenant_id ON comm.inapp_notifications(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comm_inapp_notifications_created_at ON comm.inapp_notifications(created_at);
CREATE INDEX IF NOT EXISTS idx_comm_inapp_notifications_deleted_at ON comm.inapp_notifications(deleted_at);

CREATE TABLE IF NOT EXISTS toca_auth.accounts (
  id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS toca_auth.credentials (
  id UUID PRIMARY KEY,
  email TEXT NOT NULL UNIQUE,
  account_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_credentials_account_id ON toca_auth.credentials(account_id);

CREATE TABLE IF NOT EXISTS toca_auth.invitations (
  id UUID PRIMARY KEY,
  company_id UUID NOT NULL,
  email TEXT NOT NULL,
  token TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_auth_invitations_company_id ON toca_auth.invitations(company_id);
CREATE INDEX IF NOT EXISTS idx_auth_invitations_email ON toca_auth.invitations(email);
CREATE INDEX IF NOT EXISTS idx_auth_invitations_deleted_at ON toca_auth.invitations(deleted_at);

CREATE TABLE IF NOT EXISTS toca_auth.groups (
  id UUID PRIMARY KEY,
  company_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_groups_company_id ON toca_auth.groups(company_id);

CREATE TABLE IF NOT EXISTS toca_auth.password_resets (
  id UUID PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  metadata JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS toca_auth.user_preferences (
  account_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL
);

CREATE TABLE IF NOT EXISTS crm.brokers (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE crm.brokers ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE crm.brokers ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;

CREATE INDEX IF NOT EXISTS idx_crm_brokers_tenant_id ON crm.brokers(tenant_id);
CREATE INDEX IF NOT EXISTS idx_crm_brokers_deleted_at ON crm.brokers(deleted_at);

CREATE TABLE IF NOT EXISTS crm.leads (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crm_leads_tenant_id ON crm.leads(tenant_id);
CREATE INDEX IF NOT EXISTS idx_crm_leads_created_at ON crm.leads(created_at);

CREATE TABLE IF NOT EXISTS crm.roulette_configs (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS crm.roulette_history (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_crm_roulette_history_tenant_id ON crm.roulette_history(tenant_id);
CREATE INDEX IF NOT EXISTS idx_crm_roulette_history_created_at ON crm.roulette_history(created_at);

CREATE TABLE IF NOT EXISTS portal.integrations (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_portal_integrations_tenant_id ON portal.integrations(tenant_id);

CREATE TABLE IF NOT EXISTS portal.sync_history (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_portal_sync_history_tenant_id ON portal.sync_history(tenant_id);

CREATE TABLE IF NOT EXISTS portal.leads (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_portal_leads_tenant_id ON portal.leads(tenant_id);

CREATE TABLE IF NOT EXISTS core.company_settings (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.appearance_settings (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.role_permissions (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.notification_settings (
  tenant_id UUID PRIMARY KEY,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS core.webhooks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_core_webhooks_tenant_id ON core.webhooks(tenant_id);

CREATE TABLE IF NOT EXISTS core.api_keys (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_core_api_keys_tenant_id ON core.api_keys(tenant_id);

CREATE TABLE IF NOT EXISTS core.partnerships (
  id UUID PRIMARY KEY,
  requester_id UUID NOT NULL,
  target_id UUID NOT NULL,
  status TEXT NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_core_partnerships_requester_id ON core.partnerships(requester_id);
CREATE INDEX IF NOT EXISTS idx_core_partnerships_target_id ON core.partnerships(target_id);
CREATE INDEX IF NOT EXISTS idx_core_partnerships_deleted_at ON core.partnerships(deleted_at);

CREATE TABLE IF NOT EXISTS core.properties (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  title TEXT NOT NULL,
  price NUMERIC NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  type TEXT NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_core_properties_tenant_id ON core.properties(tenant_id);
CREATE INDEX IF NOT EXISTS idx_core_properties_deleted_at ON core.properties(deleted_at);

CREATE TABLE IF NOT EXISTS deal.deals (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deal_deals_tenant_id ON deal.deals(tenant_id);

CREATE TABLE IF NOT EXISTS deal.documents (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deal_documents_tenant_id ON deal.documents(tenant_id);

CREATE TABLE IF NOT EXISTS deal.templates (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deal_templates_tenant_id ON deal.templates(tenant_id);

CREATE TABLE IF NOT EXISTS deal.contracts (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  deal_id UUID NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_deal_contracts_tenant_id ON deal.contracts(tenant_id);
CREATE INDEX IF NOT EXISTS idx_deal_contracts_deal_id ON deal.contracts(deal_id);

CREATE TABLE IF NOT EXISTS financial.payments (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_payments_tenant_id ON financial.payments(tenant_id);

CREATE TABLE IF NOT EXISTS financial.nfse (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_nfse_tenant_id ON financial.nfse(tenant_id);

CREATE TABLE IF NOT EXISTS financial.dimob (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_dimob_tenant_id ON financial.dimob(tenant_id);

CREATE TABLE IF NOT EXISTS financial.informes (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_informes_tenant_id ON financial.informes(tenant_id);

CREATE TABLE IF NOT EXISTS financial.commissions (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_commissions_tenant_id ON financial.commissions(tenant_id);

CREATE TABLE IF NOT EXISTS financial.rental_charge_configs (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_rental_charge_configs_tenant_id ON financial.rental_charge_configs(tenant_id);

CREATE TABLE IF NOT EXISTS financial.transfers (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_transfers_tenant_id ON financial.transfers(tenant_id);

CREATE TABLE IF NOT EXISTS financial.transactions (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_transactions_tenant_id ON financial.transactions(tenant_id);

CREATE TABLE IF NOT EXISTS financial.invoices (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_financial_invoices_tenant_id ON financial.invoices(tenant_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_invoices_source_event_id
  ON financial.invoices (tenant_id, ((metadata->>'source_event_id')))
  WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL;

CREATE TABLE IF NOT EXISTS financial.collections (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_collections_tenant_id ON financial.collections(tenant_id);

CREATE TABLE IF NOT EXISTS financial.charges (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_financial_charges_tenant_id ON financial.charges(tenant_id);

CREATE TABLE IF NOT EXISTS ops.maintenance (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_maintenance_tenant_id ON ops.maintenance(tenant_id);

CREATE TABLE IF NOT EXISTS ops.keys (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_keys_tenant_id ON ops.keys(tenant_id);

CREATE TABLE IF NOT EXISTS ops.inspections (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_inspections_tenant_id ON ops.inspections(tenant_id);

CREATE TABLE IF NOT EXISTS ops.tickets (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ops_tickets_tenant_id ON ops.tickets(tenant_id);

CREATE TABLE IF NOT EXISTS calendar.appointments (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_calendar_appointments_tenant_id ON calendar.appointments(tenant_id);

CREATE TABLE IF NOT EXISTS comm.inapp_notifications (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_comm_inapp_notifications_tenant_id ON comm.inapp_notifications(tenant_id);

CREATE TABLE IF NOT EXISTS comm.messages (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_comm_messages_tenant_id ON comm.messages(tenant_id);

CREATE TABLE IF NOT EXISTS comm.channels (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_channels_tenant_id ON comm.channels(tenant_id);
CREATE INDEX IF NOT EXISTS idx_comm_channels_participants ON comm.channels USING GIN ((metadata -> 'participants'));

CREATE TABLE IF NOT EXISTS comm.templates (
  id UUID PRIMARY KEY,
  tenant_id UUID NOT NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_comm_templates_tenant_id ON comm.templates(tenant_id);

CREATE TABLE IF NOT EXISTS portal.feed_cache (
  id UUID PRIMARY KEY,
  tenant_id UUID NULL,
  metadata JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  deleted_at TIMESTAMPTZ NULL
);

CREATE INDEX IF NOT EXISTS idx_portal_feed_cache_tenant_id ON portal.feed_cache(tenant_id);

CREATE TABLE IF NOT EXISTS events.outbox_events (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id UUID NULL,
  event_name TEXT NOT NULL,
  payload JSONB NOT NULL,
  status TEXT NOT NULL DEFAULT 'PENDING',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  published_at TIMESTAMPTZ NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NULL
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_status_created_at ON events.outbox_events(status, created_at);

CREATE TABLE IF NOT EXISTS events.processed_events (
  message_id TEXT PRIMARY KEY,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS events.dlq_events (
  message_id TEXT NOT NULL,
  dlq_name TEXT NOT NULL,
  queue_name TEXT NOT NULL,
  event_name TEXT NOT NULL,
  tenant_id UUID NULL,
  payload BYTEA NOT NULL,
  reason TEXT NULL,
  death_count BIGINT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  replayed_at TIMESTAMPTZ NULL,
  PRIMARY KEY (message_id, dlq_name)
);

CREATE INDEX IF NOT EXISTS idx_events_dlq_events_created_at ON events.dlq_events(created_at);
CREATE INDEX IF NOT EXISTS idx_events_dlq_events_replayed_at ON events.dlq_events(replayed_at) WHERE replayed_at IS NULL;
