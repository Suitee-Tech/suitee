package repositories

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	"github.com/oginoino/toca/toca-portal/internal/domain/lead"
	"github.com/oginoino/toca/toca-portal/internal/domain/sync"
)

type integrationPostgresRepository struct {
	db *database.PostgresDB
}

func NewIntegrationPostgresRepository(db *database.PostgresDB) integration.Repository {
	return &integrationPostgresRepository{db: db}
}

func (r *integrationPostgresRepository) Create(ctx context.Context, integ *integration.PortalIntegration) error {
	integ.CreatedAt = time.Now()
	metadata, err := marshalPortalIntegration(integ)
	if err != nil {
		return err
	}
	query := `INSERT INTO portal.integrations (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, integ.ID, integ.TenantID, metadata).Scan(&integ.CreatedAt)
}

// GetByID carrega a integração do tenant do chamador. O tenant vai no WHERE —
// nunca no lugar do tenant do próprio registro — e tenant vazio falha fechado.
func (r *integrationPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*integration.PortalIntegration, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, integration.ErrNotFound
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM portal.integrations WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, integration.ErrNotFound
		}
		return nil, fmt.Errorf("load integration: %w", err)
	}
	return unmarshalPortalIntegration(metadata)
}

// GetByIDAnyTenant é a escotilha explícita do webhook público (C5): o endpoint
// não tem tenant no contexto e o tenant sai do próprio registro. Não usar em
// rota autenticada — ali o correto é GetByID(tenantID, id).
func (r *integrationPostgresRepository) GetByIDAnyTenant(ctx context.Context, id string) (*integration.PortalIntegration, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM portal.integrations WHERE id = $1 AND deleted_at IS NULL", id).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, integration.ErrNotFound
		}
		return nil, fmt.Errorf("load integration: %w", err)
	}
	return unmarshalPortalIntegration(metadata)
}

// Update grava apenas quando a linha pertence a tenantID (o tenant do
// chamador). O tenant do agregado precisa bater com ele: um registro forjado
// com o id da vítima e o tenant do atacante não escreve nada.
func (r *integrationPostgresRepository) Update(ctx context.Context, tenantID string, integ *integration.PortalIntegration) error {
	if strings.TrimSpace(tenantID) == "" || integ == nil || integ.TenantID != tenantID {
		return integration.ErrNotFound
	}
	integ.UpdatedAt = time.Now()
	metadata, err := marshalPortalIntegration(integ)
	if err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, "UPDATE portal.integrations SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", metadata, integ.ID, tenantID)
	if err != nil {
		return err
	}
	return errIfNoRows(res)
}

func (r *integrationPostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	if strings.TrimSpace(tenantID) == "" {
		return integration.ErrNotFound
	}
	res, err := r.db.ExecContext(ctx, "UPDATE portal.integrations SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL", id, tenantID)
	if err != nil {
		return err
	}
	return errIfNoRows(res)
}

func (r *integrationPostgresRepository) ListByTenant(ctx context.Context, tenantID string) ([]*integration.PortalIntegration, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM portal.integrations
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*integration.PortalIntegration, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		i, err := unmarshalPortalIntegration(metadata)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *integrationPostgresRepository) ListActive(ctx context.Context) ([]*integration.PortalIntegration, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM portal.integrations
		WHERE deleted_at IS NULL AND (metadata->>'status') = 'active'
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*integration.PortalIntegration, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		i, err := unmarshalPortalIntegration(metadata)
		if err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *integrationPostgresRepository) GetByFeedToken(ctx context.Context, token string) (*integration.PortalIntegration, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM portal.integrations
		WHERE deleted_at IS NULL AND (metadata->>'feed_token') = $1
		LIMIT 1
	`, token).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, integration.ErrNotFound
		}
		return nil, fmt.Errorf("load integration by feed token: %w", err)
	}
	return unmarshalPortalIntegration(metadata)
}

// GetByTenantAndPortal devolve (nil, nil) quando não há integração do tenant
// para esse portal: é a checagem de duplicidade do Create, não um erro. Antes
// devolvia o erro do driver ("sql: no rows in result set") e o Create quebrava
// (e vazava o erro) para todo tenant novo.
func (r *integrationPostgresRepository) GetByTenantAndPortal(ctx context.Context, tenantID string, portal integration.PortalType) (*integration.PortalIntegration, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM portal.integrations
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'portal') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, string(portal)).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load integration by tenant and portal: %w", err)
	}
	return unmarshalPortalIntegration(metadata)
}

