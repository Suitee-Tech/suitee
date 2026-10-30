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

-- Batch 2/3: small UUID tenant tables (production-ledger aligned).
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
            ('financial', 'transfers'),
            ('ops', 'tickets'),
            ('financial', 'charges'),
            ('financial', 'transactions'),
            ('ops', 'maintenance'),
            ('core', 'appearance_settings'),
            ('crm', 'roulette_configs'),
            ('core', 'company_settings'),
            ('deal', 'templates'),
            ('calendar', 'appointments'),
            ('crm', 'brokers'),
            ('core', 'properties'),
            ('deal', 'documents'),
            ('financial', 'commissions'),
            ('ai', 'conversations'),
            ('deal', 'contracts'),
            ('deal', 'deals'),
            ('comm', 'channels'),
            ('ai', 'tool_invocations'),
            ('comm', 'messages'),
            ('crm', 'leads'),
            ('ai', 'messages'),
            ('crm', 'roulette_history')
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

CREATE INDEX IF NOT EXISTS idx_ai_messages_tenant_id
    ON ai.messages (tenant_id);

DROP POLICY IF EXISTS tenant_isolation_select ON financial.transfers;
CREATE POLICY tenant_isolation_select ON financial.transfers
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.transfers;
CREATE POLICY tenant_isolation_modify ON financial.transfers
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

ALTER TABLE financial.transfers ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ops.tickets;
CREATE POLICY tenant_isolation_select ON ops.tickets
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ops.tickets;
CREATE POLICY tenant_isolation_modify ON ops.tickets
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

ALTER TABLE ops.tickets ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.charges;
CREATE POLICY tenant_isolation_select ON financial.charges
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.charges;
CREATE POLICY tenant_isolation_modify ON financial.charges
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

ALTER TABLE financial.charges ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.transactions;
CREATE POLICY tenant_isolation_select ON financial.transactions
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.transactions;
CREATE POLICY tenant_isolation_modify ON financial.transactions
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

ALTER TABLE financial.transactions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ops.maintenance;
CREATE POLICY tenant_isolation_select ON ops.maintenance
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ops.maintenance;
CREATE POLICY tenant_isolation_modify ON ops.maintenance
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

ALTER TABLE ops.maintenance ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.appearance_settings;
CREATE POLICY tenant_isolation_select ON core.appearance_settings
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.appearance_settings;
CREATE POLICY tenant_isolation_modify ON core.appearance_settings
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

ALTER TABLE core.appearance_settings ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON crm.roulette_configs;
CREATE POLICY tenant_isolation_select ON crm.roulette_configs
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON crm.roulette_configs;
CREATE POLICY tenant_isolation_modify ON crm.roulette_configs
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

ALTER TABLE crm.roulette_configs ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.company_settings;
CREATE POLICY tenant_isolation_select ON core.company_settings
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.company_settings;
CREATE POLICY tenant_isolation_modify ON core.company_settings
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

ALTER TABLE core.company_settings ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON deal.templates;
CREATE POLICY tenant_isolation_select ON deal.templates
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON deal.templates;
CREATE POLICY tenant_isolation_modify ON deal.templates
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

ALTER TABLE deal.templates ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON calendar.appointments;
CREATE POLICY tenant_isolation_select ON calendar.appointments
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON calendar.appointments;
CREATE POLICY tenant_isolation_modify ON calendar.appointments
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

ALTER TABLE calendar.appointments ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON crm.brokers;
CREATE POLICY tenant_isolation_select ON crm.brokers
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON crm.brokers;
CREATE POLICY tenant_isolation_modify ON crm.brokers
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

ALTER TABLE crm.brokers ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON core.properties;
CREATE POLICY tenant_isolation_select ON core.properties
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON core.properties;
CREATE POLICY tenant_isolation_modify ON core.properties
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

ALTER TABLE core.properties ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON deal.documents;
CREATE POLICY tenant_isolation_select ON deal.documents
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON deal.documents;
CREATE POLICY tenant_isolation_modify ON deal.documents
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

ALTER TABLE deal.documents ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON financial.commissions;
CREATE POLICY tenant_isolation_select ON financial.commissions
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON financial.commissions;
CREATE POLICY tenant_isolation_modify ON financial.commissions
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

ALTER TABLE financial.commissions ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ai.conversations;
CREATE POLICY tenant_isolation_select ON ai.conversations
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ai.conversations;
CREATE POLICY tenant_isolation_modify ON ai.conversations
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

ALTER TABLE ai.conversations ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON deal.contracts;
CREATE POLICY tenant_isolation_select ON deal.contracts
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON deal.contracts;
CREATE POLICY tenant_isolation_modify ON deal.contracts
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

ALTER TABLE deal.contracts ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON deal.deals;
CREATE POLICY tenant_isolation_select ON deal.deals
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON deal.deals;
CREATE POLICY tenant_isolation_modify ON deal.deals
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

ALTER TABLE deal.deals ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON comm.channels;
CREATE POLICY tenant_isolation_select ON comm.channels
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON comm.channels;
CREATE POLICY tenant_isolation_modify ON comm.channels
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

ALTER TABLE comm.channels ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ai.tool_invocations;
CREATE POLICY tenant_isolation_select ON ai.tool_invocations
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ai.tool_invocations;
CREATE POLICY tenant_isolation_modify ON ai.tool_invocations
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

ALTER TABLE ai.tool_invocations ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON comm.messages;
CREATE POLICY tenant_isolation_select ON comm.messages
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON comm.messages;
CREATE POLICY tenant_isolation_modify ON comm.messages
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

ALTER TABLE comm.messages ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON crm.leads;
CREATE POLICY tenant_isolation_select ON crm.leads
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON crm.leads;
CREATE POLICY tenant_isolation_modify ON crm.leads
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

ALTER TABLE crm.leads ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON ai.messages;
CREATE POLICY tenant_isolation_select ON ai.messages
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON ai.messages;
CREATE POLICY tenant_isolation_modify ON ai.messages
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

ALTER TABLE ai.messages ENABLE ROW LEVEL SECURITY;

DROP POLICY IF EXISTS tenant_isolation_select ON crm.roulette_history;
CREATE POLICY tenant_isolation_select ON crm.roulette_history
    FOR SELECT TO authenticated
    USING (
        tenant_id = NULLIF(
            auth.jwt() -> 'app_metadata' ->> 'tenant_id',
            ''
        )::uuid
    );

DROP POLICY IF EXISTS tenant_isolation_modify ON crm.roulette_history;
CREATE POLICY tenant_isolation_modify ON crm.roulette_history
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

ALTER TABLE crm.roulette_history ENABLE ROW LEVEL SECURITY;
