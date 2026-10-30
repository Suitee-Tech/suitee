package repositories

import (
	"context"

	"github.com/oginoino/toca/pkg/database"
)

// CrossTenantRepository exposes minimal "exists in tenant" checks against
// schemas owned by other services (CRM). Used by PropertyService to validate
// owner-lead and capturing-broker IDs supplied via the captação flow before
// persisting them on a Property — without this, a tenant could attach a
// foreign broker/lead by passing its id.
//
// The repo reads `crm.brokers` and `crm.leads` directly because in the pilot
// every service shares the same Postgres instance (separate schemas). When/if
// the modules split into distinct DBs, this can be swapped for an HTTP client
// (e.g. via `pkg/clients`) without changing callers.
type CrossTenantRepository interface {
	BrokerExists(ctx context.Context, tenantID, id string) (bool, error)
	LeadExists(ctx context.Context, tenantID, id string) (bool, error)
}

type crossPostgresRepository struct {
	db *database.PostgresDB
}

func NewCrossPostgresRepository(db *database.PostgresDB) CrossTenantRepository {
	return &crossPostgresRepository{db: db}
}

func (r *crossPostgresRepository) BrokerExists(ctx context.Context, tenantID, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM crm.brokers
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		)
	`, id, tenantID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (r *crossPostgresRepository) LeadExists(ctx context.Context, tenantID, id string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM crm.leads
			WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		)
	`, id, tenantID).Scan(&exists)
	if err != nil {
		return false, err
	}
	return exists, nil
}
