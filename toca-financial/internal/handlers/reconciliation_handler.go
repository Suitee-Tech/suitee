package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// ReconciliationHandler expõe o endpoint de reconciliação dry-run
// que gera um relatório de integridade financeira sem modificar dados
// (FIN-COM-14).
type ReconciliationHandler struct {
	service *service.ReconciliationService
}

func NewReconciliationHandler(svc *service.ReconciliationService) *ReconciliationHandler {
	return &ReconciliationHandler{service: svc}
}

// Run godoc
// @Summary      Run reconciliation dry-run
// @Description  Executes a read-only reconciliation report that detects duplicates, orphans, excess allocation, sub-allocation, and status cascade inconsistencies across commissions, transactions, and payout_orders.
// @Tags         reports
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /reports/financial/reconciliation [get]
func (h *ReconciliationHandler) Run(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	rep, err := h.service.Run(c.Request.Context(), tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": rep})
}
