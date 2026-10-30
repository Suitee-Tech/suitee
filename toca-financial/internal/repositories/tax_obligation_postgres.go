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
	"github.com/oginoino/toca/toca-financial/internal/domain/tax_obligation"
)

// taxObligationPostgresRepository guarda a obrigação em financial.tax_obligations
// (id/tenant_id/metadata). As colunas indexadas do metadata — kind, competence,
// status, due_date, amount_cents — são projetadas no JSON para os filtros e para
// o índice único por (tenant, tipo, competência não cancelada).
type taxObligationPostgresRepository struct {
	db *database.PostgresDB
}

func NewTaxObligationPostgresRepository(db *database.PostgresDB) tax_obligation.Repository {
	return &taxObligationPostgresRepository{db: db}
}

func (r *taxObligationPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*tax_obligation.Obligation, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(id) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		  FROM financial.tax_obligations
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("tax obligation lookup failed: %w", err)
	}
	return decodeTaxObligation(metadata)
}

func (r *taxObligationPostgresRepository) GetByKindCompetence(ctx context.Context, tenantID string, kind tax_obligation.Kind, competence string) (*tax_obligation.Obligation, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(competence) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		  FROM financial.tax_obligations
		 WHERE tenant_id = $1
		   AND deleted_at IS NULL
		   AND metadata->>'kind' = $2
		   AND metadata->>'competence' = $3
		   AND COALESCE(metadata->>'status', '') <> 'CANCELLED'
		 ORDER BY created_at DESC
		 LIMIT 1
	`, tenantID, string(kind), competence).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("tax obligation lookup failed: %w", err)
	}
	return decodeTaxObligation(metadata)
}

func (r *taxObligationPostgresRepository) List(ctx context.Context, tenantID string, filter tax_obligation.ListFilter) ([]*tax_obligation.Obligation, error) {
	query, args := taxObligationFilterQuery(tenantID, filter, true)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tax obligations: %w", err)
	}
	defer rows.Close()

	out := make([]*tax_obligation.Obligation, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		obligation, err := decodeTaxObligation(metadata)
		if err != nil {
			return nil, err
		}
		out = append(out, obligation)
	}
	return out, rows.Err()
}

func (r *taxObligationPostgresRepository) Totals(ctx context.Context, tenantID string, filter tax_obligation.ListFilter) (*tax_obligation.Totals, error) {
	// Os totais ignoram paginação: a tela mostra "total a recolher", não a soma
	// da página visível. A MESMA query filtrada alimenta o agregado e o retrato
	// por tipo, para os dois nunca discordarem sobre o que está sendo somado.
	filteredQuery, args := taxObligationFilterQuery(tenantID, filter, false)

	query := `
		SELECT
		    COUNT(*) FILTER (WHERE metadata->>'status' = 'OPEN')                                                   AS open_count,
		    COALESCE(SUM((metadata->>'amount_cents')::bigint) FILTER (WHERE metadata->>'status' = 'OPEN'), 0)      AS open_amount,
		    COUNT(*) FILTER (WHERE metadata->>'status' = 'OPEN' AND (metadata->>'due_date')::timestamptz < now())  AS overdue_count,
		    COALESCE(SUM((metadata->>'amount_cents')::bigint) FILTER (WHERE metadata->>'status' = 'OPEN' AND (metadata->>'due_date')::timestamptz < now()), 0) AS overdue_amount,
		    COUNT(*) FILTER (WHERE metadata->>'status' = 'PAID')                                                   AS paid_count,
		    COALESCE(SUM((metadata->>'amount_cents')::bigint) FILTER (WHERE metadata->>'status' = 'PAID'), 0)      AS paid_amount
		FROM (` + filteredQuery + `) AS filtered`

	totals := &tax_obligation.Totals{ByKind: map[tax_obligation.Kind]tax_obligation.KindTotals{}}
	if err := r.db.QueryRowContext(ctx, query, args...).Scan(
		&totals.OpenCount, &totals.OpenAmountCents, &totals.OverdueCount,
		&totals.OverdueAmountCents, &totals.PaidCount, &totals.PaidAmountCents,
	); err != nil {
		return nil, fmt.Errorf("tax obligation totals: %w", err)
	}

	byKindQuery := `
		SELECT metadata->>'kind' AS kind,
		       COUNT(*) FILTER (WHERE metadata->>'status' = 'OPEN')                                               AS open_count,
		       COALESCE(SUM((metadata->>'amount_cents')::bigint) FILTER (WHERE metadata->>'status' = 'OPEN'), 0)  AS open_amount,
		       COALESCE(SUM((metadata->>'amount_cents')::bigint) FILTER (WHERE metadata->>'status' = 'PAID'), 0)  AS paid_amount
		  FROM (` + filteredQuery + `) AS filtered
		 GROUP BY 1`
	rows, err := r.db.QueryContext(ctx, byKindQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("tax obligation totals by kind: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var entry tax_obligation.KindTotals
		if err := rows.Scan(&kind, &entry.OpenCount, &entry.OpenAmountCents, &entry.PaidAmountCents); err != nil {
			return nil, err
		}
		totals.ByKind[tax_obligation.Kind(kind)] = entry
	}
	return totals, rows.Err()
}

func (r *taxObligationPostgresRepository) Create(ctx context.Context, o *tax_obligation.Obligation) error {
	if o == nil {
		return errors.New("tax obligation is required")
	}
	metadata, err := json.Marshal(o)
	if err != nil {
		return fmt.Errorf("encode tax obligation: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO financial.tax_obligations (id, tenant_id, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, o.ID, o.TenantID, string(metadata), o.CreatedAt, o.UpdatedAt)
	return err
}

// Update grava com CAS em updated_at: uma baixa concorrente não é sobrescrita
// por uma recomputação que leu o estado anterior.
func (r *taxObligationPostgresRepository) Update(ctx context.Context, o *tax_obligation.Obligation, expectedUpdatedAt time.Time) error {
	if o == nil {
		return errors.New("tax obligation is required")
	}
	metadata, err := json.Marshal(o)
	if err != nil {
		return fmt.Errorf("encode tax obligation: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE financial.tax_obligations
		   SET metadata = $1, updated_at = $2
		 WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL AND updated_at = $5
	`, string(metadata), o.UpdatedAt, o.ID, o.TenantID, expectedUpdatedAt)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: tax obligation %s", tax_obligation.ErrNotFound, o.ID)
	}
	return nil
}

