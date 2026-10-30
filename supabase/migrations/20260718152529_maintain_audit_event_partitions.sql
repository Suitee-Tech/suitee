-- Keep the append-only audit trail writable across month boundaries.
--
-- The original bootstrap created only the migration month and the following
-- month. Production consequently stopped persisting audit rows on 2026-07-01.
-- This migration repairs the current horizon, applies least-privilege RLS to
-- every child partition, and schedules a small daily guard two months ahead.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE EXTENSION IF NOT EXISTS pg_cron WITH SCHEMA pg_catalog;

GRANT USAGE ON SCHEMA cron TO postgres;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA cron TO postgres;

CREATE OR REPLACE FUNCTION audit.ensure_event_partitions(months_ahead INTEGER DEFAULT 2)
RETURNS VOID
LANGUAGE plpgsql
SET search_path = ''
SET lock_timeout = '5s'
SET statement_timeout = '30s'
AS $function$
DECLARE
    base_month_start TIMESTAMPTZ;
    partition_start TIMESTAMPTZ;
    partition_end TIMESTAMPTZ;
    partition_name TEXT;
    partition_relation REGCLASS;
    partition_rls_enabled BOOLEAN;
    month_offset INTEGER;
    target RECORD;
BEGIN
    IF months_ahead < 1 OR months_ahead > 12 THEN
        RAISE EXCEPTION 'months_ahead must be between 1 and 12';
    END IF;
    IF pg_catalog.to_regclass('audit.events') IS NULL THEN
        RAISE EXCEPTION 'audit.events is missing';
    END IF;
    IF pg_catalog.to_regprocedure('auth.jwt()') IS NULL THEN
        RAISE EXCEPTION 'auth.jwt() is unavailable';
    END IF;
    IF pg_catalog.to_regrole('authenticated') IS NULL THEN
        RAISE EXCEPTION 'authenticated role is unavailable';
    END IF;

    base_month_start :=
        pg_catalog.date_trunc(
            'month',
            pg_catalog.clock_timestamp() AT TIME ZONE 'UTC'
        ) AT TIME ZONE 'UTC';

    FOR month_offset IN 0..months_ahead LOOP
        partition_start := base_month_start
            + pg_catalog.make_interval(months => month_offset);
        partition_end := base_month_start
            + pg_catalog.make_interval(months => month_offset + 1);
        partition_name := 'events_'
            || pg_catalog.to_char(
                partition_start AT TIME ZONE 'UTC',
                'YYYY_MM'
            );

        EXECUTE pg_catalog.format(
            'CREATE TABLE IF NOT EXISTS audit.%I '
            'PARTITION OF audit.events FOR VALUES FROM (%L) TO (%L)',
            partition_name,
            partition_start,
            partition_end
        );

        SELECT child.oid::REGCLASS,
               child.relrowsecurity
        INTO partition_relation,
             partition_rls_enabled
        FROM pg_catalog.pg_inherits AS inheritance
        JOIN pg_catalog.pg_class AS parent
          ON parent.oid = inheritance.inhparent
        JOIN pg_catalog.pg_namespace AS parent_namespace
          ON parent_namespace.oid = parent.relnamespace
        JOIN pg_catalog.pg_class AS child
          ON child.oid = inheritance.inhrelid
        JOIN pg_catalog.pg_namespace AS child_namespace
          ON child_namespace.oid = child.relnamespace
        WHERE parent_namespace.nspname = 'audit'
          AND parent.relname = 'events'
          AND child_namespace.nspname = 'audit'
          AND child.relname = partition_name;

        IF partition_relation IS NULL THEN
            RAISE EXCEPTION
                'audit partition % was not attached to audit.events',
                partition_name;
        END IF;

        IF NOT partition_rls_enabled THEN
            EXECUTE pg_catalog.format(
                'ALTER TABLE %s ENABLE ROW LEVEL SECURITY',
                partition_relation
            );
        END IF;
    END LOOP;

    -- Direct partition access does not inherit the parent policy. Protect all
    -- children, including retained historical partitions, without rewriting
    -- an already-correct policy on every daily cron run.
    FOR target IN
        SELECT child.oid AS relation_oid,
               child.oid::REGCLASS AS relation_name
        FROM pg_catalog.pg_inherits AS inheritance
        JOIN pg_catalog.pg_class AS parent
          ON parent.oid = inheritance.inhparent
        JOIN pg_catalog.pg_namespace AS parent_namespace
          ON parent_namespace.oid = parent.relnamespace
        JOIN pg_catalog.pg_class AS child
          ON child.oid = inheritance.inhrelid
        WHERE parent_namespace.nspname = 'audit'
          AND parent.relname = 'events'
    LOOP
        IF EXISTS (
            SELECT 1
            FROM pg_catalog.pg_policy AS policy
            WHERE policy.polrelid = target.relation_oid
              AND policy.polname = 'tenant_isolation_modify'
        ) THEN
            EXECUTE pg_catalog.format(
                'DROP POLICY tenant_isolation_modify ON %s',
                target.relation_name
            );
        END IF;

        IF NOT EXISTS (
            SELECT 1
            FROM pg_catalog.pg_policy AS policy
            WHERE policy.polrelid = target.relation_oid
              AND policy.polname = 'tenant_isolation_select'
        ) THEN
            EXECUTE pg_catalog.format(
                $policy$
                    CREATE POLICY tenant_isolation_select ON %s
                        FOR SELECT TO authenticated
                        USING (
                            tenant_id = NULLIF(
                                (SELECT auth.jwt())
                                    -> 'app_metadata' ->> 'tenant_id',
                                ''
                            )::uuid
                        )
                $policy$,
                target.relation_name
            );
        END IF;
    END LOOP;
