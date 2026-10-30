package account

import "context"

type Repository interface {
	Create(ctx context.Context, acc *Account) error
	GetByID(ctx context.Context, id string) (*Account, error)
	GetByIndividualEmail(ctx context.Context, email string) (*Account, error)
	GetByIndividualPhone(ctx context.Context, phone string) (*Account, error)
	Update(ctx context.Context, acc *Account) error
	Delete(ctx context.Context, id string) error
	GetCompaniesByMemberID(ctx context.Context, memberID string) ([]*Account, error)
	GetCompanyByTaxIDOrEmail(ctx context.Context, taxID, email string) (*Account, error)
}

// ServiceRepository exposes the pre-authentication, cross-tenant surface of the
// account store. Register/login/invite flows run before a JWT exists, so the
// tenant-scoped pool carries no claims and the RLS policies deny every row.
// These operations therefore run on the service-role pool (BYPASSRLS) — the
// legitimate owner of cross-tenant access by token/e-mail (see migration
// 20260919150000_rls_identity_tables.sql, "CAMINHOS PRÉ-AUTENTICAÇÃO").
type ServiceRepository interface {
	CreateService(ctx context.Context, acc *Account) error
	GetByIDService(ctx context.Context, id string) (*Account, error)
	GetByIndividualEmailService(ctx context.Context, email string) (*Account, error)
	GetByIndividualPhoneService(ctx context.Context, phone string) (*Account, error)
	UpdateService(ctx context.Context, acc *Account) error
	DeleteService(ctx context.Context, id string) error
}
