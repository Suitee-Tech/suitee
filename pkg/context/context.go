package context

import (
	"context"
	"errors"
)

type contextKey string

const (
	// AccountIDKey is the key for account ID in context
	AccountIDKey contextKey = "account_id"
	// UserRoleKey is the key for user role in context
	UserRoleKey contextKey = "user_role"
	// UserTypeKey is the key for user type in context
	UserTypeKey contextKey = "user_type"
	// UserEmailKey is the key for user email in context
	UserEmailKey contextKey = "user_email"
	// UserNameKey is the key for user name in context
	UserNameKey contextKey = "user_name"
	// AvatarURLKey is the key for user avatar URL in context
	AvatarURLKey contextKey = "avatar_url"
	// TenantIDKey is the key for tenant ID in context
	TenantIDKey contextKey = "tenant_id"
	// PartnerTenantIDsKey is the key for partner tenant IDs in context
	PartnerTenantIDsKey contextKey = "partner_tenant_ids"
	// UserPermissionKey is the key for user permission in context
	UserPermissionKey contextKey = "user_permission"
	// FunctionalRoleKey is the key for functional role in context
	FunctionalRoleKey contextKey = "functional_role"
	// BaseRoleKey é o papel do catálogo em que o PERFIL DE ACESSO se baseia.
	//
	// Quando `functional_role` é um perfil personalizado (`cr_…`), este é o
	// papel de onde ele partiu (ex.: um perfil "Gestor Comercial" baseado em
	// `gestor`). Quem decide VISIBILIDADE precisa dele: sem isso, um usuário com
	// perfil personalizado ficava recortado pela própria carteira mesmo tendo as
	// mesmas permissões do gestor (incidente de 2026-09-21).
	BaseRoleKey contextKey = "base_role"
	// AuthTokenKey is the raw bearer token extracted from Authorization header
	AuthTokenKey contextKey = "auth_token"
	// RequestIDKey is the unique id assigned to one inbound HTTP request,
	// propagated to downstream calls and surfaced in every log line so
	// distributed traces and logs can be cross-referenced without joining
	// on user_id / tenant_id alone.
	RequestIDKey contextKey = "request_id"
)

// GetAccountID retrieves the account ID from the context
func GetAccountID(ctx context.Context) (string, error) {
	val := ctx.Value(AccountIDKey)
	if val == nil {
		return "", errors.New("account ID not found in context")
	}
	id, ok := val.(string)
	if !ok {
		return "", errors.New("account ID is not a string")
	}
	return id, nil
}

// GetUserRole retrieves the user role from the context
func GetUserRole(ctx context.Context) (string, error) {
	val := ctx.Value(UserRoleKey)
	if val == nil {
		return "", errors.New("user role not found in context")
	}
	role, ok := val.(string)
	if !ok {
		return "", errors.New("user role is not a string")
	}
	return role, nil
}

// GetUserPermission retrieves the user permission from the context
func GetUserPermission(ctx context.Context) (string, error) {
	val := ctx.Value(UserPermissionKey)
	if val == nil {
		return "", errors.New("user permission not found in context")
	}
	permission, ok := val.(string)
	if !ok {
		return "", errors.New("user permission is not a string")
	}
	return permission, nil
}

// GetFunctionalRole retrieves the functional role from the context
func GetFunctionalRole(ctx context.Context) (string, error) {
	val := ctx.Value(FunctionalRoleKey)
	if val == nil {
		return "", errors.New("functional role not found in context")
	}
	role, ok := val.(string)
	if !ok {
		return "", errors.New("functional role is not a string")
	}
	return role, nil
}

// WithAccountID adds the account ID to the context
func WithAccountID(ctx context.Context, accountID string) context.Context {
	return context.WithValue(ctx, AccountIDKey, accountID)
}

// WithUserRole adds the user role to the context
func WithUserRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, UserRoleKey, role)
}

// WithUserPermission adds the user permission to the context
func WithUserPermission(ctx context.Context, permission string) context.Context {
	return context.WithValue(ctx, UserPermissionKey, permission)
}

// WithFunctionalRole adds the functional role to the context
func WithFunctionalRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, FunctionalRoleKey, role)
}

// GetBaseRole devolve o papel base do perfil de acesso ("" quando o papel
// funcional já é um papel do catálogo).
func GetBaseRole(ctx context.Context) (string, error) {
	val := ctx.Value(BaseRoleKey)
	if val == nil {
		return "", errors.New("base role not found in context")
	}
	role, ok := val.(string)
	if !ok {
		return "", errors.New("base role is not a string")
	}
	return role, nil
}

