package services

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/clients"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	pkgevents "github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// ErrContractLockedForSignature is returned by UpdateContract when the contract
// has been sent for external signature. The operator must cancel the envelope
// via the signature service before edits are allowed again.
var ErrContractLockedForSignature = errors.New("contract is locked for signature")

// DocumentStatusSyncer is the minimum surface contract_service needs from
// DocumentService to keep the Central de Documentos in sync. Defined as an
// interface so the wiring is optional (services can be initialized in any
// order, and a nil syncer simply skips the broadcast).
type DocumentStatusSyncer interface {
	SyncFromContractStatus(ctx context.Context, tenantID, contractID, contractStatus string) (int, error)
	// EnsureForContract creates a placeholder Document row linked to the
	// contract if none exists yet. Lets the Central de Documentos surface
	// contracts the operator hasn't manually uploaded a file for — the
	// page shows them in "Rascunho" until signature events promote them
	// through the lifecycle. Idempotent.
	//
	// uploadedBy stamps the actor that triggered creation (operator name /
	// account id when called from a handler; "system" or "Clicksign" when
	// called from a webhook recovery path).
	EnsureForContract(ctx context.Context, tenantID, contractID, dealID, name, uploadedBy string) error
	// UploadDocxForContract persists the contract's DOCX content (decoded
	// bytes, not base64) to storage and updates the linked Document so the
	// Central de Documentos shows a real file size + download URL. Idempotent
	// on content hash. Skipped automatically once the document has crossed
	// into pendente_assinatura/assinado — the signed PDF takes over.
	UploadDocxForContract(ctx context.Context, tenantID, contractID string, docxBytes []byte, uploadedBy string) error
	// GetSignedFileURL returns a short-lived presigned URL for the
	// contract's signed PDF when it has been cached in our storage.
	// Returns empty string when no local copy exists yet (the async
	// persist handler hasn't run, or the contract isn't signed).
	GetSignedFileURL(ctx context.Context, tenantID, contractID string) (string, error)
	// EnsureSignedPDFURL returns a browser-safe presigned MinIO URL for the
	// contract's signed PDF, downloading + caching it from Clicksign first
	// when no local copy exists. Never returns the raw Clicksign URL (which
	// the browser cannot load — no CORS, ~5 min TTL).
	EnsureSignedPDFURL(ctx context.Context, tenantID, contractID string) (string, error)
}

type ContractService struct {
	contractRepo          contract.Repository
	dealRepo              deal.Repository
	eventBus              events.EventBus
	db                    *database.PostgresDB
	signature             SignatureEnvelopeCreator
	documents             DocumentStatusSyncer
	exitInspectionChecker ExitInspectionChecker
	financialPlanChecker  FinancialPlanChecker
}

// WithDocumentSyncer wires a DocumentStatusSyncer (typically the
// DocumentService) so the Central de Documentos stays in sync with the
// contract's signature lifecycle. Optional — without it, document statuses
// remain whatever was set at upload time.
func (s *ContractService) WithDocumentSyncer(syncer DocumentStatusSyncer) *ContractService {
	s.documents = syncer
	return s
}

// syncDocuments fires the Document-side mirror after a contract status
// transition. Best-effort: a failure here is logged but never blocks the
// signature event handler — the contract change is the canonical state,
// document status is a derived projection.
func (s *ContractService) syncDocuments(ctx context.Context, c *contract.Contract) {
	if s.documents == nil || c == nil {
		return
	}
	if _, err := s.documents.SyncFromContractStatus(ctx, c.TenantID, c.ID, string(c.Status)); err != nil {
		log.Printf("document status sync failed for contract %s: %v", c.ID, err)
	}
}

// syncDealSignerStatus mirrors a contract signatory status change onto the
// owning Deal's Signers[] and Parties[] arrays. These are denormalized
// projections used by the admin "Esteira" UI (ContractTab signer rows,
// DealPartiesSection PartyStatusPill) — without this mirror, contract
// updates fire but the admin sidebar still shows "Pendente".
//
// signerID is the internal Signatory.ID (shared between Contract.Signatory
// and Deal.Signer/Deal.Party — they're generated as the same UUID when the
// contract is created from the deal).
//
// Returns the deal repo error (load or update). Callers tied to a SIGNED /
// REFUSED transition should propagate it so the event bus redelivers; the
// VIEWED path is informational and can swallow the error after logging.
func (s *ContractService) syncDealSignerStatus(ctx context.Context, tenantID, dealID, signerID string, status deal.SignerStatus) error {
	return s.syncDealSignerStatusWithEmail(ctx, tenantID, dealID, signerID, "", status)
}

// syncDealSignerStatusWithEmail is the email-aware variant used by callers
// that have the signer's email handy (every Contract.Signatory does). The
// email is a tie-breaker when the Contract.Signatory.ID and Deal.Signer.ID
// have drifted — see Deal.UpdateSignerStatusByIDOrEmail for the matching
// rules. Without this projection the production "Pendente que não atualiza"
// symptom appears: contract goes SIGNED but per-signer rows lag forever.
func (s *ContractService) syncDealSignerStatusWithEmail(ctx context.Context, tenantID, dealID, signerID, signerEmail string, status deal.SignerStatus) error {
	if dealID == "" || (signerID == "" && signerEmail == "") {
		return nil
	}
	d, err := s.dealRepo.GetByID(ctx, tenantID, dealID)
	if err != nil {
		return fmt.Errorf("load deal %s for signer %s sync: %w", dealID, signerID, err)
	}
	if d == nil {
		return nil
	}
	d.UpdateSignerStatusByIDOrEmail(signerID, signerEmail, status, "")
	if err := s.dealRepo.Update(ctx, d); err != nil {
		return fmt.Errorf("update deal %s for signer %s sync: %w", dealID, signerID, err)
	}
	return nil
}

// syncAllDealSignersFromContract mirrors every signatory's status from the
// Contract.Signatories array onto the deal's Signers/Parties. Used by
// envelope_finished (where every signer is now SIGNED) and by refused
// (where the offending signer needs to flip on the deal side too).
//
// Returns the deal repo error; callers in terminal transitions must
// propagate it so RabbitMQ redelivers, otherwise the contract goes SIGNED
// but the deal stays with "Pendente" rows and only the manual Reconciliar
// button can recover.
func (s *ContractService) syncAllDealSignersFromContract(ctx context.Context, c *contract.Contract) error {
	return s.syncAllDealSignersFromContractWithTx(ctx, nil, c)
}

// syncAllDealSignersFromContractWithTx is the tx-aware variant. When tx is
// non-nil the deal Update runs through dealRepo.UpdateInTx so it commits
// atomically with the surrounding work (typically a contract Update +
// EnqueueOutboxInTx in the same callback). tx == nil falls back to the
// default pool — preserves the existing non-atomic semantics for callers
// that don't need atomicity.
func (s *ContractService) syncAllDealSignersFromContractWithTx(ctx context.Context, tx *sqlx.Tx, c *contract.Contract) error {
	if c == nil || c.DealID == "" {
		return nil
	}
	// When inside a tx, read via the same tx so we don't acquire a second
	// pool connection while holding the first — under load that pattern
	// self-deadlocks the pool (every concurrent tx holds one conn and
	// waits for a second). Same-tx read also gets a consistent snapshot
	// of the deal row alongside the contract update.
	var (
		d   *deal.Deal
		err error
	)
	if tx != nil {
		d, err = s.dealRepo.GetByIDInTx(ctx, tx, c.TenantID, c.DealID)
	} else {
		d, err = s.dealRepo.GetByID(ctx, c.TenantID, c.DealID)
	}
	if err != nil {
		return fmt.Errorf("load deal %s for bulk signer sync: %w", c.DealID, err)
	}
	if d == nil {
		return nil
	}
	// Snapshot signer statuses BEFORE the sync so we can tell whether any
	// row actually changed — drift detected vs. idempotent re-apply. Used
	// to increment metrics.signer_projection_drift_total only when the
	// projection was genuinely stale.
	before := make(map[string]deal.SignerStatus, len(d.Signers))
	for _, s := range d.Signers {
		before[s.ID] = s.Status
	}

	for _, sig := range c.Signatories {
		mapped := mapContractStatusToDeal(sig.Status)
		if mapped == "" {
			continue
		}
		d.UpdateSignerStatusByIDOrEmail(sig.ID, sig.Email, mapped, "")
	}

	drifted := false
	for _, s := range d.Signers {
		if prev, ok := before[s.ID]; ok && prev != s.Status {
			drifted = true
			break
		}
	}
	if drifted {
		metrics.IncSignerProjectionDrift(c.TenantID)
	}

	if tx != nil {
		if err := s.dealRepo.UpdateInTx(ctx, tx, d); err != nil {
			return fmt.Errorf("update deal %s for bulk signer sync (tx): %w", c.DealID, err)
		}
		return nil
	}
	if err := s.dealRepo.Update(ctx, d); err != nil {
		return fmt.Errorf("update deal %s for bulk signer sync: %w", c.DealID, err)
	}
	return nil
}

