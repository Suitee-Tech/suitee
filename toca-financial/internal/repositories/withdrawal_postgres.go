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
	"github.com/oginoino/toca/toca-financial/internal/domain/withdrawal"
)

type withdrawalPostgresRepository struct {
	db *database.PostgresDB
}

func NewWithdrawalPostgresRepository(db *database.PostgresDB) withdrawal.Repository {
	return &withdrawalPostgresRepository{db: db}
}

func (r *withdrawalPostgresRepository) Create(ctx context.Context, w *withdrawal.Withdrawal) error {
	w.CreatedAt = time.Now()
	metadata, _ := json.Marshal(w)
	if strings.TrimSpace(w.ExternalID) != "" {
		query := `INSERT INTO financial.withdrawals (id, tenant_id, metadata) VALUES ($1, $2, $3)
		          ON CONFLICT (tenant_id, ((metadata->>'external_id')))
		          WHERE deleted_at IS NULL AND (metadata->>'external_id') IS NOT NULL
		          DO NOTHING
		          RETURNING created_at`
		row := r.db.QueryRow(ctx, query, w.ID, w.TenantID, string(metadata))
		if err := row.Scan(&w.CreatedAt); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return withdrawal.ErrWithdrawalAlreadyExists
			}
			return err
		}
		return nil
	}
	query := `INSERT INTO financial.withdrawals (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, w.ID, w.TenantID, string(metadata)).Scan(&w.CreatedAt)
}

func (r *withdrawalPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*withdrawal.Withdrawal, error) {
	var metadata []byte
	err := r.db.QueryRow(ctx,
		"SELECT metadata FROM financial.withdrawals WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL",
		id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("withdrawal lookup failed: %w", err)
	}
	var w withdrawal.Withdrawal
	if err := json.Unmarshal(metadata, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *withdrawalPostgresRepository) GetByExternalID(ctx context.Context, tenantID, externalID string) (*withdrawal.Withdrawal, error) {
	externalID = strings.TrimSpace(externalID)
	if externalID == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRow(ctx,
		"SELECT metadata FROM financial.withdrawals WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'external_id') = $2 LIMIT 1",
		tenantID, externalID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("withdrawal lookup by external_id failed: %w", err)
	}
	var w withdrawal.Withdrawal
	if err := json.Unmarshal(metadata, &w); err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *withdrawalPostgresRepository) Update(ctx context.Context, w *withdrawal.Withdrawal) error {
	w.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(w)
	_, err := r.db.Exec(ctx,
		"UPDATE financial.withdrawals SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3",
		string(metadata), w.ID, w.TenantID)
	return err
}

func (r *withdrawalPostgresRepository) List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*withdrawal.Withdrawal, int64, error) {
	if limit <= 0 {
		limit = 50
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
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.withdrawals "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM financial.withdrawals %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*withdrawal.Withdrawal, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var w withdrawal.Withdrawal
		if err := json.Unmarshal(metadata, &w); err != nil {
			return nil, 0, err
		}
		out = append(out, &w)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}
