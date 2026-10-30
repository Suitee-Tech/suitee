package key

import "context"

type Repository interface {
	Create(ctx context.Context, k *Key) error
	// GetByID must be scoped by the caller's tenantID: implementations filter
	// by tenant_id and fail closed when tenantID is empty (auditoria A8/ops).
	GetByID(ctx context.Context, tenantID, id string) (*Key, error)
	Update(ctx context.Context, k *Key) error
	Delete(ctx context.Context, tenantID, id string) error
	List(ctx context.Context, tenantID string, propertyID string, status string, limit, offset int64) ([]*Key, int64, error)
}
