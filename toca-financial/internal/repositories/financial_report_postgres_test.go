package repositories

import (
	"math"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

func TestCanonicalFinancialStatusSQLContainsCurrentAndLegacyMappings(t *testing.T) {
	t.Parallel()
	expr := canonicalFinancialStatusSQL("entry")
	for _, want := range []string{
		"entry.metadata->>'status'",
		"WHEN 'PAGO' THEN 'PAID'",
		"WHEN 'PARCIALMENTE PAGO' THEN 'PARTIALLY_PAID'",
		"WHEN 'ESTORNADO' THEN 'REVERSED'",
		"WHEN 'COMPLETED' THEN 'COMPLETE'",
	} {
		if !strings.Contains(expr, want) {
			t.Fatalf("canonical status SQL missing %q:\n%s", want, expr)
		}
	}
}

func TestChargePaidAmountUsesAppendOnlyLogBeforeLegacyAggregate(t *testing.T) {
	t.Parallel()
	expr := chargePaidAmountCentsSQL("c")
	if !strings.Contains(expr, "jsonb_array_length") || !strings.Contains(expr, "jsonb_array_elements") {
		t.Fatalf("payment log is not authoritative:\n%s", expr)
	}
	if !strings.Contains(expr, "paid_amount") {
		t.Fatalf("legacy paid_amount fallback missing:\n%s", expr)
	}
}

func TestChargeReceiptSQLEmitsCompensatingReversal(t *testing.T) {
	t.Parallel()
	sql := chargeReceiptLateralSQL("c")
	for _, want := range []string{"SELECT -", "= 'REVERSED'", "c.updated_at", "receipt"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("charge receipt SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestPaymentRevenueCentsRequiresRecordedAllocation(t *testing.T) {
	t.Parallel()
	expr := paymentRevenueCentsSQL("payment")
	for _, want := range []string{
		"jsonb_typeof(payment->'allocations') = 'array'",
		"jsonb_array_length",
		"allocation->>'allocation_type'",
		"= 'revenue'",
		"ELSE 0::bigint",
	} {
		if !strings.Contains(expr, want) {
			t.Fatalf("payment revenue SQL missing %q:\n%s", want, expr)
		}
	}
}

func TestChargeRevenueSQLHasAllocationAwareReversalWithoutGrossFallback(t *testing.T) {
	t.Parallel()
	sql := chargeRevenueLateralSQL("c")
	for _, want := range []string{
		"payment_revenue.cents > 0",
		"allocation->>'allocation_type'",
		"ELSE 0::bigint",
		"SELECT -",
		"= 'REVERSED'",
		"revenue_receipt",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("charge revenue SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestChargeReceiptSQLRemainsGrossCashReceipt(t *testing.T) {
	t.Parallel()
	sql := chargeReceiptLateralSQL("c")
	if strings.Contains(sql, "allocations") {
		t.Fatalf("cash receipt SQL must not depend on DRE allocations:\n%s", sql)
	}
	if !strings.Contains(sql, "payment->>'amount_cents'") {
		t.Fatalf("cash receipt SQL no longer uses the gross payment amount:\n%s", sql)
	}
}

func TestManualDistributedReceiptUsesAgencyRetentionForOwnRevenue(t *testing.T) {
	t.Parallel()
	expr := transactionOwnRevenueCentsSQL("sale_tx")
	for _, want := range []string{
		"sale_tx.metadata->'economic_breakdown'",
		"agency_retained_cents",
		"IN ('sale', 'rent')",
		"sale_tx.metadata->>'value'",
	} {
		if !strings.Contains(expr, want) {
			t.Fatalf("manual own-revenue SQL missing %q:\n%s", want, expr)
		}
	}
}

func TestManualDistributedReceiptGetsEconomicRevenueCategory(t *testing.T) {
	t.Parallel()
	expr := transactionOwnRevenueCategorySQL("sale_tx")
	for _, want := range []string{"SALE_AGENCY_RETAINED", "RENT_AGENCY_RETAINED", "RECEITA MANUAL"} {
		if !strings.Contains(expr, want) {
			t.Fatalf("manual own-revenue category SQL missing %q:\n%s", want, expr)
		}
	}
}

func TestLegacyNetCommissionRequiresSameFrozenOperation(t *testing.T) {
	t.Parallel()
	expr := legacyNetCommissionPredicate("commission_tx")
	for _, want := range []string{
		"economic_receipt.tenant_id = commission_tx.tenant_id",
		"economic_receipt.metadata->>'operation_id' = commission_tx.metadata->>'operation_id'",
		"economic_breakdown",
		"IN ('sale', 'rent')",
	} {
		if !strings.Contains(expr, want) {
			t.Fatalf("third-party distribution SQL missing %q:\n%s", want, expr)
		}
	}
}

func TestOwnRevenueEventsKeepCashAndResultBoundariesSeparate(t *testing.T) {
	t.Parallel()
	sql := ownRevenueEventsSQL()
	for _, want := range []string{
		"revenue_receipt.cents",
		"agency_retained_cents",
		"manual_tx",
		"reversal_tx",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("own-revenue event SQL missing %q:\n%s", want, sql)
		}
	}
	if strings.Contains(sql, "chargeReceiptLateralSQL") {
		t.Fatalf("own-revenue stream must embed revenue events, not gross receipt helper names:\n%s", sql)
	}
}

func TestJournalPositionSQLUsesAppendOnlyAccountBalances(t *testing.T) {
	t.Parallel()
	sql := journalPositionSQL()
	for _, want := range []string{
		"financial.journal_entries",
		"jsonb_array_elements",
		"owner_payable",
		"commission_payable",
		"partner_payable",
		"royalty_payable",
		"tax_withheld",
		"withdrawal_payable",
		"journal_entry.tenant_id = $1",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("journal position SQL missing %q:\n%s", want, sql)
		}
	}
}

func TestApplyFinancialPositionSeparatesCommittedAndAgencyCash(t *testing.T) {
	t.Parallel()
	var out report.KPIs
	err := applyFinancialPosition(&out, financialPositionBalances{
		cash:                    2_367_000,
		taxWithheld:             10_000,
		otherPayable:            5_000,
		unexpectedPayableDebits: 300,
	})
	if err != nil {
		t.Fatalf("apply financial position: %v", err)
	}
	if out.ThirdPartyObligationsCents != 0 || out.OtherCommitmentsCents != 15_000 ||
		out.CommittedFundsCents != 15_000 || out.AgencyAvailableCashCents != 2_352_000 ||
		out.CoverageGapCents != 0 || out.UnexpectedPayableDebitCents != 300 {
		t.Fatalf("financial position = %#v", out)
	}
}

func TestApplyFinancialPositionSurfacesCoverageGap(t *testing.T) {
	t.Parallel()
	var out report.KPIs
	err := applyFinancialPosition(&out, financialPositionBalances{
		cash:              10_000,
		ownerPayable:      12_000,
		commissionPayable: 3_000,
		taxWithheld:       2_500,
	})
	if err != nil {
		t.Fatalf("apply financial position: %v", err)
	}
	if out.ThirdPartyObligationsCents != 15_000 || out.CommittedFundsCents != 17_500 ||
		out.AgencyAvailableCashCents != 0 || out.CoverageGapCents != 7_500 {
		t.Fatalf("financial position = %#v", out)
	}
}

func TestApplyFinancialPositionIncludesNegativeCashInCoverageGap(t *testing.T) {
	t.Parallel()
	var out report.KPIs
	if err := applyFinancialPosition(&out, financialPositionBalances{cash: -10_000, ownerPayable: 12_000}); err != nil {
		t.Fatalf("apply financial position: %v", err)
	}
	if out.AgencyAvailableCashCents != 0 || out.CoverageGapCents != 22_000 {
		t.Fatalf("financial position = %#v", out)
	}
}

func TestApplyFinancialPositionRejectsOverflow(t *testing.T) {
	t.Parallel()
	var out report.KPIs
	err := applyFinancialPosition(&out, financialPositionBalances{
		ownerPayable:      math.MaxInt64,
		commissionPayable: 1,
	})
	if err == nil || !strings.Contains(err.Error(), "overflows") {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestTransactionReversalOriginalIDSQLUsesStableReferenceSuffix(t *testing.T) {
	t.Parallel()
	expr := transactionReversalOriginalIDSQL("tx")
	if !strings.Contains(expr, "tx.metadata->>'description'") || !strings.Contains(expr, `\(ref: ([^)]+)\)$`) {
		t.Fatalf("reversal reference SQL is not anchored to the stable suffix:\n%s", expr)
	}
}

// O adiantamento de manutenção só existe no razão (não tem cobrança, repasse
// ou transação): entra no fluxo de caixa pelo marcador explícito da entrada,
// que também impede a duplicidade com as fontes operacionais.
func TestCashFlowReadsLedgerOnlyMovementsByExplicitMarker(t *testing.T) {
	t.Parallel()
	query := cashFlowByDaySQL()
	for _, want := range []string{
		"NULLIF(entry.metadata->>'cash_flow_source', '') IS NOT NULL",
		"leg->>'account' = 'cash'",
		"leg->>'direction' = 'debit'",
		"leg->>'direction' = 'credit'",
	} {
		if !strings.Contains(query, want) {
			t.Fatalf("fluxo de caixa não lê movimentos do razão (%q ausente)", want)
		}
	}
	// As fontes operacionais continuam presentes: o razão só soma o que não
	// tem documento próprio, senão cobranças e repasses entrariam duas vezes.
	for _, source := range []string{"inflows", "outflows_transfers", "outflows_withdrawals", "inflows_manual"} {
		if !strings.Contains(query, source) {
			t.Fatalf("fonte operacional %q sumiu do fluxo de caixa", source)
		}
	}
}

// A nota de crédito é saída de caixa quando liquidada e nunca receita própria.
func TestCashFlowCountsCreditNotesAsOutflow(t *testing.T) {
	t.Parallel()
	query := cashFlowByDaySQL()
	if !strings.Contains(query, "outflows_credit_notes") {
		t.Fatal("nota de crédito precisa aparecer como saída de caixa")
	}
	if !strings.Contains(query, "'estorno'") {
		t.Fatal("nota de crédito é estorno sem transação original")
	}
	// Estorno com transação original já tem caminho próprio (outflow_reversals):
	// aqui só entram os avulsos, reconhecidos pela ausência do sufixo "(ref: )".
	if !strings.Contains(query, "credit_tx.metadata->>'description'") {
		t.Fatal("só estornos avulsos entram como nota de crédito")
	}
}

func TestJournalEntryDateFallsBackToCreatedAt(t *testing.T) {
	t.Parallel()
	expr := journalEntryDateSQL("entry")
	if !strings.Contains(expr, "entry.metadata->>'date'") || !strings.Contains(expr, "entry.created_at") {
		t.Fatalf("data do lançamento sem fallback: %s", expr)
	}
}
