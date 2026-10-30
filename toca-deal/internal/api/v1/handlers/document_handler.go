package handlers

import (
	"bytes"
	stdcontext "context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	pkgauth "github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/pkg/virusscan"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/document"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// DealAccessChecker is the minimal surface DocumentHandler needs to validate
// that a non-admin requester is actually a participant of the deal whose
// documents they want to read. We accept just an interface so tests can stub
// it without standing up a full DealService.
type DealAccessChecker interface {
	GetByID(ctx stdcontext.Context, tenantID, id, brokerID string) (*deal.Deal, error)
}

// ContractScopeChecker valida que o contrato (e o negócio dele) está no escopo
// do chamador. Implementado pelo ContractService (P1/A25).
type ContractScopeChecker interface {
	ContractInBrokerScope(ctx stdcontext.Context, tenantID, contractID, brokerID string) error
}

type DocumentHandler struct {
	service     *service.DocumentService
	dealChecker DealAccessChecker
	// contractScope valida o escopo por corretor na leitura de documentos do
	// contrato (P1/A25). Opcional: sem ele o comportamento é o anterior.
	contractScope ContractScopeChecker
}

// WithContractScope injeta o validador de escopo do contrato.
func (h *DocumentHandler) WithContractScope(checker ContractScopeChecker) *DocumentHandler {
	h.contractScope = checker
	return h
}

func NewDocumentHandler(svc *service.DocumentService, dealChecker DealAccessChecker) *DocumentHandler {
	return &DocumentHandler{service: svc, dealChecker: dealChecker}
}

// canAccessDeal reports whether the requester can read documents tied to the
// given deal. Returns (allowed, error). An error here is *internal* (e.g.
// deal lookup failed) and should yield 500; `false` with nil error means
// authenticated-but-not-party and should yield 403.
func (h *DocumentHandler) canAccessDeal(c *gin.Context, tenantID, dealID string) (bool, error) {
	if h.dealChecker == nil {
		// Defensive: if the handler was constructed without a checker
		// (e.g. legacy test), fall back to allowing — the original
		// behavior. Production wiring always passes a checker.
		return true, nil
	}
	if pkgauth.HasTenantWideScope(
		mustUserPermission(c),
		mustFunctionalRole(c),
		mustBaseRole(c),
	) {
		return true, nil
	}
	accountID, err := context.GetAccountID(c.Request.Context())
	if err != nil || strings.TrimSpace(accountID) == "" {
		// No account_id = nothing to match against; deny.
		return false, nil
	}
	d, err := h.dealChecker.GetByID(c.Request.Context(), tenantID, dealID, "")
	if err != nil || d == nil {
		// Don't 500 on a missing/foreign deal — treat as "not a party".
		// Real DB failures still bubble via the actual list call below.
		return false, nil
	}
	if d.BrokerID == accountID {
		return true, nil
	}
	for _, p := range d.BrokerParticipations {
		if p.BrokerID == accountID {
			return true, nil
		}
	}
	for _, party := range d.Parties {
		if party.AccountID != "" && party.AccountID == accountID {
			return true, nil
		}
		// Some parties may carry only LeadID — match it too in case the
		// portal user's account_id was provisioned from the lead id.
		if party.LeadID != "" && party.LeadID == accountID {
			return true, nil
		}
	}
	for _, signer := range d.Signers {
		if signer.AccountID != "" && signer.AccountID == accountID {
			return true, nil
		}
	}
	return false, nil
}

// ── Request DTOs ──

type CreateDocumentRequest struct {
	Name              string                     `json:"name" binding:"required"`
	Category          document.Category          `json:"category" binding:"required"`
	FileType          string                     `json:"file_type" binding:"required"`
	FileSize          int64                      `json:"file_size"`
	ContractID        string                     `json:"contract_id,omitempty"`
	EntityID          string                     `json:"entity_id,omitempty"`
	EntityType        document.EntityType        `json:"entity_type,omitempty"`
	AnalysisPartyID   string                     `json:"analysis_party_id,omitempty"`
	AnalysisCriterion document.AnalysisCriterion `json:"analysis_criterion,omitempty"`
	Source            document.Source            `json:"source,omitempty"`
	ExpiresAt         *time.Time                 `json:"expires_at,omitempty"`
	Notes             string                     `json:"notes,omitempty"`
	Tags              []string                   `json:"tags,omitempty"`
	Signers           []document.DocumentSigner  `json:"signers,omitempty"`
}

type UpdateDocumentRequest struct {
	Name              string                     `json:"name,omitempty"`
	Category          document.Category          `json:"category,omitempty"`
	Status            document.DocumentStatus    `json:"status,omitempty"`
	EntityID          string                     `json:"entity_id,omitempty"`
	EntityType        document.EntityType        `json:"entity_type,omitempty"`
	AnalysisPartyID   string                     `json:"analysis_party_id,omitempty"`
	AnalysisCriterion document.AnalysisCriterion `json:"analysis_criterion,omitempty"`
	ExpiresAt         *time.Time                 `json:"expires_at,omitempty"`
	Notes             string                     `json:"notes,omitempty"`
	Tags              []string                   `json:"tags,omitempty"`
	Signers           []document.DocumentSigner  `json:"signers,omitempty"`
}

func applyAnalysisReference(d *document.Document, partyID string, criterion document.AnalysisCriterion) error {
	partyID = strings.TrimSpace(partyID)
	if partyID == "" && criterion == "" {
		return nil
	}
	if d.EntityType != document.EntityAnalysis || strings.TrimSpace(d.EntityID) == "" {
		return fmt.Errorf("analysis_party_id and analysis_criterion require an analysis entity link")
	}
	if partyID == "" {
		return fmt.Errorf("analysis_party_id is required when analysis_criterion is set")
	}
	if !criterion.IsValid() {
		return fmt.Errorf("analysis_criterion must be score, cnd, legal_opinion, or income")
	}
	d.AnalysisPartyID = partyID
	d.AnalysisCriterion = criterion
	return nil
}

// ── Create (JSON metadata) ──

func (h *DocumentHandler) Create(c *gin.Context) {
	var req CreateDocumentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	userID, _ := context.GetAccountID(c.Request.Context())
	d := document.New(req.Name, req.Category, req.FileType, req.FileSize, userID, tenantID)

	// Link to entity
	if req.EntityID != "" && req.EntityType != "" {
		d.LinkToEntity(req.EntityID, req.EntityType)
	}
	// Legacy contract_id support
	if req.ContractID != "" {
		d.LinkToContract(req.ContractID)
	}
	if err := applyAnalysisReference(d, req.AnalysisPartyID, req.AnalysisCriterion); err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	if req.Source != "" {
		d.Source = req.Source
	}
	d.ExpiresAt = req.ExpiresAt
	d.Notes = req.Notes
	if req.Tags != nil {
		d.Tags = req.Tags
	}
	if req.Signers != nil {
		d.Signers = req.Signers
	}

	if err := h.service.Create(c.Request.Context(), d); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Document created successfully", d)
}

// ── Upload (multipart file + metadata) ──

func (h *DocumentHandler) Upload(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	userID, _ := context.GetAccountID(c.Request.Context())

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.ValidationError(c, "file is required: "+err.Error())
		return
	}
	defer file.Close()

	// Read optional form fields
	name := c.PostForm("name")
	if name == "" {
		name = header.Filename
	}

	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		response.InternalServerError(c, "failed to read file header")
		return
	}

	ext, contentType, err := storage.ValidateWithMIME("toca-documents", header.Filename, header.Size, head[:n])
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	multiFile := io.MultiReader(bytes.NewReader(head[:n]), file)
	filename := name + ext

	category := document.Category(c.DefaultPostForm("category", string(document.CategoryOutros)))
	contractID := c.PostForm("contract_id")
	entityID := c.PostForm("entity_id")
	entityType := document.EntityType(c.PostForm("entity_type"))
	analysisPartyID := c.PostForm("analysis_party_id")
	analysisCriterion := document.AnalysisCriterion(c.PostForm("analysis_criterion"))
	source := document.Source(c.DefaultPostForm("source", string(document.SourceManual)))
	notes := c.PostForm("notes")

	d := document.New(name, category, contentType, header.Size, userID, tenantID)
	d.Source = source
	d.Notes = notes

	if entityID != "" && entityType != "" {
		d.LinkToEntity(entityID, entityType)
	}
	if contractID != "" {
		d.LinkToContract(contractID)
	}
	if err := applyAnalysisReference(d, analysisPartyID, analysisCriterion); err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	// Upload the file to storage
	if err := h.service.Upload(c.Request.Context(), d, filename, contentType, header.Size, multiFile); err != nil {
		respondUploadError(c, err)
		return
	}

	// Persist document metadata
	if err := h.service.Create(c.Request.Context(), d); err != nil {
		response.InternalError(c, err)
		return
	}

	// Dispara extração OCR+IA assíncrona para documentos de dados
	// (identidade/comprovante). Best-effort — nunca falha o upload.
	h.service.PublishUploaded(c.Request.Context(), d)

	response.Created(c, "Document uploaded successfully", d)
}

