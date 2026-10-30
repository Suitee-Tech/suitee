package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ops/internal/models/key"
)

type keyRepository struct {
	db *database.PostgresDB
}

func NewKeyRepository(db *database.PostgresDB) key.Repository {
	return &keyRepository{db: db}
}

func (r *keyRepository) Create(ctx context.Context, k *key.Key) error {
	if k.ID == "" {
		k.ID = uuid.NewString()
	}
	k.CreatedAt = time.Now()
	metadata, _ := json.Marshal(k)
	query := `INSERT INTO ops.keys (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, k.ID, k.TenantID, string(metadata)).Scan(&k.CreatedAt)
}

// keyGetByIDQuery é tenant-scoped de propósito (auditoria A8/ops): chaves de
// outro tenant não podem ser lidas nem sofrer check-out/check-in a partir de um
// id conhecido.
const keyGetByIDQuery = `SELECT metadata FROM ops.keys WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`

// GetByID devolve a chave apenas quando ela pertence a tenantID e falha fechado
// quando o tenant está vazio.
func (r *keyRepository) GetByID(ctx context.Context, tenantID, id string) (*key.Key, error) {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return nil, fmt.Errorf("key not found: tenant context required")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("key not found: id required")
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, keyGetByIDQuery, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("key not found: %w", err)
	}
	var k key.Key
	_ = json.Unmarshal(metadata, &k)
	return &k, nil
}

func (r *keyRepository) GetByPropertyID(ctx context.Context, propertyID string) ([]*key.Key, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM ops.keys
		WHERE deleted_at IS NULL AND (metadata->>'property_id') = $1
		ORDER BY created_at DESC
	`, propertyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*key.Key, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var k key.Key
		if err := json.Unmarshal(metadata, &k); err != nil {
			return nil, err
		}
		out = append(out, &k)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *keyRepository) Update(ctx context.Context, k *key.Key) error {
	k.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(k)
	_, err := r.db.ExecContext(ctx, "UPDATE ops.keys SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), k.ID, k.TenantID)
	return err
}

func (r *keyRepository) List(ctx context.Context, tenantID string, propertyID string, status string, limit, offset int64) ([]*key.Key, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2
	if propertyID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'property_id') = $%d", argN))
		args = append(args, propertyID)
		argN++
	}
	if status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, status)
		argN++
	}
	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM ops.keys "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM ops.keys %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*key.Key, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var k key.Key
		if err := json.Unmarshal(metadata, &k); err != nil {
			return nil, 0, err
		}
		out = append(out, &k)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *keyRepository) Delete(ctx context.Context, tenantID, id string) error {
	tenantID = strings.TrimSpace(tenantID)
	if tenantID == "" {
		return fmt.Errorf("key not found: tenant context required")
	}
	// P2/item 36: predicado de tenant na própria escrita.
	_, err := r.db.ExecContext(ctx, "UPDATE ops.keys SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}
