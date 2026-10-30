-- Prevent more than one compensating journal entry from reversing the same
-- original posting. This incremental migration covers databases that applied
-- 20260822123000 before the economic-identity index was added there.

BEGIN;

SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '60s';

CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_reversal_of
    ON financial.journal_entries (tenant_id, ((metadata ->> 'reversal_of')))
    WHERE deleted_at IS NULL
      AND NULLIF(BTRIM(metadata ->> 'reversal_of'), '') IS NOT NULL;

COMMIT;
