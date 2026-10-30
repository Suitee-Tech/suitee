-- Supabase's local `postgres` role has BYPASSRLS but is deliberately not a
-- superuser. Keep journal DELETE fail-closed in normal execution while letting
-- isolated integration fixtures remove their exact UUIDs under SET LOCAL.
--
-- This incremental replacement is needed for databases that already applied
-- 20260822123000 before the local-role distinction was exercised.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

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

COMMIT;
