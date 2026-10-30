package document

import (
	"time"

	"github.com/google/uuid"
)

// ── Categories ──

type Category string

const (
	CategoryContrato    Category = "contrato"
	CategoryAditivo     Category = "aditivo"
	CategoryLaudo       Category = "laudo"
	CategoryVistoria    Category = "vistoria"
	CategoryProcuracao  Category = "procuracao"
	CategoryRecibo      Category = "recibo"
	CategoryNotificacao Category = "notificacao"
	CategoryFinanceiro  Category = "financeiro"
	CategoryJuridico    Category = "juridico"
	CategoryFiscal      Category = "fiscal"
	CategoryIdentidade  Category = "identidade"
	CategoryComprovante Category = "comprovante"
	CategoryFoto        Category = "foto"
	CategoryOutros      Category = "outros"
)

// ── Document Status ──

type DocumentStatus string

const (
	StatusRascunho             DocumentStatus = "rascunho"
	StatusPendente             DocumentStatus = "pendente"
	StatusPendenteAssinatura   DocumentStatus = "pendente_assinatura"
	StatusParcialmenteAssinado DocumentStatus = "parcialmente_assinado"
	StatusAssinado             DocumentStatus = "assinado"
	StatusExpirado             DocumentStatus = "expirado"
	StatusCancelado            DocumentStatus = "cancelado"
)

// PersistenceStatus tracks where the document's bytes physically live —
// orthogonal to DocumentStatus, which describes business state. A
// document can be StatusAssinado (legally signed via Clicksign) while
// still PersistencePending (the local Supabase copy is still being
// downloaded by the webhook subscriber). The UI uses this to show
// "Aguardando download da Clicksign" instead of falsely promising the
// local file is ready.
//
// See `docs/domain/documents-status-mapping.md` for the full lifecycle.
type PersistenceStatus string

const (
	// PersistencePending is the default — either nothing has been
	// uploaded yet (rascunho) or the signed PDF is still being fetched
	// from Clicksign. Surfaces in the UI as "aguardando arquivo".
	PersistencePending PersistenceStatus = "pending"
	// PersistenceStored means the bytes are durably persisted in the
	// `toca-documents` bucket and the FileHash field is the SHA-256 of
	// what landed. This is the only state in which audit-grade
	// integrity verification is meaningful.
	PersistenceStored PersistenceStatus = "stored"
	// PersistenceFailed means the persist pipeline ran but did not
	// succeed (download error from Clicksign, storage outage). Backfill
	// jobs target rows in this state — see DocumentService.BackfillUnstoredSignedPDFs.
	PersistenceFailed PersistenceStatus = "failed"
)

// AllDocumentStatuses is the canonical, ordered list of statuses the
// document layer understands. Mirrors `DOCUMENT_STATUSES` in
// `toka/src/app/lib/documents.ts` and the table in
// `docs/domain/documents-status-mapping.md` — update all three
// simultaneously when adding/removing a value. The parity test in
// mapping_test.go pins the expected set.
var AllDocumentStatuses = []DocumentStatus{
	StatusRascunho,
	StatusPendente,
	StatusPendenteAssinatura,
	StatusParcialmenteAssinado,
	StatusAssinado,
	StatusExpirado,
	StatusCancelado,
}

// AllCategories is the canonical, ordered list of categories. Mirrors
// `DOCUMENT_CATEGORIES` in `toka/src/app/lib/documents.ts`.
var AllCategories = []Category{
	CategoryContrato,
	CategoryAditivo,
	CategoryLaudo,
	CategoryVistoria,
	CategoryProcuracao,
	CategoryRecibo,
	CategoryNotificacao,
	CategoryFinanceiro,
	CategoryJuridico,
	CategoryFiscal,
	CategoryIdentidade,
	CategoryComprovante,
	CategoryFoto,
	CategoryOutros,
}

// ── Signer ──

type SignerStatus string

