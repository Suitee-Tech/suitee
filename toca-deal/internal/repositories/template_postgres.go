package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

type templatePostgresRepository struct {
	db *database.PostgresDB
}

func NewTemplatePostgresRepository(db *database.PostgresDB) contract.TemplateRepository {
	return &templatePostgresRepository{db: db}
}

func (r *templatePostgresRepository) CreateTemplate(ctx context.Context, tpl *contract.ContractTemplate) error {
	if tpl.ID == "" {
		tpl.ID = uuid.NewString()
	}
	tpl.CreatedAt = time.Now()
	metadata, _ := json.Marshal(tpl)
	query := `INSERT INTO deal.templates (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, tpl.ID, tpl.TenantID, string(metadata)).Scan(&tpl.CreatedAt)
}

func (r *templatePostgresRepository) GetTemplateByID(ctx context.Context, tenantID, id string) (*contract.ContractTemplate, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM deal.templates WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	var tpl contract.ContractTemplate
	_ = json.Unmarshal(metadata, &tpl)
	return &tpl, nil
}

func (r *templatePostgresRepository) ListTemplates(ctx context.Context, tenantID string) ([]*contract.ContractTemplate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.templates
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*contract.ContractTemplate, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var tpl contract.ContractTemplate
		if err := json.Unmarshal(metadata, &tpl); err != nil {
			return nil, err
		}
		out = append(out, &tpl)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *templatePostgresRepository) UpdateTemplate(ctx context.Context, tpl *contract.ContractTemplate) error {
	tpl.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(tpl)
	// P2/item 36: escrita escopada pelo tenant do agregado (o DeleteTemplate já
	// fazia isso; o Update ficou para trás).
	_, err := r.db.ExecContext(ctx, "UPDATE deal.templates SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), tpl.ID, tpl.TenantID)
	return err
}

func (r *templatePostgresRepository) DeleteTemplate(ctx context.Context, id, tenantID string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deal.templates SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}
