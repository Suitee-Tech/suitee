-- Mirror of supabase/migrations/20260514120000_rls_signature_webhook_events.sql.
-- See that file for context and rationale.

ALTER TABLE signature.webhook_events
    ADD COLUMN IF NOT EXISTS tenant_id UUID;

UPDATE signature.webhook_events we
   SET tenant_id = e.tenant_id
  FROM signature.envelopes e
 WHERE we.external_envelope_id = e.external_envelope_id
   AND we.tenant_id IS NULL
   AND e.tenant_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_signature_webhook_events_tenant
    ON signature.webhook_events (tenant_id)
    WHERE tenant_id IS NOT NULL;

ALTER TABLE signature.webhook_events ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON signature.webhook_events;
CREATE POLICY tenant_isolation_select ON signature.webhook_events
    FOR SELECT USING (
        tenant_id IS NOT NULL
        AND tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON signature.webhook_events;
CREATE POLICY tenant_isolation_modify ON signature.webhook_events
    FOR ALL USING (
        tenant_id IS NOT NULL
        AND tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    ) WITH CHECK (
        tenant_id IS NOT NULL
        AND tenant_id = ((auth.jwt() -> 'app_metadata' ->> 'tenant_id')::uuid)
    );
