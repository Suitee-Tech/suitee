package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
)

type envelopePostgresRepository struct {
	db *database.PostgresDB
}

func NewEnvelopePostgresRepository(db *database.PostgresDB) envelope.Repository {
	return &envelopePostgresRepository{db: db}
}

func (r *envelopePostgresRepository) Create(ctx context.Context, e *envelope.SignatureEnvelope) error {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	now := time.Now()
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
	metadata, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}
	kind := string(e.Kind)
	if kind == "" {
		kind = string(envelope.EnvelopeKindContract)
	}
	// deal_id é UUID; NULLIF($4,'') tem tipo `text` e o Postgres não faz cast
	// implícito text→uuid (SQLSTATE 42804). Cast explícito necessário.
	query := `
		INSERT INTO signature.envelopes
		    (id, tenant_id, contract_id, deal_id, external_envelope_id, environment, status, metadata, last_error, created_at, updated_at, finished_at, kind)
		VALUES ($1, $2, $3, NULLIF($4, '')::uuid, NULLIF($5, ''), $6, $7, $8, NULLIF($9, ''), $10, $11, $12, $13)
	`
	_, err = r.db.ExecContext(ctx, query,
		e.ID, e.TenantID, e.ContractID, e.DealID, e.ExternalEnvelopeID, e.Environment,
		string(e.Status), string(metadata), e.LastError, e.CreatedAt, e.UpdatedAt, e.FinishedAt, kind,
	)
	return err
}

func (r *envelopePostgresRepository) Update(ctx context.Context, e *envelope.SignatureEnvelope) error {
	res, err := r.execUpdate(ctx, nil, e)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("envelope %s not found", e.ID)
	}
	return nil
}

// UpdateService é o Update do caminho sem JWT (varredura de reconciliação):
// escreve pelo pool service-role. Pelo pool de aplicação o UPDATE não encontra
// a linha (a política de RLS compara tenant_id com auth.jwt()) e o caller
// recebe "envelope not found" com o envelope existindo.
//
// ExecService devolve só erro (sem RowsAffected), então usamos RETURNING id
// para preservar a detecção de "não encontrado" do caminho normal.
func (r *envelopePostgresRepository) UpdateService(ctx context.Context, e *envelope.SignatureEnvelope) error {
	args, err := updateArgs(e)
	if err != nil {
		return err
	}
	var id string
	if err := r.db.QueryServiceRow(ctx, envelopeUpdateQuery+` RETURNING id`, args...).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("envelope %s not found", e.ID)
		}
		return err
	}
	return nil
}

// UpdateInTx mirrors Update but executes the SQL on the caller's
// transaction so the row update lands or rolls back together with any
// other writes the caller batched in the same tx (typically an
// EnqueueOutboxInTx call). A credencial (app ou serviço) vem da transação.
func (r *envelopePostgresRepository) UpdateInTx(ctx context.Context, tx *sqlx.Tx, e *envelope.SignatureEnvelope) error {
	if tx == nil {
		return fmt.Errorf("transaction is nil; use Update for non-tx callers")
	}
	res, err := r.execUpdate(ctx, tx, e)
	if err != nil {
		return err
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("envelope %s not found", e.ID)
	}
	return nil
}

// envelopeUpdateQuery fica em constante para que Update e UpdateService não
// divirjam (o UPDATE é o mesmo; só muda o pool que o executa).
const envelopeUpdateQuery = `
		UPDATE signature.envelopes
		   SET external_envelope_id = NULLIF($1, ''),
		       status = $2,
		       metadata = $3,
		       last_error = NULLIF($4, ''),
		       finished_at = $5,
		       updated_at = $6
		 WHERE id = $7 AND tenant_id = $8
	`

// updateArgs monta os argumentos do UPDATE e carimba UpdatedAt.
func updateArgs(e *envelope.SignatureEnvelope) ([]interface{}, error) {
	e.UpdatedAt = time.Now()
	metadata, err := json.Marshal(e)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}
	return []interface{}{
		e.ExternalEnvelopeID, string(e.Status), string(metadata), e.LastError,
		e.FinishedAt, e.UpdatedAt, e.ID, e.TenantID,
	}, nil
}

// execUpdate roda o UPDATE no pool padrão (tx == nil) ou na transação do
// caller. Mantém o SQL num lugar só para as variantes não divergirem.
func (r *envelopePostgresRepository) execUpdate(ctx context.Context, tx *sqlx.Tx, e *envelope.SignatureEnvelope) (sql.Result, error) {
	args, err := updateArgs(e)
	if err != nil {
		return nil, err
	}
	if tx != nil {
		return tx.ExecContext(ctx, envelopeUpdateQuery, args...)
	}
	return r.db.ExecContext(ctx, envelopeUpdateQuery, args...)
}

