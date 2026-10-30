-- Make the journal append-only at the database boundary and require a
-- ManagedBill payment reversal to point at the exact compensating posting.
-- This migration is intentionally incremental: 20260822120000 may already be
-- installed, so the existing functions/policies are replaced in place.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_insert_guard$
DECLARE
    debit_total NUMERIC;
    credit_total NUMERIC;
    original_metadata JSONB;
    expected_reversal_legs JSONB;
BEGIN
    IF jsonb_typeof(NEW.metadata) IS DISTINCT FROM 'object'
       OR NEW.metadata ->> 'id' IS DISTINCT FROM NEW.id::TEXT
       OR NEW.metadata ->> 'tenant_id' IS DISTINCT FROM NEW.tenant_id::TEXT THEN
        RAISE EXCEPTION 'journal row identity must match its metadata'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.metadata ->> 'status' IS DISTINCT FROM 'POSTED' THEN
        RAISE EXCEPTION 'journal entries must be inserted as POSTED'
            USING ERRCODE = '23514';
    END IF;

    IF NULLIF(NEW.metadata ->> 'date', '')::TIMESTAMPTZ IS NULL
       OR jsonb_typeof(NEW.metadata -> 'legs') IS DISTINCT FROM 'array'
       OR jsonb_array_length(NEW.metadata -> 'legs') < 2 THEN
        RAISE EXCEPTION 'journal entry requires a date and at least two legs'
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

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(NEW.metadata -> 'legs') AS item(leg)
        GROUP BY item.leg ->> 'account'
        HAVING COUNT(DISTINCT item.leg ->> 'direction') > 1
    ) THEN
        RAISE EXCEPTION 'journal account cannot appear on both sides of one entry'
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

    IF NULLIF(BTRIM(NEW.metadata ->> 'reversal_of'), '') IS NULL THEN
        IF NULLIF(BTRIM(NEW.metadata ->> 'reversed_by'), '') IS NOT NULL THEN
            RAISE EXCEPTION 'journal original entry cannot declare reversed_by'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        IF NULLIF(BTRIM(NEW.metadata ->> 'reversed_by'), '') IS NULL
           OR NULLIF(NEW.metadata ->> 'reconciliation_date', '') IS NOT NULL THEN
            RAISE EXCEPTION 'journal reversal requires reversed_by and cannot be reconciled'
                USING ERRCODE = '23514';
        END IF;

        SELECT original.metadata
        INTO original_metadata
        FROM financial.journal_entries AS original
        WHERE original.tenant_id = NEW.tenant_id
          AND original.id::TEXT = NEW.metadata ->> 'reversal_of'
          AND original.deleted_at IS NULL
          AND original.metadata ->> 'status' = 'REVERSED';

        IF original_metadata IS NULL THEN
            RAISE EXCEPTION 'journal reversal requires its REVERSED original entry'
                USING ERRCODE = '23514';
        END IF;

        IF NEW.metadata ->> 'contract_id' IS DISTINCT FROM original_metadata ->> 'contract_id'
           OR NEW.metadata ->> 'operation_id' IS DISTINCT FROM original_metadata ->> 'operation_id' THEN
            RAISE EXCEPTION 'journal reversal identity must match its original entry'
                USING ERRCODE = '23514';
        END IF;

        SELECT jsonb_agg(
            jsonb_set(
                item.leg,
                '{direction}',
                to_jsonb(CASE item.leg ->> 'direction'
                    WHEN 'debit' THEN 'credit'
                    WHEN 'credit' THEN 'debit'
                    ELSE '__invalid__'
                END),
                FALSE
            )
            ORDER BY item.ordinal
        )
        INTO expected_reversal_legs
        FROM jsonb_array_elements(original_metadata -> 'legs')
            WITH ORDINALITY AS item(leg, ordinal);

        IF expected_reversal_legs IS NULL
           OR NEW.metadata -> 'legs' IS DISTINCT FROM expected_reversal_legs THEN
            RAISE EXCEPTION 'journal reversal must exactly mirror its original legs'
                USING ERRCODE = '23514';
        END IF;
    END IF;

    RETURN NEW;
END
$journal_entry_insert_guard$;

DROP TRIGGER IF EXISTS trg_guard_journal_entry_insert
    ON financial.journal_entries;
