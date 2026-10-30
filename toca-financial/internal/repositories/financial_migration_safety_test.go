package repositories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readFinancialMigration(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "supabase", "migrations", name)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read financial migration %s: %v", name, err)
	}
	return string(payload)
}

func requireMigrationFragment(t *testing.T, migration, fragment string) {
	t.Helper()
	if !strings.Contains(migration, fragment) {
		t.Fatalf("migration is missing safety fragment %q", fragment)
	}
}

func TestRentalChargeConfigMigrationRewritesEveryLegacyFieldBeforeDeduplication(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260810121000_rental_charge_config_contract_unique.sql")

	for _, mapping := range []string{
		"('ID', 'id')",
		"('TenantID', 'tenant_id')",
		"('PropertyID', 'property_id')",
		"('ContractID', 'contract_id')",
		"('TenantPartyID', 'tenant_party_id')",
		"('OwnerID', 'owner_id')",
		"('StartDate', 'start_date')",
		"('EndDate', 'end_date')",
		"('FirstChargeDate', 'first_charge_date')",
		"('FirstChargeType', 'first_charge_type')",
		"('DefaultDueDay', 'default_due_day')",
		"('MonthlyAmount', 'monthly_amount')",
		"('BaseMonthlyAmount', 'base_monthly_amount')",
		"('Adjustments', 'adjustments')",
		"('FirstChargePaid', 'first_charge_paid')",
		"('MonthlyProductRevision', 'monthly_product_revision')",
		"('BundledItems', 'bundled_items')",
		"('CreatedAt', 'created_at')",
		"('UpdatedAt', 'updated_at')",
	} {
		requireMigrationFragment(t, migration, mapping)
	}

	requireMigrationFragment(t, migration, "item.value")
	requireMigrationFragment(t, migration, "OR NOT (cfg.metadata ? key_map.new_key)")
	rewriteAt := strings.Index(migration, "UPDATE financial.rental_charge_configs AS cfg\nSET metadata")
	dedupeAt := strings.Index(migration, "WITH ranked AS")
	if rewriteAt < 0 || dedupeAt < 0 || rewriteAt >= dedupeAt {
		t.Fatalf("legacy JSON rewrite must complete before rental deduplication")
	}
}

func TestSaleBillingPlanMigrationAbortsBeforeUnsafeChargeReconciliation(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260810124000_sale_billing_plan_contract_unique.sql")

	for _, fragment := range []string{
		"LOCK TABLE financial.sale_billing_plan IN SHARE ROW EXCLUSIVE MODE",
		"LOCK TABLE financial.charges IN SHARE MODE",
		"HAVING COUNT(*) FILTER (WHERE charge_count > 0) > 1",
		"unsafe SALE billing-plan deduplication: multiple plans already own charges",
		"No plan was changed.",
		"ORDER BY has_materialized_charges DESC, created_at ASC, id ASC",
	} {
		requireMigrationFragment(t, migration, fragment)
	}

	safetyCheckAt := strings.Index(migration, "unsafe SALE billing-plan deduplication")
	softDeleteAt := strings.Index(migration, "UPDATE financial.sale_billing_plan AS plan")
	if safetyCheckAt < 0 || softDeleteAt < 0 || safetyCheckAt >= softDeleteAt {
		t.Fatalf("unsafe duplicate detection must run before any SALE plan is soft-deleted")
	}
}

func TestManualReceiptTransferIdentityMigrationSeparatesCanonicalAndLegacySources(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260829221908_transfer_source_transaction_identity.sql")

	for _, fragment := range []string{
		"CREATE UNIQUE INDEX IF NOT EXISTS uq_finance_transfers_source_transaction",
		"ON financial.transfers (",
		"tenant_id,",
		"((metadata->>'source_transaction_id'))",
		"NULLIF(metadata->>'source_transaction_id', '') IS NOT NULL",
		"DROP INDEX IF EXISTS financial.uq_finance_transfers_monthly_legacy",
		"NULLIF(metadata->>'source_charge_id', '') IS NULL",
		"NULLIF(metadata->>'source_transaction_id', '') IS NULL",
		"receipt.tenant_id",
		"aggregate.tenant_id = receipt.tenant_id",
		"receipt.metadata->'economic_breakdown' IS NULL",
		"MIN(aggregate.metadata->>'deal_id') = MAX(aggregate.metadata->>'deal_id')",
		"valid.gross_cents - valid.commission_pool_cents",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}

func TestPaidManualSaleTransferBackfillFailsClosedAndPreservesTenantScope(t *testing.T) {
	t.Parallel()
	migration := readFinancialMigration(t, "20260830041920_backfill_paid_manual_sale_owner_transfers.sql")

	for _, fragment := range []string{
		"JOIN deal.deals AS deal",
		"deal.tenant_id = receipt.tenant_id",
		"UPPER(COALESCE(receipt.metadata->>'status', '')) = 'PAID'",
		"(receipt.metadata#>>'{economic_breakdown,flow}') = 'sale'",
		"UPPER(COALESCE(deal.metadata->>'type', '')) = 'SALE'",
		"commission_pool_cents = commission_payable_cents + partner_payable_cents + agency_retained_cents",
		"gross_cents - discount_cents = owner_payable_cents + commission_pool_cents",
		"existing.metadata->>'source_transaction_id' = normalized.receipt_id::text",
		"COALESCE(legacy.metadata->>'source_charge_id', '') = ''",
		"COALESCE(legacy.metadata->>'source_transaction_id', '') = ''",
		"'owner_id', candidate.owner_id",
		"'property_id', candidate.property_id",
		"'owner_balance_base_total_cents', candidate.owner_payable_cents",
		"'status', 'PENDING'",
		"ON CONFLICT DO NOTHING",
	} {
		requireMigrationFragment(t, migration, fragment)
	}
}
