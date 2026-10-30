package repositories

import (
	"context"

	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	"github.com/oginoino/toca-core/internal/domain/core/property"
)

type PropertyRepository interface {
	Create(ctx context.Context, p *property.Property) error
	GetByID(ctx context.Context, id, tenantID string) (*property.Property, error)
	GetByIDs(ctx context.Context, ids []string, tenantID string) ([]*property.Property, error)
	GetAll(ctx context.Context, filter property.PropertyFilter) ([]*property.Property, int64, error)
	Update(ctx context.Context, p *property.Property) error
	// AppendImage appende uma imagem ao JSONB de forma atômica. Uploads
	// simultâneos não podem fazer read-modify-write do array inteiro.
	AppendImage(ctx context.Context, id, tenantID string, image property.Image) (*property.Property, error)
	Delete(ctx context.Context, id, tenantID string) error

	// ListSharedForPartner lists the properties owned by ownerTenantID that
	// are opted into the partnership portfolio and are NOT exclusive
	// captures. This DELIBERATELY crosses tenant isolation — it must only be
	// reached through a permission-checked partnership service method, never
	// from a generic tenant-scoped route.
	//
	// propertyTypes and regions come from the agreement's SharingConfig and
	// let the owner company narrow what it exposes — autonomy rules. Empty
	// slices mean "share all". Regions match city/neighborhood/state.
	ListSharedForPartner(ctx context.Context, ownerTenantID string, propertyTypes, regions []string) ([]*property.Property, error)
}

type PartnershipRepository interface {
	Create(ctx context.Context, p *partnership.Partnership) error
	GetByID(ctx context.Context, id string) (*partnership.Partnership, error)
	GetByRequesterID(ctx context.Context, requesterID string) ([]*partnership.Partnership, error)
	GetByTargetID(ctx context.Context, targetID string) ([]*partnership.Partnership, error)
	List(ctx context.Context, filter map[string]interface{}) ([]*partnership.Partnership, error)
	// P2/item 36: as escritas carregam o tenant do ATOR. A parceria liga dois
	// tenants (requester/target), então o predicado é "o ator é uma das pontas" —
	// antes a sentença filtrava só por id e a segurança vinha do GetByID + checagem
	// no serviço.
	Update(ctx context.Context, actorTenantID string, p *partnership.Partnership) error
	Delete(ctx context.Context, actorTenantID, id string) error
	AddAuditLog(ctx context.Context, actorTenantID, id string, log partnership.AuditLog) error
}
