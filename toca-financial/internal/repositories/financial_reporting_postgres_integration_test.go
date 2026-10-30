package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

// TestFinancialReportingPostgres_EndToEndConsistency compiles and executes
// every report/reconciliation query against migrated PostgreSQL. It is opt-in
// because unit-test environments do not necessarily expose the local stack.
// The fixture uses one fresh tenant and removes only the exact IDs it created.
func TestFinancialReportingPostgres_EndToEndConsistency(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         databaseURI,
		MaxPoolSize: 4,
		MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tenantID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	from := now.Add(-24 * time.Hour)
	to := now.Add(time.Hour)

	partialChargeID := uuid.NewString()
	partialPaymentOneID := uuid.NewString()
	partialPaymentTwoID := uuid.NewString()
	partialRevenueItemID := uuid.NewString()
	partialOwnerItemID := uuid.NewString()
	paidChargeID := uuid.NewString()
	reversedChargeID := uuid.NewString()
	pendingChargeID := uuid.NewString()
	originalRevenueID := uuid.NewString()
	reversalID := uuid.NewString()
	expenseID := uuid.NewString()
	repasseID := uuid.NewString()
	reversedRepasseID := uuid.NewString()
	repasseReversalID := uuid.NewString()
	legacyCommissionTxID := uuid.NewString()
	legacyCommissionID := uuid.NewString()
	orphanPayoutID := uuid.NewString()
	withdrawalID := uuid.NewString()
	transferID := uuid.NewString()

	chargeIDs := []string{partialChargeID, paidChargeID, reversedChargeID, pendingChargeID}
	transactionIDs := []string{
		originalRevenueID, reversalID, expenseID, repasseID,
		reversedRepasseID, repasseReversalID, legacyCommissionTxID,
	}
	cleanupDone := false
	cleanup := func() error {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM financial.charges
			WHERE tenant_id::text = $1::text AND id IN ($2, $3, $4, $5)
		`, tenantID, chargeIDs[0], chargeIDs[1], chargeIDs[2], chargeIDs[3]); err != nil {
			return fmt.Errorf("cleanup charges: %w", err)
		}
		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM financial.transactions
			WHERE tenant_id::text = $1::text AND id IN ($2, $3, $4, $5, $6, $7, $8)
		`, tenantID,
			transactionIDs[0], transactionIDs[1], transactionIDs[2], transactionIDs[3],
			transactionIDs[4], transactionIDs[5], transactionIDs[6]); err != nil {
			return fmt.Errorf("cleanup transactions: %w", err)
		}
		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM financial.commissions WHERE tenant_id::text = $1::text AND id = $2
		`, tenantID, legacyCommissionID); err != nil {
			return fmt.Errorf("cleanup commission: %w", err)
		}
		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM financial.payout_orders WHERE tenant_id::text = $1::text AND id = $2
		`, tenantID, orphanPayoutID); err != nil {
			return fmt.Errorf("cleanup payout order: %w", err)
		}
		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM financial.withdrawals WHERE tenant_id::text = $1::text AND id = $2
		`, tenantID, withdrawalID); err != nil {
			return fmt.Errorf("cleanup withdrawal: %w", err)
		}
		if _, err := db.Exec(cleanupCtx, `
			DELETE FROM financial.transfers WHERE tenant_id::text = $1::text AND id = $2
		`, tenantID, transferID); err != nil {
			return fmt.Errorf("cleanup transfer: %w", err)
		}
		cleanupDone = true
		return nil
	}
	t.Cleanup(func() {
		if cleanupDone {
			return
		}
		if err := cleanup(); err != nil {
			t.Errorf("fixture cleanup: %v", err)
		}
	})

	insertFinancialFixture(t, ctx, db, "charges", partialChargeID, tenantID, map[string]any{
		"id": partialChargeID, "tenant_id": tenantID, "contract_id": uuid.NewString(),
		"type": "SALE", "description": "Sinal / Entrada", "status": "Parcialmente Pago",
		"total_amount": 100.00, "paid_amount": 40.00, "due_date": now.Add(-48 * time.Hour),
		"paid_at": now.Add(-3 * time.Hour), "updated_at": now.Add(-3 * time.Hour),
		"line_items": []map[string]any{
			{"id": partialRevenueItemID, "description": "Receita própria", "amount_cents": 2000, "rule_version_id": "test-v1"},
			{"id": partialOwnerItemID, "description": "Valor de terceiro", "amount_cents": 8000, "rule_version_id": "test-v1"},
		},
		"payments": []map[string]any{
			{
				"id": partialPaymentOneID, "amount_cents": 2500, "method": "PIX", "paid_at": now.Add(-4 * time.Hour),
				"allocations": []map[string]any{
					{"id": uuid.NewString(), "payment_id": partialPaymentOneID, "charge_item_id": partialRevenueItemID, "amount_cents": 500, "allocation_type": "revenue", "beneficiary_party_id": uuid.NewString(), "rule_version_id": "test-v1"},
					{"id": uuid.NewString(), "payment_id": partialPaymentOneID, "charge_item_id": partialOwnerItemID, "amount_cents": 2000, "allocation_type": "owner_payable", "beneficiary_party_id": uuid.NewString(), "rule_version_id": "test-v1"},
				},
			},
			{
				"id": partialPaymentTwoID, "amount_cents": 1500, "method": "PIX", "paid_at": now.Add(-3 * time.Hour),
				"allocations": []map[string]any{
					{"id": uuid.NewString(), "payment_id": partialPaymentTwoID, "charge_item_id": partialRevenueItemID, "amount_cents": 300, "allocation_type": "revenue", "beneficiary_party_id": uuid.NewString(), "rule_version_id": "test-v1"},
					{"id": uuid.NewString(), "payment_id": partialPaymentTwoID, "charge_item_id": partialOwnerItemID, "amount_cents": 1200, "allocation_type": "owner_payable", "beneficiary_party_id": uuid.NewString(), "rule_version_id": "test-v1"},
				},
			},
		},
	}, now.Add(-72*time.Hour), now.Add(-3*time.Hour))
	insertFinancialFixture(t, ctx, db, "charges", paidChargeID, tenantID, map[string]any{
		"id": paidChargeID, "tenant_id": tenantID, "contract_id": uuid.NewString(),
		"type": "RENT", "description": "Aluguel legado", "status": "Pago",
		"total_amount": 50.00, "paid_amount": 50.00, "due_date": now.Add(-24 * time.Hour),
		"paid_at": now.Add(-2 * time.Hour), "updated_at": now.Add(-2 * time.Hour),
		"payments": []map[string]any{
			{"id": uuid.NewString(), "amount_cents": 5000, "method": "PIX", "paid_at": now.Add(-2 * time.Hour)},
		},
	}, now.Add(-48*time.Hour), now.Add(-2*time.Hour))
	insertFinancialFixture(t, ctx, db, "charges", reversedChargeID, tenantID, map[string]any{
		"id": reversedChargeID, "tenant_id": tenantID, "contract_id": uuid.NewString(),
		"type": "RENT", "description": "Aluguel estornado", "status": "Estornado",
		"total_amount": 30.00, "due_date": now.Add(-24 * time.Hour),
		"paid_at": now.Add(-4 * time.Hour), "updated_at": now.Add(-time.Hour),
	}, now.Add(-48*time.Hour), now.Add(-time.Hour))
	insertFinancialFixture(t, ctx, db, "charges", pendingChargeID, tenantID, map[string]any{
		"id": pendingChargeID, "tenant_id": tenantID, "contract_id": uuid.NewString(),
		"type": "OTHER", "description": "Pendente legado", "status": "Pendente",
		"total_amount": 20.00, "due_date": now.Add(24 * time.Hour), "updated_at": now,
	}, now, now)

	insertFinancialFixture(t, ctx, db, "transactions", originalRevenueID, tenantID, map[string]any{
		"id": originalRevenueID, "tenant_id": tenantID, "type": "receita", "description": "Receita revertida",
		"value": 10.00, "status": "Estornado", "date": now.Add(-5 * time.Hour),
		"payment_date": now.Add(-4 * time.Hour), "updated_at": now.Add(-2 * time.Hour),
	}, now.Add(-5*time.Hour), now.Add(-2*time.Hour))
	insertFinancialFixture(t, ctx, db, "transactions", reversalID, tenantID, map[string]any{
		"id": reversalID, "tenant_id": tenantID, "type": "estorno",
		"description": fmt.Sprintf("Estorno: teste de integração (ref: %s)", originalRevenueID),
		"value":       -10.00, "status": "Pago", "date": now.Add(-2 * time.Hour),
		"payment_date": now.Add(-2 * time.Hour), "updated_at": now.Add(-2 * time.Hour),
	}, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	insertFinancialFixture(t, ctx, db, "transactions", expenseID, tenantID, map[string]any{
		"id": expenseID, "tenant_id": tenantID, "type": "despesa", "description": "Despesa operacional",
		"category": "OPERACIONAL", "value": 5.00, "status": "Pago", "date": now.Add(-time.Hour),
		"payment_date": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour),
	}, now.Add(-time.Hour), now.Add(-time.Hour))
	insertFinancialFixture(t, ctx, db, "transactions", repasseID, tenantID, map[string]any{
		"id": repasseID, "tenant_id": tenantID, "type": "repasse", "description": "Repasse liquidado",
		"category": "REPASSE", "value": 3.00, "status": "PAID", "date": now.Add(-time.Hour),
		"payment_date": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour),
	}, now.Add(-time.Hour), now.Add(-time.Hour))
	insertFinancialFixture(t, ctx, db, "transactions", reversedRepasseID, tenantID, map[string]any{
		"id": reversedRepasseID, "tenant_id": tenantID, "type": "repasse", "description": "Repasse estornado",
		"category": "REPASSE", "value": 2.00, "status": "Estornado", "date": now.Add(-3 * time.Hour),
		"payment_date": now.Add(-3 * time.Hour), "updated_at": now.Add(-2 * time.Hour),
	}, now.Add(-3*time.Hour), now.Add(-2*time.Hour))
	insertFinancialFixture(t, ctx, db, "transactions", repasseReversalID, tenantID, map[string]any{
		"id": repasseReversalID, "tenant_id": tenantID, "type": "estorno",
		"description": fmt.Sprintf("Estorno: repasse (ref: %s)", reversedRepasseID),
		"value":       -2.00, "status": "PAID", "date": now.Add(-2 * time.Hour),
		"payment_date": now.Add(-2 * time.Hour), "updated_at": now.Add(-2 * time.Hour),
	}, now.Add(-2*time.Hour), now.Add(-2*time.Hour))
	// Legacy ledger rows persisted before broker_id/role existed remain
	// reconcilable by the aggregate allocation check. They must not emit a
	// per-beneficiary orphan that cannot be attributed safely.
	insertFinancialFixture(t, ctx, db, "transactions", legacyCommissionTxID, tenantID, map[string]any{
		"id": legacyCommissionTxID, "tenant_id": tenantID, "contract_id": "contract-legacy-commission",
		"type": "comissao", "description": "Comissão legada", "category": "COMISSAO",
		"value": 4.00, "status": "Pago", "plan_hash": "legacy-plan-hash",
		"payment_date": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour),
	}, now.Add(-time.Hour), now.Add(-time.Hour))
	insertFinancialFixture(t, ctx, db, "commissions", legacyCommissionID, tenantID, map[string]any{
		"id": legacyCommissionID, "tenant_id": tenantID, "contract_id": "contract-legacy-commission",
		"deal_id": "deal-legacy-commission", "broker_id": "broker-legacy", "role": "fechamento",
		"amount": 4.00, "status": "PENDING", "plan_hash": "legacy-plan-hash",
		"updated_at": now.Add(-time.Hour),
	}, now.Add(-time.Hour), now.Add(-time.Hour))
	insertFinancialFixture(t, ctx, db, "withdrawals", withdrawalID, tenantID, map[string]any{
		"id": withdrawalID, "tenant_id": tenantID, "amount_cents": 700, "status": "COMPLETE",
		"settled_at": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour),
	}, now.Add(-6*time.Hour), now.Add(-time.Hour))
	insertFinancialFixture(t, ctx, db, "transfers", transferID, tenantID, map[string]any{
		"id": transferID, "tenant_id": tenantID, "net_amount": 9.00, "status": "Processed",
		"processed_at": now.Add(-time.Hour), "updated_at": now.Add(-time.Hour),
	}, now.Add(-6*time.Hour), now.Add(-time.Hour))

	reportRepo := NewFinancialReportPostgresRepository(db)
	charges, err := reportRepo.ChargeTotals(ctx, tenantID, nil, nil)
	if err != nil {
		t.Fatalf("charge totals: %v", err)
	}
	assertStatusBucket(t, charges, "PARTIALLY_PAID", 1, 6000)
	assertStatusBucket(t, charges, "PAID", 1, 5000)
	assertStatusBucket(t, charges, "REVERSED", 1, 3000)
	assertStatusBucket(t, charges, "PENDING", 1, 2000)

	withdrawals, err := reportRepo.WithdrawalTotals(ctx, tenantID, nil, nil)
	if err != nil {
		t.Fatalf("withdrawal totals: %v", err)
	}
	assertStatusBucket(t, withdrawals, "COMPLETE", 1, 700)

	delinquency, err := reportRepo.DelinquencyTotals(ctx, tenantID)
	if err != nil {
		t.Fatalf("delinquency totals: %v", err)
	}
	if delinquency.OverdueCount != 1 || delinquency.OverdueTotalCents != 6000 {
		t.Fatalf("delinquency = %#v, want count=1 total=6000", delinquency)
	}

	revenue, err := reportRepo.RevenueByType(ctx, tenantID, from, to)
	if err != nil {
		t.Fatalf("revenue by type: %v", err)
	}
	if got := sumCategoryCents(revenue); got != 800 {
		t.Fatalf("realized revenue cents = %d, want 800 (only allocated revenue; unclassified receipts excluded)", got)
	}
	unclassified, err := reportRepo.UnclassifiedReceiptTotal(ctx, tenantID, from, to)
	if err != nil || unclassified != 5000 {
		t.Fatalf("unclassified receipts = %d, want 5000, err=%v", unclassified, err)
	}
	expense, err := reportRepo.ExpenseByCategory(ctx, tenantID, from, to)
	if err != nil {
		t.Fatalf("expense by category: %v", err)
	}
	if got := sumCategoryCents(expense); got != 900 {
		t.Fatalf("realized expense cents = %d, want 900 (agency expense + commission, excluding third-party repasse)", got)
	}

	cashflow, err := reportRepo.CashFlowByDay(ctx, tenantID, from, to)
	if err != nil {
		t.Fatalf("cash flow: %v", err)
	}
	var inflow, outflow int64
	for _, entry := range cashflow {
		inflow += entry.InflowCents
		outflow += entry.OutflowCents
	}
	if inflow != 9000 || outflow != 2800 {
		t.Fatalf("cash flow inflow/outflow = %d/%d, want 9000/2800", inflow, outflow)
	}

	kpis, err := reportRepo.KpisSnapshot(ctx, tenantID)
	if err != nil {
		t.Fatalf("KPIs: %v", err)
	}
	if kpis.RevenueMonthCents != 800 || kpis.RevenueLast30Cents != 800 || kpis.AverageTicketCents != 266 ||
		kpis.DelinquencyPct != 40 || kpis.ReceivableCents != 8000 || kpis.OverdueCents != 6000 ||
		kpis.OverdueCount != 1 || kpis.CommissionPaidMonth != 400 {
		t.Fatalf("KPIs = %#v", kpis)
	}

	reconciliation, err := NewReconciliationPostgresRepository(db).Run(ctx, tenantID)
	if err != nil {
		t.Fatalf("reconciliation: %v", err)
	}
	if reconciliation.Summary.TotalFindings != 0 || len(reconciliation.Findings) != 0 {
		t.Fatalf("valid fixture, including ambiguous legacy commission link, produced findings: %#v", reconciliation)
	}

	// source_type is optional in old payout rows. NULL must not bypass the
	// broker-without-commission orphan check (SQL `!=` would evaluate UNKNOWN).
	insertFinancialFixture(t, ctx, db, "payout_orders", orphanPayoutID, tenantID, map[string]any{
		"id": orphanPayoutID, "tenant_id": tenantID, "recipient_type": "broker",
		"recipient_id": "broker-without-commission", "amount_cents": 125,
		"status": "PENDING_REVIEW", "updated_at": now,
	}, now, now)
	reconciliation, err = NewReconciliationPostgresRepository(db).Run(ctx, tenantID)
	if err != nil {
		t.Fatalf("reconciliation with legacy payout: %v", err)
	}
	if !hasFinding(reconciliation.Findings, "orphan", "PayoutOrder", orphanPayoutID) {
		t.Fatalf("NULL source_type payout orphan was not reported: %#v", reconciliation)
	}
	if reconciliation.Summary.TotalFindings != 1 || reconciliation.Summary.Errors != 1 || reconciliation.Summary.Orphans != 1 {
		t.Fatalf("payout orphan summary does not reflect its finding: %#v", reconciliation.Summary)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("fixture cleanup: %v", err)
	}
	assertFixtureCleanup(t, ctx, db, tenantID)
}

func TestFinancialReportingPostgres_ManualSaleUsesEconomicOwnership(t *testing.T) {
	databaseURI := os.Getenv("TOCA_TEST_DATABASE_URI")
	if databaseURI == "" {
		t.Skip("TOCA_TEST_DATABASE_URI is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI: databaseURI, MaxPoolSize: 2, MinPoolSize: 0,
	})
	if err != nil {
		t.Fatalf("connect test postgres: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	tenantID := uuid.NewString()
	contractID := uuid.NewString()
	operationID := uuid.NewString()
	receiptID := uuid.NewString()
	commissionOneID := uuid.NewString()
	commissionTwoID := uuid.NewString()
	now := time.Now().UTC().Truncate(time.Microsecond)
	from := now.Add(-time.Hour)
	to := now.Add(time.Hour)

	cleanupDone := false
	cleanup := func() error {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_, cleanupErr := db.Exec(cleanupCtx, `
			DELETE FROM financial.transactions
			WHERE tenant_id::text = $1::text AND id IN ($2, $3, $4)
		`, tenantID, receiptID, commissionOneID, commissionTwoID)
		if cleanupErr == nil {
			cleanupDone = true
		}
		return cleanupErr
	}
	t.Cleanup(func() {
		if !cleanupDone {
			if cleanupErr := cleanup(); cleanupErr != nil {
				t.Errorf("fixture cleanup: %v", cleanupErr)
			}
		}
	})

	insertFinancialFixture(t, ctx, db, "transactions", receiptID, tenantID, map[string]any{
		"id": receiptID, "tenant_id": tenantID, "contract_id": contractID,
		"operation_id": operationID, "type": "receita", "description": "Recebimento - Venda",
		"category": "RECEITA_MANUAL", "value": 980_000.00, "status": "Pago",
		"payment_date": now,
		"economic_breakdown": map[string]any{
			"flow": "sale", "gross_cents": 98_000_000,
			"commission_pool_cents": 5_880_000, "commission_payable_cents": 3_528_000,
			"agency_retained_cents": 2_352_000, "owner_payable_cents": 92_120_000,
		},
	}, now, now)
	insertFinancialFixture(t, ctx, db, "transactions", commissionOneID, tenantID, map[string]any{
		"id": commissionOneID, "tenant_id": tenantID, "contract_id": contractID,
		"operation_id": operationID, "type": "comissao", "description": "Comissão - captador",
		"category": "COMISSAO", "value": 11_760.00, "status": "Pago", "payment_date": now,
	}, now, now)
	insertFinancialFixture(t, ctx, db, "transactions", commissionTwoID, tenantID, map[string]any{
		"id": commissionTwoID, "tenant_id": tenantID, "contract_id": contractID,
		"operation_id": operationID, "type": "comissao", "description": "Comissão - fechamento",
		"category": "COMISSAO", "value": 23_520.00, "status": "Pago", "payment_date": now,
	}, now, now)

	repo := NewFinancialReportPostgresRepository(db)
	revenue, err := repo.RevenueByType(ctx, tenantID, from, to)
	if err != nil {
		t.Fatalf("sale revenue: %v", err)
	}
	if got := sumCategoryCents(revenue); got != 2_352_000 {
		t.Fatalf("sale own revenue cents = %d, want 2352000", got)
	}
	if len(revenue) != 1 || revenue[0].Category != "SALE_AGENCY_RETAINED" {
		t.Fatalf("sale revenue category = %#v", revenue)
	}

	expense, err := repo.ExpenseByCategory(ctx, tenantID, from, to)
	if err != nil {
		t.Fatalf("sale expenses: %v", err)
	}
	if got := sumCategoryCents(expense); got != 0 {
		t.Fatalf("distributed broker commissions were counted twice: %d", got)
	}

	cashflow, err := repo.CashFlowByDay(ctx, tenantID, from, to)
	if err != nil {
		t.Fatalf("sale cash flow: %v", err)
	}
	var inflow, outflow int64
	for _, entry := range cashflow {
		inflow += entry.InflowCents
		outflow += entry.OutflowCents
	}
	if inflow != 98_000_000 || outflow != 3_528_000 {
		t.Fatalf("sale gross cash inflow/outflow = %d/%d, want 98000000/3528000", inflow, outflow)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("fixture cleanup: %v", err)
	}
	assertFixtureCleanup(t, ctx, db, tenantID)
}

func insertFinancialFixture(t *testing.T, ctx context.Context, db *database.PostgresDB, table, id, tenantID string, metadata map[string]any, createdAt, updatedAt time.Time) {
	t.Helper()
	allowed := map[string]bool{
		"charges": true, "transactions": true, "withdrawals": true, "transfers": true,
		"commissions": true, "payout_orders": true,
	}
	if !allowed[table] {
		t.Fatalf("fixture table is not allowed: %s", table)
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal %s fixture: %v", table, err)
	}
	query := fmt.Sprintf(`
		INSERT INTO financial.%s (id, tenant_id, metadata, created_at, updated_at)
		VALUES ($1, $2, $3::jsonb, $4, $5)
	`, table)
	if _, err := db.Exec(ctx, query, id, tenantID, string(raw), createdAt, updatedAt); err != nil {
		t.Fatalf("insert %s fixture %s: %v", table, id, err)
	}
}

func assertStatusBucket(t *testing.T, buckets []report.StatusBucket, status string, count int, cents int64) {
	t.Helper()
	for _, bucket := range buckets {
		if bucket.Status == status {
			if bucket.Count != count || bucket.TotalCents != cents {
				t.Fatalf("bucket %s = %#v, want count=%d cents=%d", status, bucket, count, cents)
			}
			return
		}
	}
	t.Fatalf("bucket %s not found in %#v", status, buckets)
}

func sumCategoryCents(buckets []report.CategoryBucket) int64 {
	var total int64
	for _, bucket := range buckets {
		total += bucket.TotalCents
	}
	return total
}

func hasFinding(findings []report.ReconciliationFinding, category, entityType, entityID string) bool {
	for _, finding := range findings {
		if finding.Category == category && finding.EntityType == entityType && finding.EntityID == entityID {
			return true
		}
	}
	return false
}

func assertFixtureCleanup(t *testing.T, ctx context.Context, db *database.PostgresDB, tenantID string) {
	t.Helper()
	var remaining int
	err := db.QueryRow(ctx, `
		SELECT
		  (SELECT COUNT(*) FROM financial.charges WHERE tenant_id::text = $1::text) +
		  (SELECT COUNT(*) FROM financial.transactions WHERE tenant_id::text = $1::text) +
		  (SELECT COUNT(*) FROM financial.commissions WHERE tenant_id::text = $1::text) +
		  (SELECT COUNT(*) FROM financial.payout_orders WHERE tenant_id::text = $1::text) +
		  (SELECT COUNT(*) FROM financial.withdrawals WHERE tenant_id::text = $1::text) +
		  (SELECT COUNT(*) FROM financial.transfers WHERE tenant_id::text = $1::text)
	`, tenantID).Scan(&remaining)
	if err != nil {
		t.Fatalf("verify fixture cleanup: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("fixture cleanup left %d rows for tenant %s", remaining, tenantID)
	}
}