func decodeTaxObligation(metadata []byte) (*tax_obligation.Obligation, error) {
	var o tax_obligation.Obligation
	if err := json.Unmarshal(metadata, &o); err != nil {
		return nil, fmt.Errorf("decode tax obligation: %w", err)
	}
	return &o, nil
}

// taxObligationFilterQuery devolve a query (com ordem/paginação quando pedido) e
// os argumentos na ordem em que aparecem.
func taxObligationFilterQuery(tenantID string, filter tax_obligation.ListFilter, paginate bool) (string, []any) {
	args := []any{}
	conditions := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args = append(args, tenantID)
	next := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("$%d", len(args))
	}
	if len(filter.Status) > 0 {
		placeholders := make([]string, 0, len(filter.Status))
		for _, status := range filter.Status {
			placeholders = append(placeholders, next(string(status)))
		}
		conditions = append(conditions, "metadata->>'status' IN ("+strings.Join(placeholders, ", ")+")")
	}
	if len(filter.Kind) > 0 {
		placeholders := make([]string, 0, len(filter.Kind))
		for _, kind := range filter.Kind {
			placeholders = append(placeholders, next(string(kind)))
		}
		conditions = append(conditions, "metadata->>'kind' IN ("+strings.Join(placeholders, ", ")+")")
	}
	if strings.TrimSpace(filter.Competence) != "" {
		conditions = append(conditions, "metadata->>'competence' = "+next(strings.TrimSpace(filter.Competence)))
	}
	if filter.DueBefore != nil {
		conditions = append(conditions, "(metadata->>'due_date')::timestamptz <= "+next(filter.DueBefore.UTC()))
	}
	if filter.OverdueOnly {
		conditions = append(conditions, "metadata->>'status' = 'OPEN' AND (metadata->>'due_date')::timestamptz < now()")
	}
	query := "SELECT metadata FROM financial.tax_obligations WHERE " + strings.Join(conditions, " AND ")
	if paginate {
		query += " ORDER BY (metadata->>'due_date')::timestamptz ASC, created_at ASC"
		limit := filter.Limit
		if limit <= 0 || limit > 200 {
			limit = 100
		}
		query += " LIMIT " + next(limit)
		if filter.Offset > 0 {
			query += " OFFSET " + next(filter.Offset)
		}
	}
	return query, args
}