CREATE TRIGGER trg_guard_journal_entry_insert
    BEFORE INSERT ON financial.journal_entries
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_journal_entry_insert();

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_update_guard$
DECLARE
    old_status TEXT := OLD.metadata ->> 'status';
    new_status TEXT := NEW.metadata ->> 'status';
    old_reconciliation TEXT := NULLIF(OLD.metadata ->> 'reconciliation_date', '');
    new_reconciliation TEXT := NULLIF(NEW.metadata ->> 'reconciliation_date', '');
    new_metadata_updated_at TIMESTAMPTZ;
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
       OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at THEN
        RAISE EXCEPTION 'journal row identity and deletion marker are immutable'
            USING ERRCODE = '23514';
    END IF;

    IF jsonb_typeof(NEW.metadata) IS DISTINCT FROM 'object'
       OR NEW.metadata ->> 'id' IS DISTINCT FROM NEW.id::TEXT
       OR NEW.metadata ->> 'tenant_id' IS DISTINCT FROM NEW.tenant_id::TEXT THEN
        RAISE EXCEPTION 'journal row identity must match its metadata'
            USING ERRCODE = '23514';
    END IF;

    IF NULLIF(NEW.metadata ->> 'updated_at', '') IS NULL THEN
        RAISE EXCEPTION 'journal metadata updated_at is required'
            USING ERRCODE = '23514';
    END IF;
    new_metadata_updated_at := (NEW.metadata ->> 'updated_at')::TIMESTAMPTZ;
    IF new_metadata_updated_at IS DISTINCT FROM NEW.updated_at THEN
        RAISE EXCEPTION 'journal metadata updated_at must match the row timestamp'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.metadata - ARRAY['status', 'reconciliation_date', 'updated_at']::TEXT[])
       IS DISTINCT FROM
       (OLD.metadata - ARRAY['status', 'reconciliation_date', 'updated_at']::TEXT[]) THEN
        RAISE EXCEPTION 'journal economic payload is immutable'
            USING ERRCODE = '23514';
    END IF;

    IF NULLIF(NEW.metadata ->> 'updated_at', '')::TIMESTAMPTZ
           < NULLIF(OLD.metadata ->> 'updated_at', '')::TIMESTAMPTZ
       OR NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'journal updated_at must be monotonic'
            USING ERRCODE = '23514';
    END IF;

    IF old_status = 'POSTED' AND new_status = 'POSTED' THEN
        IF old_reconciliation IS NOT NULL
           OR new_reconciliation IS NULL THEN
            RAISE EXCEPTION 'journal reconciliation is append-only'
                USING ERRCODE = '23514';
        END IF;
        PERFORM new_reconciliation::TIMESTAMPTZ;
    ELSIF old_status = 'POSTED' AND new_status = 'REVERSED' THEN
        IF old_reconciliation IS DISTINCT FROM new_reconciliation THEN
            RAISE EXCEPTION 'journal reversal cannot rewrite reconciliation metadata'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        RAISE EXCEPTION 'invalid journal update: only reconciliation or POSTED to REVERSED is allowed'
            USING ERRCODE = '23514';
    END IF;

    RETURN NEW;
END
$journal_entry_update_guard$;

DROP TRIGGER IF EXISTS trg_guard_journal_entry_update
    ON financial.journal_entries;
CREATE TRIGGER trg_guard_journal_entry_update
    BEFORE UPDATE ON financial.journal_entries
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_journal_entry_update();

CREATE OR REPLACE FUNCTION financial.guard_journal_entry_delete()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $journal_entry_delete_guard$
DECLARE
    privileged_cleanup BOOLEAN;
    cleanup_txid TEXT := current_setting('toca.journal_fixture_cleanup_txid', TRUE);
    cleanup_ids TEXT := current_setting('toca.journal_fixture_cleanup_ids', TRUE);
BEGIN
    -- Fixture cleanup is an explicit transaction-local escape hatch. The role
    -- must also be superuser/BYPASSRLS, so setting the custom GUC cannot grant
    -- DELETE to authenticated clients after their DML policies/grants vanish.
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

DROP TRIGGER IF EXISTS trg_guard_journal_entry_delete
    ON financial.journal_entries;
CREATE TRIGGER trg_guard_journal_entry_delete
    BEFORE DELETE ON financial.journal_entries
    FOR EACH ROW
    EXECUTE FUNCTION financial.guard_journal_entry_delete();

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

DROP POLICY IF EXISTS tenant_isolation_modify
    ON financial.journal_entries;
