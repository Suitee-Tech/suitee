package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type roulettePostgresRepository struct {
	db *database.PostgresDB
}

func NewRoulettePostgresRepository(db *database.PostgresDB) *roulettePostgresRepository {
	return &roulettePostgresRepository{db: db}
}

func (r *roulettePostgresRepository) GetConfig(ctx context.Context, tenantID string) (*lead.RouletteConfig, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, "SELECT metadata FROM crm.roulette_configs WHERE tenant_id = $1 AND deleted_at IS NULL", tenantID).Scan(&metadata)
	if err != nil {
		if err == sql.ErrNoRows {
			// Fallback defaults (não configurado ainda)
			return &lead.RouletteConfig{
				TenantID:      tenantID,
				Enabled:       false,
				ActiveBrokers: []string{},
				LastIndex:     -1,
				UpdatedAt:     time.Now(),
			}, nil
		}
		return nil, err
	}
	var config lead.RouletteConfig
	if err := json.Unmarshal(metadata, &config); err != nil {
		return nil, err
	}
	return &config, nil
}

func (r *roulettePostgresRepository) UpdateConfig(ctx context.Context, config *lead.RouletteConfig) error {
	config.UpdatedAt = time.Now()
	metadata, _ := json.Marshal(config)
	query := `
		INSERT INTO crm.roulette_configs (tenant_id, metadata) 
		VALUES ($1, $2)
		ON CONFLICT (tenant_id) DO UPDATE SET metadata = EXCLUDED.metadata, updated_at = NOW(), deleted_at = NULL
	`
	_, err := r.db.ExecContext(ctx, query, config.TenantID, string(metadata))
	return err
}

func (r *roulettePostgresRepository) SetIndexCAS(ctx context.Context, tenantID string, stage string, expected int, next int) (bool, error) {
	pathKey := "LastIndex"
	exprKey := "metadata->>'LastIndex'"

	if stage != "" {
		if !stageKeyRx.MatchString(stage) {
			return false, fmt.Errorf("invalid stage")
		}
		pathKey = fmt.Sprintf("StageLastIndex,%s", stage)
		// Paridade com currentIndex(): quando o estágio ainda não tem ponteiro,
		// o valor lido é o LastIndex global — o CAS precisa comparar contra o
		// MESMO fallback. Antes a comparação olhava só o caminho do estágio
		// (ausente → -1) e o CAS NUNCA casava num config novo: a atribuição
		// por estágio entrava em cas_exhausted e o lead ficava sem corretor
		// (achado do deal/roulette pipeline efêmero, P1-08).
		exprKey = fmt.Sprintf("COALESCE((metadata#>>'{StageLastIndex,%s}')::int, (metadata->>'LastIndex')::int, -1)", stage)
	}

	res, err := r.db.ExecContext(ctx, fmt.Sprintf(`
		UPDATE crm.roulette_configs
		SET metadata = jsonb_set(metadata, '{%s}', to_jsonb($1::int), true),
		    updated_at = NOW()
		WHERE tenant_id = $2
		  AND COALESCE((%s)::int, -1) = $3
		  AND deleted_at IS NULL
	`, pathKey, exprKey), next, tenantID, expected)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if affected == 0 {
		logger.Warn("roulette SetIndexCAS affected 0 rows", "tenant_id", tenantID, "stage", stage, "expected", expected, "next", next)
	}
	return affected > 0, nil
}

func (r *roulettePostgresRepository) ListEnabledTenantIDs(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT tenant_id::text
		FROM crm.roulette_configs
		WHERE deleted_at IS NULL
		  AND COALESCE(metadata->>'Enabled','false') = 'true'
	`)
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

var stageKeyRx = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
