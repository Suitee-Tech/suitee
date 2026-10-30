package handlers

import (
	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

// EmailPrefsHandler expõe as preferências de e-mail transacional do usuário
// (beta F4 — docs/features/comunicacao/notificacoes-email-beta.md §7).
type EmailPrefsHandler struct {
	service *service.EmailPrefsService
}

func NewEmailPrefsHandler(service *service.EmailPrefsService) *EmailPrefsHandler {
	return &EmailPrefsHandler{service: service}
}

// List devolve todas as categorias com estado atual, label e locked.
func (h *EmailPrefsHandler) List(c *gin.Context) {
	userID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil || userID == "" {
		response.Forbidden(c, "account context required")
		return
	}
	tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
	role, _ := pkgcontext.GetFunctionalRole(c.Request.Context())

	prefs, err := h.service.List(c.Request.Context(), userID, tenantID, role)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Email preferences retrieved", prefs)
}

type updateEmailPrefsRequest struct {
	Prefs []struct {
		Category string `json:"category" binding:"required"`
		Enabled  bool   `json:"enabled"`
	} `json:"prefs" binding:"required"`
}

// Update aplica os toggles enviados e devolve a lista atualizada.
func (h *EmailPrefsHandler) Update(c *gin.Context) {
	userID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil || userID == "" {
		response.Forbidden(c, "account context required")
		return
	}
	tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
	role, _ := pkgcontext.GetFunctionalRole(c.Request.Context())

	var req updateEmailPrefsRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	updates := map[string]bool{}
	for _, p := range req.Prefs {
		updates[p.Category] = p.Enabled
	}
	prefs, err := h.service.Update(c.Request.Context(), userID, tenantID, role, updates)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Email preferences updated", prefs)
}
