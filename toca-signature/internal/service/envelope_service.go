package service

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/logger"
	pkgvalidation "github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/pdf"
)

// Sentinel errors so handlers can map domain failures to specific HTTP codes
// (404 vs 409 vs 400) via errors.Is.
var (
	ErrEnvelopeNotFound = errors.New("envelope not found")
	ErrSignerNotFound   = errors.New("signer not found")
	// ErrCancelProviderFailed indica que o provedor não aceitou o
	// cancelamento. O envelope NÃO é marcado como cancelado localmente: com o
	// link de assinatura vivo no provedor, dizer "cancelado" na tela seria
	// mentira — e é exatamente o que acontecia antes desta correção. O handler
	// traduz para 502 com o motivo, para o operador tentar de novo ou cancelar
	// no painel do provedor.
	ErrCancelProviderFailed = errors.New("cancelamento não aceito pelo provedor de assinatura")
)

// recordEnvelopeAudit emite um evento de ciclo de vida do envelope na
// auditoria central (audit.events). É o ponto único usado pelos caminhos de
// criação, webhook e reconciliação — o actor sai como "service" quando o
// contexto não carrega conta autenticada (webhooks/reconcile rodam fora de
// uma sessão de usuário).
//
// Nenhum dado PII: o After carrega apenas status, IDs e kind. E-mails de
// signatários ficam de fora de propósito (audit.events tem retenção de 5 anos).
func recordEnvelopeAudit(ctx context.Context, action string, env *envelope.SignatureEnvelope, extra map[string]any) {
	after := map[string]any{
		"status":               string(env.Status),
		"contract_id":          env.ContractID,
		"external_envelope_id": env.ExternalEnvelopeID,
		"kind":                 string(env.Kind),
	}
	if extra == nil {
		extra = map[string]any{}
	}
	// Webhooks/reconcile rodam fora de uma sessão de usuário, então o contexto
	// não carrega tenant. O envelope é a fonte autoritativa — injetar aqui
	// garante que o evento fique listável no escopo do tenant.
	ctx = pkgcontext.WithTenantID(ctx, env.TenantID)
	audit.Record(ctx, audit.Entry{
		Action:     action,
		EntityType: "Envelope",
		EntityID:   env.ID,
		After:      after,
		Metadata:   extra,
	})
}

// PDFFetcher is kept as a tiny alias around pdf.Renderer so the rest of the
// service package (and tests) don't need to import the pdf package. New
// code should target pdf.Renderer directly.
type PDFFetcher = pdf.Renderer

// DealLookup is the slice of clients.DealClient EnvelopeService actually uses.
// Defining the interface in this package keeps the service independently
// testable without faking the entire pkg/clients surface.
type DealLookup interface {
	GetContract(ctx context.Context, tenantID, contractID string) (*clients.ContractDTO, error)
	// FetchDocumentContent pulls the raw bytes of a Document persisted in
	// toca-deal so they can be uploaded as an extra attachment of the
	// Clicksign envelope. Returns the filename suggested by toca-deal
	// (used as the Clicksign document filename) and the bytes.
	FetchDocumentContent(ctx context.Context, tenantID, documentID string) (filename string, content []byte, err error)
}

// EnvelopeService is the orchestrator: it pulls a contract from toca-deal,
// renders the document, and walks the Clicksign create-envelope pipeline,
// persisting the local SignatureEnvelope along the way.
type EnvelopeService struct {
	repo       envelope.Repository
	clicksign  clicksign.API
	credsProv  credential.CredentialProvider
	dealClient DealLookup
	renderer   PDFFetcher
	eventBus   events.EventBus
	now        func() time.Time

	// db opt-in: when wired (cmd/server/main.go), Reconcile uses a single
	// Postgres transaction for envelope Update + outbox INSERT so a
	// midstream RabbitMQ outage can't leave envelope state ahead of the
	// downstream pipeline. See audit P0.1 follow-up.
	db *database.PostgresDB
}

// WithDB opts the envelope service into the atomic publish+commit path.
func (s *EnvelopeService) WithDB(db *database.PostgresDB) *EnvelopeService {
	s.db = db
	return s
}

// saveEnvelopeAndPublish persists the envelope and emits the given
// events atomically when db + RabbitMQ are wired. Same shape as
// WebhookService.saveEnvelopeAndPublish (kept per-service to avoid an
// extra abstraction layer for a two-line helper).
func (s *EnvelopeService) saveEnvelopeAndPublish(ctx context.Context, env *envelope.SignatureEnvelope, evts ...events.Event) error {
	return s.saveEnvelopeAndPublishScoped(ctx, reconcileTenantScope, env, evts...)
}