// ── OCR / AI extraction ──

// GetExtraction returns the OCR+AI extraction result for a document.
func (h *DocumentHandler) GetExtraction(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	d, err := h.service.GetExtraction(c.Request.Context(), id, tenantID)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "ok", gin.H{
		"document_id":       d.ID,
		"extraction_status": d.ExtractionStatus,
		"extraction":        d.Extraction,
		"extraction_error":  d.ExtractionError,
	})
}

// ReExtract requeues a document for OCR+AI extraction (manual re-run from the UI).
func (h *DocumentHandler) ReExtract(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	if err := h.service.RequeueExtraction(c.Request.Context(), id, tenantID); err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Extração reenfileirada", gin.H{
		"document_id":       id,
		"extraction_status": document.ExtractionPending,
	})
}

// ── Upload to existing document ──

func (h *DocumentHandler) UploadToExisting(c *gin.Context) {
	id := c.Param("id")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.ValidationError(c, "file is required: "+err.Error())
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		response.InternalServerError(c, "failed to read file header")
		return
	}

	_, contentType, err := storage.ValidateWithMIME("toca-documents", header.Filename, header.Size, head[:n])
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	multiFile := io.MultiReader(bytes.NewReader(head[:n]), file)
	filename := header.Filename

	if err := h.service.Upload(c.Request.Context(), existing, filename, contentType, header.Size, multiFile); err != nil {
		respondUploadError(c, err)
		return
	}

	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "File uploaded to document successfully", existing)
}

