package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-financial/internal/domain/broker_advance"
	"github.com/oginoino/toca/toca-financial/internal/services"
)

// BrokerAdvanceHandler expõe os adiantamentos da imobiliária ao corretor: o
// saldo a recuperar e o registro da concessão (que sai do caixa e vira valor a
// receber do corretor).
type BrokerAdvanceHandler struct {
	service *services.BrokerAdvanceService
}

func NewBrokerAdvanceHandler(service *services.BrokerAdvanceService) *BrokerAdvanceHandler {
	return &BrokerAdvanceHandler{service: service}
}

type createBrokerAdvanceRequest struct {
	BrokerID    string  `json:"broker_id" binding:"required"`
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	Description string  `json:"description"`
	GrantedAt   string  `json:"granted_at"`
}

// Create godoc
// @Summary  Registra um adiantamento ao corretor
// @Router   /broker-advances [post]
func (h *BrokerAdvanceHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := pkgctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req createBrokerAdvanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ValidationError(c, "informe o corretor e um valor maior que zero")
		return
	}
	actor, _ := pkgctx.GetAccountID(ctx)
	grantedAt := time.Time{}
	if raw := strings.TrimSpace(req.GrantedAt); raw != "" {
		if parsed, err := time.Parse("2006-01-02", raw); err == nil {
			grantedAt = parsed
		}
	}
	amountCents := int64(req.Amount*100 + 0.5)
	advance, err := h.service.Grant(ctx, tenantID, strings.TrimSpace(req.BrokerID), amountCents, req.Description, actor, grantedAt)
	if err != nil {
		if errors.Is(err, broker_advance.ErrInvalidInput) {
			response.UnprocessableEntity(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": advance})
}

// List godoc
// @Summary  Lista adiantamentos (com o saldo por corretor)
// @Router   /broker-advances [get]
func (h *BrokerAdvanceHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := pkgctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	filter := broker_advance.ListFilter{BrokerID: strings.TrimSpace(c.Query("broker_id"))}
	if raw := strings.TrimSpace(c.Query("open")); raw == "true" || raw == "1" {
		filter.OpenOnly = true
	}
	for _, status := range strings.Split(c.Query("status"), ",") {
		if trimmed := strings.TrimSpace(status); trimmed != "" {
			filter.Status = append(filter.Status, broker_advance.Status(strings.ToUpper(trimmed)))
		}
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if limit, err := strconv.Atoi(raw); err == nil {
			filter.Limit = limit
		}
	}
	items, err := h.service.List(ctx, tenantID, filter)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	balances, err := h.service.Balances(ctx, tenantID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "meta": gin.H{"balances": balances}})
}

// Cancel godoc
// @Summary  Cancela um adiantamento sem abatimentos
// @Router   /broker-advances/{id}/cancel [post]
func (h *BrokerAdvanceHandler) Cancel(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := pkgctx.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	actor, _ := pkgctx.GetAccountID(ctx)
	advance, err := h.service.Cancel(ctx, tenantID, c.Param("id"), actor, req.Reason)
	if err != nil {
		switch {
		case errors.Is(err, broker_advance.ErrNotFound):
			response.NotFound(c, "adiantamento não encontrado")
		case errors.Is(err, broker_advance.ErrHasSettlements):
			response.UnprocessableEntity(c, "adiantamento já tem abatimentos em comissões: desfaça os descontos antes de cancelar")
		default:
			response.InternalError(c, err)
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": advance})
}
