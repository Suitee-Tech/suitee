package inspection

import "context"

type ListFilter struct {
	TenantID   string
	PropertyID string
	ContractID string
	Status     string
	Type       string
	Limit      int64
	Offset     int64
}

type Repository interface {
	Create(ctx context.Context, insp *Inspection) error
	// GetByID must be scoped by the caller's tenantID: implementations filter
	// by tenant_id and fail closed when tenantID is empty (auditoria C8).
	GetByID(ctx context.Context, tenantID, id string) (*Inspection, error)
	Update(ctx context.Context, insp *Inspection) error
	Delete(ctx context.Context, tenantID, id string) error
	List(ctx context.Context, f ListFilter) ([]*Inspection, int64, error)
}
