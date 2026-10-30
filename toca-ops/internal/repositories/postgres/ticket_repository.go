package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ops/internal/models/ticket"
)

type ticketRepository struct {
	db *database.PostgresDB
}

func NewTicketRepository(db *database.PostgresDB) ticket.Repository {
	return &ticketRepository{db: db}
}

func (r *ticketRepository) Create(ctx context.Context, t *ticket.Ticket) error {
	if t.ID == "" {
		t.ID = uuid.NewString()
	}
	t.CreatedAt = time.Now()
	metadata, _ := json.Marshal(t)
	query := `INSERT INTO ops.tickets (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, t.ID, t.TenantID, string(metadata)).Scan(&t.CreatedAt)
}

func (r *ticketRepository) GetByID(ctx context.Context, tenantID, id string) (*ticket.Ticket, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM ops.tickets WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("ticket not found: %w", err)
	}
	var t ticket.Ticket
	_ = json.Unmarshal(metadata, &t)
	return &t, nil
}

func (r *ticketRepository) Update(ctx context.Context, t *ticket.Ticket) error {
	t.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(t)
	_, err := r.db.ExecContext(ctx, "UPDATE ops.tickets SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), t.ID, t.TenantID)
	return err
}

func (r *ticketRepository) List(ctx context.Context, tenantID string, filters ticket.ListFilters) ([]*ticket.Ticket, int64, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if filters.Status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, filters.Status)
		argN++
	}
	if filters.Type != "" {
		where = append(where, fmt.Sprintf("(metadata->>'type') = $%d", argN))
		args = append(args, filters.Type)
		argN++
	}
	if filters.AssigneeID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'assigned_to_id') = $%d", argN))
		args = append(args, filters.AssigneeID)
		argN++
	}
	if filters.RequesterAccountID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'requester_account_id') = $%d", argN))
		args = append(args, filters.RequesterAccountID)
		argN++
	}
	if filters.DealID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'deal_id') = $%d", argN))
		args = append(args, filters.DealID)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ops.tickets "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM ops.tickets %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*ticket.Ticket, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var t ticket.Ticket
		if err := json.Unmarshal(metadata, &t); err != nil {
			return nil, 0, err
		}
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *ticketRepository) ListByRequester(ctx context.Context, tenantID, accountID string) ([]*ticket.Ticket, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM ops.tickets
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'requester_account_id') = $2
		ORDER BY created_at DESC
	`, tenantID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*ticket.Ticket, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var t ticket.Ticket
		if err := json.Unmarshal(metadata, &t); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