END
$function$;

REVOKE ALL ON FUNCTION audit.ensure_event_partitions(INTEGER)
    FROM PUBLIC, anon, authenticated, service_role;
GRANT EXECUTE ON FUNCTION audit.ensure_event_partitions(INTEGER) TO postgres;

-- Repair the current month immediately before relying on the scheduled guard.
SELECT audit.ensure_event_partitions(2);

-- Normalize the parent and every existing child to a cached JWT lookup. This
-- is a one-time rewrite; the maintenance function only creates missing policy.
DROP POLICY IF EXISTS tenant_isolation_select ON audit.events;
CREATE POLICY tenant_isolation_select ON audit.events
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            (SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );
DROP POLICY IF EXISTS tenant_isolation_modify ON audit.events;
ALTER TABLE audit.events ENABLE ROW LEVEL SECURITY;

DO $normalize_partitions$
DECLARE
    target RECORD;
BEGIN
    FOR target IN
        SELECT child.oid::REGCLASS AS relation_name
        FROM pg_catalog.pg_inherits AS inheritance
        JOIN pg_catalog.pg_class AS parent
          ON parent.oid = inheritance.inhparent
        JOIN pg_catalog.pg_namespace AS parent_namespace
          ON parent_namespace.oid = parent.relnamespace
        JOIN pg_catalog.pg_class AS child
          ON child.oid = inheritance.inhrelid
        WHERE parent_namespace.nspname = 'audit'
          AND parent.relname = 'events'
    LOOP
        EXECUTE pg_catalog.format(
            'DROP POLICY IF EXISTS tenant_isolation_select ON %s',
            target.relation_name
        );
        EXECUTE pg_catalog.format(
            $policy$
                CREATE POLICY tenant_isolation_select ON %s
                    FOR SELECT TO authenticated
                    USING (
                        tenant_id = NULLIF(
                            (SELECT auth.jwt())
                                -> 'app_metadata' ->> 'tenant_id',
                            ''
                        )::uuid
                    )
            $policy$,
            target.relation_name
        );
    END LOOP;
END
$normalize_partitions$;

SELECT cron.schedule(
    'audit-ensure-event-partitions',
    '15 2 * * *',
    'SELECT audit.ensure_event_partitions(2);'
);
