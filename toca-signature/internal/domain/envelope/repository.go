package envelope

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// WebhookEventRow é a projeção pública de signature.webhook_events. raw_body
// é OMITIDO intencionalmente — contém PII (email, nome do signatário) e não
// deve sair do serviço através de endpoints de operador.
type WebhookEventRow struct {
	ID           string    `json:"id"`
	EventType    string    `json:"event_type"`
	DeliveryID   string    `json:"delivery_id,omitempty"`
	Processed    bool      `json:"processed"`
	ProcessError string    `json:"process_error,omitempty"`
	ReceivedAt   time.Time `json:"received_at"`
}

// Repository é a interface de persistência do agregado SignatureEnvelope.
// As implementações devem respeitar tenant_id em queries que recebem tenantID,
// e usar pool service-role para os métodos sem tenant (chamados pelo webhook
// handler que chega sem JWT).
type Repository interface {
	Create(ctx context.Context, e *SignatureEnvelope) error
	Update(ctx context.Context, e *SignatureEnvelope) error
	// UpdateInTx mirrors Update but joins the caller's transaction so the
	// envelope row update and a subsequent EnqueueOutboxInTx call land
	// or roll back together (audit P0.1 follow-up). Webhook handlers
	// use this to make state-flip + event emission atomic.
	UpdateInTx(ctx context.Context, tx *sqlx.Tx, e *SignatureEnvelope) error

	GetByID(ctx context.Context, tenantID, id string) (*SignatureEnvelope, error)

	// GetByIDService lê pelo id SEM filtro de tenant, no pool service-role.
	// É o caminho do background (varredura de reconciliação), que roda sem JWT:
	// sob RLS o pool de aplicação não enxerga a linha e o reconcile devolveria
	// "não encontrado" para um envelope que existe.
	GetByIDService(ctx context.Context, id string) (*SignatureEnvelope, error)

	// UpdateService grava pelo pool service-role, pelo mesmo motivo do
	// GetByIDService (a varredura não tem JWT para satisfazer a política de RLS).
	UpdateService(ctx context.Context, e *SignatureEnvelope) error
	GetByContractID(ctx context.Context, tenantID, contractID string) (*SignatureEnvelope, error)

	// GetByExternalID busca o envelope a partir do ID retornado pela Clicksign.
	// Service-scope: NÃO recebe tenantID porque o webhook chega sem JWT, mas
	// retorna o envelope com tenant_id para que o caller propague no contexto.
	GetByExternalID(ctx context.Context, externalEnvelopeID string) (*SignatureEnvelope, error)

	// GetByDocumentKey busca o envelope pelo document.key da Clicksign — usado
	// como fallback no handler de webhook porque a API v3 envia document.key
	// (não envelope.id) no payload. Service-scope (mesmo motivo do GetByExternalID).
	GetByDocumentKey(ctx context.Context, documentKey string) (*SignatureEnvelope, error)

	ListByTenant(ctx context.Context, tenantID string, status *Status) ([]*SignatureEnvelope, error)

	// ListStaleActive returns envelopes still in a non-terminal state whose last
	// update is older than `olderThan`. Service-scope (no tenant filter): the
	// reconciliation sweeper runs across every tenant.
	//
	// Existe como rede de segurança dos webhooks. A entrega do provedor pode
	// atrasar ou se perder — quando isso acontece o envelope fica preso em
	// RUNNING, o contrato nunca sai de PENDING_SIGNATURE e o operador só
	// descobre quando o signatário reclama. Foi exatamente o que ocorreu em
	// produção, com `signature.webhook_events` vazio.
	ListStaleActive(ctx context.Context, olderThan time.Time, limit int) ([]*SignatureEnvelope, error)

	// AppendWebhookEvent grava o webhook bruto para auditoria. Retorna o ID do
	// registro criado para que MarkWebhookProcessed possa ser chamado depois.
	AppendWebhookEvent(ctx context.Context, externalEnvelopeID, eventType, deliveryID string, body []byte, sigHeader, sourceIP string) (string, error)
	MarkWebhookProcessed(ctx context.Context, webhookID string, processError string) error

	// ReassociateWebhookEvent regrava a correlação da linha de auditoria para o
	// envelope realmente resolvido. O payload v3 da Clicksign só traz
	// document.key, então a linha nasce apontando para o doc key (e com
	// tenant_id NULL); depois do fallback por document key sabemos o envelope
	// de verdade e a linha precisa refletir isso.
	ReassociateWebhookEvent(ctx context.Context, webhookID, externalEnvelopeID, tenantID string) error

	// ListWebhookEvents retorna os últimos eventos webhook recebidos para
	// um envelope externo (Clicksign), ordenados do mais recente para o mais
	// antigo. Service-scope: não filtra por tenant (auditoria operacional).
	// O cap interno previne resultados ilimitados.
	ListWebhookEvents(ctx context.Context, externalEnvelopeID string) ([]WebhookEventRow, error)
}
