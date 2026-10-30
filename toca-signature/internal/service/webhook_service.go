package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
)

// WebhookService applies a Clicksign webhook to the local envelope and
// publishes the matching domain events on the bus.
//
// Idempotency is split across two layers:
//  1. Upstream (don't re-apply the same webhook to the envelope): we use the
//     delivery_id (or sha256 of the raw body) as the publisher event_id, so
//     the consumer side is naturally idempotent through events.processed_events.
//  2. Within this method: applying MarkSignerSigned twice is harmless —
//     SignedAt is overwritten with the same timestamp, allSigned recomputes
//     identically.
type WebhookService struct {
	repo     envelope.Repository
	eventBus events.EventBus
	now      func() time.Time

	// db opt-in: when wired (cmd/server/main.go), every state flip that
	// emits an event happens in a single Postgres transaction — repo
	// Update + outbox row INSERT either both commit or both roll back.
	// With db == nil the legacy non-atomic path is used (tests).
	db *database.PostgresDB

	// clicksign opt-in (WithClicksign): lets the handler recover the
	// signed-PDF URL from GET /envelopes/:id/documents when the
	// auto_close / sign webhook payload arrives without it. Nil in
	// tests / InMemory setups — the handler then relies only on the URL
	// carried in the webhook body.
	clicksign clicksign.API
}

func NewWebhookService(repo envelope.Repository, bus events.EventBus) *WebhookService {
	return &WebhookService{repo: repo, eventBus: bus, now: time.Now}
}

// WithDB opts the webhook handler into the atomic publish+commit path.
// Returns the receiver to enable chaining at the construction site.
func (s *WebhookService) WithDB(db *database.PostgresDB) *WebhookService {
	s.db = db
	return s
}

// WithClicksign wires the Clicksign API client so the handler can recover
// the signed-PDF URL from the documents endpoint when a finished-envelope
// webhook arrives without it. Returns the receiver to enable chaining at
// the construction site.
func (s *WebhookService) WithClicksign(api clicksign.API) *WebhookService {
	s.clicksign = api
	return s
}

// saveEnvelopeAndPublish persiste o envelope e emite os eventos de domínio na
// MESMA transação quando db + barramento RabbitMQ estão ligados.
//
// A escrita vai pelo pool SERVICE-ROLE (RunInServiceContext). O webhook da
// Clicksign é chamador de sistema — autenticado por HMAC + allowlist de IP,
// sem JWT de usuário — e é assim que a varredura de reconciliação também
// escreve. Enquanto este caminho usava a transação do pool de app, o hook de
// RLS instalava `request.jwt.claims` vazio, o UPDATE atingia zero linhas e TODO
// webhook morria em "envelope X not found" sem publicar nada: o estado do
// envelope só andava pela varredura de 5 minutos (e um envelope terminal nem
// isso). Se o pool service-role não estiver configurado, cai no pool de app —
// agora com o tenant do envelope no contexto, o que faz a RLS enxergar a
// linha.
func (s *WebhookService) saveEnvelopeAndPublish(ctx context.Context, env *envelope.SignatureEnvelope, evts ...events.Event) error {
	stampTerminalEventPublished(env, evts)
	if s.db != nil {
		if rb, ok := s.eventBus.(*events.RabbitMQEventBus); ok && rb != nil {
			err := s.db.RunInServiceContext(ctx, func(txCtx context.Context) error {
				tx := database.TxFromCtx(txCtx)
				if tx == nil {
					return fmt.Errorf("service transaction ausente no contexto")
				}
				if err := s.repo.UpdateInTx(txCtx, tx, env); err != nil {
					return err
				}
				for _, evt := range evts {
					if _, err := rb.EnqueueOutboxInTx(txCtx, tx, evt); err != nil {
						return err
					}
				}
				return nil
			})
			if err == nil {
				return nil
			}
			if !errors.Is(err, database.ErrServiceRoleRequired) {
				return err
			}
		}
	}

	if err := s.repo.Update(ctx, env); err != nil {
		return err
	}
	for _, evt := range evts {
		if s.eventBus == nil {
			continue
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			return err
		}
	}
	return nil
}