// GetContentInternal serves the raw bytes of a Document over the X-Internal-Key
// channel — used by toca-signature to attach a stored document to a Clicksign
// envelope. Only application/pdf is accepted; everything else returns 415 so
// the calling pipeline fails fast instead of uploading an unsignable file.
//
// Tenant scoping: requires `tenant_id` query param matching the document's
// tenant. Internal routes already gate by X-Internal-Key, so we don't need a
// JWT — but mismatched tenant_id is still a hard 403 to keep the cross-tenant
// boundary watertight.
func (h *DocumentHandler) GetContentInternal(c *gin.Context) {
	id := c.Param("id")
	// A3 (reauditoria de 2026-09-20): esta rota devolve os BYTES do documento e
	// recebia o tenant cru do `?tenant_id=` — uma chave nomeada com allowlist do
	// tenant A baixava documento do tenant B. Resolvedor único.
	tenantID, err := auth.ResolveTenantForInternalRequest(c, c.Query("tenant_id"))
	if err != nil {
		switch {
		case errors.Is(err, auth.ErrInternalCrossTenant):
			response.Forbidden(c, err.Error())
		case errors.Is(err, auth.ErrInternalTenantRequired):
			response.ValidationError(c, err.Error())
		default:
			response.Unauthorized(c, err.Error())
		}
		return
	}

	doc, content, err := h.service.GetFileContent(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if doc.TenantID != tenantID {
		response.Forbidden(c, "tenant mismatch")
		return
	}
	if doc.FileType != "" && doc.FileType != "application/pdf" {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{
			"error":     "document is not a PDF",
			"file_type": doc.FileType,
		})
		return
	}

	filename := doc.Name
	if filename == "" {
		filename = id + ".pdf"
	}
	if filepath.Ext(filename) == "" {
		filename += ".pdf"
	}

	c.Header("Content-Type", "application/pdf")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", content)
}