// saveEnvelopeAndPublishScoped é o save com escopo explícito: o caminho de
// background (varredura) roda sem JWT e por isso precisa da transação do pool
// service-role — a de app não enxerga nem atualiza a linha sob RLS.
func (s *EnvelopeService) saveEnvelopeAndPublishScoped(ctx context.Context, scope reconcileScope, env *envelope.SignatureEnvelope, evts ...events.Event) error {
	stampTerminalEventPublished(env, evts)
	if s.db != nil {
		if rb, ok := s.eventBus.(*events.RabbitMQEventBus); ok && rb != nil {
			if scope == reconcileServiceScope {
				return s.db.RunInServiceContext(ctx, func(txCtx context.Context) error {
					tx := database.TxFromCtx(txCtx)
					if tx == nil {
						return fmt.Errorf("service transaction ausente no contexto")
					}
					return s.updateAndEnqueue(txCtx, tx, rb, env, evts)
				})
			}
			return s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
				return s.updateAndEnqueue(ctx, tx, rb, env, evts)
			})
		}
	}
	if scope == reconcileServiceScope {
		if err := s.repo.UpdateService(ctx, env); err != nil {
			return err
		}
	} else if err := s.repo.Update(ctx, env); err != nil {
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

// updateAndEnqueue grava o envelope e enfileira os eventos na MESMA transação,
// seja ela do pool de app (caminho com JWT) ou do pool de serviço (varredura).
func (s *EnvelopeService) updateAndEnqueue(ctx context.Context, tx *sqlx.Tx, rb *events.RabbitMQEventBus, env *envelope.SignatureEnvelope, evts []events.Event) error {
	if err := s.repo.UpdateInTx(ctx, tx, env); err != nil {
		return err
	}
	for _, evt := range evts {
		if _, err := rb.EnqueueOutboxInTx(ctx, tx, evt); err != nil {
			return err
		}
	}
	return nil
}

// terminalEventNames são os eventos que encerram a vida do envelope do ponto de
// vista do toca-deal. Depois de qualquer um deles o contrato sai de
// PENDING_SIGNATURE (volta a rascunho na recusa/cancelamento/expiração, ou
// vira assinado na conclusão) — por isso a publicação precisa ser registrada no
// envelope: é esse carimbo que diz se ainda falta avisar o outro lado.
var terminalEventNames = map[string]bool{
	"signature.refused":           true,
	"signature.expired":           true,
	"signature.envelope_finished": true,
}

// stampTerminalEventPublished carimba o envelope quando o lote de eventos
// inclui um desfecho. Chamado por TODOS os caminhos que gravam + publicam
// (webhook, reconciliação, cancelamento do operador) para que nenhum deles
// deixe o toca-deal sem a notícia e para que a varredura saiba quando o assunto
// está encerrado.
func stampTerminalEventPublished(env *envelope.SignatureEnvelope, evts []events.Event) {
	if env == nil {
		return
	}
	for _, evt := range evts {
		if evt == nil {
			continue
		}
		if terminalEventNames[evt.Name()] {
			env.MarkTerminalEventPublished(time.Now())
			return
		}
	}
}

func NewEnvelopeService(
	repo envelope.Repository,
	cs clicksign.API,
	cp credential.CredentialProvider,
	dl DealLookup,
	renderer PDFFetcher,
	bus events.EventBus,
) *EnvelopeService {
	if renderer == nil {
		renderer = pdf.NewDefaultRenderer(nil)
	}
	return &EnvelopeService{
		repo:       repo,
		clicksign:  cs,
		credsProv:  cp,
		dealClient: dl,
		renderer:   renderer,
		eventBus:   bus,
		now:        time.Now,
	}
}

// CreateOptions controls per-signer auth methods and envelope-wide settings.
// SignerAuth maps the *internal* SignerID (from Contract.Signatory.ID) to
// the list of auth methods to enforce.
type CreateOptions struct {
	SignerAuth map[string][]envelope.AuthMethod
	// SignerGroups maps the internal SignerID to a 1-based Clicksign group.
	// Signers in the same group are notified together; the next group is
	// only notified after every signer in the previous group has signed.
	// Missing entries default to 0 (no ordering — legacy behavior).
	SignerGroups map[string]int
	// AttachmentDocIDs references Documents already persisted in toca-deal
	// (e.g. uploaded procuration). Their bytes are pulled via
	// DealLookup.FetchDocumentContent at envelope-create time.
	AttachmentDocIDs []string
	// InlineAttachments are PDFs uploaded by the operator in the envelope
	// creation modal — transient (live only on the envelope, not persisted
	// in toca-deal). Handlers validate filename/size before passing here.
	InlineAttachments []InlineAttachment
	Locale            string
	Deadline          *time.Time
}

// InlineAttachment carries a one-off PDF the operator attached at envelope
// creation time. Content is already pre-validated (PDF MIME, size cap) by
// the handler layer.
type InlineAttachment struct {
	Filename string
	Content  []byte
}

// CreateForContract is the entrypoint called from the SignatureRequestedEvent
// subscriber and from POST /signature/envelopes/:id/resend.
//
// Idempotency: if there is already an active envelope for the contract, return
// it untouched. An "active" envelope is one whose status is NOT in
// {CANCELED, EXPIRED, REFUSED, ERROR}. The unique partial index in Postgres
// enforces this at the DB level.
//
// Error handling: any failure after step 4 (CreateEnvelope on Clicksign side
// succeeded but a subsequent step failed) is persisted with status=ERROR and
// last_error populated. The operator can then resend or cancel via the UI.
var ErrContractNotReadyForSignature = errors.New("contract is not pending signature")

func (s *EnvelopeService) CreateForContract(ctx context.Context, tenantID, contractID string, opts CreateOptions) (*envelope.SignatureEnvelope, error) {
	// 1. Idempotency check.
	if existing, err := s.repo.GetByContractID(ctx, tenantID, contractID); err != nil {
		return nil, fmt.Errorf("check existing envelope: %w", err)
	} else if existing != nil && isActive(existing.Status) {
		return existing, nil
	} else if existing != nil && existing.Status == envelope.StatusError && existing.ExternalEnvelopeID != "" {
		return nil, errors.New("O envio anterior criou um envelope externo. Reconcilie a assinatura antes de tentar novamente.")
	}

	// 2. Fetch contract details from toca-deal.
	contract, err := s.dealClient.GetContract(ctx, tenantID, contractID)
	if err != nil {
		return nil, fmt.Errorf("fetch contract: %w", err)
	}
	if contract == nil {
		return nil, errors.New("contract not found")
	}
	if contract.Status != "PENDING_SIGNATURE" {
		return nil, ErrContractNotReadyForSignature
	}
	if len(contract.Signatories) == 0 {
		return nil, errors.New("contract has no signatories")
	}

	// 3. Resolve credentials so we know which environment (sandbox vs prod)
	// to record on the envelope. Failures here surface before we touch
	// Clicksign so we can fail fast with a clear message.
	creds, err := s.credsProv.Get(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load clicksign credentials: %w", err)
	}

	// 4. Build the local envelope shell so we can persist it as soon as we
	// have a Clicksign envelope id.
	signers := signersFromContract(contract.Signatories, opts.SignerAuth, opts.SignerGroups)
	env, err := envelope.New(tenantID, contractID, contract.DealID, creds.Environment, signers)
	if err != nil {
		return nil, err
	}

	// 5. Render PDF before any network call to Clicksign — if rendering
	// fails we'd rather fail before creating an empty envelope on their
	// side (which would then need cleanup).
	pdfBytes, err := s.renderer.Render(ctx, contract.ID, contract.Content)
	if err != nil {
		env.MarkError(fmt.Sprintf("pdf fetch: %v", err))
		_ = s.repo.Create(ctx, env)
		return env, err
	}

	log := logger.With(
		"component", "envelope_service",
		"envelope_id", env.ID,
		"contract_id", contractID,
		"tenant_id", tenantID,
	)
	log.Info("clicksign pipeline starting",
		"signers", len(env.Signers),
		"attachments", len(opts.AttachmentDocIDs)+len(opts.InlineAttachments),
		"environment", creds.Environment,
	)

	// 5b. Resolve attachment bytes (deal-stored docs + inline uploads) before
	// hitting Clicksign so a missing/corrupt attachment fails fast.
	attachments, err := s.collectAttachments(ctx, tenantID, opts)
	if err != nil {
		env.MarkError(fmt.Sprintf("collect attachments: %v", err))
		_ = s.repo.Create(ctx, env)
		return env, err
	}

	// 6. Create envelope on Clicksign. On success persist the local row
	// before any subsequent step so a webhook arriving mid-pipeline can
	// still find us via external_envelope_id.
	envName := envelopeName(contract)
	csEnv, err := s.clicksign.CreateEnvelope(ctx, tenantID, envName)
	if err != nil {
		log.Error("create envelope failed", "error", err)
		env.MarkError(fmt.Sprintf("create envelope: %v", err))
		_ = s.repo.Create(ctx, env)
		return env, err
	}
	env.ExternalEnvelopeID = csEnv.ID
	env.Status = envelope.StatusDraft
	log.Info("create envelope ok", "external_id", csEnv.ID)
	if err := s.repo.Create(ctx, env); err != nil {
		// We have a leaked envelope on Clicksign, but no local row to
		// retry from. Best we can do is propagate the error; an operator
		// will see the leak through the Clicksign dashboard.
		log.Error("persist envelope failed (clicksign envelope leaked)",
			"external_id", csEnv.ID, "error", err)
		return nil, fmt.Errorf("persist envelope: %w", err)
	}

	// 7. Upload contract PDF (always at index 0) plus each attachment.
	contractDoc := envelope.Document{Filename: contractFilename(contract.ID)}
	if err := s.uploadOne(ctx, tenantID, csEnv.ID, contractDoc.Filename, pdfBytes, env, &contractDoc, log); err != nil {
		return env, err
	}
	env.Documents = append(env.Documents, contractDoc)

	for _, att := range attachments {
		extra := envelope.Document{Filename: att.Filename}
		if err := s.uploadOne(ctx, tenantID, csEnv.ID, att.Filename, att.Content, env, &extra, log); err != nil {
			return env, err
		}
		env.Documents = append(env.Documents, extra)
	}

	// 8. Add signers + qualification + authentication requirements (one
	// requirement per signer × document × auth-atom).
	for i := range env.Signers {
		signerReq := signerRequest(env.Signers[i])
		signerID := env.Signers[i].SignerID
		if err := validateSignerForClicksign(signerReq); err != nil {
			log.Error("invalid signer payload — won't reach Clicksign",
				"signer_id", signerID, "name", signerReq.Name, "email", signerReq.Email, "error", err)
			env.MarkError(fmt.Sprintf("signer %s invalid: %v", signerID, err))
			_ = s.repo.Update(ctx, env)
			return env, fmt.Errorf("signer %s: %w", signerID, err)
		}
		csSigner, err := s.clicksign.AddSigner(ctx, tenantID, csEnv.ID, signerReq)
		if err != nil {
			// Include the actual name/email sent so the operator can spot
			// formatting issues (the Clicksign 400 "name não está em um
			// formato válido" message doesn't echo back the offending value).
			log.Error("add signer failed",
				"signer_id", signerID,
				"name", signerReq.Name,
				"email", signerReq.Email,
				"error", err)
			env.MarkError(fmt.Sprintf("add signer %s: %v", signerID, err))
			_ = s.repo.Update(ctx, env)
			return env, err
		}
		env.Signers[i].ClicksignSignerKey = csSigner.ID
		env.Signers[i].RequestSignatureKey = csSigner.Attributes.RequestSignatureKey
		log.Info("add signer ok",
			"signer_id", signerID,
			"signer_key", csSigner.ID,
			"group", env.Signers[i].Group,
			"has_request_key", csSigner.Attributes.RequestSignatureKey != "",
		)

		authAtoms := expandAuthMethods(env.Signers[i].Auth)
		if len(authAtoms) == 0 {
			// Fail loud — a signer without any auth requirement won't get
			// notified by Clicksign at all (this is the "documento não
			// chega" failure mode).
			log.Warn("signer has no auth methods (clicksign will not notify)", "signer_id", signerID)
		}

		for docIdx := range env.Documents {
			docKey := env.Documents[docIdx].ClicksignDocumentKey

			// Qualification (role) per document.
			if _, err := s.clicksign.AddRequirement(ctx, tenantID, csEnv.ID,
				clicksign.RequirementRequest{
					Action: clicksign.RequirementActionAgree,
					Role:   string(env.Signers[i].Role),
				},
				clicksign.RequirementRelationships{
					Document: docRel(docKey),
					Signer:   signerRel(csSigner.ID),
				},
			); err != nil {
				log.Error("add qualification requirement failed",
					"signer_id", signerID, "role", env.Signers[i].Role, "doc_key", docKey, "error", err)
				env.MarkError(fmt.Sprintf("add qualification req for %s (doc %s): %v", signerID, docKey, err))
				_ = s.repo.Update(ctx, env)
				return env, err
			}

			// Authentication: emit one Clicksign requirement per atom per
			// document. Composites like "selfie+document" already expanded
			// upstream into two atoms.
			for _, authVal := range authAtoms {
				if _, err := s.clicksign.AddRequirement(ctx, tenantID, csEnv.ID,
					clicksign.RequirementRequest{
						Action: clicksign.RequirementActionProvideEvidence,
						Auth:   authVal,
					},
					clicksign.RequirementRelationships{
						Document: docRel(docKey),
						Signer:   signerRel(csSigner.ID),
					},
				); err != nil {
					log.Error("add auth requirement failed",
						"signer_id", signerID, "auth", authVal, "doc_key", docKey, "error", err)
					env.MarkError(fmt.Sprintf("add %s auth req for %s (doc %s): %v", authVal, signerID, docKey, err))
					_ = s.repo.Update(ctx, env)
					return env, err
				}
				log.Info("add auth requirement ok", "signer_id", signerID, "auth", authVal, "doc_key", docKey)
			}
		}
	}

	// 9. Activate envelope, then explicitly trigger notifications. In
	// Clicksign v3, `status=running` makes the envelope actionable, but the
	// signing invitations are sent by POST /notifications.
	log.Info("activating envelope")
	if _, err := s.clicksign.ActivateEnvelope(ctx, tenantID, csEnv.ID); err != nil {
		log.Error("activate envelope failed", "error", err)
		env.MarkError(fmt.Sprintf("activate envelope: %v", err))
		_ = s.repo.Update(ctx, env)
		return env, err
	}
	log.Info("sending clicksign notifications")
	if err := s.clicksign.ResendNotification(ctx, tenantID, csEnv.ID, ""); err != nil {
		log.Error("send notifications failed", "error", err)
		env.MarkError(fmt.Sprintf("send notifications: %v", err))
		_ = s.repo.Update(ctx, env)
		return env, err
	}
	env.Status = envelope.StatusRunning
	env.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, env); err != nil {
		return env, fmt.Errorf("persist running envelope: %w", err)
	}
	log.Info("envelope running — pipeline complete", "signers_notified", len(env.Signers))
	recordEnvelopeAudit(ctx, "signature.envelope_created", env, map[string]any{
		"signers":     len(env.Signers),
		"environment": creds.Environment,
	})

	return env, nil
}

