-- Complete the defense-in-depth retrofit for tenant tables outside the 43
-- UUID business tables: legacy text tenant IDs, audit partitions, exports,
-- and service-only event plumbing.
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
            ('financial', 'partner_royalties', 'text', TRUE),
            ('financial', 'payout_orders', 'text', TRUE),
            ('financial', 'platform_subscriptions', 'text', TRUE),
            ('financial', 'reminder_rules', 'text', TRUE),
            ('financial', 'rentals', 'text', TRUE),
            ('financial', 'sale_billing_plan', 'text', TRUE),
            ('financial', 'sale_billing_plan_drafts', 'text', TRUE),
            ('financial', 'tenant_commission_rules', 'text', TRUE),
            ('financial', 'tenant_financial_settings', 'text', TRUE),
            ('financial', 'utility_accounts', 'text', TRUE),
            ('financial', 'withdrawals', 'text', TRUE),
            ('exports', 'jobs', 'uuid', TRUE),
            ('events', 'outbox_events', 'uuid', FALSE),
            ('events', 'dlq_events', 'uuid', FALSE)
        ) AS targets(schema_name, table_name, expected_type, expected_not_null)
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

        IF actual_type IS DISTINCT FROM target.expected_type::regtype::oid
           OR tenant_not_null IS DISTINCT FROM target.expected_not_null
        THEN
            RAISE EXCEPTION
                'RLS preflight failed: unexpected tenant_id shape on %',
                target_relation;
        END IF;
    END LOOP;
END
$preflight$;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.partner_royalties;
CREATE POLICY tenant_isolation_select ON financial.partner_royalties
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.partner_royalties;
CREATE POLICY tenant_isolation_modify ON financial.partner_royalties
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.partner_royalties ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.payout_orders;
CREATE POLICY tenant_isolation_select ON financial.payout_orders
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.payout_orders;
CREATE POLICY tenant_isolation_modify ON financial.payout_orders
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.payout_orders ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.platform_subscriptions;
CREATE POLICY tenant_isolation_select ON financial.platform_subscriptions
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.platform_subscriptions;
CREATE POLICY tenant_isolation_modify ON financial.platform_subscriptions
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.platform_subscriptions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.reminder_rules;
CREATE POLICY tenant_isolation_select ON financial.reminder_rules
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.reminder_rules;
CREATE POLICY tenant_isolation_modify ON financial.reminder_rules
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.reminder_rules ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.rentals;
CREATE POLICY tenant_isolation_select ON financial.rentals
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.rentals;
CREATE POLICY tenant_isolation_modify ON financial.rentals
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.rentals ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.sale_billing_plan;
CREATE POLICY tenant_isolation_select ON financial.sale_billing_plan
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.sale_billing_plan;
CREATE POLICY tenant_isolation_modify ON financial.sale_billing_plan
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.sale_billing_plan ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.sale_billing_plan_drafts;
CREATE POLICY tenant_isolation_select ON financial.sale_billing_plan_drafts
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.sale_billing_plan_drafts;
CREATE POLICY tenant_isolation_modify ON financial.sale_billing_plan_drafts
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.sale_billing_plan_drafts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.tenant_commission_rules;
CREATE POLICY tenant_isolation_select ON financial.tenant_commission_rules
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.tenant_commission_rules;
CREATE POLICY tenant_isolation_modify ON financial.tenant_commission_rules
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.tenant_commission_rules ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.tenant_financial_settings;
CREATE POLICY tenant_isolation_select ON financial.tenant_financial_settings
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.tenant_financial_settings;
CREATE POLICY tenant_isolation_modify ON financial.tenant_financial_settings
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.tenant_financial_settings ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.utility_accounts;
CREATE POLICY tenant_isolation_select ON financial.utility_accounts
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.utility_accounts;
CREATE POLICY tenant_isolation_modify ON financial.utility_accounts
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.utility_accounts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.withdrawals;
CREATE POLICY tenant_isolation_select ON financial.withdrawals
    FOR SELECT TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.withdrawals;
