package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/report"
)

type reconciliationPostgresRepository struct {
	db *database.PostgresDB
}

func NewReconciliationPostgresRepository(db *database.PostgresDB) *reconciliationPostgresRepository {
	return &reconciliationPostgresRepository{db: db}
}

// Run executa todas as verificações de reconciliação contra o banco
// e retorna um relatório dry-run com todos os findings categorizados.
// Nenhuma modificação de dados é feita — é puramente leitura.
func (r *reconciliationPostgresRepository) Run(ctx context.Context, tenantID string) (*report.ReconciliationReport, error) {
	rep := &report.ReconciliationReport{
		TenantID:    tenantID,
		GeneratedAt: time.Now().UTC(),
		DryRun:      true,
		Findings:    make([]report.ReconciliationFinding, 0),
	}

	checks := []reconciliationCheck{
		{"duplicates", r.findDuplicates},
		{"orphans", r.findOrphans},
		{"allocation gaps", r.findAllocationGaps},
		{"charge payment consistency", r.findChargePaymentIssues},
		{"transaction signs and reversals", r.findTransactionIssues},
		{"status cascade consistency", r.findConsistencyIssues},
	}
	if err := runReconciliationChecks(ctx, tenantID, rep, checks); err != nil {
		// Never return a partial report as if it were usable. The handler turns
		// this into a non-2xx response, making every SQL/query/scan error a
		// fail-closed reconciliation run.
		return nil, err
	}

	finalizeReconciliationSummary(rep)
	return rep, nil
}

type reconciliationCheck struct {
	name string
	run  func(context.Context, string, *report.ReconciliationReport) error
}

func runReconciliationChecks(ctx context.Context, tenantID string, rep *report.ReconciliationReport, checks []reconciliationCheck) error {
	for _, check := range checks {
		if err := check.run(ctx, tenantID, rep); err != nil {
			return fmt.Errorf("reconciliation %s: %w", check.name, err)
		}
	}
	return nil
}

func finalizeReconciliationSummary(rep *report.ReconciliationReport) {
	// Derive every summary field from Findings in one place. Individual checks
	// are deliberately unable to leave counters out of sync when a new finding
	// path is added or a check is refactored.
	rep.Summary = report.ReconciliationSummary{TotalFindings: len(rep.Findings)}
	for _, finding := range rep.Findings {
		switch strings.ToUpper(strings.TrimSpace(finding.Severity)) {
		case "ERROR":
			rep.Summary.Errors++
		case "WARN":
			rep.Summary.Warnings++
		default:
			// Unknown severity must not make the run look greener than it is.
			rep.Summary.Errors++
		}
		switch strings.ToLower(strings.TrimSpace(finding.Category)) {
		case "duplicate":
			rep.Summary.Duplicates++
		case "orphan":
			rep.Summary.Orphans++
		case "excess":
			rep.Summary.ExcessAlloc++
		case "suballoc":
			rep.Summary.SubAlloc++
		case "consistency":
			rep.Summary.Consistency++
		}
	}
}

// ── Duplicates ─────────────────────────────────────────────────────

