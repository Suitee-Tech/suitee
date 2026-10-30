package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-signature/internal/domain/envelope"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// EnvelopeHandler exposes endpoints over the local SignatureEnvelope state.
// Reads come from the repository directly; writes (cancel, resend) go through
// EnvelopeService so the Clicksign side and local state stay in sync.
type EnvelopeHandler struct {
	repo envelope.Repository
	svc  *service.EnvelopeService
}

// MaxAttachments caps how many extra documents (deal-stored + inline) can
// ride on a single envelope. Clicksign accepts more, but 10 is enough for
// every contract type today and keeps a single envelope from growing into
// an unreviewable artifact.
const MaxAttachments = 10

// MaxInlineAttachmentBytes caps an inline PDF upload at 20 MB — Clicksign's
// own per-document limit at the time of writing. Validated after base64 decode.
const MaxInlineAttachmentBytes = 20 * 1024 * 1024

type CreateEnvelopeForContractRequest struct {
	SignerAuth        map[string][]string   `json:"signer_auth,omitempty"`
	SignerGroups      map[string]int        `json:"signer_groups,omitempty"`
	AttachmentDocIDs  []string              `json:"attachment_document_ids,omitempty"`
	InlineAttachments []InlineAttachmentDTO `json:"inline_attachments,omitempty"`
	Locale            string                `json:"locale,omitempty"`
	Deadline          string                `json:"deadline,omitempty"`
}

// InlineAttachmentDTO carries an ad-hoc PDF upload from the operator's UI.
// The base64 payload comes WITHOUT the `data:application/pdf;base64,` prefix
// — the handler decodes and validates the size against MaxInlineAttachmentBytes.
type InlineAttachmentDTO struct {
	Filename      string `json:"filename" binding:"required"`
	ContentBase64 string `json:"content_base64" binding:"required"`
}

func NewEnvelopeHandler(repo envelope.Repository, svc *service.EnvelopeService) *EnvelopeHandler {
	return &EnvelopeHandler{repo: repo, svc: svc}
}

// List godoc
// @Summary  List signature envelopes
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Success  200 {object} response.Response
// @Router   /signature/envelopes [get]
func (h *EnvelopeHandler) List(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	envs, err := h.repo.ListByTenant(c.Request.Context(), tenantID, nil)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Envelopes retrieved successfully", envs)
}

// GetByID godoc
// @Summary  Get a signature envelope by id
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Param    id path string true "Envelope ID"
// @Success  200 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id} [get]
func (h *EnvelopeHandler) GetByID(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	env, err := h.repo.GetByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if env == nil {
		response.NotFound(c, "envelope not found")
		return
	}
	response.Success(c, "Envelope retrieved successfully", env)
}

// GetByContract godoc
// @Summary  Get the active envelope for a contract
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Param    contractId path string true "Contract ID"
// @Success  200 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/by-contract/{contractId} [get]
func (h *EnvelopeHandler) GetByContract(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	env, err := h.repo.GetByContractID(c.Request.Context(), tenantID, c.Param("contractId"))
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if env == nil {
		response.NotFound(c, "envelope not found for contract")
		return
	}
	response.Success(c, "Envelope retrieved successfully", env)
}

