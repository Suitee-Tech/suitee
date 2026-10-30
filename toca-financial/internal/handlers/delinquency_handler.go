package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type DelinquencyHandler struct {
	service *service.DelinquencyService
}

func NewDelinquencyHandler(svc *service.DelinquencyService) *DelinquencyHandler {
	return &DelinquencyHandler{service: svc}
}

// GetDashboard godoc
// @Summary      Get delinquency dashboard
// @Description  Get complete delinquency dashboard with KPIs, aging report, and collection stats
// @Tags         delinquency
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=service.DelinquencyDashboard}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /delinquency/dashboard [get]
func (h *DelinquencyHandler) GetDashboard(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	dashboard, err := h.service.GetDashboard(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Delinquency dashboard retrieved successfully", dashboard)
}

// GetAgingReport godoc
// @Summary      Get aging report
// @Description  Get overdue charges aging report with bucket distribution
// @Tags         delinquency
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /delinquency/aging [get]
func (h *DelinquencyHandler) GetAgingReport(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	report, err := h.service.GetAgingReport(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Aging report retrieved successfully", report)
}

// GetStats godoc
// @Summary      Get delinquency stats
// @Description  Get delinquency KPI statistics
// @Tags         delinquency
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /delinquency/stats [get]
func (h *DelinquencyHandler) GetStats(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	stats, err := h.service.GetDelinquencyStats(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Delinquency stats retrieved successfully", stats)
}

// ProcessManual godoc
// @Summary      Trigger manual delinquency processing
// @Description  Manually trigger the delinquency engine for the current tenant
// @Tags         delinquency
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=service.ProcessResult}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /delinquency/process [post]
func (h *DelinquencyHandler) ProcessManual(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	result, err := h.service.ProcessManualTrigger(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	response.Success(c, "Delinquency processing completed", result)
}
