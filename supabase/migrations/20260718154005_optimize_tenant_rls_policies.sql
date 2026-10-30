-- Normalize generic tenant policies for performance and least privilege.
--
-- `(SELECT auth.jwt())` becomes an initPlan evaluated once per statement.
-- A single FOR ALL policy avoids the duplicate SELECT evaluation caused by
-- overlapping FOR SELECT + FOR ALL policies. The roulette audit trail is
-- intentionally append-only for authenticated callers.

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

DO $normalize_generic_policies$
DECLARE
    target RECORD;
    policy_name TEXT;
    claim_expression TEXT;
BEGIN
    IF to_regprocedure('auth.jwt()') IS NULL THEN
        RAISE EXCEPTION 'RLS preflight failed: auth.jwt() is unavailable';
    END IF;
    IF to_regrole('authenticated') IS NULL THEN
        RAISE EXCEPTION 'RLS preflight failed: authenticated role is unavailable';
    END IF;

    FOR target IN
        SELECT child.oid::REGCLASS AS relation_name,
               child.relrowsecurity,
               tenant_column.atttypid
        FROM pg_class AS child
        JOIN pg_namespace AS namespace
          ON namespace.oid = child.relnamespace
        JOIN pg_attribute AS tenant_column
          ON tenant_column.attrelid = child.oid
         AND tenant_column.attname = 'tenant_id'
         AND tenant_column.attnum > 0
         AND NOT tenant_column.attisdropped
        WHERE child.relkind IN ('r', 'p')
          AND namespace.nspname IN (
              'ai',
              'calendar',
              'comm',
              'core',
              'crm',
              'deal',
              'exports',
              'financial',
              'ops',
              'portal',
              'signature',
              'toca_auth'
          )
          AND NOT (
              namespace.nspname = 'crm'
              AND child.relname = 'roulette_audit'
          )
        ORDER BY namespace.nspname, child.relname
    LOOP
        IF NOT target.relrowsecurity THEN
            RAISE EXCEPTION
                'RLS preflight failed: % does not have RLS enabled',
                target.relation_name;
        END IF;

        CASE target.atttypid
            WHEN 'uuid'::REGTYPE::OID THEN
                claim_expression :=
                    $claim$NULLIF(
                        (SELECT auth.jwt())
                            -> 'app_metadata' ->> 'tenant_id',
                        ''
                    )::uuid$claim$;
            WHEN 'text'::REGTYPE::OID THEN
                claim_expression :=
                    $claim$(SELECT auth.jwt())
                        -> 'app_metadata' ->> 'tenant_id'$claim$;
            ELSE
                RAISE EXCEPTION
                    'RLS preflight failed: unsupported tenant_id type on %',
                    target.relation_name;
        END CASE;

        FOREACH policy_name IN ARRAY ARRAY[
            'tenant_isolation',
            'tenant_isolation_select',
            'tenant_isolation_modify',
            'tenant_isolation_insert',
            'tenant_isolation_update',
            'tenant_isolation_delete'
        ]
        LOOP
            EXECUTE format(
                'DROP POLICY IF EXISTS %I ON %s',
                policy_name,
                target.relation_name
            );
        END LOOP;

        EXECUTE format(
            'CREATE POLICY tenant_isolation ON %s '
            'FOR ALL TO authenticated '
            'USING (tenant_id = %s) '
            'WITH CHECK (tenant_id = %s)',
            target.relation_name,
            claim_expression,
            claim_expression
        );
    END LOOP;
END
$normalize_generic_policies$;

DO $normalize_roulette_audit$
DECLARE
    relation_name REGCLASS := to_regclass('crm.roulette_audit');
    policy_name TEXT;
BEGIN
    IF relation_name IS NULL THEN
        RAISE EXCEPTION 'RLS preflight failed: crm.roulette_audit is missing';
    END IF;

    FOREACH policy_name IN ARRAY ARRAY[
        'tenant_isolation',
        'tenant_isolation_select',
        'tenant_isolation_modify',
        'tenant_isolation_insert',
        'tenant_isolation_update',
        'tenant_isolation_delete'
    ]
    LOOP
        EXECUTE format(
            'DROP POLICY IF EXISTS %I ON %s',
            policy_name,
            relation_name
        );
    END LOOP;

    EXECUTE format(
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
        relation_name
    );

    EXECUTE format(
        $policy$
            CREATE POLICY tenant_isolation_insert ON %s
                FOR INSERT TO authenticated
                WITH CHECK (
                    tenant_id = NULLIF(
                        (SELECT auth.jwt())
                            -> 'app_metadata' ->> 'tenant_id',
                        ''
                    )::uuid
                )
        $policy$,
        relation_name
    );
END
$normalize_roulette_audit$;
