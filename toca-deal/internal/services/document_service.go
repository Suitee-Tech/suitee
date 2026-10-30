package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
)

const documentBucket = "toca-documents"

// presignedURLTTL is the validity window for download URLs handed out
// by GetDownloadURL / GetSignedFileURL / checklist downloads. Set via
// the `TOCA_PRESIGNED_URL_TTL` env var (Go duration syntax — e.g.
// `5m`, `1h`); falls back to the historical 15-minute default when
// unset or invalid. Bounded to [1m, 1h]: shorter is unsafe under slow
// networks (the link expires before the operator clicks it); longer
// bypasses the rotation guarantee the presigned scheme exists for.
//
// Read at process startup via `init()` so callers can read it as a
// plain `time.Duration` without locking.
var presignedURLTTL = readPresignedTTL()

const (
	presignedURLTTLMin     = 1 * time.Minute
	presignedURLTTLMax     = 1 * time.Hour
	presignedURLTTLDefault = 15 * time.Minute
)

func readPresignedTTL() time.Duration {
	raw := os.Getenv("TOCA_PRESIGNED_URL_TTL")
	if raw == "" {
		return presignedURLTTLDefault
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < presignedURLTTLMin || d > presignedURLTTLMax {
		return presignedURLTTLDefault
	}
	return d
}

// ContractSignedURLLookup is the minimal surface DocumentService needs from
// ContractService to recover the external signed-PDF URL when a Document
// row has no local storage copy yet (the persist subscriber hasn't run yet
// or failed transiently), to enumerate contracts attached to a deal
// (for `ListByDealIncludingContracts`), and to refresh the cached
// signed-URL from Clicksign when the local cache is empty (Phase 3
// reconcile gap recovery). Wired via SetContractLookup to avoid a
// circular import between the two services.
type ContractSignedURLLookup interface {
	GetSignedDocumentURL(ctx context.Context, tenantID, contractID string) (string, error)
	ListContractIDsByDeal(ctx context.Context, tenantID, dealID string) ([]string, error)
	// RefreshSignedDocumentURLFromRemote re-hits toca-signature
	// (which hits Clicksign) to fetch the current signed-PDF URL when
	// `GetSignedDocumentURL` returns empty. Used by the backfill to
	// recover contracts stuck because the original `envelope_finished`
	// webhook came without the URL ready. Returns empty string + nil
	// error when the file isn't ready yet — caller treats that as
	// "skip, try again next sweep".
	RefreshSignedDocumentURLFromRemote(ctx context.Context, tenantID, contractID string) (string, error)
}

type DocumentService struct {
	repo           document.RepositoryInterface
	storage        storage.Service
	contractLookup ContractSignedURLLookup
	eventBus       events.EventBus
	// virusScanner varre TODO byte antes do storage (P1-13). Fail-closed:
	// nil = upload recusado; o router injeta o clamd conforme CLAMAV_URL.
	virusScanner virusscan.Scanner
}

type internalStorageURLResolver interface {
	ResolveInternalURL(raw string) (string, error)
}

func NewDocumentService(repo document.RepositoryInterface, storageService storage.Service) *DocumentService {
	return &DocumentService{
		repo:    repo,
		storage: storageService,
	}
}

// WithVirusScanner injeta o antivírus do funil de uploads (P1-13).
// Encadeável; testes usam um scanner fake.
func (s *DocumentService) WithVirusScanner(scanner virusscan.Scanner) *DocumentService {
	s.virusScanner = scanner
	return s
}

// SetEventBus attaches the event bus so PublishUploaded can emit
// document.uploaded after a user upload. Optional — tests and slim deployments
// leave it nil (publish becomes a no-op). Wired once in routes.SetupRouter.
func (s *DocumentService) SetEventBus(bus events.EventBus) {
	s.eventBus = bus
}

// SetContractLookup attaches the contract-side fallback so GetDownloadURL
// can recover the Clicksign URL when the Supabase copy is still missing.
// Optional — calling code that does not need the fallback (tests, slim
// deployments) can skip the wiring entirely.
func (s *DocumentService) SetContractLookup(lookup ContractSignedURLLookup) {
	s.contractLookup = lookup
}

// ── CRUD ──

func (s *DocumentService) Create(ctx context.Context, d *document.Document) error {
	// Normalize entity linking: if contract_id is set but entity_id is not, auto-link
	if d.ContractID != "" && d.EntityID == "" {
		d.LinkToContract(d.ContractID)
	}
	return s.repo.Create(ctx, d)
}

// tenantFromContext resolve o tenant do chamador e falha fechado.
//
// SECURITY (achado C11): sem tenant não há como provar que o documento pertence
// a quem pediu. Workers/eventos que não têm tenant no contexto (ex.: extração)
// devem setá-lo a partir do payload — o handler de extração já faz isso com
// pkgctx.WithTenantID.
func tenantFromContext(ctx context.Context) (string, error) {
	tenantID, err := pkgcontext.GetTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		return "", fmt.Errorf("tenant context is required")
	}
	return tenantID, nil
}

// GetByID devolve o documento do tenant informado.
//
// SECURITY (achado C11): o tenant é parâmetro obrigatório e chega ao SQL.
func (s *DocumentService) GetByID(ctx context.Context, tenantID, id string) (*document.Document, error) {
	d, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, fmt.Errorf("document not found")
	}
	return d, nil
}

func (s *DocumentService) Update(ctx context.Context, d *document.Document) error {
	existing, err := s.repo.GetByID(ctx, d.TenantID, d.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("document not found")
	}
	d.TenantID = existing.TenantID
	d.CreatedAt = existing.CreatedAt
	// Preserve file fields if not changing file
	if d.FileURL == "" {
		d.FileURL = existing.FileURL
	}
	if d.StorageKey == "" {
		d.StorageKey = existing.StorageKey
	}
	if d.FileSize == 0 {
		d.FileSize = existing.FileSize
	}
	if d.FileType == "" {
		d.FileType = existing.FileType
	}
	// Preserve UploadedBy / UploadDate on metadata-only edits (rename, tag,
	// notes). Without this guard, a partial PUT zeroes the provenance stamp.
	if d.UploadedBy == "" {
		d.UploadedBy = existing.UploadedBy
	}
	if d.UploadDate.IsZero() {
		d.UploadDate = existing.UploadDate
	}
	// Preserve contract linkage
	if d.ContractID == "" {
		d.ContractID = existing.ContractID
	}
	if d.EntityID == "" {
		d.EntityID = existing.EntityID
		d.EntityType = existing.EntityType
	}
	d.UpdatedAt = time.Now()
	return s.repo.Update(ctx, d)
}

