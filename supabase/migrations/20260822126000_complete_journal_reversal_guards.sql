-- Complete the database boundary for append-only journal reversals on
-- databases that may already have applied 20260822123000/124000/125000.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_insert_strict()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_insert_strict_guard$
DECLARE
    debit_total NUMERIC;
    credit_total NUMERIC;
BEGIN
    IF jsonb_typeof(NEW.metadata -> 'legs') IS DISTINCT FROM 'array'
       OR jsonb_array_length(NEW.metadata -> 'legs') < 2 THEN
        RAISE EXCEPTION 'journal entry requires at least two legs'
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.metadata -> 'legs') AS item(leg)
        WHERE CASE
            WHEN jsonb_typeof(item.leg) IS DISTINCT FROM 'object' THEN TRUE
            WHEN item.leg ->> 'account' IS NULL THEN TRUE
            WHEN item.leg ->> 'account' NOT IN (
                'cash', 'accounts_receivable', 'commission_payable',
                'owner_payable', 'partner_payable', 'royalty_payable',
                'tax_withheld', 'withdrawal_payable', 'equity', 'revenue',
                'commission_expense', 'expense'
            ) THEN TRUE
            WHEN item.leg ->> 'direction' IS NULL THEN TRUE
            WHEN item.leg ->> 'direction' NOT IN ('debit', 'credit') THEN TRUE
            WHEN jsonb_typeof(item.leg -> 'amount_cents') IS DISTINCT FROM 'number' THEN TRUE
            WHEN item.leg ->> 'amount_cents' !~ '^[0-9]+$' THEN TRUE
            ELSE (item.leg ->> 'amount_cents')::NUMERIC <= 0
                OR (item.leg ->> 'amount_cents')::NUMERIC > 9223372036854775807
        END
    ) THEN
        RAISE EXCEPTION 'journal entry contains an invalid leg'
            USING ERRCODE = '23514';
    END IF;

    SELECT
        COALESCE(SUM((item.leg ->> 'amount_cents')::NUMERIC)
            FILTER (WHERE item.leg ->> 'direction' = 'debit'), 0),
        COALESCE(SUM((item.leg ->> 'amount_cents')::NUMERIC)
            FILTER (WHERE item.leg ->> 'direction' = 'credit'), 0)
    INTO debit_total, credit_total
    FROM jsonb_array_elements(NEW.metadata -> 'legs') AS item(leg);

    IF debit_total IS DISTINCT FROM credit_total
       OR debit_total <= 0 OR credit_total <= 0 THEN
        RAISE EXCEPTION 'journal entry must be balanced with positive totals'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$journal_entry_insert_strict_guard$;

DROP TRIGGER IF EXISTS trg_guard_journal_entry_insert_strict
    ON financial.journal_entries;
CREATE TRIGGER trg_guard_journal_entry_insert_strict
    BEFORE INSERT ON financial.journal_entries
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_journal_entry_insert_strict();

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_update_timestamp()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_update_timestamp_guard$
DECLARE
    metadata_updated_at TIMESTAMPTZ;
BEGIN
    IF NULLIF(NEW.metadata ->> 'updated_at', '') IS NULL THEN
        RAISE EXCEPTION 'journal metadata updated_at is required'
            USING ERRCODE = '23514';
    END IF;
    metadata_updated_at := (NEW.metadata ->> 'updated_at')::TIMESTAMPTZ;
    IF metadata_updated_at IS DISTINCT FROM NEW.updated_at THEN
        RAISE EXCEPTION 'journal metadata updated_at must match the row timestamp'
            USING ERRCODE = '23514';
    END IF;
    RETURN NEW;
END
$journal_entry_update_timestamp_guard$;

DROP TRIGGER IF EXISTS trg_guard_journal_entry_update_timestamp
    ON financial.journal_entries;
CREATE TRIGGER trg_guard_journal_entry_update_timestamp
    BEFORE UPDATE ON financial.journal_entries
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_journal_entry_update_timestamp();

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_reversal_of
    ON financial.journal_entries (tenant_id, ((metadata ->> 'reversal_of')))
    WHERE deleted_at IS NULL
      AND NULLIF(BTRIM(metadata ->> 'reversal_of'), '') IS NOT NULL;

