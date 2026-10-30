package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
)

type managedBillPostgresRepository struct {
	db *database.PostgresDB
}

var _ managed_bill.Repository = (*managedBillPostgresRepository)(nil)

func NewManagedBillPostgresRepository(db *database.PostgresDB) managed_bill.Repository {
	return &managedBillPostgresRepository{db: db}
}

func (r *managedBillPostgresRepository) Create(ctx context.Context, bill *managed_bill.ManagedBill) error {
	if bill == nil {
		return fmt.Errorf("%w: bill is required", managed_bill.ErrInvalidInput)
	}
	evidence, history, paymentAttempts, err := encodeManagedBillDocuments(bill)
	if err != nil {
		return err
	}

	// PostgreSQL DATE is the canonical representation. Normalizing before the
	// insert also keeps an idempotent retry comparable with a reloaded row.
	bill.DueDate = civilDateIn(timeutil.LocationForTenant(ctx, bill.TenantID), bill.DueDate)
	query := `
		INSERT INTO financial.managed_bills (
			id, tenant_id, source_transfer_id, source_charge_id, charge_item_id,
			source_key, contract_id, property_id, owner_id, component, creditor,
			description, amount_cents, currency, due_date, competence_month,
			status, version, evidence, reserved_by, approved_by, paid_by,
			reconciled_by, cancelled_by, reserved_at, approved_at, paid_at,
			reconciled_at, cancelled_at, payment_reference,
			payment_journal_source_id, reconciliation_reference, cancel_reason,
			history, payment_attempts, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10, $11,
			$12, $13, 'BRL', $14, $15,
			$16, $17, $18, $19, $20, $21,
			$22, $23, $24, $25, $26,
			$27, $28, $29,
			$30, $31, $32,
			$33, $34, $35, $36
		)
		ON CONFLICT DO NOTHING
		RETURNING due_date, created_at, updated_at`
	err = r.db.QueryRow(ctx, query,
		bill.ID,
		bill.TenantID,
		bill.SourceTransferID,
		bill.SourceChargeID,
		bill.SourceChargeItemID,
		bill.SourceKey,
		bill.ContractID,
		bill.PropertyID,
		bill.OwnerID,
		bill.Component,
		bill.Creditor,
		bill.Description,
		bill.AmountCents,
		bill.DueDate,
		bill.CompetenceMonth,
		bill.Status,
		bill.Version,
		string(evidence),
		bill.ReservedBy,
		nullIfBlank(bill.ApprovedBy),
		nullIfBlank(bill.PaidBy),
		nullIfBlank(bill.ReconciledBy),
		nullIfBlank(bill.CancelledBy),
		bill.ReservedAt,
		bill.ApprovedAt,
		bill.PaidAt,
		bill.ReconciledAt,
		bill.CancelledAt,
		nullIfBlank(bill.PaymentReference),
		nullIfBlank(bill.PaymentJournalSourceID),
		nullIfBlank(bill.ReconciliationReference),
		nullIfBlank(bill.CancellationReason),
		string(history),
		string(paymentAttempts),
		bill.CreatedAt,
		bill.UpdatedAt,
	).Scan(&bill.DueDate, &bill.CreatedAt, &bill.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return managed_bill.ErrAlreadyExists
	}
	if err != nil {
		return fmt.Errorf("create managed bill: %w", err)
	}
	return nil
}

func (r *managedBillPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*managed_bill.ManagedBill, error) {
	return r.getOne(ctx, managedBillSelect+`
		WHERE tenant_id = $1 AND id = $2`, tenantID, id)
}

func (r *managedBillPostgresRepository) GetBySourceKey(ctx context.Context, tenantID, sourceKey string) (*managed_bill.ManagedBill, error) {
	bill, err := scanManagedBill(r.db.QueryRow(ctx, managedBillSelect+`
		WHERE tenant_id = $1 AND source_key = $2`, tenantID, sourceKey))
	if err == nil && bill != nil {
		bill.DueDate = civilDateFromDB(timeutil.LocationForTenant(ctx, tenantID), bill.DueDate)
	}
	if errors.Is(err, sql.ErrNoRows) {
		// Source-key lookup is an idempotency probe. Absence is expected before
		// the first command and remains indistinguishable across tenants.
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get managed bill by source key: %w", err)
	}
	return bill, nil
}

