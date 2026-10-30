package repositories

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

type financialReportPostgresRepository struct {
	db *database.PostgresDB
}

func NewFinancialReportPostgresRepository(db *database.PostgresDB) *financialReportPostgresRepository {
	return &financialReportPostgresRepository{db: db}
}

// canonicalFinancialStatusSQL is the compatibility boundary for persisted
// lifecycle values. Current writers use upper-case English values, while old
// rows and still-supported clients may contain title-case English or
// Portuguese values. Reports must group those rows together instead of
// silently omitting them.
func canonicalFinancialStatusSQL(alias string) string {
	raw := fmt.Sprintf("UPPER(BTRIM(COALESCE(%s.metadata->>'status', '')))", alias)
	return `CASE ` + raw + `
		WHEN 'PAGO' THEN 'PAID'
		WHEN 'PAGA' THEN 'PAID'
		WHEN 'PENDENTE' THEN 'PENDING'
		WHEN 'EM ATRASO' THEN 'OVERDUE'
		WHEN 'PARCIALMENTE PAGO' THEN 'PARTIALLY_PAID'
		WHEN 'PARCIALMENTE PAGA' THEN 'PARTIALLY_PAID'
		WHEN 'ESTORNADO' THEN 'REVERSED'
		WHEN 'ESTORNADA' THEN 'REVERSED'
		WHEN 'CANCELADO' THEN 'CANCELLED'
		WHEN 'CANCELADA' THEN 'CANCELLED'
		WHEN 'COMPLETED' THEN 'COMPLETE'
		WHEN 'DONE' THEN 'COMPLETE'
		WHEN 'CONCLUIDO' THEN 'COMPLETE'
		WHEN 'CONCLUÍDO' THEN 'COMPLETE'
		WHEN 'CONCLUIDA' THEN 'COMPLETE'
		WHEN 'CONCLUÍDA' THEN 'COMPLETE'
		WHEN 'PROCESSADO' THEN 'PROCESSED'
		WHEN 'PROCESSADA' THEN 'PROCESSED'
		ELSE ` + raw + ` END`
}

func canonicalTransactionTypeSQL(alias string) string {
	return fmt.Sprintf("LOWER(BTRIM(COALESCE(%s.metadata->>'type', '')))", alias)
}

func chargePaymentsArraySQL(alias string) string {
	return fmt.Sprintf(`CASE WHEN jsonb_typeof(%[1]s.metadata->'payments') = 'array'
		THEN %[1]s.metadata->'payments' ELSE '[]'::jsonb END`, alias)
}

// Charge amounts are persisted as reais, while PaymentEntry amounts are
// already cents. ABS is intentional: entity type controls economic direction;
// a malformed negative amount is surfaced by reconciliation and cannot invert
// a report silently.
func chargeAmountCentsSQL(alias string) string {
	return fmt.Sprintf(`ABS(ROUND(COALESCE(NULLIF(%s.metadata->>'total_amount', '')::numeric, 0) * 100))::bigint`, alias)
}

func chargeLegacyPaidAmountCentsSQL(alias string) string {
	return fmt.Sprintf(`ROUND(COALESCE(NULLIF(%s.metadata->>'paid_amount', '')::numeric, 0) * 100)::bigint`, alias)
}

func chargePaymentsSumCentsSQL(alias string) string {
	payments := chargePaymentsArraySQL(alias)
	return `(SELECT COALESCE(SUM(COALESCE(NULLIF(payment->>'amount_cents', '')::bigint, 0)), 0)::bigint
		FROM jsonb_array_elements(` + payments + `) AS payment)`
}

// The append-only payment log is authoritative when present. paid_amount is a
// compatibility fallback for rows created before PaymentEntry existed. The
// oldest fully-settled rows sometimes persisted only status + total_amount;
// for PAID/REVERSED only, total_amount is therefore the final legacy fallback.
func chargePaidAmountCentsSQL(alias string) string {
	payments := chargePaymentsArraySQL(alias)
	return `CASE WHEN jsonb_array_length(` + payments + `) > 0
		THEN ABS(` + chargePaymentsSumCentsSQL(alias) + `)
		WHEN NULLIF(` + alias + `.metadata->>'paid_amount', '') IS NOT NULL
		THEN ABS(` + chargeLegacyPaidAmountCentsSQL(alias) + `)
		WHEN ` + canonicalFinancialStatusSQL(alias) + ` IN ('PAID', 'REVERSED')
		THEN ` + chargeAmountCentsSQL(alias) + `
		ELSE 0::bigint END`
}

func chargeOutstandingCentsSQL(alias string) string {
	return `GREATEST(` + chargeAmountCentsSQL(alias) + ` - ` + chargePaidAmountCentsSQL(alias) + `, 0)`
}

func chargeBucketCentsSQL(alias string) string {
	return `CASE WHEN ` + canonicalFinancialStatusSQL(alias) + ` = 'PARTIALLY_PAID'
		THEN ` + chargeOutstandingCentsSQL(alias) + ` ELSE ` + chargeAmountCentsSQL(alias) + ` END`
}

// chargeReceiptLateralSQL emits one cash event per receipt. A reversed charge
// keeps its original receipt dates and emits a compensating negative event on
// the reversal timestamp, so historical periods are not rewritten and a range
// containing both sides nets to zero.
func chargeReceiptLateralSQL(alias string) string {
	payments := chargePaymentsArraySQL(alias)
	return `
		CROSS JOIN LATERAL (
			SELECT ABS(COALESCE(NULLIF(payment->>'amount_cents', '')::bigint, 0)) AS cents,
			       NULLIF(payment->>'paid_at', '')::timestamptz AS paid_at
			FROM jsonb_array_elements(` + payments + `) AS payment
			WHERE ABS(COALESCE(NULLIF(payment->>'amount_cents', '')::bigint, 0)) > 0
			UNION ALL
			SELECT ` + chargePaidAmountCentsSQL(alias) + ` AS cents,
			       NULLIF(` + alias + `.metadata->>'paid_at', '')::timestamptz AS paid_at
			WHERE jsonb_array_length(` + payments + `) = 0
			  AND ` + chargePaidAmountCentsSQL(alias) + ` > 0
			UNION ALL
			SELECT -` + chargePaidAmountCentsSQL(alias) + ` AS cents,
			       COALESCE(NULLIF(` + alias + `.metadata->>'updated_at', '')::timestamptz,
			                ` + alias + `.updated_at, ` + alias + `.created_at) AS paid_at
			WHERE ` + canonicalFinancialStatusSQL(alias) + ` = 'REVERSED'
			  AND ` + chargePaidAmountCentsSQL(alias) + ` > 0
		) AS receipt`
}

func paymentAllocationsArraySQL(alias string) string {
	return fmt.Sprintf(`CASE WHEN jsonb_typeof(%[1]s->'allocations') = 'array'
		THEN %[1]s->'allocations' ELSE '[]'::jsonb END`, alias)
}

