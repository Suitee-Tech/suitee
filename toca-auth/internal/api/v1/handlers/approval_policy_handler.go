package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oginoino/toca-auth/internal/domain/tenantsettings"
	"github.com/oginoino/toca-auth/internal/service"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
)

type ApprovalPolicyHandler struct {
	service *service.ApprovalPolicyService
}

func NewApprovalPolicyHandler(policyService *service.ApprovalPolicyService) *ApprovalPolicyHandler {
	return &ApprovalPolicyHandler{service: policyService}
}

type updateApprovalPolicyRequest struct {
	Enabled      *bool    `json:"enabled"`
	ThresholdBRL *float64 `json:"threshold_brl"`
}

// GetInternal serves the authenticated service-to-service contract consumed
// by toca-deal. Tenant scope comes from the path because internal-key calls do
// not carry an end-user JWT.
func (h *ApprovalPolicyHandler) GetInternal(c *gin.Context) {
	tenantID := c.Param("tenant_id")
	if _, err := uuid.Parse(tenantID); err != nil {
		response.ValidationError(c, "valid tenant_id is required")
		return
	}

	policy, err := h.service.Get(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalServerError(c, "failed to load approval policy")
		return
	}
	response.Success(c, "Approval policy retrieved", policy)
}

func (h *ApprovalPolicyHandler) GetCurrent(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	policy, err := h.service.Get(c.Request.Context(), tenantID)
	if err != nil {
		response.InternalServerError(c, "failed to load approval policy")
		return
	}
	response.Success(c, "Approval policy retrieved", policy)
}

func (h *ApprovalPolicyHandler) UpdateCurrent(c *gin.Context) {
	tenantID, err := pkgcontext.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req updateApprovalPolicyRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	if req.Enabled == nil || req.ThresholdBRL == nil {
		response.ValidationError(c, "enabled and threshold_brl are required")
		return
	}

	candidate := tenantsettings.DefaultApprovalPolicy()
	candidate.Enabled = *req.Enabled
	candidate.ThresholdBRL = *req.ThresholdBRL
	if err := candidate.Validate(); err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	actorID, _ := pkgcontext.GetAccountID(c.Request.Context())
	policy, err := h.service.Update(c.Request.Context(), tenantID, service.UpdateApprovalPolicyInput{
		Enabled:      *req.Enabled,
		ThresholdBRL: *req.ThresholdBRL,
		UpdatedBy:    actorID,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "failed to update approval policy")
		return
	}
	response.Success(c, "Approval policy updated", policy)
}