func (r *reconciliationPostgresRepository) findDuplicates(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	tables := []struct {
		name       string
		entityType string
	}{
		{"financial.commissions", "Commission"},
		{"financial.transactions", "Transaction"},
		{"financial.charges", "Charge"},
		{"financial.payout_orders", "PayoutOrder"},
	}

	for _, t := range tables {
		query := fmt.Sprintf(`
			SELECT (metadata->>'source_event_id'), COUNT(1)
			FROM %s
			WHERE tenant_id = $1
			  AND deleted_at IS NULL
			  AND (metadata->>'source_event_id') IS NOT NULL
			GROUP BY (metadata->>'source_event_id')
			HAVING COUNT(1) > 1
			LIMIT 100`, t.name)

		rows, err := r.db.QueryContext(ctx, query, tenantID)
		if err != nil {
			return fmt.Errorf("query duplicate %s: %w", t.entityType, err)
		}

		for rows.Next() {
			var sourceEventID string
			var count int64
			if err := rows.Scan(&sourceEventID, &count); err != nil {
				_ = rows.Close()
				return fmt.Errorf("scan duplicate %s: %w", t.entityType, err)
			}
			rep.Findings = append(rep.Findings, report.ReconciliationFinding{
				Severity:   "ERROR",
				Category:   "duplicate",
				EntityType: t.entityType,
				EntityID:   sourceEventID,
				Reference:  fmt.Sprintf("%d occurrences", count),
				Message:    fmt.Sprintf("%s com source_event_id duplicado: %d registros", t.entityType, count),
			})
			rep.Summary.Duplicates++
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("iterate duplicate %s: %w", t.entityType, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close duplicate %s rows: %w", t.entityType, err)
		}
	}
	return nil
}

// ── Orphans ────────────────────────────────────────────────────────

func (r *reconciliationPostgresRepository) findOrphans(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	// Commission sem Transaction correspondente (mesmo plan_hash + contract_id)
	if err := r.findCommissionWithoutTransaction(ctx, tenantID, rep); err != nil {
		return err
	}

	// Transaction comissao sem Commission correspondente
	if err := r.findTransactionWithoutCommission(ctx, tenantID, rep); err != nil {
		return err
	}

	// PayoutOrder (broker) sem Commission vinculada via payout_order_id
	return r.findPayoutWithoutCommission(ctx, tenantID, rep)
}

func (r *reconciliationPostgresRepository) findCommissionWithoutTransaction(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	query := `
		SELECT c.id
		FROM financial.commissions c
		WHERE c.tenant_id = $1
		  AND c.deleted_at IS NULL
		  AND NULLIF(BTRIM(c.metadata->>'plan_hash'), '') IS NOT NULL
		  AND NULLIF(BTRIM(c.metadata->>'contract_id'), '') IS NOT NULL
		  AND NULLIF(BTRIM(c.metadata->>'broker_id'), '') IS NOT NULL
		  AND NULLIF(BTRIM(c.metadata->>'role'), '') IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM financial.transactions t
			WHERE t.tenant_id = c.tenant_id
			  AND t.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("t") + ` = 'comissao'
			  AND (t.metadata->>'plan_hash') = (c.metadata->>'plan_hash')
			  AND (t.metadata->>'contract_id') = (c.metadata->>'contract_id')
			  AND (t.metadata->>'broker_id') = (c.metadata->>'broker_id')
			  AND (t.metadata->>'role') = (c.metadata->>'role')
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM financial.transactions legacy_t
			WHERE legacy_t.tenant_id = c.tenant_id
			  AND legacy_t.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("legacy_t") + ` = 'comissao'
			  AND (legacy_t.metadata->>'plan_hash') = (c.metadata->>'plan_hash')
			  AND (legacy_t.metadata->>'contract_id') = (c.metadata->>'contract_id')
			  AND (
			    NULLIF(BTRIM(legacy_t.metadata->>'broker_id'), '') IS NULL
			    OR NULLIF(BTRIM(legacy_t.metadata->>'role'), '') IS NULL
			  )
		  )
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query commission without transaction: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan commission without transaction: %w", err)
		}
		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   "WARN",
			Category:   "orphan",
			EntityType: "Commission",
			EntityID:   id,
			Message:    "Commission sem Transaction correlacionável (plan_hash + contract_id + broker_id + role)",
		})
		rep.Summary.Orphans++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate commission without transaction: %w", err)
	}
	return nil
}

func (r *reconciliationPostgresRepository) findTransactionWithoutCommission(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	query := `
		SELECT t.id
		FROM financial.transactions t
		WHERE t.tenant_id = $1
		  AND t.deleted_at IS NULL
		  AND ` + canonicalTransactionTypeSQL("t") + ` = 'comissao'
		  AND NULLIF(BTRIM(t.metadata->>'plan_hash'), '') IS NOT NULL
		  AND NULLIF(BTRIM(t.metadata->>'contract_id'), '') IS NOT NULL
		  AND NULLIF(BTRIM(t.metadata->>'broker_id'), '') IS NOT NULL
		  AND NULLIF(BTRIM(t.metadata->>'role'), '') IS NOT NULL
		  AND NOT EXISTS (
			SELECT 1 FROM financial.commissions c
			WHERE c.tenant_id = t.tenant_id
			  AND c.deleted_at IS NULL
			  AND (c.metadata->>'plan_hash') = (t.metadata->>'plan_hash')
			  AND (c.metadata->>'contract_id') = (t.metadata->>'contract_id')
			  AND (c.metadata->>'broker_id') = (t.metadata->>'broker_id')
			  AND (c.metadata->>'role') = (t.metadata->>'role')
		  )
		  AND NOT EXISTS (
			SELECT 1 FROM financial.commissions legacy_c
			WHERE legacy_c.tenant_id = t.tenant_id
			  AND legacy_c.deleted_at IS NULL
			  AND (legacy_c.metadata->>'plan_hash') = (t.metadata->>'plan_hash')
			  AND (legacy_c.metadata->>'contract_id') = (t.metadata->>'contract_id')
			  AND (
			    NULLIF(BTRIM(legacy_c.metadata->>'broker_id'), '') IS NULL
			    OR NULLIF(BTRIM(legacy_c.metadata->>'role'), '') IS NULL
			  )
		  )
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query transaction without commission: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan transaction without commission: %w", err)
		}
		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   "WARN",
			Category:   "orphan",
			EntityType: "Transaction",
			EntityID:   id,
			Message:    "Transaction comissao sem Commission correlacionável (plan_hash + contract_id + broker_id + role)",
		})
		rep.Summary.Orphans++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate transaction without commission: %w", err)
	}
	return nil
}

