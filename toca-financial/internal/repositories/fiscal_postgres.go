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
	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
)

type fiscalPostgresRepository struct {
	db *database.PostgresDB
}

func NewFiscalPostgresRepository(db *database.PostgresDB) *fiscalPostgresRepository {
	return &fiscalPostgresRepository{db: db}
}

func (r *fiscalPostgresRepository) CreateNfse(ctx context.Context, n *fiscal.Nfse) error {
	n.CreatedAt = time.Now()
	metadata, _ := json.Marshal(n)
	query := `INSERT INTO financial.nfse (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, n.ID, n.TenantID, string(metadata)).Scan(&n.CreatedAt)
}

func (r *fiscalPostgresRepository) UpdateNfse(ctx context.Context, n *fiscal.Nfse) error {
	n.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(n)
	_, err := r.db.ExecContext(ctx, "UPDATE financial.nfse SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), n.ID, n.TenantID)
	return err
}

func (r *fiscalPostgresRepository) ListNfse(ctx context.Context, tenantID, status, referenceMonth string, limit, offset int64) ([]*fiscal.Nfse, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

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
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM financial.nfse "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.nfse %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*fiscal.Nfse, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var n fiscal.Nfse
		if err := json.Unmarshal(metadata, &n); err != nil {
			return nil, 0, err
		}
		out = append(out, &n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *fiscalPostgresRepository) CreateDimob(ctx context.Context, d *fiscal.DimobReport) error {
	d.CreatedAt = time.Now()
	metadata, _ := json.Marshal(d)
	query := `INSERT INTO financial.dimob (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, d.ID, d.TenantID, string(metadata)).Scan(&d.CreatedAt)
}

func (r *fiscalPostgresRepository) UpdateDimob(ctx context.Context, d *fiscal.DimobReport) error {
	metadata, _ := json.Marshal(d)
	_, err := r.db.ExecContext(ctx, "UPDATE financial.dimob SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), d.ID, d.TenantID)
	return err
}

func (r *fiscalPostgresRepository) ListDimob(ctx context.Context, tenantID string) ([]*fiscal.DimobReport, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM financial.dimob
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*fiscal.DimobReport, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d fiscal.DimobReport
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *fiscalPostgresRepository) GetDimobByID(ctx context.Context, tenantID, id string) (*fiscal.DimobReport, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM financial.dimob WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("dimob not found: %w", err)
	}
	var d fiscal.DimobReport
	_ = json.Unmarshal(metadata, &d)
	return &d, nil
}

func (r *fiscalPostgresRepository) CreateInforme(ctx context.Context, i *fiscal.Informe) error {
	i.CreatedAt = time.Now()
	metadata, _ := json.Marshal(i)
	query := `INSERT INTO financial.informes (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, i.ID, i.TenantID, string(metadata)).Scan(&i.CreatedAt)
}

func (r *fiscalPostgresRepository) UpdateInforme(ctx context.Context, i *fiscal.Informe) error {
	metadata, _ := json.Marshal(i)
	_, err := r.db.ExecContext(ctx, "UPDATE financial.informes SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), i.ID, i.TenantID)
	return err
}

func (r *fiscalPostgresRepository) ListInformes(ctx context.Context, tenantID string, year int) ([]*fiscal.Informe, error) {
	args := []any{tenantID}
	query := `
		SELECT metadata
		FROM financial.informes
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`
	if year > 0 {
		query += " AND (metadata->>'year')::int = $2"
		args = append(args, year)
	}
	query += " ORDER BY created_at DESC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*fiscal.Informe, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var i fiscal.Informe
		if err := json.Unmarshal(metadata, &i); err != nil {
			return nil, err
		}
		out = append(out, &i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *fiscalPostgresRepository) GetInformeByID(ctx context.Context, tenantID, id string) (*fiscal.Informe, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM financial.informes WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("informe not found: %w", err)
	}
	var i fiscal.Informe
	_ = json.Unmarshal(metadata, &i)
	return &i, nil
}

func (r *fiscalPostgresRepository) ListDeadlines(ctx context.Context, tenantID string) ([]*fiscal.FiscalDeadline, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM financial.fiscal_deadlines
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY (metadata->>'due_date')::timestamptz ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*fiscal.FiscalDeadline, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d fiscal.FiscalDeadline
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *fiscalPostgresRepository) AddReportHistory(ctx context.Context, h *fiscal.ReportHistory) error {
	if h.GeneratedAt.IsZero() {
		h.GeneratedAt = time.Now()
	}
	metadata, err := json.Marshal(h)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO financial.report_history (id, tenant_id, metadata) VALUES ($1, $2, $3)`, h.ID, h.TenantID, string(metadata))
	return err
}