CREATE POLICY tenant_isolation_modify ON financial.withdrawals
    FOR ALL TO authenticated
    USING (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    )
    WITH CHECK (
        tenant_id = (auth.jwt() -> 'app_metadata' ->> 'tenant_id')
    );

ALTER TABLE financial.withdrawals ENABLE ROW LEVEL SECURITY;

-- Partition names are time-dependent: the original audit migration creates
-- the current and next month at replay time. Discover children from the
-- catalog instead of hard-coding the production month.
DO $audit_partitions$
DECLARE
    target RECORD;
BEGIN
    IF to_regclass('audit.events') IS NULL THEN
        RAISE EXCEPTION 'RLS preflight failed: audit.events is missing';
    END IF;

    FOR target IN
        SELECT child_namespace.nspname AS schema_name,
               child.relname AS table_name
        FROM pg_inherits AS inheritance
        JOIN pg_class AS parent
          ON parent.oid = inheritance.inhparent
        JOIN pg_namespace AS parent_namespace
          ON parent_namespace.oid = parent.relnamespace
        JOIN pg_class AS child
          ON child.oid = inheritance.inhrelid
        JOIN pg_namespace AS child_namespace
          ON child_namespace.oid = child.relnamespace
        WHERE parent_namespace.nspname = 'audit'
          AND parent.relname = 'events'
    LOOP
        EXECUTE format(
            'DROP POLICY IF EXISTS tenant_isolation_select ON %I.%I',
            target.schema_name,
            target.table_name
        );
        EXECUTE format(
            $policy$
                CREATE POLICY tenant_isolation_select ON %I.%I
                    FOR SELECT TO authenticated
                    USING (
                        tenant_id = NULLIF(
                            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
                            ''
                        )::uuid
                    )
            $policy$,
            target.schema_name,
            target.table_name
        );
        EXECUTE format(
            'DROP POLICY IF EXISTS tenant_isolation_modify ON %I.%I',
            target.schema_name,
            target.table_name
        );
        EXECUTE format(
            'ALTER TABLE %I.%I ENABLE ROW LEVEL SECURITY',
            target.schema_name,
            target.table_name
        );
    END LOOP;
END
$audit_partitions$;

DROP POLICY IF EXISTS tenant_isolation_select ON exports.jobs;
CREATE POLICY tenant_isolation_select ON exports.jobs
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON exports.jobs;
CREATE POLICY tenant_isolation_modify ON exports.jobs
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

ALTER TABLE exports.jobs ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON events.outbox_events;
DROP POLICY IF EXISTS tenant_isolation_modify ON events.outbox_events;
ALTER TABLE events.outbox_events ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON events.dlq_events;
DROP POLICY IF EXISTS tenant_isolation_modify ON events.dlq_events;
ALTER TABLE events.dlq_events ENABLE ROW LEVEL SECURITY;

-- portal.feed_cache is present in fresh installs but absent from the current
-- production lineage. Keep fresh environments protected without requiring the
-- optional table to exist in production.
ALTER TABLE IF EXISTS portal.feed_cache ENABLE ROW LEVEL SECURITY;

DO $optional_table$
BEGIN
    IF to_regclass('portal.feed_cache') IS NOT NULL THEN
        EXECUTE 'DROP POLICY IF EXISTS tenant_isolation_select ON portal.feed_cache';
        EXECUTE $policy$
            CREATE POLICY tenant_isolation_select ON portal.feed_cache
                FOR SELECT TO authenticated
                USING (
                    tenant_id = NULLIF(
                        auth.jwt() -> 'app_metadata' ->> 'tenant_id',
                        ''
                    )::uuid
                )
        $policy$;

        EXECUTE 'DROP POLICY IF EXISTS tenant_isolation_modify ON portal.feed_cache';
        EXECUTE $policy$
            CREATE POLICY tenant_isolation_modify ON portal.feed_cache
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
                )
        $policy$;
    END IF;
END
$optional_table$;
