package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
)

type brokerPostgresRepository struct {
	db *database.PostgresDB
}

func NewBrokerPostgresRepository(db *database.PostgresDB) *brokerPostgresRepository {
	return &brokerPostgresRepository{db: db}
}

func (r *brokerPostgresRepository) Create(ctx context.Context, b *broker.Broker) error {
	metadata, _ := json.Marshal(b)
	// A identidade do corretor é (tenant_id, id): `id` é o account_id da pessoa
	// e a mesma pessoa pode ser corretora em mais de uma imobiliária. Com o
	// conflito resolvido só por `id`, a segunda empresa caía na linha da
	// primeira e a política de RLS recusava a atualização (500 no sync de
	// membros, que cria o corretor ao abrir as telas que os carregam).
	query := `
		INSERT INTO crm.brokers (id, tenant_id, metadata)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, id) DO UPDATE
		SET metadata = EXCLUDED.metadata
		RETURNING created_at
	`
	return r.db.QueryRowContext(ctx, query, b.ID, b.TenantID, string(metadata)).Scan(&b.CreatedAt)
}

// GetByID resolve o corretor dentro do tenant informado.
//
// SECURITY (achado A19): filtra por `tenant_id` e falha fechado com tenant
// vazio — antes a query era `WHERE id = $1` e vazava PII de corretor de
// qualquer tenant.
func (r *brokerPostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*broker.Broker, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("broker not found: tenant is required")
	}
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM crm.brokers WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("broker not found: %w", err)
	}
	var b broker.Broker
	_ = json.Unmarshal(metadata, &b)
	return &b, nil
}

// GetByIDService resolve o corretor pelo POOL DE SERVIÇO, para os caminhos
// cross-tenant do portal do cliente: o tenant do token do cliente é a conta
// local dele, então sob RLS o pool de aplicação não enxerga o corretor da
// imobiliária dona do lead. O tenant continua explícito na query — o
// chamador passa o tenant do LEAD (já autorizado), não o do token.
func (r *brokerPostgresRepository) GetByIDService(ctx context.Context, tenantID, id string) (*broker.Broker, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("broker not found: tenant is required")
	}
	var metadata []byte
	err := r.db.QueryServiceRow(ctx, "SELECT metadata FROM crm.brokers WHERE id = $1 AND tenant_id = $2", id, tenantID).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("broker not found: %w", err)
	}
	var b broker.Broker
	_ = json.Unmarshal(metadata, &b)
	return &b, nil
}

func (r *brokerPostgresRepository) Update(ctx context.Context, b *broker.Broker) error {
	b.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(b)
	// P2/item 36: a escrita é escopada pelo tenant do próprio agregado.
	_, err := r.db.ExecContext(ctx, "UPDATE crm.brokers SET metadata = $1 WHERE id = $2 AND tenant_id = $3", string(metadata), b.ID, b.TenantID)
	return err
}

func (r *brokerPostgresRepository) SetPaused(ctx context.Context, brokerID, tenantID string, paused bool) (bool, error) {
	return r.SetPausedUntil(ctx, brokerID, tenantID, paused, nil)
}

// SetPausedUntil atualiza Paused e PausedUntil em uma única operação. Quando
// `until` é nil, remove a chave do JSONB para preservar o contrato pre-P1.7
// (pausa indefinida). Caso contrário, persiste o timestamp em ISO8601.
func (r *brokerPostgresRepository) SetPausedUntil(ctx context.Context, brokerID, tenantID string, paused bool, until *time.Time) (bool, error) {
	var (
		query string
		args  []any
	)
	if until == nil {
		query = `
			UPDATE crm.brokers
			SET metadata = (jsonb_set(metadata, '{Paused}', to_jsonb($1::bool), true)) #- '{PausedUntil}'
			WHERE id = $2
			  AND tenant_id = $3
			  AND deleted_at IS NULL
		`
		args = []any{paused, brokerID, tenantID}
	} else {
		query = `
			UPDATE crm.brokers
			SET metadata = jsonb_set(
				jsonb_set(metadata, '{Paused}', to_jsonb($1::bool), true),
				'{PausedUntil}', to_jsonb($2::timestamptz), true
			)
			WHERE id = $3
			  AND tenant_id = $4
			  AND deleted_at IS NULL
		`
		args = []any{paused, until.UTC(), brokerID, tenantID}
	}
	res, err := r.db.ExecContext(ctx, query, args...)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *brokerPostgresRepository) Delete(ctx context.Context, id, tenantID string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM crm.brokers WHERE id = $1 AND tenant_id = $2", id, tenantID)
	return err
}

