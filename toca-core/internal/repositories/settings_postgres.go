package repositories

import (
	"context"
	"encoding/json"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/database"
)

type companySettingsPostgresRepository struct {
	db *database.PostgresDB
}

func NewCompanySettingsPostgresRepository(db *database.PostgresDB) settings.CompanySettingsRepository {
	return &companySettingsPostgresRepository{db: db}
}

func (r *companySettingsPostgresRepository) GetByTenantID(ctx context.Context, tenantID string) (*settings.CompanySettings, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM core.company_settings WHERE tenant_id = $1", tenantID).Scan(&metadata)
	if err != nil {
		// Retornar nil para não encontrado
		return nil, nil
	}
	var s settings.CompanySettings
	_ = json.Unmarshal(metadata, &s)
	return &s, nil
}

func (r *companySettingsPostgresRepository) Upsert(ctx context.Context, s *settings.CompanySettings) error {
	metadata, _ := json.Marshal(s)
	query := `
		INSERT INTO core.company_settings (tenant_id, metadata) 
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET metadata = EXCLUDED.metadata
	`
	_, err := r.db.ExecContext(ctx, query, s.TenantID, string(metadata))
	return err
}
