package utility_account

import "context"

type Repository interface {
	Create(ctx context.Context, a *UtilityAccount) error
	GetByID(ctx context.Context, tenantID, id string) (*UtilityAccount, error)
	ListByContract(ctx context.Context, tenantID, contractID string) ([]*UtilityAccount, error)
	Update(ctx context.Context, a *UtilityAccount) error
	Delete(ctx context.Context, tenantID, id string) error
}
