package handlers

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/storage"
)

// ProofUploadHandler é um endpoint genérico que aceita upload e visualização
// autenticada de comprovantes financeiros (payout, charge reconciliation,
// commission). O caller (UI) anexa o link interno ao markPaid do payout /
// commission / charge reconcile-manual.
//
// Storage: bucket "financial-proofs" via pkg/storage.SupabaseStorage.
// LGPD: bucket é privado; o conteúdo só sai pelo endpoint autenticado.
type ProofUploadHandler struct {
	storage  storage.Service
	bucket   string
	eventBus events.EventBus
}

const (
	ProofBucket       = storage.FinancialProofsBucket
	ProofContentRoute = "/api/v1/proofs/content/"
)

// validProofKinds restringe os valores aceitos em ?kind= — espelha as
// entidades cobertas pelo upload. Adicionar uma nova categoria
// (ex.: "inspection") exige cadastrar aqui para a UI evitar usar valor
// fora do conjunto.
var validProofKinds = map[string]bool{
	"payout":      true,
	"charge":      true,
	"commission":  true,
	"transaction": true,
}

var proofExtensionByContentType = map[string]string{
	"application/pdf": ".pdf",
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
}

func NewProofUploadHandler(s storage.Service) *ProofUploadHandler {
	return &ProofUploadHandler{storage: s, bucket: ProofBucket}
}

// WithEventBus liga o barramento para publicar financial.proof_uploaded
// após o upload. Nil-safe: sem bus a publicação vira no-op (testes/dev).
func (h *ProofUploadHandler) WithEventBus(bus events.EventBus) *ProofUploadHandler {
	h.eventBus = bus
	return h
}

