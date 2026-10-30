package repository

import (
	"context"
	"encoding/json"

	"github.com/oginoino/toca-auth/internal/domain/preferences"
	"github.com/oginoino/toca/pkg/database"
)

type preferencesPostgresRepository struct {
	db *database.PostgresDB
}

func NewPreferencesPostgresRepository(db *database.PostgresDB) preferences.Repository {
	return &preferencesPostgresRepository{db: db}
}

func (r *preferencesPostgresRepository) GetByAccountID(ctx context.Context, accountID string) (*preferences.UserPreferences, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM toca_auth.user_preferences WHERE account_id = $1", accountID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var prefs preferences.UserPreferences
	_ = json.Unmarshal(metadata, &prefs)
	return &prefs, nil
}

func (r *preferencesPostgresRepository) Upsert(ctx context.Context, prefs *preferences.UserPreferences) error {
	metadata, _ := json.Marshal(prefs)
	query := `
		INSERT INTO toca_auth.user_preferences (account_id, metadata) 
		VALUES ($1, $2)
		ON CONFLICT (account_id) DO UPDATE SET metadata = EXCLUDED.metadata
	`
	_, err := r.db.ExecContext(ctx, query, prefs.AccountID, string(metadata))
	return err
}
