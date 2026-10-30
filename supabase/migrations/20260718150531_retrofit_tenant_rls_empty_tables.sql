-- Production RLS retrofit.
--
-- Historical migrations checked auth.jwt() with to_regproc('auth.jwt()').
-- That signature lookup returns NULL, so their policy blocks exited without
-- enabling RLS. This forward-only migration uses an explicit table list and
-- to_regprocedure('auth.jwt()'), avoiding both the original bug and accidental
-- coverage of tables created by later migrations.
--
-- Policies target authenticated explicitly and authorize only the immutable
-- app_metadata.tenant_id claim. API roles still have no grants on these custom
-- schemas; the policies are defense in depth while the current service pool
-- remains on a BYPASSRLS role.

-- Batch 1/3: empty UUID tenant tables (production-ledger aligned).
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

DO $preflight$
DECLARE
    target RECORD;
    target_relation REGCLASS;
    actual_type OID;
    tenant_not_null BOOLEAN;
BEGIN
    IF to_regprocedure('auth.jwt()') IS NULL THEN
        RAISE EXCEPTION 'RLS preflight failed: auth.jwt() is unavailable';
    END IF;
    IF to_regrole('authenticated') IS NULL THEN
        RAISE EXCEPTION 'RLS preflight failed: authenticated role is unavailable';
    END IF;

    FOR target IN
        SELECT *
        FROM (VALUES
            ('comm', 'templates'),
            ('core', 'api_keys'),
            ('core', 'notification_settings'),
            ('core', 'role_permissions'),
            ('core', 'webhooks'),
            ('crm', 'lead_phone_quarantine'),
            ('financial', 'collections'),
            ('financial', 'dimob'),
            ('financial', 'informes'),
            ('financial', 'invoices'),
            ('financial', 'nfse'),
            ('financial', 'payments'),
            ('financial', 'rental_charge_configs'),
            ('ops', 'inspections'),
            ('ops', 'keys'),
            ('portal', 'integrations'),
            ('portal', 'leads'),
            ('portal', 'sync_history')
        ) AS targets(schema_name, table_name)
    LOOP
        target_relation := to_regclass(
            format('%I.%I', target.schema_name, target.table_name)
        );
        IF target_relation IS NULL THEN
            RAISE EXCEPTION
                'RLS preflight failed: %.% is missing',
                target.schema_name,
                target.table_name;
        END IF;

        SELECT attribute.atttypid, attribute.attnotnull
        INTO actual_type, tenant_not_null
        FROM pg_attribute AS attribute
        WHERE attribute.attrelid = target_relation
          AND attribute.attname = 'tenant_id'
          AND attribute.attnum > 0
          AND NOT attribute.attisdropped;

        IF actual_type IS DISTINCT FROM 'uuid'::regtype::oid
           OR tenant_not_null IS DISTINCT FROM TRUE
        THEN
            RAISE EXCEPTION
                'RLS preflight failed: %.tenant_id must be UUID NOT NULL',
                target_relation;
        END IF;
    END LOOP;
END
$preflight$;

DROP POLICY IF EXISTS tenant_isolation_select ON comm.templates;
CREATE POLICY tenant_isolation_select ON comm.templates
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON comm.templates;
CREATE POLICY tenant_isolation_modify ON comm.templates
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE comm.templates ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.api_keys;
CREATE POLICY tenant_isolation_select ON core.api_keys
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.api_keys;
CREATE POLICY tenant_isolation_modify ON core.api_keys
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE core.api_keys ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.notification_settings;
CREATE POLICY tenant_isolation_select ON core.notification_settings
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.notification_settings;
CREATE POLICY tenant_isolation_modify ON core.notification_settings
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE core.notification_settings ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.role_permissions;
CREATE POLICY tenant_isolation_select ON core.role_permissions
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.role_permissions;
CREATE POLICY tenant_isolation_modify ON core.role_permissions
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE core.role_permissions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.webhooks;
CREATE POLICY tenant_isolation_select ON core.webhooks
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.webhooks;
CREATE POLICY tenant_isolation_modify ON core.webhooks
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE core.webhooks ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON crm.lead_phone_quarantine;
CREATE POLICY tenant_isolation_select ON crm.lead_phone_quarantine
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON crm.lead_phone_quarantine;
CREATE POLICY tenant_isolation_modify ON crm.lead_phone_quarantine
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE crm.lead_phone_quarantine ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.collections;
CREATE POLICY tenant_isolation_select ON financial.collections
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.collections;
CREATE POLICY tenant_isolation_modify ON financial.collections
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.collections ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.dimob;
CREATE POLICY tenant_isolation_select ON financial.dimob
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.dimob;
CREATE POLICY tenant_isolation_modify ON financial.dimob
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.dimob ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.informes;
CREATE POLICY tenant_isolation_select ON financial.informes
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.informes;
CREATE POLICY tenant_isolation_modify ON financial.informes
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.informes ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.invoices;
CREATE POLICY tenant_isolation_select ON financial.invoices
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.invoices;
CREATE POLICY tenant_isolation_modify ON financial.invoices
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.invoices ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.nfse;
CREATE POLICY tenant_isolation_select ON financial.nfse
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.nfse;
CREATE POLICY tenant_isolation_modify ON financial.nfse
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.nfse ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.payments;
CREATE POLICY tenant_isolation_select ON financial.payments
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.payments;
CREATE POLICY tenant_isolation_modify ON financial.payments
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.payments ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.rental_charge_configs;
CREATE POLICY tenant_isolation_select ON financial.rental_charge_configs
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.rental_charge_configs;
CREATE POLICY tenant_isolation_modify ON financial.rental_charge_configs
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE financial.rental_charge_configs ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ops.inspections;
CREATE POLICY tenant_isolation_select ON ops.inspections
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ops.inspections;
CREATE POLICY tenant_isolation_modify ON ops.inspections
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE ops.inspections ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ops.keys;
CREATE POLICY tenant_isolation_select ON ops.keys
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ops.keys;
CREATE POLICY tenant_isolation_modify ON ops.keys
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE ops.keys ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON portal.integrations;
CREATE POLICY tenant_isolation_select ON portal.integrations
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON portal.integrations;
CREATE POLICY tenant_isolation_modify ON portal.integrations
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE portal.integrations ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON portal.leads;
CREATE POLICY tenant_isolation_select ON portal.leads
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON portal.leads;
CREATE POLICY tenant_isolation_modify ON portal.leads
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE portal.leads ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON portal.sync_history;
CREATE POLICY tenant_isolation_select ON portal.sync_history
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON portal.sync_history;
CREATE POLICY tenant_isolation_modify ON portal.sync_history
    FOR ALL TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    )
    WITH CHECK (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

ALTER TABLE portal.sync_history ENABLE ROW LEVEL SECURITY;
