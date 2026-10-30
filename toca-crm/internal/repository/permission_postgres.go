package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/oginoino/toca/pkg/database"
)

type permissionPostgresRepository struct {
	db *database.PostgresDB
}

func NewPermissionPostgresRepository(db *database.PostgresDB) *permissionPostgresRepository {
	return &permissionPostgresRepository{db: db}
}

func (r *permissionPostgresRepository) GetRolesMap(ctx context.Context, tenantID, requiredPermission string) (map[string]bool, error) {
	var metadata []byte
	// The permissions data resides in the core schema in postgres
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM core.role_permissions WHERE tenant_id = $1", tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("no permissions configured for tenant %s: %w", tenantID, err)
	}

	var doc struct {
		TenantID    string `json:"tenant_id"`
		Permissions []struct {
			ID    string          `json:"id"`
			Roles map[string]bool `json:"roles"`
		} `json:"permissions"`
	}

	if err := json.Unmarshal(metadata, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal permissions: %w", err)
	}

	for _, p := range doc.Permissions {
		if p.ID == requiredPermission {
			return p.Roles, nil
		}
	}

	return map[string]bool{}, nil
}
