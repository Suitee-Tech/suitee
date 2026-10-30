package transaction

import (
	"context"
	"time"
)

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go

type SearchFilter struct {
	ContractID  string
	Category    string
	Type        string
	MinAmount   *float64
	MaxAmount   *float64
	DateFrom    *time.Time
	DateTo      *time.Time
	Status      string
	Description string
	Limit       int64
	Offset      int64
}

type RepositoryInterface interface {
	Create(ctx context.Context, t *Transaction) error
	CreateBatch(ctx context.Context, txs []*Transaction) error
	GetByID(ctx context.Context, tenantID, id string) (*Transaction, error)
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*Transaction, error)
	Update(ctx context.Context, t *Transaction) error
	Delete(ctx context.Context, id, tenantID string) error
	List(ctx context.Context, tenantID string, contractID string, limit, offset int64) ([]*Transaction, int64, error)
	Search(ctx context.Context, tenantID string, filter SearchFilter) ([]*Transaction, int64, error)
}