func (s *DocumentService) Delete(ctx context.Context, id, tenantID string) error {
	// Delete the file from storage if it exists
	doc, err := s.repo.GetByID(ctx, tenantID, id)
	if err == nil && doc != nil && doc.StorageKey != "" && s.storage != nil {
		_ = s.storage.Delete(ctx, documentBucket, doc.StorageKey)
	}
	return s.repo.Delete(ctx, id, tenantID)
}

// ── OCR / AI extraction ──

// shouldExtract gates which uploads trigger OCR+AI extraction: only
// data-bearing personal documents (identidade/comprovante) and certidões
// (juridico) that are PDF or image. Skips contracts, signed PDFs, photos and
// system-generated docs.
func shouldExtract(d *document.Document) bool {
	if d == nil {
		return false
	}
	switch d.Category {
	case document.CategoryIdentidade, document.CategoryComprovante, document.CategoryJuridico:
	default:
		return false
	}
	ft := strings.ToLower(d.FileType)
	return ft == "application/pdf" || strings.HasPrefix(ft, "image/")
}

// PublishUploaded emits document.uploaded for eligible uploads so the dedicated
// subscriber runs OCR+AI extraction asynchronously. Best-effort and nil-safe —
// never fails the upload.
func (s *DocumentService) PublishUploaded(ctx context.Context, d *document.Document) {
	if s.eventBus == nil || !shouldExtract(d) {
		return
	}
	evt := contracts.DocumentUploadedEvent{
		Data: contracts.DocumentUploadedEventPayload{
			DocumentID: d.ID,
			TenantID:   d.TenantID,
			EntityID:   d.EntityID,
			EntityType: string(d.EntityType),
			Category:   string(d.Category),
			FileType:   d.FileType,
			UploadedBy: d.UploadedBy,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to enqueue document.uploaded for doc=%s: %v", d.ID, err)
	}
}

// SetExtractionStatus updates only the extraction lifecycle fields.
func (s *DocumentService) SetExtractionStatus(ctx context.Context, id string, status document.ExtractionStatus, errMsg string) error {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return err
	}
	d, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("document not found")
	}
	now := time.Now()
	d.ExtractionStatus = status
	d.ExtractionError = errMsg
	d.ExtractionUpdatedAt = &now
	d.UpdatedAt = now
	return s.repo.Update(ctx, d)
}

// SetExtraction persists the extraction result + terminal status on the document.
func (s *DocumentService) SetExtraction(ctx context.Context, id string, ext *document.DocumentExtraction, status document.ExtractionStatus, errMsg string) error {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return err
	}
	d, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if d == nil {
		return fmt.Errorf("document not found")
	}
	now := time.Now()
	d.Extraction = ext
	d.ExtractionStatus = status
	d.ExtractionError = errMsg
	d.ExtractionUpdatedAt = &now
	d.UpdatedAt = now
	return s.repo.Update(ctx, d)
}

// GetExtraction returns the document (with its Extraction) after a tenant check.
func (s *DocumentService) GetExtraction(ctx context.Context, id, tenantID string) (*document.Document, error) {
	d, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if d.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}
	return d, nil
}

// RequeueExtraction marks a document pending and re-publishes document.uploaded so
// the extraction subscriber runs again (manual re-extract from the UI).
func (s *DocumentService) RequeueExtraction(ctx context.Context, id, tenantID string) error {
	d, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return err
	}
	if d.TenantID != tenantID {
		return fmt.Errorf("access denied")
	}
	if err := s.SetExtractionStatus(ctx, id, document.ExtractionPending, ""); err != nil {
		return err
	}
	s.PublishUploaded(ctx, d)
	return nil
}

// ── File Upload ──

func (s *DocumentService) Upload(ctx context.Context, d *document.Document, filename, contentType string, size int64, content io.Reader) error {
	if s.storage == nil {
		return fmt.Errorf("storage service not configured")
	}

	// P1-13: TODO byte passa pelo antivírus ANTES de existir no storage.
	// Fail-closed — scanner ausente/indisponível recusa o upload; vírus
	// detectado marca o documento com estado auditável (PersistenceFailed
	// + LastPersistError + trilha) e devolve ErrVirusFound.
	if s.virusScanner == nil {
		return fmt.Errorf("%w: virus scanner not configured for uploads", virusscan.ErrScannerUnavailable)
	}
	// O stream vem de multipart/HTTP (não-seekable): spool em memória para
	// escanear e persistir exatamente os mesmos bytes. O tamanho já é
	// limitado por categoria no ValidateWithMIME do handler; o scanner
	// ainda aplica o próprio teto (MaxBytes) como segunda linha.
	spooled, err := io.ReadAll(content)
	if err != nil {
		return fmt.Errorf("failed to read upload stream: %w", err)
	}
	now := time.Now()
	if _, err := s.virusScanner.Scan(ctx, bytes.NewReader(spooled)); err != nil {
		d.PersistenceStatus = document.PersistenceFailed
		d.LastPersistError = err.Error()
		d.PersistAttemptedAt = &now
		if errors.Is(err, virusscan.ErrVirusFound) {
			audit.Record(ctx, audit.Entry{
				Action:     "document.virus_rejected",
				EntityType: "document",
				EntityID:   d.ID,
				Metadata: map[string]any{
					"filename": filename,
					"error":    err.Error(),
					"tenant":   d.TenantID,
				},
			})
		}
		return fmt.Errorf("virus scan failed for %s: %w", filename, err)
	}

	// Generate a unique storage key: tenant/category/uuid_filename
	ext := filepath.Ext(filename)
	sanitized := storage.SanitizeFilenameBase(strings.TrimSuffix(filename, ext))
	storageKey := fmt.Sprintf("%s/%s/%s_%s%s", d.TenantID, d.Category, uuid.New().String()[:8], sanitized, ext)

	// Hash captura exatamente os bytes persistidos (o spool já foi
	// varrido pelo antivírus acima).
	fileHash := sha256.Sum256(spooled)

	url, err := s.storage.Upload(ctx, documentBucket, storageKey, contentType, bytes.NewReader(spooled))
	if err != nil {
		// Surface the failure on the Document so the backfill job
		// (BackfillUnstoredSignedPDFs) and operators can find it. The
		// caller is still responsible for persisting d via the repo.
		d.PersistenceStatus = document.PersistenceFailed
		d.LastPersistError = err.Error()
		d.PersistAttemptedAt = &now
		return fmt.Errorf("failed to upload file: %w", err)
	}

	d.FileURL = url
	d.StorageKey = storageKey
	d.FileType = contentType
	d.FileSize = size
	d.FileHash = hex.EncodeToString(fileHash[:])
	d.PersistenceStatus = document.PersistenceStored
	d.LastPersistError = ""
	d.PersistAttemptedAt = &now
	d.UploadDate = now
	d.UpdatedAt = now

	return nil
}