// ── Download (redirect to file URL) ──

func respondDocumentDownloadError(c *gin.Context, err error) {
	message := err.Error()
	switch {
	case strings.Contains(message, "access denied"):
		response.Forbidden(c, message)
	case strings.Contains(message, "document not found"),
		strings.Contains(message, "document has no file uploaded"):
		response.NotFound(c, message)
	default:
		response.InternalServerError(c, message)
	}
}

func (h *DocumentHandler) Download(c *gin.Context) {
	id := c.Param("id")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	url, err := h.service.GetDownloadURL(c.Request.Context(), id, tenantID)
	if err != nil {
		respondDocumentDownloadError(c, err)
		return
	}

	// B8: storage local servido pelo endpoint AUTENTICADO (o mount estático
	// /uploads foi removido); fora do storage local cai no redirect abaixo.
	if serveLocalUploadIfPresent(c, url) {
		return
	}

	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *DocumentHandler) GetDownloadURL(c *gin.Context) {
	id := c.Param("id")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	url, err := h.service.GetDownloadURL(c.Request.Context(), id, tenantID)
	if err != nil {
		respondDocumentDownloadError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"url":        url,
		"expires_at": time.Now().Add(15 * time.Minute).Unix(),
	})
}

// ── Get by ID ──

func (h *DocumentHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	// SECURITY (achado C11): o documento é resolvido dentro do tenant do
	// chamador. Antes, esta rota (a única da família sem checagem) devolvia
	// metadados de documentos de qualquer tenant pelo UUID.
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	d, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Document retrieved successfully", d)
}

// ── Update metadata ──

func (h *DocumentHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateDocumentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Category != "" {
		existing.Category = req.Category
	}
	if req.Status != "" {
		existing.Status = req.Status
	}
	if req.EntityID != "" && req.EntityType != "" {
		existing.LinkToEntity(req.EntityID, req.EntityType)
	}
	if req.AnalysisPartyID != "" || req.AnalysisCriterion != "" {
		if err := applyAnalysisReference(existing, req.AnalysisPartyID, req.AnalysisCriterion); err != nil {
			response.ValidationError(c, err.Error())
			return
		}
	}
	if req.ExpiresAt != nil {
		existing.ExpiresAt = req.ExpiresAt
	}
	if req.Notes != "" {
		existing.Notes = req.Notes
	}
	if req.Tags != nil {
		existing.Tags = req.Tags
	}
	if req.Signers != nil {
		existing.Signers = req.Signers
	}

	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Document updated successfully", existing)
}

// ── Delete ──

func (h *DocumentHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id, tenantID); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Document deleted successfully", nil)
}

// ── List with filters ──

