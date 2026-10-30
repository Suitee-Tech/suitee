package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/reminder_rule"
)

type reminderRulePostgresRepository struct {
	db *database.PostgresDB
}

func NewReminderRulePostgresRepository(db *database.PostgresDB) reminder_rule.Repository {
	return &reminderRulePostgresRepository{db: db}
}

func (r *reminderRulePostgresRepository) Create(ctx context.Context, rule *reminder_rule.ReminderRule) error {
	rule.CreatedAt = time.Now()
	rule.UpdatedAt = rule.CreatedAt
	metadata, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	query := `INSERT INTO financial.reminder_rules (id, tenant_id, metadata) VALUES ($1, $2, $3) RETURNING created_at`
	return r.db.QueryRowContext(ctx, query, rule.ID, rule.TenantID, string(metadata)).Scan(&rule.CreatedAt)
}

func (r *reminderRulePostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*reminder_rule.ReminderRule, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT metadata FROM financial.reminder_rules WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&metadata)
	if err != nil {
		return nil, fmt.Errorf("reminder_rule not found: %w", err)
	}
	var rule reminder_rule.ReminderRule
	if err := json.Unmarshal(metadata, &rule); err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *reminderRulePostgresRepository) ListByTenant(ctx context.Context, tenantID string) ([]*reminder_rule.ReminderRule, error) {
	return r.list(ctx,
		`SELECT metadata FROM financial.reminder_rules
		WHERE tenant_id = $1 AND deleted_at IS NULL
		ORDER BY created_at ASC`,
		tenantID,
	)
}

func (r *reminderRulePostgresRepository) ListEnabled(ctx context.Context, tenantID string) ([]*reminder_rule.ReminderRule, error) {
	return r.list(ctx,
		`SELECT metadata FROM financial.reminder_rules
		WHERE tenant_id = $1 AND deleted_at IS NULL AND (metadata->>'enabled') = 'true'
		ORDER BY created_at ASC`,
		tenantID,
	)
}

func (r *reminderRulePostgresRepository) list(ctx context.Context, query string, args ...interface{}) ([]*reminder_rule.ReminderRule, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*reminder_rule.ReminderRule, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		var rule reminder_rule.ReminderRule
		if err := json.Unmarshal(metadata, &rule); err != nil {
			return nil, err
		}
		out = append(out, &rule)
	}
	return out, rows.Err()
}

func (r *reminderRulePostgresRepository) Update(ctx context.Context, rule *reminder_rule.ReminderRule) error {
	rule.UpdatedAt = time.Now()
	metadata, err := json.Marshal(rule)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx,
		`UPDATE financial.reminder_rules SET metadata = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3 AND deleted_at IS NULL`,
		string(metadata), rule.ID, rule.TenantID,
	)
	return err
}

func (r *reminderRulePostgresRepository) Delete(ctx context.Context, tenantID, id string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE financial.reminder_rules SET deleted_at = NOW() WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	)
	return err
}

func (r *reminderRulePostgresRepository) ListDistinctTenantIDs(ctx context.Context) ([]string, error) {
	// Pool de SERVIÇO: a pergunta é inerentemente cross-tenant, feita no boot
	// pelo worker sem tenant no contexto. Pelo pool de aplicação o RLS filtra
	// tudo e a régua diária não enxerga nenhum tenant (foi o "tenants=0" do
	// beta, com regra ativa no banco).
	rows, err := r.db.QueryServiceRows(ctx,
		`SELECT DISTINCT tenant_id::text FROM financial.reminder_rules
		WHERE deleted_at IS NULL AND (metadata->>'enabled') = 'true'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