// CreateForDistrato cria o envelope de assinatura do DISTRATO de um
// contrato rescindido (#436): um SEGUNDO envelope do mesmo contrato, com
// o documento (distrato gerado em toca-deal) e os signatários vindos do
// evento — não do contrato em si.
//
// A pipeline Clicksign (criar envelope → upload → signers/requirements →
// ativar) é repetida de propósito em vez de extraída de CreateForContract:
// mantém esta adição estritamente aditiva, sem tocar no caminho de
// assinatura do contrato. Um refator de dedup pode vir depois.
//
// Idempotência: se já há um envelope de distrato ATIVO para o contrato,
// devolve-o. O envelope de contrato é ignorado aqui — os dois coexistem
// (índice único parcial em (contract_id, kind)).
func (s *EnvelopeService) CreateForDistrato(ctx context.Context, tenantID, contractID, dealID, documentURL string, signers []envelope.Signer) (*envelope.SignatureEnvelope, error) {
	if documentURL == "" {
		return nil, errors.New("distrato document_url is required")
	}
	if len(signers) == 0 {
		return nil, errors.New("distrato has no signers")
	}

	// 1. Idempotência — só considera um envelope de distrato ativo. O de
	// distrato, quando existe, é o mais recente do contrato.
	if existing, err := s.repo.GetByContractID(ctx, tenantID, contractID); err != nil {
		return nil, fmt.Errorf("check existing envelope: %w", err)
	} else if existing != nil && existing.Kind == envelope.EnvelopeKindDistrato && isActive(existing.Status) {
		return existing, nil
	}

	// 2. Credenciais (ambiente sandbox vs prod).
	creds, err := s.credsProv.Get(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("load clicksign credentials: %w", err)
	}

	// 3. Envelope local com kind=distrato.
	env, err := envelope.NewWithKind(tenantID, contractID, dealID, creds.Environment, envelope.EnvelopeKindDistrato, signers)
	if err != nil {
		return nil, err
	}

	log := logger.With(
		"component", "envelope_service",
		"envelope_id", env.ID,
		"contract_id", contractID,
		"tenant_id", tenantID,
		"kind", string(envelope.EnvelopeKindDistrato),
	)

	// 4. Render do PDF do distrato. O documento chega como URL — o
	// renderer baixa via HTTPS. Falha antes de tocar a Clicksign.
	pdfBytes, err := s.renderer.Render(ctx, contractID, documentURL)
	if err != nil {
		env.MarkError(fmt.Sprintf("distrato pdf fetch: %v", err))
		_ = s.repo.Create(ctx, env)
		return env, err
	}
	log.Info("clicksign distrato pipeline starting", "signers", len(env.Signers), "environment", creds.Environment)

	// 5. Cria o envelope na Clicksign e persiste a linha local.
	csEnv, err := s.clicksign.CreateEnvelope(ctx, tenantID, fmt.Sprintf("Distrato %s", contractID))
	if err != nil {
		log.Error("create distrato envelope failed", "error", err)
		env.MarkError(fmt.Sprintf("create envelope: %v", err))
		_ = s.repo.Create(ctx, env)
		return env, err
	}
	env.ExternalEnvelopeID = csEnv.ID
	env.Status = envelope.StatusDraft
	if err := s.repo.Create(ctx, env); err != nil {
		log.Error("persist distrato envelope failed (clicksign envelope leaked)", "external_id", csEnv.ID, "error", err)
		return nil, fmt.Errorf("persist envelope: %w", err)
	}

	// 6. Upload do PDF do distrato (índice 0).
	doc := envelope.Document{Filename: fmt.Sprintf("distrato-%s.pdf", contractID)}
	if err := s.uploadOne(ctx, tenantID, csEnv.ID, doc.Filename, pdfBytes, env, &doc, log); err != nil {
		return env, err
	}
	env.Documents = append(env.Documents, doc)

	// 7. Signers + requirements (qualification por role + authentication).
	for i := range env.Signers {
		signerReq := signerRequest(env.Signers[i])
		signerID := env.Signers[i].SignerID
		if err := validateSignerForClicksign(signerReq); err != nil {
			env.MarkError(fmt.Sprintf("signer %s invalid: %v", signerID, err))
			_ = s.repo.Update(ctx, env)
			return env, fmt.Errorf("signer %s: %w", signerID, err)
		}
		csSigner, err := s.clicksign.AddSigner(ctx, tenantID, csEnv.ID, signerReq)
		if err != nil {
			log.Error("add signer failed", "signer_id", signerID, "error", err)
			env.MarkError(fmt.Sprintf("add signer %s: %v", signerID, err))
			_ = s.repo.Update(ctx, env)
			return env, err
		}
		env.Signers[i].ClicksignSignerKey = csSigner.ID
		env.Signers[i].RequestSignatureKey = csSigner.Attributes.RequestSignatureKey

		authAtoms := expandAuthMethods(env.Signers[i].Auth)
		for docIdx := range env.Documents {
			docKey := env.Documents[docIdx].ClicksignDocumentKey
			if _, err := s.clicksign.AddRequirement(ctx, tenantID, csEnv.ID,
				clicksign.RequirementRequest{Action: clicksign.RequirementActionAgree, Role: string(env.Signers[i].Role)},
				clicksign.RequirementRelationships{Document: docRel(docKey), Signer: signerRel(csSigner.ID)},
			); err != nil {
				env.MarkError(fmt.Sprintf("add qualification req for %s (doc %s): %v", signerID, docKey, err))
				_ = s.repo.Update(ctx, env)
				return env, err
			}
			for _, authVal := range authAtoms {
				if _, err := s.clicksign.AddRequirement(ctx, tenantID, csEnv.ID,
					clicksign.RequirementRequest{Action: clicksign.RequirementActionProvideEvidence, Auth: authVal},
					clicksign.RequirementRelationships{Document: docRel(docKey), Signer: signerRel(csSigner.ID)},
				); err != nil {
					env.MarkError(fmt.Sprintf("add %s auth req for %s (doc %s): %v", authVal, signerID, docKey, err))
					_ = s.repo.Update(ctx, env)
					return env, err
				}
			}
		}
	}

	// 8. Ativa o envelope e dispara as notificações de assinatura.
	if _, err := s.clicksign.ActivateEnvelope(ctx, tenantID, csEnv.ID); err != nil {
		env.MarkError(fmt.Sprintf("activate envelope: %v", err))
		_ = s.repo.Update(ctx, env)
		return env, err
	}
	if err := s.clicksign.ResendNotification(ctx, tenantID, csEnv.ID, ""); err != nil {
		env.MarkError(fmt.Sprintf("send notifications: %v", err))
		_ = s.repo.Update(ctx, env)
		return env, err
	}
	env.Status = envelope.StatusRunning
	env.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, env); err != nil {
		return env, fmt.Errorf("persist running envelope: %w", err)
	}
	log.Info("distrato envelope running — pipeline complete", "signers_notified", len(env.Signers))
	recordEnvelopeAudit(ctx, "signature.envelope_created", env, map[string]any{
		"signers":     len(env.Signers),
		"environment": creds.Environment,
	})
	return env, nil
}