// saveContractWithEvents persists the contract, optionally syncs the
// deal's signer projection, and emits the given outbox events — all in a
// single Postgres transaction when db + RabbitMQ bus are wired. Falls
// back to the legacy non-atomic path (separate Update + PublishReliable
// calls, errors logged) when atomicity isn't available. The shared
// helper keeps SignContract / MarkSignedExternally / HandleSignatureRefused
// honest about the new pattern without duplicating the tx scaffolding.
func (s *ContractService) saveContractWithEvents(ctx context.Context, c *contract.Contract, syncDealSigners bool, evts ...events.Event) error {
	return s.saveContractRevisionWithEvents(ctx, c, syncDealSigners, nil, evts...)
}

func (s *ContractService) saveContractRevisionWithEvents(ctx context.Context, c *contract.Contract, syncDealSigners bool, expected *time.Time, evts ...events.Event) error {
	if s.db != nil {
		if rb, ok := s.eventBus.(*events.RabbitMQEventBus); ok && rb != nil {
			return s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
				var err error
				if expected != nil {
					guarded, ok := s.contractRepo.(interface {
						UpdateIfUnchangedInTx(context.Context, *sqlx.Tx, *contract.Contract, time.Time) error
					})
					if !ok {
						return errors.New("contract repository does not support transactional revisions")
					}
					err = guarded.UpdateIfUnchangedInTx(ctx, tx, c, *expected)
				} else {
					err = s.contractRepo.UpdateInTx(ctx, tx, c)
				}
				if err != nil {
					return err
				}
				if syncDealSigners {
					if err := s.syncAllDealSignersFromContractWithTx(ctx, tx, c); err != nil {
						return err
					}
				}
				for _, evt := range evts {
					if _, err := rb.EnqueueOutboxInTx(ctx, tx, evt); err != nil {
						return err
					}
				}
				return nil
			})
		}
	}
	var err error
	if expected != nil {
		err = s.contractRepo.UpdateIfUnchanged(ctx, c, *expected)
	} else {
		err = s.contractRepo.Update(ctx, c)
	}
	if err != nil {
		return err
	}
	if syncDealSigners {
		if err := s.syncAllDealSignersFromContract(ctx, c); err != nil {
			return err
		}
	}
	for _, evt := range evts {
		if s.eventBus == nil {
			continue
		}
		if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
			log.Printf("WARN: failed to enqueue %s event for contract=%s: %v", evt.Name(), c.ID, err)
		}
	}
	return nil
}

// SyncDealSignersFromActiveContract looks up the contract by dealID and, if
// it exists, mirrors its signatory statuses onto the deal — same logic as
// the envelope_finished event handler, but callable on demand. Used by
// DealService.MoveDealStatus to auto-heal a stale Signers projection before
// blocking the "all required signers must sign" gate. Also a convenient
// recovery primitive: ContractService can be re-used to backfill old deals.
//
// When a signature client is wired (production path), the contract's
// Signatories are first refreshed from the envelope state in toca-signature
// — the canonical source. This handles the mid-flow drift where individual
// signer events were lost between toca-signature and toca-deal, leaving
// Contract.Signatories stale even though the envelope reflects every
// signature. Without this step, syncing Deal.Signers from a stale Contract
// just propagates the drift one layer down.
func (s *ContractService) SyncDealSignersFromActiveContract(ctx context.Context, tenantID, dealID string) error {
	if dealID == "" {
		return nil
	}
	contracts, err := s.contractRepo.GetByDealID(ctx, tenantID, dealID)
	if err != nil {
		return fmt.Errorf("lookup contracts for deal %s: %w", dealID, err)
	}
	for _, c := range contracts {
		if c == nil {
			continue
		}
		if err := s.refreshContractFromEnvelope(ctx, tenantID, c); err != nil {
			log.Printf("SyncDealSignersFromActiveContract: refresh from envelope failed for contract=%s deal=%s tenant=%s: %v (continuing with cached contract)",
				c.ID, dealID, tenantID, err)
		}
		if err := s.syncAllDealSignersFromContract(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

// refreshContractFromEnvelope re-reads the envelope state from toca-signature
// and applies any per-signer drift to Contract.Signatories before the
// downstream deal-side sync runs. The envelope is the canonical source of
// truth (it's where Clicksign webhooks land directly), so reading it here
// closes the gap when per-signer events between toca-signature and toca-deal
// were lost or never delivered.
//
// No-op when the signature client isn't wired (tests/in-memory setups).
// Persists the contract via Update so subsequent syncs see the fresh state.
// Errors are returned so the caller can log them, but they don't abort the
// sync (the cached contract is still usable).
//
// Logs every step (lookup, per-signer match, persist) so a Reconciliar
// click that doesn't move the badge can be forensic-traced via the
// toca-deal log without instrumentation.
func (s *ContractService) refreshContractFromEnvelope(ctx context.Context, tenantID string, c *contract.Contract) error {
	if s == nil || c == nil {
		return nil
	}
	// Terminal contract status: every signatory is already in their final
	// state locally, so re-reading the envelope only adds a 15s synchronous
	// hop to toca-signature (and a transitive Clicksign round-trip on its
	// reconcile path) without any drift to apply. Skipping here is what
	// keeps the deal "Concluído" transition snappy when the contract has
	// already been signed end-to-end.
	if c.Status == contract.StatusSigned {
		log.Printf("refreshContractFromEnvelope: contract=%s tenant=%s already SIGNED — skipping envelope round-trip", c.ID, tenantID)
		return nil
	}
	if s.signature == nil {
		log.Printf("refreshContractFromEnvelope: signature client not wired (SIGNATURE_SERVICE_URL unset?) — contract=%s tenant=%s — skipping envelope refresh, sync will use cached Contract.Signatories",
			c.ID, tenantID)
		return nil
	}
	env, err := s.signature.GetByContract(ctx, tenantID, c.ID)
	if err != nil {
		return fmt.Errorf("envelope lookup contract=%s tenant=%s: %w", c.ID, tenantID, err)
	}
	if env == nil {
		log.Printf("refreshContractFromEnvelope: envelope not found for contract=%s tenant=%s — no recovery possible from envelope side", c.ID, tenantID)
		return nil
	}
	if len(env.Signers) == 0 {
		log.Printf("refreshContractFromEnvelope: envelope %s for contract=%s has zero signers — possible toca-signature bug or unfinished envelope setup", env.ID, c.ID)
		return nil
	}

	log.Printf("refreshContractFromEnvelope: contract=%s envelope=%s status=%s signers=%d — comparing against %d local signatories",
		c.ID, env.ID, env.Status, len(env.Signers), len(c.Signatories))

	changed := false
	for _, envSig := range env.Signers {
		mapped := mapEnvelopeStatusToContract(envSig.Status)
		if mapped == "" {
			log.Printf("refreshContractFromEnvelope: envelope signer %s (%s) has unknown status=%q — skipping", envSig.SignerID, envSig.Email, envSig.Status)
			continue
		}
		matched := false
		for i := range c.Signatories {
			byID := c.Signatories[i].ID == envSig.SignerID
			byEmail := !byID && envSig.Email != "" && strings.EqualFold(c.Signatories[i].Email, envSig.Email)
			if !byID && !byEmail {
				continue
			}
			matched = true
			if c.Signatories[i].Status == mapped {
				log.Printf("refreshContractFromEnvelope: signer %s already %s — no-op", envSig.SignerID, mapped)
				break
			}
			previous := c.Signatories[i].Status
			c.Signatories[i].Status = mapped
			if mapped == contract.SignatoryStatusSigned && c.Signatories[i].SignedAt == nil {
				if ts, perr := time.Parse(time.RFC3339, envSig.SignedAt); perr == nil {
					c.Signatories[i].SignedAt = &ts
				} else {
					now := time.Now()
					c.Signatories[i].SignedAt = &now
				}
			}
			changed = true
			matchKind := "id"
			if byEmail {
				matchKind = "email"
			}
			log.Printf("refreshContractFromEnvelope: applied envelope signer %s (%s) → contract signatory match-by-%s, status %s→%s",
				envSig.SignerID, envSig.Email, matchKind, previous, mapped)
			break
		}
		if !matched {
			log.Printf("refreshContractFromEnvelope: envelope signer %s (%s) did NOT match any contract.Signatories — ID drift suspected", envSig.SignerID, envSig.Email)
		}
	}

	// Promote contract status if envelope finished but local lags. Don't
	// regress from SIGNED — terminal stays terminal.
	if envelopeStatusFinished(env.Status) && c.Status != contract.StatusSigned {
		log.Printf("refreshContractFromEnvelope: envelope %s is %s, contract was %s — promoting contract to SIGNED",
			env.ID, env.Status, c.Status)
		c.Status = contract.StatusSigned
		changed = true
	}

	if !changed {
		log.Printf("refreshContractFromEnvelope: nothing to apply for contract=%s envelope=%s (already aligned)", c.ID, env.ID)
		return nil
	}
	c.UpdatedAt = time.Now()
	if err := s.contractRepo.Update(ctx, c); err != nil {
		return fmt.Errorf("persist contract refresh contract=%s: %w", c.ID, err)
	}
	log.Printf("refreshContractFromEnvelope: persisted refreshed contract=%s", c.ID)
	return nil
}

// mapEnvelopeStatusToContract translates the toca-signature envelope
// signer status (uppercase strings from the JSON DTO) to the
// contract.SignatoryStatus enum. Returns "" for unknown values so the
// caller can skip them safely.
func mapEnvelopeStatusToContract(status string) contract.SignatoryStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "SIGNED":
		return contract.SignatoryStatusSigned
	case "REFUSED":
		return contract.SignatoryStatusRefused
	case "VIEWED":
		return contract.SignatoryStatusViewed
	case "PENDING":
		return contract.SignatoryStatusPending
	}
	return ""
}

func envelopeStatusFinished(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FINISHED", "CLOSED":
		return true
	}
	return false
}

