package handlers

import (
	"math"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-financial/internal/domain/utility_account"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type UtilityAccountHandler struct {
	service *service.UtilityAccountService
}

func NewUtilityAccountHandler(s *service.UtilityAccountService) *UtilityAccountHandler {
	return &UtilityAccountHandler{service: s}
}

type CreateUtilityAccountRequest struct {
	ContractID       string     `json:"contract_id" binding:"required"`
	PropertyID       string     `json:"property_id" binding:"required"`
	Type             string     `json:"type" binding:"required"`
	Provider         string     `json:"provider" binding:"required"`
	AccountNumber    string     `json:"account_number"`
	ResponsibleParty string     `json:"responsible_party" binding:"required"`
	EstimatedAmount  float64    `json:"estimated_amount"`
	PaymentMethod    string     `json:"payment_method"`
	AutoPayEnabled   bool       `json:"auto_pay_enabled"`
	DueDay           int        `json:"due_day"`
	NextDueDate      *time.Time `json:"next_due_date"`
	Notes            string     `json:"notes"`
	BillingMode      string     `json:"billing_mode"`
	BundledItemKey   string     `json:"bundled_item_key"`
}

type UpdateUtilityAccountRequest struct {
	Provider         *string    `json:"provider"`
	AccountNumber    *string    `json:"account_number"`
	ResponsibleParty *string    `json:"responsible_party"`
	EstimatedAmount  *float64   `json:"estimated_amount"`
	PaymentMethod    *string    `json:"payment_method"`
	AutoPayEnabled   *bool      `json:"auto_pay_enabled"`
	DueDay           *int       `json:"due_day"`
	NextDueDate      *time.Time `json:"next_due_date"`
	LastPaymentDate  *time.Time `json:"last_payment_date"`
	Status           *string    `json:"status"`
	Notes            *string    `json:"notes"`
	BillingMode      *string    `json:"billing_mode"`
	BundledItemKey   *string    `json:"bundled_item_key"`
}

func (h *UtilityAccountHandler) Create(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req CreateUtilityAccountRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	estCents := int64(math.Round(req.EstimatedAmount * 100))
	in := service.CreateUtilityAccountInput{
		ContractID:       req.ContractID,
		PropertyID:       req.PropertyID,
		Type:             utility_account.Type(req.Type),
		Provider:         req.Provider,
		AccountNumber:    req.AccountNumber,
		ResponsibleParty: utility_account.ResponsibleParty(req.ResponsibleParty),
		EstimatedAmount:  estCents,
		PaymentMethod:    req.PaymentMethod,
		AutoPayEnabled:   req.AutoPayEnabled,
		DueDay:           req.DueDay,
		NextDueDate:      req.NextDueDate,
		Notes:            req.Notes,
		BillingMode:      utility_account.BillingMode(req.BillingMode),
		BundledItemKey:   req.BundledItemKey,
	}
	a, err := h.service.Create(ctx, tenantID, in)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Created(c, "Utility account created", a)
}

func (h *UtilityAccountHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	contractID := c.Query("contract_id")
	if contractID == "" {
		response.ValidationError(c, "contract_id query param is required")
		return
	}
	items, err := h.service.ListByContract(ctx, tenantID, contractID)
	if err != nil {
		respondInternalError(c, err)
		return
	}
	response.Success(c, "Utility accounts retrieved", items)
}

func (h *UtilityAccountHandler) GetByID(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	a, err := h.service.GetByID(ctx, tenantID, c.Param("id"))
	if err != nil || a == nil {
		response.NotFound(c, "utility account not found")
		return
	}
	response.Success(c, "Utility account retrieved", a)
}

func (h *UtilityAccountHandler) Update(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := context.GetTenantID(ctx)
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	var req UpdateUtilityAccountRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	in := service.UpdateUtilityAccountInput{
		Provider:        req.Provider,
		AccountNumber:   req.AccountNumber,
		PaymentMethod:   req.PaymentMethod,
		AutoPayEnabled:  req.AutoPayEnabled,
		DueDay:          req.DueDay,
		NextDueDate:     req.NextDueDate,
		LastPaymentDate: req.LastPaymentDate,
		Notes:           req.Notes,
		BundledItemKey:  req.BundledItemKey,
	}
	if req.ResponsibleParty != nil {
		rp := utility_account.ResponsibleParty(*req.ResponsibleParty)
		in.ResponsibleParty = &rp
	}
	if req.EstimatedAmount != nil {
		cents := int64(math.Round(*req.EstimatedAmount * 100))
		in.EstimatedAmount = &cents
	}
	if req.Status != nil {
		st := utility_account.Status(*req.Status)
		in.Status = &st
	}
	if req.BillingMode != nil {
		mode := utility_account.BillingMode(*req.BillingMode)
		in.BillingMode = &mode
	}
	a, err := h.service.Update(ctx, tenantID, c.Param("id"), in)
	if err != nil {
		if err.Error() == "utility account not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Utility account updated", a)
}

func (h *UtilityAccountHandler) Delete(c *gin.Context) {
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
	response.Success(c, "Utility account deleted", nil)
}