CREATE OR REPLACE FUNCTION financial.require_single_journal_reversal_at_commit()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $single_journal_reversal_guard$
DECLARE
    reversal_count BIGINT;
BEGIN
    IF OLD.metadata ->> 'status' = 'POSTED'
       AND NEW.metadata ->> 'status' = 'REVERSED' THEN
        SELECT COUNT(*)
        INTO reversal_count
        FROM financial.journal_entries AS reversal
        WHERE reversal.tenant_id = NEW.tenant_id
          AND reversal.deleted_at IS NULL
          AND reversal.metadata ->> 'reversal_of' = NEW.id::TEXT;

        IF reversal_count IS DISTINCT FROM 1::BIGINT THEN
            RAISE EXCEPTION 'journal reversal must commit with exactly one compensating entry'
                USING ERRCODE = '23514';
        END IF;
    END IF;
    RETURN NULL;
END
$single_journal_reversal_guard$;

DO $journal_reversal_preflight$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM financial.journal_entries AS original
        LEFT JOIN financial.journal_entries AS reversal
          ON reversal.tenant_id = original.tenant_id
         AND reversal.deleted_at IS NULL
         AND reversal.metadata ->> 'reversal_of' = original.id::TEXT
        WHERE original.deleted_at IS NULL
          AND original.metadata ->> 'status' = 'REVERSED'
        GROUP BY original.tenant_id, original.id
        HAVING COUNT(reversal.id) <> 1
    ) THEN
        RAISE EXCEPTION 'cannot enforce journal reversal invariant: existing REVERSED entry lacks exactly one mirror';
    END IF;
END
$journal_reversal_preflight$;

DROP TRIGGER IF EXISTS trg_require_single_journal_reversal
    ON financial.journal_entries;
CREATE CONSTRAINT TRIGGER trg_require_single_journal_reversal
    AFTER UPDATE ON financial.journal_entries
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW
    EXECUTE FUNCTION financial.require_single_journal_reversal_at_commit();

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_delete()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_delete_guard$
DECLARE
    privileged_cleanup BOOLEAN;
    cleanup_txid TEXT := current_setting('toca.journal_fixture_cleanup_txid', TRUE);
    cleanup_ids TEXT := current_setting('toca.journal_fixture_cleanup_ids', TRUE);
BEGIN
    SELECT COALESCE(
        (role.rolsuper OR role.rolbypassrls)
        AND cleanup_txid = pg_current_xact_id()::TEXT
        AND OLD.id::TEXT = ANY(string_to_array(cleanup_ids, ',')),
        FALSE
    )
    INTO privileged_cleanup
    FROM pg_catalog.pg_roles AS role
    WHERE role.rolname = CURRENT_USER;

    IF NOT COALESCE(privileged_cleanup, FALSE) THEN
        RAISE EXCEPTION 'journal entries are append-only and cannot be deleted'
            USING ERRCODE = '23514';
    END IF;
    RETURN OLD;
END
$journal_entry_delete_guard$;

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_truncate()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_truncate_guard$
BEGIN
    RAISE EXCEPTION 'journal entries are append-only and cannot be truncated'
        USING ERRCODE = '23514';
END
$journal_entry_truncate_guard$;

DROP TRIGGER IF EXISTS trg_guard_journal_entry_truncate
    ON financial.journal_entries;
CREATE TRIGGER trg_guard_journal_entry_truncate
    BEFORE TRUNCATE ON financial.journal_entries
    FOR EACH STATEMENT
    EXECUTE FUNCTION financial.guard_journal_entry_truncate();

REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON financial.journal_entries FROM PUBLIC, anon, authenticated;