func (r *brokerPostgresRepository) List(ctx context.Context, tenantID string) ([]*broker.Broker, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT metadata FROM crm.brokers WHERE tenant_id = $1 ORDER BY created_at DESC", tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var brokers []*broker.Broker
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var b broker.Broker
		if err := json.Unmarshal(metadata, &b); err != nil {
			return nil, err
		}
		brokers = append(brokers, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return brokers, nil
}

func (r *brokerPostgresRepository) Search(ctx context.Context, tenantID string, filter broker.BrokerFilter) ([]*broker.Broker, int64, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	args := make([]interface{}, 0, 10)
	args = append(args, tenantID)
	argN := 2

	var where strings.Builder
	where.WriteString(" WHERE tenant_id = $1 AND deleted_at IS NULL")

	if s := strings.TrimSpace(filter.Search); s != "" {
		where.WriteString(fmt.Sprintf(" AND ((metadata->>'Name') ILIKE $%d OR (metadata->>'Email') ILIKE $%d)", argN, argN))
		args = append(args, "%"+s+"%")
		argN++
	}

	if len(filter.Specialties) > 0 {
		where.WriteString(fmt.Sprintf(" AND (metadata->'Specialties') ?| $%d", argN))
		args = append(args, filter.Specialties)
		argN++
	}

	if len(filter.Regions) > 0 {
		where.WriteString(fmt.Sprintf(" AND (metadata->'Regions') ?| $%d", argN))
		args = append(args, filter.Regions)
		argN++
	}

	if filter.Active != nil {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Active') = $%d", argN))
		args = append(args, fmt.Sprintf("%t", *filter.Active))
		argN++
	}

	if filter.Paused != nil {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Paused') = $%d", argN))
		args = append(args, fmt.Sprintf("%t", *filter.Paused))
		argN++
	}

	if r := strings.TrimSpace(filter.Role); r != "" {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'Role') = $%d", argN))
		args = append(args, r)
		argN++
	}

	if filter.IsPriority != nil {
		where.WriteString(fmt.Sprintf(" AND (metadata->>'IsPriority') = $%d", argN))
		args = append(args, fmt.Sprintf("%t", *filter.IsPriority))
		argN++
	}

	var total int64
	countQuery := "SELECT COUNT(*) FROM crm.brokers" + where.String()
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	where.WriteString(fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", argN, argN+1))
	args = append(args, limit, offset)

	query := "SELECT metadata FROM crm.brokers" + where.String()
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result := make([]*broker.Broker, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, 0, err
		}
		var b broker.Broker
		if err := json.Unmarshal(metadata, &b); err != nil {
			return nil, 0, err
		}
		result = append(result, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return result, total, nil
}

func (r *brokerPostgresRepository) GetPerformance(ctx context.Context, brokerID, tenantID string) (*broker.BrokerPerformance, error) {
	var totalLeads int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'AssignedBrokerID') = $2
	`, tenantID, brokerID).Scan(&totalLeads); err != nil {
		return nil, err
	}

	var activeLeads int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'AssignedBrokerID') = $2
		  AND COALESCE(metadata->>'Status','') NOT IN ('CONVERTED','LOST')
	`, tenantID, brokerID).Scan(&activeLeads); err != nil {
		return nil, err
	}

	var convertedLeads int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(1)
		FROM crm.leads
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'AssignedBrokerID') = $2
		  AND (metadata->>'Status') = 'CONVERTED'
	`, tenantID, brokerID).Scan(&convertedLeads); err != nil {
		return nil, err
	}

	conversion := 0.0
	if totalLeads > 0 {
		conversion = float64(convertedLeads) / float64(totalLeads)
	}

	var sales float64
	_ = r.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM((metadata->>'value')::numeric), 0)
		FROM deal.deals
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND (metadata->>'broker_id') = $2
		  AND (metadata->>'status') = 'CONVERTED'
	`, tenantID, brokerID).Scan(&sales)

	return &broker.BrokerPerformance{
		SalesVolume:    sales,
		ActiveLeads:    int(activeLeads),
		ConversionRate: conversion,
	}, nil
}

func (r *brokerPostgresRepository) GetByIDs(ctx context.Context, tenantID string, ids []string) (map[string]*broker.Broker, error) {
	if len(ids) == 0 {
		return map[string]*broker.Broker{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id::text, metadata
		FROM crm.brokers
		WHERE tenant_id = $1
		  AND deleted_at IS NULL
		  AND id::text = ANY($2)
	`, tenantID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]*broker.Broker{}
	for rows.Next() {
		var id string
		var metadata []byte
		if err := rows.Scan(&id, &metadata); err != nil {
			return nil, err
		}
		var b broker.Broker
		if err := json.Unmarshal(metadata, &b); err != nil {
			return nil, err
		}
		if b.ID == "" {
			b.ID = id
		}
		out[id] = &b
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