// paymentRevenueCentsSQL is the DRE boundary between cash received and own
// revenue. Once a payment has explicit allocations, only slices classified as
// revenue belong in the result. Missing allocations remain explicitly
// unclassified in the report; gross receipt amounts are not agency revenue.
func paymentRevenueCentsSQL(alias string) string {
	allocations := paymentAllocationsArraySQL(alias)
	return `CASE WHEN jsonb_array_length(` + allocations + `) > 0
		THEN (SELECT COALESCE(SUM(ABS(COALESCE(NULLIF(allocation->>'amount_cents', '')::bigint, 0))), 0)::bigint
		      FROM jsonb_array_elements(` + allocations + `) AS allocation
		      WHERE LOWER(BTRIM(COALESCE(allocation->>'allocation_type', ''))) = 'revenue')
		ELSE 0::bigint END`
}

// chargeRevenuePaidAmountCentsSQL returns the revenue portion accumulated by a
// charge. It is used to emit an allocation-aware counter-entry on reversal.
func chargeRevenuePaidAmountCentsSQL(alias string) string {
	payments := chargePaymentsArraySQL(alias)
	return `CASE WHEN jsonb_array_length(` + payments + `) > 0
		THEN (SELECT COALESCE(SUM(` + paymentRevenueCentsSQL("revenue_payment") + `), 0)::bigint
		      FROM jsonb_array_elements(` + payments + `) AS revenue_payment)
		ELSE 0::bigint END`
}

// chargeRevenueLateralSQL emits DRE revenue events. Allocation-aware payments
// contribute only their own-revenue slices; unallocated receipts are reported
// separately as unclassified. Cash flow intentionally continues to use
// chargeReceiptLateralSQL and therefore remains a gross-receipt view.
func chargeRevenueLateralSQL(alias string) string {
	payments := chargePaymentsArraySQL(alias)
	return `
		CROSS JOIN LATERAL (
			SELECT payment_revenue.cents,
			       payment_revenue.paid_at
			FROM (
				SELECT ` + paymentRevenueCentsSQL("payment") + ` AS cents,
				       NULLIF(payment->>'paid_at', '')::timestamptz AS paid_at
				FROM jsonb_array_elements(` + payments + `) AS payment
			) AS payment_revenue
			WHERE payment_revenue.cents > 0
			UNION ALL
			SELECT -` + chargeRevenuePaidAmountCentsSQL(alias) + ` AS cents,
			       COALESCE(NULLIF(` + alias + `.metadata->>'updated_at', '')::timestamptz,
			                ` + alias + `.updated_at, ` + alias + `.created_at) AS paid_at
			WHERE ` + canonicalFinancialStatusSQL(alias) + ` = 'REVERSED'
			  AND ` + chargeRevenuePaidAmountCentsSQL(alias) + ` > 0
		) AS revenue_receipt`
}

// A receivable Transaction may mirror a Charge through source_event_id.
// Charge receipts are the source of truth, so both the original transaction
// and its reversal must be excluded from the manual cash stream.
func unlinkedManualRevenuePredicate(alias string) string {
	return `NOT EXISTS (
		SELECT 1
		FROM financial.charges linked_charge
		WHERE linked_charge.tenant_id = ` + alias + `.tenant_id
		  AND linked_charge.deleted_at IS NULL
		  AND NULLIF(` + alias + `.metadata->>'source_event_id', '') IS NOT NULL
		  AND linked_charge.metadata->>'source_event_id' = ` + alias + `.metadata->>'source_event_id'
	)`
}

// dayBucketSQL converte um instante para o DIA CIVIL no fuso configurado
// ("$4" é o nome do fuso, vindo de timeutil.TimezoneName()).
//
// O banco roda em UTC: sem o AT TIME ZONE, uma baixa às 22h em Brasília caía no
// dia seguinte (e, na virada do mês, no mês seguinte) — medido no beta em
// 01/10/2026 00:55 UTC, quando o dia civil local ainda era 30/09. O nome do fuso
// vem por parâmetro para o SQL não depender da configuração do servidor.
func dayBucketSQL(timestampExpr string) string {
	return "DATE_TRUNC('day', (" + timestampExpr + ") AT TIME ZONE $4)"
}

// cashFlowByDaySQL uniformiza as fontes de caixa em (day, inflow, outflow):
// recebimentos de cobranças, repasses, saques, transações avulsas, notas de
// crédito e — pelo marcador explícito — os movimentos que só existem no razão.
// journalEntryDateSQL usa a data econômica do lançamento, com a data de
// criação como fallback para entradas legadas.
func journalEntryDateSQL(alias string) string {
	return `COALESCE(NULLIF(` + alias + `.metadata->>'date', '')::timestamptz, ` + alias + `.created_at)`
}

func transactionCashDateSQL(alias string) string {
	return `COALESCE(NULLIF(` + alias + `.metadata->>'payment_date', '')::timestamptz, ` + alias + `.created_at)`
}

func transactionEconomicBreakdownSQL(alias string) string {
	return `CASE WHEN jsonb_typeof(` + alias + `.metadata->'economic_breakdown') = 'object'
		THEN ` + alias + `.metadata->'economic_breakdown' ELSE '{}'::jsonb END`
}

// UnclassifiedReceiptTotal exposes collected cash whose economic allocation
// is unknown. It remains outside own revenue instead of silently recognizing
// rents, deposits or seller funds as agency income.
func (r *financialReportPostgresRepository) UnclassifiedReceiptTotal(ctx context.Context, tenantID string, from, to time.Time) (int64, error) {
	payments := chargePaymentsArraySQL("c")
	unknownTotal := `CASE WHEN jsonb_array_length(` + payments + `) = 0
		THEN ` + chargePaidAmountCentsSQL("c") + ` ELSE
		(SELECT COALESCE(SUM(ABS((p->>'amount_cents')::bigint)), 0)
		 FROM jsonb_array_elements(` + payments + `) p
		 WHERE jsonb_array_length(` + paymentAllocationsArraySQL("p") + `) = 0) END`
	query := `SELECT COALESCE(SUM(unknown_receipt.cents), 0)::bigint
		FROM financial.charges c CROSS JOIN LATERAL (
		 SELECT ABS((p->>'amount_cents')::bigint) AS cents, (p->>'paid_at')::timestamptz AS paid_at
		 FROM jsonb_array_elements(` + payments + `) p
		 WHERE jsonb_array_length(` + paymentAllocationsArraySQL("p") + `) = 0
		 UNION ALL
		 SELECT ` + chargePaidAmountCentsSQL("c") + `, NULLIF(c.metadata->>'paid_at', '')::timestamptz
		 WHERE jsonb_array_length(` + payments + `) = 0
		 UNION ALL
		 SELECT -(` + unknownTotal + `), COALESCE(NULLIF(c.metadata->>'updated_at', '')::timestamptz, c.updated_at, c.created_at)
		 WHERE ` + canonicalFinancialStatusSQL("c") + ` = 'REVERSED'
		) unknown_receipt
		WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
		 AND ` + canonicalFinancialStatusSQL("c") + ` IN ('PAID', 'PARTIALLY_PAID', 'REVERSED')
		 AND unknown_receipt.paid_at BETWEEN $2 AND $3`
	var cents int64
	err := r.db.QueryRow(ctx, query, tenantID, from, to).Scan(&cents)
	return cents, err
}

