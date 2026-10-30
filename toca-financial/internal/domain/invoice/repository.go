package invoice

import (
	"context"
	"time"
)

type SearchFilter struct {
	CustomerID   string
	CustomerName string
	Status       string
	MinAmount    *float64
	MaxAmount    *float64
	IssuedFrom   *time.Time
	IssuedTo     *time.Time
	Limit        int64
	Offset       int64
	ReferenceID  string
}

type Repository interface {
	Create(ctx context.Context, invoice *Invoice) error
	GetByID(ctx context.Context, tenantID, id string) (*Invoice, error)
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*Invoice, error)
	// SumOpenAmountCentsByChargeID returns the amount reserved by linked
	// invoices that are not terminal. PAID invoices are already reflected in
	// the canonical Charge balance and CANCELLED invoices release their
	// reservation, so neither status participates in this sum.
	SumOpenAmountCentsByChargeID(ctx context.Context, tenantID, chargeID, excludedInvoiceID string) (int64, error)
	GetByReferenceID(ctx context.Context, tenantID, referenceID string) ([]*Invoice, error)
	Update(ctx context.Context, invoice *Invoice) error
	List(ctx context.Context, tenantID string, status *Status, referenceID *string) ([]*Invoice, error)
	Search(ctx context.Context, tenantID string, filter SearchFilter) ([]*Invoice, int64, error)
}
