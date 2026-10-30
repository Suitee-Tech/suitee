package repositories

import (
	"strings"
	"testing"
)

func TestManagedBillMigrationDefinesNormalizedTenantSafeLifecycle(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260820234459_create_financial_managed_bills.sql")

	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS financial.managed_bills",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_charges_tenant_id_id",
		"FOREIGN KEY (tenant_id, source_transfer_id)",
		"REFERENCES financial.transfers (tenant_id, id)",
		"FOREIGN KEY (tenant_id, source_charge_id)",
		"REFERENCES financial.charges (tenant_id, id)",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_managed_bills_source_key",
		"ON financial.managed_bills (tenant_id, source_key)",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_managed_bills_transfer_item",
		"ON financial.managed_bills (tenant_id, source_transfer_id, charge_item_id)",
		"amount_cents        BIGINT      NOT NULL CHECK (amount_cents > 0)",
		"currency            TEXT        NOT NULL DEFAULT 'BRL' CHECK (currency = 'BRL')",
		"status IN ('RESERVED', 'APPROVED', 'PAID', 'RECONCILED', 'CANCELLED')",
		"version             BIGINT      NOT NULL DEFAULT 1 CHECK (version > 0)",
		"jsonb_typeof(evidence) = 'array'",
		"jsonb_typeof(history) = 'array'",
		"CREATE INDEX IF NOT EXISTS idx_finance_managed_bills_tenant_status_due",
		"ON financial.managed_bills (tenant_id, status, due_date, id)",
		"CREATE INDEX IF NOT EXISTS idx_finance_managed_bills_tenant_contract_competence",
		"ON financial.managed_bills (tenant_id, contract_id, competence_month, created_at DESC)",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestManagedBillMigrationEnforcesCASImmutableHistoryAndLifecycle(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260820234459_create_financial_managed_bills.sql")

	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION financial.guard_managed_bill_insert()",
		"managed bill must be inserted as RESERVED version 1",
		"managed bill lifecycle milestones must be empty on insert",
		"managed bill initial history is invalid",
		"CREATE TRIGGER trg_guard_managed_bill_insert",
		"CREATE OR REPLACE FUNCTION financial.guard_managed_bill_update()",
		"NEW.source_key IS DISTINCT FROM OLD.source_key",
		"NEW.amount_cents IS DISTINCT FROM OLD.amount_cents",
		"NEW.owner_id IS DISTINCT FROM OLD.owner_id",
		"NEW.due_date IS DISTINCT FROM OLD.due_date",
		"managed bill economic attributes are immutable",
		"NEW.version IS DISTINCT FROM OLD.version + 1",
		"OLD.status = 'RESERVED' AND NEW.status IN ('APPROVED', 'CANCELLED')",
		"OLD.status = 'APPROVED' AND NEW.status IN ('PAID', 'CANCELLED')",
		"OLD.status = 'PAID' AND NEW.status = 'RECONCILED'",
		"managed bill approval milestone is write-once",
		"managed bill payment milestone is write-once",
		"managed bill reconciliation milestone is write-once",
		"managed bill lifecycle milestones are write-once",
		"managed bill updated_at must be monotonic",
		"managed bill history must append exactly one entry",
		"managed bill history entries are immutable",
		"managed bill history version must match row version",
		"managed bill evidence is append-only",
		"managed bill evidence entries are immutable",
		"CREATE TRIGGER trg_guard_managed_bill_update",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestManagedBillMigrationHasTenantRLSWithoutDeletePolicy(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260820234459_create_financial_managed_bills.sql")

	for _, fragment := range []string{
		"ALTER TABLE financial.managed_bills ENABLE ROW LEVEL SECURITY",
		"CREATE POLICY tenant_isolation_select ON financial.managed_bills",
		"FOR SELECT TO authenticated",
		"CREATE POLICY tenant_isolation_insert ON financial.managed_bills",
		"FOR INSERT TO authenticated",
		"CREATE POLICY tenant_isolation_update ON financial.managed_bills",
		"FOR UPDATE TO authenticated",
		"(SELECT auth.jwt()) -> 'app_metadata' ->> 'tenant_id'",
		"REVOKE DELETE ON financial.managed_bills FROM authenticated",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
	if strings.Contains(strings.ToUpper(migration), "FOR DELETE TO AUTHENTICATED") {
		t.Fatal("managed bill lifecycle must not expose a DELETE policy")
	}
}

func TestManagedBillPaymentAttemptsMigrationArchivesOnlyCoherentReversals(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822120000_add_managed_bill_payment_attempts.sql")

	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS payment_attempts JSONB NOT NULL DEFAULT '[]'::jsonb",
		"jsonb_typeof(payment_attempts) = 'array'",
		"CREATE OR REPLACE FUNCTION financial.guard_managed_bill_insert()",
		"managed bill payment attempts must be empty on insert",
		"CREATE OR REPLACE FUNCTION financial.guard_managed_bill_update()",
		"OLD.status IN ('PAID', 'RECONCILED') AND NEW.status = 'APPROVED'",
		"managed bill active payment milestones must be cleared on reversal",
		"jsonb_array_length(NEW.payment_attempts) <> jsonb_array_length(OLD.payment_attempts) + 1",
		"managed bill payment reversal must append exactly one payment attempt",
		"managed bill payment attempts are immutable",
		"latest_attempt ->> 'id' IS DISTINCT FROM OLD.payment_journal_source_id",
		"managed bill payment attempt id must match the payment journal source",
		"prior.entry ->> 'reversal_reference' = latest_attempt ->> 'reversal_reference'",
		"prior.entry ->> 'reversal_journal_source_id' = latest_attempt ->> 'reversal_journal_source_id'",
		"managed bill payment reversal references must be unique",
		"managed bill payment attempt does not match active payment snapshot",
		"reversal_journal.metadata ->> 'source_event_id' = latest_attempt ->> 'reversal_journal_source_id'",
		"reversal_journal.metadata ->> 'reversal_of' = original_journal.id::TEXT",
		"original_journal.tenant_id = OLD.tenant_id",
		"original_journal.metadata ->> 'source_event_id' = OLD.payment_journal_source_id",
		"managed bill payment reversal journal is missing or inconsistent",
		"latest_history ->> 'action' IS DISTINCT FROM 'payment_reversed'",
		"managed bill payment reversal history is inconsistent",
		"managed bill payment attempts may change only on payment reversal",
		"managed bill payment_reversed history requires a payment reversal",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestManagedBillPaymentAttemptsMigrationPreservesExistingGuards(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822120000_add_managed_bill_payment_attempts.sql")

	for _, fragment := range []string{
		"managed bill source identity is immutable",
		"managed bill economic attributes are immutable",
		"managed bill version must increment by exactly one",
		"managed bill approval milestone is write-once",
		"managed bill payment milestone is write-once",
		"managed bill reconciliation milestone is write-once",
		"managed bill lifecycle milestones are write-once",
		"managed bill updated_at must be monotonic",
		"managed bill history must append exactly one entry",
		"managed bill history entries are immutable",
		"managed bill history version must match row version",
		"managed bill evidence is append-only",
		"managed bill evidence entries are immutable",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestJournalManagedBillReversalHardeningMigrationIsAppendOnlyAndReadOnlyForAuthenticated(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822123000_harden_journal_managed_bill_reversals.sql")

	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION financial.guard_journal_entry_insert()",
		"journal entry must be balanced",
		"item.leg ->> 'account' IS NULL",
		"item.leg ->> 'direction' IS NULL",
		"9223372036854775807",
		"balanced with positive totals",
		"journal original entry cannot declare reversed_by",
		"journal reversal requires reversed_by",
		"journal reversal must exactly mirror its original legs",
		"CREATE TRIGGER trg_guard_journal_entry_insert",
		"CREATE OR REPLACE FUNCTION financial.guard_journal_entry_update()",
		"journal economic payload is immutable",
		"journal metadata updated_at is required",
		"journal metadata updated_at must match the row timestamp",
		"journal reconciliation is append-only",
		"old_status = 'POSTED' AND new_status = 'REVERSED'",
		"CREATE TRIGGER trg_guard_journal_entry_update",
		"CREATE OR REPLACE FUNCTION financial.guard_journal_entry_delete()",
		"current_setting('toca.journal_fixture_cleanup_txid', TRUE)",
		"current_setting('toca.journal_fixture_cleanup_ids', TRUE)",
		"cleanup_txid = pg_current_xact_id()::TEXT",
		"OLD.id::TEXT = ANY(string_to_array(cleanup_ids, ','))",
		"journal entries are append-only and cannot be deleted",
		"CREATE TRIGGER trg_guard_journal_entry_delete",
		"CREATE TRIGGER trg_guard_journal_entry_truncate",
		"journal entries are append-only and cannot be truncated",
		"REVOKE INSERT, UPDATE, DELETE ON financial.journal_entries FROM PUBLIC, anon, authenticated",
		"REVOKE TRUNCATE ON financial.journal_entries FROM PUBLIC, anon, authenticated",
		"CREATE CONSTRAINT TRIGGER trg_require_single_journal_reversal",
		"DEFERRABLE INITIALLY DEFERRED",
		"existing REVERSED entry lacks exactly one mirror",
		"journal reversal must commit with exactly one compensating entry",
		"DROP POLICY IF EXISTS tenant_isolation_modify",
		"DROP POLICY IF EXISTS tenant_isolation_insert",
		"DROP POLICY IF EXISTS tenant_isolation_update",
		"DROP POLICY IF EXISTS tenant_isolation_delete",
		"REVOKE INSERT, UPDATE, DELETE ON financial.journal_entries FROM PUBLIC, anon, authenticated",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_reversal_of",
		"NULLIF(BTRIM(metadata ->> 'reversal_of'), '') IS NOT NULL",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
	if strings.Contains(migration, "CREATE POLICY tenant_isolation_insert ON financial.journal_entries") ||
		strings.Contains(migration, "CREATE POLICY tenant_isolation_update ON financial.journal_entries") ||
		strings.Contains(migration, "CREATE POLICY tenant_isolation_delete ON financial.journal_entries") {
		t.Fatal("authenticated journal access must remain read-only")
	}
	if strings.Contains(migration, "ARRAY['status', 'reconciliation_date', 'updated_at', 'reversed_by']") {
		t.Fatal("reversed_by must remain part of the immutable journal payload")
	}
}

func TestJournalReversalUniquenessMigrationProtectsEconomicIdentity(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822125000_enforce_unique_journal_reversal.sql")
	for _, fragment := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_reversal_of",
		"ON financial.journal_entries (tenant_id, ((metadata ->> 'reversal_of')))",
		"WHERE deleted_at IS NULL",
		"NULLIF(BTRIM(metadata ->> 'reversal_of'), '') IS NOT NULL",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestJournalManagedBillReversalHardeningMigrationRequiresExactEconomicWitness(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822123000_harden_journal_managed_bill_reversals.sql")

	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION financial.is_managed_bill_payment_reversal_witness(",
		"reversal_journal.metadata ->> 'reversed_by' = p_reversed_by",
		"reversal_journal.metadata ->> 'description' =",
		"'Estorno: ' || p_reversal_reason",
		"original_journal.metadata ->> 'operation_id' = p_payment_reference",
		"original_journal.metadata ->> 'description' =",
		"'Pagamento de conta administrada: ' || p_bill_description",
		"'account', 'owner_payable'",
		"'account', 'cash'",
		"'managed_bill_id', p_bill_id::TEXT",
		"NULLIF(original_journal.metadata ->> 'reconciliation_date', '')::TIMESTAMPTZ",
		"NULLIF(BTRIM(original_journal.metadata ->> 'reversal_of'), '') IS NULL",
		"NULLIF(BTRIM(original_journal.metadata ->> 'reversed_by'), '') IS NULL",
		"p_reversed_at >= p_paid_at",
		"p_reversed_at >= p_reconciled_at",
		"CREATE OR REPLACE FUNCTION financial.guard_managed_bill_update()",
		"IF NOT financial.is_managed_bill_payment_reversal_witness(",
		"managed bill payment reversal journal is missing or economically inconsistent",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestJournalFixtureCleanupMigrationRequiresLocalFlagAndPrivilegedRole(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822124000_allow_journal_fixture_cleanup_for_bypassrls.sql")
	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION financial.guard_journal_entry_delete()",
		"role.rolsuper OR role.rolbypassrls",
		"current_setting('toca.journal_fixture_cleanup_txid', TRUE)",
		"current_setting('toca.journal_fixture_cleanup_ids', TRUE)",
		"cleanup_txid = pg_current_xact_id()::TEXT",
		"OLD.id::TEXT = ANY(string_to_array(cleanup_ids, ','))",
		"journal entries are append-only and cannot be deleted",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestJournalReversalGuardCompletionMigrationClosesDatabaseBypasses(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260822126000_complete_journal_reversal_guards.sql")
	for _, fragment := range []string{
		"CREATE OR REPLACE FUNCTION financial.guard_journal_entry_insert_strict()",
		"item.leg ->> 'account' IS NULL",
		"item.leg ->> 'direction' IS NULL",
		"9223372036854775807",
		"journal entry must be balanced with positive totals",
		"CREATE OR REPLACE FUNCTION financial.guard_journal_entry_update_timestamp()",
		"journal metadata updated_at must match the row timestamp",
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_journal_entries_reversal_of",
		"CREATE CONSTRAINT TRIGGER trg_require_single_journal_reversal",
		"DEFERRABLE INITIALLY DEFERRED",
		"existing REVERSED entry lacks exactly one mirror",
		"journal reversal must commit with exactly one compensating entry",
		"current_setting('toca.journal_fixture_cleanup_txid', TRUE)",
		"current_setting('toca.journal_fixture_cleanup_ids', TRUE)",
		"OLD.id::TEXT = ANY(string_to_array(cleanup_ids, ','))",
		"CREATE TRIGGER trg_guard_journal_entry_truncate",
		"REVOKE INSERT, UPDATE, DELETE, TRUNCATE ON financial.journal_entries FROM PUBLIC, anon, authenticated",
		"NULLIF(BTRIM(original_journal.metadata ->> 'reversal_of'), '') IS NULL",
		"NULLIF(BTRIM(original_journal.metadata ->> 'reversed_by'), '') IS NULL",
		"p_reversed_at >= p_paid_at",
		"p_reversed_at >= p_reconciled_at",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}
