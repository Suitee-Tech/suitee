package collection

import (
	"context"
	"time"
)

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go

type RepositoryInterface interface {
	Create(ctx context.Context, c *Collection) error
	CreateIdempotent(ctx context.Context, c *Collection) error
	GetByID(ctx context.Context, tenantID, id string) (*Collection, error)
	Update(ctx context.Context, c *Collection) error
	List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*Collection, int64, error)
	AddAction(ctx context.Context, tenantID, id string, action Action) error
	FindByChargeID(ctx context.Context, tenantID, chargeID string) (*Collection, error)
	FindByContract(ctx context.Context, contractID, tenantID string) ([]*Collection, error)
	FindActiveByDebtor(ctx context.Context, debtorID, tenantID string) (*Collection, error)
	GetStats(ctx context.Context, tenantID string) (*CollectionStats, error)
}

// VersionedRepository prevents stale read-modify-write operations from
// reopening a resolved case or erasing a concurrently appended charge.
type VersionedRepository interface {
	UpdateIfVersion(ctx context.Context, c *Collection, expectedUpdatedAt time.Time) (bool, error)
}
