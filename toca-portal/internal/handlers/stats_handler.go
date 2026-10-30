package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-portal/internal/domain/lead"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

type StatsHandler struct {
	integService *service.IntegrationService
	leadRepo     lead.Repository
}

func NewStatsHandler(integService *service.IntegrationService, leadRepo lead.Repository) *StatsHandler {
	return &StatsHandler{
		integService: integService,
		leadRepo:     leadRepo,
	}
}

// Dashboard godoc
// @Summary      Portal dashboard
// @Description  Overview stats for portal integrations (counts, last syncs, leads by portal)
// @Tags         portal-stats
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /portal-stats/dashboard [get]
func (h *StatsHandler) Dashboard(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	// Get integrations
	integrations, err := h.integService.ListByTenant(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// Get lead stats by portal
	ctx := c.Request.Context()
	totalLeadsCount, leadStats, err := h.leadRepo.GetStats(ctx, tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	// Build dashboard response
	portalStats := make([]gin.H, 0)
	for _, integ := range integrations {
		portalStats = append(portalStats, gin.H{
			"id":               integ.ID,
			"portal":           integ.Portal,
			"status":           integ.Status,
			"last_sync_at":     integ.LastSyncAt,
			"last_sync_status": integ.LastSyncStatus,
			"sync_frequency":   integ.SyncFrequency,
			"leads_count":      leadStats[string(integ.Portal)],
			"error_message":    integ.ErrorMessage,
		})
	}

	totalLeads := int64(0)
	for _, count := range leadStats {
		totalLeads += count
	}
	if totalLeadsCount > totalLeads {
		totalLeads = totalLeadsCount
	}

	response.Success(c, "Dashboard retrieved successfully", gin.H{
		"integrations":       portalStats,
		"total_integrations": len(integrations),
		"total_leads":        totalLeads,
		"leads_by_portal":    leadStats,
	})
}

// PortalLeads godoc
// @Summary      List portal leads
// @Description  Paginated list of leads captured from all portal integrations for the tenant
// @Tags         portal-stats
// @Produce      json
// @Security     BearerAuth
// @Param        limit   query     int  false  "Limit (default 20)"
// @Param        offset  query     int  false  "Offset"
// @Success      200     {object}  response.Response
// @Failure      403     {object}  response.Response
// @Failure      500     {object}  response.Response
// @Router       /portal-stats/leads [get]
func (h *StatsHandler) PortalLeads(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	leads, total, err := h.leadRepo.ListByTenant(c.Request.Context(), tenantID, limit, offset)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Portal leads retrieved successfully", leads, page, int(limit), total)
}

// LeadsByIntegration godoc
// @Summary      List leads for an integration
// @Description  Paginated list of leads captured from a specific integration
// @Tags         integrations
// @Produce      json
// @Security     BearerAuth
// @Param        id      path      string  true   "Integration ID"
// @Param        limit   query     int     false  "Limit (default 20)"
// @Param        offset  query     int     false  "Offset"
// @Success      200     {object}  response.Response
// @Failure      403     {object}  response.Response
// @Failure      500     {object}  response.Response
// @Router       /integrations/{id}/leads [get]
func (h *StatsHandler) LeadsByIntegration(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil || tenantID == "" {
		// B10 (revisão de permissões de 2026-09-19): não existe fallback de tenant
		// para o account_id. Em token pessoal (type=individual) o atalho transformava
		// "sem tenant" em "meu próprio id como tenant" — escopo inventado no cliente
		// para um recurso que é da empresa. Sem tenant, a resposta é 403.
		response.Forbidden(c, "tenant context required")
		return
	}

	id := c.Param("id")
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	leads, total, err := h.leadRepo.ListByIntegration(c.Request.Context(), tenantID, id, limit, offset)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Portal leads retrieved successfully", leads, page, int(limit), total)
}
