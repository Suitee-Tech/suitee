package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/utility_account"
)

type utilityAccountPostgresRepository struct {
	db *database.PostgresDB
}

func NewUtilityAccountPostgresRepository(db *database.PostgresDB) utility_account.Repository {
	return &utilityAccountPostgresRepository{db: db}
}

func (r *utilityAccountPostgresRepository) Create(ctx context.Context, a *utility_account.UtilityAccount) error {
	a.CreatedAt = time.Now()
	a.UpdatedAt = a.CreatedAt
	metadata, err := json.Marshal(a)
	if err != nil {
		return err
	}
	query := `INSERT INTO financial.utility_accounts (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, a.ID, a.TenantID, string(metadata)).Scan(&a.CreatedAt)
}

func (r *utilityAccountPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*utility_account.UtilityAccount, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT metadata FROM financial.utility_accounts WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("utility_account not found: %w", err)
	}
	var a utility_account.UtilityAccount
	if err := json.Unmarshal(metadata, &a); err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *utilityAccountPostgresRepository) ListByContract(ctx context.Context, tenantID, contractID string) ([]*utility_account.UtilityAccount, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT metadata FROM financial.utility_accounts
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'contract_id') = $2
		ORDER BY created_at DESC`,
		tenantID, contractID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*utility_account.UtilityAccount, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var a utility_account.UtilityAccount
		if err := json.Unmarshal(metadata, &a); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (r *utilityAccountPostgresRepository) Update(ctx context.Context, a *utility_account.UtilityAccount) error {
	a.UpdatedAt = time.Now()
	metadata, err := json.Marshal(a)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE financial.utility_accounts SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL`,
		string(metadata), a.ID, a.TenantID,
	)
	return err
}

func (r *utilityAccountPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE financial.utility_accounts SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	)
	return err
}