// ResendNotification asks Clicksign to re-send the signing notification for
// either a specific signer (signerID != "") or every pending signer in the
// envelope. Only valid for active envelopes — finished/canceled/refused/
// expired envelopes are rejected before any network call.
func (s *EnvelopeService) ResendNotification(ctx context.Context, tenantID, envelopeID, signerID string) (*envelope.SignatureEnvelope, error) {
	env, err := s.repo.GetByID(ctx, tenantID, envelopeID)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, ErrEnvelopeNotFound
	}
	if env.ExternalEnvelopeID == "" {
		return nil, fmt.Errorf("envelope has not been created on clicksign yet")
	}
	// Resend only makes sense while signers can still act — that is, after
	// activation (RUNNING/PARTIALLY_SIGNED). DRAFT/PENDING means the pipeline
	// hasn't activated yet; FINISHED/CANCELED/REFUSED/EXPIRED/ERROR are
	// terminal.
	if env.Status != envelope.StatusRunning && env.Status != envelope.StatusPartiallySigned {
		return nil, fmt.Errorf("envelope status %s does not allow resend", env.Status)
	}

	signerKey := ""
	if signerID != "" {
		s := env.SignerByID(signerID)
		if s == nil {
			return nil, ErrSignerNotFound
		}
		if s.Status == envelope.SignerStatusSigned {
			return env, fmt.Errorf("signer %s has already signed", signerID)
		}
		signerKey = s.ClicksignSignerKey
		if signerKey == "" {
			return env, fmt.Errorf("signer %s has no clicksign key (not yet added)", signerID)
		}
	}

	if err := s.clicksign.ResendNotification(ctx, tenantID, env.ExternalEnvelopeID, signerKey); err != nil {
		return env, fmt.Errorf("resend notification: %w", err)
	}
	return env, nil
}

// EditSignerInput captures what an operator can change about a signer that's
// still pending. Same shape as the request — no Auth here because changing
// the auth method post-creation requires also editing the Clicksign
// requirement record and is out of scope for the MVP.
type EditSignerInput struct {
	Name          string
	Email         string
	PhoneNumber   string
	Documentation string
	Birthday      string
}

// EditSigner updates a signer in-place on Clicksign and locally. Disallowed
// for terminal envelopes (FINISHED/REFUSED/CANCELED/EXPIRED/ERROR) and for
// signers that have already signed/refused — Clicksign rejects the PATCH
// anyway, but blocking here avoids a wasted round-trip and gives a precise
// error.
func (s *EnvelopeService) EditSigner(ctx context.Context, tenantID, envelopeID, signerID string, in EditSignerInput) (*envelope.SignatureEnvelope, error) {
	env, err := s.repo.GetByID(ctx, tenantID, envelopeID)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, ErrEnvelopeNotFound
	}
	if !isActive(env.Status) && env.Status != envelope.StatusFinished {
		return env, fmt.Errorf("envelope status %s does not allow signer edits", env.Status)
	}
	if env.Status == envelope.StatusFinished {
		return env, fmt.Errorf("envelope already finished; signers can't be edited")
	}

	sig := env.SignerByID(signerID)
	if sig == nil {
		return env, ErrSignerNotFound
	}
	if sig.Status == envelope.SignerStatusSigned || sig.Status == envelope.SignerStatusRefused {
		return env, fmt.Errorf("signer %s status %s does not allow edits", signerID, sig.Status)
	}

	cleanName := pkgvalidation.SanitizePersonName(in.Name)
	if err := pkgvalidation.ValidateFullPersonName(cleanName, "Nome do signatário"); err != nil {
		return env, err
	}
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return env, fmt.Errorf("email is empty")
	}
	hasDoc := strings.TrimSpace(in.Documentation) != ""
	req := clicksign.SignerRequest{
		Name:             cleanName,
		Email:            email,
		PhoneNumber:      strings.TrimSpace(in.PhoneNumber),
		HasDocumentation: hasDoc,
		Documentation:    strings.TrimSpace(in.Documentation),
		Birthday:         strings.TrimSpace(in.Birthday),
		Refusable:        true,
	}

	// Only call Clicksign if the signer was already pushed there. For
	// envelopes still in PENDING/DRAFT (pre-pipeline) the Clicksign signer
	// doesn't exist yet — local update is enough.
	if sig.ClicksignSignerKey != "" {
		if _, err := s.clicksign.UpdateSigner(ctx, tenantID, env.ExternalEnvelopeID, sig.ClicksignSignerKey, req); err != nil {
			return env, fmt.Errorf("update signer on clicksign: %w", err)
		}
	}

	// Apply locally. Mutate the slice entry directly — SignerByID returned
	// a pointer into env.Signers, but be explicit for readability.
	for i := range env.Signers {
		if env.Signers[i].SignerID != signerID {
			continue
		}
		env.Signers[i].Name = cleanName
		env.Signers[i].Email = email
		env.Signers[i].PhoneNumber = req.PhoneNumber
		env.Signers[i].Documentation = req.Documentation
		env.Signers[i].Birthday = req.Birthday
		break
	}
	env.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, env); err != nil {
		return env, err
	}
	return env, nil
}

// AddSignerInput is what a new signer needs to be appended to an existing
// envelope. Auth determines which Clicksign requirements get created — same
// expansion rules as the initial pipeline (composite "selfie+document"
// expands into two requirements).
type AddSignerInput struct {
	Name          string
	Email         string
	PhoneNumber   string
	Documentation string
	Birthday      string
	Role          envelope.SignerRole
	Auth          []envelope.AuthMethod
	// Group sets the Clicksign signing-order group for the new signer. 0
	// keeps the legacy behavior (signer notified at activation time).
	Group int
}

