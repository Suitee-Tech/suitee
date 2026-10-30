package sync

import "context"

type Repository interface {
	Create(ctx context.Context, s *PortalSync) error
	Update(ctx context.Context, s *PortalSync) error
	// GetLatestByIntegrationID / ListByIntegrationID são escopados pelo tenant
	// do chamador: o histórico de sync de outro tenant não é legível (C4).
	GetLatestByIntegrationID(ctx context.Context, tenantID, integrationID string) (*PortalSync, error)
	ListByIntegrationID(ctx context.Context, tenantID, integrationID string, limit, offset int64) ([]*PortalSync, int64, error)
}
