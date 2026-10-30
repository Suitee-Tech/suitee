package preferences

import "context"

type Repository interface {
	GetByAccountID(ctx context.Context, accountID string) (*UserPreferences, error)
	Upsert(ctx context.Context, prefs *UserPreferences) error
}