// AddSignerToEnvelope appends a new signer to an active envelope. Performs
// the same Clicksign pipeline as the initial CreateForContract (signer +
// qualification + auth requirements + notification) but for a single signer.
// Returns the updated envelope.
func (s *EnvelopeService) AddSignerToEnvelope(ctx context.Context, tenantID, envelopeID string, in AddSignerInput) (*envelope.SignatureEnvelope, error) {
	env, err := s.repo.GetByID(ctx, tenantID, envelopeID)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, ErrEnvelopeNotFound
	}
	if env.Status != envelope.StatusDraft && env.Status != envelope.StatusRunning && env.Status != envelope.StatusPartiallySigned {
		return env, fmt.Errorf("envelope status %s does not allow adding signers", env.Status)
	}
	if env.ExternalEnvelopeID == "" {
		return env, fmt.Errorf("envelope has not been created on clicksign yet")
	}
	if len(env.Documents) == 0 {
		return env, fmt.Errorf("envelope has no clicksign documents — pipeline incomplete")
	}

	cleanName := pkgvalidation.SanitizePersonName(in.Name)
	if err := pkgvalidation.ValidateFullPersonName(cleanName, "Nome do signatário"); err != nil {
		return env, err
	}
	email := strings.TrimSpace(in.Email)
	if email == "" {
		return env, fmt.Errorf("email is empty")
	}
	role := in.Role
	if role == "" {
		role = envelope.SignerRoleSign
	}
	auth := in.Auth
	if len(auth) == 0 {
		auth = []envelope.AuthMethod{envelope.AuthEmailToken}
	}

	hasDoc := strings.TrimSpace(in.Documentation) != ""
	req := clicksign.SignerRequest{
		Name:             cleanName,
		Email:            email,
		PhoneNumber:      strings.TrimSpace(in.PhoneNumber),
		HasDocumentation: hasDoc,
		Documentation:    strings.TrimSpace(in.Documentation),
		Birthday:         strings.TrimSpace(in.Birthday),
		Refusable:        true,
		Group:            in.Group,
	}
	csSigner, err := s.clicksign.AddSigner(ctx, tenantID, env.ExternalEnvelopeID, req)
	if err != nil {
		return env, fmt.Errorf("add signer on clicksign: %w", err)
	}

	// Requirements: one qualification + one auth-atom requirement per
	// (signer × document). Mirrors the CreateForContract pipeline.
	authAtoms := expandAuthMethods(auth)
	for docIdx := range env.Documents {
		docKey := env.Documents[docIdx].ClicksignDocumentKey
		if _, err := s.clicksign.AddRequirement(ctx, tenantID, env.ExternalEnvelopeID,
			clicksign.RequirementRequest{Action: clicksign.RequirementActionAgree, Role: string(role)},
			clicksign.RequirementRelationships{
				Document: docRel(docKey),
				Signer:   signerRel(csSigner.ID),
			},
		); err != nil {
			return env, fmt.Errorf("add qualification requirement (doc %s): %w", docKey, err)
		}
		for _, atom := range authAtoms {
			if _, err := s.clicksign.AddRequirement(ctx, tenantID, env.ExternalEnvelopeID,
				clicksign.RequirementRequest{Action: clicksign.RequirementActionProvideEvidence, Auth: atom},
				clicksign.RequirementRelationships{
					Document: docRel(docKey),
					Signer:   signerRel(csSigner.ID),
				},
			); err != nil {
				return env, fmt.Errorf("add %s auth requirement (doc %s): %w", atom, docKey, err)
			}
		}
	}

	// If the envelope is already RUNNING (signers are being notified), push
	// the invite for the new signer too — otherwise they'd sit forever.
	if env.Status == envelope.StatusRunning || env.Status == envelope.StatusPartiallySigned {
		if err := s.clicksign.ResendNotification(ctx, tenantID, env.ExternalEnvelopeID, csSigner.ID); err != nil {
			return env, fmt.Errorf("notify new signer: %w", err)
		}
	}

	// Append locally. Use a new internal ID (UUID-like) — Clicksign signer
	// keys are guaranteed unique, so we just reuse them for the local id
	// as well (consistent with how legacy signers map their SignerID).
	newSigner := envelope.Signer{
		SignerID:            csSigner.ID,
		ClicksignSignerKey:  csSigner.ID,
		Name:                cleanName,
		Email:               email,
		PhoneNumber:         req.PhoneNumber,
		Documentation:       req.Documentation,
		Birthday:            req.Birthday,
		Role:                role,
		Auth:                auth,
		Status:              envelope.SignerStatusPending,
		RequestSignatureKey: csSigner.Attributes.RequestSignatureKey,
		Group:               in.Group,
	}
	env.Signers = append(env.Signers, newSigner)
	env.UpdatedAt = s.now()
	if err := s.repo.Update(ctx, env); err != nil {
		return env, err
	}
	return env, nil
}

// Reconcile pulls the current envelope state from Clicksign and applies any
// status drift to the local copy + republishes the matching domain events.
// Built as the manual fallback for envelopes that didn't get webhook updates
// applied locally (e.g. payload shape regressions, RabbitMQ outages, or
// envelopes that ack'd 200 with "envelope not found locally" before the
// document.key fallback shipped).
//
// Idempotent: republished events carry deterministic EventIDs derived from
// the envelope external id so consumers (toca-deal) dedup via processed_events.
func (s *EnvelopeService) Reconcile(ctx context.Context, tenantID, envelopeID string) (*envelope.SignatureEnvelope, error) {
	return s.reconcile(ctx, tenantID, envelopeID, reconcileTenantScope)
}

// ReconcileServiceScope é o Reconcile do caminho de background (varredura de
// reconciliação): roda SEM JWT, então toda leitura/escrita no banco precisa ir
// pelo pool service-role.
//
// Sem isso a rede de segurança é decorativa: a política de RLS de
// signature.envelopes compara tenant_id com auth.jwt() e o pool de aplicação
// preenche essa claim a partir do tenant do CONTEXTO (applyJWTClaimsHook) — na
// varredura não existe tenant nenhum, então a claim vai vazia e a query devolve
// zero linhas. Medido no beta em 30/09/2026: `ListStaleActive` devolvia 0
// envelopes pelo pool de app e 2 pelo de serviço — por isso o envelope 9d5bd37a
// continuou RUNNING do nosso lado com o contrato em PENDING_SIGNATURE e o
// envelope já fechado na Clicksign.
//
// (Handler que recebe o tenant no contexto — consumidores de evento, requisição
// de usuário — NÃO precisa deste caminho: ali a claim é preenchida e a RLS
// segue filtrando por tenant.)
func (s *EnvelopeService) ReconcileServiceScope(ctx context.Context, tenantID, envelopeID string) (*envelope.SignatureEnvelope, error) {
	return s.reconcile(ctx, tenantID, envelopeID, reconcileServiceScope)
}

// reconcileScope diz qual credencial de banco o Reconcile pode usar.
type reconcileScope int

const (
	// reconcileTenantScope é o caminho da API (botão "Sincronizar"): o caller
	// tem JWT e a RLS filtra pelo tenant dele.
	reconcileTenantScope reconcileScope = iota
	// reconcileServiceScope é o caminho sem JWT (varredura).
	reconcileServiceScope
)

