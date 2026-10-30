package settings

import "context"

type CompanySettingsRepository interface {
	GetByTenantID(ctx context.Context, tenantID string) (*CompanySettings, error)
	Upsert(ctx context.Context, settings *CompanySettings) error
}

type AppearanceRepository interface {
	GetByTenantID(ctx context.Context, tenantID string) (*AppearanceSettings, error)
	Upsert(ctx context.Context, settings *AppearanceSettings) error
}

type PermissionsRepository interface {
	GetByTenantID(ctx context.Context, tenantID string) (*RolePermissions, error)
	Upsert(ctx context.Context, settings *RolePermissions) error
}

type NotificationSettingsRepository interface {
	GetByTenantID(ctx context.Context, tenantID string) (*NotificationSettings, error)
	Upsert(ctx context.Context, settings *NotificationSettings) error
}

type WebhookRepository interface {
	Create(ctx context.Context, webhook *WebhookEndpoint) error
	GetByID(ctx context.Context, id string) (*WebhookEndpoint, error)
	ListByTenantID(ctx context.Context, tenantID string) ([]WebhookEndpoint, error)
	Update(ctx context.Context, webhook *WebhookEndpoint) error
	// Delete soft-deletes the webhook, but only when it belongs to tenantID
	// (P0/C6 — o tenant tem de entrar no WHERE junto com o id). Quando nenhuma
	// linha é afetada (webhook de outro tenant, inexistente ou já removido) o
	// contrato é devolver sql.ErrNoRows, para o handler responder 404 em vez de
	// um sucesso falso.
	Delete(ctx context.Context, tenantID, id string) error
}

type APIKeyRepository interface {
	Create(ctx context.Context, apiKey *APIKey) error
	ListByTenantID(ctx context.Context, tenantID string) ([]APIKey, error)
	// Revoke desativa a API key, mas apenas quando ela pertence a tenantID
	// (P0/C6). Nenhuma linha afetada => sql.ErrNoRows => 404.
	Revoke(ctx context.Context, tenantID, id string) error
}
