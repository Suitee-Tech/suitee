package integration

import (
	"context"
	"errors"
	"time"
)

// ErrNotFound é devolvido quando a integração não existe, está soft-deleted ou
// pertence a OUTRO tenant. É deliberadamente o mesmo erro nos três casos: o
// handler traduz para 404 e um chamador de outro tenant não consegue
// distinguir "não existe" de "existe, mas não é seu" (C4).
var ErrNotFound = errors.New("integration not found")

type Repository interface {
	Create(ctx context.Context, integ *PortalIntegration) error

	// GetByID busca a integração escopada pelo tenant do chamador:
	// WHERE id = $1 AND tenant_id = $2. Tenant vazio falha fechado.
	GetByID(ctx context.Context, tenantID, id string) (*PortalIntegration, error)

	// GetByIDAnyTenant é a escotilha EXPLÍCITA e documentada do webhook
	// público (C5), que não tem tenant no contexto — o tenant é derivado do
	// próprio registro carregado. Nunca usar em rota autenticada.
	GetByIDAnyTenant(ctx context.Context, id string) (*PortalIntegration, error)

	// GetByFeedToken é o caminho do feed público (GET /portals/feed/:token),
	// sem tenant por design: o token é o segredo.
	GetByFeedToken(ctx context.Context, token string) (*PortalIntegration, error)

	// GetByTenantAndPortal devolve (nil, nil) quando não existe integração do
	// tenant para o portal — é a checagem de duplicidade do Create, não um erro.
	GetByTenantAndPortal(ctx context.Context, tenantID string, portal PortalType) (*PortalIntegration, error)

	// Update grava somente se a linha ainda pertencer a tenantID e o agregado
	// também: WHERE id = $1 AND tenant_id = $2. 0 linhas afetadas = ErrNotFound.
	Update(ctx context.Context, tenantID string, integ *PortalIntegration) error

	// Delete faz soft-delete somente se a linha pertencer a tenantID.
	// 0 linhas afetadas = ErrNotFound.
	Delete(ctx context.Context, tenantID, id string) error

	ListByTenant(ctx context.Context, tenantID string) ([]*PortalIntegration, error)
	ListActive(ctx context.Context) ([]*PortalIntegration, error)
}

type FeedCacheRepository interface {
	Get(ctx context.Context, integrationID string) (*FeedCache, error)
	Set(ctx context.Context, cache *FeedCache) error
}

type FeedCache struct {
	IntegrationID string
	Content       []byte
	UpdatedAt     time.Time
}