func (r *reconciliationPostgresRepository) findPayoutWithoutCommission(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	query := `
		SELECT p.id
		FROM financial.payout_orders p
		WHERE p.tenant_id::text = $1::text
		  AND p.deleted_at IS NULL
		  AND (p.metadata->>'recipient_type') = 'broker'
		  AND (p.metadata->>'source_type') IS DISTINCT FROM 'royalty_settlement'
		  AND NOT EXISTS (
			SELECT 1 FROM financial.commissions c
			WHERE c.tenant_id::text = p.tenant_id::text
			  AND c.deleted_at IS NULL
			  AND (c.metadata->>'payout_order_id') = p.id::text
		  )
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query payout without commission: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("scan payout without commission: %w", err)
		}
		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   "ERROR",
			Category:   "orphan",
			EntityType: "PayoutOrder",
			EntityID:   id,
			Message:    "PayoutOrder (broker) sem Commission vinculada via payout_order_id",
		})
		rep.Summary.Orphans++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate payout without commission: %w", err)
	}
	return nil
}

// ── Allocation Gaps ────────────────────────────────────────────────

func (r *reconciliationPostgresRepository) findAllocationGaps(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	// Para cada contract_id com commissions, compara a soma dos amounts
	// das commissions com a soma das transactions de comissão.
	// Excess: transactions > commissions  |  Suballoc: transactions < commissions
	query := `
		WITH comm_sums AS (
			SELECT
				(c.metadata->>'contract_id') AS contract_id,
				SUM(ABS(ROUND(COALESCE(NULLIF(c.metadata->>'amount', '')::numeric, 0) * 100)))::bigint AS total_commission_cents
			FROM financial.commissions c
			WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
			  AND (c.metadata->>'contract_id') IS NOT NULL
			  AND ` + canonicalFinancialStatusSQL("c") + ` <> 'CANCELLED'
			GROUP BY (c.metadata->>'contract_id')
		),
		tx_sums AS (
			SELECT
				(t.metadata->>'contract_id') AS contract_id,
				SUM(ABS(ROUND(COALESCE(NULLIF(t.metadata->>'value', '')::numeric, 0) * 100)))::bigint AS total_ledger_cents
			FROM financial.transactions t
			WHERE t.tenant_id = $1 AND t.deleted_at IS NULL
			  AND ` + canonicalTransactionTypeSQL("t") + ` = 'comissao'
			  AND (t.metadata->>'contract_id') IS NOT NULL
			  AND ` + canonicalFinancialStatusSQL("t") + ` <> 'CANCELLED'
			GROUP BY (t.metadata->>'contract_id')
		)
		SELECT
			COALESCE(c.contract_id, t.contract_id),
			COALESCE(c.total_commission_cents, 0),
			COALESCE(t.total_ledger_cents, 0)
		FROM comm_sums c
		FULL OUTER JOIN tx_sums t ON c.contract_id = t.contract_id
		WHERE ABS(COALESCE(c.total_commission_cents, 0) - COALESCE(t.total_ledger_cents, 0)) > 2
		ORDER BY ABS(COALESCE(c.total_commission_cents, 0) - COALESCE(t.total_ledger_cents, 0)) DESC
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query allocation gaps: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var contractID string
		var commCents, txCents int64
		if err := rows.Scan(&contractID, &commCents, &txCents); err != nil {
			return fmt.Errorf("scan allocation gaps: %w", err)
		}

		diffCents := txCents - commCents
		category := "suballoc"
		severity := "WARN"
		if diffCents > 0 {
			category = "excess"
			severity = "ERROR"
		}

		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   severity,
			Category:   category,
			EntityType: "Contract",
			EntityID:   contractID,
			Reference:  fmt.Sprintf("ledger_cents=%d commission_cents=%d diff_cents=%d", txCents, commCents, diffCents),
			Message:    fmt.Sprintf("Contract %s: divergência entre ledger (%d centavos) e commissions (%d centavos)", contractID, txCents, commCents),
		})

		if category == "excess" {
			rep.Summary.ExcessAlloc++
		} else {
			rep.Summary.SubAlloc++
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate allocation gaps: %w", err)
	}
	return nil
}