func (s *EnvelopeService) reconcile(ctx context.Context, tenantID, envelopeID string, scope reconcileScope) (*envelope.SignatureEnvelope, error) {
	var (
		env *envelope.SignatureEnvelope
		err error
	)
	if scope == reconcileServiceScope {
		env, err = s.repo.GetByIDService(ctx, envelopeID)
	} else {
		env, err = s.repo.GetByID(ctx, tenantID, envelopeID)
	}
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, ErrEnvelopeNotFound
	}
	if env.ExternalEnvelopeID == "" {
		return nil, fmt.Errorf("envelope has not been created on clicksign yet")
	}

	remote, err := s.clicksign.GetEnvelope(ctx, tenantID, env.ExternalEnvelopeID)
	if err != nil {
		return env, fmt.Errorf("fetch envelope from clicksign: %w", err)
	}
	if remote == nil {
		return env, fmt.Errorf("clicksign returned empty envelope")
	}

	now := s.now()
	deliveryID := "reconcile:" + env.ExternalEnvelopeID + ":" + now.UTC().Format(time.RFC3339)

	// Clicksign envelope statuses we care about: "running", "closed",
	// "canceled". The exact strings depend on the API version — match
	// defensively.
	remoteStatus := strings.ToLower(strings.TrimSpace(remote.Attributes.Status))
	switch remoteStatus {
	case "closed", "finished":
		if env.Status == envelope.StatusFinished {
			return env, nil // already aligned
		}
		// Mark every still-pending signer as signed (we don't have per-signer
		// timestamps from GetEnvelope, so we use `now` as the best estimate
		// and stage a signer_signed event for each so toca-deal can reflect
		// them). All events go into one slice so the atomic helper publishes
		// them in a single transaction with the envelope Update.
		var evts []events.Event
		for i := range env.Signers {
			if env.Signers[i].Status != envelope.SignerStatusSigned {
				signed := now
				env.Signers[i].Status = envelope.SignerStatusSigned
				env.Signers[i].SignedAt = &signed
				evts = append(evts, s.buildSignerSignedReconcile(env, &env.Signers[i], now, deliveryID))
			}
		}
		env.Status = envelope.StatusFinished
		env.FinishedAt = &now
		env.UpdatedAt = now
		// Hydrate the signed PDF URL from Clicksign so the persist
		// subscriber on the toca-deal side can download it. Without
		// this, a reconcile that finds an envelope freshly `closed`
		// would emit envelope_finished with an empty URL — same
		// failure mode as the original webhook race.
		signedDoc := s.fetchRemoteSignedDocURL(ctx, tenantID, env.ExternalEnvelopeID, env.ContractID)
		evts = append(evts, s.buildEnvelopeFinishedReconcile(env, now, deliveryID, signedDoc))
		if err := s.saveEnvelopeAndPublishScoped(ctx, scope, env, evts...); err != nil {
			return env, err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_reconciled", env, map[string]any{"source": "reconcile", "remote_status": remoteStatus})
	case "canceled":
		if env.Status == envelope.StatusCanceled && env.TerminalEventAnnounced() {
			return env, nil
		}
		// Local já cancelado sem o desfecho anunciado: o toca-deal nunca soube
		// e o contrato segue travado em PENDING_SIGNATURE. Não é "já alinhado"
		// — é o estado preso que o carimbo existe para detectar (incidente de
		// 2026-10-05: operador cancelou, o cancelamento no provedor falhou, o
		// envelope ficou CANCELED local e o contrato sem botão de reenviar).
		env.MarkCanceled()
		evt := s.buildRefusedReconcile(env, "envelope canceled (reconciled)", now, deliveryID)
		if err := s.saveEnvelopeAndPublishScoped(ctx, scope, env, evt); err != nil {
			return env, err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_reconciled", env, map[string]any{"source": "reconcile", "remote_status": remoteStatus})
	case "running":
		// Remote says running and local says something else — promote local
		// to RUNNING + emit envelope_started so toca-deal can move the
		// contract out of DRAFT.
		var evts []events.Event
		if env.Status == envelope.StatusDraft || env.Status == envelope.StatusPending {
			env.Status = envelope.StatusRunning
			env.UpdatedAt = now
			evts = append(evts, s.buildEnvelopeStartedReconcile(env, now, deliveryID))
		}
		// Per-signer drift detection. GetEnvelope above only surfaces
		// envelope-level status, never the individual signer state.
		// ListSigners fetches each signer's current status from Clicksign
		// directly — recovers from cases where individual sign webhooks
		// were lost between Clicksign and toca-signature. Apply any
		// transitions locally and emit matching domain events so the
		// downstream toca-deal projection catches up.
		signerEvts, perSignerDirty := s.reconcileSignersFromRemote(ctx, tenantID, env, now, deliveryID)
		evts = append(evts, signerEvts...)
		if len(evts) > 0 {
			if perSignerDirty {
				env.UpdatedAt = now
			}
			if err := s.saveEnvelopeAndPublishScoped(ctx, scope, env, evts...); err != nil {
				return env, err
			}
		}
	default:
		// Unknown / nothing to reconcile — return as-is. Could be "draft"
		// (operator created on Clicksign side and we haven't picked it up).
	}
	return env, nil
}

// reconcileSignersFromRemote calls Clicksign's ListSigners and applies any
// per-signer transitions (PENDING → SIGNED / REFUSED) to the local envelope,
// returning the matching domain events for atomic publish. Used by
// Reconcile when the envelope is mid-flow ("running") to recover from
// lost individual sign webhooks. Errors are logged and the function
// continues — the envelope-status reconcile alone is still useful.
func (s *EnvelopeService) reconcileSignersFromRemote(ctx context.Context, tenantID string, env *envelope.SignatureEnvelope, now time.Time, deliveryID string) ([]events.Event, bool) {
	if env == nil || env.ExternalEnvelopeID == "" {
		return nil, false
	}
	remoteSigners, err := s.clicksign.ListSigners(ctx, tenantID, env.ExternalEnvelopeID)
	if err != nil {
		logger.Warn("Reconcile: ListSigners failed; falling back to envelope-only reconcile", map[string]interface{}{
			"envelope_id":          env.ID,
			"external_envelope_id": env.ExternalEnvelopeID,
			"error":                err.Error(),
		})
		return nil, false
	}
	if len(remoteSigners) == 0 {
		return nil, false
	}

	var evts []events.Event
	dirty := false
	for _, remote := range remoteSigners {
		remoteStatus := strings.ToLower(strings.TrimSpace(remote.Attributes.Status))
		// We only act on terminal-style remote states. "pending"/"viewed"
		// stay as-is locally because we don't model VIEWED at the envelope
		// level (toca-deal Contract.Signatory has VIEWED but envelope.Signer
		// only has PENDING/SIGNED/REFUSED).
		var targetStatus envelope.SignerStatus
		switch remoteStatus {
		case "signed", "completed", "closed":
			targetStatus = envelope.SignerStatusSigned
		case "refused", "declined":
			targetStatus = envelope.SignerStatusRefused
		default:
			continue
		}
		for i := range env.Signers {
			local := &env.Signers[i]
			matched := local.ClicksignSignerKey == remote.ID ||
				(remote.Attributes.Email != "" && strings.EqualFold(local.Email, remote.Attributes.Email))
			if !matched {
				continue
			}
			if local.Status == targetStatus {
				break
			}
			local.Status = targetStatus
			if targetStatus == envelope.SignerStatusSigned && local.SignedAt == nil {
				if ts, perr := time.Parse(time.RFC3339, remote.Attributes.SignedAt); perr == nil {
					local.SignedAt = &ts
				} else {
					local.SignedAt = &now
				}
				evts = append(evts, s.buildSignerSignedReconcile(env, local, now, deliveryID))
			}
			if targetStatus == envelope.SignerStatusRefused {
				reason := remote.Attributes.RefusedReason
				if reason == "" {
					reason = "refused on clicksign (reconciled)"
				}
				evts = append(evts, s.buildRefusedReconcile(env, reason, now, deliveryID))
			}
			dirty = true
			break
		}
	}
	return evts, dirty
}

// build*Reconcile factor the event-construction out of Reconcile so the
// atomic saveEnvelopeAndPublish helper can stage multiple events for the
// same envelope update. Deterministic EventIDs derived from the reconcile
// deliveryID guarantee that re-running Reconcile is a downstream no-op
// (consumers dedup via events.processed_events).
func (s *EnvelopeService) buildSignerSignedReconcile(env *envelope.SignatureEnvelope, signer *envelope.Signer, at time.Time, deliveryID string) pkgevents.SignatureSignerSignedEvent {
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
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *EnvelopeService) buildEnvelopeFinishedReconcile(env *envelope.SignatureEnvelope, at time.Time, deliveryID, signedDoc string) pkgevents.SignatureEnvelopeFinishedEvent {
	return pkgevents.SignatureEnvelopeFinishedEvent{
		EventIDVal: deliveryID + ":finished",
		Data: pkgevents.SignatureEnvelopeFinishedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			FinishedAt:         at.UTC().Format(time.RFC3339),
			// Reconcile path used to omit SignedDocumentURL, which left
			// the toca-deal persist subscriber permanently in no-op when
			// Clicksign sent the original `envelope_finished` webhook
			// without the URL ready (CDN race). Callers in Reconcile
			// now hydrate this via fetchRemoteSignedDocURL before
			// publishing — empty string is still allowed for the cases
			// where Clicksign genuinely hasn't produced the file yet.
			SignedDocumentURL: signedDoc,
			Timestamp:         s.now().UTC().Format(time.RFC3339),
		},
	}
}

