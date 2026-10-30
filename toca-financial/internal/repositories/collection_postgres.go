package repositories

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/collection"
)

type collectionPostgresRepository struct {
	db *database.PostgresDB
}

func NewCollectionPostgresRepository(db *database.PostgresDB) collection.RepositoryInterface {
	return &collectionPostgresRepository{db: db}
}

func (r *collectionPostgresRepository) Create(ctx context.Context, c *collection.Collection) error {
	c.CreatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode collection: %w", err)
	}
	query := `INSERT INTO financial.collections (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRow(ctx, query, c.ID, c.TenantID, string(metadata)).Scan(&c.CreatedAt)
}

// CreateIdempotent insere a collection com proteção ON CONFLICT via
// source_event_id. Se o mesmo source_event_id já existir para o tenant,
// a inserção é ignorada (DO NOTHING) e retorna nil sem erro (FIN-COM-12).
func (r *collectionPostgresRepository) CreateIdempotent(ctx context.Context, c *collection.Collection) error {
	c.CreatedAt = time.Now()
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode collection: %w", err)
	}
	query := `INSERT INTO financial.collections (id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, ((metadata->>'source_event_id')))
		WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL
		DO NOTHING
		RETURNING created_at`
	err = r.db.QueryRow(ctx, query, c.ID, c.TenantID, string(metadata)).Scan(&c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // já existe — idempotente
	}
	return err
}

func (r *collectionPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*collection.Collection, error) {
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx, "SELECT metadata, updated_at FROM financial.collections WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("collection not found: %w", err)
	}
	var c collection.Collection
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, fmt.Errorf("decode collection: %w", err)
	}
	c.UpdatedAt = persistedUpdatedAt
	return &c, nil
}

func (r *collectionPostgresRepository) Update(ctx context.Context, c *collection.Collection) error {
	c.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	metadata, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("encode collection: %w", err)
	}
	result, err := r.db.Exec(ctx, "UPDATE financial.collections SET metadata = $1, updated_at = $4 WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), c.ID, c.TenantID, c.UpdatedAt)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("collection not found")
	}
	return nil
}

func (r *collectionPostgresRepository) UpdateIfVersion(ctx context.Context, c *collection.Collection, expectedUpdatedAt time.Time) (bool, error) {
	c.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
	metadata, err := json.Marshal(c)
	if err != nil {
		return false, fmt.Errorf("encode collection: %w", err)
	}
	result, err := r.db.Exec(ctx, `
		UPDATE financial.collections
		SET metadata = $1, updated_at = $4
		WHERE id = $2
		  AND tenant_id = $3
		  AND deleted_at IS NULL
		  AND updated_at = $5
	`, string(metadata), c.ID, c.TenantID, c.UpdatedAt, expectedUpdatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (r *collectionPostgresRepository) List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*collection.Collection, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2
	if status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, status)
		argN++
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(1) FROM financial.collections "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata, updated_at FROM financial.collections %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*collection.Collection, 0)
	for rows.Next() {
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, 0, err
		}
		var c collection.Collection
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, 0, err
		}
		c.UpdatedAt = persistedUpdatedAt
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *collectionPostgresRepository) AddAction(ctx context.Context, tenantID, id string, action collection.Action) error {
	if action.Date.IsZero() {
		action.Date = time.Now()
	}
	b, err := json.Marshal(action)
	if err != nil {
		return err
	}
	arr := []byte("[" + string(b) + "]")
	_, err = r.db.Exec(ctx, `
		UPDATE financial.collections
		SET metadata = jsonb_set(metadata, '{actions}', COALESCE(metadata->'actions', '[]'::jsonb) || $1::jsonb, true),
		    updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL
	`, string(arr), id, tenantID)
	return err
}

func (r *collectionPostgresRepository) FindByChargeID(ctx context.Context, tenantID, chargeID string) (*collection.Collection, error) {
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx, `
		SELECT metadata, updated_at
		FROM financial.collections
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->'charge_ids') ? $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, chargeID).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("collection lookup by charge_id: %w", err)
	}
	var c collection.Collection
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	c.UpdatedAt = persistedUpdatedAt
	return &c, nil
}

func (r *collectionPostgresRepository) FindByContract(ctx context.Context, contractID, tenantID string) ([]*collection.Collection, error) {
	rows, err := r.db.Query(ctx, `
		SELECT metadata, updated_at
		FROM financial.collections
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'contract_id') = $2
		ORDER BY created_at DESC
	`, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*collection.Collection, 0)
	for rows.Next() {
		var metadata []byte
		var persistedUpdatedAt time.Time
		if err := rows.Scan(&metadata, &persistedUpdatedAt); err != nil {
			return nil, err
		}
		var c collection.Collection
		if err := json.Unmarshal(metadata, &c); err != nil {
			return nil, err
		}
		c.UpdatedAt = persistedUpdatedAt
		out = append(out, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *collectionPostgresRepository) FindActiveByDebtor(ctx context.Context, debtorID, tenantID string) (*collection.Collection, error) {
	var metadata []byte
	var persistedUpdatedAt time.Time
	err := r.db.QueryRow(ctx, `
		SELECT metadata, updated_at
		FROM financial.collections
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'debtor_id') = $2 AND (metadata->>'status') = 'ACTIVE'
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, debtorID).Scan(&metadata, &persistedUpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("collection lookup by debtor: %w", err)
	}
	var c collection.Collection
	if err := json.Unmarshal(metadata, &c); err != nil {
		return nil, err
	}
	c.UpdatedAt = persistedUpdatedAt
	return &c, nil
}

func (r *collectionPostgresRepository) GetStats(ctx context.Context, tenantID string) (*collection.CollectionStats, error) {
	rows, err := r.db.Query(ctx, `
		SELECT (metadata->>'status') AS status, COUNT(1)::int AS cnt, COALESCE(SUM((metadata->>'total_debt')::numeric), 0)::float8 AS total_debt
		FROM financial.collections
		WHERE tenant_id = $1 AND deleted_at IS NULL
		GROUP BY 1
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := &collection.CollectionStats{}
	for rows.Next() {
		var status string
		var cnt int
		var debt float64
		if err := rows.Scan(&status, &cnt, &debt); err != nil {
			return nil, err
		}
		switch status {
		case string(collection.StatusActive):
			stats.Active = cnt
		case string(collection.StatusNegotiated):
			stats.Negotiated = cnt
		case string(collection.StatusGuaranteeActivated):
			stats.GuaranteeActivated = cnt
		case string(collection.StatusJudicial):
			stats.Judicial = cnt
		case string(collection.StatusResolved):
			stats.Resolved = cnt
		}
		// A dívida do caso RESOLVED é histórica: somá-la inflava o total que a
		// tela e a métrica exibem como "em aberto" (G22 do plano). O valor
		// recuperado é calculado no serviço, onde há acesso às cobranças pagas.
		if status != string(collection.StatusResolved) {
			stats.TotalDebt += debt
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}
