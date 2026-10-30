package charge

import (
	"context"
	"errors"
	"time"
)

// ErrChargeAlreadyExists is returned by Create when an upstream-event-driven
// insert collides with the partial unique index on (tenant_id, source_event_id).
// Callers should treat this as a benign no-op — the charge already landed
// from a previous broker delivery.
var ErrChargeAlreadyExists = errors.New("charge already exists for this source_event_id")

// ErrChargeAlreadyPaid is returned by service operations that cannot run
// on a paid charge (e.g. Cancel). Handlers map this to 409 Conflict — the
// caller's intent is well-formed but the current state forbids it.
var ErrChargeAlreadyPaid = errors.New("charge is already paid")

// ErrChargeAlreadyCancelled is returned by service operations that would
// be a no-op on an already-cancelled charge (e.g. Cancel). Handlers map
// this to 409 Conflict.
var ErrChargeAlreadyCancelled = errors.New("charge is already cancelled")

// ErrChargeNotFound is returned by service Get/Update paths when the
// repository finds no row for the given id under the tenant. Handlers
// map this to 404.
var ErrChargeNotFound = errors.New("charge not found")

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go

type SearchFilter struct {
	ContractID     string
	ReferenceMonth string
	Status         string

	PartyIDs []string

	CustomerID   string
	CustomerName string
	DealID       string

	MinAmount *float64
	MaxAmount *float64

	DueFrom  *time.Time
	DueTo    *time.Time
	PaidFrom *time.Time
	PaidTo   *time.Time

	PaymentMethod string

	Limit  int64
	Offset int64
}

type RepositoryInterface interface {
	Create(ctx context.Context, c *Charge) error
	GetByID(ctx context.Context, tenantID, id string) (*Charge, error)
	// GetBySourceEventID returns the charge that was materialized from the
	// given upstream event, if any. Used by handlers that need to read back
	// the existing row after Create returned ErrChargeAlreadyExists.
	GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*Charge, error)
	Update(ctx context.Context, c *Charge) error
	List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*Charge, int64, error)
	ListByPartyIDs(ctx context.Context, tenantID string, partyIDs []string, status, referenceMonth string, limit, offset int64) ([]*Charge, int64, error)
	// ListByLeadIDs lista charges onde leadIDs ∩ (lead_id ∪ owner_id) é
	// não-vazio. Entry point para o portal cliente (#460) — tenant vê
	// charges onde é o lead pagador (lead_id), owner vê charges das
	// propriedades que tem repasse (owner_id). Lista vazia devolve vazio
	// imediatamente para evitar SELECT WHERE FALSE.
	ListByLeadIDs(ctx context.Context, tenantID string, leadIDs []string, status, referenceMonth string, limit, offset int64) ([]*Charge, int64, error)
	Search(ctx context.Context, tenantID string, filter SearchFilter) ([]*Charge, int64, error)
	ListOverdue(ctx context.Context, tenantID string) ([]*Charge, error)
	BulkUpdateStatus(ctx context.Context, tenantID string, ids []string, status Status) error
	ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*Charge, error)
	ListPendingPastDue(ctx context.Context, tenantID string) ([]*Charge, error)
	ListDistinctTenantIDs(ctx context.Context) ([]string, error)
	// ListByDealAndCustomer lista cobranças de um deal para uma parte.
	//
	// SECURITY (achado A3): o tenant é obrigatório — a versão anterior filtrava
	// só por deal + parte e permitia leitura cross-tenant pela rota interna.
	ListByDealAndCustomer(ctx context.Context, tenantID, dealID, accountID, role string) ([]*Charge, error)
	AgingReport(ctx context.Context, tenantID string) ([]AgingBucket, error)
	GetDelinquencyStats(ctx context.Context, tenantID string) (*DelinquencyStats, error)
}

// AgingBucket represents a group of overdue charges by age range
type AgingBucket struct {
	Label     string  `json:"label"`
	MinDays   int     `json:"min_days"`
	MaxDays   int     `json:"max_days"`
	Count     int     `json:"count"`
	TotalDebt float64 `json:"total_debt"`
}

// DelinquencyStats represents dashboard KPIs for delinquency management
type DelinquencyStats struct {
	TotalOverdue       int     `json:"total_overdue"`
	TotalDebtAmount    float64 `json:"total_debt_amount"`
	AverageDebtAmount  float64 `json:"average_debt_amount"`
	AverageDaysOverdue float64 `json:"average_days_overdue"`
	ActiveCollections  int     `json:"active_collections"`
	RecoveryRate       float64 `json:"recovery_rate"`
}