DROP POLICY IF EXISTS tenant_isolation_insert
    ON financial.journal_entries;
DROP POLICY IF EXISTS tenant_isolation_update
    ON financial.journal_entries;
DROP POLICY IF EXISTS tenant_isolation_delete
    ON financial.journal_entries;
REVOKE INSERT, UPDATE, DELETE ON financial.journal_entries FROM PUBLIC, anon, authenticated;
REVOKE TRUNCATE ON financial.journal_entries FROM PUBLIC, anon, authenticated;

-- One immutable journal entry can have only one compensating mirror. The
-- source_event_id index protects one command identity; this second identity
-- protects the economic fact itself even if a privileged writer supplies a
-- different command/source ID concurrently.
CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_reversal_of
    ON financial.journal_entries (tenant_id, ((metadata ->> 'reversal_of')))
    WHERE deleted_at IS NULL
      AND NULLIF(BTRIM(metadata ->> 'reversal_of'), '') IS NOT NULL;

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

CREATE OR REPLACE FUNCTION financial.guard_managed_bill_update()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $managed_bill_guard$
DECLARE
    is_payment_reversal BOOLEAN := (
        OLD.status IN ('PAID', 'RECONCILED') AND NEW.status = 'APPROVED'
    );
    latest_attempt JSONB;
    latest_history JSONB;
