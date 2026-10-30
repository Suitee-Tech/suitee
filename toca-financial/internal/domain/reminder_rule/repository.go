package reminder_rule

import "context"

type Repository interface {
	Create(ctx context.Context, r *ReminderRule) error
	GetByID(ctx context.Context, tenantID, id string) (*ReminderRule, error)
	ListByTenant(ctx context.Context, tenantID string) ([]*ReminderRule, error)
	ListEnabled(ctx context.Context, tenantID string) ([]*ReminderRule, error)
	Update(ctx context.Context, r *ReminderRule) error
	Delete(ctx context.Context, tenantID, id string) error
	// ListDistinctTenantIDs powers the scheduler: it iterates over each
	// tenant that has at least one enabled rule and evaluates them.
	ListDistinctTenantIDs(ctx context.Context) ([]string, error)
}
