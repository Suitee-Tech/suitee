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
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

type chargePostgresRepository struct {
	db *database.PostgresDB
}

func NewChargePostgresRepository(db *database.PostgresDB) charge.RepositoryInterface {
	return &chargePostgresRepository{db: db}
}

func (r *chargePostgresRepository) Create(ctx context.Context, c *charge.Charge) error {
	c.CreatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode charge metadata: %w", err)
	}
	// When SourceEventID is set, this charge originated from an upstream
	// broker event — use ON CONFLICT DO NOTHING against the partial unique
	// index uq_finance_charges_source_event_id so reentries collapse to a
	// single row. RETURNING is split so we can distinguish "inserted" from
	// "skipped" via the absence of a returned created_at.
	if c.SourceEventID != "" {
		query := `INSERT INTO financial.charges (id, tenant_id, metadata) VALUES ($1, $2, $3)
		          ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
		          WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
		          DO NOTHING
		          RETURNING created_at`
		row := r.db.QueryRow(ctx, query, c.ID, c.TenantID, string(metadata))
		if err := row.Scan(&c.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return charge.ErrChargeAlreadyExists
			}
			return err
		}
		return nil
	}
	query := `INSERT INTO financial.charges (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, c.ID, c.TenantID, string(metadata)).Scan(&c.CreatedAt)
}

func (r *chargePostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	return r.getByID(ctx, tenantID, id, false)
}

// GetByIDForUpdate provides the serialization point used by atomic invoice
// issuance. It deliberately remains an optional repository capability so
// lightweight charge.RepositoryInterface fakes do not need SQL locking APIs.
func (r *chargePostgresRepository) GetByIDForUpdate(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	return r.getByID(ctx, tenantID, id, true)
}

func (r *chargePostgresRepository) getByID(ctx context.Context, tenantID, id string, forUpdate bool) (*charge.Charge, error) {
	var metadata []byte
	query := "SELECT metadata FROM financial.charges WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL"
	if forUpdate {
		query += " FOR UPDATE"
	}
	err := r.db.QueryRow(ctx, query, id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, charge.ErrNotFound
		}
		return nil, fmt.Errorf("charge not found: %w", err)
	}
	var c charge.Charge
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, fmt.Errorf("decode charge metadata: %w", err)
	}
	return &c, nil
}

// GetBySourceEventID resolves the charge previously materialized from a
// cross-service event. Returns (nil, nil) when nothing matches — handlers
// use that to decide whether to create or to reconcile against existing.
func (r *chargePostgresRepository) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*charge.Charge, error) {
	if sourceEventID == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRow(
		ctx,
		"SELECT metadata FROM financial.charges WHERE tenant_id = $1 AND (metadata->>'source_event_id') = $2 AND deleted_at IS NULL LIMIT 1",
		tenantID, sourceEventID,
	).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("charge lookup by source_event_id failed: %w", err)
	}
	var c charge.Charge
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (r *chargePostgresRepository) Update(ctx context.Context, c *charge.Charge) error {
	c.UpdatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode charge metadata: %w", err)
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.charges SET metadata = $1 WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), c.ID, c.TenantID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return charge.ErrNotFound
	}
	return nil
}

// UpdateIfVersion performs the payment write as an optimistic compare-and-
// swap over metadata.updated_at. ChargeService retries a zero-row result,
// re-reading the newly committed payments first, so concurrent receipts are
// accumulated instead of one JSON document silently replacing the other.
func (r *chargePostgresRepository) UpdateIfVersion(ctx context.Context, c *charge.Charge, expected time.Time) (bool, error) {
	c.UpdatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return false, fmt.Errorf("encode charge metadata: %w", err)
	}
	// Passa o versão esperada como string UTC RFC3339 (em vez de time.Time)
	// para eliminar a ambiguidade de encoding do driver em comparação
	// `timestamptz = $param` — o mesmo instante pode ser serializado com
	// offset de fuso diferente e nunca casar, gerando "modified concurrently".
	expectedStr := ""
	if !expected.IsZero() {
		expectedStr = expected.UTC().Format(time.RFC3339Nano)
	}
	result, err := r.db.Exec(ctx, `UPDATE financial.charges
		SET metadata = $1
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND (
			NULLIF(metadata->>'updated_at', '')::timestamptz = NULLIF($4, '')::timestamptz
			OR ($5 AND NOT (metadata ? 'updated_at'))
		  )`, string(metadata), c.ID, c.TenantID, expectedStr, expected.IsZero())
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (r *chargePostgresRepository) List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if contractID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'contract_id') = $%d", argN))
		args = append(args, contractID)
		argN++
	}
	if status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, status)
		argN++
	}
	if referenceMonth != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') = $%d", argN))
		args = append(args, referenceMonth)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.charges "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.charges %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *chargePostgresRepository) ListByPartyIDs(ctx context.Context, tenantID string, partyIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	if len(partyIDs) == 0 {
		return []*charge.Charge{}, 0, nil
	}
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL", "(metadata->>'tenant_party_id') = ANY($2)"}
	args := []any{tenantID, partyIDs}
	argN := 3

	if status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, status)
		argN++
	}
	if referenceMonth != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') = $%d", argN))
		args = append(args, referenceMonth)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.charges "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.charges %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// ListByLeadIDs serve o portal cliente: lista charges em que algum
// leadID da conta do caller aparece tanto na posição customer
// (metadata->>'lead_id') quanto na posição owner (metadata->>'owner_id').
// Permite que:
//   - Inquilino veja charges que precisa pagar (lead_id ∈ leadIDs)
//   - Owner veja charges de imóveis seus (owner_id ∈ leadIDs)
//
// Filtros opcionais (status, referenceMonth) seguem o mesmo padrão dos
// outros listings. ARRAY[]::text[] vazio devolveria zero rows — short-
// circuita antes para evitar round-trip.
func (r *chargePostgresRepository) ListByLeadIDs(ctx context.Context, tenantID string, leadIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	if len(leadIDs) == 0 {
		return []*charge.Charge{}, 0, nil
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{
		"tenant_id = $1",
		"deleted_at IS NULL",
		"((metadata->>'lead_id') = ANY($2) OR (metadata->>'owner_id') = ANY($2))",
	}
	args := []any{tenantID, leadIDs}
	argN := 3

	if status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, status)
		argN++
	}
	if referenceMonth != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') = $%d", argN))
		args = append(args, referenceMonth)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.charges "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf(
		"SELECT metadata FROM financial.charges %s ORDER BY (metadata->>'due_date') DESC, created_at DESC LIMIT $%d OFFSET $%d",
		whereSQL, argN, argN+1,
	)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *chargePostgresRepository) Search(ctx context.Context, tenantID string, filter charge.SearchFilter) ([]*charge.Charge, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if v := strings.TrimSpace(filter.ContractID); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'contract_id') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.Status); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.ReferenceMonth); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'reference_month') = $%d", argN))
		args = append(args, v)
		argN++
	}
	if len(filter.PartyIDs) > 0 {
		where = append(where, fmt.Sprintf("(metadata->>'tenant_party_id') = ANY($%d)", argN))
		args = append(args, filter.PartyIDs)
		argN++
	}

	if v := strings.TrimSpace(filter.CustomerID); v != "" {
		where = append(where, fmt.Sprintf("((metadata->>'customer_id') = $%d OR (metadata->>'tenant_party_id') = $%d)", argN, argN))
		args = append(args, v)
		argN++
	}
	if v := strings.TrimSpace(filter.CustomerName); v != "" {
		where = append(where, fmt.Sprintf("(COALESCE(metadata->>'customer_name','') ILIKE $%d OR COALESCE(metadata->>'description','') ILIKE $%d)", argN, argN))
		args = append(args, "%"+v+"%")
		argN++
	}
	if v := strings.TrimSpace(filter.DealID); v != "" {
		where = append(where, fmt.Sprintf("((metadata->>'deal_id') = $%d OR (metadata->>'contract_id') = $%d)", argN, argN))
		args = append(args, v)
		argN++
	}

	amountExpr := "COALESCE((metadata->>'total_amount')::numeric, (metadata->>'amount')::numeric, 0)"
	if filter.MinAmount != nil {
		where = append(where, amountExpr+fmt.Sprintf(" >= $%d", argN))
		args = append(args, *filter.MinAmount)
		argN++
	}
	if filter.MaxAmount != nil {
		where = append(where, amountExpr+fmt.Sprintf(" <= $%d", argN))
		args = append(args, *filter.MaxAmount)
		argN++
	}

	if filter.DueFrom != nil {
		where = append(where, fmt.Sprintf("(metadata->>'due_date')::timestamptz >= $%d", argN))
		args = append(args, *filter.DueFrom)
		argN++
	}
	if filter.DueTo != nil {
		where = append(where, fmt.Sprintf("(metadata->>'due_date')::timestamptz <= $%d", argN))
		args = append(args, *filter.DueTo)
		argN++
	}

	if filter.PaidFrom != nil {
		where = append(where, fmt.Sprintf("NULLIF(metadata->>'paid_at','')::timestamptz >= $%d", argN))
		args = append(args, *filter.PaidFrom)
		argN++
	}
	if filter.PaidTo != nil {
		where = append(where, fmt.Sprintf("NULLIF(metadata->>'paid_at','')::timestamptz <= $%d", argN))
		args = append(args, *filter.PaidTo)
		argN++
	}

	if v := strings.TrimSpace(filter.PaymentMethod); v != "" {
		where = append(where, fmt.Sprintf("(metadata->>'payment_method') = $%d", argN))
		args = append(args, v)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.charges "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.charges %s ORDER BY created_at DESC, id DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, 0, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// chargeDueOverdueSQL devolve o predicado de "cobrança vencida" na regra do
// produto: a cobrança vence NO DIA e só conta atraso no dia SEGUINTE.
//
// Por que não `< NOW()`: a data de vencimento é gravada como meia-noite UTC
// (`2026-10-10T00:00:00Z` = "vence em 10/10", que é o que a tela mostra) e o
// contêiner roda em UTC. Comparar instantes marcava como vencida a cobrança do
// PRÓPRIO dia (e, no horário de Brasília, a partir das 21h da véspera). Agora a
// comparação é entre DATAS CIVIS: o dia do vencimento (em UTC, como foi gravado)
// contra o dia de hoje no fuso do tenant.
//
// `dueDateExpr` é a expressão da data no SQL (ex.: `metadata->>'due_date'`) e
// `tzParam` é o placeholder do nome do fuso (ex.: `$2`), resolvido com
// timeutil.TimezoneNameForTenant.
func chargeDueOverdueSQL(dueDateExpr, tzParam string) string {
	return fmt.Sprintf(
		"((%s)::timestamptz AT TIME ZONE 'UTC')::date < (NOW() AT TIME ZONE %s)::date",
		dueDateExpr, tzParam)
}

// chargeOverdueDaysSQL calcula os dias de atraso em dias civis (0 no dia do
// vencimento), usado pelo aging e pelas estatísticas.
func chargeOverdueDaysSQL(dueDateExpr, tzParam string) string {
	return fmt.Sprintf(
		"GREATEST(0, ((NOW() AT TIME ZONE %s)::date - ((%s)::timestamptz AT TIME ZONE 'UTC')::date))",
		tzParam, dueDateExpr)
}

func (r *chargePostgresRepository) ListOverdue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	tz := timeutil.TimezoneNameForTenant(ctx, tenantID)
	rows, err := r.db.Query(ctx, `
		SELECT metadata
		FROM financial.charges
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (
		    (metadata->>'status') = 'OVERDUE'
		    OR ((metadata->>'status') IN ('PENDING','PARTIALLY_PAID') AND `+chargeDueOverdueSQL("metadata->>'due_date'", "$2")+`)
		  )
		ORDER BY (metadata->>'due_date')::timestamptz ASC
	`, tenantID, tz)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *chargePostgresRepository) BulkUpdateStatus(ctx context.Context, tenantID string, ids []string, status charge.Status) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		UPDATE financial.charges
		SET metadata = jsonb_set(metadata, '{status}', to_jsonb($1::text), true),
		    updated_at = NOW()
		WHERE tenant_id = $2
		  AND deleted_at IS NULL
		  AND id::text = ANY($3)
	`, string(status), tenantID, ids)
	return err
}