BEGIN
    IF NEW.id IS DISTINCT FROM OLD.id
       OR NEW.tenant_id IS DISTINCT FROM OLD.tenant_id
       OR NEW.source_transfer_id IS DISTINCT FROM OLD.source_transfer_id
       OR NEW.source_charge_id IS DISTINCT FROM OLD.source_charge_id
       OR NEW.charge_item_id IS DISTINCT FROM OLD.charge_item_id
       OR NEW.source_key IS DISTINCT FROM OLD.source_key THEN
        RAISE EXCEPTION 'managed bill source identity is immutable'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.contract_id IS DISTINCT FROM OLD.contract_id
       OR NEW.property_id IS DISTINCT FROM OLD.property_id
       OR NEW.owner_id IS DISTINCT FROM OLD.owner_id
       OR NEW.component IS DISTINCT FROM OLD.component
       OR NEW.creditor IS DISTINCT FROM OLD.creditor
       OR NEW.description IS DISTINCT FROM OLD.description
       OR NEW.amount_cents IS DISTINCT FROM OLD.amount_cents
       OR NEW.currency IS DISTINCT FROM OLD.currency
       OR NEW.due_date IS DISTINCT FROM OLD.due_date
       OR NEW.competence_month IS DISTINCT FROM OLD.competence_month
       OR NEW.reserved_by IS DISTINCT FROM OLD.reserved_by
       OR NEW.reserved_at IS DISTINCT FROM OLD.reserved_at
       OR NEW.created_at IS DISTINCT FROM OLD.created_at THEN
        RAISE EXCEPTION 'managed bill economic attributes are immutable'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.version IS DISTINCT FROM OLD.version + 1 THEN
        RAISE EXCEPTION 'managed bill version must increment by exactly one'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.status IS DISTINCT FROM OLD.status
       AND NOT (
            (OLD.status = 'RESERVED' AND NEW.status IN ('APPROVED', 'CANCELLED'))
            OR (OLD.status = 'APPROVED' AND NEW.status IN ('PAID', 'CANCELLED'))
            OR (OLD.status = 'PAID' AND NEW.status = 'RECONCILED')
            OR is_payment_reversal
       ) THEN
        RAISE EXCEPTION 'invalid managed bill transition: % -> %', OLD.status, NEW.status
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.approved_by IS DISTINCT FROM OLD.approved_by
        OR NEW.approved_at IS DISTINCT FROM OLD.approved_at)
       AND NOT (OLD.status = 'RESERVED' AND NEW.status = 'APPROVED') THEN
        RAISE EXCEPTION 'managed bill approval milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.paid_by IS DISTINCT FROM OLD.paid_by
        OR NEW.paid_at IS DISTINCT FROM OLD.paid_at
        OR NEW.payment_reference IS DISTINCT FROM OLD.payment_reference
        OR NEW.payment_journal_source_id IS DISTINCT FROM OLD.payment_journal_source_id)
       AND NOT (
            (OLD.status = 'APPROVED' AND NEW.status = 'PAID')
            OR is_payment_reversal
       ) THEN
        RAISE EXCEPTION 'managed bill payment milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.reconciled_by IS DISTINCT FROM OLD.reconciled_by
        OR NEW.reconciled_at IS DISTINCT FROM OLD.reconciled_at
        OR NEW.reconciliation_reference IS DISTINCT FROM OLD.reconciliation_reference)
       AND NOT (
            (OLD.status = 'PAID' AND NEW.status = 'RECONCILED')
            OR is_payment_reversal
       ) THEN
        RAISE EXCEPTION 'managed bill reconciliation milestone is write-once'
            USING ERRCODE = '23514';
    END IF;

    IF (NEW.cancelled_by IS DISTINCT FROM OLD.cancelled_by
        OR NEW.cancelled_at IS DISTINCT FROM OLD.cancelled_at
        OR NEW.cancel_reason IS DISTINCT FROM OLD.cancel_reason)
       AND NOT (
            OLD.status IN ('RESERVED', 'APPROVED')
            AND NEW.status = 'CANCELLED'
       ) THEN
        RAISE EXCEPTION 'managed bill lifecycle milestones are write-once'
            USING ERRCODE = '23514';
    END IF;

    IF NEW.updated_at < OLD.updated_at THEN
        RAISE EXCEPTION 'managed bill updated_at must be monotonic'
            USING ERRCODE = '23514';
    END IF;

    IF jsonb_array_length(NEW.history) <> jsonb_array_length(OLD.history) + 1 THEN
        RAISE EXCEPTION 'managed bill history must append exactly one entry'
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(OLD.history) WITH ORDINALITY AS prior(entry, ordinal)
        WHERE NEW.history -> ((prior.ordinal - 1)::INTEGER) IS DISTINCT FROM prior.entry
    ) THEN
        RAISE EXCEPTION 'managed bill history entries are immutable'
            USING ERRCODE = '23514';
    END IF;

    latest_history := NEW.history -> -1;
    IF NULLIF(latest_history ->> 'version', '')::BIGINT IS DISTINCT FROM NEW.version THEN
        RAISE EXCEPTION 'managed bill history version must match row version'
            USING ERRCODE = '23514';
    END IF;

    IF jsonb_array_length(NEW.evidence) < jsonb_array_length(OLD.evidence) THEN
        RAISE EXCEPTION 'managed bill evidence is append-only'
            USING ERRCODE = '23514';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM jsonb_array_elements(OLD.evidence) WITH ORDINALITY AS prior(entry, ordinal)
        WHERE NEW.evidence -> ((prior.ordinal - 1)::INTEGER) IS DISTINCT FROM prior.entry
    ) THEN
        RAISE EXCEPTION 'managed bill evidence entries are immutable'
            USING ERRCODE = '23514';
    END IF;

    IF is_payment_reversal THEN
        IF NEW.paid_by IS NOT NULL OR NEW.paid_at IS NOT NULL
           OR NEW.payment_reference IS NOT NULL OR NEW.payment_journal_source_id IS NOT NULL
           OR NEW.reconciled_by IS NOT NULL OR NEW.reconciled_at IS NOT NULL
           OR NEW.reconciliation_reference IS NOT NULL THEN
            RAISE EXCEPTION 'managed bill active payment milestones must be cleared on reversal'
                USING ERRCODE = '23514';
        END IF;

        IF jsonb_array_length(NEW.payment_attempts) <> jsonb_array_length(OLD.payment_attempts) + 1 THEN
            RAISE EXCEPTION 'managed bill payment reversal must append exactly one payment attempt'
                USING ERRCODE = '23514';
        END IF;

        IF EXISTS (
            SELECT 1
            FROM jsonb_array_elements(OLD.payment_attempts) WITH ORDINALITY AS prior(entry, ordinal)
            WHERE NEW.payment_attempts -> ((prior.ordinal - 1)::INTEGER) IS DISTINCT FROM prior.entry
        ) THEN
            RAISE EXCEPTION 'managed bill payment attempts are immutable'
                USING ERRCODE = '23514';
        END IF;

        latest_attempt := NEW.payment_attempts -> -1;
        IF NULLIF(BTRIM(latest_attempt ->> 'id'), '') IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversal_reference'), '') IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversal_journal_source_id'), '') IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversed_by'), '') IS NULL
           OR NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ IS NULL
           OR NULLIF(BTRIM(latest_attempt ->> 'reversal_reason'), '') IS NULL THEN
            RAISE EXCEPTION 'managed bill payment reversal metadata is required'
                USING ERRCODE = '23514';
        END IF;

        IF latest_attempt ->> 'id' IS DISTINCT FROM OLD.payment_journal_source_id THEN
            RAISE EXCEPTION 'managed bill payment attempt id must match the payment journal source'
                USING ERRCODE = '23514';
        END IF;

        IF EXISTS (
            SELECT 1
            FROM jsonb_array_elements(OLD.payment_attempts) AS prior(entry)
            WHERE prior.entry ->> 'id' = latest_attempt ->> 'id'
        ) THEN
            RAISE EXCEPTION 'managed bill payment attempt id must be unique'
                USING ERRCODE = '23514';
        END IF;

        IF EXISTS (
            SELECT 1
            FROM jsonb_array_elements(OLD.payment_attempts) AS prior(entry)
            WHERE prior.entry ->> 'reversal_reference' = latest_attempt ->> 'reversal_reference'
               OR prior.entry ->> 'reversal_journal_source_id' = latest_attempt ->> 'reversal_journal_source_id'
        ) THEN
            RAISE EXCEPTION 'managed bill payment reversal references must be unique'
                USING ERRCODE = '23514';
        END IF;

        IF latest_attempt ->> 'payment_reference' IS DISTINCT FROM OLD.payment_reference
           OR latest_attempt ->> 'payment_journal_source_id' IS DISTINCT FROM OLD.payment_journal_source_id
           OR latest_attempt ->> 'paid_by' IS DISTINCT FROM OLD.paid_by
           OR NULLIF(latest_attempt ->> 'paid_at', '')::TIMESTAMPTZ IS DISTINCT FROM OLD.paid_at
           OR NULLIF(latest_attempt ->> 'reconciled_by', '') IS DISTINCT FROM OLD.reconciled_by
           OR NULLIF(latest_attempt ->> 'reconciled_at', '')::TIMESTAMPTZ IS DISTINCT FROM OLD.reconciled_at
           OR NULLIF(latest_attempt ->> 'reconciliation_reference', '') IS DISTINCT FROM OLD.reconciliation_reference THEN
            RAISE EXCEPTION 'managed bill payment attempt does not match active payment snapshot'
                USING ERRCODE = '23514';
        END IF;

        IF NOT financial.is_managed_bill_payment_reversal_witness(
            OLD.tenant_id,
            OLD.id,
            OLD.contract_id,
            OLD.description,
            OLD.amount_cents,
            OLD.payment_reference,
            OLD.payment_journal_source_id,
            OLD.paid_at,
            OLD.reconciled_at,
            latest_attempt ->> 'reversal_journal_source_id',
            latest_attempt ->> 'reversed_by',
            NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ,
            latest_attempt ->> 'reversal_reason'
        ) THEN
            RAISE EXCEPTION 'managed bill payment reversal journal is missing or economically inconsistent'
                USING ERRCODE = '23514';
        END IF;

        IF latest_history ->> 'action' IS DISTINCT FROM 'payment_reversed'
           OR latest_history ->> 'actor' IS DISTINCT FROM latest_attempt ->> 'reversed_by'
           OR NULLIF(latest_history ->> 'at', '')::TIMESTAMPTZ IS DISTINCT FROM NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ
           OR NULLIF(latest_history ->> 'reference', '') IS DISTINCT FROM NULLIF(latest_attempt ->> 'reversal_reference', '')
           OR NULLIF(latest_history ->> 'reason', '') IS DISTINCT FROM NULLIF(latest_attempt ->> 'reversal_reason', '')
           OR NULLIF(latest_attempt ->> 'reversed_at', '')::TIMESTAMPTZ IS DISTINCT FROM NEW.updated_at THEN
            RAISE EXCEPTION 'managed bill payment reversal history is inconsistent'
                USING ERRCODE = '23514';
        END IF;
    ELSE
        IF NEW.payment_attempts IS DISTINCT FROM OLD.payment_attempts THEN
            RAISE EXCEPTION 'managed bill payment attempts may change only on payment reversal'
                USING ERRCODE = '23514';
        END IF;
        IF latest_history ->> 'action' = 'payment_reversed' THEN
            RAISE EXCEPTION 'managed bill payment_reversed history requires a payment reversal'
                USING ERRCODE = '23514';
        END IF;
    END IF;

    RETURN NEW;
END
$managed_bill_guard$;

COMMIT;
