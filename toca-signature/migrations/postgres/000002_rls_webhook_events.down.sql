DROP POLICY IF EXISTS tenant_isolation_modify ON signature.webhook_events;
DROP POLICY IF EXISTS tenant_isolation_select ON signature.webhook_events;
ALTER TABLE signature.webhook_events DISABLE ROW LEVEL SECURITY;
DROP INDEX IF EXISTS signature.idx_signature_webhook_events_tenant;
ALTER TABLE signature.webhook_events DROP COLUMN IF EXISTS tenant_id;