func (r *chargePostgresRepository) ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*charge.Charge, error) {
	if len(ids) == 0 {
		return []*charge.Charge{}, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT metadata
		FROM financial.charges
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND id::text = ANY($2)
	`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0, len(ids))
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *chargePostgresRepository) ListPendingPastDue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	tz := timeutil.TimezoneNameForTenant(ctx, tenantID)
	rows, err := r.db.Query(ctx, `
		SELECT metadata
		FROM financial.charges
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'status') IN ('PENDING', 'PARTIALLY_PAID')
		  AND `+chargeDueOverdueSQL("metadata->>'due_date'", "$2")+`
		ORDER BY (metadata->>'due_date')::timestamptz ASC
	`, tenantID, tz)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *chargePostgresRepository) ListDistinctTenantIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT DISTINCT tenant_id::text
		FROM financial.charges
		WHERE deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// ListByDealAndCustomer lista cobranças de um deal para uma parte.
//
// SECURITY (achado A3 da revisão de permissões de 2026-09-19): a consulta
// filtrava só por deal + parte, SEM tenant — quem alcançasse a rota interna
// (chave compartilhada) lia cobranças de qualquer tenant informando um deal/parte.
// O tenant agora é obrigatório e vem do predicado SQL.
func (r *chargePostgresRepository) ListByDealAndCustomer(ctx context.Context, tenantID, dealID, accountID, role string) ([]*charge.Charge, error) {
	if strings.TrimSpace(tenantID) == "" || dealID == "" || accountID == "" {
		return []*charge.Charge{}, nil
	}

	whereRole := "(metadata->>'tenant_party_id') = $3"
	if strings.EqualFold(role, "owner") {
		whereRole = "(metadata->>'owner_id') = $3"
	}

	query := fmt.Sprintf(`
		SELECT metadata
		FROM financial.charges
		WHERE deleted_at IS NULL
		  AND tenant_id = $2
		  AND ((metadata->>'contract_id') = $1 OR (metadata->>'deal_id') = $1)
		  AND %s
		ORDER BY (metadata->>'due_date')::timestamptz DESC
	`, whereRole)

	rows, err := r.db.Query(ctx, query, dealID, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*charge.Charge, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var c charge.Charge
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *chargePostgresRepository) AgingReport(ctx context.Context, tenantID string) ([]charge.AgingBucket, error) {
	rows, err := r.db.Query(ctx, `
		WITH base AS (
			SELECT
				`+chargeOverdueDaysSQL("metadata->>'due_date'", "$2")+`::int AS days_overdue,
				-- Saldo LÍQUIDO: o aging somava o total cheio e ignorava o que já foi
				-- pago, inflando a faixa de quem pagou parcialmente (G31 do plano de
				-- inadimplência).
				GREATEST(0, COALESCE((metadata->>'total_amount')::numeric, 0)
					- COALESCE((metadata->>'paid_amount')::numeric, 0)) AS amount
			FROM financial.charges
			WHERE tenant_id = $1
			  AND deleted_at IS NULL
			  AND (metadata->>'status') IN ('OVERDUE','PENDING','PARTIALLY_PAID')
			  AND `+chargeDueOverdueSQL("metadata->>'due_date'", "$2")+`
		)
		SELECT
			CASE
				WHEN days_overdue BETWEEN 0 AND 30 THEN '0-30'
				WHEN days_overdue BETWEEN 31 AND 60 THEN '31-60'
				WHEN days_overdue BETWEEN 61 AND 90 THEN '61-90'
				ELSE '90+'
			END AS bucket,
			COUNT(1)::int AS cnt,
			COALESCE(SUM(amount), 0)::float8 AS total_debt
		FROM base
		GROUP BY 1
		ORDER BY 1
	`, tenantID, timeutil.TimezoneNameForTenant(ctx, tenantID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := map[string]*charge.AgingBucket{
		"0-30":  {Label: "0-30", MinDays: 0, MaxDays: 30},
		"31-60": {Label: "31-60", MinDays: 31, MaxDays: 60},
		"61-90": {Label: "61-90", MinDays: 61, MaxDays: 90},
		"90+":   {Label: "90+", MinDays: 91, MaxDays: -1},
	}

	for rows.Next() {
		var label string
		var cnt int
		var totalDebt float64
		if err := rows.Scan(&label, &cnt, &totalDebt); err != nil {
			return nil, err
		}
		if b, ok := buckets[label]; ok {
			b.Count = cnt
			b.TotalDebt = totalDebt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]charge.AgingBucket, 0, 4)
	out = append(out, *buckets["0-30"], *buckets["31-60"], *buckets["61-90"], *buckets["90+"])
	return out, nil
}

func (r *chargePostgresRepository) GetDelinquencyStats(ctx context.Context, tenantID string) (*charge.DelinquencyStats, error) {
	var stats charge.DelinquencyStats
	err := r.db.QueryRow(ctx, `
		WITH overdue AS (
			SELECT
				-- Saldo líquido do que já foi pago (G31).
				GREATEST(0, COALESCE((metadata->>'total_amount')::numeric, 0)
					- COALESCE((metadata->>'paid_amount')::numeric, 0)) AS amount,
				`+chargeOverdueDaysSQL("metadata->>'due_date'", "$2")+`::float8 AS days
			FROM financial.charges
			WHERE tenant_id = $1
			  AND deleted_at IS NULL
			  AND (metadata->>'status') IN ('OVERDUE','PENDING','PARTIALLY_PAID')
			  AND `+chargeDueOverdueSQL("metadata->>'due_date'", "$2")+`
		)
		SELECT
			COUNT(1)::int AS total_overdue,
			COALESCE(SUM(amount), 0)::float8 AS total_debt,
			COALESCE(AVG(amount), 0)::float8 AS avg_debt,
			COALESCE(AVG(days), 0)::float8 AS avg_days
		FROM overdue
	`, tenantID, timeutil.TimezoneNameForTenant(ctx, tenantID)).Scan(&stats.TotalOverdue, &stats.TotalDebtAmount, &stats.AverageDebtAmount, &stats.AverageDaysOverdue)
	if err != nil {
		return nil, err
	}
	stats.ActiveCollections = 0
	stats.RecoveryRate = 0
	return &stats, nil
}