// mapContractStatusToDeal bridges the two enums (kept distinct because
// contract.SignatoryStatus tracks the granular VIEWED state the deal
// doesn't model). Returns "" when there's nothing to mirror.
func mapContractStatusToDeal(s contract.SignatoryStatus) deal.SignerStatus {
	switch s {
	case contract.SignatoryStatusSigned:
		return deal.SignerStatusSigned
	case contract.SignatoryStatusRefused:
		return deal.SignerStatusRejected
	case contract.SignatoryStatusPending, contract.SignatoryStatusViewed:
		return deal.SignerStatusPending
	}
	return ""
}

type SignatureEnvelopeCreator interface {
	CreateForContract(ctx context.Context, tenantID, contractID string, req clients.CreateSignatureEnvelopeRequest) (*clients.SignatureEnvelopeDTO, error)
	GetByContract(ctx context.Context, tenantID, contractID string) (*clients.SignatureEnvelopeDTO, error)
	// RefreshSignedDocumentURL re-fetches the signed PDF URL from
	// Clicksign for contracts whose local `SignedDocumentURL` is empty.
	// Returns "" when the file is genuinely not ready yet (caller
	// should skip and retry later); error only for transport failures.
	// Optional on the interface so legacy test fakes that don't need
	// the recovery path still compile — callers must nil-check.
	RefreshSignedDocumentURL(ctx context.Context, tenantID, contractID string) (string, error)
}

func NewContractService(contractRepo contract.Repository, dealRepo deal.Repository, eventBus events.EventBus, db *database.PostgresDB, signature ...SignatureEnvelopeCreator) *ContractService {
	var signatureClient SignatureEnvelopeCreator
	if len(signature) > 0 {
		signatureClient = signature[0]
	}
	return &ContractService{
		contractRepo:         contractRepo,
		dealRepo:             dealRepo,
		eventBus:             eventBus,
		db:                   db,
		signature:            signatureClient,
		financialPlanChecker: clients.NewFinancialClient(),
	}
}

func (s *ContractService) CreateContract(ctx context.Context, tenantID string, req *contract.CreateContractRequest) (*contract.Contract, error) {
	// 1. Verify Deal exists
	d, err := s.dealRepo.GetByID(ctx, tenantID, req.DealID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, errors.New("deal not found")
	}

	// 1.5 Validate signatories
	if err := contract.ValidateSignatories(req.Type, req.Signatories); err != nil {
		return nil, err
	}

	// 1.6 T-01: guard server-side do plano financeiro. Um contrato de VENDA
	// só é gerado com draft de recebimento ativo e total igual ao valor do
	// negócio — o contract.signed nunca fabrica cobrança à vista e sem o
	// draft a venda financiada assinaria sem nenhuma cobrança programada.
	if req.Type == contract.TypeVenda {
		if err := s.requireSalePlanReady(ctx, tenantID, req.DealID, d.Value); err != nil {
			return nil, err
		}
	}

	// 2. Create Contract
	c, err := contract.New(
		req.DealID,
		req.Content,
		req.Signatories,
		req.StartDate,
		req.EndDate,
		tenantID,
		req.Type,
	)
	if err != nil {
		return nil, err
	}

	// Snapshot CRM fields from the Deal (authoritative) onto the Contract so
	// the listing UI can render Cliente/Imóvel/Valor without joining back
	// every time. The previous flow accepted these from the request and a
	// frontend bug sent client=customer_id (UUID) — deriving from the Deal
	// is both correct and resilient to caller mistakes.
	c.PropertyID = d.PropertyID
	c.CustomerID = d.CustomerID
	c.Value = d.Value
	c.CommissionPct = d.Commission
	customerRole := deal.PartyRoleTenant
	if req.Type == contract.TypeVenda {
		customerRole = deal.PartyRoleBuyer
	}
	if p := d.PrimaryByRole(customerRole); p != nil {
		c.Client = p.Name
	}
	// PropertyTitle has no field on Deal — accept the snapshot from the
	// caller. property.updated keeps it fresh later via NewPropertyUpdatedHandler.
	c.PropertyTitle = req.PropertyTitle
	c.LateFeePct = req.LateFeePct
	c.MonthlyInterestPct = req.MonthlyInterestPct
	c.AdjustIndex = req.AdjustIndex
	c.AutoAdjust = req.AutoAdjust
	c.NextAdjustment = req.NextAdjustment

	if err := s.contractRepo.Create(ctx, c); err != nil {
		return nil, err
	}

	// Surface the new contract in the Central de Documentos right away
	// (status=rascunho). Without this the page only sees Documents that
	// were manually uploaded — and contracts generated automatically from
	// templates never appear until someone attaches a file.
	if s.documents != nil {
		actor := actorFromContext(ctx)
		if err := s.documents.EnsureForContract(ctx, tenantID, c.ID, c.DealID, contractDocumentName(c, d), actor); err != nil {
			log.Printf("document placeholder creation failed for contract %s: %v", c.ID, err)
		}
		// If the operator already supplied the rendered DOCX, persist it
		// so the document row has a real file_size/file_url instead of a
		// zero-byte placeholder. Best-effort: a storage hiccup logs but
		// doesn't abort contract creation.
		if bytes, err := decodeContractContent(c.Content); err == nil && len(bytes) > 0 {
			if err := s.documents.UploadDocxForContract(ctx, tenantID, c.ID, bytes, actor); err != nil {
				log.Printf("docx auto-upload failed for contract %s: %v", c.ID, err)
			}
		}
	}

	return c, nil
}

// decodeContractContent normalizes the wire format of contract.Content into
// raw bytes. Operators send the rendered DOCX as base64 (sometimes prefixed
// with a data: URI). Returns empty slice + nil when there's nothing to
// decode, so callers can guard with len() == 0.
func decodeContractContent(content string) ([]byte, error) {
	if content == "" {
		return nil, nil
	}
	// Strip data URI prefix if present (e.g. "data:application/...;base64,...").
	if idx := strings.Index(content, "base64,"); idx >= 0 {
		content = content[idx+len("base64,"):]
	}
	// Tolerate either standard or URL-safe base64; raw input passes through
	// as a fallback when callers send the bytes already decoded as text.
	if decoded, err := base64.StdEncoding.DecodeString(content); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(content); err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("contract content is not valid base64")
}

// actorFromContext resolves a human-readable identifier for the caller, used
// to stamp Document.UploadedBy. Prefers the display name, then email, then
// account id; falls back to "system" when the context is bare (e.g. an
// internal background job that didn't propagate user identity).
func actorFromContext(ctx context.Context) string {
	if name, err := pkgctx.GetUserName(ctx); err == nil && name != "" {
		return name
	}
	if email, err := pkgctx.GetEmail(ctx); err == nil && email != "" {
		return email
	}
	if id, err := pkgctx.GetAccountID(ctx); err == nil && id != "" {
		return id
	}
	return "system"
}

// contractDocumentName builds a human-readable label for the placeholder
// Document row. Prefers the deal's display id (operator-visible) and falls
// back to the contract type or id so the Central de Documentos always
// shows something recognizable.
func contractDocumentName(c *contract.Contract, d *deal.Deal) string {
	if c == nil {
		return "Contrato"
	}
	if d != nil && d.DisplayID != "" {
		return "Contrato " + d.DisplayID
	}
	if c.Type != "" {
		return "Contrato " + string(c.Type)
	}
	return "Contrato " + c.ID
}

// GetContract devolve o contrato do tenant.
//
// P1/A25: o contrato é lido escopado pelo negócio dele — um corretor não lê o
// contrato de outro corretor (o negócio do contrato é resolvido com o escopo do
// chamador). Fora de escopo devolve o MESMO "contract not found" do caso
// inexistente, para não confirmar a existência do registro.
func (s *ContractService) GetContract(ctx context.Context, tenantID, id, brokerID string) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}
	if strings.TrimSpace(brokerID) != "" && strings.TrimSpace(c.DealID) != "" {
		if err := s.dealInBrokerScope(ctx, tenantID, c.DealID, brokerID); err != nil {
			return nil, errors.New("contract not found")
		}
	}
	return c, nil
}

