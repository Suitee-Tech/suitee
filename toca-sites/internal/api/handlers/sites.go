package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-sites/internal/domain/site"
	"github.com/oginoino/toca/toca-sites/internal/services"
)

// SiteHandler expõe as rotas autenticadas do painel. O tenant vem SEMPRE do
// JWT (ou da credencial interna) — nunca do corpo/query.
type SiteHandler struct {
	sites *services.SiteService
	sync  *services.SyncService
}

func NewSiteHandler(sites *services.SiteService, sync *services.SyncService) *SiteHandler {
	return &SiteHandler{sites: sites, sync: sync}
}

func tenantOf(c *gin.Context) string {
	return strings.TrimSpace(c.GetString("tenant_id"))
}

// GetMe godoc
// @Summary      Current tenant site
// @Tags         sites
// @Router       /sites/me [get]
func (h *SiteHandler) GetMe(c *gin.Context) {
	tenantID := tenantOf(c)
	if tenantID == "" {
		response.Unauthorized(c, "tenant is required")
		return
	}
	overview, err := h.sites.GetOverview(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Site carregado", overview)
}

// Readiness godoc
// @Summary      Public site readiness (TLS + first successful response)
// @Tags         sites
// @Router       /sites/me/readiness [get]
func (h *SiteHandler) Readiness(c *gin.Context) {
	tenantID := tenantOf(c)
	if tenantID == "" {
		response.Unauthorized(c, "tenant is required")
		return
	}
	out, err := h.sites.Readiness(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Prontidão do site", out)
}

// SaveMe godoc
// @Summary      Create/update the tenant site
// @Tags         sites
// @Router       /sites/me [put]
func (h *SiteHandler) SaveMe(c *gin.Context) {
	tenantID := tenantOf(c)
	if tenantID == "" {
		response.Unauthorized(c, "tenant is required")
		return
	}
	var in services.SaveSiteInput
	if err := c.ShouldBindJSON(&in); err != nil {
		response.ValidationError(c, "payload inválido")
		return
	}
	overview, err := h.sites.Save(c.Request.Context(), tenantID, in)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Site atualizado", overview)
}

// ListPages godoc
// @Summary      List editable pages
// @Tags         sites
// @Router       /sites/pages [get]
func (h *SiteHandler) ListPages(c *gin.Context) {
	tenantID := tenantOf(c)
	overview, err := h.sites.GetOverview(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Páginas carregadas", gin.H{"pages": overview.Pages})
}

type savePageRequest struct {
	Title          string          `json:"title"`
	Subtitle       string          `json:"subtitle"`
	Body           json.RawMessage `json:"body"`
	SEOTitle       string          `json:"seo_title"`
	SEODescription string          `json:"seo_description"`
}

// UpdatePage godoc
// @Summary      Update one editable page
// @Tags         sites
// @Router       /sites/pages/{key} [put]
func (h *SiteHandler) UpdatePage(c *gin.Context) {
	tenantID := tenantOf(c)
	var req savePageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "payload inválido")
		return
	}
	page, err := h.sites.SavePage(c.Request.Context(), tenantID, c.Param("key"), site.Page{
		Title:          req.Title,
		Subtitle:       req.Subtitle,
		Body:           req.Body,
		SEOTitle:       req.SEOTitle,
		SEODescription: req.SEODescription,
	})
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Página atualizada", page)
}

// ListingURL godoc
// @Summary      Canonical public URL of a listing
// @Tags         sites
// @Router       /sites/listings/{propertyId}/url [get]
func (h *SiteHandler) ListingURL(c *gin.Context) {
	out, err := h.sites.ListingURL(c.Request.Context(), tenantOf(c), c.Param("propertyId"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "URL do anúncio", out)
}

// ListingStats godoc
// @Summary      Performance metrics for a listing
// @Tags         sites
// @Router       /sites/listings/{propertyId}/stats [get]
func (h *SiteHandler) ListingStats(c *gin.Context) {
	out, err := h.sites.ListingStats(c.Request.Context(), tenantOf(c), c.Param("propertyId"))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Métricas do anúncio", out)
}

// Resync godoc
// @Summary      Re-sync a property projection from toca-core
// @Tags         sites
// @Router       /sites/listings/{propertyId}/resync [post]
func (h *SiteHandler) Resync(c *gin.Context) {
	tenantID := tenantOf(c)
	propertyID := c.Param("propertyId")
	if err := h.sync.HandlePropertyEvent(c.Request.Context(), tenantID, propertyID); err != nil {
		writeError(c, err)
		return
	}
	out, err := h.sites.ListingStats(c.Request.Context(), tenantID, propertyID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Projeção ressincronizada", out)
}

// Stats godoc
// @Summary      Site aggregates
// @Tags         sites
// @Router       /sites/stats [get]
func (h *SiteHandler) Stats(c *gin.Context) {
	out, err := h.sites.SiteStats(c.Request.Context(), tenantOf(c))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Métricas do site", out)
}

// PortalListingURL é a rota usada pelo portal do cliente para decidir se
// mostra o CTA "Ver anúncio no site". Isolada do resto do painel porque o
// papel `cliente` é rejeitado nas demais rotas.
//
// O JWT do cliente do portal carrega o tenant da PRÓPRIA CONTA (ver
// toca-auth: `tenant_id = acc.ID`), não o da imobiliária do deal. Por isso a
// query pode apontar `tenant_id` explicitamente — mas a superfície devolve
// APENAS a URL pública de um anúncio já publicado e com o site ligado. Não
// expõe rascunho, métrica, lead nem qualquer dado do tenant: é a mesma
// informação que qualquer visitante do site veria.
func (h *SiteHandler) PortalListingURL(c *gin.Context) {
	propertyID := c.Query("property_id")
	if propertyID == "" {
		propertyID = c.Param("propertyId")
	}
	tenantID := strings.TrimSpace(c.Query("tenant_id"))
	if tenantID == "" {
		tenantID = tenantOf(c)
	}
	if _, err := uuid.Parse(tenantID); err != nil {
		response.ValidationError(c, "tenant inválido")
		return
	}
	out, err := h.sites.ListingURL(c.Request.Context(), tenantID, propertyID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "URL do anúncio", out)
}

// ReconcileMe re-projeta o estoque no Payload em background.
// @Summary      Re-sync the whole storefront projection
// @Tags         sites
// @Router       /sites/me/reconcile [post]
func (h *SiteHandler) ReconcileMe(c *gin.Context) {
	tenantID := tenantOf(c)
	if err := h.sites.Reconcile(c.Request.Context(), tenantID); err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Sincronização da vitrine iniciada", gin.H{"started": true})
}

// PortalSite devolve a imobiliária do deal para o portal do cliente.
// @Summary      Public site metadata for the client portal
// @Tags         sites
// @Router       /sites/portal/site [get]
func (h *SiteHandler) PortalSite(c *gin.Context) {
	tenantID := strings.TrimSpace(c.Query("tenant_id"))
	if tenantID == "" {
		tenantID = tenantOf(c)
	}
	if _, err := uuid.Parse(tenantID); err != nil {
		response.ValidationError(c, "tenant inválido")
		return
	}
	out, err := h.sites.PortalSite(c.Request.Context(), tenantID)
	if err != nil {
		writeError(c, err)
		return
	}
	response.Success(c, "Site da imobiliária", out)
}

// Leads lista os leads do site para o painel (paginação simples).
func (h *SiteHandler) Leads(c *gin.Context) {
	tenantID := tenantOf(c)
	limit := parseIntDefault(c.Query("limit"), 50)
	if limit > 200 {
		limit = 200
	}
	offset := parseIntDefault(c.Query("offset"), 0)

	// Repository exposto via service? Mantemos a leitura no domínio de
	// engagement para não vazar SQL no handler.
	leads, total, err := h.sites.ListLeads(c.Request.Context(), tenantID, int64(limit), int64(offset))
	if err != nil {
		writeError(c, err)
		return
	}
	response.Paginated(c, "Leads do site", leads, offset/limit+1, limit, total)
}

func writeError(c *gin.Context, err error) {
	var validation services.ValidationError
	switch {
	case errors.As(err, &validation):
		response.ValidationErrorFields(c, []response.ValidationFieldError{{
			Field:   validation.Field,
			Message: validation.Message,
			Code:    "invalid",
		}})
	case errors.Is(err, services.ErrValidation):
		response.ValidationError(c, err.Error())
	case errors.Is(err, services.ErrDiscarded):
		c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "recebido"})
	case errors.Is(err, site.ErrNotFound):
		response.NotFound(c, "site ou anúncio não encontrado")
	case errors.Is(err, site.ErrSiteDisabled):
		response.NotFound(c, "site não encontrado")
	case errors.Is(err, site.ErrSlugTaken):
		response.Conflict(c, "essa marca já está em uso; escolha outra")
	case errors.Is(err, site.ErrSlugInvalid):
		response.ValidationError(c, err.Error())
	default:
		response.InternalError(c, err)
	}
}

func parseIntDefault(raw string, def int) int {
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return def
	}
	return n
}