// Receipt journals freeze whether a commission was recognized gross or net.
// Use only the original entry explicitly linked to this receipt, preserving
// historical net postings and keeping compensating reversals separate.
func manualReceiptJournalRevenueSQL(alias string) string {
	return `(SELECT SUM(CASE WHEN leg->>'direction' = 'credit' THEN 1 ELSE -1 END
		* COALESCE(NULLIF(leg->>'amount_cents', '')::bigint, 0))::bigint
		FROM financial.journal_entries receipt_journal
		CROSS JOIN LATERAL jsonb_array_elements(receipt_journal.metadata->'legs') leg
		WHERE receipt_journal.tenant_id = ` + alias + `.tenant_id
		  AND receipt_journal.deleted_at IS NULL
		  AND receipt_journal.metadata->>'operation_id' = ` + alias + `.id::text
		  AND NULLIF(receipt_journal.metadata->>'reversal_of', '') IS NULL
		  AND leg->>'account' = 'revenue')`
}

// transactionOwnRevenueCentsSQL keeps manual distributed receipts aligned
// with the same ownership boundary used by charge allocations. A sale receipt
// uses its posted revenue credit; third-party liabilities never enter DRE.
// Legacy rows retain their original net model when no journal proves otherwise.
func transactionOwnRevenueCentsSQL(alias string) string {
	breakdown := transactionEconomicBreakdownSQL(alias)
	return `COALESCE(` + manualReceiptJournalRevenueSQL(alias) + `, CASE WHEN LOWER(BTRIM(COALESCE(` + breakdown + `->>'flow', ''))) IN ('sale', 'rent')
		AND NULLIF(` + breakdown + `->>'agency_retained_cents', '') IS NOT NULL
		THEN GREATEST(COALESCE(NULLIF(` + breakdown + `->>'agency_retained_cents', '')::bigint, 0), 0)
		   + CASE WHEN LOWER(BTRIM(COALESCE(` + breakdown + `->>'flow', ''))) = 'rent'
		          THEN GREATEST(COALESCE(NULLIF(` + breakdown + `->>'admin_fee_cents', '')::bigint, 0), 0)
		          ELSE 0 END
		ELSE ABS(ROUND(COALESCE(NULLIF(` + alias + `.metadata->>'value', '')::numeric, 0) * 100))::bigint END)`
}

func transactionOwnRevenueCategorySQL(alias string) string {
	breakdown := transactionEconomicBreakdownSQL(alias)
	return `CASE LOWER(BTRIM(COALESCE(` + breakdown + `->>'flow', '')))
		WHEN 'sale' THEN CASE WHEN ` + manualReceiptJournalRevenueSQL(alias) + ` IS NOT NULL THEN 'SALE_INTERMEDIATION' ELSE 'SALE_AGENCY_RETAINED' END
		WHEN 'rent' THEN CASE WHEN ` + manualReceiptJournalRevenueSQL(alias) + ` IS NOT NULL THEN 'RENT_INTERMEDIATION' ELSE 'RENT_AGENCY_RETAINED' END
		ELSE COALESCE(NULLIF(` + alias + `.metadata->>'category', ''), 'RECEITA MANUAL') END`
}

// A distributed commission is excluded only when its receipt actually used a
// net model. Gross recognition must retain the broker expense. No description
// or coincidental amount links operations across tenants.
func legacyNetCommissionPredicate(alias string) string {
	breakdown := transactionEconomicBreakdownSQL("economic_receipt")
	return `EXISTS (
		SELECT 1 FROM financial.transactions economic_receipt
		WHERE economic_receipt.tenant_id = ` + alias + `.tenant_id
		  AND economic_receipt.deleted_at IS NULL
		  AND ` + canonicalTransactionTypeSQL("economic_receipt") + ` = 'receita'
		  AND NULLIF(` + alias + `.metadata->>'operation_id', '') IS NOT NULL
		  AND economic_receipt.metadata->>'operation_id' = ` + alias + `.metadata->>'operation_id'
		  AND LOWER(BTRIM(COALESCE(` + breakdown + `->>'flow', ''))) IN ('sale', 'rent')
		  AND (` + manualReceiptJournalRevenueSQL("economic_receipt") + ` IS NULL
		       OR ` + manualReceiptJournalRevenueSQL("economic_receipt") + ` <
		          COALESCE(NULLIF(` + breakdown + `->>'agency_retained_cents', '')::bigint, 0)
		          + COALESCE(NULLIF(` + breakdown + `->>'commission_payable_cents', '')::bigint, 0)
		          + COALESCE(NULLIF(` + breakdown + `->>'admin_fee_cents', '')::bigint, 0))
	)`
}

// ownRevenueEventsSQL is the common realized-revenue stream used by DRE and
// KPI snapshots. Charge payments use allocations; manual distributed receipts
// use EconomicBreakdown; legacy postings retain their historical gross value.
func ownRevenueEventsSQL() string {
	return `
		SELECT COALESCE(c.metadata->>'type', 'OTHER') AS type,
		       revenue_receipt.cents,
		       revenue_receipt.paid_at
		FROM financial.charges c
		` + chargeRevenueLateralSQL("c") + `
		WHERE c.tenant_id = $1
		  AND c.deleted_at IS NULL
		  AND ` + canonicalFinancialStatusSQL("c") + ` IN ('PAID', 'PARTIALLY_PAID', 'REVERSED')
		UNION ALL
		SELECT ` + transactionOwnRevenueCategorySQL("manual_tx") + ` AS type,
		       ` + transactionOwnRevenueCentsSQL("manual_tx") + ` AS cents,
		       ` + transactionCashDateSQL("manual_tx") + ` AS paid_at
		FROM financial.transactions manual_tx
		WHERE manual_tx.tenant_id = $1
		  AND manual_tx.deleted_at IS NULL
		  AND ` + canonicalTransactionTypeSQL("manual_tx") + ` = 'receita'
		  AND ` + canonicalFinancialStatusSQL("manual_tx") + ` IN ('PAID', 'REVERSED')
		  AND ` + unlinkedManualRevenuePredicate("manual_tx") + `
		  AND ` + transactionOwnRevenueCentsSQL("manual_tx") + ` > 0
		UNION ALL
		SELECT ` + transactionOwnRevenueCategorySQL("original_tx") + ` AS type,
		       -` + transactionOwnRevenueCentsSQL("original_tx") + ` AS cents,
		       ` + transactionCashDateSQL("reversal_tx") + ` AS paid_at
		FROM financial.transactions reversal_tx
		JOIN financial.transactions original_tx
		  ON original_tx.tenant_id = reversal_tx.tenant_id
		 AND original_tx.deleted_at IS NULL
		 AND original_tx.id::text = ` + transactionReversalOriginalIDSQL("reversal_tx") + `
		WHERE reversal_tx.tenant_id = $1
		  AND reversal_tx.deleted_at IS NULL
		  AND ` + canonicalTransactionTypeSQL("reversal_tx") + ` = 'estorno'
		  AND ` + canonicalFinancialStatusSQL("reversal_tx") + ` = 'PAID'
		  AND ` + canonicalTransactionTypeSQL("original_tx") + ` = 'receita'
		  AND ` + unlinkedManualRevenuePredicate("original_tx") + `
		  AND ` + transactionOwnRevenueCentsSQL("original_tx") + ` > 0`
}

