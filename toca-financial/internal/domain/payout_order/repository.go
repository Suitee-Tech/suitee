package payout_order

import (
	"context"
	"time"
)

// ListFilter narrows a List query without using positional args. All
// fields are optional; empty strings/zero values fall through.
type ListFilter struct {
	Status     string
	SourceType string
	SourceID   string
	ContractID string
	// DealID filtra pelo negócio. Necessário porque a identidade econômica de um
	// plano (plan hash) é do negócio, não do contrato: um aditivo tem contrato
	// novo e plano idêntico, então um filtro por contrato não enxerga as ordens
	// já materializadas pelo contrato original.
	DealID        string
	RecipientType string
	RecipientID   string
	GroupID       string
	Limit         int64
	Offset        int64
}

type Repository interface {
	Create(ctx context.Context, order *PayoutOrder) error
	GetByID(ctx context.Context, tenantID, id string) (*PayoutOrder, error)
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*PayoutOrder, error)
	List(ctx context.Context, tenantID string, filter ListFilter) ([]*PayoutOrder, int64, error)
	Totals(ctx context.Context, tenantID string, filter ListFilter) (*PayoutOrderTotals, error)
	ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*PayoutOrder, error)
	Update(ctx context.Context, order *PayoutOrder) error
	Delete(ctx context.Context, tenantID, id string) error
}

// TransitionRepository is the compare-and-set persistence surface used by
// approve/pay/cancel. Keeping it separate preserves CRUD-only repository
// implementations while making state transitions explicitly reject an unsafe
// read-modify-write fallback.
type TransitionRepository interface {
	UpdateIfStatus(ctx context.Context, order *PayoutOrder, expected Status, expectedUpdatedAt time.Time) error
}

// TransitionTransactionRepository lets the service commit a payout CAS and
// its linked Commission cascade as one database transaction.
type TransitionTransactionRepository interface {
	RunTransitionTransaction(ctx context.Context, fn func(context.Context) error) error
}

// RoyaltyTransitionTransactionRepository is deliberately separate because a
// royalty payout belongs to the payer tenant while the royalty row belongs to
// the payee tenant. Implementations must run both CAS writes in one verified
// service-role transaction; a tenant-scoped transaction cannot see both rows.
type RoyaltyTransitionTransactionRepository interface {
	RunRoyaltyTransitionTransaction(ctx context.Context, fn func(context.Context) error) error
}