// WithBaseRole adds the access profile's base role to the context
func WithBaseRole(ctx context.Context, role string) context.Context {
	return context.WithValue(ctx, BaseRoleKey, role)
}

// WithUserType adds the user type to the context
func WithUserType(ctx context.Context, userType string) context.Context {
	return context.WithValue(ctx, UserTypeKey, userType)
}

// WithUserEmail adds the user email to the context
func WithUserEmail(ctx context.Context, email string) context.Context {
	return context.WithValue(ctx, UserEmailKey, email)
}

// GetEmail retrieves the user email from the context
func GetEmail(ctx context.Context) (string, error) {
	val := ctx.Value(UserEmailKey)
	if val == nil {
		return "", errors.New("user email not found in context")
	}
	email, ok := val.(string)
	if !ok {
		return "", errors.New("user email is not a string")
	}
	return email, nil
}

// GetUserName retrieves the user name from the context
func GetUserName(ctx context.Context) (string, error) {
	val := ctx.Value(UserNameKey)
	if val == nil {
		return "", errors.New("user name not found in context")
	}
	name, ok := val.(string)
	if !ok {
		return "", errors.New("user name is not a string")
	}
	return name, nil
}

// WithUserName adds the user name to the context
func WithUserName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, UserNameKey, name)
}

// GetAvatarURL retrieves the avatar URL from the context
func GetAvatarURL(ctx context.Context) string {
	val := ctx.Value(AvatarURLKey)
	if val == nil {
		return ""
	}
	url, ok := val.(string)
	if !ok {
		return ""
	}
	return url
}

// WithAvatarURL adds the avatar URL to the context
func WithAvatarURL(ctx context.Context, avatarURL string) context.Context {
	return context.WithValue(ctx, AvatarURLKey, avatarURL)
}

// GetTenantID retrieves the tenant ID from the context
func GetTenantID(ctx context.Context) (string, error) {
	val := ctx.Value(TenantIDKey)
	if val == nil {
		return "", errors.New("tenant ID not found in context")
	}
	id, ok := val.(string)
	if !ok {
		return "", errors.New("tenant ID is not a string")
	}
	return id, nil
}

// WithTenantID adds the tenant ID to the context
func WithTenantID(ctx context.Context, tenantID string) context.Context {
	return context.WithValue(ctx, TenantIDKey, tenantID)
}

// GetPartnerTenantIDs retrieves the partner tenant IDs from the context
func GetPartnerTenantIDs(ctx context.Context) ([]string, error) {
	val := ctx.Value(PartnerTenantIDsKey)
	if val == nil {
		return nil, errors.New("partner tenant IDs not found in context")
	}
	ids, ok := val.([]string)
	if !ok {
		return nil, errors.New("partner tenant IDs is not a slice of strings")
	}
	return ids, nil
}

// WithPartnerTenantIDs adds the partner tenant IDs to the context
func WithPartnerTenantIDs(ctx context.Context, partnerTenantIDs []string) context.Context {
	return context.WithValue(ctx, PartnerTenantIDsKey, partnerTenantIDs)
}

func GetAuthToken(ctx context.Context) (string, error) {
	val := ctx.Value(AuthTokenKey)
	if val == nil {
		return "", errors.New("auth token not found in context")
	}
	token, ok := val.(string)
	if !ok {
		return "", errors.New("auth token is not a string")
	}
	return token, nil
}

func WithAuthToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, AuthTokenKey, token)
}

func RequireTenantID(ctx context.Context) (string, error) {
	return GetTenantID(ctx)
}

func RequireAccountID(ctx context.Context) (string, error) {
	return GetAccountID(ctx)
}

// GetRequestID returns the per-request id stamped by the request-id
// middleware. Returns empty string (no error) when missing — callers
// (notably the logger handler) should treat absence as benign and skip
// the request_id attribute.
func GetRequestID(ctx context.Context) string {
	val := ctx.Value(RequestIDKey)
	if val == nil {
		return ""
	}
	id, ok := val.(string)
	if !ok {
		return ""
	}
	return id
}

// WithRequestID stamps the request id on the context. Called by the
// request-id middleware once per inbound request; downstream code should
// only read with GetRequestID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}
