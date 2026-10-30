package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
)

type supplierRepository struct {
	db *database.PostgresDB
}

func NewSupplierRepository(db *database.PostgresDB) supplier.Repository {
	return &supplierRepository{db: db}
}

func (r *supplierRepository) Create(ctx context.Context, v *supplier.Supplier) error {
	if v.ID == "" {
		v.ID = uuid.NewString()
	}
	now := time.Now()
	v.CreatedAt = now
	v.UpdatedAt = now
	metadata, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal supplier: %w", err)
	}
	query := `INSERT INTO ops.suppliers (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	if err := r.db.QueryRowContext(ctx, query, v.ID, v.TenantID, string(metadata)).Scan(&v.CreatedAt); err != nil {
		return fmt.Errorf("insert supplier: %w", err)
	}
	return nil
}

func (r *supplierRepository) GetByID(ctx context.Context, tenantID, id string) (*supplier.Supplier, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata FROM ops.suppliers
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, supplier.ErrNotFound
		}
		return nil, fmt.Errorf("get supplier: %w", err)
	}
	var v supplier.Supplier
	if err := json.Unmarshal(metadata, &v); err != nil {
		return nil, fmt.Errorf("decode supplier: %w", err)
	}
	return &v, nil
}

func (r *supplierRepository) Update(ctx context.Context, v *supplier.Supplier) error {
	v.UpdatedAt = time.Now()
	metadata, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal supplier: %w", err)
	}
	commandTag, err := r.db.ExecContext(ctx, `
		UPDATE ops.suppliers SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(metadata), v.ID, v.TenantID)
	if err != nil {
		return fmt.Errorf("update supplier: %w", err)
	}
	if rows := affectedRows(commandTag); rows == 0 {
		return supplier.ErrNotFound
	}
	return nil
}

// Delete é lógico: o histórico de chamados continua apontando para o
// prestador, que só sai da seleção.
func (r *supplierRepository) Delete(ctx context.Context, tenantID, id string) error {
	commandTag, err := r.db.ExecContext(ctx, `
		UPDATE ops.suppliers SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID)
	if err != nil {
		return fmt.Errorf("delete supplier: %w", err)
	}
	if rows := affectedRows(commandTag); rows == 0 {
		return supplier.ErrNotFound
	}
	return nil
}

func (r *supplierRepository) List(ctx context.Context, tenantID string, filter supplier.Filter) ([]*supplier.Supplier, int64, error) {
	conditions := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if query := strings.TrimSpace(filter.Query); query != "" {
		conditions = append(conditions, fmt.Sprintf(`(
			metadata->>'name' ILIKE $%d OR
			metadata->>'service_type' ILIKE $%d OR
			metadata->>'email' ILIKE $%d OR
			metadata->>'phone' ILIKE $%d OR
			metadata->>'notes' ILIKE $%d
		)`, argN, argN, argN, argN, argN))
		args = append(args, "%"+query+"%")
		argN++
	}
	if serviceType := strings.TrimSpace(filter.ServiceType); serviceType != "" {
		conditions = append(conditions, fmt.Sprintf("metadata->>'service_type' = $%d", argN))
		args = append(args, serviceType)
		argN++
	}
	if filter.ActiveOnly {
		conditions = append(conditions, "(metadata->>'active')::boolean IS TRUE")
	}

	where := strings.Join(conditions, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM ops.suppliers WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count suppliers: %w", err)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args = append(args, limit, filter.Offset)
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT metadata FROM ops.suppliers
		WHERE %s
		ORDER BY lower(metadata->>'name') ASC, created_at ASC
		LIMIT $%d OFFSET $%d
	`, where, argN, argN+1), args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list suppliers: %w", err)
	}
	defer rows.Close()

	out := make([]*supplier.Supplier, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, fmt.Errorf("scan supplier: %w", err)
		}
		var v supplier.Supplier
		if err := json.Unmarshal(metadata, &v); err != nil {
			return nil, 0, fmt.Errorf("decode supplier: %w", err)
		}
		out = append(out, &v)
	}
	return out, total, rows.Err()
}

// FindDuplicate reutiliza o cadastro existente quando documento ou contato já
// pertencem a um prestador do mesmo tenant.
func (r *supplierRepository) FindDuplicate(ctx context.Context, tenantID, document, phone, email string) (*supplier.Supplier, error) {
	document = strings.TrimSpace(document)
	phone = strings.TrimSpace(phone)
	email = strings.TrimSpace(email)
	if document == "" && phone == "" && email == "" {
		return nil, nil
	}

	conditions := make([]string, 0, 3)
	args := []any{tenantID}
	argN := 2
	if document != "" {
		conditions = append(conditions, fmt.Sprintf("regexp_replace(metadata->>'document', '[^0-9]', '', 'g') = $%d", argN))
		args = append(args, document)
		argN++
	}
	if phone != "" {
		// Compatibilidade com cadastros antigos, com máscara ou sem o país.
		conditions = append(conditions, fmt.Sprintf("regexp_replace(metadata->>'phone', '[^0-9]', '', 'g') IN ($%d, $%d)", argN, argN+1))
		args = append(args, strings.TrimPrefix(phone, "+55"), strings.TrimPrefix(phone, "+"))
		argN += 2
	}
	if email != "" {
		conditions = append(conditions, fmt.Sprintf("lower(trim(metadata->>'email')) = lower($%d)", argN))
		args = append(args, email)
		// argN não avança mais: email é o último predicado opcional do WHERE.
	}

	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata FROM ops.suppliers
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (`+strings.Join(conditions, " OR ")+`)
		ORDER BY created_at ASC
		LIMIT 1
	`, args...).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find supplier duplicate: %w", err)
	}
	var v supplier.Supplier
	if err := json.Unmarshal(metadata, &v); err != nil {
		return nil, fmt.Errorf("decode supplier: %w", err)
	}
	return &v, nil
}

// affectedRows lê o command tag tratando o erro do driver como "sem linhas".
func affectedRows(tag sql.Result) int64 {
	if tag == nil {
		return 0
	}
	rows, err := tag.RowsAffected()
	if err != nil {
		return 0
	}
	return rows
}