const (
	SignerPendente SignerStatus = "pendente"
	SignerAssinado SignerStatus = "assinado"
	SignerRecusado SignerStatus = "recusado"
)

type DocumentSigner struct {
	ID       string       `json:"id" bson:"id"`
	Name     string       `json:"name" bson:"name"`
	Email    string       `json:"email" bson:"email"`
	Role     string       `json:"role" bson:"role"`
	Status   SignerStatus `json:"status" bson:"status"`
	SignedAt *time.Time   `json:"signed_at,omitempty" bson:"signed_at,omitempty"`
}

// ── Entity Types (what the document is linked to) ──

type EntityType string

const (
	EntityContract   EntityType = "contract"
	EntityDeal       EntityType = "deal"
	EntityProperty   EntityType = "property"
	EntityLead       EntityType = "lead"
	EntityInspection EntityType = "inspection"
	EntityFiscal     EntityType = "fiscal"
	EntityChecklist  EntityType = "checklist"
	// EntityAnalysis liga o documento à Due Diligence de um negócio
	// (comprovantes que validam a análise agregada: relatório Serasa/SPC,
	// certidão CND, parecer jurídico assinado etc.).
	EntityAnalysis EntityType = "analysis"
	// EntityFinancial liga o documento a um comprovante financeiro
	// (payout/charge/commission/transaction). Materializado a partir do
	// evento financial.proof_uploaded; sem entity_id específico, os
	// comprovantes se agrupam na origem "financeiro" da Central.
	EntityFinancial EntityType = "financial"
	EntityGeneral   EntityType = "general"
)

// AnalysisCriterion identifies the Due Diligence question supported by an
// analysis document. Keeping this relation in the document metadata avoids
// encoding party IDs in filenames or user-facing tags while preserving the
// deal-level entity link used by the Central de Documentos.
type AnalysisCriterion string

const (
	AnalysisCriterionScore        AnalysisCriterion = "score"
	AnalysisCriterionCND          AnalysisCriterion = "cnd"
	AnalysisCriterionLegalOpinion AnalysisCriterion = "legal_opinion"
	AnalysisCriterionIncome       AnalysisCriterion = "income"
)

func (c AnalysisCriterion) IsValid() bool {
	switch c {
	case AnalysisCriterionScore, AnalysisCriterionCND, AnalysisCriterionLegalOpinion, AnalysisCriterionIncome:
		return true
	default:
		return false
	}
}

// ── Source (which service created the document) ──

type Source string

const (
	SourceDeal      Source = "toca-deal"
	SourceCore      Source = "toca-core"
	SourceFinancial Source = "toca-financial"
	SourceOps       Source = "toca-ops"
	SourceComm      Source = "toca-comm"
	SourcePortal    Source = "portal"
	SourceManual    Source = "manual"
)

// ── OCR / AI extraction ──

// ExtractionStatus tracks the async OCR+AI extraction lifecycle of a document.
// Orthogonal to DocumentStatus/PersistenceStatus. Empty means "not applicable"
// (no extraction was ever dispatched for this document).
type ExtractionStatus string

const (
	ExtractionNone       ExtractionStatus = ""
	ExtractionPending    ExtractionStatus = "pending"
	ExtractionProcessing ExtractionStatus = "processing"
	ExtractionCompleted  ExtractionStatus = "completed"
	ExtractionFailed     ExtractionStatus = "failed"
	ExtractionEmpty      ExtractionStatus = "empty"
)