func journalPositionSQL() string {
	return `
		SELECT
		  COALESCE(SUM(balance_cents) FILTER (WHERE account = 'cash'), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'owner_payable'), 0), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'commission_payable'), 0), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'partner_payable'), 0), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'royalty_payable'), 0), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'tax_withheld'), 0), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'other_payable'), 0), 0)::bigint,
		  GREATEST(-COALESCE(SUM(balance_cents) FILTER (WHERE account = 'withdrawal_payable'), 0), 0)::bigint,
		  COALESCE(SUM(GREATEST(balance_cents, 0)) FILTER (
		    WHERE account IN (
		      'owner_payable', 'commission_payable', 'partner_payable', 'royalty_payable',
		      'tax_withheld', 'other_payable', 'withdrawal_payable'
		    )
		  ), 0)::bigint
		FROM (
		  SELECT
		    leg->>'account' AS account,
		    SUM(CASE
		      WHEN leg->>'direction' = 'debit'
		        THEN COALESCE(NULLIF(leg->>'amount_cents', '')::bigint, 0)
		      WHEN leg->>'direction' = 'credit'
		        THEN -COALESCE(NULLIF(leg->>'amount_cents', '')::bigint, 0)
		      ELSE 0
		    END)::bigint AS balance_cents
		  FROM financial.journal_entries journal_entry
		  CROSS JOIN LATERAL jsonb_array_elements(
		    CASE WHEN jsonb_typeof(journal_entry.metadata->'legs') = 'array'
		      THEN journal_entry.metadata->'legs' ELSE '[]'::jsonb END
		  ) AS leg
		  WHERE journal_entry.tenant_id = $1
		    AND journal_entry.deleted_at IS NULL
		  GROUP BY leg->>'account'
		) account_balances`
}

type financialPositionBalances struct {
	cash                    int64
	ownerPayable            int64
	commissionPayable       int64
	partnerPayable          int64
	royaltyPayable          int64
	taxWithheld             int64
	otherPayable            int64
	withdrawalPayable       int64
	unexpectedPayableDebits int64
}

func checkedNonNegativeCentsSum(values ...int64) (int64, error) {
	var total int64
	for _, value := range values {
		if value < 0 {
			return 0, errors.New("financial position contains a negative obligation")
		}
		if total > math.MaxInt64-value {
			return 0, errors.New("financial position total overflows int64")
		}
		total += value
	}
	return total, nil
}

func applyFinancialPosition(out *report.KPIs, balances financialPositionBalances) error {
	if out == nil {
		return errors.New("financial position output is required")
	}
	thirdParty, err := checkedNonNegativeCentsSum(
		balances.ownerPayable,
		balances.commissionPayable,
		balances.partnerPayable,
		balances.royaltyPayable,
	)
	if err != nil {
		return err
	}
	otherCommitments, err := checkedNonNegativeCentsSum(
		balances.taxWithheld,
		balances.otherPayable,
		balances.withdrawalPayable,
	)
	if err != nil {
		return err
	}
	committed, err := checkedNonNegativeCentsSum(thirdParty, otherCommitments)
	if err != nil {
		return err
	}

	out.CashBalanceCents = balances.cash
	out.OwnerPayableCents = balances.ownerPayable
	out.CommissionPayableCents = balances.commissionPayable
	out.PartnerPayableCents = balances.partnerPayable
	out.RoyaltyPayableCents = balances.royaltyPayable
	out.TaxWithheldCents = balances.taxWithheld
	out.OtherPayableCents = balances.otherPayable
	out.WithdrawalPayableCents = balances.withdrawalPayable
	out.ThirdPartyObligationsCents = thirdParty
	out.OtherCommitmentsCents = otherCommitments
	out.CommittedFundsCents = committed
	out.UnexpectedPayableDebitCents = balances.unexpectedPayableDebits

	if balances.cash >= committed {
		out.AgencyAvailableCashCents = balances.cash - committed
		return nil
	}
	if balances.cash >= 0 {
		out.CoverageGapCents = committed - balances.cash
		return nil
	}
	if balances.cash == math.MinInt64 {
		return errors.New("financial position cash balance overflows coverage calculation")
	}
	gap, err := checkedNonNegativeCentsSum(committed, -balances.cash)
	if err != nil {
		return err
	}
	out.CoverageGapCents = gap
	return nil
}

// Transaction.Reverse stores the original posting id in the stable suffix
// "(ref: <id>)". Until a dedicated reversal_of_id is persisted, this is the
// backwards-compatible relationship used by reports and reconciliation.
func transactionReversalOriginalIDSQL(alias string) string {
	return `SUBSTRING(COALESCE(` + alias + `.metadata->>'description', '') FROM '\(ref: ([^)]+)\)$')`
}