// ── Charge payment consistency ────────────────────────────────────

type chargePaymentState struct {
	ID                    string
	Status                string
	TotalCents            int64
	LegacyPaidCents       int64
	PaymentsSumCents      int64
	EffectivePaidCents    int64
	PaymentCount          int64
	HasLegacyPaidAmount   bool
	HasNonPositivePayment bool
}

func (s chargePaymentState) effectivePaidCents() int64 {
	return s.EffectivePaidCents
}

func chargePaymentConsistencyFindings(s chargePaymentState) []report.ReconciliationFinding {
	findings := make([]report.ReconciliationFinding, 0, 3)
	add := func(message, reference string) {
		findings = append(findings, report.ReconciliationFinding{
			Severity:   "ERROR",
			Category:   "consistency",
			EntityType: "Charge",
			EntityID:   s.ID,
			Reference:  reference,
			Message:    message,
		})
	}
	if s.TotalCents <= 0 {
		add("Charge com total não positivo", fmt.Sprintf("total_cents=%d", s.TotalCents))
	}
	if s.HasNonPositivePayment {
		add("Charge contém PaymentEntry com sinal inválido", "payment amount_cents must be > 0")
	}
	if s.PaymentCount > 0 && (!s.HasLegacyPaidAmount || s.LegacyPaidCents != s.PaymentsSumCents) {
		add("paid_amount diverge da soma append-only de payments",
			fmt.Sprintf("paid_amount_cents=%d payments_sum_cents=%d", s.LegacyPaidCents, s.PaymentsSumCents))
	}

	paid := s.effectivePaidCents()
	switch strings.ToUpper(strings.TrimSpace(s.Status)) {
	case "PAID", "REVERSED":
		if paid != s.TotalCents {
			add("Status fechado sem pagamento integral",
				fmt.Sprintf("status=%s total_cents=%d paid_cents=%d", s.Status, s.TotalCents, paid))
		}
	case "PARTIALLY_PAID":
		if paid <= 0 || paid >= s.TotalCents {
			add("Status PARTIALLY_PAID incompatível com o saldo",
				fmt.Sprintf("total_cents=%d paid_cents=%d", s.TotalCents, paid))
		}
	case "PENDING", "OVERDUE", "CANCELLED":
		if paid != 0 {
			add("Status sem recebimento contém valor pago",
				fmt.Sprintf("status=%s paid_cents=%d", s.Status, paid))
		}
	default:
		add("Status de Charge desconhecido", fmt.Sprintf("status=%s", s.Status))
	}
	return findings
}

