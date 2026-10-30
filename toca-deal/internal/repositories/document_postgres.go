package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
)

type documentPostgresRepository struct {
	db *database.PostgresDB
}

func NewDocumentPostgresRepository(db *database.PostgresDB) document.RepositoryInterface {
	return &documentPostgresRepository{db: db}
}

func (r *documentPostgresRepository) Create(ctx context.Context, d *document.Document) error {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	d.CreatedAt = time.Now()
	metadata, _ := json.Marshal(d)
	query := `INSERT INTO deal.documents (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, d.ID, d.TenantID, string(metadata)).Scan(&d.CreatedAt)
}

// GetByID resolve o documento dentro do tenant informado.
//
// SECURITY (achado C11): a query filtra por `tenant_id` além do `id` e falha
// fechado quando o tenant é vazio — não degrada para busca global.
func (r *documentPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*document.Document, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("document not found: tenant is required")
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM deal.documents WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}
	var d document.Document
	_ = json.Unmarshal(metadata, &d)
	return &d, nil
}

func (r *documentPostgresRepository) Update(ctx context.Context, d *document.Document) error {
	d.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(d)
	_, err := r.db.ExecContext(ctx, "UPDATE deal.documents SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), d.ID, d.TenantID)
	return err
}

func (r *documentPostgresRepository) Delete(ctx context.Context, id, tenantID string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE deal.documents SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	return err
}

func (r *documentPostgresRepository) ListByContract(ctx context.Context, contractID, tenantID string) ([]*document.Document, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.documents
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'contract_id') = $2
		ORDER BY created_at DESC
	`, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*document.Document, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d document.Document
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *documentPostgresRepository) ListByEntity(ctx context.Context, entityID string, entityType document.EntityType, tenantID string) ([]*document.Document, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.documents
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'entity_id') = $2 AND (metadata->>'entity_type') = $3
		ORDER BY created_at DESC
	`, tenantID, entityID, string(entityType))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*document.Document, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var d document.Document
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *documentPostgresRepository) List(ctx context.Context, tenantID string, filters document.ListFilters) ([]*document.Document, int64, error) {
	limit := filters.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filters.Offset
	if offset < 0 {
		offset = 0
	}

	where := []string{"tenant_id = $1", "deleted_at IS NULL"}
	args := []any{tenantID}
	argN := 2

	if filters.ContractID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'contract_id') = $%d", argN))
		args = append(args, filters.ContractID)
		argN++
	}
	if filters.EntityID != "" {
		where = append(where, fmt.Sprintf("(metadata->>'entity_id') = $%d", argN))
		args = append(args, filters.EntityID)
		argN++
	}
	if filters.EntityType != "" {
		where = append(where, fmt.Sprintf("(metadata->>'entity_type') = $%d", argN))
		args = append(args, string(filters.EntityType))
		argN++
	}
	if filters.Category != "" {
		where = append(where, fmt.Sprintf("(metadata->>'category') = $%d", argN))
		args = append(args, string(filters.Category))
		argN++
	}
	if filters.Status != "" {
		where = append(where, fmt.Sprintf("(metadata->>'status') = $%d", argN))
		args = append(args, string(filters.Status))
		argN++
	}
	if filters.Source != "" {
		where = append(where, fmt.Sprintf("(metadata->>'source') = $%d", argN))
		args = append(args, string(filters.Source))
		argN++
	}
	if q := strings.TrimSpace(filters.Q); q != "" {
		// ILIKE against name AND notes — operators typed in the Central
		// de Documentos search box should find documents by their human
		// label or by a snippet from the notes field. % wraps below.
		where = append(where, fmt.Sprintf(
			"((metadata->>'name') ILIKE $%d OR (metadata->>'notes') ILIKE $%d)",
			argN, argN,
		))
		args = append(args, "%"+q+"%")
		argN++
	}
	if len(filters.Tags) > 0 {
		// OR semantics: match documents whose tags[] contains any of the
		// supplied values. Built as a list of `metadata->'tags' ? $N`
		// checks because the `?` operator takes a single text and avoids
		// the need to import lib/pq just to pass a string array.
		ors := make([]string, 0, len(filters.Tags))
		for _, tag := range filters.Tags {
			if tag == "" {
				continue
			}
			ors = append(ors, fmt.Sprintf("(metadata->'tags') ? $%d", argN))
			args = append(args, tag)
			argN++
		}
		if len(ors) > 0 {
			where = append(where, "("+strings.Join(ors, " OR ")+")")
		}
	}

	whereSQL := "WHERE " + strings.Join(where, " AND ")

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM deal.documents "+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM deal.documents %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", whereSQL, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*document.Document, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var d document.Document
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, 0, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *documentPostgresRepository) Search(ctx context.Context, tenantID, query string, limit, offset int64) ([]*document.Document, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	search := "%" + strings.TrimSpace(query) + "%"
	if strings.TrimSpace(query) == "" {
		search = "%"
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM deal.documents
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'name') ILIKE $2
	`, tenantID, search).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM deal.documents
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'name') ILIKE $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, search, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*document.Document, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var d document.Document
		if err := json.Unmarshal(metadata, &d); err != nil {
			return nil, 0, err
		}
		out = append(out, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

// DistinctTags returns the alphabetical set of every tag value currently in
// use within the tenant. Implemented with jsonb_array_elements_text so the
// query stays scoped to the documents table — no separate tags table needed.
func (r *documentPostgresRepository) DistinctTags(ctx context.Context, tenantID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT tag
		FROM deal.documents,
		     jsonb_array_elements_text(COALESCE(metadata->'tags', '[]'::jsonb)) AS tag
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY tag ASC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, err
		}
		if strings.TrimSpace(tag) == "" {
			continue
		}
		out = append(out, tag)
	}
	return out, rows.Err()
}