func (r *envelopePostgresRepository) GetByID(ctx context.Context, tenantID, id string) (*envelope.SignatureEnvelope, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT metadata FROM signature.envelopes WHERE id = $1 AND tenant_id = $2 AND deleted_at IS NULL`,
		id, tenantID,
	).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return decodeEnvelope(metadata)
}

// GetByIDService lê o envelope pelo id sem filtro de tenant, no pool
// service-role. É o caminho do background (varredura de reconciliação), que
// roda sem JWT: com o pool de aplicação a RLS esconde a linha e o reconcile
// devolveria "envelope não encontrado" para um envelope que existe.
func (r *envelopePostgresRepository) GetByIDService(ctx context.Context, id string) (*envelope.SignatureEnvelope, error) {
	return scanEnvelopeRow(
		r.db.QueryServiceRow(ctx, `SELECT metadata FROM signature.envelopes WHERE id = $1 AND deleted_at IS NULL`, id),
		"id="+id,
	)
}

func (r *envelopePostgresRepository) GetByContractID(ctx context.Context, tenantID, contractID string) (*envelope.SignatureEnvelope, error) {
	var metadata []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT metadata
		  FROM signature.envelopes
		 WHERE tenant_id = $1
		   AND contract_id = $2
		   AND deleted_at IS NULL
		 ORDER BY created_at DESC
		 LIMIT 1
	`, tenantID, contractID).Scan(&metadata)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return decodeEnvelope(metadata)
}

// externalIDLookupQuery busca o envelope pelo id externo da Clicksign. Fica em
// constante para o teste de forma (envelope_postgres_query_test.go) travar o
// contrato da query.
const externalIDLookupQuery = `
	SELECT metadata
	  FROM signature.envelopes
	 WHERE external_envelope_id = $1 AND deleted_at IS NULL
	 ORDER BY created_at DESC
	 LIMIT 1
`

// GetByExternalID usa o pool service-role (bypass RLS) porque o webhook handler
// chega sem JWT. Caller é responsável por propagar o tenant_id retornado para
// o ctx antes de qualquer Update().
func (r *envelopePostgresRepository) GetByExternalID(ctx context.Context, externalEnvelopeID string) (*envelope.SignatureEnvelope, error) {
	return scanEnvelopeRow(
		r.db.QueryServiceRow(ctx, externalIDLookupQuery, externalEnvelopeID),
		"external_envelope_id="+externalEnvelopeID,
	)
}

// metadataScanner é o mínimo que *pgx.Row expõe — evita importar pgx aqui só
// para declarar o parâmetro do helper.
type metadataScanner interface {
	Scan(dest ...any) error
}

// scanEnvelopeRow converte "nenhuma linha" em (nil, nil) e QUALQUER outro erro
// em erro de verdade.
//
// Antes os dois lookups do webhook tratavam TODO erro de Scan como "não
// encontrado" (o comentário assumia que só existia o sentinela de no-rows).
// Quando a query do GetByDocumentKey quebrou ("cannot extract elements from a
// scalar"), o erro foi engolido, o handler acksou 200 com "envelope not found
// locally" e o evento foi descartado — foi assim que o contrato 277b965e ficou
// preso em PENDING_SIGNATURE com o envelope já fechado na Clicksign
// (30/09/2026). Erro real precisa subir para o handler devolver 5xx e o evento
// ser reentregue, não virar "não existe".
func scanEnvelopeRow(row metadataScanner, label string) (*envelope.SignatureEnvelope, error) {
	var metadata []byte
	if err := row.Scan(&metadata); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("lookup envelope by %s: %w", label, err)
	}
	return decodeEnvelope(metadata)
}