func (h *DocumentHandler) List(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	contractID := c.Query("contract_id")
	entityID := c.Query("entity_id")
	entityType := c.Query("entity_type")

	// Shortcut: list by contract (backward compat)
	if contractID != "" && entityID == "" {
		docs, err := h.service.ListByContract(c.Request.Context(), contractID, tenantID)
		if err != nil {
			if isNotImplemented(err) {
				response.Success(c, "Documents retrieved successfully", []*document.Document{})
				return
			}
			response.InternalError(c, err)
			return
		}
		response.Success(c, "Documents retrieved successfully", docs)
		return
	}

	// Shortcut: list by entity
	if entityID != "" && entityType != "" {
		// Defense in depth: when listing documents of a specific deal, make
		// sure the requester is privileged (admin/owner/manager) or actually
		// a party/broker of that deal — otherwise any role with `documents:
		// read` could read any deal's documents in their tenant.
		if document.EntityType(entityType) == document.EntityDeal {
			allowed, _ := h.canAccessDeal(c, tenantID, entityID)
			if !allowed {
				response.Forbidden(c, "you are not a party of this deal")
				return
			}
		}
		docs, err := h.service.ListByEntity(c.Request.Context(), entityID, document.EntityType(entityType), tenantID)
		if err != nil {
			if isNotImplemented(err) {
				response.Success(c, "Documents retrieved successfully", []*document.Document{})
				return
			}
			response.InternalError(c, err)
			return
		}
		response.Success(c, "Documents retrieved successfully", docs)
		return
	}

	// Full list with filters
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	filters := document.ListFilters{
		ContractID: contractID,
		EntityID:   entityID,
		Category:   document.Category(c.Query("category")),
		Status:     document.DocumentStatus(c.Query("status")),
		Source:     document.Source(c.Query("source")),
		Q:          strings.TrimSpace(c.Query("q")),
		Limit:      limit,
		Offset:     offset,
	}
	if entityType != "" {
		filters.EntityType = document.EntityType(entityType)
	}
	// tags=foo,bar OR repeated tag=foo&tag=bar
	rawTags := c.Query("tags")
	if rawTags != "" {
		for _, t := range strings.Split(rawTags, ",") {
			if v := strings.TrimSpace(t); v != "" {
				filters.Tags = append(filters.Tags, v)
			}
		}
	}
	for _, t := range c.QueryArray("tag") {
		if v := strings.TrimSpace(t); v != "" {
			filters.Tags = append(filters.Tags, v)
		}
	}

	// Same gate applies when /documents is queried with a deal filter via
	// the generic listing path — protects e.g. ?entity_type=deal&entity_id=X.
	if filters.EntityType == document.EntityDeal && filters.EntityID != "" {
		allowed, _ := h.canAccessDeal(c, tenantID, filters.EntityID)
		if !allowed {
			response.Forbidden(c, "you are not a party of this deal")
			return
		}
	}

	docs, total, err := h.service.List(c.Request.Context(), tenantID, filters)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Documents retrieved successfully", []*document.Document{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Documents retrieved successfully", docs, page, int(limit), total)
}

// ── Known tags ──

