package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type rouletteHistoryPostgresRepository struct {
	db *database.PostgresDB
}

func NewRouletteHistoryPostgresRepository(db *database.PostgresDB) *rouletteHistoryPostgresRepository {
	return &rouletteHistoryPostgresRepository{db: db}
}

func (r *rouletteHistoryPostgresRepository) Create(ctx context.Context, h *lead.RouletteHistory) error {
	if h.CreatedAt.IsZero() {
		h.CreatedAt = time.Now()
	}
	metadata, _ := json.Marshal(h)
	query := `INSERT INTO crm.roulette_history (id, tenant_id, metadata) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, h.ID, h.TenantID, string(metadata))
	return err
}

func (r *rouletteHistoryPostgresRepository) List(ctx context.Context, tenantID string, limit, offset int64) ([]*lead.RouletteHistory, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM crm.roulette_history
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM crm.roulette_history
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*lead.RouletteHistory, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var h lead.RouletteHistory
		if err := json.Unmarshal(metadata, &h); err != nil {
			return nil, 0, err
		}
		out = append(out, &h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}
