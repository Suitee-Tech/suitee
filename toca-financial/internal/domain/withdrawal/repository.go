package withdrawal

import "context"

type Repository interface {
	Create(ctx context.Context, w *Withdrawal) error
	GetByID(ctx context.Context, tenantID, id string) (*Withdrawal, error)
	GetByExternalID(ctx context.Context, tenantID, externalID string) (*Withdrawal, error)
	Update(ctx context.Context, w *Withdrawal) error
	List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*Withdrawal, int64, error)
}
