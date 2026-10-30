package handlers

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// CommissionRulesHandler expõe GET/PUT /commission-rules.
//   - GET (finance:read): devolve sempre as 4 roles (overrides + defaults).
//   - PUT (finance:write): replace-all do conjunto.
type CommissionRulesHandler struct {
	service *service.CommissionRulesService
}

func NewCommissionRulesHandler(s *service.CommissionRulesService) *CommissionRulesHandler {
	return &CommissionRulesHandler{service: s}
}

type RuleDTO struct {
	Role              string  `json:"role" binding:"required"`
	DefaultPercentage float64 `json:"default_percentage" binding:"min=0,max=100"`
}

type UpdateCommissionRulesRequest struct {
	Rules []RuleDTO `json:"rules" binding:"required"`
}

// Get godoc
// @Summary      Get tenant commission rules
// @Tags         commission-rules
// @Produce      json
// @Security     BearerAuth
// @Success      200 {object} response.Response{data=[]commission.Rule}
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /commission-rules [get]
func (h *CommissionRulesHandler) Get(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	rules, err := h.service.Get(ctx, tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Commission rules retrieved successfully", rules)
}

// Update godoc
// @Summary      Update tenant commission rules
// @Tags         commission-rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.UpdateCommissionRulesRequest true "Update Request"
// @Success      200 {object} response.Response{data=[]commission.Rule}
// @Failure      400 {object} response.Response
// @Failure      403 {object} response.Response
// @Failure      500 {object} response.Response
// @Router       /commission-rules [put]
func (h *CommissionRulesHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UpdateCommissionRulesRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	rules := make([]commission.Rule, 0, len(req.Rules))
	for _, r := range req.Rules {
		rules = append(rules, commission.Rule{
			TenantID:          tenantID,
			Role:              commission.Role(strings.ToLower(strings.TrimSpace(r.Role))),
			DefaultPercentage: r.DefaultPercentage,
		})
	}
	updated, err := h.service.Update(ctx, tenantID, rules)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Commission rules updated successfully", updated)
}
