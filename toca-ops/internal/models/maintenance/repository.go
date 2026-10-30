package maintenance

import "context"

type Repository interface {
	Create(ctx context.Context, ticket *MaintenanceTicket) error
	// GetByID must be scoped by the caller's tenantID: implementations filter
	// by tenant_id and fail closed when tenantID is empty (auditoria A8/ops).
	GetByID(ctx context.Context, tenantID, id string) (*MaintenanceTicket, error)
	Update(ctx context.Context, ticket *MaintenanceTicket) error
	Delete(ctx context.Context, tenantID, id string) error
	List(ctx context.Context, tenantID string, status string, requesterLeadID string, rentalID string, limit, offset int64) ([]*MaintenanceTicket, int64, error)
	ListByProperty(ctx context.Context, tenantID, propertyID string, limit, offset int64) ([]*MaintenanceTicket, int64, error)
}