func (r *reconciliationPostgresRepository) findChargePaymentIssues(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	payments := chargePaymentsArraySQL("c")
	query := `
		WITH charge_state AS (
			SELECT c.id::text AS id,
			       ` + canonicalFinancialStatusSQL("c") + ` AS status,
			       ROUND(COALESCE(NULLIF(c.metadata->>'total_amount', '')::numeric, 0) * 100)::bigint AS total_cents,
			       ` + chargeLegacyPaidAmountCentsSQL("c") + ` AS legacy_paid_cents,
			       ` + chargePaymentsSumCentsSQL("c") + ` AS payments_sum_cents,
			       ` + chargePaidAmountCentsSQL("c") + ` AS effective_paid_cents,
			       jsonb_array_length(` + payments + `)::bigint AS payment_count,
			       NULLIF(c.metadata->>'paid_amount', '') IS NOT NULL AS has_legacy_paid_amount,
			       EXISTS (
			         SELECT 1 FROM jsonb_array_elements(` + payments + `) payment
			         WHERE COALESCE(NULLIF(payment->>'amount_cents', '')::bigint, 0) <= 0
			       ) AS has_non_positive_payment
			FROM financial.charges c
			WHERE c.tenant_id = $1 AND c.deleted_at IS NULL
		)
		SELECT id, status, total_cents, legacy_paid_cents, payments_sum_cents,
		       effective_paid_cents, payment_count, has_legacy_paid_amount,
		       has_non_positive_payment
		FROM charge_state
		WHERE total_cents <= 0
		   OR has_non_positive_payment
		   OR (payment_count > 0 AND (NOT has_legacy_paid_amount OR legacy_paid_cents <> payments_sum_cents))
		   OR (status IN ('PAID', 'REVERSED') AND effective_paid_cents <> total_cents)
		   OR (status = 'PARTIALLY_PAID' AND
		       (effective_paid_cents <= 0 OR effective_paid_cents >= total_cents))
		   OR (status IN ('PENDING', 'OVERDUE', 'CANCELLED') AND
		       effective_paid_cents <> 0)
		   OR status NOT IN ('PENDING', 'OVERDUE', 'PARTIALLY_PAID', 'PAID', 'REVERSED', 'CANCELLED')
		ORDER BY id
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query charge payment consistency: %w", err)
	}
	for rows.Next() {
		var state chargePaymentState
		if err := rows.Scan(
			&state.ID,
			&state.Status,
			&state.TotalCents,
			&state.LegacyPaidCents,
			&state.PaymentsSumCents,
			&state.EffectivePaidCents,
			&state.PaymentCount,
			&state.HasLegacyPaidAmount,
			&state.HasNonPositivePayment,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan charge payment consistency: %w", err)
		}
		rep.Findings = append(rep.Findings, chargePaymentConsistencyFindings(state)...)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate charge payment consistency: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close charge payment consistency rows: %w", err)
	}
	return nil
}

// ── Transaction signs and reversals ───────────────────────────────

type transactionReversalState struct {
	ID             string
	Type           string
	Status         string
	ValueCents     int64
	OriginalID     string
	OriginalType   string
	OriginalStatus string
	OriginalCents  int64
}

func absCents(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func transactionConsistencyFindings(s transactionReversalState) []report.ReconciliationFinding {
	findings := make([]report.ReconciliationFinding, 0, 4)
	add := func(category, message, reference string) {
		findings = append(findings, report.ReconciliationFinding{
			Severity:   "ERROR",
			Category:   category,
			EntityType: "Transaction",
			EntityID:   s.ID,
			Reference:  reference,
			Message:    message,
		})
	}
	txType := strings.ToLower(strings.TrimSpace(s.Type))
	status := strings.ToUpper(strings.TrimSpace(s.Status))
	knownStatus := status == "PENDING" || status == "PAID" || status == "OVERDUE" || status == "REVERSED"
	if !knownStatus {
		add("consistency", "Status de Transaction desconhecido", fmt.Sprintf("status=%s", s.Status))
	}

	switch txType {
	case "receita", "despesa", "repasse", "comissao":
		if s.ValueCents <= 0 {
			add("consistency", "Transaction normal deve persistir magnitude positiva",
				fmt.Sprintf("type=%s value_cents=%d", txType, s.ValueCents))
		}
	case "estorno":
		if s.ValueCents >= 0 {
			add("consistency", "Transaction de estorno deve ter sinal negativo",
				fmt.Sprintf("value_cents=%d", s.ValueCents))
		}
		if status != "PAID" {
			add("consistency", "Transaction de estorno deve estar liquidada",
				fmt.Sprintf("status=%s", s.Status))
		}
		if s.OriginalID == "" {
			add("orphan", "Transaction de estorno sem lançamento original no mesmo tenant", "missing reversal original")
			break
		}
		if strings.ToLower(strings.TrimSpace(s.OriginalType)) == "estorno" {
			add("consistency", "Estorno não pode apontar para outro estorno", "original_id="+s.OriginalID)
		}
		if strings.ToUpper(strings.TrimSpace(s.OriginalStatus)) != "REVERSED" {
			add("consistency", "Lançamento original do estorno não está REVERSED",
				fmt.Sprintf("original_id=%s original_status=%s", s.OriginalID, s.OriginalStatus))
		}
		if delta := absCents(s.ValueCents) - absCents(s.OriginalCents); delta > 2 || delta < -2 {
			add("consistency", "Valor do estorno diverge do lançamento original",
				fmt.Sprintf("reversal_cents=%d original_cents=%d", s.ValueCents, s.OriginalCents))
		}
	default:
		add("consistency", "Tipo de Transaction desconhecido", fmt.Sprintf("type=%s", s.Type))
	}
	return findings
}

func (r *reconciliationPostgresRepository) findTransactionIssues(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	query := `
		SELECT t.id::text,
		       ` + canonicalTransactionTypeSQL("t") + `,
		       ` + canonicalFinancialStatusSQL("t") + `,
		       ROUND(COALESCE(NULLIF(t.metadata->>'value', '')::numeric, 0) * 100)::bigint,
		       COALESCE(original.id::text, ''),
		       COALESCE(` + canonicalTransactionTypeSQL("original") + `, ''),
		       COALESCE(` + canonicalFinancialStatusSQL("original") + `, ''),
		       ROUND(COALESCE(NULLIF(original.metadata->>'value', '')::numeric, 0) * 100)::bigint
		FROM financial.transactions t
		LEFT JOIN financial.transactions original
		  ON original.tenant_id = t.tenant_id
		 AND original.deleted_at IS NULL
		 AND original.id::text = ` + transactionReversalOriginalIDSQL("t") + `
		WHERE t.tenant_id = $1
		  AND t.deleted_at IS NULL
		  AND (
		    ` + canonicalTransactionTypeSQL("t") + ` = 'estorno'
		    OR ROUND(COALESCE(NULLIF(t.metadata->>'value', '')::numeric, 0) * 100)::bigint <= 0
		    OR ` + canonicalTransactionTypeSQL("t") + ` NOT IN ('receita', 'despesa', 'repasse', 'comissao')
		    OR ` + canonicalFinancialStatusSQL("t") + ` NOT IN ('PENDING', 'PAID', 'OVERDUE', 'REVERSED')
		  )
		ORDER BY t.id
		LIMIT 100`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query transaction signs and reversals: %w", err)
	}
	for rows.Next() {
		var state transactionReversalState
		if err := rows.Scan(
			&state.ID,
			&state.Type,
			&state.Status,
			&state.ValueCents,
			&state.OriginalID,
			&state.OriginalType,
			&state.OriginalStatus,
			&state.OriginalCents,
		); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan transaction signs and reversals: %w", err)
		}
		rep.Findings = append(rep.Findings, transactionConsistencyFindings(state)...)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate transaction signs and reversals: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close transaction signs and reversals rows: %w", err)
	}

	missingReversalQuery := `
		SELECT original.id::text
		FROM financial.transactions original
		WHERE original.tenant_id = $1
		  AND original.deleted_at IS NULL
		  AND ` + canonicalTransactionTypeSQL("original") + ` <> 'estorno'
		  AND ` + canonicalFinancialStatusSQL("original") + ` = 'REVERSED'
		  AND NOT EXISTS (
		    SELECT 1
		    FROM financial.transactions reversal
		    WHERE reversal.tenant_id = original.tenant_id
		      AND reversal.deleted_at IS NULL
		      AND ` + canonicalTransactionTypeSQL("reversal") + ` = 'estorno'
		      AND ` + canonicalFinancialStatusSQL("reversal") + ` = 'PAID'
		      AND ` + transactionReversalOriginalIDSQL("reversal") + ` = original.id::text
		  )
		ORDER BY original.id
		LIMIT 100`
	missingRows, err := r.db.QueryContext(ctx, missingReversalQuery, tenantID)
	if err != nil {
		return fmt.Errorf("query reversed transactions without counter-posting: %w", err)
	}
	for missingRows.Next() {
		var id string
		if err := missingRows.Scan(&id); err != nil {
			_ = missingRows.Close()
			return fmt.Errorf("scan reversed transaction without counter-posting: %w", err)
		}
		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   "ERROR",
			Category:   "consistency",
			EntityType: "Transaction",
			EntityID:   id,
			Message:    "Transaction REVERSED sem lançamento de estorno correspondente",
		})
	}
	if err := missingRows.Err(); err != nil {
		_ = missingRows.Close()
		return fmt.Errorf("iterate reversed transactions without counter-posting: %w", err)
	}
	if err := missingRows.Close(); err != nil {
		return fmt.Errorf("close reversed transactions without counter-posting rows: %w", err)
	}

	return r.findMonetarySignIssues(ctx, tenantID, rep)
}

func (r *reconciliationPostgresRepository) findMonetarySignIssues(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	query := `
		SELECT entity_type, id, cents
		FROM (
			SELECT 'Transfer' AS entity_type, t.id::text AS id,
			       ROUND(COALESCE(NULLIF(t.metadata->>'net_amount', '')::numeric, 0) * 100)::bigint AS cents
			FROM financial.transfers t
			WHERE t.tenant_id::text = $1::text AND t.deleted_at IS NULL
			  AND COALESCE(NULLIF(t.metadata->>'net_amount', '')::numeric, 0) < 0
			UNION ALL
			SELECT 'Withdrawal', w.id::text,
			       COALESCE(NULLIF(w.metadata->>'amount_cents', '')::bigint, 0)
			FROM financial.withdrawals w
			WHERE w.tenant_id::text = $1::text AND w.deleted_at IS NULL
			  AND COALESCE(NULLIF(w.metadata->>'amount_cents', '')::bigint, 0) < 0
			UNION ALL
			SELECT 'PayoutOrder', p.id::text,
			       COALESCE(NULLIF(p.metadata->>'amount_cents', '')::bigint, 0)
			FROM financial.payout_orders p
			WHERE p.tenant_id::text = $1::text AND p.deleted_at IS NULL
			  AND COALESCE(NULLIF(p.metadata->>'amount_cents', '')::bigint, 0) < 0
			UNION ALL
			SELECT 'Commission', c.id::text,
			       ROUND(COALESCE(NULLIF(c.metadata->>'amount', '')::numeric, 0) * 100)::bigint
			FROM financial.commissions c
			WHERE c.tenant_id::text = $1::text AND c.deleted_at IS NULL
			  AND COALESCE(NULLIF(c.metadata->>'amount', '')::numeric, 0) < 0
		) invalid_signs
		ORDER BY entity_type, id
		LIMIT 100`
	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query monetary signs: %w", err)
	}
	for rows.Next() {
		var entityType, id string
		var cents int64
		if err := rows.Scan(&entityType, &id, &cents); err != nil {
			_ = rows.Close()
			return fmt.Errorf("scan monetary signs: %w", err)
		}
		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   "ERROR",
			Category:   "consistency",
			EntityType: entityType,
			EntityID:   id,
			Reference:  fmt.Sprintf("value_cents=%d", cents),
			Message:    entityType + " com magnitude persistida negativa",
		})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("iterate monetary signs: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close monetary signs rows: %w", err)
	}
	return nil
}

// ── Consistency ────────────────────────────────────────────────────

func (r *reconciliationPostgresRepository) findConsistencyIssues(ctx context.Context, tenantID string, rep *report.ReconciliationReport) error {
	// Commission.status inconsistente com payout_order.status vinculado.
	// Canonicaliza title-case/Portuguese legacy values before comparing so a
	// legacy "Paid"/"Pago" row is equivalent to current "PAID".
	query := `
		SELECT c.id::text, ` + canonicalFinancialStatusSQL("c") + `, ` + canonicalFinancialStatusSQL("p") + `
		FROM financial.commissions c
		JOIN financial.payout_orders p
		  ON p.tenant_id::text = c.tenant_id::text
		 AND p.deleted_at IS NULL
		 AND (c.metadata->>'payout_order_id') = p.id::text
		WHERE c.tenant_id::text = $1::text
		  AND c.deleted_at IS NULL
		  AND ` + canonicalFinancialStatusSQL("p") + ` IN ('PAID', 'CANCELLED')
		  AND ` + canonicalFinancialStatusSQL("c") + ` <> ` + canonicalFinancialStatusSQL("p") + `
		LIMIT 100`

	rows, err := r.db.QueryContext(ctx, query, tenantID)
	if err != nil {
		return fmt.Errorf("query consistency issues: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id, commStatus, payoutStatus string
		if err := rows.Scan(&id, &commStatus, &payoutStatus); err != nil {
			return fmt.Errorf("scan consistency issues: %w", err)
		}
		rep.Findings = append(rep.Findings, report.ReconciliationFinding{
			Severity:   "ERROR",
			Category:   "consistency",
			EntityType: "Commission",
			EntityID:   id,
			Reference:  fmt.Sprintf("payout=%s commission=%s", payoutStatus, commStatus),
			Message:    fmt.Sprintf("Status cascade quebrado: payout_order %s mas commission ainda %s", payoutStatus, commStatus),
		})
		rep.Summary.Consistency++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate consistency issues: %w", err)
	}
	return nil
}