// ContractInBrokerScope confirma que o contrato existe no tenant e que o negócio
// dele está no escopo do chamador (P1/A25). Serve aos handlers de outro serviço
// (documentos do contrato) que não têm acesso ao repositório de negócios.
func (s *ContractService) ContractInBrokerScope(ctx context.Context, tenantID, contractID, brokerID string) error {
	if strings.TrimSpace(brokerID) == "" || strings.TrimSpace(contractID) == "" {
		return nil
	}
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil || c == nil {
		return ErrDealNotInScope
	}
	if strings.TrimSpace(c.DealID) == "" {
		return nil
	}
	return s.dealInBrokerScope(ctx, tenantID, c.DealID, brokerID)
}

// GetSignedDocumentURL returns the external Clicksign URL stored on the
// contract when MarkSignedExternally processed the envelope_finished event.
// Used by DocumentService.GetDownloadURL as a recovery path when the local
// Supabase copy is missing — empty string when no signed URL is available.
// Satisfies the ContractSignedURLLookup interface in document_service.go.
func (s *ContractService) GetSignedDocumentURL(ctx context.Context, tenantID, contractID string) (string, error) {
	if tenantID == "" || contractID == "" {
		return "", nil
	}
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return "", err
	}
	if c == nil {
		return "", nil
	}
	return c.SignedDocumentURL, nil
}

// RefreshSignedDocumentURLFromRemote asks toca-signature to re-query
// Clicksign for the freshest signed-PDF URL on a contract whose local
// `SignedDocumentURL` is empty (Phase 3 reconcile gap recovery). On
// success the contract row is updated in place so subsequent
// `GetSignedDocumentURL` calls hit the cache. Returns "" when the file
// is genuinely not ready (caller skips and retries later) or when no
// signature client is wired (legacy test setups).
//
// Satisfies the ContractSignedURLLookup interface used by
// DocumentService.BackfillUnstoredSignedPDFs.
func (s *ContractService) RefreshSignedDocumentURLFromRemote(ctx context.Context, tenantID, contractID string) (string, error) {
	if tenantID == "" || contractID == "" {
		return "", nil
	}
	if s.signature == nil {
		return "", nil
	}
	url, err := s.signature.RefreshSignedDocumentURL(ctx, tenantID, contractID)
	if err != nil || url == "" {
		return url, err
	}
	// Cache the freshly-fetched URL on the contract so future
	// GetSignedDocumentURL calls (download UI, ContractTab fallback)
	// don't re-hit toca-signature. Best-effort: a write failure
	// shouldn't prevent the backfill from using the URL we already
	// have in hand.
	if c, err := s.contractRepo.GetByID(ctx, tenantID, contractID); err == nil && c != nil && c.SignedDocumentURL != url {
		c.SignedDocumentURL = url
		c.UpdatedAt = time.Now()
		_ = s.contractRepo.Update(ctx, c)
	}
	return url, nil
}

// ListContractIDsByDeal returns the IDs of every contract attached to a
// given deal. Used by DocumentService.ListByDealIncludingContracts so
// the Esteira Documentation tab can surface signed PDFs alongside
// checklist items without a second round-trip. Empty slice (never nil)
// for deals with no contracts. Satisfies ContractSignedURLLookup.
func (s *ContractService) ListContractIDsByDeal(ctx context.Context, tenantID, dealID string) ([]string, error) {
	if tenantID == "" || dealID == "" {
		return []string{}, nil
	}
	contracts, err := s.contractRepo.GetByDealID(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(contracts))
	for _, c := range contracts {
		if c != nil && c.ID != "" {
			ids = append(ids, c.ID)
		}
	}
	return ids, nil
}

// SignerIdentity é a identidade de quem está assinando, extraída do JWT
// verificado (nunca do corpo da requisição).
type SignerIdentity struct {
	AccountID string
	Email     string
}

// signerOwnsSignatory decide se a identidade do chamador corresponde ao
// signatário-alvo do contrato.
//
// SECURITY (achado C12 da revisão de permissões de 2026-09-19): antes,
// `POST /contracts/:id/sign` assinava o signatário cujo `signatory_id` viesse no
// corpo, sem confrontar o chamador — qualquer usuário com `contracts:write`
// assinava em nome do proprietário, do comprador ou do banco, e o evento
// `contract.signed` disparava efeitos financeiros. Não repudiação era
// inexistente. O fluxo legítimo (ver `toka/e2e/contract-financial.spec.ts`)
// cria o signatário com `id = accountId` do próprio usuário e `email` dele,
// então aceitamos as duas provas — mas exigimos uma delas.
func signerOwnsSignatory(sig contract.Signatory, signer SignerIdentity) bool {
	if strings.TrimSpace(signer.AccountID) != "" && sig.ID == signer.AccountID {
		return true
	}
	sigEmail := strings.TrimSpace(strings.ToLower(sig.Email))
	callerEmail := strings.TrimSpace(strings.ToLower(signer.Email))
	return sigEmail != "" && callerEmail != "" && sigEmail == callerEmail
}

func (s *ContractService) SignContract(ctx context.Context, tenantID, contractID, signatoryID, signature string, signer SignerIdentity) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}

	// A assinatura interna pode ser reenviada pelo navegador depois de timeout
	// ou retry. Quando este mesmo signatário já fechou um contrato SIGNED, não
	// atualize o agregado nem publique outro contract.signed: os consumidores
	// financeiros usam o ID do evento para idempotência, portanto um novo evento
	// representaria incorretamente um novo fato econômico. Ainda sincronizamos a
	// projeção do deal para que o retry possa reparar uma gravação anterior parcial.
	if c.Status == contract.StatusSigned {
		for _, sig := range c.Signatories {
			if sig.ID == signatoryID && sig.SignedAt != nil {
				// SECURITY (C12): o replay idempotente também exige identidade.
				if !signerOwnsSignatory(sig, signer) {
					return nil, errors.New("unauthorized")
				}
				if err := s.syncAllDealSignersFromContract(ctx, c); err != nil {
					return nil, err
				}
				return c, nil
			}
		}
	}

	// Snapshot do estado pré-assinatura para o audit (#471). Captura
	// status do contrato + quantos signatários já tinham assinado, para
	// que o /audit-trail possa diferenciar "primeira assinatura" de
	// "assinatura que fechou o contrato".
	beforeStatus := c.Status
	beforeSignedCount := 0
	for _, sig := range c.Signatories {
		if sig.SignedAt != nil {
			beforeSignedCount++
		}
	}

	// Find Signatory
	found := false
	allSigned := true
	now := time.Now()

	for i, sig := range c.Signatories {
		if sig.ID == signatoryID {
			// SECURITY (C12): só o próprio signatário (ou o titular do e-mail
			// dele) pode assinar. Sem esta checagem, `signature` e
			// `signatory_id` vinham ambos do corpo.
			if !signerOwnsSignatory(sig, signer) {
				return nil, errors.New("unauthorized")
			}
			c.Signatories[i].Signature = signature
			c.Signatories[i].SignedAt = &now
			c.Signatories[i].Status = contract.SignatoryStatusSigned
			found = true
		}
		if c.Signatories[i].SignedAt == nil {
			allSigned = false
		}
	}

	if !found {
		return nil, errors.New("signatory not found in contract")
	}

	// Update Status if all signed
	if allSigned {
		c.Status = contract.StatusSigned
	} else {
		c.Status = contract.StatusPending
	}

	// When allSigned, build the event up-front so the atomic helper can stage
	// the contract Update + deal-signers mirror + outbox INSERT in one tx.
	var evts []events.Event
	if allSigned {
		// Mesmo invariante do caminho MarkSignedExternally: sem o negócio o
		// evento sai vazio e o financeiro/ops rejeitam — nunca emitir.
		d, err := s.dealRepo.GetByID(ctx, tenantID, c.DealID)
		if err != nil {
			return nil, fmt.Errorf("load deal %s for contract %s: %w", c.DealID, c.ID, err)
		}
		evt, err := buildContractSignedEventFor(c, d, now)
		if err != nil {
			return nil, err
		}
		evts = append(evts, evt)
	}

	if err := s.saveContractWithEvents(ctx, c, true, evts...); err != nil {
		return nil, err
	}

	// Contract signatures are legally significant — every signing is
	// auditable for the full retention window. before/after omit the
	// signature bytes (those live in the Signatory row itself) so the
	// audit blob stays small. #471: agora capturamos status anterior
	// + signed_count_before para reconstituir cronologia.
	signedCountAfter := 0
	for _, sig := range c.Signatories {
		if sig.SignedAt != nil {
			signedCountAfter++
		}
	}
	audit.Record(ctx, audit.Entry{
		Action:     "contract.sign",
		EntityType: "Contract",
		EntityID:   c.ID,
		Before: map[string]any{
			"status":              string(beforeStatus),
			"signed_count_before": beforeSignedCount,
		},
		After: map[string]any{
			"status":             string(c.Status),
			"deal_id":            c.DealID,
			"signatory_id":       signatoryID,
			"all_signed":         allSigned,
			"signed_count_after": signedCountAfter,
		},
		Metadata: map[string]any{"path": "SignContract"},
	})

	return c, nil
}