// Process is called by the HTTP handler with the parsed payload and the raw
// body (kept for audit + idempotency keying). It returns a non-nil error
// only on technical failures (DB unavailable, bus down). Unknown event types
// are logged-and-acked.
func (s *WebhookService) Process(ctx context.Context, payload clicksign.WebhookPayload, rawBody []byte, deliveryID, sourceIP, sigHeader string) error {
	if deliveryID == "" {
		deliveryID = payload.Event.DeliveryID
	}
	if deliveryID == "" {
		// Fall back to a content-derived id so dedup still works.
		deliveryID = bodyHash(rawBody)
	}

	// Pull the envelope id from any of the top-level locations Clicksign has
	// used historically. v3 reuses the document.key as envelope identifier
	// in webhook payloads, while older v1 wrappers nested it under envelope.
	// Falling back to a placeholder lets us record the event for audit even
	// when we can't correlate to a local envelope (e.g. shape drift on the
	// Clicksign side) — the raw body is the source of truth on disk.
	externalEnvelopeID := extractExternalEnvelopeID(payload)
	auditEnvelopeID := externalEnvelopeID
	if auditEnvelopeID == "" {
		auditEnvelopeID = "unknown:" + deliveryID
	}
	eventName := normalizeEventName(payload.Event.Name)

	// Persist the audit row FIRST — before any domain validation — so a
	// payload with unfamiliar shape still leaves a forensic breadcrumb.
	// Before this change a missing envelope.id silently aborted the handler
	// and produced 200/empty-table, which is indistinguishable from "webhook
	// never arrived" when debugging.
	webhookID, err := s.repo.AppendWebhookEvent(ctx, auditEnvelopeID, eventName, deliveryID, rawBody, sigHeader, sourceIP)
	if err != nil {
		return fmt.Errorf("persist webhook event: %w", err)
	}

	if externalEnvelopeID == "" {
		return s.repo.MarkWebhookProcessed(ctx, webhookID,
			fmt.Sprintf("could not extract envelope id from payload (event=%s)", eventName))
	}

	// Two-step lookup: external_envelope_id first (works when Clicksign sends
	// envelope.id in the payload), document key as fallback (the v3 reality —
	// payload.document.key is the doc UUID we persisted as
	// envelope.Document.ClicksignDocumentKey, NOT the same UUID as
	// envelope.id). Without this fallback every v3 webhook landed on
	// "envelope not found locally" even though the envelope was right there.
	env, err := s.repo.GetByExternalID(ctx, externalEnvelopeID)
	if err != nil {
		_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
		return err
	}
	if env == nil && payload.Document != nil && payload.Document.Key != "" {
		env, err = s.repo.GetByDocumentKey(ctx, payload.Document.Key)
		if err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
	}
	if env == nil {
		// Could be an envelope created outside our system (manual via Clicksign
		// dashboard) or a webhook that arrived before the local INSERT
		// finished. Either way: ack with a marker and let it be — Clicksign
		// won't retry on 200, and a future retransmit will find it.
		return s.repo.MarkWebhookProcessed(ctx, webhookID, "envelope not found locally")
	}

	// A linha de auditoria nasceu apontando para o que veio no payload — no
	// v3, o document.key. Agora que sabemos o envelope de verdade, religamos a
	// linha para o id externo + tenant, senão ela fica órfã (tenant_id NULL) e
	// some da tela de webhooks do envelope.
	s.relinkWebhookAudit(ctx, webhookID, auditEnvelopeID, env)

	// O webhook não traz identidade de usuário: o tenant vem da linha que
	// acabamos de resolver (chaveada pelo envelope/documento do payload
	// HMAC-verificado), nunca do corpo. Sem isso o contexto chega sem tenant,
	// o pool de app instala claims vazios e as escritas deste fluxo não
	// enxergam a linha sob RLS.
	if env.TenantID != "" {
		ctx = pkgctx.WithTenantID(ctx, env.TenantID)
	}

	occurredAt := payload.Event.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = s.now()
	}

	// Webhook age = delay between Clicksign emitting the event and us
	// processing it. p99 spikes hint at queue saturation or our consumer
	// falling behind. Only recorded when occurredAt was provided by the
	// payload (the IsZero fallback above would produce a meaningless 0).
	if !payload.Event.OccurredAt.IsZero() {
		if age := s.now().Sub(payload.Event.OccurredAt); age > 0 {
			metrics.ObserveWebhookSignatureEventAge(age)
		}
	}

	switch eventName {
	case "sign":
		if payload.Signer == nil {
			return s.repo.MarkWebhookProcessed(ctx, webhookID, "sign event without signer")
		}
		signer, allSigned := env.MarkSignerSigned(payload.Signer.Key, occurredAt)
		if signer == nil {
			// Signer key didn't match any local signer. Persist the
			// audit row but don't fail loudly — could be a witness we
			// don't track or a stale webhook.
			return s.repo.MarkWebhookProcessed(ctx, webhookID, "signer key not found")
		}
		evts := []events.Event{
			s.buildSignerSignedEvent(env, signer, occurredAt, deliveryID, payload.Signer.EvidenceURL),
		}
		if allSigned {
			evts = append(evts, s.buildEnvelopeFinishedEvent(env, occurredAt, deliveryID, s.resolveSignedDocURL(ctx, env, payload.Document)))
		}
		if err := s.saveEnvelopeAndPublish(ctx, env, evts...); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
		recordEnvelopeAudit(ctx, "signature.signer_signed", env, map[string]any{
			"source":    "webhook",
			"signer_id": signer.SignerID,
		})
		if allSigned {
			recordEnvelopeAudit(ctx, "signature.envelope_finished", env, map[string]any{"source": "webhook"})
		}

	case "auto_close":
		// auto_close fires after every signer has signed. If we somehow
		// missed individual `sign` events, force the envelope to FINISHED
		// AND retroactively mark each pending signer as SIGNED — otherwise
		// the admin UI shows the envelope as Assinado while individual
		// signer rows still display Pendente (mismatch between header and
		// children badges).
		if env.Status != envelope.StatusFinished {
			env.Status = envelope.StatusFinished
			finished := occurredAt
			env.FinishedAt = &finished
			env.UpdatedAt = s.now()
		}
		for i := range env.Signers {
			if env.Signers[i].Status == envelope.SignerStatusSigned ||
				env.Signers[i].Status == envelope.SignerStatusRefused {
				continue
			}
			env.Signers[i].Status = envelope.SignerStatusSigned
			if env.Signers[i].SignedAt == nil {
				signed := occurredAt
				env.Signers[i].SignedAt = &signed
			}
		}
		evt := s.buildEnvelopeFinishedEvent(env, occurredAt, deliveryID, s.resolveSignedDocURL(ctx, env, payload.Document))
		if err := s.saveEnvelopeAndPublish(ctx, env, evt); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_finished", env, map[string]any{"source": "webhook", "event": "auto_close"})

	case "refusal":
		signerKey := ""
		reason := ""
		if payload.Signer != nil {
			signerKey = payload.Signer.Key
			reason = payload.Signer.RefusedReason
		}
		signer := env.MarkRefused(signerKey, reason)
		signerID := ""
		if signer != nil {
			signerID = signer.SignerID
		}
		evt := s.buildRefusedEvent(env, signerID, reason, occurredAt, deliveryID)
		if err := s.saveEnvelopeAndPublish(ctx, env, evt); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
		recordEnvelopeAudit(ctx, "signature.signer_refused", env, map[string]any{
			"source":    "webhook",
			"signer_id": signerID,
			"reason":    reason,
		})

	case "cancel":
		env.MarkCanceled()
		// Treat cancel as refused-with-no-signer so toca-deal can release
		// the lock with the same handler.
		evt := s.buildRefusedEvent(env, "", "envelope canceled", occurredAt, deliveryID)
		if err := s.saveEnvelopeAndPublish(ctx, env, evt); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_canceled", env, map[string]any{"source": "webhook"})

	case "deadline", "expired":
		// Both events terminate the envelope as EXPIRED. A dedicated
		// SignatureExpiredEvent makes the distinction observable downstream
		// (metrics by termination reason) while toca-deal applies the same
		// "back to DRAFT + release lock" recovery as for refusal.
		env.MarkExpired(occurredAt)
		evt := s.buildExpiredEvent(env, occurredAt, deliveryID)
		if err := s.saveEnvelopeAndPublish(ctx, env, evt); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_expired", env, map[string]any{"source": "webhook", "event": eventName})

	case "signature_started":
		// Envelope went from DRAFT to RUNNING — first signers are being
		// notified. Promote the local envelope status and emit the new
		// envelope_started event so toca-deal can move the contract from
		// DRAFT to PENDING_SIGNATURE without waiting for the first signer.
		if env.Status == envelope.StatusDraft || env.Status == envelope.StatusPending {
			env.Status = envelope.StatusRunning
			env.UpdatedAt = s.now()
		}
		evt := s.buildEnvelopeStartedEvent(env, occurredAt, deliveryID)
		if err := s.saveEnvelopeAndPublish(ctx, env, evt); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_started", env, map[string]any{"source": "webhook"})

	case "view":
		// Signer opened the signing link. Useful UX signal (admin sees
		// "Visualizado", portal cliente can drop reminders). Only updates
		// the signer; envelope status stays untouched.
		if payload.Signer == nil {
			return s.repo.MarkWebhookProcessed(ctx, webhookID, "view event without signer")
		}
		signer := env.MarkSignerViewed(payload.Signer.Key, occurredAt)
		if signer == nil {
			return s.repo.MarkWebhookProcessed(ctx, webhookID, "signer key not found for view")
		}
		evt := s.buildSignerViewedEvent(env, signer, occurredAt, deliveryID)
		if err := s.saveEnvelopeAndPublish(ctx, env, evt); err != nil {
			_ = s.repo.MarkWebhookProcessed(ctx, webhookID, err.Error())
			return err
		}

	case "upload", "add_signer", "request_signature", "update_envelope", "update_signer", "document_closed":
		// Informational events. They are persisted to webhook_events for
		// audit and acked here with an explicit note so the process_error
		// column doubles as "no-op reason" (queryable later).
		return s.repo.MarkWebhookProcessed(ctx, webhookID,
			fmt.Sprintf("no domain action for event=%s", payload.Event.Name))

	default:
		// Unknown event type — persist with a marker so operators can spot
		// new Clicksign events that we haven't taught the service yet.
		return s.repo.MarkWebhookProcessed(ctx, webhookID,
			fmt.Sprintf("unhandled event=%s", payload.Event.Name))
	}

	return s.repo.MarkWebhookProcessed(ctx, webhookID, "")
}

