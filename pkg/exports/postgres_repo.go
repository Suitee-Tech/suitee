package exports

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/oginoino/toca/pkg/database"
)

// PostgresRepository — implementação default da Repository sobre o
// schema exports.jobs (migration 20260519130000_export_jobs.sql).
// Padrão JSONB-metadata: blob inteiro vai em `metadata`, campos
// indexados (status, requested_by, etc.) ficam denormalizados para
// queries da fila.
type PostgresRepository struct {
	db *database.PostgresDB
}

func NewPostgresRepository(db *database.PostgresDB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) Create(ctx context.Context, j *Job) error {
	metadata, err := json.Marshal(j)
	if err != nil {
		return err
	}
	const q = `
		INSERT INTO exports.jobs (id, tenant_id, requested_by, entity_type, format, status, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING created_at, updated_at
	`
	return r.db.QueryRowContext(ctx, q,
		j.ID, j.TenantID, j.RequestedBy, string(j.EntityType), string(j.Format), string(j.Status), string(metadata),
	).Scan(&j.CreatedAt, new(interface{}))
}

func (r *PostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*Job, error) {
	var metadata []byte
	const q = `
		SELECT metadata
		  FROM exports.jobs
		 WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL
		 LIMIT 1
	`
	err := r.db.QueryRowContext(ctx, q, id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, ErrJobNotFound
	}
	var j Job
	if err := json.Unmarshal(metadata, &j); err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *PostgresRepository) Update(ctx context.Context, j *Job) error {
	metadata, err := json.Marshal(j)
	if err != nil {
		return err
	}
	const q = `
		UPDATE exports.jobs
		   SET status = $1, metadata = $2
		 WHERE id = $3 AND tenant_id = $4 AND deleted_at IS NULL
	`
	res, err := r.db.ExecContext(ctx, q, string(j.Status), string(metadata), j.ID, j.TenantID)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return errors.New("export job not found")
	}
	return nil
}

func (r *PostgresRepository) ListByRequester(ctx context.Context, tenantID, accountID string, limit, offset int) ([]*Job, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `
		SELECT metadata
		  FROM exports.jobs
		 WHERE tenant_id = $1 AND requested_by = $2 AND deleted_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT $3 OFFSET $4
	`
	rows, err := r.db.QueryContext(ctx, q, tenantID, accountID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*Job, 0, limit)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var j Job
		if err := json.Unmarshal(metadata, &j); err != nil {
			return nil, err
		}
		out = append(out, &j)
	}
	return out, rows.Err()
}

// ClaimNextQueued — atomicamente pega o próximo job em QUEUED, marca
// como RUNNING, e devolve. Usa UPDATE ... RETURNING para que múltiplos
// workers concorrentes (ou múltiplas instâncias replicadas) não peguem
// o mesmo job. Se a fila estiver vazia, retorna (nil, nil) — não é
// erro, só nada pra fazer.
//
// O método NÃO usa LIMIT 1 dentro do UPDATE direto (Postgres não
// suporta LIMIT em UPDATE pra escolher rows arbitrárias) — em vez,
// usa um SELECT em CTE ordenado, com FOR UPDATE SKIP LOCKED pra
// permitir paralelismo entre workers concorrentes.
func (r *PostgresRepository) ClaimNextQueued(ctx context.Context) (*Job, error) {
	const q = `
		WITH next AS (
			SELECT id
			  FROM exports.jobs
			 WHERE status = 'QUEUED' AND deleted_at IS NULL
			 ORDER BY created_at ASC
			 LIMIT 1
			 FOR UPDATE SKIP LOCKED
		)
		UPDATE exports.jobs j
		   SET status = 'RUNNING',
		       metadata = jsonb_set(
		           jsonb_set(j.metadata, '{Status}', '"RUNNING"', true),
		           '{StartedAt}', to_jsonb(now()), true
		       )
		  FROM next
		 WHERE j.id = next.id
		RETURNING j.metadata
	`
	var metadata []byte
	pool := r.db.ServicePool
	if pool == nil {
		pool = r.db.GetPool()
	}
	err := pool.QueryRow(ctx, q).Scan(&metadata)
	if err != nil {
		// pgx returns ErrNoRows quando o CTE não devolve nada (fila
		// vazia). Tratamos como (nil, nil) — não é erro.
		if err.Error() == "no rows in result set" {
			return nil, nil
		}
		return nil, err
	}
	var j Job
	if err := json.Unmarshal(metadata, &j); err != nil {
		return nil, err
	}
	return &j, nil
}