// ListContractsByDeal lista os contratos de um negócio.
//
// P1/A25: era só tenant-scoped — com `contracts:read` (que o `corretor` tem) um
// corretor lia contratos de clientes de outro corretor. O negócio passa a ser
// resolvido com o escopo do chamador antes da listagem.
func (s *ContractService) ListContractsByDeal(ctx context.Context, tenantID, dealID, brokerID string) ([]*contract.Contract, error) {
	if err := s.dealInBrokerScope(ctx, tenantID, dealID, brokerID); err != nil {
		return nil, err
	}
	return s.contractRepo.GetByDealID(ctx, tenantID, dealID)
}

// dealInBrokerScope confirma que o negócio existe e está no escopo do chamador.
// brokerID vazio (admin/owner/manager, ou chamador interno) não recorta.
func (s *ContractService) dealInBrokerScope(ctx context.Context, tenantID, dealID, brokerID string) error {
	if s.dealRepo == nil || strings.TrimSpace(dealID) == "" {
		return nil
	}
	var (
		d   *deal.Deal
		err error
	)
	if strings.TrimSpace(brokerID) == "" {
		d, err = s.dealRepo.GetByID(ctx, tenantID, dealID)
	} else {
		d, err = s.dealRepo.GetByIDScoped(ctx, tenantID, dealID, brokerID)
	}
	if err != nil || d == nil {
		return ErrDealNotInScope
	}
	return nil
}

func (s *ContractService) ListContracts(ctx context.Context, tenantID string, status *contract.Status) ([]*contract.Contract, error) {
	return s.contractRepo.List(ctx, tenantID, status)
}

func (s *ContractService) UpdateContract(ctx context.Context, tenantID, id string, req *contract.CreateContractRequest) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}

	// Block edits while a signature envelope is in flight. Once the contract
	// has been refused (envelope_refused webhook clears the lock) or canceled,
	// it becomes editable again. SIGNED contracts stay locked permanently.
	if c.SignatureLockedAt != nil && c.Status != contract.StatusDraft {
		return nil, ErrContractLockedForSignature
	}

	if req.Status != "" {
		c.Status = req.Status
	}
	if req.Type != "" {
		c.Type = req.Type
	}
	if req.PropertyID != "" {
		c.PropertyID = req.PropertyID
	}
	if req.PropertyTitle != "" {
		c.PropertyTitle = req.PropertyTitle
	}
	if req.Client != "" {
		c.Client = req.Client
	}
	if req.Value > 0 {
		c.Value = req.Value
	}
	if req.CommissionPct > 0 {
		c.CommissionPct = req.CommissionPct
	}
	// Per-contract rates: same > 0 convention as CommissionPct above. To
	// "disable" the rule and fall back to the global default, leave the field
	// out of the payload (or set to 0 server-side via a separate clear flow
	// once that's needed by the UI).
	if req.LateFeePct > 0 {
		c.LateFeePct = req.LateFeePct
	}
	if req.MonthlyInterestPct > 0 {
		c.MonthlyInterestPct = req.MonthlyInterestPct
	}
	if req.AdjustIndex != "" {
		c.AdjustIndex = req.AdjustIndex
	}
	c.AutoAdjust = req.AutoAdjust
	if req.NextAdjustment != "" {
		c.NextAdjustment = req.NextAdjustment
	}
	if req.Content != "" {
		c.Content = req.Content
	}
	if req.Signatories != nil {
		contractType := c.Type
		if req.Type != "" {
			contractType = req.Type
		}
		if err := contract.ValidateSignatories(contractType, req.Signatories); err != nil {
			return nil, err
		}
		c.Signatories = req.Signatories
	}
	if !req.StartDate.IsZero() {
		c.StartDate = req.StartDate
	}
	if req.EndDate != nil {
		c.EndDate = req.EndDate
	}

	if err := s.contractRepo.Update(ctx, c); err != nil {
		return nil, err
	}

	// Mirror the latest DOCX into the linked Document whenever the operator
	// sends new content. Skipped automatically downstream once the contract
	// is locked for signature — UploadDocxForContract refuses to overwrite
	// signed/pending-signature documents.
	if s.documents != nil && req.Content != "" {
		if bytes, err := decodeContractContent(c.Content); err == nil && len(bytes) > 0 {
			if err := s.documents.UploadDocxForContract(ctx, tenantID, c.ID, bytes, actorFromContext(ctx)); err != nil {
				log.Printf("docx auto-upload failed for contract %s: %v", c.ID, err)
			}
		}
	}
	return c, nil
}

func (s *ContractService) DeleteContract(ctx context.Context, tenantID, id string) error {
	return s.contractRepo.Delete(ctx, id, tenantID)
}

// SendForSignatureRequest is the operator-supplied configuration for kicking
// off an external signature flow. SignerAuth maps Contract.Signatory.ID to
// the list of auth methods to enforce on Clicksign (email, whatsapp, sms,
// selfie, etc.). SignerGroups maps the same IDs to a 1-based Clicksign group
// for sequential signing. AttachmentDocIDs/InlineAttachments carry extra
// documents to bundle into the same envelope.
type SendForSignatureRequest struct {
	SignerAuth        map[string][]string    `json:"signer_auth"`
	SignerGroups      map[string]int         `json:"signer_groups,omitempty"`
	AttachmentDocIDs  []string               `json:"attachment_document_ids,omitempty"`
	InlineAttachments []SendInlineAttachment `json:"inline_attachments,omitempty"`
	Deadline          *time.Time             `json:"deadline,omitempty"`
	Locale            string                 `json:"locale,omitempty"`
}

// SendInlineAttachment is the service-layer shape for an inline PDF upload
// — same as the HTTP DTO but lives in this package so handlers depend on
// the service, not vice-versa.
type SendInlineAttachment struct {
	Filename      string
	ContentBase64 string
}

