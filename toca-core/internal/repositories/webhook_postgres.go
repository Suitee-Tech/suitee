package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/database"
)

type webhookPostgresRepository struct {
	db *database.PostgresDB
}

func NewWebhookPostgresRepository(db *database.PostgresDB) settings.WebhookRepository {
	return &webhookPostgresRepository{db: db}
}

// webhookMetadata é a projeção persistida em core.webhooks.metadata. O modelo
// de domínio tem Secret com `json:"-"` (write-only: nunca serializado nas
// respostas da API — P0/A1), então a persistência usa esta projeção explícita
// para continuar guardando o signing secret.
type webhookMetadata struct {
	ID            interface{} `json:"id,omitempty"`
	TenantID      string      `json:"tenant_id"`
	URL           string      `json:"url"`
	Events        []string    `json:"events"`
	Active        bool        `json:"active"`
	Secret        string      `json:"secret"`
	LastTriggered time.Time   `json:"lastTriggered"`
}

func toWebhookMetadata(w *settings.WebhookEndpoint) webhookMetadata {
	return webhookMetadata{
		ID:            w.ID,
		TenantID:      w.TenantID,
		URL:           w.URL,
		Events:        w.Events,
		Active:        w.Active,
		Secret:        w.Secret,
		LastTriggered: w.LastTriggered,
	}
}

// toDomain reconstrói o agregado a partir da linha. rowID é o id da coluna
// (fonte de verdade); metadata.id só é usado quando presente, para linhas
// gravadas antes desta projeção.
func (m webhookMetadata) toDomain(rowID string) settings.WebhookEndpoint {
	w := settings.WebhookEndpoint{
		TenantID:      m.TenantID,
		URL:           m.URL,
		Events:        m.Events,
		Active:        m.Active,
		Secret:        m.Secret,
		LastTriggered: m.LastTriggered,
	}
	if id, ok := m.ID.(string); ok && id != "" {
		w.ID = id
	} else if rowID != "" {
		w.ID = rowID
	}
	return w
}

func (r *webhookPostgresRepository) Create(ctx context.Context, w *settings.WebhookEndpoint) error {
	metadata, _ := json.Marshal(toWebhookMetadata(w))
	query := `INSERT INTO core.webhooks (tenant_id, metadata) VALUES ($1, $2) RETURNING id`
	// Assuming ID is generated via PostgreSQL UUID
	err := r.db.QueryRowContext(ctx, query, w.TenantID, string(metadata)).Scan(&w.ID)
	return err
}

func (r *webhookPostgresRepository) GetByID(ctx context.Context, id string) (*settings.WebhookEndpoint, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM core.webhooks WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var m webhookMetadata
	_ = json.Unmarshal(metadata, &m)
	w := m.toDomain(id)
	return &w, nil
}

func (r *webhookPostgresRepository) ListByTenantID(ctx context.Context, tenantID string) ([]settings.WebhookEndpoint, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, metadata
		FROM core.webhooks
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]settings.WebhookEndpoint, 0)
	for rows.Next() {
		var id string
		var metadata []byte
		if err := rows.Scan(&id, &metadata); err != nil {
			return nil, err
		}
		var m webhookMetadata
		if err := json.Unmarshal(metadata, &m); err != nil {
			return nil, err
		}
		out = append(out, m.toDomain(id))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *webhookPostgresRepository) Update(ctx context.Context, w *settings.WebhookEndpoint) error {
	metadata, _ := json.Marshal(toWebhookMetadata(w))
	_, err := r.db.ExecContext(ctx, "UPDATE core.webhooks SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), w.ID, w.TenantID)
	return err
}

// Delete faz o soft delete somente dentro do tenant do chamador: o tenant_id
// entra no WHERE junto com o id (P0/C6). Zero linhas afetadas significam
// "não existe no seu tenant" — devolvemos sql.ErrNoRows para o handler
// responder 404 em vez de um sucesso falso.
func (r *webhookPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx, "UPDATE core.webhooks SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