// KnownTags returns the alphabetical set of tag values currently in use by
// the tenant. Drives the tag autocomplete in the Central de Documentos and
// the "Pastas relevantes" sidebar's tag-folder section.
func (h *DocumentHandler) KnownTags(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	tags, err := h.service.ListKnownTags(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Tags retrieved successfully", tags)
}

// ── Search ──

func (h *DocumentHandler) Search(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	query := c.Query("q")
	if query == "" {
		response.ValidationError(c, "query parameter 'q' is required")
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	docs, total, err := h.service.Search(c.Request.Context(), tenantID, query, limit, offset)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Documents found", docs, page, int(limit), total)
}

// ── Create new version ──

func (h *DocumentHandler) CreateVersion(c *gin.Context) {
	id := c.Param("id")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	userID, _ := context.GetAccountID(c.Request.Context())

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.ValidationError(c, "file is required: "+err.Error())
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		response.InternalServerError(c, "failed to read file header")
		return
	}

	_, contentType, err := storage.ValidateWithMIME("toca-documents", header.Filename, header.Size, head[:n])
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	multiFile := io.MultiReader(bytes.NewReader(head[:n]), file)

	newDoc, err := h.service.CreateVersion(c.Request.Context(), id, tenantID, header.Filename, contentType, header.Size, multiFile, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "New document version created", newDoc)
}

// ── List by entity (convenience endpoint) ──

// ListByContract is the convenience endpoint `GET /contracts/:id/documents`.
// Returns every Document linked to the contract via either the legacy
// ContractID field or the entity_id/entity_type pair (laudos, comprovantes,
// recibos created by FASE 3 subscribers).
func (h *DocumentHandler) ListByContract(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	contractID := c.Param("id")
	if strings.TrimSpace(contractID) == "" {
		response.ValidationError(c, "contract id is required")
		return
	}
	// P1/A25: documentos de um contrato são lidos no escopo do chamador — um
	// corretor não lê documento de contrato de outro corretor.
	if h.contractScope != nil {
		if err := h.contractScope.ContractInBrokerScope(c.Request.Context(), tenantID, contractID, callerBrokerScope(c)); err != nil {
			response.NotFound(c, "contract not found")
			return
		}
	}

	docs, err := h.service.ListByContract(c.Request.Context(), contractID, tenantID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Documents retrieved successfully", []*document.Document{})
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Documents retrieved successfully", docs)
}

// ListByDeal is the convenience endpoint `GET /deals/:id/documents`. Internally
// it reuses the same party-aware gate + service.ListByEntity as `?entity_type=
// deal&entity_id=:id`, but the dedicated path is friendlier for clients and
// keeps Swagger docs scoped under /deals.
func (h *DocumentHandler) ListByDeal(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	dealID := c.Param("id")
	if strings.TrimSpace(dealID) == "" {
		response.ValidationError(c, "deal id is required")
		return
	}

	allowed, _ := h.canAccessDeal(c, tenantID, dealID)
	if !allowed {
		response.Forbidden(c, "you are not a party of this deal")
		return
	}

	// Includes contract-linked docs (signed PDFs, attachments) — without
	// this the Esteira Documentation tab would miss the assinado PDF
	// that ships via Clicksign and only surface in ContractTab.
	docs, err := h.service.ListByDealIncludingContracts(c.Request.Context(), tenantID, dealID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Documents retrieved successfully", []*document.Document{})
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Documents retrieved successfully", docs)
}

func (h *DocumentHandler) ListByEntity(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	entityType := document.EntityType(c.Param("entityType"))
	entityID := c.Param("entityId")

	// Same party-aware gate as the generic List path: a non-privileged user
	// must be a party/broker/signer of the deal to see its documents.
	if entityType == document.EntityDeal {
		allowed, _ := h.canAccessDeal(c, tenantID, entityID)
		if !allowed {
			response.Forbidden(c, "you are not a party of this deal")
			return
		}
	}

	docs, err := h.service.ListByEntity(c.Request.Context(), entityID, entityType, tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Documents retrieved successfully", docs)
}

// BackfillSignedPDFs godoc
// @Summary      Re-run the signed-PDF persistence pipeline for stuck rows
// @Description  Targets contract-linked documents in `status=assinado` whose
//
//	bytes aren't durably stored (pending/failed). Idempotent —
//	successful rows stop matching on the next run.
//
// @Tags         documents
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=service.BackfillStats}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /documents/backfill-signed [post]
func (h *DocumentHandler) BackfillSignedPDFs(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	stats, err := h.service.BackfillUnstoredSignedPDFs(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Backfill completed", stats)
}

// respondUploadError mapeia os erros tipados do funil de upload (P1-13):
// vírus detectado → 422 (arquivo rejeitado, estado auditável no documento);
// scanner indisponível → 503 (falha de infraestrutura, operador tenta de novo).
func respondUploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, virusscan.ErrVirusFound):
		response.Error(c, http.StatusUnprocessableEntity, "Arquivo rejeitado pela verificação de segurança.")
	case errors.Is(err, virusscan.ErrFileTooLarge):
		response.Error(c, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Arquivo acima do limite de %d MB.", virusscan.MaxUploadBytes>>20))
	case errors.Is(err, virusscan.ErrScannerUnavailable):
		// Infraestrutura: a verificação está fora, o arquivo não foi aceito.
		response.Error(c, http.StatusServiceUnavailable, "Verificação de segurança indisponível. Tente novamente em instantes.")
	default:
		// Um 500 sem rastro foi o que tornou este incidente invisível: o erro
		// real fica no log com o request id que o cliente recebe.
		log.Printf("ERROR: upload failed: %v", err)
		response.InternalError(c, err)
	}
}
