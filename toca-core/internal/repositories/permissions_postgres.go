package repositories

import (
	"context"
	"encoding/json"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/database"
)

type permissionsPostgresRepository struct {
	db *database.PostgresDB
}

func NewPermissionsPostgresRepository(db *database.PostgresDB) settings.PermissionsRepository {
	return &permissionsPostgresRepository{db: db}
}

func (r *permissionsPostgresRepository) GetByTenantID(ctx context.Context, tenantID string) (*settings.RolePermissions, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM core.role_permissions WHERE tenant_id = $1", tenantID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var s settings.RolePermissions
	_ = json.Unmarshal(metadata, &s)
	return &s, nil
}

func (r *permissionsPostgresRepository) Upsert(ctx context.Context, s *settings.RolePermissions) error {
	metadata, _ := json.Marshal(s)
	query := `
		INSERT INTO core.role_permissions (tenant_id, metadata) 
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET metadata = EXCLUDED.metadata
	`
	_, err := r.db.ExecContext(ctx, query, s.TenantID, string(metadata))
	return err
}
