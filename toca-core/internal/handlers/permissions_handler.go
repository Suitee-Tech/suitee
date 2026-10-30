package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	service "github.com/oginoino/toca-core/internal/services"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
)

type PermissionsHandler struct {
	settingsService *service.SettingsService
}

func NewPermissionsHandler(settingsService *service.SettingsService) *PermissionsHandler {
	return &PermissionsHandler{settingsService: settingsService}
}

func (h *PermissionsHandler) GetMyPermissions(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appCtx.GetTenantID(ctx)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "tenant id not found in context")
		return
	}

	functionalRole, _ := appCtx.GetFunctionalRole(ctx)
	if functionalRole == "" {
		functionalRole = "corretor" // default fallback
	}

	profile, _ := appCtx.GetUserPermission(ctx) // Profile

	permissions, err := h.settingsService.GetEffectivePermissions(ctx, tenantID, functionalRole)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	// PAPEL BASE do perfil de acesso: quando o `functional_role` é um perfil
	// personalizado (`cr_…`), o frontend precisa do papel de onde ele partiu para
	// decidir VISIBILIDADE com a mesma regra do servidor
	// (pkg/auth.HasTenantWideScope) — sem ele, um perfil "Gestor Comercial" via a
	// lista de leads vazia mesmo com as permissões do gestor (incidente de
	// 2026-09-21).
	baseRole, err := h.settingsService.GetAccessProfileBaseRole(ctx, tenantID, functionalRole)
	if err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"functional_role": functionalRole,
		"base_role":       baseRole,
		"profile":         profile,
		"permissions":     permissions,
	})
}
