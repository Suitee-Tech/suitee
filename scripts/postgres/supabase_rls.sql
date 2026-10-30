ALTER TABLE crm.brokers ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON crm.brokers;
CREATE POLICY tenant_isolation_select ON crm.brokers FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON crm.brokers;
CREATE POLICY tenant_isolation_modify ON crm.brokers FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE crm.leads ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON crm.leads;
CREATE POLICY tenant_isolation_select ON crm.leads FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON crm.leads;
CREATE POLICY tenant_isolation_modify ON crm.leads FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE crm.roulette_configs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON crm.roulette_configs;
CREATE POLICY tenant_isolation_select ON crm.roulette_configs FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON crm.roulette_configs;
CREATE POLICY tenant_isolation_modify ON crm.roulette_configs FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE crm.roulette_history ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON crm.roulette_history;
CREATE POLICY tenant_isolation_select ON crm.roulette_history FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON crm.roulette_history;
CREATE POLICY tenant_isolation_modify ON crm.roulette_history FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE portal.integrations ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON portal.integrations;
CREATE POLICY tenant_isolation_select ON portal.integrations FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON portal.integrations;
CREATE POLICY tenant_isolation_modify ON portal.integrations FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE portal.sync_history ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON portal.sync_history;
CREATE POLICY tenant_isolation_select ON portal.sync_history FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON portal.sync_history;
CREATE POLICY tenant_isolation_modify ON portal.sync_history FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE portal.leads ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON portal.leads;
CREATE POLICY tenant_isolation_select ON portal.leads FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON portal.leads;
CREATE POLICY tenant_isolation_modify ON portal.leads FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.company_settings ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.company_settings;
CREATE POLICY tenant_isolation_select ON core.company_settings FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.company_settings;
CREATE POLICY tenant_isolation_modify ON core.company_settings FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.appearance_settings ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.appearance_settings;
CREATE POLICY tenant_isolation_select ON core.appearance_settings FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.appearance_settings;
CREATE POLICY tenant_isolation_modify ON core.appearance_settings FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.role_permissions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.role_permissions;
CREATE POLICY tenant_isolation_select ON core.role_permissions FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.role_permissions;
CREATE POLICY tenant_isolation_modify ON core.role_permissions FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.notification_settings ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.notification_settings;
CREATE POLICY tenant_isolation_select ON core.notification_settings FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.notification_settings;
CREATE POLICY tenant_isolation_modify ON core.notification_settings FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.webhooks ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.webhooks;
CREATE POLICY tenant_isolation_select ON core.webhooks FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.webhooks;
CREATE POLICY tenant_isolation_modify ON core.webhooks FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.api_keys ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.api_keys;
CREATE POLICY tenant_isolation_select ON core.api_keys FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.api_keys;
CREATE POLICY tenant_isolation_modify ON core.api_keys FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE core.properties ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON core.properties;
CREATE POLICY tenant_isolation_select ON core.properties FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON core.properties;
CREATE POLICY tenant_isolation_modify ON core.properties FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE deal.deals ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON deal.deals;
CREATE POLICY tenant_isolation_select ON deal.deals FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON deal.deals;
CREATE POLICY tenant_isolation_modify ON deal.deals FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE deal.documents ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON deal.documents;
CREATE POLICY tenant_isolation_select ON deal.documents FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON deal.documents;
CREATE POLICY tenant_isolation_modify ON deal.documents FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE deal.templates ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON deal.templates;
CREATE POLICY tenant_isolation_select ON deal.templates FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON deal.templates;
CREATE POLICY tenant_isolation_modify ON deal.templates FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE deal.contracts ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON deal.contracts;
CREATE POLICY tenant_isolation_select ON deal.contracts FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON deal.contracts;
CREATE POLICY tenant_isolation_modify ON deal.contracts FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.payments ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.payments;
CREATE POLICY tenant_isolation_select ON financial.payments FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.payments;
CREATE POLICY tenant_isolation_modify ON financial.payments FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.nfse ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.nfse;
CREATE POLICY tenant_isolation_select ON financial.nfse FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.nfse;
CREATE POLICY tenant_isolation_modify ON financial.nfse FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.dimob ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.dimob;
CREATE POLICY tenant_isolation_select ON financial.dimob FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.dimob;
CREATE POLICY tenant_isolation_modify ON financial.dimob FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.informes ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.informes;
CREATE POLICY tenant_isolation_select ON financial.informes FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.informes;
CREATE POLICY tenant_isolation_modify ON financial.informes FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.commissions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.commissions;
CREATE POLICY tenant_isolation_select ON financial.commissions FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.commissions;
CREATE POLICY tenant_isolation_modify ON financial.commissions FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.rental_charge_configs ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.rental_charge_configs;
CREATE POLICY tenant_isolation_select ON financial.rental_charge_configs FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.rental_charge_configs;
CREATE POLICY tenant_isolation_modify ON financial.rental_charge_configs FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.transfers ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.transfers;
CREATE POLICY tenant_isolation_select ON financial.transfers FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.transfers;
CREATE POLICY tenant_isolation_modify ON financial.transfers FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.transactions ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.transactions;
CREATE POLICY tenant_isolation_select ON financial.transactions FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.transactions;
CREATE POLICY tenant_isolation_modify ON financial.transactions FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.invoices ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.invoices;
CREATE POLICY tenant_isolation_select ON financial.invoices FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.invoices;
CREATE POLICY tenant_isolation_modify ON financial.invoices FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.collections ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.collections;
CREATE POLICY tenant_isolation_select ON financial.collections FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.collections;
CREATE POLICY tenant_isolation_modify ON financial.collections FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE financial.charges ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.charges;
CREATE POLICY tenant_isolation_select ON financial.charges FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.charges;
CREATE POLICY tenant_isolation_modify ON financial.charges FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE ops.maintenance ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON ops.maintenance;
CREATE POLICY tenant_isolation_select ON ops.maintenance FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON ops.maintenance;
CREATE POLICY tenant_isolation_modify ON ops.maintenance FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE ops.keys ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON ops.keys;
CREATE POLICY tenant_isolation_select ON ops.keys FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON ops.keys;
CREATE POLICY tenant_isolation_modify ON ops.keys FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE ops.inspections ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON ops.inspections;
CREATE POLICY tenant_isolation_select ON ops.inspections FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON ops.inspections;
CREATE POLICY tenant_isolation_modify ON ops.inspections FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE ops.tickets ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON ops.tickets;
CREATE POLICY tenant_isolation_select ON ops.tickets FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON ops.tickets;
CREATE POLICY tenant_isolation_modify ON ops.tickets FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE calendar.appointments ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON calendar.appointments;
CREATE POLICY tenant_isolation_select ON calendar.appointments FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON calendar.appointments;
CREATE POLICY tenant_isolation_modify ON calendar.appointments FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE comm.inapp_notifications ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON comm.inapp_notifications;
CREATE POLICY tenant_isolation_select ON comm.inapp_notifications FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON comm.inapp_notifications;
CREATE POLICY tenant_isolation_modify ON comm.inapp_notifications FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));

ALTER TABLE comm.messages ENABLE ROW LEVEL SECURITY;
DROP POLICY IF EXISTS tenant_isolation_select ON comm.messages;
CREATE POLICY tenant_isolation_select ON comm.messages FOR SELECT USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
DROP POLICY IF EXISTS tenant_isolation_modify ON comm.messages;
CREATE POLICY tenant_isolation_modify ON comm.messages FOR ALL USING (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)) WITH CHECK (tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid));