func (r *fiscalPostgresRepository) GetDimobByYear(ctx context.Context, tenantID string, year int) (*fiscal.DimobReport, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM financial.dimob
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'year')::int = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, year).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("dimob not found: %w", err)
	}
	var d fiscal.DimobReport
	if err := json.Unmarshal(metadata, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *fiscalPostgresRepository) SaveDimob(ctx context.Context, d *fiscal.DimobReport) error {
	return r.UpdateDimob(ctx, d)
}

func (r *fiscalPostgresRepository) SaveNfse(ctx context.Context, n *fiscal.Nfse) error {
	return r.UpdateNfse(ctx, n)
}

func (r *fiscalPostgresRepository) SaveInforme(ctx context.Context, i *fiscal.Informe) error {
	return r.UpdateInforme(ctx, i)
}

func (r *fiscalPostgresRepository) GetCalendar(ctx context.Context, tenantID string, year int) ([]*fiscal.FiscalDeadline, error) {
	args := []any{tenantID}
	query := `
		SELECT metadata
		FROM financial.fiscal_deadlines
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`
	if year > 0 {
		query += " AND EXTRACT(YEAR FROM (metadata->>'due_date')::timestamptz)::int = $2"
		args = append(args, year)
	}
	query += " ORDER BY (metadata->>'due_date')::timestamptz ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*fiscal.FiscalDeadline, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d fiscal.FiscalDeadline
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *fiscalPostgresRepository) SaveReport(ctx context.Context, h *fiscal.ReportHistory) error {
	return r.AddReportHistory(ctx, h)
}

func (r *fiscalPostgresRepository) ListReportHistory(ctx context.Context, tenantID string, filter fiscal.ReportHistoryFilter) ([]*fiscal.ReportHistory, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	// Filtros opcionais: tipo (DIMOB/NFSE/INFORME) e ano-calendário —
	// o ano é derivado do nome do relatório ("DIMOB 2025"), que é o
	// contrato atual da tabela; o filtro por created_at cobriria relatórios
	// de outro tipo com a mesma data.
	where := `tenant_id = $1 AND deleted_at IS NULL`
	args := []any{tenantID}
	if filter.Type != "" {
		args = append(args, filter.Type)
		where += fmt.Sprintf(" AND metadata->>'type' = $%d", len(args))
	}
	if filter.Year > 0 {
		args = append(args, fmt.Sprintf("%%%d", filter.Year))
		where += fmt.Sprintf(" AND metadata->>'name' LIKE $%d", len(args))
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM financial.report_history
		WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM financial.report_history
		WHERE `+where+`
		ORDER BY created_at DESC
		LIMIT $`+fmt.Sprintf("%d", len(args)-1)+` OFFSET $`+fmt.Sprintf("%d", len(args)), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []*fiscal.ReportHistory
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var h fiscal.ReportHistory
		if err := json.Unmarshal(metadata, &h); err != nil {
			return nil, 0, err
		}
		reports = append(reports, &h)
	}
	return reports, total, rows.Err()
}

// ArchiveReport faz o soft delete do registro do histórico (deleted_at):
// o arquivo gerado deixa de aparecer na listagem, mas a linha permanece
// para auditoria.
func (r *fiscalPostgresRepository) ArchiveReport(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE financial.report_history
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID)
	if err != nil {
		return err
	}
	if rows, err := res.RowsAffected(); err == nil && rows == 0 {
		return fmt.Errorf("report not found")
	}
	return nil
}

func (r *fiscalPostgresRepository) GetReportByID(ctx context.Context, tenantID, id string) (*fiscal.ReportHistory, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM financial.report_history
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("report not found: %w", err)
	}
	var h fiscal.ReportHistory
	if err := json.Unmarshal(metadata, &h); err != nil {
		return nil, err
	}
	return &h, nil
}

func (r *fiscalPostgresRepository) CountChargesByYear(ctx context.Context, tenantID string, year int, t string) (int, float64, error) {
	var cnt int
	var total float64
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)::int, COALESCE(SUM((metadata->>'total_amount')::numeric), 0)::float8
		FROM financial.charges
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND EXTRACT(YEAR FROM (metadata->>'due_date')::timestamptz)::int = $2
		  AND ($3 = '' OR (metadata->>'type') = $3)
	`, tenantID, year, t).Scan(&cnt, &total)
	if err != nil {
		return 0, 0, err
	}
	return cnt, total, nil
}
