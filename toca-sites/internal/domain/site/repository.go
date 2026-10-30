package site

import (
	"context"
	"time"
)

// SiteRepository isola o CRUD do registro da vitrine. Toda leitura de painel
// carrega o tenant explicitamente; a única escotilha cross-tenant é
// FindBySlug, usada pela resolução de Host do site público.
type SiteRepository interface {
	GetByTenant(ctx context.Context, tenantID string) (*Site, error)
	// FindBySlug resolve o site pelo `{marca}` do Host. Devolve ErrNotFound
	// para slug inexistente e ErrSiteDisabled para site desligado.
	FindBySlug(ctx context.Context, slug string) (*Site, error)
	Create(ctx context.Context, s *Site) error
	Update(ctx context.Context, s *Site) error
	// SlugTaken devolve true quando OUTRO tenant já usa o slug.
	SlugTaken(ctx context.Context, tenantID, slug string) (bool, error)
	SetPayloadTenantID(ctx context.Context, tenantID, siteID, payloadTenantID string) error

	ListPages(ctx context.Context, tenantID, siteID string) ([]Page, error)
	GetPage(ctx context.Context, tenantID, siteID string, key PageKey) (*Page, error)
	UpsertPages(ctx context.Context, tenantID, siteID string, pages []Page) error
}

// ListingRepository guarda a projeção local do anúncio. A fonte continua sendo
// core.properties; esta tabela existe para SEO/URL canônica e para amarrar
// views/leads a um listing Payload.
type ListingRepository interface {
	Upsert(ctx context.Context, l *Listing) (*Listing, error)
	GetByProperty(ctx context.Context, tenantID, propertyID string) (*Listing, error)
	// SeoSlugTaken devolve true quando outro imóvel do mesmo tenant já ocupa o
	// slug SEO. A URL canônica precisa ser estável e única por vitrine.
	SeoSlugTaken(ctx context.Context, tenantID, seoSlug, propertyID string) (bool, error)
	MarkUnpublished(ctx context.Context, tenantID, propertyID string) (*Listing, error)
	SetPayloadDocID(ctx context.Context, tenantID, listingID, payloadDocID string) error
	// GetPublishedForPublic devolve o listing de um site que esteja publicado.
	// Usado para validar beacon/formulário: nada de aceitar property_id de
	// outro tenant.
	GetPublishedForPublic(ctx context.Context, tenantID, siteID, propertyID string) (*PublicListing, error)
	CountPublished(ctx context.Context, tenantID string) (int64, error)
	ListByTenant(ctx context.Context, tenantID string, limit, offset int64) ([]Listing, error)
}

// EngagementRepository persiste views/leads e calcula os agregados do painel.
type EngagementRepository interface {
	CreateView(ctx context.Context, v *ListingView) error
	StatsForListing(ctx context.Context, tenantID, propertyID string, now time.Time) (*ListingStats, error)
	StatsForSite(ctx context.Context, tenantID string, now time.Time) (*SiteStats, error)
	CreateLead(ctx context.Context, l *Lead) error
	UpdateLead(ctx context.Context, l *Lead) error
	// FindDuplicateLead procura um lead do MESMO tenant/imóvel com o mesmo
	// e-mail/telefone dentro da janela de deduplicação.
	FindDuplicateLead(ctx context.Context, tenantID, propertyID, email, phone string, since time.Time) (*Lead, error)
	ListLeads(ctx context.Context, tenantID string, limit, offset int64) ([]Lead, int64, error)
}
