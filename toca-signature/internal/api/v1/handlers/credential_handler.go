package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
)

// CredentialHandler expõe a gestão das credenciais Clicksign POR TENANT
// (P1-14): PUT grava/rotaciona (segredos criptografados em repouso),
// GET devolve o estado mascarado (nunca segredos) e GET /rotations a
// trilha de rotação. Guarda: settings:manage_integrations (admin).
type CredentialHandler struct {
	provider *credential.DBCredentialProvider
}

func NewCredentialHandler(provider *credential.DBCredentialProvider) *CredentialHandler {
	return &CredentialHandler{provider: provider}
}

// StoreCredentialRequest é o corpo do PUT — os segredos NUNCA são ecoados
// de volta.
type StoreCredentialRequest struct {
	BaseURL       string `json:"base_url"`
	AccessToken   string `json:"access_token" binding:"required"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
	Environment   string `json:"environment,omitempty"`
	AuthScheme    string `json:"auth_scheme,omitempty"`
	Note          string `json:"note,omitempty"`
}

// StoreCredential godoc
// @Summary      Store/rotate tenant Clicksign credentials
// @Description  Grava (ou rotaciona) a credencial Clicksign do tenant, criptografada em repouso; registra a rotação na trilha. Secrets nunca são devolvidos.
// @Tags         signature-admin
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.StoreCredentialRequest true "Credential payload"
// @Success      200 {object} response.Response{data=handlers.StoreCredentialResponse}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /signature/admin/clicksign-credentials [put]
func (h *CredentialHandler) StoreCredential(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req StoreCredentialRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	actorID, _ := context.GetAccountID(c.Request.Context())
	actorEmail, _ := context.GetEmail(c.Request.Context())
	rotatedBy := actorID
	if rotatedBy == "" {
		rotatedBy = actorEmail
	}
	if rotatedBy == "" {
		rotatedBy = "unknown"
	}

	revision, err := h.provider.Store(
		c.Request.Context(), tenantID,
		req.BaseURL, req.AccessToken, req.WebhookSecret, req.Environment, req.AuthScheme,
		rotatedBy, strings.TrimSpace(req.Note),
	)
	if err != nil {
		if errors.Is(err, credential.ErrCredentialCryptoKeyMissing) {
			response.Error(c, http.StatusServiceUnavailable, "clicagens por tenant indisponíveis: "+err.Error())
			return
		}
		respondCredentialError(c, err)
		return
	}

	audit.Record(c.Request.Context(), audit.Entry{
		Action:     "clicksign.credentials_rotated",
		EntityType: "clicksign_credentials",
		EntityID:   tenantID,
		Metadata: map[string]any{
			"revision": revision,
			"note":     strings.TrimSpace(req.Note),
		},
	})

	response.Success(c, "Clicksign credentials stored", StoreCredentialResponse{
		TenantID: tenantID,
		Revision: revision,
	})
}

// StoreCredentialResponse confirma a gravação sem ecoar segredo.
type StoreCredentialResponse struct {
	TenantID string `json:"tenant_id"`
	Revision int    `json:"revision"`
}

// GetCredential godoc
// @Summary      Get masked tenant Clicksign credentials state
// @Description  Devolve o estado mascarado (base_url, environment, auth_scheme, revision, has_token) — nunca os segredos.
// @Tags         signature-admin
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=credential.MaskedInfo}
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /signature/admin/clicksign-credentials [get]
func (h *CredentialHandler) GetCredential(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	info, err := h.provider.Info(c.Request.Context(), tenantID)
	if err != nil {
		respondCredentialError(c, err)
		return
	}
	response.Success(c, "Clicksign credentials retrieved", info)
}

// ListCredentialRotations godoc
// @Summary      List tenant Clicksign credential rotations
// @Description  Trilha append-only de rotações (quem, quando, revisão).
// @Tags         signature-admin
// @Produce      json
// @Security     BearerAuth
// @Param        limit query int false "Limit"
// @Success      200 {object} response.Response{data=[]credential.CredentialRotation}
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /signature/admin/clicksign-credentials/rotations [get]
func (h *CredentialHandler) ListCredentialRotations(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	rotations, err := h.provider.Rotations(c.Request.Context(), tenantID, limit)
	if err != nil {
		respondCredentialError(c, err)
		return
	}
	response.Success(c, "Clicksign credential rotations retrieved", rotations)
}

func respondCredentialError(c *gin.Context, err error) {
	if errors.Is(err, credential.ErrCredentialCryptoKeyMissing) {
		response.Error(c, http.StatusServiceUnavailable, err.Error())
		return
	}
	response.InternalError(c, err)
}
