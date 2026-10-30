package lead

import "context"

type Repository interface {
	Create(ctx context.Context, pl *PortalLead) error
	Update(ctx context.Context, pl *PortalLead) error
	GetByPortalAndContact(ctx context.Context, portalID, email, phone string) (*PortalLead, error)
	List(ctx context.Context, tenantID, status, portalID string, limit, offset int64) ([]*PortalLead, int64, error)
	ListByTenant(ctx context.Context, tenantID string, limit, offset int64) ([]*PortalLead, int64, error)
	ListByIntegration(ctx context.Context, tenantID, integrationID string, limit, offset int64) ([]*PortalLead, int64, error)
	FindDuplicate(ctx context.Context, tenantID, portalID, email, phone, propertyID string) (*PortalLead, error)
	GetStats(ctx context.Context, tenantID string) (int64, map[string]int64, error)
}