// errIfNoRows converte "0 linhas afetadas" no sentinela de não encontrado,
// preservando o 404 em vez de devolver sucesso silencioso numa linha alheia.
func errIfNoRows(res sql.Result) error {
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return integration.ErrNotFound
	}
	return nil
}

type syncPostgresRepository struct {
	db *database.PostgresDB
}

func NewSyncPostgresRepository(db *database.PostgresDB) sync.Repository {
	return &syncPostgresRepository{db: db}
}

func (r *syncPostgresRepository) Create(ctx context.Context, s *sync.PortalSync) error {
	metadata, _ := json.Marshal(s)
	query := `INSERT INTO portal.sync_history (id, tenant_id, metadata) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, s.ID, s.TenantID, string(metadata))
	return err
}

func (r *syncPostgresRepository) Update(ctx context.Context, s *sync.PortalSync) error {
	metadata, _ := json.Marshal(s)
	_, err := r.db.ExecContext(ctx, "UPDATE portal.sync_history SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), s.ID, s.TenantID)
	return err
}

func (r *syncPostgresRepository) GetLatestByIntegrationID(ctx context.Context, tenantID, integrationID string) (*sync.PortalSync, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM portal.sync_history
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'integration_id') = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, integrationID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, integration.ErrNotFound
		}
		return nil, fmt.Errorf("load latest sync: %w", err)
	}
	var s sync.PortalSync
	if err := json.Unmarshal(metadata, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *syncPostgresRepository) ListByIntegrationID(ctx context.Context, tenantID, integrationID string, limit, offset int64) ([]*sync.PortalSync, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM portal.sync_history
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'integration_id') = $2
	`, tenantID, integrationID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT metadata
		FROM portal.sync_history
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'integration_id') = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, tenantID, integrationID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*sync.PortalSync, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var s sync.PortalSync
		if err := json.Unmarshal(metadata, &s); err != nil {
			return nil, 0, err
		}
		out = append(out, &s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

type portalLeadPostgresRepository struct {
	db *database.PostgresDB
}

func NewPortalLeadPostgresRepository(db *database.PostgresDB) lead.Repository {
	return &portalLeadPostgresRepository{db: db}
}

func (r *portalLeadPostgresRepository) Create(ctx context.Context, pl *lead.PortalLead) error {
	metadata, _ := json.Marshal(pl)
	query := `INSERT INTO portal.leads (id, tenant_id, metadata) VALUES ($1, $2, $3)`
	_, err := r.db.ExecContext(ctx, query, pl.ID, pl.TenantID, string(metadata))
	return err
}

func (r *portalLeadPostgresRepository) Update(ctx context.Context, pl *lead.PortalLead) error {
	metadata, _ := json.Marshal(pl)
	_, err := r.db.ExecContext(ctx, "UPDATE portal.leads SET metadata = $1, updated_at = NOW() WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL", string(metadata), pl.ID, pl.TenantID)
	return err
}

func (r *portalLeadPostgresRepository) GetByPortalAndContact(ctx context.Context, portalID, email, phone string) (*lead.PortalLead, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM portal.leads
		WHERE deleted_at IS NULL
		  AND (metadata->>'integration_id') = $1
		  AND (
		    (metadata->>'email') = $2
		    OR (metadata->>'phone') = $3
		  )
		ORDER BY created_at DESC
		LIMIT 1
	`, portalID, email, phone).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("lead not found: %w", err)
	}
	var pl lead.PortalLead
	if err := json.Unmarshal(metadata, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

func (r *portalLeadPostgresRepository) List(ctx context.Context, tenantID, status, portalID string, limit, offset int64) ([]*lead.PortalLead, int64, error) {
	if limit <= 0 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}

	args := []any{tenantID}
	where := "WHERE tenant_id = $1 AND deleted_at IS NULL"
	argN := 2
	if status != "" {
		where += fmt.Sprintf(" AND (metadata->>'status') = $%d", argN)
		args = append(args, status)
		argN++
	}
	if portalID != "" {
		where += fmt.Sprintf(" AND (metadata->>'integration_id') = $%d", argN)
		args = append(args, portalID)
		argN++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(1) FROM portal.leads "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, limit, offset)
	query := fmt.Sprintf("SELECT metadata FROM portal.leads %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d", where, argN, argN+1)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]*lead.PortalLead, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var pl lead.PortalLead
		if err := json.Unmarshal(metadata, &pl); err != nil {
			return nil, 0, err
		}
		out = append(out, &pl)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *portalLeadPostgresRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int64) ([]*lead.PortalLead, int64, error) {
	return r.List(ctx, tenantID, "", "", limit, offset)
}

func (r *portalLeadPostgresRepository) ListByIntegration(ctx context.Context, tenantID, integrationID string, limit, offset int64) ([]*lead.PortalLead, int64, error) {
	return r.List(ctx, tenantID, "", integrationID, limit, offset)
}

func (r *portalLeadPostgresRepository) FindDuplicate(ctx context.Context, tenantID, portalID, email, phone, propertyID string) (*lead.PortalLead, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM portal.leads
		WHERE tenant_id = $1 AND deleted_at IS NULL
		  AND (metadata->>'integration_id') = $2
		  AND (metadata->>'property_id') = $3
		  AND (
		    (metadata->>'email') = $4
		    OR (metadata->>'phone') = $5
		  )
		ORDER BY created_at DESC
		LIMIT 1
	`, tenantID, portalID, propertyID, email, phone).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var pl lead.PortalLead
	if err := json.Unmarshal(metadata, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

func (r *portalLeadPostgresRepository) GetStats(ctx context.Context, tenantID string) (int64, map[string]int64, error) {
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM portal.leads
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`, tenantID).Scan(&total); err != nil {
		return 0, nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT (metadata->>'status') AS status, COUNT(1)
		FROM portal.leads
		WHERE tenant_id = $1 AND deleted_at IS NULL
		GROUP BY (metadata->>'status')
	`, tenantID)
	if err != nil {
		return 0, nil, err
	}
	defer rows.Close()

	counts := map[string]int64{}
	for rows.Next() {
		var status string
		var cnt int64
		if err := rows.Scan(&status, &cnt); err != nil {
			return 0, nil, err
		}
		counts[status] = cnt
	}
	if err := rows.Err(); err != nil {
		return 0, nil, err
	}

	return total, counts, nil
}

type feedCachePostgresRepository struct {
	db *database.PostgresDB
}

func NewFeedCachePostgresRepository(db *database.PostgresDB) integration.FeedCacheRepository {
	return &feedCachePostgresRepository{db: db}
}

func (r *feedCachePostgresRepository) Get(ctx context.Context, integrationID string) (*integration.FeedCache, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		FROM portal.feed_cache
		WHERE id = $1
	`, integrationID).Scan(&metadata)
	if err != nil {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil, err
	}
	raw, _ := m["content_b64"].(string)
	updatedAtRaw, _ := m["updated_at"].(string)
	content, _ := base64.StdEncoding.DecodeString(raw)
	updatedAt, _ := time.Parse(time.RFC3339Nano, updatedAtRaw)

	return &integration.FeedCache{
		IntegrationID: integrationID,
		Content:       content,
		UpdatedAt:     updatedAt,
	}, nil
}

func (r *feedCachePostgresRepository) Set(ctx context.Context, cache *integration.FeedCache) error {
	if cache.UpdatedAt.IsZero() {
		cache.UpdatedAt = time.Now()
	}
	m := map[string]any{
		"integration_id": cache.IntegrationID,
		"content_b64":    base64.StdEncoding.EncodeToString(cache.Content),
		"updated_at":     cache.UpdatedAt.Format(time.RFC3339Nano),
	}
	metadata, err := json.Marshal(m)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO portal.feed_cache (id, tenant_id, metadata)
		VALUES ($1, NULL, $2)
		ON CONFLICT (id) DO UPDATE SET metadata = EXCLUDED.metadata, updated_at = NOW(), deleted_at = NULL
	`, cache.IntegrationID, string(metadata))
	return err
}

func marshalPortalIntegration(integ *integration.PortalIntegration) ([]byte, error) {
	if integ == nil {
		return nil, fmt.Errorf("integration is nil")
	}
	b, err := json.Marshal(integ)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	m["feed_token"] = integ.FeedToken
	m["webhook_secret"] = integ.WebhookSecret
	return json.Marshal(m)
}

func unmarshalPortalIntegration(metadata []byte) (*integration.PortalIntegration, error) {
	var m map[string]any
	if err := json.Unmarshal(metadata, &m); err != nil {
		return nil, err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var i integration.PortalIntegration
	if err := json.Unmarshal(b, &i); err != nil {
		return nil, err
	}
	if v, ok := m["feed_token"].(string); ok {
		i.FeedToken = v
	}
	if v, ok := m["webhook_secret"].(string); ok {
		i.WebhookSecret = v
	}
	return &i, nil
}