// ChargeTotals aggregates receivables (financial.charges) by status. The period
// filter, when given, is applied to the charge due date.
func (r *financialReportPostgresRepository) ChargeTotals(ctx context.Context, tenantID string, from, to *time.Time) ([]report.StatusBucket, error) {
	where := []string{"c.tenant_id = $1", "c.deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2
	if from != nil {
		where = append(where, fmt.Sprintf("(c.metadata->>'due_date')::timestamptz >= $%d", argN))
		args = append(args, *from)
		argN++
	}
	if to != nil {
		where = append(where, fmt.Sprintf("(c.metadata->>'due_date')::timestamptz <= $%d", argN))
		args = append(args, *to)
	}
	query := `
		SELECT ` + canonicalFinancialStatusSQL("c") + `,
		       COUNT(1),
		       COALESCE(SUM(` + chargeBucketCentsSQL("c") + `), 0)::bigint
		FROM financial.charges c
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY 1 ORDER BY 1`
	return scanStatusBuckets(ctx, r.db, query, args)
}

// WithdrawalTotals aggregates financial.withdrawals by status, filtered by
// creation date.
func (r *financialReportPostgresRepository) WithdrawalTotals(ctx context.Context, tenantID string, from, to *time.Time) ([]report.StatusBucket, error) {
	where := []string{"w.tenant_id = $1", "w.deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2
	if from != nil {
		where = append(where, fmt.Sprintf("w.created_at >= $%d", argN))
		args = append(args, *from)
		argN++
	}
	if to != nil {
		where = append(where, fmt.Sprintf("w.created_at <= $%d", argN))
		args = append(args, *to)
	}
	query := `
		SELECT ` + canonicalFinancialStatusSQL("w") + `,
		       COUNT(1),
		       COALESCE(SUM(ABS(COALESCE(NULLIF(w.metadata->>'amount_cents', '')::bigint, 0))), 0)::bigint
		FROM financial.withdrawals w
		WHERE ` + strings.Join(where, " AND ") + `
		GROUP BY 1 ORDER BY 1`
	return scanStatusBuckets(ctx, r.db, query, args)
}

// DelinquencyTotals summarises overdue receivables as of now (status pending /
// overdue / partially-paid with a past due date).
// chargeOverduePredicateSQL devolve o predicado de "cobrança vencida" na regra do
// produto: vence NO DIA e só conta atraso no dia seguinte, comparando datas civis
// (o vencimento em UTC, como foi gravado; "hoje" no fuso do tenant). É o mesmo
// predicado de `charge_postgres.go` — o painel financeiro mostrava a cobrança do
// próprio dia como vencida porque contava `< NOW()`.
func chargeOverduePredicateSQL(dueDateExpr, tzParam string) string {
	return fmt.Sprintf(
		"((%s)::timestamptz AT TIME ZONE 'UTC')::date < (NOW() AT TIME ZONE %s)::date",
		dueDateExpr, tzParam)
}

func (r *financialReportPostgresRepository) DelinquencyTotals(ctx context.Context, tenantID string) (report.DelinquencySnapshot, error) {
	var snap report.DelinquencySnapshot
	tz := timeutil.TimezoneNameForTenant(ctx, tenantID)
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1),
		       COALESCE(SUM(`+chargeBucketCentsSQL("c")+`), 0)::bigint,
		       COALESCE(AVG(GREATEST(0, ((NOW() AT TIME ZONE $2)::date - ((c.metadata->>'due_date')::timestamptz AT TIME ZONE 'UTC')::date))), 0)::float8
		FROM financial.charges c
		WHERE c.tenant_id = $1
		  AND c.deleted_at IS NULL
		  AND `+canonicalFinancialStatusSQL("c")+` IN ('OVERDUE', 'PENDING', 'PARTIALLY_PAID')
		  AND `+chargeOverduePredicateSQL("c.metadata->>'due_date'", "$2")+`
	`, tenantID, tz).Scan(&snap.OverdueCount, &snap.OverdueTotalCents, &snap.AverageDaysOverdue)
	if err != nil {
		return report.DelinquencySnapshot{}, err
	}
	return snap, nil
}

func scanStatusBuckets(ctx context.Context, db *database.PostgresDB, query string, args []any) ([]report.StatusBucket, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]report.StatusBucket, 0)
	for rows.Next() {
		var b report.StatusBucket
		if err := rows.Scan(&b.Status, &b.Count, &b.TotalCents); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// RevenueByType agrega a receita própria realizada de charges PAID +
// PARTIALLY_PAID por type. PaymentEntries com allocations consideram apenas
// allocation_type=revenue; registros sem allocations ficam a classificar. Cada evento usa sua própria data de pagamento.
func (r *financialReportPostgresRepository) RevenueByType(ctx context.Context, tenantID string, from, to time.Time) ([]report.CategoryBucket, error) {
	query := `
		SELECT type, COUNT(*)::bigint, COALESCE(SUM(cents), 0)::bigint
		FROM (` + ownRevenueEventsSQL() + `) u
		WHERE paid_at BETWEEN $2 AND $3
		GROUP BY type
		ORDER BY type
	`
	return scanCategoryBuckets(ctx, r.db, query, []any{tenantID, from, to})
}

// ExpenseByCategory aggregates agency expenses and broker commissions.
// Owner/partner transfers clear third-party liabilities and are never expenses. Apenas saídas liquidadas entram no realizado, pela
// payment_date (created_at é fallback para registros legados).
func (r *financialReportPostgresRepository) ExpenseByCategory(ctx context.Context, tenantID string, from, to time.Time) ([]report.CategoryBucket, error) {
	query := `
		SELECT category, COUNT(*)::bigint, COALESCE(SUM(cents), 0)::bigint
		FROM (
			SELECT COALESCE(NULLIF(expense_tx.metadata->>'category', ''), 'OUTROS') AS category,
			       ABS(ROUND(COALESCE(NULLIF(expense_tx.metadata->>'value', '')::numeric, 0) * 100))::bigint AS cents
			FROM financial.transactions expense_tx
			WHERE expense_tx.tenant_id = $1
			  AND expense_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("expense_tx") + ` IN ('despesa', 'comissao')
			  AND ` + canonicalFinancialStatusSQL("expense_tx") + ` IN ('PAID', 'REVERSED')
			  AND ` + transactionCashDateSQL("expense_tx") + ` BETWEEN $2 AND $3
			  AND NOT (` + canonicalTransactionTypeSQL("expense_tx") + ` = 'comissao'
			    AND ` + legacyNetCommissionPredicate("expense_tx") + `)
			UNION ALL
			SELECT COALESCE(NULLIF(original_tx.metadata->>'category', ''), 'OUTROS') AS category,
			       -ABS(ROUND(COALESCE(NULLIF(reversal_tx.metadata->>'value', '')::numeric, 0) * 100))::bigint AS cents
			FROM financial.transactions reversal_tx
			JOIN financial.transactions original_tx
			  ON original_tx.tenant_id = reversal_tx.tenant_id
			 AND original_tx.deleted_at IS NULL
			 AND original_tx.id::text = ` + transactionReversalOriginalIDSQL("reversal_tx") + `
			WHERE reversal_tx.tenant_id = $1
			  AND reversal_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("reversal_tx") + ` = 'estorno'
			  AND ` + canonicalFinancialStatusSQL("reversal_tx") + ` = 'PAID'
			  AND ` + canonicalTransactionTypeSQL("original_tx") + ` IN ('despesa', 'comissao')
			  AND ` + transactionCashDateSQL("reversal_tx") + ` BETWEEN $2 AND $3
			  AND NOT (` + canonicalTransactionTypeSQL("original_tx") + ` = 'comissao'
			    AND ` + legacyNetCommissionPredicate("original_tx") + `)
		) expense_events
		GROUP BY category
		ORDER BY category
	`
	return scanCategoryBuckets(ctx, r.db, query, []any{tenantID, from, to})
}

// Commission cash settlement can be smaller than the expense when taxes or
// other deductions remain payable. Resolve it through the immutable ledger
// link and original settlement journal, never a broker/amount coincidence.
func transactionCashOutflowCentsSQL(alias string) string {
	return `COALESCE((SELECT SUM(CASE WHEN leg->>'account' = 'cash'
		THEN CASE WHEN leg->>'direction' = 'credit' THEN 1 ELSE -1 END
		     * COALESCE(NULLIF(leg->>'amount_cents', '')::bigint, 0) ELSE 0 END)::bigint
		FROM financial.commissions cash_commission
		JOIN financial.journal_entries settlement
		  ON settlement.tenant_id = cash_commission.tenant_id
		 AND settlement.metadata->>'operation_id' = cash_commission.id::text
		CROSS JOIN LATERAL jsonb_array_elements(settlement.metadata->'legs') leg
		WHERE cash_commission.tenant_id = ` + alias + `.tenant_id
		  AND cash_commission.deleted_at IS NULL AND settlement.deleted_at IS NULL
		  AND cash_commission.metadata->>'ledger_transaction_id' = ` + alias + `.id::text
		  AND NULLIF(settlement.metadata->>'reversal_of', '') IS NULL
		  AND ` + canonicalTransactionTypeSQL(alias) + ` = 'comissao'
		HAVING COUNT(*) > 0),
		ABS(ROUND(COALESCE(NULLIF(` + alias + `.metadata->>'value', '')::numeric, 0) * 100))::bigint)`
}

// CashFlowByDay devolve buckets diários no fuso configurado com inflow (charges PAID)
// e outflow (transfers PROCESSED + withdrawals COMPLETE + lançamentos de
// despesa/repasse/comissão liquidados). Granularidade
// week/month é responsabilidade do service (ele reagrega a série diária).
func (r *financialReportPostgresRepository) CashFlowByDay(ctx context.Context, tenantID string, from, to time.Time) ([]report.CashFlowEntry, error) {
	rows, err := r.db.QueryContext(ctx, cashFlowByDaySQL(), tenantID, from, to, timeutil.TimezoneNameForTenant(ctx, tenantID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]report.CashFlowEntry, 0)
	for rows.Next() {
		var day time.Time
		var inflow, outflow int64
		if err := rows.Scan(&day, &inflow, &outflow); err != nil {
			return nil, err
		}
		out = append(out, report.CashFlowEntry{
			Period:       day.Format("2006-01-02"),
			InflowCents:  inflow,
			OutflowCents: outflow,
			NetCents:     inflow - outflow,
		})
	}
	return out, rows.Err()
}

// cashFlowByDaySQL uniformiza as fontes de caixa em (day, inflow, outflow):
// recebimentos de cobranças, repasses, saques, transações avulsas, notas de
// crédito e — pelo marcador explícito — os movimentos que só existem no razão.
func cashFlowByDaySQL() string {
	return `
		WITH inflows AS (
			SELECT ` + dayBucketSQL("receipt.paid_at") + ` AS day,
			       receipt.cents
			FROM financial.charges c
			` + chargeReceiptLateralSQL("c") + `
			WHERE c.tenant_id::text = $1::text
			  AND c.deleted_at IS NULL
			  AND ` + canonicalFinancialStatusSQL("c") + ` IN ('PAID', 'PARTIALLY_PAID', 'REVERSED')
			  AND receipt.paid_at BETWEEN $2 AND $3
		),
		outflows_transfers AS (
			SELECT ` + dayBucketSQL("COALESCE(NULLIF(t.metadata->>'processed_at', '')::timestamptz, t.created_at)") + ` AS day,
			       ABS(ROUND(COALESCE(NULLIF(t.metadata->>'net_amount', '')::numeric, 0) * 100))::bigint AS cents
			FROM financial.transfers t
			WHERE t.tenant_id::text = $1::text
			  AND t.deleted_at IS NULL
			  AND ` + canonicalFinancialStatusSQL("t") + ` IN ('PROCESSED', 'PAID', 'COMPLETE')
			  AND COALESCE(NULLIF(t.metadata->>'processed_at', '')::timestamptz, t.created_at) BETWEEN $2 AND $3
		),
		outflows_withdrawals AS (
			SELECT ` + dayBucketSQL(`COALESCE(
			         NULLIF(w.metadata->>'settled_at', '')::timestamptz,
			         NULLIF(w.metadata->>'completed_at', '')::timestamptz,
			         w.created_at)`) + ` AS day,
			       ABS(COALESCE(NULLIF(w.metadata->>'amount_cents', '')::bigint, 0)) AS cents
			FROM financial.withdrawals w
			WHERE w.tenant_id::text = $1::text
			  AND w.deleted_at IS NULL
			  AND ` + canonicalFinancialStatusSQL("w") + ` IN ('COMPLETE', 'PAID')
			  AND COALESCE(
			        NULLIF(w.metadata->>'settled_at', '')::timestamptz,
			        NULLIF(w.metadata->>'completed_at', '')::timestamptz,
			        w.created_at) BETWEEN $2 AND $3
		),
		inflows_manual AS (
			SELECT ` + dayBucketSQL(transactionCashDateSQL("manual_tx")) + ` AS day,
			       ROUND(COALESCE(NULLIF(manual_tx.metadata->>'value', '')::numeric, 0) * 100)::bigint AS cents
			FROM financial.transactions manual_tx
			WHERE manual_tx.tenant_id::text = $1::text
			  AND manual_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("manual_tx") + ` = 'receita'
			  AND ` + canonicalFinancialStatusSQL("manual_tx") + ` IN ('PAID', 'REVERSED')
			  AND ` + transactionCashDateSQL("manual_tx") + ` BETWEEN $2 AND $3
			  AND ` + unlinkedManualRevenuePredicate("manual_tx") + `
			  AND ROUND(COALESCE(NULLIF(manual_tx.metadata->>'value', '')::numeric, 0) * 100)::bigint > 0
		),
		-- Nota de crédito avulsa (estorno sem transação original): devolução de
		-- valor de terceiro retido a maior, saída de caixa quando liquidada.
		outflows_credit_notes AS (
			SELECT ` + dayBucketSQL(transactionCashDateSQL("credit_tx")) + ` AS day,
			       ABS(ROUND(COALESCE(NULLIF(credit_tx.metadata->>'value', '')::numeric, 0) * 100))::bigint AS cents
			FROM financial.transactions credit_tx
			WHERE credit_tx.tenant_id::text = $1::text
			  AND credit_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("credit_tx") + ` = 'estorno'
			  AND ` + canonicalFinancialStatusSQL("credit_tx") + ` IN ('PAID', 'REVERSED')
			  AND ` + transactionCashDateSQL("credit_tx") + ` BETWEEN $2 AND $3
			  AND ` + transactionReversalOriginalIDSQL("credit_tx") + ` IS NULL
		),
		-- Movimentos de caixa que só existem no razão (ex.: adiantamento de
		-- manutenção a reembolsar): entram pelo marcador explícito da entrada,
		-- para não duplicar o que já vem de cobranças, repasses e transações.
		inflows_ledger AS (
			SELECT ` + dayBucketSQL(journalEntryDateSQL("entry")) + ` AS day,
			       SUM(COALESCE(NULLIF(leg->>'amount_cents', '')::bigint, 0)) AS cents
			FROM financial.journal_entries entry
			CROSS JOIN LATERAL jsonb_array_elements(
			  CASE WHEN jsonb_typeof(entry.metadata->'legs') = 'array'
			    THEN entry.metadata->'legs' ELSE '[]'::jsonb END
			) AS leg
			WHERE entry.tenant_id::text = $1::text
			  AND entry.deleted_at IS NULL
			  AND NULLIF(entry.metadata->>'cash_flow_source', '') IS NOT NULL
			  AND leg->>'account' = 'cash'
			  AND leg->>'direction' = 'debit'
			  AND ` + journalEntryDateSQL("entry") + ` BETWEEN $2 AND $3
			GROUP BY 1
		),
		outflows_ledger AS (
			SELECT ` + dayBucketSQL(journalEntryDateSQL("entry")) + ` AS day,
			       SUM(COALESCE(NULLIF(leg->>'amount_cents', '')::bigint, 0)) AS cents
			FROM financial.journal_entries entry
			CROSS JOIN LATERAL jsonb_array_elements(
			  CASE WHEN jsonb_typeof(entry.metadata->'legs') = 'array'
			    THEN entry.metadata->'legs' ELSE '[]'::jsonb END
			) AS leg
			WHERE entry.tenant_id::text = $1::text
			  AND entry.deleted_at IS NULL
			  AND NULLIF(entry.metadata->>'cash_flow_source', '') IS NOT NULL
			  AND leg->>'account' = 'cash'
			  AND leg->>'direction' = 'credit'
			  AND ` + journalEntryDateSQL("entry") + ` BETWEEN $2 AND $3
			GROUP BY 1
		),
		inflow_reversals AS (
			SELECT ` + dayBucketSQL(transactionCashDateSQL("reversal_tx")) + ` AS day,
			       -` + transactionCashOutflowCentsSQL("original_tx") + ` AS cents
			FROM financial.transactions reversal_tx
			JOIN financial.transactions original_tx
			  ON original_tx.tenant_id = reversal_tx.tenant_id
			 AND original_tx.deleted_at IS NULL
			 AND original_tx.id::text = ` + transactionReversalOriginalIDSQL("reversal_tx") + `
			WHERE reversal_tx.tenant_id::text = $1::text
			  AND reversal_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("reversal_tx") + ` = 'estorno'
			  AND ` + canonicalFinancialStatusSQL("reversal_tx") + ` = 'PAID'
			  AND ` + canonicalTransactionTypeSQL("original_tx") + ` = 'receita'
			  AND ` + transactionCashDateSQL("reversal_tx") + ` BETWEEN $2 AND $3
			  AND ` + unlinkedManualRevenuePredicate("original_tx") + `
		),
		outflows_manual AS (
			SELECT ` + dayBucketSQL(transactionCashDateSQL("expense_tx")) + ` AS day,
			       ` + transactionCashOutflowCentsSQL("expense_tx") + ` AS cents
			FROM financial.transactions expense_tx
			WHERE expense_tx.tenant_id::text = $1::text
			  AND expense_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("expense_tx") + ` IN ('despesa', 'repasse', 'comissao')
			  AND ` + canonicalFinancialStatusSQL("expense_tx") + ` IN ('PAID', 'REVERSED')
			  AND ` + transactionCashDateSQL("expense_tx") + ` BETWEEN $2 AND $3
		),
		outflow_reversals AS (
			SELECT ` + dayBucketSQL(transactionCashDateSQL("reversal_tx")) + ` AS day,
			       -` + transactionCashOutflowCentsSQL("original_tx") + ` AS cents
			FROM financial.transactions reversal_tx
			JOIN financial.transactions original_tx
			  ON original_tx.tenant_id = reversal_tx.tenant_id
			 AND original_tx.deleted_at IS NULL
			 AND original_tx.id::text = ` + transactionReversalOriginalIDSQL("reversal_tx") + `
			WHERE reversal_tx.tenant_id::text = $1::text
			  AND reversal_tx.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("reversal_tx") + ` = 'estorno'
			  AND ` + canonicalFinancialStatusSQL("reversal_tx") + ` = 'PAID'
			  AND ` + canonicalTransactionTypeSQL("original_tx") + ` IN ('despesa', 'repasse', 'comissao')
			  AND ` + transactionCashDateSQL("reversal_tx") + ` BETWEEN $2 AND $3
		),
		all_days AS (
			SELECT day, SUM(cents)::bigint AS inflow, 0::bigint AS outflow FROM inflows GROUP BY day
			UNION ALL
			SELECT day, 0::bigint, SUM(cents)::bigint FROM outflows_transfers GROUP BY day
			UNION ALL
			SELECT day, 0::bigint, SUM(cents)::bigint FROM outflows_withdrawals GROUP BY day
			UNION ALL
			SELECT day, SUM(cents)::bigint, 0::bigint FROM inflows_manual GROUP BY day
			UNION ALL
			SELECT day, SUM(cents)::bigint, 0::bigint FROM inflow_reversals GROUP BY day
			UNION ALL
			SELECT day, 0::bigint, SUM(cents)::bigint FROM outflows_manual GROUP BY day
			UNION ALL
			SELECT day, 0::bigint, SUM(cents)::bigint FROM outflow_reversals GROUP BY day
			UNION ALL
			SELECT day, 0::bigint, SUM(cents)::bigint FROM outflows_credit_notes GROUP BY day
			UNION ALL
			SELECT day, SUM(cents)::bigint, 0::bigint FROM inflows_ledger GROUP BY day
			UNION ALL
			SELECT day, 0::bigint, SUM(cents)::bigint FROM outflows_ledger GROUP BY day
		)
		SELECT day, SUM(inflow)::bigint, SUM(outflow)::bigint
		FROM all_days
		GROUP BY day
		ORDER BY day
	`
}

// KpisSnapshot consolida a posição operacional e os indicadores realizados.
// Receita usa a mesma fronteira de titularidade econômica da DRE; cobrança e
// inadimplência continuam baseadas nos recebimentos brutos das charges.
func (r *financialReportPostgresRepository) KpisSnapshot(ctx context.Context, tenantID string) (report.KPIs, error) {
	// As janelas do KPI são DIAS CIVIS no fuso configurado: em UTC, entre 21h e
	// meia-noite de Brasília o "mês" já virou e a receita do mês aparecia zerada
	// (medido no beta em 01/10/2026 00:58 UTC com o movimento de 30/09 21:58 BRT).
	now := time.Now()
	loc := timeutil.LocationForTenant(ctx, tenantID)
	local := now.In(loc)
	monthStart := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
	last30 := timeutil.StartOfDayIn(loc, now).AddDate(0, 0, -30)

	out := report.KPIs{TenantID: tenantID, GeneratedAt: now}

	var ownRevenueEventCount int
	// 1) Receita própria realizada: allocations para charges e breakdown para
	// recebimentos manuais distribuídos. A contagem positiva alimenta a receita
	// média por evento sem contar o contralançamento de um estorno duas vezes.
	if err := r.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(cents) FILTER (WHERE paid_at >= $2), 0)::bigint,
		  COALESCE(SUM(cents) FILTER (WHERE paid_at >= $3), 0)::bigint,
		  COUNT(*) FILTER (WHERE paid_at >= $2 AND cents > 0)
		FROM (`+ownRevenueEventsSQL()+`) own_revenue_events
	`, tenantID, monthStart, last30).Scan(
		&out.RevenueMonthCents,
		&out.RevenueLast30Cents,
		&ownRevenueEventCount,
	); err != nil {
		return out, err
	}

	// 2) Receivable + Overdue. PARTIALLY_PAID contributes only the balance
	// still open (total_amount - paid_amount), never the original full amount.
	tz := timeutil.TimezoneNameForTenant(ctx, tenantID)
	// "Vencido" no KPI usa a mesma regra do produto (vence no dia, atrasa no dia
	// seguinte): era aqui que a cobrança do próprio dia entrava em VENCIDOS.
	//
	// O predicado de data é a ÚNICA fonte: o KPI não conta mais "status = OVERDUE"
	// sozinho, porque o status é estado derivado e pode estar defasado (foi o que
	// aconteceu com as cobranças marcadas pela regra antiga — o motor as devolve
	// para PENDING, mas o painel não pode depender do momento em que isso roda).
	vencido := chargeOverduePredicateSQL("c.metadata->>'due_date'", "$2")
	if err := r.db.QueryRowContext(ctx, `
			SELECT
			  COALESCE(SUM(CASE WHEN `+canonicalFinancialStatusSQL("c")+` IN ('PENDING','OVERDUE','PARTIALLY_PAID') THEN `+chargeBucketCentsSQL("c")+` ELSE 0 END), 0)::bigint,
			  COALESCE(SUM(CASE WHEN `+canonicalFinancialStatusSQL("c")+` IN ('OVERDUE','PENDING','PARTIALLY_PAID') AND `+vencido+` THEN `+chargeBucketCentsSQL("c")+` ELSE 0 END), 0)::bigint,
		  COUNT(*) FILTER (WHERE `+canonicalFinancialStatusSQL("c")+` IN ('OVERDUE','PENDING','PARTIALLY_PAID') AND `+vencido+`)
		FROM financial.charges c
		WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
	`, tenantID, tz).Scan(&out.ReceivableCents, &out.OverdueCents, &out.OverdueCount); err != nil {
		return out, err
	}

	// 3) Inadimplência compara a carteira vencida à cobrança bruta realizada,
	// nunca à receita própria (que excluiria o dinheiro dos proprietários).
	var collectedMonthCents int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(receipt.cents), 0)::bigint
		FROM financial.charges c
		`+chargeReceiptLateralSQL("c")+`
		WHERE c.tenant_id = $1
		  AND c.deleted_at IS NULL
		  AND `+canonicalFinancialStatusSQL("c")+` IN ('PAID', 'PARTIALLY_PAID', 'REVERSED')
		  AND receipt.paid_at >= $2
	`, tenantID, monthStart).Scan(&collectedMonthCents); err != nil {
		return out, err
	}
	denom := out.OverdueCents + collectedMonthCents
	if denom > 0 {
		out.DelinquencyPct = float64(out.OverdueCents) / float64(denom) * 100
	}

	// 4) Receita própria média por evento realizado.
	if ownRevenueEventCount > 0 {
		out.AverageTicketCents = out.RevenueMonthCents / int64(ownRevenueEventCount)
	}

	// 5) Contratos ativos = rentals com status ACTIVE
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM financial.rentals rental
		WHERE rental.tenant_id = $1 AND rental.deleted_at IS NULL
		  AND `+canonicalFinancialStatusSQL("rental")+` = 'ACTIVE'
	`, tenantID).Scan(&out.ActiveContractCount); err != nil {
		return out, fmt.Errorf("active contract KPI: %w", err)
	}

	// 6) Comissões liquidadas no mês, líquidas de estornos. The original
	// posting remains on its payment date and the counter-posting is applied on
	// the reversal date, matching DRE/cash-flow period semantics.
	if err := r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(cents), 0)::bigint
		FROM (
			SELECT ABS(ROUND(COALESCE(NULLIF(commission_tx.metadata->>'value', '')::numeric, 0) * 100))::bigint AS cents
			FROM financial.transactions commission_tx
			WHERE commission_tx.tenant_id = $1
			  AND commission_tx.deleted_at IS NULL
			  AND `+canonicalTransactionTypeSQL("commission_tx")+` = 'comissao'
			  AND `+canonicalFinancialStatusSQL("commission_tx")+` IN ('PAID', 'REVERSED')
			  AND `+transactionCashDateSQL("commission_tx")+` >= $2
			UNION ALL
			SELECT -ABS(ROUND(COALESCE(NULLIF(reversal_tx.metadata->>'value', '')::numeric, 0) * 100))::bigint AS cents
			FROM financial.transactions reversal_tx
			JOIN financial.transactions original_tx
			  ON original_tx.tenant_id = reversal_tx.tenant_id
			 AND original_tx.deleted_at IS NULL
			 AND original_tx.id::text = `+transactionReversalOriginalIDSQL("reversal_tx")+`
			WHERE reversal_tx.tenant_id = $1
			  AND reversal_tx.deleted_at IS NULL
			  AND `+canonicalTransactionTypeSQL("reversal_tx")+` = 'estorno'
			  AND `+canonicalFinancialStatusSQL("reversal_tx")+` = 'PAID'
			  AND `+canonicalTransactionTypeSQL("original_tx")+` = 'comissao'
			  AND `+transactionCashDateSQL("reversal_tx")+` >= $2
		) commission_events
	`, tenantID, monthStart).Scan(&out.CommissionPaidMonth); err != nil {
		return out, fmt.Errorf("commission paid KPI: %w", err)
	}

	// 7) Posição contábil corrente. O journal é a fonte única para caixa e
	// passivos: obrigações de terceiros não são reconstruídas a partir das
	// linhas espelhadas da inbox. Reversões entram como contralançamentos e
	// portanto neutralizam naturalmente o saldo original.
	var position financialPositionBalances
	if err := r.db.QueryRowContext(ctx, journalPositionSQL(), tenantID).Scan(
		&position.cash,
		&position.ownerPayable,
		&position.commissionPayable,
		&position.partnerPayable,
		&position.royaltyPayable,
		&position.taxWithheld,
		&position.otherPayable,
		&position.withdrawalPayable,
		&position.unexpectedPayableDebits,
	); err != nil {
		return out, fmt.Errorf("financial position KPI: %w", err)
	}
	if err := applyFinancialPosition(&out, position); err != nil {
		return out, fmt.Errorf("financial position KPI: %w", err)
	}

	return out, nil
}

func scanCategoryBuckets(ctx context.Context, db *database.PostgresDB, query string, args []any) ([]report.CategoryBucket, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]report.CategoryBucket, 0)
	for rows.Next() {
		var b report.CategoryBucket
		if err := rows.Scan(&b.Category, &b.Count, &b.TotalCents); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}