// GetFileURL returns the file URL for a document
func (s *DocumentService) GetFileURL(ctx context.Context, id string) (string, error) {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return "", err
	}
	doc, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return "", err
	}
	if doc.FileURL == "" {
		return "", fmt.Errorf("document has no file uploaded")
	}
	return doc.FileURL, nil
}

// GetFileContent returns the raw bytes plus the document metadata, used by
// the internal cross-service endpoint that lets toca-signature attach a
// stored Document to a Clicksign envelope. Tenant verification is the
// caller's responsibility — we only enforce that the file exists.
//
// The lookup prefers the local on-disk path (dev) and signs remote objects
// before fetching them. Network failures surface to the caller; we don't try
// to recover by re-uploading from Clicksign.
// signedPDFNeedsRefresh identifica o PDF assinado antigo, persistido antes
// de versionarmos a origem. Ele pode ser o arquivo de uma tentativa cancelada;
// o próximo acesso deve buscar o PDF do envelope final antes de servi-lo.
func signedPDFNeedsRefresh(d *document.Document) bool {
	return d != nil &&
		d.Category == document.CategoryContrato &&
		d.ContractID != "" &&
		d.StorageKey != "" &&
		strings.EqualFold(d.FileType, "application/pdf") &&
		d.SignedSourceHash == ""
}

func (s *DocumentService) GetFileContent(ctx context.Context, id string) (*document.Document, []byte, error) {
	tenantID, err := tenantFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	doc, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, nil, err
	}
	if signedPDFNeedsRefresh(doc) && s.contractLookup != nil {
		if _, refreshErr := s.EnsureSignedPDFURL(ctx, tenantID, doc.ContractID); refreshErr == nil {
			if fresh, freshErr := s.GetByID(ctx, tenantID, id); freshErr == nil {
				doc = fresh
			}
		}
	}
	if doc.FileURL == "" && doc.StorageKey == "" {
		return doc, nil, fmt.Errorf("document has no file uploaded")
	}

	// Local-storage fast path: mirrors DocumentHandler.Download. A resolução do
	// caminho é a única implementação (B8) e fica confinada ao STORAGE_DIR.
	if localPath, ok := storage.LocalUploadFile(doc.FileURL); ok {
		content, err := os.ReadFile(localPath)
		if err != nil {
			return doc, nil, fmt.Errorf("read local file: %w", err)
		}
		return doc, content, nil
	}

	remoteURL := doc.FileURL
	if doc.StorageKey != "" {
		if s.storage == nil {
			return doc, nil, fmt.Errorf("document storage is not configured")
		}
		remoteURL, err = s.storage.GetPresignedURL(ctx, documentBucket, doc.StorageKey, presignedURLTTL)
		if err != nil {
			return doc, nil, fmt.Errorf("sign document URL: %w", err)
		}
		if resolver, ok := s.storage.(internalStorageURLResolver); ok {
			remoteURL, err = resolver.ResolveInternalURL(remoteURL)
			if err != nil {
				return doc, nil, fmt.Errorf("resolve internal document URL: %w", err)
			}
		}
	}
	if remoteURL == "" {
		return doc, nil, fmt.Errorf("document has no downloadable URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, remoteURL, nil)
	if err != nil {
		return doc, nil, fmt.Errorf("build request: %w", err)
	}
	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return doc, nil, fmt.Errorf("fetch document: %w", err)
	}
	defer drainAndClose(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return doc, nil, fmt.Errorf("fetch document: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return doc, nil, fmt.Errorf("read body: %w", err)
	}
	return doc, body, nil
}

// drainAndClose drains the response body (so the underlying TCP connection
// can be returned to the pool for keep-alive reuse) and closes it. Using
// plain `defer resp.Body.Close()` without draining leaks idle TCP sockets
// on error paths — under load that exhausts file descriptors and stalls
// downstream HTTP calls from the same process.
func drainAndClose(body io.ReadCloser) {
	if body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, body)
	_ = body.Close()
}

func (s *DocumentService) GetDownloadURL(ctx context.Context, id, tenantID string) (string, error) {
	doc, err := s.GetByID(ctx, tenantID, id)
	if err != nil {
		return "", err
	}
	if doc.TenantID != tenantID {
		return "", fmt.Errorf("access denied")
	}
	if signedPDFNeedsRefresh(doc) && s.contractLookup != nil {
		if fresh, refreshErr := s.EnsureSignedPDFURL(ctx, tenantID, doc.ContractID); refreshErr == nil && fresh != "" {
			return fresh, nil
		}
	}
	if doc.StorageKey != "" {
		if s.storage == nil {
			return "", fmt.Errorf("document storage is not configured")
		}
		return s.storage.GetPresignedURL(ctx, documentBucket, doc.StorageKey, presignedURLTTL)
	}
	if doc.FileURL != "" {
		return doc.FileURL, nil
	}

	// No local copy yet. For contract-linked documents we can fall back to
	// the Clicksign signed-PDF URL stored on the Contract — that URL is
	// what triggered PersistSignedContractPDF in the first place, so it's
	// guaranteed to point to a real file. We try the persist on demand so
	// the next fetch hits Supabase instead of round-tripping Clicksign.
	if doc.Category == document.CategoryContrato && doc.ContractID != "" && s.contractLookup != nil {
		externalURL, _ := s.contractLookup.GetSignedDocumentURL(ctx, tenantID, doc.ContractID)
		if externalURL != "" {
			// Best-effort persist. If it succeeds the next call lands on
			// the Supabase path above. If it fails we still have the
			// Clicksign URL to hand back below.
			if err := s.PersistSignedContractPDF(ctx, tenantID, doc.ContractID, externalURL); err == nil {
				if fresh, ferr := s.repo.GetByID(ctx, tenantID, id); ferr == nil && fresh != nil && fresh.StorageKey != "" {
					return s.storage.GetPresignedURL(ctx, documentBucket, fresh.StorageKey, presignedURLTTL)
				}
			}
			// Hand back the external URL — but record the fallback so
			// the dashboard surfaces sustained reliance on it (means
			// the persist pipeline is degraded).
			metrics.IncSignedPdfFallbackClicksign()
			return externalURL, nil
		}
	}
	return "", fmt.Errorf("document has no file uploaded")
}

// ── Signature sync ──

// SyncFromContractStatus propagates a contract.Status change onto every
// Document linked to that contract. The Central de Documentos page filters
// by Document.Status (rascunho/pendente_assinatura/parcialmente_assinado/
// assinado/...), so without this hook a contract that gets signed via
// Clicksign would still show "Rascunho" or "Pendente" in the documents
// view — the same drift we fixed between contract.Status and
// contract.Signatories[].Status.
//
// Idempotent and best-effort: failures on individual documents are logged
// but don't abort the rest. Returns the count of documents updated.
func (s *DocumentService) SyncFromContractStatus(ctx context.Context, tenantID, contractID, contractStatus string) (int, error) {
	docs, err := s.repo.ListByContract(ctx, contractID, tenantID)
	if err != nil {
		return 0, err
	}
	target := document.StatusFromContractStatus(contract.Status(contractStatus))
	if target == "" {
		return 0, nil // not a status we mirror
	}
	updated := 0
	for _, d := range docs {
		if d.Status == target {
			continue
		}
		d.Status = target
		d.UpdatedAt = time.Now()
		if err := s.repo.Update(ctx, d); err != nil {
			// Don't abort — sync should be best-effort across documents.
			continue
		}
		updated++
	}
	return updated, nil
}