// SendForSignature publishes a SignatureRequestedEvent on the bus — the
// signature service picks it up, creates the envelope on Clicksign, and
// reports back via the signature.signer_signed / signature.envelope_finished
// events handled by MarkSignerSignedExternally / MarkSignedExternally.
//
// On success the contract is marked PENDING_SIGNATURE and locked for edits.
// The operator can cancel via the signature service to unlock + retry with
// different signers/auth methods.
func (s *ContractService) SendForSignature(ctx context.Context, tenantID, contractID string, req SendForSignatureRequest) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}
	if len(c.Signatories) == 0 {
		return nil, errors.New("contract has no signatories")
	}
	// If the contract has already been sent and the envelope is in flight,
	// don't republish. Operator must cancel first via signature service.
	if c.SignatureLockedAt != nil || c.Status != contract.StatusDraft {
		return nil, ErrContractLockedForSignature
	}

	// O contrato nasce com uma cópia dos signatários do deal. Uma correção de
	// e-mail/nome feita na esteira atualiza Deal.Signers, mas não essa cópia;
	// sem sincronizar aqui, o Clicksign recebe o endereço antigo mesmo após o
	// operador corrigir o lead/parte. A fonte de verdade é o deal.
	var dealRec *deal.Deal
	if s.dealRepo != nil {
		dealRec, err = s.dealRepo.GetByID(ctx, tenantID, c.DealID)
		if err != nil {
			return nil, err
		}
	}
	if dealRec != nil {
		for i := range c.Signatories {
			for _, ds := range dealRec.Signers {
				if ds.ID != c.Signatories[i].ID {
					continue
				}
				if name := strings.TrimSpace(ds.Name); name != "" {
					c.Signatories[i].Name = name
				}
				if email := strings.TrimSpace(ds.Email); email != "" {
					c.Signatories[i].Email = email
				}
				break
			}
		}
	}

	// T-01: a ÚLTIMA linha antes do efeito financeiro é o envio. Um contrato
	// de VENDA só sai para assinatura com o plano de recebimento ativo e
	// compatível com o valor do negócio — sem isso a assinatura não
	// materializaria cobrança nenhuma (nunca um fallback à vista fabricado).
	if c.Type == contract.TypeVenda {
		if s.dealRepo == nil {
			return nil, errors.New("repositório de negócios indisponível")
		}
		if dealRec == nil {
			return nil, fmt.Errorf("%w: negócio %s não encontrado para validar o plano de recebimento", ErrSalePlanNotReady, c.DealID)
		}
		if err := s.requireSalePlanReady(ctx, tenantID, c.DealID, dealRec.Value); err != nil {
			return nil, err
		}
	}

	// O envelope só existe se o evento `signature.requested` for publicado e
	// consumido. Sem barramento, marcar o contrato como enviado deixaria o
	// operador com um contrato travado em PENDING_SIGNATURE e nenhum envelope no
	// provedor (incidente de 2026-09-30: documento "enviado" que nunca chegou ao
	// Clicksign). Falha ALTA e o contrato continua rascunho.
	if s.eventBus == nil {
		return nil, errors.New("barramento de eventos indisponível: não é possível enviar para assinatura agora")
	}

	expectedUpdatedAt := c.UpdatedAt
	copyContract := *c
	copyContract.Signatories = append([]contract.Signatory(nil), c.Signatories...)
	c = &copyContract
	now := time.Now()
	c.Status = contract.StatusPending
	c.SignatureProvider = "clicksign"
	c.ExternalEnvelopeID = "" // The next send must not inherit the previous attempt.
	c.SentForSignatureAt = &now
	c.SignatureLockedAt = &now
	c.UpdatedAt = now

	// Persist auth methods + signing-order group on each signatory so they
	// survive across retries. Group from req.SignerGroups overrides the
	// contract-stored default; missing entries keep whatever's there.
	for i := range c.Signatories {
		if methods, ok := req.SignerAuth[c.Signatories[i].ID]; ok {
			c.Signatories[i].AuthMethods = methods
		}
		if group, ok := req.SignerGroups[c.Signatories[i].ID]; ok {
			c.Signatories[i].Group = group
		}
	}

	if err := s.contractRepo.UpdateIfUnchanged(ctx, c, expectedUpdatedAt); err != nil {
		return nil, err
	}

	// A criação do envelope NÃO acontece nesta requisição.
	//
	// Renderizar o contrato em PDF e criar o envelope no provedor leva ~27s
	// (medido: Gotenberg/Chromium 27,2s para o PDF), muito acima do tempo de um
	// request — o cliente interno cortava em 15s e o gateway devolvia 504, com o
	// contrato ainda revertido para rascunho. A tentativa síncrona foi removida:
	// o evento publicado abaixo é consumido pelo toca-signature, que cria o
	// envelope com o tempo que precisar. A tela de contrato já acompanha o
	// envelope por polling enquanto está em PENDING_SIGNATURE.
	//
	// Requisição rápida, trabalho pesado no worker.
	if s.eventBus != nil {
		evt := buildSignatureRequestedEvent(c, req, now)
		// O evento é o caminho que GARANTE o envelope (o worker o consome), então
		// a publicação não pode depender do contexto da requisição: num timeout
		// de gateway o contexto chega cancelado, o evento não sairia e o contrato
		// ficaria marcado como enviado sem ninguém criar o envelope.
		publishCtx, cancelPublish := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancelPublish()
		if err := s.eventBus.Publish(publishCtx, evt); err != nil {
			// O evento é o ÚNICO caminho para o envelope existir. Devolver
			// sucesso aqui deixaria o contrato travado em PENDING_SIGNATURE sem
			// ninguém para criar nada — o estado sem saída que gerou os 409.
			// Melhor falhar: o operador tenta de novo.
			sentUpdatedAt := c.UpdatedAt
			c.Status = contract.StatusDraft
			c.SignatureLockedAt = nil
			c.SentForSignatureAt = nil
			c.UpdatedAt = time.Now()
			recoveryCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			if updateErr := s.contractRepo.UpdateIfUnchanged(recoveryCtx, c, sentUpdatedAt); updateErr != nil {
				log.Printf("signature.requested publish failed; contract recovery update failed: %v", updateErr)
			}
			return c, err
		}
	}

	return c, nil
}

// MarkSignerSignedExternally is invoked by the signature.signer_signed
// subscriber when one signer completes their signature. It updates that
// signer's Status/SignedAt/Signature/EvidenceURL and promotes the contract
// to PARTIALLY_SIGNED if there are still pending signers. MarkSignedExternally
// handles the final FINISHED transition on auto_close.
func (s *ContractService) MarkSignerSignedExternally(ctx context.Context, tenantID, contractID, signerID string, signedAt time.Time, evidenceURL string, externalEnvelopeID string) error {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("contract not found")
	}
	matches, err := s.signatureAttemptMatches(ctx, c, externalEnvelopeID)
	if err != nil {
		return err
	}
	if !matches || (c.Status != contract.StatusPending && c.Status != contract.StatusPartiallySigned) {
		return nil
	}
	expected := c.UpdatedAt
	copy := *c
	copy.Signatories = append([]contract.Signatory(nil), c.Signatories...)
	c = &copy
	found := false
	needsSave := false
	for i := range c.Signatories {
		if c.Signatories[i].ID == signerID {
			if c.Signatories[i].SignedAt == nil {
				ts := signedAt
				c.Signatories[i].SignedAt = &ts
				needsSave = true
			}
			if c.Signatories[i].Status != contract.SignatoryStatusSigned {
				c.Signatories[i].Status = contract.SignatoryStatusSigned
				needsSave = true
			}
			if c.Signatories[i].Signature == "" {
				c.Signatories[i].Signature = "external:" + signerID
				needsSave = true
			}
			if evidenceURL != "" && c.Signatories[i].EvidenceURL == "" {
				c.Signatories[i].EvidenceURL = evidenceURL
				needsSave = true
			}
			found = true
			break
		}
	}
	if !found {
		// Signer might be a witness we don't track, or a stale event for
		// a contract whose signatories list was rewritten. Don't error
		// (would re-queue); ignore it without changing the revision.
		return nil
	}
	// Promote to PARTIALLY_SIGNED if not all signers have signed yet. Don't
	// regress from SIGNED/ACTIVE if a stale signer event arrives after the
	// envelope_finished has already landed.
	if c.Status == contract.StatusPending {
		allSigned := true
		for _, sig := range c.Signatories {
			if sig.SignedAt == nil {
				allSigned = false
				break
			}
		}
		if !allSigned {
			c.Status = contract.StatusPartiallySigned
			needsSave = true
		}
	}
	if c.ExternalEnvelopeID == "" {
		c.ExternalEnvelopeID = externalEnvelopeID
		needsSave = true
	}
	if needsSave {
		c.UpdatedAt = time.Now()
		if err := s.contractRepo.UpdateIfUnchanged(ctx, c, expected); err != nil {
			return err
		}
	}
	s.syncDocuments(ctx, c)
	// Resolve the signer's email from the contract signatory so the deal-side
	// fallback can match by email when the IDs drift.
	signerEmail := ""
	for _, sig := range c.Signatories {
		if sig.ID == signerID {
			signerEmail = sig.Email
			break
		}
	}
	if err := s.syncDealSignerStatusWithEmail(ctx, tenantID, c.DealID, signerID, signerEmail, deal.SignerStatusSigned); err != nil {
		// Returning the error lets the event bus redeliver so the deal
		// projection eventually catches up — contract is already updated
		// so the redelivery is a pure no-op on the contract side.
		return err
	}
	return nil
}

// PortalSignatureView is the projection the client portal renders for
// "Sua Assinatura" — minimal PII about other signers (name + role + status,
// no email/phone) plus the URL the current user needs to sign. Built here
// rather than on the wire because toca-signature is admin-only and the
// portal handler needs to filter for the calling user's email.
type PortalSignatureView struct {
	HasEnvelope    bool
	ContractID     string
	EnvelopeID     string
	EnvelopeStatus string
	Environment    string // "sandbox" or "production" — Clicksign widget needs it
	Deadline       string
	SignedDocURL   string
	MySigner       *PortalSignerView
	OtherSigners   []PortalSignerView
	SignedCount    int
	TotalCount     int
}

type PortalSignerView struct {
	SignerID string
	Name     string
	Role     string
	Status   string
	SignedAt string
	// RequestSignatureKey is the Clicksign per-signer URL token the widget
	// uses. Only populated for the calling user (MySigner) — never leaks
	// for other signers (would let one signer hijack another's flow).
	RequestSignatureKey string
}

