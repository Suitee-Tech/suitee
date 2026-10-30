package contract

import (
	"context"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

var ErrConcurrentModification = errors.New("O contrato foi alterado. Atualize a página antes de tentar novamente.")

type Repository interface {
	Create(ctx context.Context, contract *Contract) error
	GetByID(ctx context.Context, tenantID, id string) (*Contract, error)
	GetByDealID(ctx context.Context, tenantID, dealID string) ([]*Contract, error)
	// ListByPropertyID returns every contract (any status, any tenant) that
	// references the given PropertyID. Used by the property.updated
	// subscriber to refresh the PropertyTitle snapshot stored on each
	// contract metadata. Cross-tenant by design — callers run on the
	// service pool (BYPASSRLS).
	ListByPropertyID(ctx context.Context, propertyID string) ([]*Contract, error)
	Update(ctx context.Context, contract *Contract) error
	UpdateIfUnchanged(ctx context.Context, contract *Contract, expectedUpdatedAt time.Time) error
	// UpdateInTx mirrors Update but joins the caller's transaction so
	// the contract Update + outbox event INSERT land or roll back as a
	// unit (audit P0.1 follow-up).
	UpdateInTx(ctx context.Context, tx *sqlx.Tx, contract *Contract) error
	Delete(ctx context.Context, id, tenantID string) error
	List(ctx context.Context, tenantID string, status *Status) ([]*Contract, error)
	// ListAutoAdjustDue returns every contract (cross-tenant) whose
	// auto_adjust flag is true and whose next_adjustment is <= today (ISO
	// YYYY-MM-DD comparison). Powers the daily auto-readjust scheduler;
	// uses the service pool because the sweep needs to cover every tenant.
	ListAutoAdjustDue(ctx context.Context, today time.Time) ([]*Contract, error)
	// ListMissingCRMSnapshot returns every non-deleted contract whose
	// denormalized CRM snapshot is incomplete — at least one of client,
	// property_id, customer_id, or value is empty/zero. Cross-tenant; the
	// snapshot backfill cmd iterates all tenants in one pass and reloads
	// each contract's deal in the proper tenant scope before persisting.
	// Optional flag tenantID, when non-empty, narrows the sweep to that
	// tenant only (supports per-tenant rollout).
	ListMissingCRMSnapshot(ctx context.Context, tenantID string) ([]*Contract, error)
}

type TemplateRepository interface {
	CreateTemplate(ctx context.Context, tpl *ContractTemplate) error
	GetTemplateByID(ctx context.Context, tenantID, id string) (*ContractTemplate, error)
	ListTemplates(ctx context.Context, tenantID string) ([]*ContractTemplate, error)
	UpdateTemplate(ctx context.Context, tpl *ContractTemplate) error
	DeleteTemplate(ctx context.Context, id, tenantID string) error
}
