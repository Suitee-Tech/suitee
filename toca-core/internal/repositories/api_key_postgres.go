package repositories

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/oginoino/toca-core/internal/domain/core/settings"
	"github.com/oginoino/toca/pkg/database"
)

type apiKeyPostgresRepository struct {
	db *database.PostgresDB
}

func NewAPIKeyPostgresRepository(db *database.PostgresDB) settings.APIKeyRepository {
	return &apiKeyPostgresRepository{db: db}
}

func (r *apiKeyPostgresRepository) Create(ctx context.Context, key *settings.APIKey) error {
	metadata, _ := json.Marshal(key)
	query := `INSERT INTO core.api_keys (tenant_id, metadata) VALUES ($1, $2) RETURNING id`
	err := r.db.QueryRowContext(ctx, query, key.TenantID, metadata).Scan(&key.ID)
	return err
}

func (r *apiKeyPostgresRepository) ListByTenantID(ctx context.Context, tenantID string) ([]settings.APIKey, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, metadata
		FROM core.api_keys
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]settings.APIKey, 0)
	for rows.Next() {
		var id string
		var metadata []byte
		if err := rows.Scan(&id, &metadata); err != nil {
			return nil, err
		}
		var k settings.APIKey
		if err := json.Unmarshal(metadata, &k); err != nil {
			return nil, err
		}
		// O metadata é gravado antes do RETURNING, então `id` costuma vir null:
		// sem esta checagem (nil-safe) a listagem devolvia "id": null e a
		// revogação pela UI era impossível.
		if idMeta, ok := k.ID.(string); !ok || idMeta == "" {
			k.ID = id
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// Revoke desativa a API key somente dentro do tenant do chamador: o tenant_id
// entra no WHERE junto com o id (P0/C6). Zero linhas afetadas => sql.ErrNoRows,
// para o handler responder 404 em vez de um sucesso falso.
//
// A chave JSON é `isActive` (a mesma que Create grava e que a API devolve em
// settings.APIKey.IsActive): a versão anterior escrevia `is_active` e por isso
// a revogação não aparecia em nenhuma leitura — a chave continuava "Ativa".
func (r *apiKeyPostgresRepository) Revoke(ctx context.Context, tenantID, id string) error {
	res, err := r.db.ExecContext(ctx, "UPDATE core.api_keys SET metadata = jsonb_set(metadata, '{isActive}', 'false'::jsonb) WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
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