func (r *managedBillPostgresRepository) getOne(ctx context.Context, query string, args ...any) (*managed_bill.ManagedBill, error) {
	bill, err := scanManagedBill(r.db.QueryRow(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, managed_bill.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get managed bill: %w", err)
	}
	if bill != nil && len(args) > 0 {
		if tenantID, ok := args[0].(string); ok {
			bill.DueDate = civilDateFromDB(timeutil.LocationForTenant(ctx, tenantID), bill.DueDate)
		}
	}
	return bill, nil
}

func (r *managedBillPostgresRepository) SumActiveReservedCentsByTransfer(ctx context.Context, tenantID, transferID string) (int64, error) {
	var amountCents int64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(SUM(amount_cents), 0)::BIGINT
		FROM financial.managed_bills
		WHERE tenant_id = $1
		  AND source_transfer_id = $2
		  AND status IN ('RESERVED', 'APPROVED')`, tenantID, transferID).Scan(&amountCents)
	if err != nil {
		return 0, fmt.Errorf("sum active managed bill reservations: %w", err)
	}
	return amountCents, nil
}

func (r *managedBillPostgresRepository) UpdateIfVersion(ctx context.Context, bill *managed_bill.ManagedBill, expectedVersion int64) (bool, error) {
	if bill == nil {
		return false, fmt.Errorf("%w: bill is required", managed_bill.ErrInvalidInput)
	}
	if expectedVersion <= 0 || bill.Version != expectedVersion+1 {
		return false, managed_bill.ErrConcurrentModification
	}
	evidence, history, paymentAttempts, err := encodeManagedBillDocuments(bill)
	if err != nil {
		return false, err
	}

	var persistedVersion int64
	err = r.db.QueryRow(ctx, `
		UPDATE financial.managed_bills
		SET status = $1,
		    version = version + 1,
		    evidence = $2,
		    approved_by = $3,
		    paid_by = $4,
		    reconciled_by = $5,
		    cancelled_by = $6,
		    approved_at = $7,
		    paid_at = $8,
		    reconciled_at = $9,
		    cancelled_at = $10,
		    payment_reference = $11,
		    payment_journal_source_id = $12,
		    reconciliation_reference = $13,
		    cancel_reason = $14,
		    history = $15,
		    payment_attempts = $16,
		    updated_at = $17
		WHERE tenant_id = $18
		  AND id = $19
		  AND version = $20
		RETURNING version`,
		bill.Status,
		string(evidence),
		nullIfBlank(bill.ApprovedBy),
		nullIfBlank(bill.PaidBy),
		nullIfBlank(bill.ReconciledBy),
		nullIfBlank(bill.CancelledBy),
		bill.ApprovedAt,
		bill.PaidAt,
		bill.ReconciledAt,
		bill.CancelledAt,
		nullIfBlank(bill.PaymentReference),
		nullIfBlank(bill.PaymentJournalSourceID),
		nullIfBlank(bill.ReconciliationReference),
		nullIfBlank(bill.CancellationReason),
		string(history),
		string(paymentAttempts),
		bill.UpdatedAt,
		bill.TenantID,
		bill.ID,
		expectedVersion,
	).Scan(&persistedVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return false, managed_bill.ErrConcurrentModification
	}
	if err != nil {
		return false, fmt.Errorf("update managed bill: %w", err)
	}
	if persistedVersion != bill.Version {
		return false, managed_bill.ErrConcurrentModification
	}
	return true, nil
}

func (r *managedBillPostgresRepository) List(ctx context.Context, tenantID string, filter managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error) {
	where, args := managedBillListPredicate(tenantID, filter)

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM financial.managed_bills "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count managed bills: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	args = append(args, limit, offset)
	query := managedBillSelect + "\n" + where + fmt.Sprintf(
		" ORDER BY due_date ASC, created_at ASC, id ASC LIMIT $%d OFFSET $%d",
		len(args)-1,
		len(args),
	)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list managed bills: %w", err)
	}
	defer rows.Close()

	bills := make([]*managed_bill.ManagedBill, 0)
	for rows.Next() {
		bill, scanErr := scanManagedBill(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan managed bill list: %w", scanErr)
		}
		if bill != nil {
			bill.DueDate = civilDateFromDB(timeutil.LocationForTenant(ctx, tenantID), bill.DueDate)
		}
		bills = append(bills, bill)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate managed bill list: %w", err)
	}
	return bills, total, nil
}

func managedBillListPredicate(tenantID string, filter managed_bill.Filter) (string, []any) {
	clauses := []string{"tenant_id = $1"}
	args := []any{tenantID}
	appendFilter := func(column string, value any) {
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf("%s = $%d", column, len(args)))
	}
	if value := strings.TrimSpace(filter.SourceTransferID); value != "" {
		appendFilter("source_transfer_id", value)
	}
	if value := strings.TrimSpace(filter.ContractID); value != "" {
		appendFilter("contract_id", value)
	}
	if value := strings.TrimSpace(filter.PropertyID); value != "" {
		appendFilter("property_id", value)
	}
	if value := strings.TrimSpace(filter.OwnerID); value != "" {
		appendFilter("owner_id", value)
	}
	if value := strings.TrimSpace(filter.CompetenceMonth); value != "" {
		appendFilter("competence_month", value)
	}
	if filter.Status != "" {
		appendFilter("status", filter.Status)
	}
	if value := strings.TrimSpace(filter.Search); value != "" {
		pattern := "%" + strings.NewReplacer(
			`\`, `\\`,
			`%`, `\%`,
			`_`, `\_`,
		).Replace(value) + "%"
		args = append(args, pattern)
		placeholder := fmt.Sprintf("$%d", len(args))
		searchColumns := []string{
			"component", "creditor", "description", "contract_id",
			"property_id", "competence_month", "status",
		}
		searchClauses := make([]string, 0, len(searchColumns))
		for _, column := range searchColumns {
			searchClauses = append(searchClauses, column+" ILIKE "+placeholder+" ESCAPE '\\'")
		}
		clauses = append(clauses, "("+strings.Join(searchClauses, " OR ")+")")
	}
	return "WHERE " + strings.Join(clauses, " AND "), args
}

const managedBillSelect = `SELECT
	id::TEXT,
	tenant_id::TEXT,
	source_key,
	source_transfer_id::TEXT,
	source_charge_id::TEXT,
	charge_item_id::TEXT,
	contract_id,
	property_id,
	owner_id,
	component,
	creditor,
	description,
	amount_cents,
	due_date,
	competence_month,
	status,
	version,
	reserved_by,
	reserved_at,
	approved_by,
	approved_at,
	paid_by,
	paid_at,
	reconciled_by,
	reconciled_at,
	cancelled_by,
	cancelled_at,
	payment_reference,
	payment_journal_source_id,
	reconciliation_reference,
	cancel_reason,
	evidence,
	history,
	payment_attempts,
	created_at,
	updated_at
FROM financial.managed_bills`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanManagedBill(row rowScanner) (*managed_bill.ManagedBill, error) {
	bill := &managed_bill.ManagedBill{}
	var (
		status                  string
		approvedBy              sql.NullString
		approvedAt              sql.NullTime
		paidBy                  sql.NullString
		paidAt                  sql.NullTime
		reconciledBy            sql.NullString
		reconciledAt            sql.NullTime
		cancelledBy             sql.NullString
		cancelledAt             sql.NullTime
		paymentReference        sql.NullString
		paymentJournalSourceID  sql.NullString
		reconciliationReference sql.NullString
		cancellationReason      sql.NullString
		evidence                []byte
		history                 []byte
		paymentAttempts         []byte
	)
	err := row.Scan(
		&bill.ID,
		&bill.TenantID,
		&bill.SourceKey,
		&bill.SourceTransferID,
		&bill.SourceChargeID,
		&bill.SourceChargeItemID,
		&bill.ContractID,
		&bill.PropertyID,
		&bill.OwnerID,
		&bill.Component,
		&bill.Creditor,
		&bill.Description,
		&bill.AmountCents,
		&bill.DueDate,
		&bill.CompetenceMonth,
		&status,
		&bill.Version,
		&bill.ReservedBy,
		&bill.ReservedAt,
		&approvedBy,
		&approvedAt,
		&paidBy,
		&paidAt,
		&reconciledBy,
		&reconciledAt,
		&cancelledBy,
		&cancelledAt,
		&paymentReference,
		&paymentJournalSourceID,
		&reconciliationReference,
		&cancellationReason,
		&evidence,
		&history,
		&paymentAttempts,
		&bill.CreatedAt,
		&bill.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	bill.Status = managed_bill.Status(status)
	bill.ApprovedBy, bill.ApprovedAt = nullableString(approvedBy), nullableTime(approvedAt)
	bill.PaidBy, bill.PaidAt = nullableString(paidBy), nullableTime(paidAt)
	bill.ReconciledBy, bill.ReconciledAt = nullableString(reconciledBy), nullableTime(reconciledAt)
	bill.CancelledBy, bill.CancelledAt = nullableString(cancelledBy), nullableTime(cancelledAt)
	bill.PaymentReference = nullableString(paymentReference)
	bill.PaymentJournalSourceID = nullableString(paymentJournalSourceID)
	bill.ReconciliationReference = nullableString(reconciliationReference)
	bill.CancellationReason = nullableString(cancellationReason)
	if err := json.Unmarshal(evidence, &bill.Evidence); err != nil {
		return nil, fmt.Errorf("decode managed bill evidence: %w", err)
	}
	if err := json.Unmarshal(history, &bill.History); err != nil {
		return nil, fmt.Errorf("decode managed bill history: %w", err)
	}
	if err := json.Unmarshal(paymentAttempts, &bill.PaymentAttempts); err != nil {
		return nil, fmt.Errorf("decode managed bill payment attempts: %w", err)
	}
	if bill.PaymentAttempts == nil {
		bill.PaymentAttempts = []managed_bill.PaymentAttempt{}
	}
	return bill, nil
}

func encodeManagedBillDocuments(bill *managed_bill.ManagedBill) ([]byte, []byte, []byte, error) {
	evidence := bill.Evidence
	if evidence == nil {
		evidence = []managed_bill.Evidence{}
	}
	history := bill.History
	if history == nil {
		history = []managed_bill.HistoryEntry{}
	}
	paymentAttempts := bill.PaymentAttempts
	if paymentAttempts == nil {
		paymentAttempts = []managed_bill.PaymentAttempt{}
	}
	evidencePayload, err := json.Marshal(evidence)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode managed bill evidence: %w", err)
	}
	historyPayload, err := json.Marshal(history)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode managed bill history: %w", err)
	}
	paymentAttemptsPayload, err := json.Marshal(paymentAttempts)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode managed bill payment attempts: %w", err)
	}
	return evidencePayload, historyPayload, paymentAttemptsPayload, nil
}

// civilDate normaliza para a meia-noite do DIA CIVIL no fuso configurado.
//
// Antes chamava-se dateOnlyUTC e fazia `value.UTC()` antes de ler ano/mês/dia:
// um vencimento às 21h em Brasília (00h UTC do dia seguinte) era gravado com a
// data do DIA SEGUINTE. O nome antigo estava no call site, então a mudança de
// nome é intencional — quem ler o código não deve reintroduzir o `.UTC()`.
func civilDate(value time.Time) time.Time {
	return timeutil.StartOfDay(value)
}

// civilDateIn é a variante por tenant: o vencimento é um dia civil no fuso da
// imobiliária, não no fuso do processo.
func civilDateIn(loc *time.Location, value time.Time) time.Time {
	return timeutil.StartOfDayIn(loc, value)
}

// civilDateFromDB reinterpreta o valor lido da coluna DATE (meia-noite UTC)
// como a DATA CIVIL do tenant — o inverso de civilDateIn, aplicado no write.
//
// Sem isso a releitura desloca o vencimento em um dia: DATE 2026-10-09 volta
// como 2026-10-09T00:00Z e, convertido para America/Sao_Paulo, cai no dia
// civil 2026-10-08, enquanto o mesmo vencimento no metadata do negócio
// (2026-10-10T00:00Z) cai em 2026-10-09. A comparação de identidade do
// managed bill acusava "different economic data" em toda re-afirmação
// idempotente do mesmo bill.
func civilDateFromDB(loc *time.Location, value time.Time) time.Time {
	if value.IsZero() {
		return value
	}
	year, month, day := value.Date()
	if loc == nil {
		loc = timeutil.Location()
	}
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func nullIfBlank(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	at := value.Time.UTC()
	return &at
}
