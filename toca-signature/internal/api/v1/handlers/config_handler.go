package handlers

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
)

// ConfigHandler exposes the public configuration the operator needs to set
// up Clicksign on their side — primarily the webhook URL derived from
// PUBLIC_GATEWAY_URL.
type ConfigHandler struct {
	creds credential.CredentialProvider
}

func NewConfigHandler(creds credential.CredentialProvider) *ConfigHandler {
	return &ConfigHandler{creds: creds}
}

type SignatureConfig struct {
	WebhookURL  string `json:"webhook_url"`
	Environment string `json:"environment"`
	BaseURL     string `json:"base_url"`
	HasToken    bool   `json:"has_token"`
	HasSecret   bool   `json:"has_secret"`
}

// Get godoc
// @Summary  Get Clicksign integration config
// @Description  Returns the webhook URL the operator should register in the
// @Description  Clicksign dashboard, plus a summary of which credentials are
// @Description  configured. No secret values are returned.
// @Tags         signature
// @Security     BearerAuth
// @Produce      json
// @Success      200 {object} response.Response{data=SignatureConfig}
// @Router       /signature/config [get]
func (h *ConfigHandler) Get(c *gin.Context) {
	cfg := SignatureConfig{
		WebhookURL: clicksign.PublicWebhookURL(),
	}
	creds, err := h.creds.Get(context.Background(), "")
	if err == nil && creds != nil {
		cfg.Environment = creds.Environment
		cfg.BaseURL = creds.BaseURL
		cfg.HasToken = creds.AccessToken != ""
		cfg.HasSecret = creds.WebhookSecret != ""
	}
	response.Success(c, "Signature configuration", cfg)
}