// ExtractedField espelha um campo identificado pela IA (ver toca-ai
// domain.ExtractedField). `Applied` indica que o campo já foi gravado na party
// (auto-aplicado por alta confiança ou validado pelo operador).
type ExtractedField struct {
	Name              string  `json:"name" bson:"name"`
	Label             string  `json:"label,omitempty" bson:"label,omitempty"`
	Target            string  `json:"target,omitempty" bson:"target,omitempty"`
	Value             string  `json:"value" bson:"value"`
	RawValue          string  `json:"raw_value,omitempty" bson:"raw_value,omitempty"`
	Evidence          string  `json:"evidence,omitempty" bson:"evidence,omitempty"`
	Confidence        float64 `json:"confidence" bson:"confidence"`
	Tier              string  `json:"tier" bson:"tier"`
	Source            string  `json:"source" bson:"source"`
	ValidationStatus  string  `json:"validation_status" bson:"validation_status"`
	ValidationMessage string  `json:"validation_message,omitempty" bson:"validation_message,omitempty"`
	Applied           bool    `json:"applied,omitempty" bson:"applied,omitempty"`
}

// DocumentExtraction é o resultado da extração OCR+IA persistido no documento
// (dentro da metadata JSONB — sem migration). Mirror do toca-ai ExtractedDocument.
type DocumentExtraction struct {
	Status            string           `json:"status" bson:"status"`
	Error             string           `json:"error,omitempty" bson:"error,omitempty"`
	Fields            []ExtractedField `json:"fields" bson:"fields"`
	OverallConfidence float64          `json:"overall_confidence" bson:"overall_confidence"`
	Model             string           `json:"model,omitempty" bson:"model,omitempty"`
	OCRProvider       string           `json:"ocr_provider,omitempty" bson:"ocr_provider,omitempty"`
	OCRConfidence     *float64         `json:"ocr_confidence,omitempty" bson:"ocr_confidence,omitempty"`
	AppliedPartyID    string           `json:"applied_party_id,omitempty" bson:"applied_party_id,omitempty"`
	DurationMS        int64            `json:"duration_ms,omitempty" bson:"duration_ms,omitempty"`
	ExtractedAt       time.Time        `json:"extracted_at" bson:"extracted_at"`
}

// ── Document: single source of truth ──

