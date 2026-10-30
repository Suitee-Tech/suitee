package document

import "context"

type RepositoryInterface interface {
	Create(ctx context.Context, d *Document) error
	// GetByID resolve o documento DENTRO do tenant informado.
	//
	// SECURITY (achado C11 da revisão de permissões de 2026-09-19): a query
	// filtrava só por `id`, então qualquer usuário com `documents:read` lia
	// metadados (nome, notas, tags, signatários, storage key) de documentos de
	// outro tenant. Não reintroduza um caminho sem tenant aqui.
	GetByID(ctx context.Context, tenantID, id string) (*Document, error)
	// Update grava no tenant do próprio documento (`d.TenantID`), como defesa
	// em profundidade do achado C11.
	Update(ctx context.Context, d *Document) error
	Delete(ctx context.Context, id, tenantID string) error

	// Query by contract (backward compat)
	ListByContract(ctx context.Context, contractID, tenantID string) ([]*Document, error)

	// Query by entity (generic linking)
	ListByEntity(ctx context.Context, entityID string, entityType EntityType, tenantID string) ([]*Document, error)

	// Full list with filters and pagination
	List(ctx context.Context, tenantID string, filters ListFilters) ([]*Document, int64, error)

	// Search by name/tags
	Search(ctx context.Context, tenantID, query string, limit, offset int64) ([]*Document, int64, error)

	// DistinctTags returns every tag value currently in use by the tenant.
	// Backs the Central de Documentos tag autocomplete / sidebar pinned
	// folders. Order is alphabetical to keep UI rendering stable; counts
	// are not exposed here because the page derives them client-side from
	// the visible list slice.
	DistinctTags(ctx context.Context, tenantID string) ([]string, error)
}

// ListFilters holds query parameters for listing documents
type ListFilters struct {
	ContractID string
	EntityID   string
	EntityType EntityType
	Category   Category
	Status     DocumentStatus
	Source     Source
	// Q is a case-insensitive substring match against the document name
	// (and notes, when populated). Empty string disables the filter.
	Q string
	// Tags is an OR set: a document matches when its tags[] array contains
	// any of the values supplied. Empty slice disables the filter.
	Tags   []string
	Limit  int64
	Offset int64
}
