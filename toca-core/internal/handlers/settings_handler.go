package handlers

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oginoino/toca-core/internal/domain/core/settings"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
)

type SettingsHandler struct {
	settingsService *service.SettingsService
}

func NewSettingsHandler(settingsService *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{
		settingsService: settingsService,
	}
}

// Helper to get tenantID
func getTenantID(c *gin.Context) string {
	return c.GetString("tenant_id")
}

// Company Settings
func (h *SettingsHandler) GetCompanySettings(c *gin.Context) {
	tenantID := getTenantID(c)
	res, err := h.settingsService.GetCompanySettings(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// GetTenantTimezoneInternal devolve o fuso configurado da imobiliária para os
// serviços de fluxo (financeiro, negócios, agenda) montarem dias civis.
//
// Rota INTERNA: chega com a credencial de serviço e sem JWT de usuário (o fluxo
// não tem usuário na ponta — consumidor de evento, job, chamada serviço-a-
// serviço). O tenant vem da credencial/allowlist via
// auth.ResolveTenantForInternalRequest, nunca de um JWT.
func (h *SettingsHandler) GetTenantTimezoneInternal(c *gin.Context) {
	tenantID, err := auth.ResolveTenantForInternalRequest(c, c.Query("tenant_id"))
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	timezone, err := h.settingsService.TenantTimezone(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tenant_id": tenantID, "timezone": timezone})
}

func (h *SettingsHandler) UpdateCompanySettings(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.CompanySettings
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.settingsService.UpdateCompanySettings(c.Request.Context(), tenantID, &req); err != nil {
		if errors.Is(err, service.ErrInvalidTimezone) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "company settings updated"})
}

func (h *SettingsHandler) UploadLogo(c *gin.Context) {
	tenantID := getTenantID(c)

	file, err := c.FormFile("logo")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "logo file is required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer src.Close()

	head := make([]byte, 512)
	n, err := src.Read(head)
	if err != nil && err != io.EOF {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file header"})
		return
	}

	ext, detectedMIME, err := storage.ValidateWithMIME("toca-logos", file.Filename, file.Size, head[:n])
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	multiSrc := io.MultiReader(bytes.NewReader(head[:n]), src)
	uniqueFilename := fmt.Sprintf("%s%s", uuid.NewString(), ext)

	logoURL, err := h.settingsService.UploadCompanyLogo(c.Request.Context(), tenantID, uniqueFilename, detectedMIME, multiSrc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"logo_url": logoURL}})
}

// UploadLoginBanner recebe a imagem de banner da tela de login enviada pela
// imobiliária. Mesmo contrato do UploadLogo (bucket, validação de MIME real e
// nome único); o que muda é o campo do formulário e o destino — a URL vai para
// `AppearanceSettings.LoginBannerURL`, que tem precedência sobre o preset.
func (h *SettingsHandler) UploadLoginBanner(c *gin.Context) {
	tenantID := getTenantID(c)

	file, err := c.FormFile("banner")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "banner file is required"})
		return
	}

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open file"})
		return
	}
	defer src.Close()

	head := make([]byte, 512)
	n, err := src.Read(head)
	if err != nil && err != io.EOF {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file header"})
		return
	}

	ext, detectedMIME, err := storage.ValidateWithMIME("toca-logos", file.Filename, file.Size, head[:n])
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	multiSrc := io.MultiReader(bytes.NewReader(head[:n]), src)
	uniqueFilename := fmt.Sprintf("%s%s", uuid.NewString(), ext)

	bannerURL, err := h.settingsService.UploadLoginBanner(c.Request.Context(), tenantID, uniqueFilename, detectedMIME, multiSrc)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"banner_url": bannerURL}})
}