// fetchRemoteSignedDocURL queries Clicksign for the envelope's documents and
// returns the signed-PDF URL of the contract document. Used by Reconcile and
// by the on-demand refresh endpoint to recover from `envelope_finished`
// payloads that arrived without the signed URL populated.
//
// contractID selects the right document: a multi-document envelope also
// carries attachments, so SignedContractURL matches "contrato-<id>.pdf"
// rather than blindly taking the first entry.
//
// Best-effort: a network error or empty list returns "" with a nil error so
// the caller can still publish a (URL-less) finished event rather than
// failing the whole reconcile. The deal-side backfill or a subsequent
// refresh call picks it up later.
func (s *EnvelopeService) fetchRemoteSignedDocURL(ctx context.Context, tenantID, externalEnvelopeID, contractID string) string {
	if externalEnvelopeID == "" {
		return ""
	}
	docs, err := s.clicksign.ListDocuments(ctx, tenantID, externalEnvelopeID)
	if err != nil || len(docs) == 0 {
		return ""
	}
	return clicksign.SignedContractURL(docs, contractID)
}

// RefreshSignedDocumentURL is the on-demand recovery path for the
// toca-deal backfill: given a contract whose `SignedDocumentURL`
// stayed empty (because the original `envelope_finished` webhook came
// in without it), we hit Clicksign's documents endpoint to get the
// current URL. Returns "" with nil error when the envelope simply
// doesn't have a signed document yet (signing in progress or
// genuinely no document) — caller treats that as "skip, try again
// later".
//
// Errors are returned only for transport-level failures the caller
// should surface (e.g. unknown contract, Clicksign auth down). The
// distinction matters for metrics: empty-string is operational
// (CDN race), error is infrastructure.
func (s *EnvelopeService) RefreshSignedDocumentURL(ctx context.Context, tenantID, contractID string) (string, error) {
	if tenantID == "" || contractID == "" {
		return "", fmt.Errorf("tenant id and contract id are required")
	}
	env, err := s.repo.GetByContractID(ctx, tenantID, contractID)
	if err != nil {
		return "", fmt.Errorf("lookup envelope: %w", err)
	}
	if env == nil {
		return "", ErrEnvelopeNotFound
	}
	if env.ExternalEnvelopeID == "" {
		// Envelope was never registered with Clicksign — nothing to
		// refresh. Empty string keeps the contract behaving like
		// "still being prepared".
		return "", nil
	}
	return s.fetchRemoteSignedDocURL(ctx, tenantID, env.ExternalEnvelopeID, contractID), nil
}

func (s *EnvelopeService) buildRefusedReconcile(env *envelope.SignatureEnvelope, reason string, at time.Time, deliveryID string) pkgevents.SignatureRefusedEvent {
	return pkgevents.SignatureRefusedEvent{
		EventIDVal: deliveryID + ":refused",
		Data: pkgevents.SignatureRefusedEventPayload{
			TenantID:           env.TenantID,
			ContractID:         env.ContractID,
			EnvelopeID:         env.ID,
			ExternalEnvelopeID: env.ExternalEnvelopeID,
			Reason:             reason,
			RefusedAt:          at.UTC().Format(time.RFC3339),
			Timestamp:          s.now().UTC().Format(time.RFC3339),
		},
	}
}

func (s *EnvelopeService) buildEnvelopeStartedReconcile(env *envelope.SignatureEnvelope, at time.Time, deliveryID string) pkgevents.SignatureEnvelopeStartedEvent {
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

// Cancel encerra o envelope no provedor e libera o contrato no toca-deal.
//
// Três coisas precisam acontecer juntas, e antes desta correção nenhuma
// acontecia de verdade:
//
//  1. cancelar NO PROVEDOR — feito por documento (ver clicksign.CancelEnvelope:
//     a v3 recusa `status=canceled` no envelope). Sem isso o link de assinatura
//     continua válido e alguém pode assinar um documento que o operador já deu
//     por cancelado. Falha aqui SOBE: o operador precisa saber que o link
//     segue vivo (antes a falha era engolida em last_error, o envelope ficava
//     CANCELED local e a tela dizia "cancelado").
//  2. liberar o CONTRATO — o envelope ser CANCELED do nosso lado não muda nada
//     no toca-deal, que continua com o contrato em PENDING_SIGNATURE e travado
//     para reenvio. Publicamos o mesmo `signature.refused` que o webhook
//     `cancel` publica (SignerID vazio), que é o caminho que devolve o
//     contrato para rascunho.
//  3. ser idempotente — se o envelope já está cancelado e o desfecho JÁ foi
//     anunciado, repetir a chamada não republica nada; se está cancelado e o
//     anúncio nunca saiu (o caso que prende o contrato), ele sai agora.
func (s *EnvelopeService) Cancel(ctx context.Context, tenantID, envelopeID string) (*envelope.SignatureEnvelope, error) {
	env, err := s.repo.GetByID(ctx, tenantID, envelopeID)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, errors.New("envelope not found")
	}
	// Uma vez só: o caminho de escrita depende de haver tenant no contexto
	// (requisição do operador) ou não (worker/chamada de serviço).
	scope := s.writeScopeFor(ctx)
	if env.ExternalEnvelopeID != "" {
		if err := s.clicksign.CancelEnvelope(ctx, tenantID, env.ExternalEnvelopeID); err != nil {
			env.LastError = fmt.Sprintf("cancel on clicksign failed: %v", err)
			// Persistir o motivo para o operador ver na tela do envelope; o
			// status local NÃO muda — dizer "cancelado" com o link vivo é o
			// defeito que esta correção elimina.
			if updateErr := s.saveEnvelopeAndPublishScoped(ctx, scope, env); updateErr != nil {
				log.Printf("cancel: persist last_error for envelope %s: %v", env.ID, updateErr)
			}
			return nil, fmt.Errorf("%w: cancel on clicksign failed: %v", ErrCancelProviderFailed, err)
		}
	}
	env.LastError = ""
	wasCanceled := env.Status == envelope.StatusCanceled
	env.MarkCanceled()

	if env.TerminalEventAnnounced() {
		// Já cancelado e já anunciado: nada a publicar, mas o estado local
		// segue consistente.
		if err := s.saveEnvelopeAndPublishScoped(ctx, scope, env); err != nil {
			return nil, err
		}
		recordEnvelopeAudit(ctx, "signature.envelope_canceled", env, map[string]any{"source": "operator", "already_announced": true})
		return env, nil
	}

	now := s.now()
	evt := s.buildRefusedReconcile(env, "cancelado pelo operador", now, "operator-cancel:"+env.ID)
	if err := s.saveEnvelopeAndPublishScoped(ctx, scope, env, evt); err != nil {
		return nil, err
	}
	recordEnvelopeAudit(ctx, "signature.envelope_canceled", env, map[string]any{
		"source":           "operator",
		"already_canceled": wasCanceled,
	})
	return env, nil
}

// writeScopeFor escolhe a credencial de banco do caminho de escrita: com tenant
// no contexto (requisição de usuário) o pool de app basta e a RLS filtra; sem
// tenant (worker, varredura, chamada interna de serviço) a linha é invisível
// para o pool de app e a escrita tem de ir pelo pool service-role. Errar essa
// escolha é o defeito recorrente desta base: o UPDATE afeta zero linhas e o
// caller recebe "envelope não encontrado" com a linha existindo.
func (s *EnvelopeService) writeScopeFor(ctx context.Context) reconcileScope {
	if tenantID, err := pkgcontext.GetTenantID(ctx); err == nil && strings.TrimSpace(tenantID) != "" {
		return reconcileTenantScope
	}
	return reconcileServiceScope
}