// Cancel godoc
// @Summary  Cancel an active signature envelope
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Param    id path string true "Envelope ID"
// @Success  200 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id}/cancel [post]
func (h *EnvelopeHandler) Cancel(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	env, err := h.svc.Cancel(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		if errors.Is(err, service.ErrEnvelopeNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		if errors.Is(err, service.ErrCancelProviderFailed) {
			// 502 (e não 500): o nosso lado está saudável, quem recusou foi o
			// provedor. A mensagem precisa chegar ao operador — é ela que diz
			// que o link de assinatura continua válido.
			response.Error(c, http.StatusBadGateway, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Envelope canceled", env)
}

// EditSignerRequest is the payload for PATCH /envelopes/:id/signers/:signerId.
// Auth methods are intentionally not editable here — that requires also
// rewriting Clicksign requirements and is out of scope for the MVP.
type EditSignerRequest struct {
	Name          string `json:"name" binding:"required"`
	Email         string `json:"email" binding:"required,email"`
	PhoneNumber   string `json:"phone_number,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	Birthday      string `json:"birthday,omitempty"`
}

// EditSigner godoc
// @Summary  Edit a signer that hasn't acted yet
// @Description Updates name/email/phone/documentation on Clicksign + locally.
// @Description Rejected if envelope is terminal or signer already
// @Description signed/refused.
// @Tags     signature
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id path string true "Envelope ID"
// @Param    signerId path string true "Internal Signer ID"
// @Param    request body handlers.EditSignerRequest true "Signer fields"
// @Success  200 {object} response.Response
// @Failure  400 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id}/signers/{signerId} [patch]
func (h *EnvelopeHandler) EditSigner(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req EditSignerRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	env, err := h.svc.EditSigner(c.Request.Context(), tenantID, c.Param("id"), c.Param("signerId"), service.EditSignerInput{
		Name:          req.Name,
		Email:         req.Email,
		PhoneNumber:   req.PhoneNumber,
		Documentation: req.Documentation,
		Birthday:      req.Birthday,
	})
	if err != nil {
		if errors.Is(err, service.ErrEnvelopeNotFound) || errors.Is(err, service.ErrSignerNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Signer updated", env)
}

// AddSignerRequest is the payload for POST /envelopes/:id/signers. The auth
// list maps to Clicksign requirements; defaults to ["email"] when omitted.
type AddSignerRequest struct {
	Name          string   `json:"name" binding:"required"`
	Email         string   `json:"email" binding:"required,email"`
	PhoneNumber   string   `json:"phone_number,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
	Birthday      string   `json:"birthday,omitempty"`
	Role          string   `json:"role,omitempty"`
	Auth          []string `json:"auth,omitempty"`
	Group         int      `json:"group,omitempty"`
}

// AddSigner godoc
// @Summary  Add a signer to an active envelope
// @Description Pushes a new signer + qualification + auth requirements to
// @Description Clicksign and triggers the notification when the envelope is
// @Description already RUNNING. Rejected for terminal envelopes.
// @Tags     signature
// @Security BearerAuth
// @Accept   json
// @Produce  json
// @Param    id path string true "Envelope ID"
// @Param    request body handlers.AddSignerRequest true "Signer fields"
// @Success  200 {object} response.Response
// @Failure  400 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id}/signers [post]
func (h *EnvelopeHandler) AddSigner(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req AddSignerRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	auth := make([]envelope.AuthMethod, 0, len(req.Auth))
	for _, a := range req.Auth {
		auth = append(auth, envelope.AuthMethod(a))
	}
	env, err := h.svc.AddSignerToEnvelope(c.Request.Context(), tenantID, c.Param("id"), service.AddSignerInput{
		Name:          req.Name,
		Email:         req.Email,
		PhoneNumber:   req.PhoneNumber,
		Documentation: req.Documentation,
		Birthday:      req.Birthday,
		Role:          envelope.SignerRole(req.Role),
		Auth:          auth,
		Group:         req.Group,
	})
	if err != nil {
		if errors.Is(err, service.ErrEnvelopeNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Signer added", env)
}

// Reconcile godoc
// @Summary  Reconcile local envelope state with Clicksign
// @Description Force-refresh local state from Clicksign. Useful when webhooks
// @Description were missed (e.g. payload shape regressions, queue outages).
// @Description Republishes the matching domain events so toca-deal catches up.
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Param    id path string true "Envelope ID"
// @Success  200 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id}/reconcile [post]
func (h *EnvelopeHandler) Reconcile(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	env, err := h.svc.Reconcile(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		if errors.Is(err, service.ErrEnvelopeNotFound) {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Envelope reconciled", env)
}

// Resend godoc
// @Summary  Resend signing notification to all or a single signer
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Param    id        path  string true  "Envelope ID"
// @Param    signer_id query string false "Internal signer id; empty resends to every pending signer"
// @Success  200 {object} response.Response
// @Failure  400 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id}/resend [post]
func (h *EnvelopeHandler) Resend(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	signerID := c.Query("signer_id")
	if signerID == "" {
		// Allow body { "signer_id": "..." } as an alternative — convenient
		// for clients that prefer JSON over query strings.
		var body struct {
			SignerID string `json:"signer_id"`
		}
		_ = c.ShouldBindJSON(&body)
		signerID = body.SignerID
	}
	env, err := h.svc.ResendNotification(c.Request.Context(), tenantID, c.Param("id"), signerID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrEnvelopeNotFound):
			response.NotFound(c, err.Error())
		case errors.Is(err, service.ErrSignerNotFound):
			response.NotFound(c, err.Error())
		default:
			// State-conflict: envelope already finished/canceled, signer
			// already signed, etc. Surface as 409.
			response.Conflict(c, err.Error())
		}
		return
	}
	response.Success(c, "Notification resent", env)
}

// ListEvents godoc
// @Summary  List webhook events received for an envelope (audit trail)
// @Tags     signature
// @Security BearerAuth
// @Produce  json
// @Param    id path string true "Envelope ID"
// @Success  200 {object} response.Response
// @Failure  404 {object} response.Response
// @Router   /signature/envelopes/{id}/events [get]
func (h *EnvelopeHandler) ListEvents(c *gin.Context) {
	tenantID, err := pkgctx.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	env, err := h.repo.GetByID(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if env == nil {
		response.NotFound(c, "envelope not found")
		return
	}
	if env.ExternalEnvelopeID == "" {
		response.Success(c, "Envelope has no Clicksign events yet", []envelope.WebhookEventRow{})
		return
	}
	events, err := h.repo.ListWebhookEvents(c.Request.Context(), env.ExternalEnvelopeID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Events retrieved successfully", events)
}

// internalTenant resolve o tenant efetivo de uma rota interna.
//
// SECURITY (achado A3 da revisão de permissões de 2026-09-19): antes o tenant
// vinha cru do `?tenant_id=` do chamador. Agora:
//   - chamada com JWT de usuário → tenant do token (pedir outro é 403);
//   - chamada de serviço → tenant explícito, restrito à allowlist da credencial
//     quando INTERNAL_SERVICE_KEYS define uma.
func internalTenant(c *gin.Context) (string, bool) {
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
		return "", false
	}
	return tenantID, true
}

// GetByContractInternal is the X-Internal-Key variant used by toca-deal
// (SignatureClient.GetByContract).
func (h *EnvelopeHandler) GetByContractInternal(c *gin.Context) {
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	env, err := h.repo.GetByContractID(c.Request.Context(), tenantID, c.Param("contractId"))
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if env == nil {
		response.NotFound(c, "envelope not found for contract")
		return
	}
	response.Success(c, "Envelope retrieved successfully", env)
}

// RefreshSignedURLResponse is the body returned by RefreshSignedDocumentURLInternal —
// kept as its own type so the deal-side SignatureClient can parse it
// with strong typing instead of poking at a generic map.
type RefreshSignedURLResponse struct {
	// SignedDocumentURL is the current Clicksign URL for the signed
	// PDF. Empty when the envelope hasn't produced one yet (signing in
	// progress, or genuine CDN delay).
	SignedDocumentURL string `json:"signed_document_url"`
}

// RefreshSignedDocumentURLInternal hits Clicksign to recover the
// signed-PDF URL for a contract whose local `SignedDocumentURL`
// stayed empty after the original `envelope_finished` webhook.
// Called by the toca-deal backfill (DocumentService.BackfillUnstoredSignedPDFs)
// before it skips a row as "no source URL available".
//
// Returns 404 only when the envelope itself is missing — an envelope
// without a signed document yet returns 200 with `signed_document_url=""`
// so the caller can distinguish "not yet" from "envelope deleted".
func (h *EnvelopeHandler) RefreshSignedDocumentURLInternal(c *gin.Context) {
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	url, err := h.svc.RefreshSignedDocumentURL(c.Request.Context(), tenantID, c.Param("contractId"))
	if err != nil {
		if errors.Is(err, service.ErrEnvelopeNotFound) {
			response.NotFound(c, "envelope not found for contract")
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Signed document URL refreshed", RefreshSignedURLResponse{SignedDocumentURL: url})
}

// CreateForContractInternal is the synchronous X-Internal-Key path used by
// toca-deal immediately after a user sends a contract for Clicksign. The
// EnvelopeService is idempotent, so this returns an existing active envelope
// when RabbitMQ already created one.
func (h *EnvelopeHandler) CreateForContractInternal(c *gin.Context) {
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}

	var req CreateEnvelopeForContractRequest
	_ = c.ShouldBindJSON(&req)

	opts, err := req.toCreateOptions()
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	env, err := h.svc.CreateForContract(c.Request.Context(), tenantID, c.Param("contractId"), opts)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Created(c, "Envelope created successfully", env)
}

// toCreateOptions translates the wire DTO into the service-layer options and
// validates request-shape errors (oversized attachments, malformed base64,
// over-cap counts). Domain rules (signerID must exist, etc.) stay in the
// service.
func (r CreateEnvelopeForContractRequest) toCreateOptions() (service.CreateOptions, error) {
	opts := service.CreateOptions{
		Locale:       r.Locale,
		SignerAuth:   make(map[string][]envelope.AuthMethod, len(r.SignerAuth)),
		SignerGroups: r.SignerGroups,
	}
	for signerID, methods := range r.SignerAuth {
		for _, method := range methods {
			if method != "" {
				opts.SignerAuth[signerID] = append(opts.SignerAuth[signerID], envelope.AuthMethod(method))
			}
		}
	}
	if len(opts.SignerAuth) == 0 {
		opts.SignerAuth = nil
	}
	if r.Deadline != "" {
		deadline, err := time.Parse(time.RFC3339, r.Deadline)
		if err != nil {
			return opts, fmt.Errorf("deadline must be RFC3339")
		}
		opts.Deadline = &deadline
	}

	for signerID, group := range r.SignerGroups {
		if group < 0 {
			return opts, fmt.Errorf("signer %s group must be >= 0", signerID)
		}
	}

	if len(r.AttachmentDocIDs)+len(r.InlineAttachments) > MaxAttachments {
		return opts, fmt.Errorf("envelope supports at most %d attachments", MaxAttachments)
	}
	if len(r.AttachmentDocIDs) > 0 {
		opts.AttachmentDocIDs = make([]string, 0, len(r.AttachmentDocIDs))
		for _, id := range r.AttachmentDocIDs {
			id = strings.TrimSpace(id)
			if id == "" {
				return opts, fmt.Errorf("attachment_document_ids contains empty id")
			}
			opts.AttachmentDocIDs = append(opts.AttachmentDocIDs, id)
		}
	}
	if len(r.InlineAttachments) > 0 {
		opts.InlineAttachments = make([]service.InlineAttachment, 0, len(r.InlineAttachments))
		for i, in := range r.InlineAttachments {
			filename := strings.TrimSpace(in.Filename)
			if filename == "" {
				return opts, fmt.Errorf("inline_attachments[%d].filename is required", i)
			}
			if ext := strings.ToLower(filepath.Ext(filename)); ext != ".pdf" {
				return opts, fmt.Errorf("inline_attachments[%d] must be a .pdf file", i)
			}
			b64 := strings.TrimSpace(in.ContentBase64)
			if b64 == "" {
				return opts, fmt.Errorf("inline_attachments[%d].content_base64 is required", i)
			}
			if comma := strings.Index(b64, ","); comma >= 0 && strings.HasPrefix(b64, "data:") {
				b64 = b64[comma+1:]
			}
			content, err := base64.StdEncoding.DecodeString(b64)
			if err != nil {
				return opts, fmt.Errorf("inline_attachments[%d] invalid base64: %v", i, err)
			}
			if len(content) > MaxInlineAttachmentBytes {
				return opts, fmt.Errorf("inline_attachments[%d] exceeds %d bytes", i, MaxInlineAttachmentBytes)
			}
			opts.InlineAttachments = append(opts.InlineAttachments, service.InlineAttachment{
				Filename: filename,
				Content:  content,
			})
		}
	}
	return opts, nil
}
