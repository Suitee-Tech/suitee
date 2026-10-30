package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
)

type maintenanceRepository struct {
	db *database.PostgresDB
}

func NewMaintenanceRepository(db *database.PostgresDB) maintenance.Repository {
	return &maintenanceRepository{db: db}
}

func (r *maintenanceRepository) Create(ctx context.Context, m *maintenance.MaintenanceTicket) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	m.CreatedAt = time.Now()
	metadata, _ := json.Marshal(m)
	query := `INSERT INTO ops.maintenance (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, m.ID, m.TenantID, string(metadata)).Scan(&m.CreatedAt)
}

// maintenanceGetByIDQuery é tenant-scoped de propósito (auditoria A8/ops): a
// busca por id sem tenant entregava chamado de manutenção de outro tenant —
// incluindo anexos privados, que o handler converte em URL pré-assinada.
const maintenanceGetByIDQuery = `SELECT metadata FROM ops.maintenance WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`

// GetByID devolve o chamado apenas quando ele pertence a tenantID e falha
// fechado quando o tenant está vazio.
func (r *maintenanceRepository) GetByID(ctx context.Context, tenantID, id string) (*maintenance.MaintenanceTicket, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("maintenance not found: tenant context required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("maintenance not found: id required")
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, maintenanceGetByIDQuery, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("maintenance not found: %w", err)
	}
	var m maintenance.MaintenanceTicket
	_ = json.Unmarshal(metadata, &m)
	return &m, nil
}

func (r *maintenanceRepository) Update(ctx context.Context, m *maintenance.MaintenanceTicket) error {
	m.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(m)
	_, err := r.db.ExecContext(ctx, "UPDATE ops.maintenance SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), m.ID, m.TenantID)
	return err
}

func (r *maintenanceRepository) Delete(ctx context.Context, tenantID, id string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fmt.Errorf("maintenance not found: tenant context required")
	}
	// P2/item 36: predicado de tenant na própria escrita.
	_, err := r.db.ExecContext(ctx, "UPDATE ops.maintenance SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}

func (r *maintenanceRepository) List(ctx context.Context, tenantID, status string, requesterLeadID string, rentalID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
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
	if requesterLeadID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'requester_lead_id') = $%d", argN))
		args = append(args, requesterLeadID)
		argN++
	}
	if rentalID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'rental_id') = $%d", argN))
		args = append(args, rentalID)
		argN++
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ops.maintenance "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM ops.maintenance %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*maintenance.MaintenanceTicket, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var m maintenance.MaintenanceTicket
		if err := json.Unmarshal(metadata, &m); err != nil {
			return nil, 0, err
		}
		out = append(out, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *maintenanceRepository) ListByProperty(ctx context.Context, tenantID, propertyID string, limit, offset int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL", "(metadata->>'property_id') = $2"}
	args := []any{tenantID, propertyID}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ops.maintenance "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM ops.maintenance
		`+whereSQL+`
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, propertyID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*maintenance.MaintenanceTicket, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var m maintenance.MaintenanceTicket
		if err := json.Unmarshal(metadata, &m); err != nil {
			return nil, 0, err
		}
		out = append(out, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
