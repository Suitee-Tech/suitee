package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/database"
)

// SentExternalsRepository tracks which external dispatches (WhatsApp/SMS/Email)
// have already been performed for a given source event + channel + recipient.
// Used to make external dispatch idempotent across RabbitMQ redeliveries.
type SentExternalsRepository interface {
	// RegisterIfFirst attempts to record a dispatch. Returns true when the row
	// was newly inserted (caller should perform the real dispatch) and false
	// when the same (event_id, channel, recipient_id) was already recorded
	// (caller MUST skip the dispatch to avoid duplicate messages). category é
	// opcional ("" para dispatches legados) e alimenta o gate de cooldown.
	RegisterIfFirst(ctx context.Context, eventID, channel, recipientID, tenantID, category string) (bool, error)

	// HasRecentDispatch reports whether a dispatch for (channel, recipient_id,
	// category) exists within the last `window` for the tenant. Used as
	// cooldown gate: evita rajadas de e-mails da MESMA categoria para o mesmo
	// destinatário. Dispatch events com Force ignoram este gate. category
	// vazia consulta apenas dispatches legados (sem categoria).
	HasRecentDispatch(ctx context.Context, tenantID, channel, recipientID, category string, window time.Duration) (bool, error)
}

type sentExternalsPostgresRepository struct {
	db *database.PostgresDB
}

func NewSentExternalsPostgresRepository(db *database.PostgresDB) SentExternalsRepository {
	return &sentExternalsPostgresRepository{db: db}
}

// RegisterIfFirst uses the service-role pool because subscribers running
// without a JWT context still need to register dispatches. The CHECK on
// channel is enforced at the DB level (see migration).
func (r *sentExternalsPostgresRepository) RegisterIfFirst(ctx context.Context, eventID, channel, recipientID, tenantID, category string) (bool, error) {
	if eventID == "" || channel == "" || recipientID == "" || tenantID == "" {
		return false, fmt.Errorf("event_id, channel, recipient_id and tenant_id are required")
	}
	if r.db == nil || r.db.ServicePool == nil {
		// In tests without a wired DB the helper is a permissive no-op so the
		// dispatch still happens — production always has a service pool.
		return true, nil
	}
	query := `
		INSERT INTO comm.sent_externals (event_id, channel, recipient_id, tenant_id, category)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		ON CONFLICT (event_id, channel, recipient_id) DO NOTHING
	`
	res, err := r.db.ServicePool.Exec(ctx, query, eventID, channel, recipientID, tenantID, category)
	if err != nil {
		return false, fmt.Errorf("register sent_external: %w", err)
	}
	return res.RowsAffected() > 0, nil
}

// HasRecentDispatch consulta o ledger pelo índice (tenant_id, channel,
// category, sent_at DESC). janela em segundos para manter o parâmetro
// numérico (sem parsing de interval) — make_interval(secs => $4) resolve no
// Postgres.
func (r *sentExternalsPostgresRepository) HasRecentDispatch(ctx context.Context, tenantID, channel, recipientID, category string, window time.Duration) (bool, error) {
	if tenantID == "" || channel == "" || recipientID == "" || window <= 0 {
		return false, nil
	}
	if r.db == nil || r.db.ServicePool == nil {
		// Sem DB wired (testes) o cooldown fica permissivo — não bloqueia.
		return false, nil
	}
	var exists bool
	err := r.db.ServicePool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM comm.sent_externals
			WHERE tenant_id = $1
			  AND channel = $2
			  AND recipient_id = $3
			  AND COALESCE(category, '') = $5
			  AND sent_at > NOW() - make_interval(secs => $4)
		)
	`, tenantID, channel, recipientID, window.Seconds(), category).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check recent sent_external: %w", err)
	}
	return exists, nil
}