CREATE OR REPLACE FUNCTION financial.is_managed_bill_payment_reversal_witness(
    p_tenant_id UUID,
    p_bill_id UUID,
    p_contract_id TEXT,
    p_bill_description TEXT,
    p_amount_cents BIGINT,
    p_payment_reference TEXT,
    p_payment_journal_source_id TEXT,
    p_paid_at TIMESTAMPTZ,
    p_reconciled_at TIMESTAMPTZ,
    p_reversal_journal_source_id TEXT,
    p_reversed_by TEXT,
    p_reversed_at TIMESTAMPTZ,
    p_reversal_reason TEXT
)
RETURNS BOOLEAN
LANGUAGE SQL
STABLE
AS $managed_bill_reversal_witness$
    SELECT COALESCE(
        p_paid_at IS NOT NULL
        AND p_reversed_at IS NOT NULL
        AND p_reversed_at >= p_paid_at
        AND (p_reconciled_at IS NULL OR p_reversed_at >= p_reconciled_at)
        AND EXISTS (
            SELECT 1
            FROM financial.journal_entries AS original_journal
            JOIN financial.journal_entries AS reversal_journal
              ON reversal_journal.tenant_id = original_journal.tenant_id
             AND reversal_journal.deleted_at IS NULL
             AND reversal_journal.metadata ->> 'id' = reversal_journal.id::TEXT
             AND reversal_journal.metadata ->> 'tenant_id' = p_tenant_id::TEXT
             AND reversal_journal.metadata ->> 'contract_id' = p_contract_id
             AND reversal_journal.metadata ->> 'operation_id' = p_payment_reference
             AND reversal_journal.metadata ->> 'source_event_id' = p_reversal_journal_source_id
             AND reversal_journal.metadata ->> 'reversal_of' = original_journal.id::TEXT
             AND reversal_journal.metadata ->> 'status' = 'POSTED'
             AND NULLIF(reversal_journal.metadata ->> 'reconciliation_date', '') IS NULL
             AND reversal_journal.metadata ->> 'reversed_by' = p_reversed_by
             AND NULLIF(reversal_journal.metadata ->> 'date', '')::TIMESTAMPTZ = p_reversed_at
             AND reversal_journal.metadata ->> 'description' =
                 'Estorno: ' || p_reversal_reason || ' (ref: ' || original_journal.id::TEXT || ')'
             AND reversal_journal.metadata -> 'legs' = jsonb_build_array(
                 jsonb_build_object(
                     'account', 'owner_payable',
                     'direction', 'credit',
                     'amount_cents', p_amount_cents,
                     'managed_bill_id', p_bill_id::TEXT
                 ),
                 jsonb_build_object(
                     'account', 'cash',
                     'direction', 'debit',
                     'amount_cents', p_amount_cents,
                     'managed_bill_id', p_bill_id::TEXT
                 )
             )
            WHERE original_journal.tenant_id = p_tenant_id
              AND original_journal.deleted_at IS NULL
              AND original_journal.metadata ->> 'id' = original_journal.id::TEXT
              AND original_journal.metadata ->> 'tenant_id' = p_tenant_id::TEXT
              AND original_journal.metadata ->> 'contract_id' = p_contract_id
              AND original_journal.metadata ->> 'operation_id' = p_payment_reference
              AND original_journal.metadata ->> 'source_event_id' = p_payment_journal_source_id
              AND original_journal.metadata ->> 'status' = 'REVERSED'
              AND NULLIF(BTRIM(original_journal.metadata ->> 'reversal_of'), '') IS NULL
              AND NULLIF(BTRIM(original_journal.metadata ->> 'reversed_by'), '') IS NULL
              AND NULLIF(original_journal.metadata ->> 'date', '')::TIMESTAMPTZ = p_paid_at
              AND NULLIF(original_journal.metadata ->> 'reconciliation_date', '')::TIMESTAMPTZ
                  IS NOT DISTINCT FROM p_reconciled_at
              AND original_journal.metadata ->> 'description' =
                  'Pagamento de conta administrada: ' || p_bill_description
              AND original_journal.metadata -> 'legs' = jsonb_build_array(
                  jsonb_build_object(
                      'account', 'owner_payable',
                      'direction', 'debit',
                      'amount_cents', p_amount_cents,
                      'managed_bill_id', p_bill_id::TEXT
                  ),
                  jsonb_build_object(
                      'account', 'cash',
                      'direction', 'credit',
                      'amount_cents', p_amount_cents,
                      'managed_bill_id', p_bill_id::TEXT
                  )
              )
        ),
        FALSE
    );
$managed_bill_reversal_witness$;

COMMIT;