// documentKeyLookupQuery procura pelo clicksign_document_key dentro do JSONB
// metadata — em ambas as posições suportadas:
//   - metadata->'documents' (array, formato pós multi-doc)
//   - metadata->'document'   (objeto, formato legado pré multi-doc)
//
// O CASE dentro do jsonb_array_elements é obrigatório: `metadata->'documents'`
// pode ser JSON `null` (3 envelopes antigos no beta em 30/09/2026), e o
// Postgres não garante a ordem de avaliação de um AND — um
// `jsonb_typeof(...) = 'array' AND EXISTS (jsonb_array_elements(...))` ainda
// pode explodir com "cannot extract elements from a scalar". Com o CASE, a
// função nunca recebe algo que não seja array.
const documentKeyLookupQuery = `
	SELECT metadata
	  FROM signature.envelopes
	 WHERE deleted_at IS NULL
	   AND (
	       metadata->'document'->>'clicksign_document_key' = $1
	       OR EXISTS (
	           SELECT 1 FROM jsonb_array_elements(
	               CASE
	                   WHEN jsonb_typeof(metadata->'documents') = 'array' THEN metadata->'documents'
	                   ELSE '[]'::jsonb
	               END
	           ) d
	            WHERE d->>'clicksign_document_key' = $1
	       )
	   )
	 ORDER BY created_at DESC
	 LIMIT 1
`

// GetByDocumentKey é o fallback do webhook: o payload v3 da Clicksign envia
// document.key (que é doc.ID, não envelope.id) — e o envelope.id pode até não
// vir no webhook. Service-role pelo mesmo motivo do GetByExternalID.
func (r *envelopePostgresRepository) GetByDocumentKey(ctx context.Context, documentKey string) (*envelope.SignatureEnvelope, error) {
	return scanEnvelopeRow(
		r.db.QueryServiceRow(ctx, documentKeyLookupQuery, documentKey),
		"clicksign_document_key="+documentKey,
	)
}

// ReassociateWebhookEvent corrige a correlação da linha de auditoria quando o
// payload v3 trouxe document.key no lugar do envelope.id: reescreve
// external_envelope_id para o id externo do envelope e estampa tenant_id.
//
// Sem isso a linha fica com o doc key e tenant_id NULL — invisível para a tela
// de webhooks do envelope (que filtra por external_envelope_id) e para
// qualquer leitura por tenant, justo o rastro que se precisa ter em mãos
// quando o webhook "some".
func (r *envelopePostgresRepository) ReassociateWebhookEvent(ctx context.Context, webhookID, externalEnvelopeID, tenantID string) error {
	return r.db.ExecService(ctx, `
		UPDATE signature.webhook_events
		   SET external_envelope_id = $2,
		       tenant_id = COALESCE(tenant_id, NULLIF($3, '')::uuid)
		 WHERE id = $1
		   AND (
		       external_envelope_id IS DISTINCT FROM $2
		       OR tenant_id IS DISTINCT FROM NULLIF($3, '')::uuid
		   )
	`, webhookID, externalEnvelopeID, tenantID)
}

func (r *envelopePostgresRepository) ListByTenant(ctx context.Context, tenantID string, status *envelope.Status) ([]*envelope.SignatureEnvelope, error) {
	args := []any{tenantID}
	query := `
		SELECT metadata
		  FROM signature.envelopes
		 WHERE tenant_id = $1 AND deleted_at IS NULL
	`
	if status != nil && *status != "" {
		query += ` AND status = $2`
		args = append(args, string(*status))
	}
	query += ` ORDER BY created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*envelope.SignatureEnvelope, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		env, err := decodeEnvelope(metadata)
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, rows.Err()
}

// ListStaleActive devolve envelopes ainda não terminais cuja última atualização
// passou de `olderThan`. Roda sem filtro de tenant: é a varredura de
// reconciliação, que existe para não depender da entrega do webhook.
//
// Pool service-role obrigatório: a varredura roda sem JWT e a política de RLS
// de signature.envelopes compara tenant_id com auth.jwt() — pelo pool de
// aplicação a query devolve SEMPRE zero linhas, sem erro, e a rede de segurança
// fica decorativa. Medido no beta em 30/09/2026: 0 envelopes pelo pool de app e
// 2 pelo de serviço para o mesmo WHERE.
// ListStaleActive lista os envelopes que a varredura de reconciliação deve
// sincronizar: os que estão em andamento E os que terminaram sem que o desfecho
// tenha sido anunciado ao toca-deal (`terminal_event_published_at` ausente).
//
// O segundo grupo entrou em 2026-10-05: um envelope CANCELED local cujo
// `signature.refused` nunca saiu ficava fora da varredura para sempre — o
// contrato seguia em PENDING_SIGNATURE, travado para reenvio, e nenhum caminho
// automático voltava a olhar para ele. A varredura é justamente a rede de
// segurança de quem ficou sem webhook; ela precisa enxergar o que ficou preso.
//
// A janela de 7 dias limita o custo: um envelope terminal antigo cujo desfecho
// nunca saiu (ou que já foi apagado no provedor) para de ser consultado em vez
// de gerar um GET a cada ciclo para sempre.
func (r *envelopePostgresRepository) ListStaleActive(ctx context.Context, olderThan time.Time, limit int) ([]*envelope.SignatureEnvelope, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT metadata
		  FROM signature.envelopes
		 WHERE deleted_at IS NULL
		   AND external_envelope_id IS NOT NULL
		   AND external_envelope_id <> ''
		   AND updated_at < $1
		   AND (
		         status IN ('PENDING', 'DRAFT', 'RUNNING', 'PARTIALLY_SIGNED')
		      OR (
		           status IN ('CANCELED', 'REFUSED', 'EXPIRED')
		           AND metadata->>'terminal_event_published_at' IS NULL
		           AND updated_at > NOW() - INTERVAL '7 days'
		         )
		       )
		 ORDER BY updated_at ASC
		 LIMIT $2`, olderThan, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]*envelope.SignatureEnvelope, 0)
	for rows.Next() {
		var metadata []byte
		if err := rows.Scan(&metadata); err != nil {
			return nil, err
		}
		env, err := decodeEnvelope(metadata)
		if err != nil {
			return nil, err
		}
		out = append(out, env)
	}
	return out, rows.Err()
}

