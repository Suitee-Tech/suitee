package repository

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/database"
)

// EmailPrefsRepository guarda as preferências de e-mail transacional por
// usuário/categoria (comm.notification_prefs). Ausência de linha = ligado.
type EmailPrefsRepository interface {
	// IsEnabled devolve true quando a categoria está ligada para o usuário
	// (ou quando não existe linha — default ligado).
	IsEnabled(ctx context.Context, userID, tenantID, category string) (bool, error)
	// Set liga/desliga uma categoria. A linha é upsertada.
	Set(ctx context.Context, userID, tenantID, category string, enabled bool) error
}

type emailPrefsPostgresRepository struct {
	db *database.PostgresDB
}

func NewEmailPrefsPostgresRepository(db *database.PostgresDB) EmailPrefsRepository {
	return &emailPrefsPostgresRepository{db: db}
}

func (r *emailPrefsPostgresRepository) IsEnabled(ctx context.Context, userID, tenantID, category string) (bool, error) {
	if userID == "" || tenantID == "" || category == "" {
		return false, fmt.Errorf("user_id, tenant_id and category are required")
	}
	if r.db == nil || r.db.ServicePool == nil {
		// Sem DB wired (testes) o default é ligado — permissive no-op.
		return true, nil
	}
	var enabled bool
	err := r.db.ServicePool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT enabled FROM comm.notification_prefs
			 WHERE user_id = $1 AND tenant_id = $2 AND category = $3),
			TRUE
		)
	`, userID, tenantID, category).Scan(&enabled)
	if err != nil {
		return false, fmt.Errorf("read notification_pref: %w", err)
	}
	return enabled, nil
}

func (r *emailPrefsPostgresRepository) Set(ctx context.Context, userID, tenantID, category string, enabled bool) error {
	if userID == "" || tenantID == "" || category == "" {
		return fmt.Errorf("user_id, tenant_id and category are required")
	}
	if r.db == nil || r.db.ServicePool == nil {
		// Sem DB wired (testes) aceita silenciosamente.
		return nil
	}
	_, err := r.db.ServicePool.Exec(ctx, `
		INSERT INTO comm.notification_prefs (user_id, tenant_id, category, enabled)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, tenant_id, category)
		DO UPDATE SET enabled = EXCLUDED.enabled, updated_at = NOW()
	`, userID, tenantID, category, enabled)
	if err != nil {
		return fmt.Errorf("write notification_pref: %w", err)
	}
	return nil
}
