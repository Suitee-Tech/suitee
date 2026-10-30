package handlers

import (
	"errors"
	"io"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

type WebhookHandler struct {
	webhookService *service.WebhookService
}

func NewWebhookHandler(webhookService *service.WebhookService) *WebhookHandler {
	return &WebhookHandler{webhookService: webhookService}
}

// ReceiveLead godoc
// @Summary      Receive portal lead webhook
// @Description  Public endpoint that receives lead webhooks from portals (ZAP, OLX, VivaReal). The X-Hub-Signature HMAC header is mandatory; requests without it are rejected with 401.
// @Tags         portals
// @Accept       json
// @Produce      json
// @Param        integrationID    path      string  true  "Integration ID"
// @Param        X-Hub-Signature  header    string  true  "HMAC signature of the request body (sha256=<hex>)"
// @Success      201              {object}  response.Response
// @Failure      400              {object}  response.Response
// @Failure      401              {object}  response.Response
// @Failure      403              {object}  response.Response
// @Failure      404              {object}  response.Response
// @Failure      429              {object}  response.Response
// @Failure      500              {object}  response.Response
// @Router       /portals/webhook/{integrationID} [post]
func (h *WebhookHandler) ReceiveLead(c *gin.Context) {
	integrationID := c.Param("integrationID")
	if integrationID == "" {
		response.ValidationError(c, "missing integration ID")
		return
	}

	// Read raw body for HMAC validation
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.ValidationError(c, "failed to read request body")
		return
	}

	// Get HMAC signature from header — mandatory (C5)
	signature := c.GetHeader("X-Hub-Signature")

	portalLead, err := h.webhookService.ProcessWebhook(c.Request.Context(), integrationID, body, signature)
	if err != nil {
		// O detalhe (incluindo erro de driver/banco) fica no log do servidor;
		// o chamador anônimo recebe sempre uma mensagem genérica.
		log.Printf("[WebhookHandler] Error processing webhook (integration=%s): %v", integrationID, err)
		switch {
		case errors.Is(err, service.ErrSignatureRequired), errors.Is(err, service.ErrSignatureInvalid):
			response.Unauthorized(c, "missing or invalid webhook signature")
		case errors.Is(err, service.ErrWebhookSecretMissing):
			response.InternalServerError(c, "webhook is not configured for this integration")
		case errors.Is(err, integration.ErrNotFound):
			response.NotFound(c, "integration not found")
		case errors.Is(err, service.ErrIntegrationInactive):
			response.Forbidden(c, "integration is not active")
		case errors.Is(err, service.ErrWebhookRateLimited):
			c.Header("Retry-After", "60")
			response.Error(c, http.StatusTooManyRequests, "rate limit exceeded for this integration")
		case errors.Is(err, service.ErrInvalidPayload):
			response.ValidationError(c, "invalid lead payload")
		default:
			response.InternalServerError(c, "failed to process webhook")
		}
		return
	}

	response.Created(c, "Lead received successfully", gin.H{
		"portal_lead_id": portalLead.ID,
		"status":         portalLead.Status,
		"crm_lead_id":    portalLead.CRMLeadID,
	})
}