func (r *envelopePostgresRepository) AppendWebhookEvent(ctx context.Context, externalEnvelopeID, eventType, deliveryID string, body []byte, sigHeader, sourceIP string) (string, error) {
	id := uuid.NewString()
	// tenant_id is derived from the parent envelope (signature.envelopes) so the
	// row is filterable by RLS. If no envelope matches (race or spoofed webhook),
	// tenant_id stays NULL — the row is visible only via the service pool.
	query := `
		INSERT INTO signature.webhook_events
		    (id, external_envelope_id, event_type, delivery_id, raw_body, signature_header, source_ip, tenant_id)
		VALUES (
		    $1, $2, $3, NULLIF($4, ''), $5, NULLIF($6, ''), NULLIF($7, ''),
		    (SELECT tenant_id FROM signature.envelopes
		      WHERE external_envelope_id = $2
		      ORDER BY created_at DESC LIMIT 1)
		)
	`
	if err := r.db.ExecService(ctx, query, id, externalEnvelopeID, eventType, deliveryID, string(body), sigHeader, sourceIP); err != nil {
		return "", err
	}
	return id, nil
}

func (r *envelopePostgresRepository) MarkWebhookProcessed(ctx context.Context, webhookID string, processError string) error {
	query := `
		UPDATE signature.webhook_events
		   SET processed = TRUE,
		       process_error = NULLIF($2, '')
		 WHERE id = $1
	`
	return r.db.ExecService(ctx, query, webhookID, processError)
}

// ListWebhookEvents projeta as últimas linhas de signature.webhook_events
// para um envelope. raw_body é INTENCIONALMENTE omitido — contém PII e nunca
// deve sair via endpoint de operador. Pool service-role: a UI já valida o
// tenant antes de chegar aqui via GetByID/GetByContractID.
func (r *envelopePostgresRepository) ListWebhookEvents(ctx context.Context, externalEnvelopeID string) ([]envelope.WebhookEventRow, error) {
	rows, err := r.db.QueryServiceRows(ctx, `
		SELECT id, event_type, COALESCE(delivery_id, ''), processed,
		       COALESCE(process_error, ''), received_at
		  FROM signature.webhook_events
		 WHERE external_envelope_id = $1
		 ORDER BY received_at DESC
		 LIMIT 200
	`, externalEnvelopeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]envelope.WebhookEventRow, 0)
	for rows.Next() {
		var row envelope.WebhookEventRow
		if err := rows.Scan(
			&row.ID, &row.EventType, &row.DeliveryID,
			&row.Processed, &row.ProcessError, &row.ReceivedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func decodeEnvelope(metadata []byte) (*envelope.SignatureEnvelope, error) {
	var e envelope.SignatureEnvelope
	if err := json.Unmarshal(metadata, &e); err != nil {
		return nil, fmt.Errorf("decode envelope metadata: %w", err)
	}
	normalizeDocuments(&e)
	return &e, nil
}

// normalizeDocuments hoists the legacy singular `document` field (envelopes
// persisted before multi-doc support) into the `documents` slice so callers
// can always iterate over `e.Documents`. The legacy field is cleared after
// migration to avoid double-counting on subsequent writes.
func normalizeDocuments(e *envelope.SignatureEnvelope) {
	if e == nil {
		return
	}
	if len(e.Documents) == 0 && e.Document != nil {
		e.Documents = []envelope.Document{*e.Document}
	}
	e.Document = nil
}
