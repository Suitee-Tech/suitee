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
	"github.com/oginoino/toca/toca-financial/internal/domain/broker_advance"
)

type brokerAdvancePostgresRepository struct {
	db *database.PostgresDB
}

func NewBrokerAdvancePostgresRepository(db *database.PostgresDB) broker_advance.Repository {
	return &brokerAdvancePostgresRepository{db: db}
}

func (r *brokerAdvancePostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*broker_advance.Advance, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(id) == "" {
		return nil, nil
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata FROM financial.broker_advances
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
	`, id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("broker advance lookup failed: %w", err)
	}
	return decodeBrokerAdvance(metadata)
}

func (r *brokerAdvancePostgresRepository) ListByBroker(ctx context.Context, tenantID, brokerID string, filter broker_advance.ListFilter) ([]*broker_advance.Advance, error) {
	query := `
		SELECT metadata FROM financial.broker_advances
		 WHERE tenant_id = $1 AND deleted_at IS NULL AND metadata->>'broker_id' = $2`
	args := []any{tenantID, brokerID}
	if filter.OpenOnly {
		query += ` AND metadata->>'status' = 'OPEN'`
	} else if len(filter.Status) > 0 {
		placeholders := make([]string, 0, len(filter.Status))
		for _, status := range filter.Status {
			args = append(args, string(status))
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		query += ` AND metadata->>'status' IN (` + strings.Join(placeholders, ", ") + `)`
	}
	// FIFO: o adiantamento mais antigo é o primeiro a ser abatido.
	query += ` ORDER BY (metadata->>'granted_at')::timestamptz ASC, created_at ASC`
	return r.scanAdvances(ctx, query, args)
}

func (r *brokerAdvancePostgresRepository) List(ctx context.Context, tenantID string, filter broker_advance.ListFilter) ([]*broker_advance.Advance, error) {
	query := `SELECT metadata FROM financial.broker_advances WHERE tenant_id = $1 AND deleted_at IS NULL`
	args := []any{tenantID}
	if strings.TrimSpace(filter.BrokerID) != "" {
		args = append(args, strings.TrimSpace(filter.BrokerID))
		query += fmt.Sprintf(" AND metadata->>'broker_id' = $%d", len(args))
	}
	if filter.OpenOnly {
		query += ` AND metadata->>'status' = 'OPEN'`
	} else if len(filter.Status) > 0 {
		placeholders := make([]string, 0, len(filter.Status))
		for _, status := range filter.Status {
			args = append(args, string(status))
			placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		}
		query += ` AND metadata->>'status' IN (` + strings.Join(placeholders, ", ") + `)`
	}
	query += ` ORDER BY (metadata->>'granted_at')::timestamptz DESC, created_at DESC`
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	args = append(args, limit)
	query += fmt.Sprintf(" LIMIT $%d", len(args))
	if filter.Offset > 0 {
		args = append(args, filter.Offset)
		query += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	return r.scanAdvances(ctx, query, args)
}

func (r *brokerAdvancePostgresRepository) Balances(ctx context.Context, tenantID string) ([]broker_advance.BrokerBalance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata->>'broker_id' AS broker_id,
		       COUNT(*) FILTER (WHERE metadata->>'status' = 'OPEN')                                              AS open_count,
		       COALESCE(SUM((metadata->>'amount_cents')::bigint - (metadata->>'settled_cents')::bigint)
		                FILTER (WHERE metadata->>'status' = 'OPEN'), 0)                                          AS outstanding,
		       COALESCE(SUM((metadata->>'settled_cents')::bigint), 0)                                            AS settled
		  FROM financial.broker_advances
		 WHERE tenant_id = $1 AND deleted_at IS NULL AND metadata->>'broker_id' IS NOT NULL
		 GROUP BY 1
		 ORDER BY outstanding DESC`, tenantID)
	if err != nil {
		return nil, fmt.Errorf("broker advance balances: %w", err)
	}
	defer rows.Close()
	out := make([]broker_advance.BrokerBalance, 0)
	for rows.Next() {
		var entry broker_advance.BrokerBalance
		if err := rows.Scan(&entry.BrokerID, &entry.OpenCount, &entry.OutstandingCents, &entry.SettledCents); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

func (r *brokerAdvancePostgresRepository) Create(ctx context.Context, a *broker_advance.Advance) error {
	if a == nil {
		return errors.New("broker advance is required")
	}
	metadata, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode broker advance: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO financial.broker_advances (id, tenant_id, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
	`, a.ID, a.TenantID, string(metadata), a.CreatedAt, a.UpdatedAt)
	return err
}

// Update grava com CAS em updated_at: dois abatimentos concorrentes não
// sobrescrevem o saldo um do outro.
func (r *brokerAdvancePostgresRepository) Update(ctx context.Context, a *broker_advance.Advance, expectedUpdatedAt any) error {
	if a == nil {
		return errors.New("broker advance is required")
	}
	expected, _ := expectedUpdatedAt.(time.Time)
	metadata, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("encode broker advance: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE financial.broker_advances
		   SET metadata = $1, updated_at = $2
		 WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL AND updated_at = $5
	`, string(metadata), a.UpdatedAt, a.ID, a.TenantID, expected)
	if err != nil {
		return err
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("%w: %s", broker_advance.ErrNotFound, a.ID)
	}
	return nil
}

func (r *brokerAdvancePostgresRepository) scanAdvances(ctx context.Context, query string, args []any) ([]*broker_advance.Advance, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list broker advances: %w", err)
	}
	defer rows.Close()
	out := make([]*broker_advance.Advance, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		advance, err := decodeBrokerAdvance(metadata)
		if err != nil {
			return nil, err
		}
		out = append(out, advance)
	}
	return out, rows.Err()
}

func decodeBrokerAdvance(metadata []byte) (*broker_advance.Advance, error) {
	var a broker_advance.Advance
	if err := json.Unmarshal(metadata, &a); err != nil {
		return nil, fmt.Errorf("decode broker advance: %w", err)
	}
	return &a, nil
}