// GetPortalSignatureView builds the per-user signature payload for the
// client portal. callerEmail is matched case-insensitively against the
// contract signatories so the user always finds themselves regardless of
// how their email is stored upstream.
func (s *ContractService) GetPortalSignatureView(ctx context.Context, tenantID, dealID, callerEmail string) (*PortalSignatureView, error) {
	cs, err := s.contractRepo.GetByDealID(ctx, tenantID, dealID)
	if err != nil {
		return nil, err
	}
	if len(cs) == 0 {
		return &PortalSignatureView{HasEnvelope: false}, nil
	}
	// Newest contract first — the repository orders by created_at desc.
	c := cs[0]
	view := &PortalSignatureView{
		ContractID: c.ID,
		TotalCount: len(c.Signatories),
		Deadline:   "",
	}

	// Serve the signed PDF from our own storage (MinIO). The raw Clicksign S3
	// URL is not loadable by the browser — its bucket sends no CORS header for
	// the app origin and the presigned link expires in ~5 min.
	// EnsureSignedPDFURL downloads + caches the PDF server-side on first
	// access; SignedDocURL stays empty until the contract is signed.
	if s.documents != nil && c.Status == contract.StatusSigned {
		if url, err := s.documents.EnsureSignedPDFURL(ctx, tenantID, c.ID); err == nil {
			view.SignedDocURL = url
		}
	}

	// Hydrate from the live envelope when we can — it has the granular
	// statuses (VIEWED, PARTIALLY_SIGNED). When toca-signature is offline
	// or no envelope exists yet, fall back to the contract's denormalized
	// signatory snapshot.
	var envSigners map[string]clients.SignatureEnvelopeSignerDTO
	if s.signature != nil {
		env, sigErr := s.signature.GetByContract(ctx, tenantID, c.ID)
		if sigErr == nil && env != nil {
			view.HasEnvelope = true
			view.EnvelopeID = env.ID
			view.EnvelopeStatus = env.Status
			view.Environment = env.Environment
			envSigners = make(map[string]clients.SignatureEnvelopeSignerDTO, len(env.Signers))
			for _, s := range env.Signers {
				envSigners[s.SignerID] = s
			}
		}
		// On error we deliberately ignore — the portal still gets a
		// useful response from the contract row.
	}

	lowerEmail := strings.ToLower(strings.TrimSpace(callerEmail))
	for _, sig := range c.Signatories {
		status := string(sig.Status)
		if status == "" {
			status = "PENDING"
		}
		signedAt := ""
		if sig.SignedAt != nil {
			signedAt = sig.SignedAt.Format(time.RFC3339)
		}
		if env, ok := envSigners[sig.ID]; ok {
			if env.Status != "" {
				status = env.Status
			}
			if env.SignedAt != "" {
				signedAt = env.SignedAt
			}
		}
		signer := PortalSignerView{
			SignerID: sig.ID,
			Name:     sig.Name,
			Role:     string(sig.Role),
			Status:   status,
			SignedAt: signedAt,
		}
		if status == string(contract.SignatoryStatusSigned) {
			view.SignedCount++
		}
		if strings.EqualFold(sig.Email, lowerEmail) {
			// Per-signer Clicksign widget key is only exposed for the
			// calling user — never for other signers.
			if env, ok := envSigners[sig.ID]; ok {
				signer.RequestSignatureKey = env.RequestSignatureKey
			}
			view.MySigner = &signer
		} else {
			view.OtherSigners = append(view.OtherSigners, signer)
		}
	}
	// Fallback envelope status when toca-signature is unreachable: derive
	// from contract.Status so the portal still shows something coherent.
	if view.EnvelopeStatus == "" {
		view.EnvelopeStatus = string(c.Status)
	}
	return view, nil
}

// MarkEnvelopeStartedExternally is invoked by the signature.envelope_started
// subscriber when the envelope transitions to RUNNING on Clicksign. Records
// the envelope for the pending send without reopening a draft or final state.
func (s *ContractService) MarkEnvelopeStartedExternally(ctx context.Context, tenantID, contractID, externalEnvelopeID string) error {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("contract not found")
	}
	// SendForSignature has already moved the contract to PENDING_SIGNATURE.
	// Record the envelope without reopening a refused draft or a final state.
	if c.Status != contract.StatusPending && c.Status != contract.StatusPartiallySigned {
		return nil
	}
	matches, err := s.signatureAttemptMatches(ctx, c, externalEnvelopeID)
	if err != nil || !matches {
		return err
	}
	if c.ExternalEnvelopeID == externalEnvelopeID {
		return nil
	}
	expectedUpdatedAt := c.UpdatedAt
	copyContract := *c
	c = &copyContract
	c.ExternalEnvelopeID = externalEnvelopeID
	c.UpdatedAt = time.Now()
	if err := s.contractRepo.UpdateIfUnchanged(ctx, c, expectedUpdatedAt); err != nil {
		return err
	}
	s.syncDocuments(ctx, c)
	return nil
}

// MarkSignerViewedExternally is invoked by the signature.signer_viewed
// subscriber. Sets the signer's Status to VIEWED and stamps ViewedAt — only
// if the signer hasn't signed yet (don't regress).
func (s *ContractService) MarkSignerViewedExternally(ctx context.Context, tenantID, contractID, signerID string, viewedAt time.Time) error {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("contract not found")
	}
	// Contract-level guard for defense in depth. If the contract is already
	// SIGNED, a late VIEWED webhook from Clicksign must not regress any
	// individual signer — even when the signer-level guard below would
	// already protect a properly synced signatory list. This catches the
	// case where Signatories[] drifted from the envelope (e.g. the signer's
	// signed-status update was lost) and an out-of-order VIEWED would
	// otherwise be allowed through.
	if c.Status == contract.StatusSigned {
		return nil
	}
	for i := range c.Signatories {
		if c.Signatories[i].ID != signerID {
			continue
		}
		if c.Signatories[i].Status == contract.SignatoryStatusSigned ||
			c.Signatories[i].Status == contract.SignatoryStatusRefused {
			return nil // don't regress
		}
		ts := viewedAt
		c.Signatories[i].Status = contract.SignatoryStatusViewed
		c.Signatories[i].ViewedAt = &ts
		c.UpdatedAt = time.Now()
		if err := s.contractRepo.Update(ctx, c); err != nil {
			return err
		}
		// Deal-side has no granular VIEWED state — keep it on PENDING but
		// invoke the sync anyway so other side effects (UpdatedAt bump,
		// participant lookup cache) stay consistent. Best-effort: a sync
		// failure here means the deal stays on PENDING, which is the
		// correct target state anyway, so swallow the error after logging.
		if err := s.syncDealSignerStatus(ctx, tenantID, c.DealID, signerID, deal.SignerStatusPending); err != nil {
			log.Printf("deal sync on VIEWED failed for %s/%s: %v", c.DealID, signerID, err)
		}
		return nil
	}
	return nil
}

// MarkSignedExternally is invoked by the signature.envelope_finished subscriber
// when ALL signers have signed. It flips the contract to SIGNED and re-emits
// the existing ContractSignedEvent — preserving the downstream pipeline
// (toca-financial creates the Rental aggregate from this same event).
func (s *ContractService) MarkSignedExternally(ctx context.Context, tenantID, contractID, externalEnvelopeID string, finishedAt time.Time, signedDocURL string) (*contract.Contract, error) {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("contract not found")
	}
	matches, err := s.signatureAttemptMatches(ctx, c, externalEnvelopeID)
	if err != nil {
		return nil, err
	}
	if !matches {
		return c, nil
	}
	if c.Status != contract.StatusPending && c.Status != contract.StatusPartiallySigned && c.Status != contract.StatusSigned {
		return c, nil
	}
	expected := c.UpdatedAt
	// Idempotent on the contract update + event republish: if already signed
	// for this envelope, skip both. We still re-run the deal sync though —
	// a prior delivery might have failed there and left dealDetails.signers
	// stale ("Pendente" while the contract is SIGNED). This makes redelivery
	// of envelope_finished a free recovery path for the projection.
	if c.Status == contract.StatusSigned {
		// Backfill do vínculo com o envelope: o contrato pode ter chegado a
		// SIGNED por outro caminho (ex.: operador marcou/reconciliou antes deste
		// evento) e ficaria sem `external_envelope_id` para sempre, porque o
		// caminho idempotente abaixo não reescreve o registro. Sem o vínculo,
		// toda correlação futura depende de um lookup HTTP e a regra "tentativa
		// anterior não destrava um novo envio" perde a referência local.
		if c.ExternalEnvelopeID == "" && externalEnvelopeID != "" {
			copy := *c
			copy.Signatories = append([]contract.Signatory(nil), c.Signatories...)
			copy.ExternalEnvelopeID = externalEnvelopeID
			copy.UpdatedAt = time.Now()
			if err := s.contractRepo.Update(ctx, &copy); err != nil {
				return nil, err
			}
			c = &copy
		}
		if err := s.syncAllDealSignersFromContract(ctx, c); err != nil {
			return nil, err
		}
		return c, nil
	}

	copy := *c
	copy.Signatories = append([]contract.Signatory(nil), c.Signatories...)
	c = &copy
	now := time.Now()
	for i := range c.Signatories {
		// envelope_finished is the terminal event — every signer either
		// already had their individual signer_signed processed OR was
		// signed implicitly via auto_close (Clicksign batches it when
		// the last signer triggers auto-close). Either way, the contract
		// is fully signed at this point, so reflect that on each row:
		// granular SIGNED status + SignedAt timestamp. Before this change
		// the snapshot stayed PENDING in the contract while the contract
		// status flipped to SIGNED — admin showed "Assinado" header with
		// signers still pending below.
		if c.Signatories[i].SignedAt == nil {
			ts := finishedAt
			c.Signatories[i].SignedAt = &ts
			if c.Signatories[i].Signature == "" {
				c.Signatories[i].Signature = "external:autoclose"
			}
		}
		if c.Signatories[i].Status != contract.SignatoryStatusRefused {
			c.Signatories[i].Status = contract.SignatoryStatusSigned
		}
	}
	c.Status = contract.StatusSigned
	c.ExternalEnvelopeID = externalEnvelopeID
	if signedDocURL != "" {
		c.SignedDocumentURL = signedDocURL
	}
	c.UpdatedAt = now

	// Atomic path: contract Update + deal-signers mirror + outbox INSERT
	// in one tx so toca-financial can never observe a SIGNED contract
	// without the corresponding event (or vice versa). The deal lookup
	// for the event payload is a pre-tx read — fine, it doesn't write.
	//
	// O lookup NÃO pode falhar em silêncio: sem o negócio o payload sai sem
	// type/value/parties e todo consumidor rejeita (financeiro: "invalid
	// contract value"; ops: "missing contract/tenant/property") — a locação,
	// a comissão de intermediação e o repasse com dedução nunca nascem.
	d, err := s.dealRepo.GetByID(ctx, tenantID, c.DealID)
	if err != nil {
		return nil, fmt.Errorf("load deal %s for contract %s: %w", c.DealID, c.ID, err)
	}
	evt, err := buildContractSignedEventFor(c, d, now)
	if err != nil {
		return nil, err
	}
	if err := s.saveContractRevisionWithEvents(ctx, c, true, &expected, evt); err != nil {
		// Terminal transition — without successful propagation, the
		// contract would go SIGNED but the deal's Signers[]/Parties[]
		// stay PENDING and the admin "Esteira" sidebar gets permanently
		// stale until someone clicks Reconciliar. Returning lets the
		// bus redeliver.
		return nil, err
	}
	// Best-effort document mirror — runs outside the tx because the
	// Document side is a derived projection that can be rebuilt later
	// from the canonical contract.
	s.syncDocuments(ctx, c)
	// When the contract just signed is an amendment, expire the original.
	// Best-effort: a failure here only delays the original's EXPIRED flip
	// — the amendment is already SIGNED and the operator can reconcile by
	// hand. We don't fail the envelope_finished pipeline over a downstream
	// projection hop.
	if err := s.SupersedeOriginalOnAmendmentSigned(ctx, c); err != nil {
		log.Printf("WARN: failed to supersede original for amendment %s: %v", c.ID, err)
	}
	return c, nil
}