// EnsureForContract creates a placeholder Document tied to a contract when
// none exists yet. Idempotent: re-running is a no-op once the placeholder
// (or any operator-uploaded document) is already linked. Used by
// ContractService.CreateContract so the Central de Documentos always shows
// contracts even before the operator uploads a file.
//
// uploadedBy stamps the actor that triggered the placeholder creation —
// typically the operator's user id from the request context, or "system"
// when called from a webhook recovery path. Without this stamp the
// Central de Documentos rendered "Enviado por: undefined" because the
// frontend has nothing to fall back to.
//
// FileType is pre-set to the DOCX mime since the contract draft is always
// generated from a .docx template; it will be overwritten by Upload() when
// the actual bytes land (and again when PersistSignedContractPDF replaces
// the draft with the signed PDF).
func (s *DocumentService) EnsureForContract(ctx context.Context, tenantID, contractID, dealID, name, uploadedBy string) error {
	if contractID == "" || tenantID == "" {
		return nil
	}
	existing, err := s.repo.ListByContract(ctx, contractID, tenantID)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}
	_ = dealID // reserved for future use — Document has no first-class deal id field today
	if uploadedBy == "" {
		uploadedBy = "system"
	}
	now := time.Now()
	d := &document.Document{
		ID:                uuid.NewString(),
		TenantID:          tenantID,
		Name:              name,
		Category:          document.CategoryContrato,
		Status:            document.StatusRascunho,
		Source:            document.SourceDeal,
		ContractID:        contractID,
		EntityID:          contractID,
		EntityType:        document.EntityContract,
		FileType:          "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		UploadedBy:        uploadedBy,
		UploadDate:        now,
		Signers:           []document.DocumentSigner{},
		Tags:              []string{},
		Version:           1,
		PersistenceStatus: document.PersistencePending,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	return s.repo.Create(ctx, d)
}

// UploadDocxForContract persists the contract's DOCX bytes to MinIO and
// updates the linked Document row so the Central de Documentos surfaces a
// downloadable draft (file_size, file_url, file_type) instead of the empty
// placeholder. Called from ContractService.CreateContract / UpdateContract
// whenever contract.Content is populated by the operator.
//
// Idempotent on content hash: a SHA-256 of the bytes is embedded in the
// storage key, so retries with the same content skip the upload. Skipped
// outright once the document has crossed into pendente_assinatura /
// assinado — at that point the signed PDF (or the canonical envelope at
// Clicksign) is the source of truth, not the draft.
func (s *DocumentService) UploadDocxForContract(ctx context.Context, tenantID, contractID string, docxBytes []byte, uploadedBy string) error {
	if len(docxBytes) == 0 || tenantID == "" || contractID == "" {
		return nil
	}
	if s.storage == nil {
		return fmt.Errorf("storage service not configured")
	}

	docs, err := s.repo.ListByContract(ctx, contractID, tenantID)
	if err != nil {
		return fmt.Errorf("list documents for contract %s: %w", contractID, err)
	}
	var d *document.Document
	for _, x := range docs {
		if x.Category == document.CategoryContrato {
			d = x
			break
		}
	}
	if d == nil {
		name := fmt.Sprintf("Contrato %s", contractID)
		if err := s.EnsureForContract(ctx, tenantID, contractID, "", name, uploadedBy); err != nil {
			return fmt.Errorf("ensure document for contract %s: %w", contractID, err)
		}
		docs, err = s.repo.ListByContract(ctx, contractID, tenantID)
		if err != nil {
			return fmt.Errorf("re-list documents for contract %s: %w", contractID, err)
		}
		for _, x := range docs {
			if x.Category == document.CategoryContrato {
				d = x
				break
			}
		}
		if d == nil {
			return fmt.Errorf("contract document missing after EnsureForContract for %s", contractID)
		}
	}

	// Once the envelope is in flight or signed, the DOCX is no longer the
	// canonical artifact — bail rather than overwrite the signed PDF.
	if d.Status == document.StatusAssinado ||
		d.Status == document.StatusPendenteAssinatura ||
		d.Status == document.StatusParcialmenteAssinado {
		return nil
	}

	sum := sha256.Sum256(docxBytes)
	hash := hex.EncodeToString(sum[:])
	if d.StorageKey != "" && strings.Contains(d.StorageKey, hash[:16]) {
		// Same content already persisted — webhook/handler retry.
		return nil
	}

	if uploadedBy != "" {
		d.UploadedBy = uploadedBy
	}
	filename := fmt.Sprintf("rascunho-%s-%s.docx", contractID, hash[:8])
	contentType := "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

	if err := s.Upload(ctx, d, filename, contentType, int64(len(docxBytes)), bytes.NewReader(docxBytes)); err != nil {
		return fmt.Errorf("upload contract docx: %w", err)
	}
	return s.repo.Update(ctx, d)
}