// Appearance
func (h *SettingsHandler) GetAppearance(c *gin.Context) {
	tenantID := getTenantID(c)
	res, err := h.settingsService.GetAppearance(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *SettingsHandler) UpdateAppearance(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.AppearanceSettings
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.settingsService.UpdateAppearance(c.Request.Context(), tenantID, &req); err != nil {
		// N4: valor recusado na validação é 400 (culpa do payload), não 500.
		if errors.Is(err, service.ErrInvalidAppearance) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "appearance updated"})
}

// Permissions
func (h *SettingsHandler) GetPermissions(c *gin.Context) {
	tenantID := getTenantID(c)
	res, err := h.settingsService.GetPermissions(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *SettingsHandler) UpdatePermissions(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.RolePermissions
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.settingsService.UpdatePermissions(c.Request.Context(), tenantID, &req); err != nil {
		// P0.2/A6: matriz com ids/papéis fora do catálogo é erro do CLIENTE, não
		// do servidor — 400 com a lista do que não bate, em vez de 500 (ou, pior,
		// do antigo 200 mentiroso que descartava os itens desconhecidos).
		var invalid *settings.InvalidPermissionMatrixError
		if errors.As(err, &invalid) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":                  invalid.Error(),
				"unknown_permission_ids": invalid.UnknownPermissionIDs,
				"unknown_roles":          invalid.UnknownRoles,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "permissions updated"})
}

// ─────────────────────── Perfis de acesso personalizados ───────────────────────

// mapAccessProfileError traduz os erros de perfil para o status certo: payload
// inválido é erro do CLIENTE (400 com a lista do que não bate, mesmo contrato da
// matriz em A6); perfil inexistente é 404 — nunca 200 mentiroso nem 500.
func mapAccessProfileError(c *gin.Context, err error) {
	var invalid *settings.InvalidAccessProfileError
	if errors.As(err, &invalid) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":                  invalid.Error(),
			"invalid_ids":            invalid.InvalidIDs,
			"duplicated_ids":         invalid.DuplicatedIDs,
			"missing_names":          invalid.MissingNames,
			"unknown_base_roles":     invalid.UnknownBaseRoles,
			"unknown_permission_ids": invalid.UnknownPermissionIDs,
		})
		return
	}
	var notFound *settings.ErrAccessProfileNotFound
	if errors.As(err, &notFound) {
		response.NotFound(c, notFound.Error())
		return
	}
	var invalidMatrix *settings.InvalidPermissionMatrixError
	if errors.As(err, &invalidMatrix) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":                  invalidMatrix.Error(),
			"unknown_permission_ids": invalidMatrix.UnknownPermissionIDs,
			"unknown_roles":          invalidMatrix.UnknownRoles,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *SettingsHandler) ListAccessProfiles(c *gin.Context) {
	profiles, err := h.settingsService.ListAccessProfiles(c.Request.Context(), getTenantID(c))
	if err != nil {
		mapAccessProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profiles})
}

func (h *SettingsHandler) CreateAccessProfile(c *gin.Context) {
	var req service.AccessProfileInput
	if !validation.BindJSON(c, &req) {
		return
	}
	profile, err := h.settingsService.CreateAccessProfile(c.Request.Context(), getTenantID(c), req)
	if err != nil {
		mapAccessProfileError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true, "data": profile})
}

func (h *SettingsHandler) UpdateAccessProfile(c *gin.Context) {
	var req service.AccessProfileInput
	if !validation.BindJSON(c, &req) {
		return
	}
	profile, err := h.settingsService.UpdateAccessProfile(c.Request.Context(), getTenantID(c), c.Param("profile_id"), req)
	if err != nil {
		mapAccessProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": profile})
}

func (h *SettingsHandler) DeleteAccessProfile(c *gin.Context) {
	if err := h.settingsService.DeleteAccessProfile(c.Request.Context(), getTenantID(c), c.Param("profile_id")); err != nil {
		mapAccessProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "perfil de acesso removido"})
}

