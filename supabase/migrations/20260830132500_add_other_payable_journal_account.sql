-- Register non-tax commission withholdings as an explicit liability instead
-- of collapsing them into tax or partner balances. Both insert guards carry
-- the journal account allow-list and must evolve together.

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
                'tax_withheld', 'other_payable', 'withdrawal_payable',
                'equity', 'revenue', 'commission_expense', 'expense'
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
                'tax_withheld', 'other_payable', 'withdrawal_payable',
                'equity', 'revenue', 'commission_expense', 'expense'
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

COMMIT;
