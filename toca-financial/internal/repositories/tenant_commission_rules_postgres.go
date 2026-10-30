package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
)

// TenantCommissionRulesRepository é o port consumido pelo
// commission_rules_service. Definido localmente (e não em domain/commission)
// para preservar a separação atual: o pacote `commission` só carrega
// regras e cálculo puro; persistência fica no service layer.
type TenantCommissionRulesRepository interface {
	List(ctx context.Context, tenantID string) ([]commission.Rule, error)
	Upsert(ctx context.Context, tenantID string, rules []commission.Rule) error
}

type tenantCommissionRulesPostgresRepository struct {
	db *database.PostgresDB
}

func NewTenantCommissionRulesPostgresRepository(db *database.PostgresDB) TenantCommissionRulesRepository {
	return &tenantCommissionRulesPostgresRepository{db: db}
}

func (r *tenantCommissionRulesPostgresRepository) List(ctx context.Context, tenantID string) ([]commission.Rule, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	rows, err := r.db.Query(ctx, `
		SELECT role, metadata
		FROM financial.tenant_commission_rules
		WHERE tenant_id = $1 AND deleted_at IS NULL
	`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]commission.Rule, 0)
	for rows.Next() {
		var role string
		var metadata []byte
		if err := rows.Scan(&role, &metadata); err != nil {
			return nil, err
		}
		// metadata shape (per migration 20260510120200):
		//   { "default_percentage": float, "updated_at": rfc3339 }
		var payload struct {
			DefaultPercentage float64   `json:"default_percentage"`
			UpdatedAt         time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal(metadata, &payload); err != nil {
			return nil, err
		}
		out = append(out, commission.Rule{
			TenantID:          tenantID,
			Role:              commission.Role(role),
			DefaultPercentage: payload.DefaultPercentage,
			UpdatedAt:         payload.UpdatedAt,
		})
	}
	return out, rows.Err()
}

// Upsert substitui o conjunto de regras do tenant numa única transação.
// Semântica "replace-all" — caller envia o conjunto desejado e o repo
// garante que só ele fica. Hard-delete da row antiga via deleted_at = NOW()
// quando o conjunto novo não inclui o role; reativação no INSERT ON CONFLICT
// limpa o deleted_at. Mantém histórico em audit (chamado pelo service).
func (r *tenantCommissionRulesPostgresRepository) Upsert(ctx context.Context, tenantID string, rules []commission.Rule) error {
	if tenantID == "" {
		return errors.New("tenant_id is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()

	// Soft-delete tudo do tenant; o INSERT abaixo reativa o que continuar.
	if _, err := tx.ExecContext(ctx, `
		UPDATE financial.tenant_commission_rules
		   SET deleted_at = NOW()
		 WHERE tenant_id = $1 AND deleted_at IS NULL
	`, tenantID); err != nil {
		return err
	}

	for _, rule := range rules {
		payload := map[string]any{
			"default_percentage": rule.DefaultPercentage,
			"updated_at":         now.Format(time.RFC3339),
		}
		metadata, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO financial.tenant_commission_rules (tenant_id, role, metadata)
			VALUES ($1, $2, $3)
			ON CONFLICT (tenant_id, role) DO UPDATE
			   SET metadata = EXCLUDED.metadata,
			       deleted_at = NULL,
			       updated_at = NOW()
		`, tenantID, string(rule.Role), string(metadata)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