// PersistSignedContractPDF downloads the signed PDF from the URL Clicksign
// returned on `envelope_finished` and stores it in MinIO via the existing
// Upload pipeline, then writes FileURL/StorageKey back onto the contract's
// Document row so it surfaces in the Central de Documentos with a working
// download. Idempotent: a second invocation with the same source URL is a
// no-op once StorageKey has been populated (RabbitMQ at-least-once delivery
// means retries are expected).
func (s *DocumentService) PersistSignedContractPDF(ctx context.Context, tenantID, contractID, sourceURL string) error {
	if sourceURL == "" || tenantID == "" || contractID == "" {
		return nil
	}
	if s.storage == nil {
		return fmt.Errorf("storage service not configured")
	}
	// A URL do Clicksign é pré-assinada: a query muda a cada renovação. A
	// identidade estável é o caminho do objeto; usar a URL inteira faria o
	// mesmo PDF parecer uma tentativa nova em cada retry/backfill.
	sourceIdentity := sourceURL
	if i := strings.IndexAny(sourceIdentity, "?#"); i >= 0 {
		sourceIdentity = sourceIdentity[:i]
	}
	sourceSum := sha256.Sum256([]byte(sourceIdentity))
	sourceHash := hex.EncodeToString(sourceSum[:])

	docs, err := s.repo.ListByContract(ctx, contractID, tenantID)
	if err != nil {
		return fmt.Errorf("list documents for contract %s: %w", contractID, err)
	}
	var d *document.Document
	for _, x := range docs {
		if x.Category == document.CategoryContrato {
			d = x
			break
		}
	}
	if d == nil {
		// Placeholder was never created (createContract path didn't reach
		// EnsureForContract). Create one now so we always have a Document
		// row to attach the file to. "Clicksign" as uploadedBy because the
		// only reason we'd be here is the signed-PDF webhook recovery.
		name := fmt.Sprintf("Contrato %s", contractID)
		if err := s.EnsureForContract(ctx, tenantID, contractID, "", name, "Clicksign"); err != nil {
			return fmt.Errorf("ensure document for contract %s: %w", contractID, err)
		}
		docs, err = s.repo.ListByContract(ctx, contractID, tenantID)
		if err != nil {
			return fmt.Errorf("re-list documents for contract %s: %w", contractID, err)
		}
		for _, x := range docs {
			if x.Category == document.CategoryContrato {
				d = x
				break
			}
		}
		if d == nil {
			return fmt.Errorf("contract document still missing after EnsureForContract for %s", contractID)
		}
	}

	// Idempotência verdadeira: só pula o download quando o row já carrega o
	// PDF assinado. UploadDocxForContract também popula StorageKey (rascunho
	// DOCX) antes do envelope ser enviado, então StorageKey != "" sozinho
	// flipava o status para Assinado mantendo o arquivo no DOCX — a UI então
	// abria o rascunho no SuperDoc em vez do PDF assinado.
	if d.StorageKey != "" && strings.EqualFold(d.FileType, "application/pdf") && d.SignedSourceHash == sourceHash {
		if d.Status != document.StatusAssinado {
			d.Status = document.StatusAssinado
			d.UpdatedAt = time.Now()
			return s.repo.Update(ctx, d)
		}
		return nil
	}

	// Short timeout (10s) so a slow/unreachable Clicksign CDN can't pin a
	// RabbitMQ worker for a full minute. Failures fall through to the
	// subscriber's Nack path and land in the DLQ for manual replay — that's
	// cheaper than holding a worker hostage. Previous value was 60s and was
	// observed pinning the deal-side pool when running against a sandbox
	// that couldn't reach the Clicksign signed-PDF host.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return s.markPersistFailed(ctx, d, "request", fmt.Errorf("build request for signed PDF: %w", err))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return s.markPersistFailed(ctx, d, "download", fmt.Errorf("download signed PDF: %w", err))
	}
	defer drainAndClose(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return s.markPersistFailed(ctx, d, "download", fmt.Errorf("download signed PDF returned status %d", resp.StatusCode))
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/pdf"
	}

	// Read the body fully so we can validate structure *before* writing
	// to durable storage. PDFs from Clicksign are typically <2MB so the
	// memory cost is bounded; reading-then-uploading also lets us emit
	// a meaningful error if the bytes aren't actually a PDF.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return s.markPersistFailed(ctx, d, "read", fmt.Errorf("read signed PDF body: %w", err))
	}
	if err := document.ValidateSignedPDF(body); err != nil {
		return s.markPersistFailed(ctx, d, "validation", fmt.Errorf("validate signed PDF: %w", err))
	}

	filename := document.SignedContractFilename(contractID, d.Version)

	// Stamp before Upload so the metadata bump uses the Clicksign identity
	// even if Upload's UpdatedAt overwrites our local timestamp.
	d.UploadedBy = "Clicksign"
	if err := s.Upload(ctx, d, filename, contentType, int64(len(body)), bytes.NewReader(body)); err != nil {
		// Upload has already stamped PersistenceFailed/LastPersistError —
		// persist that so the backfill job can pick this up.
		metrics.IncSignedPdfPersistFailed("storage")
		if uerr := s.repo.Update(ctx, d); uerr != nil {
			return fmt.Errorf("upload signed PDF (and failed to persist failure marker: %v): %w", uerr, err)
		}
		return fmt.Errorf("upload signed PDF: %w", err)
	}
	// Captura status anterior para o Before do audit (#471). d.Status já
	// reflete o estado pré-upload (Upload faz s.repo.Update mas para
	// arquivo+metadados, sem mudar status — esse é setado abaixo).
	beforeStatus := d.Status
	beforeHash := d.FileHash

	d.Status = document.StatusAssinado
	d.SignedSourceHash = sourceHash
	if !strings.Contains(strings.ToLower(d.Name), "assinado") {
		d.Name = fmt.Sprintf("Contrato assinado — %s", strings.TrimPrefix(d.Name, "Contrato "))
	}
	if err := s.repo.Update(ctx, d); err != nil {
		metrics.IncSignedPdfPersistFailed("storage")
		return err
	}
	metrics.IncSignedPdfPersistSucceeded()

	// Forensic trail — the signed PDF landing is the event a legal
	// dispute would walk back to. Include the SHA-256 from the upload
	// so post-hoc verification can confirm the stored bytes match.
	// Before/after garantem diff completo (#471) — disputes podem precisar
	// reconstituir o status anterior (Enviado/Aguardando/etc.).
	audit.Record(ctx, audit.Entry{
		Action:     "document.signed",
		EntityType: "Document",
		EntityID:   d.ID,
		Before: map[string]any{
			"status":    string(beforeStatus),
			"file_hash": beforeHash,
		},
		After: map[string]any{
			"status":    string(document.StatusAssinado),
			"file_hash": d.FileHash,
		},
		Metadata: map[string]any{
			"contract_id": contractID,
			"source_url":  sourceURL,
			"storage_key": d.StorageKey,
			"file_name":   filename,
		},
	})
	return nil
}

// BackfillStats reports the outcome of a BackfillUnstoredSignedPDFs run.
// Surfaced via the admin endpoint so operators can confirm the queue is
// draining (or see exactly which docs are still stuck).
type BackfillStats struct {
	Scanned      int      `json:"scanned"`
	Reattempted  int      `json:"reattempted"`
	Succeeded    int      `json:"succeeded"`
	Failed       int      `json:"failed"`
	SkippedNoURL int      `json:"skipped_no_url"`
	FailedDocIDs []string `json:"failed_doc_ids,omitempty"`
}