// helpers -------------------------------------------------------------------

func isActive(s envelope.Status) bool {
	switch s {
	case envelope.StatusCanceled, envelope.StatusExpired, envelope.StatusRefused, envelope.StatusError:
		return false
	}
	return true
}

func signersFromContract(sigs []clients.ContractSignatoryDTO, authByID map[string][]envelope.AuthMethod, groupByID map[string]int) []envelope.Signer {
	out := make([]envelope.Signer, 0, len(sigs))
	for _, s := range sigs {
		auth := authByID[s.ID]
		if len(auth) == 0 {
			// fall back to whatever the contract carries; default to email.
			for _, a := range s.AuthMethods {
				auth = append(auth, envelope.AuthMethod(a))
			}
			if len(auth) == 0 {
				auth = []envelope.AuthMethod{envelope.AuthEmailToken}
			}
		}
		role := envelope.SignerRoleSign
		if strings.EqualFold(s.Role, "testemunha") || strings.EqualFold(s.Role, "witness") {
			role = envelope.SignerRoleWitness
		}
		// Group precedence: per-envelope override > contract default > 0.
		group := s.Group
		if g, ok := groupByID[s.ID]; ok {
			group = g
		}
		out = append(out, envelope.Signer{
			SignerID:      s.ID,
			Name:          s.Name,
			Email:         s.Email,
			PhoneNumber:   s.PhoneNumber,
			Documentation: s.Documentation,
			Birthday:      s.Birthday,
			Role:          role,
			Auth:          auth,
			Status:        envelope.SignerStatusPending,
			Group:         group,
		})
	}
	return out
}

func envelopeName(c *clients.ContractDTO) string {
	if c.DisplayID != "" {
		return "Contrato " + c.DisplayID
	}
	return "Contrato " + c.ID
}

// DocumentRequest builds the Clicksign /documents payload for the contract's
// main PDF. Kept for backward-compatible callers and tests; new attachment
// uploads should use DocumentRequestWithFilename directly.
func DocumentRequest(contractID string, pdf []byte) clicksign.DocumentRequest {
	return DocumentRequestWithFilename(contractFilename(contractID), pdf)
}

// DocumentRequestWithFilename wraps an already-typed PDF as a Clicksign
// /documents upload. The caller owns the filename — used for attachments
// whose name is set by the operator or pulled from toca-deal.
func DocumentRequestWithFilename(filename string, pdf []byte) clicksign.DocumentRequest {
	return clicksign.DocumentRequest{
		Filename:      filename,
		ContentBase64: "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(pdf),
	}
}

func contractFilename(contractID string) string {
	return fmt.Sprintf("contrato-%s.pdf", contractID)
}

func signerRequest(s envelope.Signer) clicksign.SignerRequest {
	return clicksign.SignerRequest{
		Name:             pkgvalidation.SanitizePersonName(s.Name),
		Email:            strings.TrimSpace(s.Email),
		PhoneNumber:      strings.TrimSpace(s.PhoneNumber),
		HasDocumentation: s.Documentation != "",
		Documentation:    strings.TrimSpace(s.Documentation),
		Birthday:         strings.TrimSpace(s.Birthday),
		Refusable:        true,
		Group:            s.Group,
	}
}

// collectAttachments materializes every extra document the caller wants on
// the envelope: bytes from toca-deal's Document entity plus operator inline
// uploads. Order is preserved (deal-stored docs first, then inline). An empty
// opts.AttachmentDocIDs + opts.InlineAttachments returns nil, nil — the most
// common case for envelopes without attachments.
func (s *EnvelopeService) collectAttachments(ctx context.Context, tenantID string, opts CreateOptions) ([]InlineAttachment, error) {
	total := len(opts.AttachmentDocIDs) + len(opts.InlineAttachments)
	if total == 0 {
		return nil, nil
	}
	out := make([]InlineAttachment, 0, total)
	for _, docID := range opts.AttachmentDocIDs {
		filename, content, err := s.dealClient.FetchDocumentContent(ctx, tenantID, docID)
		if err != nil {
			return nil, fmt.Errorf("fetch attachment %s: %w", docID, err)
		}
		if len(content) == 0 {
			return nil, fmt.Errorf("attachment %s has no content", docID)
		}
		if filename == "" {
			filename = fmt.Sprintf("anexo-%s.pdf", docID)
		}
		out = append(out, InlineAttachment{Filename: filename, Content: content})
	}
	for _, in := range opts.InlineAttachments {
		if len(in.Content) == 0 {
			return nil, fmt.Errorf("inline attachment %q has no content", in.Filename)
		}
		filename := in.Filename
		if filename == "" {
			filename = "anexo.pdf"
		}
		out = append(out, InlineAttachment{Filename: filename, Content: in.Content})
	}
	return out, nil
}

// uploadOne posts a single document to Clicksign and fills the local
// envelope.Document with the returned key, hash and size. On error it marks
// the local envelope as ERROR and persists before returning, matching the
// existing failure handling on the rest of the pipeline.
func (s *EnvelopeService) uploadOne(ctx context.Context, tenantID, externalEnvelopeID, filename string, content []byte, env *envelope.SignatureEnvelope, doc *envelope.Document, log *logger.Logger) error {
	req := DocumentRequestWithFilename(filename, content)
	csDoc, err := s.clicksign.UploadDocument(ctx, tenantID, externalEnvelopeID, req)
	if err != nil {
		log.Error("upload document failed", "filename", filename, "error", err)
		env.MarkError(fmt.Sprintf("upload document %q: %v", filename, err))
		_ = s.repo.Update(ctx, env)
		return err
	}
	doc.ClicksignDocumentKey = csDoc.ID
	doc.SHA256 = sha256Hex(content)
	doc.SizeBytes = int64(len(content))
	doc.Filename = req.Filename
	log.Info("upload document ok", "document_key", csDoc.ID, "filename", req.Filename, "size_bytes", len(content))
	return nil
}

// validateSignerForClicksign is the last-mile check before we hit the
// Clicksign API. The platform-wide rules live in pkg/validation — duplicate
// them here only to add fields specific to the Clicksign POST shape (Email).
//
// All upstream entry points (toca-auth UpdateProfile, toca-deal AddSigner)
// already run the same name rules, so reaching this with an invalid name
// means a record predates the validation or came in through a path we
// haven't blindado yet. Failing fast here keeps API quota intact and
// surfaces a precise error in the log.
func validateSignerForClicksign(req clicksign.SignerRequest) error {
	if err := pkgvalidation.ValidateFullPersonName(req.Name, "name"); err != nil {
		return err
	}
	if req.Email == "" {
		return fmt.Errorf("email is empty")
	}
	return nil
}

func docRel(id string) *clicksign.RequirementRel {
	return &clicksign.RequirementRel{Data: clicksign.RequirementRelData{Type: "documents", ID: id}}
}

func signerRel(id string) *clicksign.RequirementRel {
	return &clicksign.RequirementRel{Data: clicksign.RequirementRelData{Type: "signers", ID: id}}
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// expandAuthMethods turns our internal AuthMethod list into the flat list of
// Clicksign `auth` strings that need separate provide_evidence requirements.
//
// Most methods are 1:1; "selfie+document" is the special case — Clicksign has
// no combined token for it, so we emit BOTH `selfie` AND `official_document`
// as separate requirements (the signer must satisfy both).
//
// Duplicates are dropped so the operator selecting overlapping methods (e.g.
// `selfie` + `selfie+document`) doesn't create redundant requirements.
func expandAuthMethods(methods []envelope.AuthMethod) []string {
	out := make([]string, 0, len(methods))
	seen := map[string]bool{}
	add := func(v string) {
		if v == "" || seen[v] {
			return
		}
		seen[v] = true
		out = append(out, v)
	}
	for _, m := range methods {
		switch m {
		case envelope.AuthEmailToken:
			add("email")
		case envelope.AuthWhatsApp:
			add("whatsapp")
		case envelope.AuthSMS:
			add("sms")
		case envelope.AuthSelfie:
			add("selfie")
		case envelope.AuthICPBrasil:
			add("icp_brasil")
		case envelope.AuthSelfiePlusDoc:
			add("selfie")
			add("official_document")
		}
	}
	return out
}