// HandleSignatureRefused is invoked by the signature.refused subscriber. The
// contract goes back to DRAFT with the lock cleared so the operator can fix
// it (wrong signer, wrong document, etc.) and resend.
func (s *ContractService) HandleSignatureRefused(ctx context.Context, tenantID, contractID, externalEnvelopeID, signerID, reason string) error {
	c, err := s.contractRepo.GetByID(ctx, tenantID, contractID)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("contract not found")
	}
	// Redelivery may be recovering a failed projection after the refusal
	// already committed. Mirror the same attempt without writing the contract.
	if c.Status == contract.StatusDraft && c.SignatureLockedAt == nil && externalEnvelopeID != "" && c.ExternalEnvelopeID == externalEnvelopeID {
		return s.syncSignatureRefusal(ctx, c, signerID)
	}
	if c.Status != contract.StatusPending && c.Status != contract.StatusPartiallySigned {
		return nil
	}
	matches, err := s.signatureAttemptMatches(ctx, c, externalEnvelopeID)
	if err != nil || !matches {
		if err == nil {
			log.Printf("ignored obsolete signature refusal/expiration for contract %s", contractID)
		}
		return err
	}
	if signerID != "" {
		found := false
		for _, signer := range c.Signatories {
			if signer.ID == signerID {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	expectedUpdatedAt := c.UpdatedAt
	copyContract := *c
	copyContract.Signatories = append([]contract.Signatory(nil), c.Signatories...)
	c = &copyContract
	c.ExternalEnvelopeID = externalEnvelopeID
	// Mirror the refusal onto the specific signatory so the admin sidebar
	// shows "Recusou" instead of staying pending.
	if signerID != "" {
		for i := range c.Signatories {
			if c.Signatories[i].ID == signerID {
				c.Signatories[i].Status = contract.SignatoryStatusRefused
				break
			}
		}
	}
	c.Status = contract.StatusDraft
	c.SignatureLockedAt = nil
	c.UpdatedAt = time.Now()
	if err := s.contractRepo.UpdateIfUnchanged(ctx, c, expectedUpdatedAt); err != nil {
		return err
	}
	return s.syncSignatureRefusal(ctx, c, signerID)
}

// buildSignatureRequestedEvent maps a Contract + send options into the event
// payload consumed by toca-signature.
func buildSignatureRequestedEvent(c *contract.Contract, req SendForSignatureRequest, now time.Time) pkgevents.SignatureRequestedEvent {
	signers := make([]pkgevents.SignatureRequestedSigner, 0, len(c.Signatories))
	for _, sig := range c.Signatories {
		auth := req.SignerAuth[sig.ID]
		if len(auth) == 0 {
			auth = sig.AuthMethods
		}
		if len(auth) == 0 {
			auth = []string{"email"}
		}
		signers = append(signers, pkgevents.SignatureRequestedSigner{
			SignerID:      sig.ID,
			Name:          sig.Name,
			Email:         sig.Email,
			PhoneNumber:   sig.PhoneNumber,
			Documentation: sig.Documentation,
			Birthday:      sig.Birthday,
			Role:          string(sig.Role),
			Auth:          auth,
		})
	}
	deadline := ""
	if req.Deadline != nil {
		deadline = req.Deadline.UTC().Format(time.RFC3339)
	}
	return pkgevents.SignatureRequestedEvent{
		Data: pkgevents.SignatureRequestedEventPayload{
			TenantID:    c.TenantID,
			ContractID:  c.ID,
			DealID:      c.DealID,
			DisplayID:   c.DisplayID,
			Signers:     signers,
			DocumentURL: c.Content,
			Locale:      req.Locale,
			Deadline:    deadline,
			Timestamp:   now.UTC().Format(time.RFC3339),
		},
	}
}

func formatOptionalTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// buildContractSignedEvent mirrors the existing SignContract() logic so the
// downstream consumer (toca-financial) receives the same payload regardless of
// whether the signature came from an internal flow or from Clicksign.
// buildContractSignedEventFor é o único caminho aceito para emitir
// deal.contract.signed: exige o negócio carregado.
//
// Sem o negócio o builder preenche só os campos do contrato (type/vazio,
// value 0, sem parties/property/owner/customer) e TODO consumidor rejeita o
// evento — o toca-financial responde "invalid contract value: must be greater
// than 0" e o toca-ops "missing contract/tenant/property". O resultado
// observado: contrato de locação assinado sem Rental, sem comissão de
// intermediação nas duas primeiras cobranças e repasse ao proprietário sem a
// dedução da comissão. Falhar aqui (e deixar o bus redeliver) é o
// comportamento correto.
func buildContractSignedEventFor(c *contract.Contract, d *deal.Deal, now time.Time) (contract.ContractSignedEvent, error) {
	if d == nil {
		return contract.ContractSignedEvent{}, fmt.Errorf(
			"deal %s not found for contract %s: refusing to emit deal.contract.signed without deal data (type/value/parties)",
			c.DealID, c.ID)
	}
	return buildContractSignedEvent(c, d, now), nil
}

func buildContractSignedEvent(c *contract.Contract, d *deal.Deal, now time.Time) contract.ContractSignedEvent {
	var event contract.ContractSignedEvent
	event.Data.ContractID = c.ID
	event.Data.DealID = c.DealID
	event.Data.TenantID = c.TenantID
	event.Data.SignedAt = now.Format(time.RFC3339)
	event.Data.StartDate = c.StartDate.Format(time.RFC3339)
	if c.EndDate != nil {
		event.Data.EndDate = c.EndDate.Format(time.RFC3339)
	}
	if d != nil {
		event.Data.Type = string(d.Type)
		event.Data.Value = d.Value
		event.Data.CommissionPct = d.Commission
		event.Data.CustomerID = d.CustomerID
		event.Data.OwnerID = d.OwnerID
		event.Data.PropertyID = d.PropertyID

		var bps []contract.BrokerParticipationPayload
		for _, bp := range d.BrokerParticipations {
			bps = append(bps, contract.BrokerParticipationPayload{
				BrokerID:   bp.BrokerID,
				Role:       string(bp.Role),
				Percentage: bp.Percentage,
			})
		}
		event.Data.BrokerParticipations = bps

		var parties []contract.PartyPayload
		for _, p := range d.Parties {
			parties = append(parties, contract.PartyPayload{
				PartyID:   p.ID,
				LeadID:    p.LeadID,
				AccountID: p.AccountID,
				Email:     p.Email,
				Role:      string(p.Role),
				IsPrimary: p.IsPrimary,
				SharePct:  p.SharePct,
			})
		}
		event.Data.Parties = parties
	}
	return event
}