type ProofUploadResponse struct {
	URL       string `json:"url"`
	ObjectKey string `json:"object_key"`
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// Upload godoc
// @Summary      Upload financial proof
// @Description  Multipart upload de um comprovante (PDF/imagem). Devolve um link interno que exige autenticação para leitura.
// @Tags         proofs
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file formData file true "Arquivo do comprovante"
// @Param        kind formData string true "payout | charge | commission"
// @Success      201 {object} response.Response{data=handlers.ProofUploadResponse}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /proofs [post]
func (h *ProofUploadHandler) Upload(c *gin.Context) {
	if h.storage == nil {
		response.Error(c, http.StatusServiceUnavailable, "proof storage is not configured")
		return
	}

	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	kind := strings.ToLower(strings.TrimSpace(c.PostForm("kind")))
	if !validProofKinds[kind] {
		response.ValidationError(c, "invalid kind; expected payout|charge|commission")
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		response.ValidationError(c, "file is required")
		return
	}
	if file.Size > 20*1024*1024 {
		response.ValidationError(c, "file exceeds 20MB limit")
		return
	}
	f, err := file.Open()
	if err != nil {
		respondInternalError(c, err)
		return
	}
	defer f.Close()

	// Nao confia no Content-Type informado pelo navegador. Detectar os bytes
	// impede que um executavel renomeado para .pdf seja persistido no bucket.
	header := make([]byte, 512)
	n, readErr := f.Read(header)
	if readErr != nil && readErr != io.EOF {
		respondInternalError(c, readErr)
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		respondInternalError(c, err)
		return
	}
	contentType := http.DetectContentType(header[:n])
	ext, allowed := proofExtensionByContentType[contentType]
	if !allowed {
		response.ValidationError(c, "file must be PDF, JPG or PNG")
		return
	}
	// Object path layout: <tenant>/<kind>/<yyyy-mm>/<uuid><ext>
	// Mantém arquivos isolados por tenant + facilita TTL/cleanup por mês.
	now := time.Now().UTC()
	objName := fmt.Sprintf("%s/%s/%s/%s%s",
		tenantID, kind, now.Format("2006-01"), uuid.NewString(), ext)

	if _, err := h.storage.Upload(ctx, h.bucket, objName, contentType, f); err != nil {
		logger.ErrorContext(ctx, "financial proof upload failed",
			"bucket", h.bucket,
			"kind", kind,
			"error", err.Error(),
		)
		response.Error(c, http.StatusServiceUnavailable, "proof storage temporarily unavailable")
		return
	}

	audit.Record(ctx, audit.Entry{
		Action:     "proof.uploaded",
		EntityType: "FinancialProof",
		EntityID:   objName,
		Metadata: map[string]any{
			"kind":         kind,
			"size_bytes":   file.Size,
			"content_type": contentType,
			"bucket":       h.bucket,
		},
	})

	// Centralização: notifica o toca-deal para materializar o comprovante
	// como Document (origem "financeiro", categoria "comprovante") na Central
	// de Documentos. Best-effort — uma falha de publish não desfaz o upload.
	if h.eventBus != nil {
		actorID, _ := context.GetAccountID(ctx)
		evt := contracts.FinancialProofUploadedEvent{
			EventIDVal: events.DeriveNotificationID(objName, "financial", "proof"),
			Data: contracts.FinancialProofUploadedEventPayload{
				TenantID:    tenantID,
				Kind:        kind,
				URL:         proofContentURL(objName),
				Filename:    file.Filename,
				ContentType: contentType,
				SizeBytes:   file.Size,
				ObjectKey:   objName,
				UploadedBy:  actorID,
			},
		}
		if err := h.eventBus.PublishReliable(ctx, evt); err != nil {
			logger.ErrorContext(ctx, "financial proof uploaded but document materialization event failed",
				"object_key", objName,
				"error", err.Error(),
			)
		}
	}

	response.Created(c, "Proof uploaded successfully", ProofUploadResponse{
		URL:       proofContentURL(objName),
		ObjectKey: objName,
		Filename:  file.Filename,
		SizeBytes: file.Size,
	})
}

// View godoc
// @Summary      View financial proof
// @Description  Streams a private proof after authenticating and authorizing the current tenant.
// @Tags         proofs
// @Produce      application/pdf
// @Produce      image/jpeg
// @Produce      image/png
// @Security     BearerAuth
// @Param        token path string true "Opaque proof content token"
// @Success      200 {file} binary
// @Failure      401 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      404 {object} response.Response
// @Router       /proofs/content/{token} [get]
// View streams a private proof only after the API authentication and
// finance:read middleware have accepted the caller. The browser receives no
// storage URL or storage credential, so every read is re-authorized by the
// application.
func (h *ProofUploadHandler) View(c *gin.Context) {
	if h.storage == nil {
		response.Error(c, http.StatusServiceUnavailable, "proof storage is not configured")
		return
	}

	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	objectKey, contentType, ok := decodeProofObjectToken(c.Param("token"), tenantID)
	if !ok {
		// Do not disclose whether another tenant's object exists.
		response.NotFound(c, "proof not found")
		return
	}

	downloader, ok := h.storage.(storage.DownloadService)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "proof download is not supported")
		return
	}
	body, _, size, err := downloader.Download(ctx, h.bucket, objectKey)
	if err != nil {
		logger.ErrorContext(ctx, "financial proof download failed",
			"bucket", h.bucket,
			"tenant_id", tenantID,
			"error", err.Error(),
		)
		response.NotFound(c, "proof not found")
		return
	}
	defer body.Close()

	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `inline; filename="comprovante`+proofExtension(objectKey)+`"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.DataFromReader(http.StatusOK, size, contentType, body, nil)
}

func proofContentURL(objectKey string) string {
	return ProofContentRoute + base64.RawURLEncoding.EncodeToString([]byte(objectKey))
}

func decodeProofObjectToken(token, tenantID string) (string, string, bool) {
	decodedKey, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(token, "/"))
	if err != nil {
		return "", "", false
	}
	objectKey := string(decodedKey)
	if strings.ContainsAny(objectKey, "\\\x00") {
		return "", "", false
	}
	parts := strings.Split(objectKey, "/")
	if len(parts) != 4 || parts[0] != tenantID || !validProofKinds[parts[1]] {
		return "", "", false
	}
	if _, err := time.Parse("2006-01", parts[2]); err != nil {
		return "", "", false
	}

	name := parts[3]
	dot := strings.LastIndexByte(name, '.')
	if dot <= 0 {
		return "", "", false
	}
	if _, err := uuid.Parse(name[:dot]); err != nil {
		return "", "", false
	}
	contentType, ok := proofContentTypeByExtension(name[dot:])
	if !ok {
		return "", "", false
	}
	return objectKey, contentType, true
}

func proofContentTypeByExtension(ext string) (string, bool) {
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf", true
	case ".jpg", ".jpeg":
		return "image/jpeg", true
	case ".png":
		return "image/png", true
	default:
		return "", false
	}
}

func proofExtension(objectKey string) string {
	name := objectKey[strings.LastIndexByte(objectKey, '/')+1:]
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return ".bin"
	}
	ext := strings.ToLower(name[dot:])
	if _, ok := proofContentTypeByExtension(ext); ok {
		return ext
	}
	return ".bin"
}
