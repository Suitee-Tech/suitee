package managed_bill

import (
	"context"
)

type Filter struct {
	SourceTransferID string
	ContractID       string
	PropertyID       string
	OwnerID          string
	CompetenceMonth  string
	Status           Status
	Search           string
	Limit            int64
	Offset           int64
}

// Repository keeps tenant scope explicit on every read and uses the aggregate
// Version for compare-and-swap lifecycle mutations.
type Repository interface {
	Create(ctx context.Context, bill *ManagedBill) error
	GetByID(ctx context.Context, tenantID, id string) (*ManagedBill, error)
	GetBySourceKey(ctx context.Context, tenantID, sourceKey string) (*ManagedBill, error)
	SumActiveReservedCentsByTransfer(ctx context.Context, tenantID, transferID string) (int64, error)
	UpdateIfVersion(ctx context.Context, bill *ManagedBill, expectedVersion int64) (bool, error)
	List(ctx context.Context, tenantID string, filter Filter) ([]*ManagedBill, int64, error)
}
