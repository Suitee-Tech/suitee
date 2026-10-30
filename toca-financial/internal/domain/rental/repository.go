package rental

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// LeadRole is used by ListByLeadID to scope the query to rentals where the
// lead is *either* the tenant *or* the owner. Returning both at once would
// confuse downstream callers; passing the role is explicit.
type LeadRole string

const (
	LeadRoleTenant LeadRole = "tenant"
	LeadRoleOwner  LeadRole = "owner"
)

type Repository interface {
	Create(ctx context.Context, r *Rental) error
	// CreateInTx mirrors Create but joins the caller's transaction so the
	// rental insert and a subsequent EnqueueOutboxInTx call land or roll
	// back together (audit P0.1 follow-up). Callers that don't need that
	// atomicity should keep using Create.
	CreateInTx(ctx context.Context, tx *sqlx.Tx, r *Rental) error
	GetByID(ctx context.Context, tenantID, id string) (*Rental, error)
	GetByDealID(ctx context.Context, tenantID, dealID string) (*Rental, error)
	GetByContractID(ctx context.Context, tenantID, contractID string) (*Rental, error)
	ListByLeadID(ctx context.Context, tenantID, leadID string, role LeadRole) ([]*Rental, error)
	// ListForLeadIDs returns rentals where ANY of the given leadIDs is the
	// tenant or owner. Used by the portal handler that resolves the user's
	// JWT account_id to multiple leads (a user can be tenant of one rental
	// and owner of another).
	ListForLeadIDs(ctx context.Context, tenantID string, leadIDs []string) ([]*Rental, error)
	ListByPropertyID(ctx context.Context, tenantID, propertyID string) ([]*Rental, error)
	Update(ctx context.Context, r *Rental) error
}