type Document struct {
	ID       string `json:"id" bson:"_id"`
	TenantID string `json:"tenant_id" bson:"tenant_id"`

	// File storage
	Name       string `json:"name" bson:"name"`
	FileURL    string `json:"file_url,omitempty" bson:"file_url,omitempty"`
	FileType   string `json:"file_type" bson:"file_type"`                         // MIME type: application/pdf, image/png, etc.
	FileSize   int64  `json:"file_size" bson:"file_size"`                         // bytes
	StorageKey string `json:"storage_key,omitempty" bson:"storage_key,omitempty"` // internal storage path
	// SHA-256 of the stored bytes, hex-encoded. Computed at upload time
	// (see DocumentService.Upload) so we can prove a signed PDF wasn't
	// silently swapped in storage, and so dedupe checks (e.g. the DOCX
	// idempotency in UploadDocxForContract) have a stable key. Empty for
	// legacy rows uploaded before this field existed — those can be
	// rehashed lazily on first download if integrity verification is ever
	// needed.
	FileHash string `json:"file_hash,omitempty" bson:"file_hash,omitempty"`
	// SignedSourceHash identifica a URL/origem do PDF assinado que gerou
	// StorageKey. Um contrato cancelado e reenviado produz um PDF novo; sem
	// essa identidade o guard de idempotência confundia a tentativa antiga com
	// a final e mantinha o arquivo cancelado. Vazio em linhas antigas: o
	// próximo download/reconciliação baixa a versão atual.
	SignedSourceHash string `json:"signed_source_hash,omitempty" bson:"signed_source_hash,omitempty"`
	// PersistenceStatus reports whether the bytes referenced by
	// StorageKey are actually present in the bucket. Distinct from
	// `Status` (business lifecycle) — a document can be `assinado` but
	// still `PersistencePending` during the brief window between
	// envelope_finished and PersistSignedContractPDF completing.
	PersistenceStatus PersistenceStatus `json:"persistence_status,omitempty" bson:"persistence_status,omitempty"`
	// LastPersistError captures the most recent failure reason from
	// PersistSignedContractPDF so operators and the backfill job have
	// breadcrumbs when investigating stuck documents. Cleared when a
	// later persist succeeds.
	LastPersistError string `json:"last_persist_error,omitempty" bson:"last_persist_error,omitempty"`
	// PersistAttemptedAt is the timestamp of the last persist attempt
	// (successful or not). Lets the backfill job target stale rows
	// older than N minutes without re-scanning everything.
	PersistAttemptedAt *time.Time `json:"persist_attempted_at,omitempty" bson:"persist_attempted_at,omitempty"`

	// Classification
	Category Category       `json:"category" bson:"category"`
	Status   DocumentStatus `json:"status" bson:"status"`

	// Entity linking: allows linking to any entity across services
	EntityID   string     `json:"entity_id,omitempty" bson:"entity_id,omitempty"`
	EntityType EntityType `json:"entity_type,omitempty" bson:"entity_type,omitempty"`
	// AnalysisPartyID + AnalysisCriterion scope a Due Diligence attachment to
	// one party and one question. Both remain empty for general evidence.
	AnalysisPartyID   string            `json:"analysis_party_id,omitempty" bson:"analysis_party_id,omitempty"`
	AnalysisCriterion AnalysisCriterion `json:"analysis_criterion,omitempty" bson:"analysis_criterion,omitempty"`

	// Legacy/compat: direct contract link (kept for backward compat, prefer entity_id+entity_type)
	ContractID string `json:"contract_id,omitempty" bson:"contract_id,omitempty"`

	// Provenance
	Source     Source    `json:"source" bson:"source"`
	UploadedBy string    `json:"uploaded_by" bson:"uploaded_by"`
	UploadDate time.Time `json:"upload_date" bson:"upload_date"`

	// Digital signatures
	Signers   []DocumentSigner `json:"signers" bson:"signers"`
	ExpiresAt *time.Time       `json:"expires_at,omitempty" bson:"expires_at,omitempty"`

	// Metadata
	Notes    string   `json:"notes,omitempty" bson:"notes,omitempty"`
	Tags     []string `json:"tags,omitempty" bson:"tags,omitempty"`
	Version  int      `json:"version" bson:"version"`
	ParentID string   `json:"parent_id,omitempty" bson:"parent_id,omitempty"` // for versioned docs

	// OCR / AI extraction — preenchido assincronamente após o upload de
	// documentos de dados (identidade/comprovante). Ver document_extraction_handler.go.
	ExtractionStatus    ExtractionStatus    `json:"extraction_status,omitempty" bson:"extraction_status,omitempty"`
	Extraction          *DocumentExtraction `json:"extraction,omitempty" bson:"extraction,omitempty"`
	ExtractionError     string              `json:"extraction_error,omitempty" bson:"extraction_error,omitempty"`
	ExtractionUpdatedAt *time.Time          `json:"extraction_updated_at,omitempty" bson:"extraction_updated_at,omitempty"`

	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// New creates a new Document as single source of truth.
// EntityID + EntityType link it to any entity across all microservices.
func New(name string, category Category, fileType string, fileSize int64, uploadedBy, tenantID string) *Document {
	now := time.Now()
	return &Document{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		Name:       name,
		Category:   category,
		FileType:   fileType,
		FileSize:   fileSize,
		Status:     StatusPendente,
		Source:     SourceManual,
		UploadedBy: uploadedBy,
		UploadDate: now,
		Signers:    []DocumentSigner{},
		Tags:       []string{},
		Version:    1,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// LinkToEntity sets the entity association for the document
func (d *Document) LinkToEntity(entityID string, entityType EntityType) {
	d.EntityID = entityID
	d.EntityType = entityType
	d.UpdatedAt = time.Now()
}

// LinkToContract sets the contract association (backward compat + entity link)
func (d *Document) LinkToContract(contractID string) {
	d.ContractID = contractID
	d.EntityID = contractID
	d.EntityType = EntityContract
	d.UpdatedAt = time.Now()
}
