package invitation

import "context"

type Repository interface {
	Create(ctx context.Context, invite *Invitation) error
	GetByToken(ctx context.Context, token string) (*Invitation, error)
	GetByEmailAndCompany(ctx context.Context, email, companyID string) (*Invitation, error)
	UpdateStatus(ctx context.Context, companyID, id, status string) error
	ListByCompany(ctx context.Context, companyID string) ([]*Invitation, error)
}

// ServiceRepository exposes the pre-authentication surface of the invitation
// store. The public invite link (GET /auth/invitations/:token) and its accept
// flow run before/outside a JWT for the target company, so the tenant-scoped
// pool has no matching tenant_id claim and the RLS policies deny every row.
// These run on the service-role pool (BYPASSRLS) instead.
type ServiceRepository interface {
	GetByTokenService(ctx context.Context, token string) (*Invitation, error)
	UpdateStatusService(ctx context.Context, companyID, id, status string) error
}