// Notifications
func (h *SettingsHandler) GetNotifications(c *gin.Context) {
	tenantID := getTenantID(c)
	res, err := h.settingsService.GetNotificationSettings(c.Request.Context(), tenantID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *SettingsHandler) UpdateNotifications(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.NotificationSettings
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.settingsService.UpdateNotificationSettings(c.Request.Context(), tenantID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "notification settings updated"})
}

// WebhookEndpointCreated é a resposta do POST /company-settings/webhooks.
// Repete os campos públicos de settings.WebhookEndpoint e acrescenta o
// signing secret, que é write-only (`json:"-"`) no modelo de domínio e por
// isso NUNCA aparece na listagem — apenas aqui, uma única vez, na criação
// (P0/A1).
type WebhookEndpointCreated struct {
	ID            interface{} `json:"id"`
	TenantID      string      `json:"tenant_id"`
	URL           string      `json:"url"`
	Events        []string    `json:"events"`
	Active        bool        `json:"active"`
	LastTriggered time.Time   `json:"lastTriggered"`
	Secret        string      `json:"secret"`
}

func newWebhookEndpointCreated(w *settings.WebhookEndpoint) WebhookEndpointCreated {
	return WebhookEndpointCreated{
		ID:            w.ID,
		TenantID:      w.TenantID,
		URL:           w.URL,
		Events:        w.Events,
		Active:        w.Active,
		LastTriggered: w.LastTriggered,
		Secret:        w.Secret,
	}
}

// Webhooks
func (h *SettingsHandler) CreateWebhook(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.WebhookEndpoint
	if !validation.BindJSON(c, &req) {
		return
	}
	if err := h.settingsService.CreateWebhook(c.Request.Context(), tenantID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, newWebhookEndpointCreated(&req))
}

func (h *SettingsHandler) ListWebhooks(c *gin.Context) {
	tenantID := getTenantID(c)
	res, err := h.settingsService.ListWebhooks(c.Request.Context(), tenantID)
	if err != nil {
		if isNotImplemented(err) {
			c.JSON(http.StatusOK, []settings.WebhookEndpoint{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

func (h *SettingsHandler) UpdateWebhook(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.WebhookEndpoint
	if !validation.BindJSON(c, &req) {
		return
	}
	req.ID = c.Param("id")
	if err := h.settingsService.UpdateWebhook(c.Request.Context(), tenantID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "webhook updated"})
}

// DeleteWebhook apaga o webhook dentro do tenant do chamador. O tenant vem do
// contexto autenticado; sem ele o repositório filtraria só pelo id (P0/C6).
// Zero linhas afetadas (webhook de outro tenant, inexistente ou já removido)
// responde 404 — nunca um "sucesso" falso.
func (h *SettingsHandler) DeleteWebhook(c *gin.Context) {
	tenantID := getTenantID(c)
	id := c.Param("id")
	if err := h.settingsService.DeleteWebhook(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "webhook deleted"})
}

// API Keys
func (h *SettingsHandler) CreateAPIKey(c *gin.Context) {
	tenantID := getTenantID(c)
	var req settings.APIKey
	if !validation.BindJSON(c, &req) {
		return
	}

	rawKey, err := h.settingsService.GenerateAPIKey(c.Request.Context(), tenantID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"api_key": req,
		"raw_key": rawKey, // only returned once
	})
}

func (h *SettingsHandler) ListAPIKeys(c *gin.Context) {
	tenantID := getTenantID(c)
	res, err := h.settingsService.ListAPIKeys(c.Request.Context(), tenantID)
	if err != nil {
		if isNotImplemented(err) {
			c.JSON(http.StatusOK, []settings.APIKey{})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, res)
}

// RevokeAPIKey revoga a API key dentro do tenant do chamador (P0/C6). Zero
// linhas afetadas responde 404 — nunca um "sucesso" falso.
func (h *SettingsHandler) RevokeAPIKey(c *gin.Context) {
	tenantID := getTenantID(c)
	id := c.Param("id")
	if err := h.settingsService.RevokeAPIKey(c.Request.Context(), tenantID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "api key not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "api key revoked"})
}
