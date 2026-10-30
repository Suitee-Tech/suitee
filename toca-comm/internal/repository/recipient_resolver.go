package repository

import (
	"context"
	"fmt"

	"github.com/oginoino/toca/pkg/database"
)

// EmailRecipientResolver resolve o e-mail de um destinatário de notificação
// transacional a partir da identidade mista do domínio: corretor
// (crm.brokers), conta de operador/cliente do portal (toca_auth.accounts) e
// lead (crm.leads). Usa o service pool (BYPASSRLS) porque os consumidores de
// eventos rodam sem JWT e o destinatário pode ser de outro tenant do chamador.
type EmailRecipientResolver interface {
	// ParticipantEmailByID devolve o e-mail associado à identidade, nesta
	// ordem: corretor (id em crm.brokers) → conta (id em toca_auth.accounts).
	// Devolve "" quando não encontra e-mail.
	ParticipantEmailByID(ctx context.Context, tenantID, participantID string) (string, error)
	// LeadEmailByID devolve o e-mail do lead (crm.leads). Devolve "" quando
	// não encontra.
	LeadEmailByID(ctx context.Context, tenantID, leadID string) (string, error)
}

type emailRecipientPostgresResolver struct {
	db *database.PostgresDB
}

func NewEmailRecipientPostgresResolver(db *database.PostgresDB) EmailRecipientResolver {
	return &emailRecipientPostgresResolver{db: db}
}

func (r *emailRecipientPostgresResolver) ParticipantEmailByID(ctx context.Context, tenantID, participantID string) (string, error) {
	if participantID == "" {
		return "", fmt.Errorf("participant_id is required")
	}
	if r.db == nil || r.db.ServicePool == nil {
		// Sem DB wired (testes) não há como resolver — devolve vazio.
		return "", nil
	}
	var email string
	err := r.db.ServicePool.QueryRow(ctx, `
		SELECT COALESCE(
			(SELECT NULLIF(metadata->>'Email', '')
			   FROM crm.brokers
			  WHERE tenant_id = $1 AND id::text = $2 AND deleted_at IS NULL
			  LIMIT 1),
			(SELECT COALESCE(NULLIF(a.metadata->'Individual'->>'Email', ''),
			                 NULLIF(a.metadata->>'Email', ''), '')
			   FROM toca_auth.accounts a
			  WHERE a.id::text = $2
			  LIMIT 1),
			''
		)
	`, tenantID, participantID).Scan(&email)
	if err != nil {
		return "", fmt.Errorf("resolve participant email: %w", err)
	}
	return email, nil
}

func (r *emailRecipientPostgresResolver) LeadEmailByID(ctx context.Context, tenantID, leadID string) (string, error) {
	if tenantID == "" || leadID == "" {
		return "", fmt.Errorf("tenant_id and lead_id are required")
	}
	if r.db == nil || r.db.ServicePool == nil {
		return "", nil
	}
	var email string
	err := r.db.ServicePool.QueryRow(ctx, `
		SELECT COALESCE(metadata->>'Email', '')
		FROM crm.leads
		WHERE tenant_id = $1 AND id::text = $2 AND deleted_at IS NULL
		LIMIT 1
	`, tenantID, leadID).Scan(&email)
	if err != nil {
		return "", fmt.Errorf("resolve lead email: %w", err)
	}
	return email, nil
}
