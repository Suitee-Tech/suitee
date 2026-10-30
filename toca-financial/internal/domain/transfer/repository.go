package transfer

import (
	"context"
	"errors"
	"time"
)

// ErrTransferAlreadyExists is returned when Create collides either with the
// canonical (tenant, source_charge_id) identity or with the legacy/manual
// (tenant, contract, reference_month) identity. The caller must re-fetch by
// the same key it attempted; charge-backed transfers use source_charge_id.
var ErrTransferAlreadyExists = errors.New("transfer already exists for this financial identity")

// ErrConcurrentModification protects lifecycle transitions from overwriting a
// deduction or another status change committed after the transfer was read.
var ErrConcurrentModification = errors.New("transfer was modified concurrently")

//go:generate mockgen -source=repository.go -destination=mocks/repository_mock.go

type SearchFilter struct {
	ContractID     string
	OwnerID        string
	BrokerID       string
	Status         string
	ReferenceMonth string
	MinAmount      *float64
	MaxAmount      *float64
	DateFrom       *time.Time
	DateTo         *time.Time
	Limit          int64
	Offset         int64
}

// PortalListFilter restringe o resultado de ListByOwnerLeadIDs (portal owner).
// Status e Year são opcionais — vazio = sem filtro.
type PortalListFilter struct {
	Status string // ex.: "PENDING" | "APPROVED" | "PROCESSED"
	Year   int    // YYYY; 0 = todos os anos
	Limit  int64
	Offset int64
}

type RepositoryInterface interface {
	Create(ctx context.Context, t *OwnerTransfer) error
	GetByID(ctx context.Context, tenantID, id string) (*OwnerTransfer, error)
	// GetByContractMonth returns one row for legacy/manual monthly workflows.
	// Automatic charge-backed transfers use source_charge_id because SALE may
	// contain more than one installment in the same reference month.
	GetByContractMonth(ctx context.Context, tenantID, contractID, referenceMonth string) (*OwnerTransfer, error)
	Update(ctx context.Context, t *OwnerTransfer) error
	List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*OwnerTransfer, int64, error)
	// ListByOwnerLeadIDs lista repasses cujo owner_id ∈ ownerLeadIDs.
	// Usado pelo portal cliente (#456) — a resolução account_id → leads
	// fica no caller (PortalTransferHandler.ListMine via CrossTenantRepository).
	// ownerLeadIDs vazio devolve lista vazia (não retorna todos os transfers
	// do tenant — sem filtro seria leak entre owners).
	ListByOwnerLeadIDs(ctx context.Context, tenantID string, ownerLeadIDs []string, filter PortalListFilter) ([]*OwnerTransfer, int64, error)
	UpdateStatus(ctx context.Context, tenantID, id string, status Status) error
	Search(ctx context.Context, tenantID string, filter SearchFilter) ([]*OwnerTransfer, int64, error)
}
