package group

import "context"

type Repository interface {
	Create(ctx context.Context, group *Group) error
	GetByID(ctx context.Context, id string) (*Group, error)
	Update(ctx context.Context, group *Group) error
	Delete(ctx context.Context, companyID, id string) error
	ListByCompanyID(ctx context.Context, companyID string) ([]*Group, error)
	GetByMemberAndCompany(ctx context.Context, accountID, companyID string) ([]*Group, error)
	RemoveMemberFromAllGroups(ctx context.Context, companyID, accountID string) error
}