// buildSignerSignedEvent factors the event-construction out of
// publishSignerSigned so the same payload can be used by the atomic
// saveEnvelopeAndPublish helper. The legacy publish* methods stay as
// thin wrappers for callers that still need the non-tx path.
func (s *WebhookService) buildSignerSignedEvent(env *envelope.SignatureEnvelope, signer *envelope.Signer, at time.Time, deliveryID, evidenceURL string) pkgevents.SignatureSignerSignedEvent {
	return pkgevents.SignatureSignerSignedEvent{
		EventIDVal: deliveryID + ":signer:" + signer.SignerID,
		Data: pkgevents.SignatureSignerSignedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			SignerID:           signer.SignerID,
			SignerEmail:        signer.Email,
			SignedAt:           at.UTC().Format(time.RFC3339),
			EvidenceURL:        evidenceURL,
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *WebhookService) publishSignerSigned(ctx context.Context, env *envelope.SignatureEnvelope, signer *envelope.Signer, at time.Time, deliveryID, evidenceURL string) error {
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.PublishReliable(ctx, s.buildSignerSignedEvent(env, signer, at, deliveryID, evidenceURL))
}

func (s *WebhookService) buildEnvelopeFinishedEvent(env *envelope.SignatureEnvelope, at time.Time, deliveryID, signedDoc string) pkgevents.SignatureEnvelopeFinishedEvent {
	return pkgevents.SignatureEnvelopeFinishedEvent{
		EventIDVal: deliveryID + ":finished",
		Data: pkgevents.SignatureEnvelopeFinishedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			FinishedAt:         at.UTC().Format(time.RFC3339),
			SignedDocumentURL:  signedDoc,
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *WebhookService) publishEnvelopeFinished(ctx context.Context, env *envelope.SignatureEnvelope, at time.Time, deliveryID, signedDoc string) error {
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.PublishReliable(ctx, s.buildEnvelopeFinishedEvent(env, at, deliveryID, signedDoc))
}

func (s *WebhookService) buildEnvelopeStartedEvent(env *envelope.SignatureEnvelope, at time.Time, deliveryID string) pkgevents.SignatureEnvelopeStartedEvent {
	return pkgevents.SignatureEnvelopeStartedEvent{
		EventIDVal: deliveryID + ":started",
		Data: pkgevents.SignatureEnvelopeStartedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			StartedAt:          at.UTC().Format(time.RFC3339),
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *WebhookService) publishEnvelopeStarted(ctx context.Context, env *envelope.SignatureEnvelope, at time.Time, deliveryID string) error {
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.PublishReliable(ctx, s.buildEnvelopeStartedEvent(env, at, deliveryID))
}

func (s *WebhookService) buildSignerViewedEvent(env *envelope.SignatureEnvelope, signer *envelope.Signer, at time.Time, deliveryID string) pkgevents.SignatureSignerViewedEvent {
	return pkgevents.SignatureSignerViewedEvent{
		EventIDVal: deliveryID + ":signer_viewed:" + signer.SignerID,
		Data: pkgevents.SignatureSignerViewedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			SignerID:           signer.SignerID,
			SignerEmail:        signer.Email,
			ViewedAt:           at.UTC().Format(time.RFC3339),
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *WebhookService) publishSignerViewed(ctx context.Context, env *envelope.SignatureEnvelope, signer *envelope.Signer, at time.Time, deliveryID string) error {
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.PublishReliable(ctx, s.buildSignerViewedEvent(env, signer, at, deliveryID))
}

func (s *WebhookService) buildExpiredEvent(env *envelope.SignatureEnvelope, at time.Time, deliveryID string) pkgevents.SignatureExpiredEvent {
	return pkgevents.SignatureExpiredEvent{
		EventIDVal: deliveryID + ":expired",
		Data: pkgevents.SignatureExpiredEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			ExpiredAt:          at.UTC().Format(time.RFC3339),
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *WebhookService) publishExpired(ctx context.Context, env *envelope.SignatureEnvelope, at time.Time, deliveryID string) error {
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.PublishReliable(ctx, s.buildExpiredEvent(env, at, deliveryID))
}

func (s *WebhookService) buildRefusedEvent(env *envelope.SignatureEnvelope, signerID, reason string, at time.Time, deliveryID string) pkgevents.SignatureRefusedEvent {
	return pkgevents.SignatureRefusedEvent{
		EventIDVal: deliveryID + ":refused",
		Data: pkgevents.SignatureRefusedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			SignerID:           signerID,
			Reason:             reason,
			RefusedAt:          at.UTC().Format(time.RFC3339),
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *WebhookService) publishRefused(ctx context.Context, env *envelope.SignatureEnvelope, signerID, reason string, at time.Time, deliveryID string) error {
	if s.eventBus == nil {
		return nil
	}
	return s.eventBus.PublishReliable(ctx, s.buildRefusedEvent(env, signerID, reason, at, deliveryID))
}

func bodyHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// extractExternalEnvelopeID locates the envelope identifier across the shapes
// Clicksign has shipped. v3 webhook payloads (current) put the envelope UUID
// under `document.key`; v1 wrappers used `envelope.id`. Falling through means
// the payload arrived in a shape we don't recognize — the caller persists an
// audit row anyway and the operator inspects raw_body.
// relinkWebhookAudit aponta a linha de auditoria para o envelope resolvido.
//
// Nunca falha o processamento: o rastro de auditoria é importante, mas perder
// um webhook por causa dele seria pior. A falha fica no log.
func (s *WebhookService) relinkWebhookAudit(ctx context.Context, webhookID, auditEnvelopeID string, env *envelope.SignatureEnvelope) {
	if webhookID == "" || env == nil || env.ExternalEnvelopeID == "" {
		return
	}
	if err := s.repo.ReassociateWebhookEvent(ctx, webhookID, env.ExternalEnvelopeID, env.TenantID); err != nil {
		log.Printf("clicksign webhook %s: reassociate audit row (payload id %q → envelope %s): %v",
			webhookID, auditEnvelopeID, env.ExternalEnvelopeID, err)
	}
}

func extractExternalEnvelopeID(p clicksign.WebhookPayload) string {
	if p.Envelope != nil && p.Envelope.ID != "" {
		return p.Envelope.ID
	}
	if p.Document != nil && p.Document.Key != "" {
		return p.Document.Key
	}
	return ""
}

// normalizeEventName accepts both the Ruby-style names shown in the Clicksign
// dashboard ("Event::Sign", "Event::AutoClose") and the snake_case form the
// HTTP payload usually carries ("sign", "auto_close"). The dashboard label is
// what operators see when triaging, but the JSON payload has historically
// used the lowercase form; normalizing here means the downstream switch
// matches regardless of which form arrives.
func normalizeEventName(name string) string {
	if name == "" {
		return ""
	}
	trimmed := strings.TrimPrefix(name, "Event::")
	if trimmed == name {
		return name // already in plain form (e.g. "sign", "auto_close")
	}
	// CamelCase ("AutoClose", "DocumentClosed") → snake_case ("auto_close").
	var out strings.Builder
	out.Grow(len(trimmed) + 4)
	for i, r := range trimmed {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			r = r - 'A' + 'a'
		}
		out.WriteRune(r)
	}
	return out.String()
}

func signedDocURL(d *clicksign.WebhookDocument) string {
	if d == nil {
		return ""
	}
	if d.SignedFileURL != "" {
		return d.SignedFileURL
	}
	return d.DownloadURL
}

// resolveSignedDocURL returns the signed-PDF URL for a finished envelope.
// It prefers the URL carried in the webhook payload; Clicksign frequently
// omits it on the auto_close / sign payloads because the signed-file CDN
// lags the event. When that happens it falls back to a live
// GET /envelopes/:id/documents so the envelope_finished event still carries
// a usable URL for the toca-deal persist subscriber — without this the
// contract Document stays on the draft DOCX until an operator runs Reconcile
// or the deal-side backfill by hand.
//
// Best-effort: returns "" when neither the payload nor the API has the URL
// ready (narrow CDN race). Reconcile and the deal-side backfill remain the
// recovery path for that residual case.
func (s *WebhookService) resolveSignedDocURL(ctx context.Context, env *envelope.SignatureEnvelope, doc *clicksign.WebhookDocument) string {
	if u := signedDocURL(doc); u != "" {
		return u
	}
	if s.clicksign == nil || env == nil || env.ExternalEnvelopeID == "" {
		return ""
	}
	docs, err := s.clicksign.ListDocuments(ctx, env.TenantID, env.ExternalEnvelopeID)
	if err != nil {
		return ""
	}
	return clicksign.SignedContractURL(docs, env.ContractID)
}