// BackfillUnstoredSignedPDFs walks every contract-linked document in
// the tenant whose `Status=assinado` but bytes still aren't durably
// stored (`StorageKey=""` or `PersistenceStatus != stored`) and reruns
// `PersistSignedContractPDF` against the contract's external signed
// URL. Idempotent — successful rows go from `pending`/`failed` to
// `stored` and stop matching the filter on subsequent runs.
//
// Targeted at incidents where the webhook subscriber crashed or
// Supabase was briefly unreachable. Intended to be called from:
//   - the admin endpoint POST /admin/documents/backfill-signed
//   - a future cron (see roadmap §3 S6)
//
// Returns BackfillStats with per-outcome counts plus the IDs of docs
// that still failed (useful for the operator to triage individually).
// `tenantID` is required — backfill is scoped per-tenant to keep
// blast radius bounded if something's wrong with the bucket.
func (s *DocumentService) BackfillUnstoredSignedPDFs(ctx context.Context, tenantID string) (BackfillStats, error) {
	stats := BackfillStats{}
	if tenantID == "" {
		return stats, fmt.Errorf("tenant id required")
	}
	metrics.IncSignedPdfBackfillRun()
	defer func() {
		// Stamp recovery count even on partial failure — the metric is
		// "how many did the sweep actually fix?", not "did the sweep
		// complete cleanly?".
		if stats.Succeeded > 0 {
			metrics.AddSignedPdfBackfillRecovered(uint64(stats.Succeeded))
		}
	}()
	if s.contractLookup == nil {
		// Without contract lookup we can't recover the source URL,
		// which is the whole point of the backfill. Fail loud so the
		// operator notices the misconfiguration instead of silently
		// succeeding with zero work done.
		return stats, fmt.Errorf("contract lookup not wired — cannot backfill signed PDFs")
	}

	// Scan every signed document in the tenant. We use the Status
	// filter because there's no `PersistenceStatus` filter today —
	// adding one is roadmap §3 S6 follow-up. For tenants with very
	// large signed-document volumes this should grow a paginated
	// variant; for now a single scan is acceptable since the operator
	// invokes it on-demand.
	docs, _, err := s.repo.List(ctx, tenantID, document.ListFilters{
		Status: document.StatusAssinado,
		Limit:  10_000,
	})
	if err != nil {
		return stats, fmt.Errorf("list signed documents: %w", err)
	}

	for _, d := range docs {
		stats.Scanned++
		if d == nil {
			continue
		}
		// Só pula quando o storage já carrega o PDF assinado. Rows que
		// persistiram apenas o rascunho DOCX (mime DOCX) precisam ser
		// reprocessadas — eram exatamente o caso que mantinha contratos
		// abrindo o DOCX em vez do PDF na UI antes do fix.
		if d.StorageKey != "" &&
			d.PersistenceStatus == document.PersistenceStored &&
			strings.EqualFold(d.FileType, "application/pdf") &&
			d.SignedSourceHash != "" {
			continue
		}
		if d.ContractID == "" {
			// Not contract-linked → we don't know where to fetch the
			// signed bytes from. Counted as skipped, not failed.
			stats.SkippedNoURL++
			continue
		}
		// Always fetch a FRESH Clicksign URL: the S3 presigned link expires
		// in ~5 min, so the cached contract.SignedDocumentURL is almost
		// certainly stale and downloading it returns 403.
		// RefreshSignedDocumentURLFromRemote re-queries Clicksign live. When
		// it returns empty the signed file genuinely is not ready yet —
		// skip and let the next backfill sweep retry.
		signedURL, err := s.contractLookup.RefreshSignedDocumentURLFromRemote(ctx, tenantID, d.ContractID)
		if err != nil || signedURL == "" {
			stats.SkippedNoURL++
			continue
		}
		stats.Reattempted++
		if err := s.PersistSignedContractPDF(ctx, tenantID, d.ContractID, signedURL); err != nil {
			stats.Failed++
			stats.FailedDocIDs = append(stats.FailedDocIDs, d.ID)
			continue
		}
		stats.Succeeded++
	}
	return stats, nil
}

// markPersistFailed stamps the Document with the persist-failed marker
// and best-effort persists it so BackfillUnstoredSignedPDFs can find
// stuck rows. Also increments the failed counter so the Grafana board
// surfaces the trend. Returns the original cause unchanged — callers
// should `return s.markPersistFailed(ctx, d, "reason", err)` to
// preserve the error chain.
//
// `reason` is the bounded enum used by `metrics.IncSignedPdfPersistFailed`
// (download|validation|storage|read|request|other) — keep this in sync
// with the help text in pkg/metrics/metrics.go when adding cases.
func (s *DocumentService) markPersistFailed(ctx context.Context, d *document.Document, reason string, cause error) error {
	now := time.Now()
	d.PersistenceStatus = document.PersistenceFailed
	d.LastPersistError = cause.Error()
	d.PersistAttemptedAt = &now
	d.UpdatedAt = now
	// Update is best-effort — if it fails we don't shadow the original
	// cause, since that's what the operator actually needs to see.
	_ = s.repo.Update(ctx, d)
	metrics.IncSignedPdfPersistFailed(reason)
	return cause
}

// GetSignedFileURL returns a presigned MinIO URL for the contract's signed
// PDF once PersistSignedContractPDF has cached it locally. Returns empty
// string when no signed PDF is stored yet — callers should fall back to
// Contract.SignedDocumentURL (Clicksign's external URL).
//
// The file-type guard is essential: the contract Document starts life as the
// editable DOCX draft (UploadDocxForContract) and only becomes the signed PDF
// after PersistSignedContractPDF runs. Without the guard this returned the
// DOCX storage key, so the "signed file" viewer (esteira + portal) opened the
// draft in the SuperDoc editor instead of the signed PDF.
func (s *DocumentService) GetSignedFileURL(ctx context.Context, tenantID, contractID string) (string, error) {
	if s.storage == nil || tenantID == "" || contractID == "" {
		return "", nil
	}
	docs, err := s.repo.ListByContract(ctx, contractID, tenantID)
	if err != nil {
		return "", err
	}
	for _, d := range docs {
		if d.Category != document.CategoryContrato || d.StorageKey == "" {
			continue
		}
		// Only the persisted signed PDF qualifies — never the DOCX draft.
		if !strings.EqualFold(d.FileType, "application/pdf") {
			continue
		}
		return s.storage.GetPresignedURL(ctx, documentBucket, d.StorageKey, presignedURLTTL)
	}
	return "", nil
}

