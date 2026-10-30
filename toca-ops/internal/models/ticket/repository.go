package ticket

import "context"

type ListFilters struct {
	Status             string
	Type               string
	AssigneeID         string
	RequesterAccountID string
	DealID             string
	Limit              int64
	Offset             int64
}

type Repository interface {
	Create(ctx context.Context, t *Ticket) error
	GetByID(ctx context.Context, tenantID, id string) (*Ticket, error)
	Update(ctx context.Context, t *Ticket) error
	List(ctx context.Context, tenantID string, filters ListFilters) ([]*Ticket, int64, error)
	ListByRequester(ctx context.Context, tenantID, accountID string) ([]*Ticket, error)
}
