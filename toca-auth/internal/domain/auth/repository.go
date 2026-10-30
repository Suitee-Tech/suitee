package auth

import "context"

type Repository interface {
	Create(ctx context.Context, cred *Credential) error
	GetByEmail(ctx context.Context, email string) (*Credential, error)
	GetByAccountID(ctx context.Context, accountID string) (*Credential, error)
	UpdatePassword(ctx context.Context, email, passwordHash string) error
	VerifyEmail(ctx context.Context, email string) error
}

// ServiceRepository exposes the pre-authentication surface of the credential
// store. Login/OTP/invite lookups by e-mail happen before a JWT exists, so the
// tenant-scoped pool has no claims and the RLS policies deny every row; these
// run on the service-role pool (BYPASSRLS) instead.
type ServiceRepository interface {
	CreateService(ctx context.Context, cred *Credential) error
	GetByEmailService(ctx context.Context, email string) (*Credential, error)
	GetByAccountIDService(ctx context.Context, accountID string) (*Credential, error)
}

type PasswordResetRepository interface {
	Create(ctx context.Context, reset *PasswordResetToken) error
	GetByToken(ctx context.Context, token string) (*PasswordResetToken, error)
	MarkAsUsed(ctx context.Context, token string) error
}
