-- SaaS subscriptions only. Real-estate charges retain their settlement flow.
-- Fail on ambiguous legacy duplicates; never silently delete billing evidence.
CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_subscription_one_per_tenant
 ON financial.platform_subscriptions(tenant_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_subscription_provider
 ON financial.platform_subscriptions((metadata->>'subscription_id'))
 WHERE deleted_at IS NULL AND metadata->>'subscription_id' IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_platform_subscription_external
 ON financial.platform_subscriptions((metadata->'checkout'->>'external_id'))
 WHERE deleted_at IS NULL AND metadata->'checkout'->>'external_id' IS NOT NULL;

-- Payment/entitlement state must not be editable through the Supabase Data API.
DROP POLICY IF EXISTS tenant_isolation_modify ON financial.platform_subscriptions;
DROP POLICY IF EXISTS tenant_isolation_select ON financial.platform_subscriptions;
CREATE POLICY tenant_isolation_select ON financial.platform_subscriptions
 FOR SELECT TO authenticated USING (tenant_id=(SELECT auth.jwt())->'app_metadata'->>'tenant_id');
REVOKE ALL ON financial.platform_subscriptions FROM anon,authenticated;
GRANT SELECT ON financial.platform_subscriptions TO authenticated;
GRANT ALL ON financial.platform_subscriptions TO service_role;

CREATE TABLE financial.platform_subscription_events (
 event_id text PRIMARY KEY,
 tenant_id uuid NOT NULL,
 event_type text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_platform_subscription_events_tenant ON financial.platform_subscription_events(tenant_id);
ALTER TABLE financial.platform_subscription_events ENABLE ROW LEVEL SECURITY;
REVOKE ALL ON financial.platform_subscription_events FROM anon,authenticated;
GRANT ALL ON financial.platform_subscription_events TO service_role;
-- Service-only idempotency ledger. No browser policies by design.
CREATE POLICY service_writer ON financial.platform_subscription_events FOR ALL TO service_role USING(true) WITH CHECK(true);

CREATE TABLE financial.platform_subscription_payments (
 payment_id text PRIMARY KEY,
 tenant_id uuid NOT NULL,
 subscription_id text NOT NULL,
 amount_cents bigint NOT NULL CHECK(amount_cents>0),
 paid_at timestamptz NOT NULL,
 receipt_url text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_platform_subscription_payments_tenant ON financial.platform_subscription_payments(tenant_id,paid_at DESC);
ALTER TABLE financial.platform_subscription_payments ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_select ON financial.platform_subscription_payments FOR SELECT TO authenticated
 USING (tenant_id=NULLIF((SELECT auth.jwt())->'app_metadata'->>'tenant_id','')::uuid);
REVOKE ALL ON financial.platform_subscription_payments FROM anon,authenticated;
GRANT SELECT ON financial.platform_subscription_payments TO authenticated;
GRANT ALL ON financial.platform_subscription_payments TO service_role;
