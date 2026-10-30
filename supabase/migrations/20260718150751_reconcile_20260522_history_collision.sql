-- Immutable reconciliation checkpoint for the 20260522120000 collision.
--
-- Production recorded both 20260522120000 and 20260522120001 as the
-- partner_royalties migration. Git records 20260522120000 as the former
-- Abacate webhook table and 20260522120001 as partner_royalties. Both
-- histories converge after 20260528130000 removes the deprecated Abacate
-- table. Do not repair, delete, rename, or rewrite either legacy version.
-- This forward-only checkpoint proves that the converged schema is intact.

DO $migration$
BEGIN
    IF to_regclass('financial.partner_royalties') IS NULL THEN
        RAISE EXCEPTION
            'migration lineage invariant failed: financial.partner_royalties is missing';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM (
            VALUES
                ('id',             'uuid',                     'NO'),
                ('tenant_id',      'text',                     'NO'),
                ('partnership_id', 'text',                     'YES'),
                ('deal_id',        'text',                     'YES'),
                ('contract_id',    'text',                     'YES'),
                ('payer_tenant',   'text',                     'YES'),
                ('payee_tenant',   'text',                     'YES'),
                ('base_amount',    'numeric',                  'NO'),
                ('percentage',     'numeric',                  'NO'),
                ('amount',         'numeric',                  'NO'),
                ('status',         'text',                     'NO'),
                ('metadata',       'jsonb',                    'NO'),
                ('created_at',     'timestamp with time zone', 'NO'),
                ('updated_at',     'timestamp with time zone', 'YES'),
                ('deleted_at',     'timestamp with time zone', 'YES')
        ) AS expected(column_name, data_type, is_nullable)
        LEFT JOIN information_schema.columns AS actual
          ON actual.table_schema = 'financial'
         AND actual.table_name = 'partner_royalties'
         AND actual.column_name = expected.column_name
        WHERE actual.column_name IS NULL
           OR actual.data_type <> expected.data_type
           OR actual.is_nullable <> expected.is_nullable
    ) THEN
        RAISE EXCEPTION
            'migration lineage invariant failed: financial.partner_royalties has an unexpected shape';
    END IF;

    IF to_regclass('financial.idx_finance_partner_royalties_tenant') IS NULL
       OR to_regclass('financial.idx_finance_partner_royalties_partnership') IS NULL
       OR to_regclass('financial.uq_finance_partner_royalties_source_event_id') IS NULL
    THEN
        RAISE EXCEPTION
            'migration lineage invariant failed: partner royalty indexes are missing';
    END IF;

    IF to_regclass('financial.abacate_webhook_events') IS NOT NULL THEN
        RAISE EXCEPTION
            'migration lineage invariant failed: deprecated Abacate webhook table still exists';
    END IF;
END
$migration$;
