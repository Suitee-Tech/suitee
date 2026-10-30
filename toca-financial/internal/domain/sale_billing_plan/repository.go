package sale_billing_plan

import "context"

type Repository interface {
	// Create persists plan or, when another delivery already persisted a plan
	// for the same tenant and contract, returns that canonical winner.
	Create(ctx context.Context, plan *SaleBillingPlan) (*SaleBillingPlan, error)
	GetByID(ctx context.Context, tenantID, id string) (*SaleBillingPlan, error)
	GetByContractID(ctx context.Context, tenantID, contractID string) (*SaleBillingPlan, error)
	Update(ctx context.Context, plan *SaleBillingPlan) error
	Delete(ctx context.Context, tenantID, id string) error
}