// EnsureSignedPDFURL returns a browser-safe presigned MinIO URL for the
// contract's signed PDF. When the PDF is not cached locally yet it resolves
// the Clicksign URL (cached on the contract, or refreshed live via
// toca-signature), downloads it server-side and persists it to MinIO first.
//
// It deliberately never returns the raw Clicksign S3 URL: that bucket sends
// no CORS header for the app origin (the browser PDF viewer cannot fetch it)
// and the presigned link expires in ~5 min. Returns "" when the signed PDF
// genuinely cannot be obtained yet — callers surface a "try again" state.
func (s *DocumentService) EnsureSignedPDFURL(ctx context.Context, tenantID, contractID string) (string, error) {
	if tenantID == "" || contractID == "" {
		return "", nil
	}
	// Fast path: a cópia durável já existe E foi gerada por um PDF com origem
	// versionada. Linhas antigas (source hash vazio) podem apontar para o PDF
	// de uma tentativa cancelada; nesse caso forçamos o refresh abaixo.
	if url, err := s.GetSignedFileURL(ctx, tenantID, contractID); err == nil && url != "" {
		docs, listErr := s.repo.ListByContract(ctx, contractID, tenantID)
		stale := false
		if listErr == nil {
			for _, d := range docs {
				if signedPDFNeedsRefresh(d) {
					stale = true
					break
				}
			}
		}
		if !stale {
			return url, nil
		}
	}
	if s.contractLookup == nil {
		return "", nil
	}
	// Always fetch a FRESH Clicksign URL right before downloading. The S3
	// presigned link expires in ~5 min, so the value cached on
	// contract.SignedDocumentURL is almost always stale — downloading it
	// returns 403. RefreshSignedDocumentURLFromRemote re-queries Clicksign
	// for a link that is valid right now.
	src, _ := s.contractLookup.RefreshSignedDocumentURLFromRemote(ctx, tenantID, contractID)
	if src == "" {
		return "", nil
	}
	// Download (server-side — no CORS) and cache in MinIO, then hand back the
	// durable presigned URL.
	if err := s.PersistSignedContractPDF(ctx, tenantID, contractID, src); err != nil {
		return "", err
	}
	return s.GetSignedFileURL(ctx, tenantID, contractID)
}

// ── Query methods ──

// ReclassifyContractAsAmendment promotes the contract Document for a
// signed amendment from CategoryContrato to CategoryAditivo. The original
// contract keeps CategoryContrato (it lives in a different ContractID).
// Idempotent: no-op once the row is already classified.
func (s *DocumentService) ReclassifyContractAsAmendment(ctx context.Context, tenantID, contractID string) error {
	if tenantID == "" || contractID == "" {
		return nil
	}
	docs, err := s.repo.ListByContract(ctx, contractID, tenantID)
	if err != nil {
		return err
	}
	for _, d := range docs {
		if d == nil {
			continue
		}
		if d.Category == document.CategoryContrato {
			d.Category = document.CategoryAditivo
			d.UpdatedAt = time.Now()
			if err := s.repo.Update(ctx, d); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *DocumentService) ListByContract(ctx context.Context, contractID, tenantID string) ([]*document.Document, error) {
	return s.repo.ListByContract(ctx, contractID, tenantID)
}

func (s *DocumentService) ListByEntity(ctx context.Context, entityID string, entityType document.EntityType, tenantID string) ([]*document.Document, error) {
	return s.repo.ListByEntity(ctx, entityID, entityType, tenantID)
}

// ListByDealIncludingContracts returns every document tied to the deal,
// whether linked directly (`entity_type=deal`) or via one of the deal's
// contracts (`entity_type=contract`, `contract_id` set). The Esteira
// Documentation tab uses this so signed PDFs from the contract show up
// next to checklist uploads instead of being hidden in the separate
// ContractTab.
//
// Falls back to the deal-only view when the contract lookup hasn't been
// wired (older test setups) — the caller still gets a useful result.
// Deduplicates by document ID since a doc can technically be reachable
// via both paths if EntityID was set to dealID and ContractID to the
// contract.
func (s *DocumentService) ListByDealIncludingContracts(ctx context.Context, tenantID, dealID string) ([]*document.Document, error) {
	dealDocs, err := s.repo.ListByEntity(ctx, dealID, document.EntityDeal, tenantID)
	if err != nil {
		return nil, err
	}
	if s.contractLookup == nil {
		return dealDocs, nil
	}
	contractIDs, err := s.contractLookup.ListContractIDsByDeal(ctx, tenantID, dealID)
	if err != nil {
		// Non-fatal: returning what we have is more useful than 500ing
		// just because the contract index momentarily failed.
		return dealDocs, nil
	}

	seen := make(map[string]bool, len(dealDocs))
	out := make([]*document.Document, 0, len(dealDocs))
	for _, d := range dealDocs {
		if d == nil || seen[d.ID] {
			continue
		}
		seen[d.ID] = true
		out = append(out, d)
	}
	for _, cID := range contractIDs {
		cdocs, err := s.repo.ListByContract(ctx, cID, tenantID)
		if err != nil {
			continue // best-effort per contract
		}
		for _, d := range cdocs {
			if d == nil || seen[d.ID] {
				continue
			}
			seen[d.ID] = true
			out = append(out, d)
		}
	}
	return out, nil
}

func (s *DocumentService) List(ctx context.Context, tenantID string, filters document.ListFilters) ([]*document.Document, int64, error) {
	return s.repo.List(ctx, tenantID, filters)
}

func (s *DocumentService) Search(ctx context.Context, tenantID, query string, limit, offset int64) ([]*document.Document, int64, error) {
	return s.repo.Search(ctx, tenantID, query, limit, offset)
}

// ListKnownTags surfaces the distinct set of tag values used inside the
// tenant. Backs the Central de Documentos tag autocomplete + the "Pastas
// relevantes" sidebar.
func (s *DocumentService) ListKnownTags(ctx context.Context, tenantID string) ([]string, error) {
	return s.repo.DistinctTags(ctx, tenantID)
}

// ── Version management ──

func (s *DocumentService) CreateVersion(ctx context.Context, parentID, tenantID string, filename, contentType string, size int64, content io.Reader, uploadedBy string) (*document.Document, error) {
	parent, err := s.GetByID(ctx, tenantID, parentID)
	if err != nil {
		return nil, fmt.Errorf("parent document not found: %w", err)
	}
	if parent.TenantID != tenantID {
		return nil, fmt.Errorf("access denied")
	}

	// Create a new document as a version of the parent
	newDoc := document.New(parent.Name, parent.Category, contentType, size, uploadedBy, tenantID)
	newDoc.ParentID = parentID
	newDoc.Version = parent.Version + 1
	newDoc.EntityID = parent.EntityID
	newDoc.EntityType = parent.EntityType
	newDoc.ContractID = parent.ContractID
	newDoc.Source = parent.Source
	newDoc.Tags = parent.Tags

	if err := s.Upload(ctx, newDoc, filename, contentType, size, content); err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, newDoc); err != nil {
		return nil, err
	}
	return newDoc, nil
}

// ChecklistDocumentInput descreve um arquivo que o cliente (ou o operador)
// anexou a um item do checklist da esteira digital e que precisa aparecer
// organizado no painel de documentos da imobiliária.
type ChecklistDocumentInput struct {
	TenantID     string
	DealID       string
	ContractID   string
	PartyID      string
	ItemID       string
	ItemName     string
	ItemCategory string
	// RequestedFieldLabels são os rótulos dos campos pedidos no item ("CPF",
	// "Comprovante de renda"): ajudam a classificar quando o nome do item é
	// genérico.
	RequestedFieldLabels []string

	FileName    string
	StorageKey  string
	FileSize    int64
	ContentType string
	UploadedBy  string
	UploadedAt  time.Time
}

// EnsureForChecklistItem materializa (ou atualiza) no painel de documentos o
// arquivo anexado a um item do checklist.
//
// Por que existe: o upload do cliente vive dentro de `deal.deals.metadata.
// checklist[]`, e o painel lê `deal.documents`. Sem este espelho o documento
// enviado pela esteira digital simplesmente não existia para a imobiliária.
//
// Ordem de operações: o checklist é a fonte da verdade (o cliente já teve o
// upload confirmado), então a falha aqui deve ser reportada, não silenciosa —
// mas quem chama decide se derruba a requisição (o portal registra WARN e segue,
// porque o arquivo já está salvo). O ID é determinístico por item
// (document.ChecklistDocumentID), então reenvio/replay ATUALIZA a mesma linha em
// vez de duplicar o documento no painel.
func (s *DocumentService) EnsureForChecklistItem(ctx context.Context, in ChecklistDocumentInput) (*document.Document, error) {
	if strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.ItemID) == "" {
		return nil, nil
	}
	if strings.TrimSpace(in.StorageKey) == "" && strings.TrimSpace(in.FileName) == "" {
		// Item cumprido só com preenchimento de formulário: não há arquivo.
		return nil, nil
	}

	classification := document.ClassifyChecklistItem(in.ItemName, in.ItemCategory, in.RequestedFieldLabels...)
	name := checklistDocumentName(in)
	fileType := strings.TrimSpace(in.ContentType)
	if fileType == "" {
		fileType = mimeForChecklistFile(in.FileName)
	}
	entityType := document.EntityDeal
	entityID := in.DealID
	if entityID == "" {
		// Sem negócio não há como organizar por esteira; usa o próprio item,
		// assim o documento continua rastreável (EntityType = checklist).
		entityType = document.EntityChecklist
		entityID = in.ItemID
	}
	uploadedBy := strings.TrimSpace(in.UploadedBy)
	if uploadedBy == "" {
		uploadedBy = "cliente"
	}
	uploadedAt := in.UploadedAt
	if uploadedAt.IsZero() {
		uploadedAt = time.Now()
	}
	fileURL := fmt.Sprintf("/api/v1/deals/%s/checklist/%s/download", in.DealID, in.ItemID)

	id := document.ChecklistDocumentID(in.ItemID)
	existing, err := s.repo.GetByID(ctx, in.TenantID, id)
	if err != nil {
		// O repositório real devolve "not found" como erro embrulhado
		// (`fmt.Errorf("document not found: %w", sql.ErrNoRows)`), então
		// ausência de linha não pode abortar a criação do espelho.
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("lookup checklist document %s: %w", id, err)
		}
		existing = nil
	}

	if existing != nil {
		changed := false
		if in.StorageKey != "" && existing.StorageKey != in.StorageKey {
			existing.StorageKey = in.StorageKey
			existing.Version++
			changed = true
		}
		if name != "" && existing.Name != name {
			existing.Name = name
			changed = true
		}
		if existing.Category != classification.Category {
			existing.Category = classification.Category
			changed = true
		}
		if !sameTags(existing.Tags, classification.Tags) {
			existing.Tags = classification.Tags
			changed = true
		}
		if in.FileSize > 0 && existing.FileSize != in.FileSize {
			existing.FileSize = in.FileSize
			changed = true
		}
		if fileType != "" && existing.FileType != fileType {
			existing.FileType = fileType
			changed = true
		}
		if existing.EntityID != entityID || existing.EntityType != entityType {
			existing.EntityID, existing.EntityType = entityID, entityType
			changed = true
		}
		if existing.ContractID == "" && in.ContractID != "" {
			existing.ContractID = in.ContractID
			changed = true
		}
		if !changed {
			return existing, nil
		}
		existing.FileURL = fileURL
		existing.PersistenceStatus = document.PersistenceStored
		existing.UploadedBy = uploadedBy
		existing.UploadDate = uploadedAt
		existing.UpdatedAt = time.Now()
		if err := s.repo.Update(ctx, existing); err != nil {
			return nil, fmt.Errorf("update checklist document %s: %w", id, err)
		}
		return existing, nil
	}

	now := time.Now()
	doc := &document.Document{
		ID:                id,
		TenantID:          in.TenantID,
		Name:              name,
		FileURL:           fileURL,
		FileType:          fileType,
		FileSize:          in.FileSize,
		StorageKey:        in.StorageKey,
		PersistenceStatus: document.PersistenceStored,
		Category:          classification.Category,
		// "pendente": o arquivo chegou pelo cliente e aguarda conferência da
		// imobiliária — o mesmo estado usado nos comprovantes recebidos.
		Status:     document.StatusPendente,
		EntityID:   entityID,
		EntityType: entityType,
		ContractID: in.ContractID,
		Source:     document.SourcePortal,
		UploadedBy: uploadedBy,
		UploadDate: uploadedAt,
		Signers:    []document.DocumentSigner{},
		Tags:       classification.Tags,
		Version:    1,
		Notes:      checklistDocumentNote(in),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.repo.Create(ctx, doc); err != nil {
		return nil, fmt.Errorf("create checklist document %s: %w", id, err)
	}
	return doc, nil
}

