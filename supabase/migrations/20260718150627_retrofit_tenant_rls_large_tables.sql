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

-- Batch 3/3: high-traffic UUID tenant tables (production-ledger aligned).
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
            ('comm', 'inapp_notifications'),
            ('crm', 'roulette_audit')
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

DROP POLICY IF EXISTS tenant_isolation_select ON comm.inapp_notifications;
CREATE POLICY tenant_isolation_select ON comm.inapp_notifications
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON comm.inapp_notifications;
CREATE POLICY tenant_isolation_modify ON comm.inapp_notifications
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

ALTER TABLE comm.inapp_notifications ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON crm.roulette_audit;
CREATE POLICY tenant_isolation_select ON crm.roulette_audit
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON crm.roulette_audit;
CREATE POLICY tenant_isolation_modify ON crm.roulette_audit
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

ALTER TABLE crm.roulette_audit ENABLE ROW LEVEL SECURITY;
