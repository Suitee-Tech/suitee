package repositories

import (
	"context"
	"encoding/json"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/database"
)

type appearancePostgresRepository struct {
	db *database.PostgresDB
}

func NewAppearancePostgresRepository(db *database.PostgresDB) settings.AppearanceRepository {
	return &appearancePostgresRepository{db: db}
}

func (r *appearancePostgresRepository) GetByTenantID(ctx context.Context, tenantID string) (*settings.AppearanceSettings, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM core.appearance_settings WHERE tenant_id = $1", tenantID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var s settings.AppearanceSettings
	_ = json.Unmarshal(metadata, &s)
	return &s, nil
}

func (r *appearancePostgresRepository) Upsert(ctx context.Context, s *settings.AppearanceSettings) error {
	metadata, _ := json.Marshal(s)
	query := `
		INSERT INTO core.appearance_settings (tenant_id, metadata) 
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET metadata = EXCLUDED.metadata
	`
	_, err := r.db.ExecContext(ctx, query, s.TenantID, string(metadata))
	return err
}