// checklistDocumentName compõe o nome exibido no painel: o item da esteira
// primeiro (é o que o operador reconhece) e o nome do arquivo depois.
func checklistDocumentName(in ChecklistDocumentInput) string {
	item := strings.TrimSpace(in.ItemName)
	file := strings.TrimSpace(in.FileName)
	switch {
	case item != "" && file != "" && !strings.EqualFold(item, file):
		return item + " — " + file
	case item != "":
		return item
	default:
		return file
	}
}

// checklistDocumentNote registra a origem e a parte responsável, para o
// operador saber de quem é o documento dentro do painel.
func checklistDocumentNote(in ChecklistDocumentInput) string {
	parts := []string{"Enviado pela esteira digital"}
	if strings.TrimSpace(in.PartyID) != "" {
		parts = append(parts, "parte "+strings.TrimSpace(in.PartyID))
	}
	return strings.Join(parts, " · ")
}

func sameTags(current, want []string) bool {
	if len(current) != len(want) {
		return false
	}
	seen := make(map[string]int, len(current))
	for _, tag := range current {
		seen[tag]++
	}
	for _, tag := range want {
		seen[tag]--
		if seen[tag] < 0 {
			return false
		}
	}
	return true
}

// mimeForChecklistFile cobre o caso em que o upload não informa o content-type.
func mimeForChecklistFile(name string) string {
	switch strings.ToLower(path.Ext(strings.TrimSpace(name))) {
	case ".pdf":
		return "application/pdf"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	case ".heic":
		return "image/heic"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}
