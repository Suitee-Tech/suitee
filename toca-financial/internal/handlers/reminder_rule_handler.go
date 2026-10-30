package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/reminder_rule"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type ReminderRuleHandler struct {
	service *service.ReminderRuleService
}

func NewReminderRuleHandler(svc *service.ReminderRuleService) *ReminderRuleHandler {
	return &ReminderRuleHandler{service: svc}
}

type CreateReminderRuleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	TriggerType string   `json:"trigger_type" binding:"required"`
	OffsetDays  int      `json:"offset_days"`
	Channels    []string `json:"channels" binding:"required"`
	Template    string   `json:"template" binding:"required"`
	Enabled     *bool    `json:"enabled"`
}

type UpdateReminderRuleRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	TriggerType *string  `json:"trigger_type"`
	OffsetDays  *int     `json:"offset_days"`
	Channels    []string `json:"channels"`
	Template    *string  `json:"template"`
	Enabled     *bool    `json:"enabled"`
}

func toChannels(in []string) []reminder_rule.Channel {
	out := make([]reminder_rule.Channel, 0, len(in))
	for _, c := range in {
		out = append(out, reminder_rule.Channel(c))
	}
	return out
}

func (h *ReminderRuleHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CreateReminderRuleRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	rule, err := h.service.Create(ctx, tenantID, service.CreateRuleInput{
		Name:        req.Name,
		Description: req.Description,
		TriggerType: reminder_rule.TriggerType(req.TriggerType),
		OffsetDays:  req.OffsetDays,
		Channels:    toChannels(req.Channels),
		Template:    req.Template,
		Enabled:     req.Enabled,
	})
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Created(c, "Reminder rule created", rule)
}

func (h *ReminderRuleHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	items, err := h.service.ListByTenant(ctx, tenantID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Reminder rules retrieved", items)
}

func (h *ReminderRuleHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	rule, err := h.service.GetByID(ctx, tenantID, c.Param("id"))
	if err != nil || rule == nil {
		response.NotFound(c, "reminder rule not found")
		return
	}
	response.Success(c, "Reminder rule retrieved", rule)
}

func (h *ReminderRuleHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UpdateReminderRuleRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	in := service.UpdateRuleInput{
		Name:        req.Name,
		Description: req.Description,
		OffsetDays:  req.OffsetDays,
		Template:    req.Template,
		Enabled:     req.Enabled,
	}
	if req.TriggerType != nil {
		tt := reminder_rule.TriggerType(*req.TriggerType)
		in.TriggerType = &tt
	}
	if req.Channels != nil {
		in.Channels = toChannels(req.Channels)
	}
	rule, err := h.service.Update(ctx, tenantID, c.Param("id"), in)
	if err != nil {
		if err.Error() == "reminder rule not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Reminder rule updated", rule)
}

func (h *ReminderRuleHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	if err := h.service.Delete(ctx, tenantID, c.Param("id")); err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Reminder rule deleted", nil)
}

// RunNow lets an operator manually trigger the engine for the current tenant
// without waiting for the daily cron. Returns the EvaluationResult so the
// UI can show how many notifications went out.
func (h *ReminderRuleHandler) RunNow(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	res, err := h.service.EvaluateAndDispatch(ctx, tenantID, time.Now())
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Régua avaliada", res)
}
