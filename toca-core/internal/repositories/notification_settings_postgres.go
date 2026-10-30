package repositories

import (
	"context"
	"encoding/json"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/database"
)

type notificationSettingsPostgresRepository struct {
	db *database.PostgresDB
}

func NewNotificationSettingsPostgresRepository(db *database.PostgresDB) settings.NotificationSettingsRepository {
	return &notificationSettingsPostgresRepository{db: db}
}

func (r *notificationSettingsPostgresRepository) GetByTenantID(ctx context.Context, tenantID string) (*settings.NotificationSettings, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM core.notification_settings WHERE tenant_id = $1", tenantID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var s settings.NotificationSettings
	_ = json.Unmarshal(metadata, &s)
	return &s, nil
}

func (r *notificationSettingsPostgresRepository) Upsert(ctx context.Context, s *settings.NotificationSettings) error {
	metadata, _ := json.Marshal(s)
	query := `
		INSERT INTO core.notification_settings (tenant_id, metadata) 
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET metadata = EXCLUDED.metadata
	`
	_, err := r.db.ExecContext(ctx, query, s.TenantID, string(metadata))
	return err
}
