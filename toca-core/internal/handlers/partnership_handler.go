package handlers

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
)

type PartnershipHandler struct {
	service *service.PartnershipService
}

func NewPartnershipHandler(service *service.PartnershipService) *PartnershipHandler {
	return &PartnershipHandler{
		service: service,
	}
}

// Create godoc
// @Summary      Create partnership
// @Description  Create a new partnership agreement
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreatePartnershipRequest true "Create Partnership Request"
// @Success      201  {object}  response.Response{data=partnership.Partnership}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships [post]
func (h *PartnershipHandler) Create(c *gin.Context) {
	var req CreatePartnershipRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	requesterID := c.GetString("tenant_id")
	if requesterID == "" {
		response.Unauthorized(c, "tenant_id not found in context")
		return
	}

	p := req.ToDomain(requesterID)

	if err := h.service.CreatePartnership(c.Request.Context(), p, c.GetString("user_id")); err != nil {
		respondAgreementError(c, err)
		return
	}

	response.Created(c, "Partnership created successfully", p)
}

// List godoc
// @Summary      List partnerships
// @Description  List partnerships for the current company context
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]partnership.Partnership}
// @Failure      500  {object}  response.Response
// @Router       /partnerships [get]
func (h *PartnershipHandler) List(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		response.Unauthorized(c, "tenant_id not found in context")
		return
	}

	// Fetch where tenant is either requester or target
	// Since service.List takes a filter map, we might need two queries or a complex filter.
	// MongoDB repository implementation takes a simple map which implies AND conditions.
	// So we can't do OR easily with that simple map.
	// But the service has ListPartnerships which calls repo.List.
	// I should probably add specific methods to service/repo for "GetByCompanyID" which does the OR query.
	// For now, I'll fetch both and merge in handler or (better) update repo.

	// Let's rely on adding a specialized method to repository later if needed.
	// For now, let's just fetch where requester_id = tenantID (as we are mostly interested in what we manage)
	// But we also want to see who manages us.

	// I will fetch both and merge.
	asRequester, err := h.service.ListPartnerships(c.Request.Context(), map[string]interface{}{"requester_id": tenantID})
	if err != nil {
		if isNotImplemented(err) {
			asRequester = []*partnership.Partnership{}
		} else {
			response.InternalError(c, err)
			return
		}
	}

	asTarget, err := h.service.ListPartnerships(c.Request.Context(), map[string]interface{}{"target_id": tenantID})
	if err != nil {
		if isNotImplemented(err) {
			asTarget = []*partnership.Partnership{}
		} else {
			response.InternalError(c, err)
			return
		}
	}

	// Merge unique
	unique := make(map[string]*partnership.Partnership)
	for _, p := range asRequester {
		unique[p.ID] = p
	}
	for _, p := range asTarget {
		unique[p.ID] = p
	}

	result := make([]*partnership.Partnership, 0, len(unique))
	for _, p := range unique {
		result = append(result, p)
	}

	response.Success(c, "Partnerships retrieved successfully", result)
}

// GetByID godoc
// @Summary      Get partnership by ID
// @Description  Get partnership details
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Success      200  {object}  response.Response{data=partnership.Partnership}
// @Failure      404  {object}  response.Response
// @Router       /partnerships/{id} [get]
func (h *PartnershipHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	p, err := h.service.GetPartnership(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	// Security check: ensure tenant is part of this partnership
	tenantID := c.GetString("tenant_id")
	if p.RequesterID != tenantID && p.TargetID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}

	response.Success(c, "Partnership retrieved successfully", p)
}

// UpdateFinancialConfig godoc
// @Summary      Update financial configuration
// @Description  Update royalties, fees, etc.
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Param        request body handlers.UpdateFinancialConfigRequest true "Update Config"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/{id}/financial-config [put]
func (h *PartnershipHandler) UpdateFinancialConfig(c *gin.Context) {
	id := c.Param("id")
	var req UpdateFinancialConfigRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")
	if err := h.service.UpdateFinancialConfig(c.Request.Context(), id, req.FinancialConfig, userID, tenantID); err != nil {
		if err.Error() == "access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Financial configuration updated", nil)
}

// UpdatePermissions godoc
// @Summary      Update permissions
// @Description  Update access rights for the partner
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Param        request body handlers.UpdatePermissionsRequest true "Update Permissions"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/{id}/permissions [put]
func (h *PartnershipHandler) UpdatePermissions(c *gin.Context) {
	id := c.Param("id")
	var req UpdatePermissionsRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")
	if err := h.service.UpdatePermissions(c.Request.Context(), id, req.Permissions, userID, tenantID); err != nil {
		if err.Error() == "access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Permissions updated", nil)
}

// UpdateSharingConfig godoc
// @Summary      Update sharing configuration
// @Description  Update the partnership's shared-portfolio rules (owner-contact PII visibility, co-brokering, property type/region whitelists)
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Param        request body handlers.UpdateSharingConfigRequest true "Update Sharing Config"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/{id}/sharing-config [put]
func (h *PartnershipHandler) UpdateSharingConfig(c *gin.Context) {
	id := c.Param("id")
	var req UpdateSharingConfigRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	userID := c.GetString("user_id")
	tenantID := c.GetString("tenant_id")
	if err := h.service.UpdateSharingConfig(c.Request.Context(), id, req.SharingConfig, userID, tenantID); err != nil {
		if err.Error() == "access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "partnership not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Sharing configuration updated", nil)
}

// Accept godoc
// @Summary      Accept partnership invitation
// @Description  The invited (target) company accepts a pending partnership/franchise invitation, moving it to active
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Success      200  {object}  response.Response{data=partnership.Partnership}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/{id}/accept [post]
func (h *PartnershipHandler) Accept(c *gin.Context) {
	id := c.Param("id")

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		response.Unauthorized(c, "tenant_id not found in context")
		return
	}
	userID := c.GetString("user_id")
	if userID == "" {
		userID = tenantID
	}

	p, err := h.service.AcceptPartnership(c.Request.Context(), id, tenantID, userID)
	if err != nil {
		switch err.Error() {
		case "partnership not found":
			response.NotFound(c, err.Error())
		case "access denied":
			response.Forbidden(c, err.Error())
		case "partnership is not pending":
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Partnership invitation accepted", p)
}

// Reject godoc
// @Summary      Reject partnership invitation
// @Description  The invited (target) company rejects a pending partnership/franchise invitation, moving it to inactive
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Success      200  {object}  response.Response{data=partnership.Partnership}
// @Failure      400  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/{id}/reject [post]
func (h *PartnershipHandler) Reject(c *gin.Context) {
	id := c.Param("id")

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		response.Unauthorized(c, "tenant_id not found in context")
		return
	}
	userID := c.GetString("user_id")
	if userID == "" {
		userID = tenantID
	}

	p, err := h.service.RejectPartnership(c.Request.Context(), id, tenantID, userID)
	if err != nil {
		switch err.Error() {
		case "partnership not found":
			response.NotFound(c, err.Error())
		case "access denied":
			response.Forbidden(c, err.Error())
		case "partnership is not pending":
			response.Conflict(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Partnership invitation rejected", p)
}

// SharedPortfolio godoc
// @Summary      Get the partner's shared portfolio
// @Description  Returns the COUNTERPART company's shared properties for an active partnership the caller belongs to. Each property is privacy-projected (PII stripped, address coarsened). Requires the partnership to grant shared_portfolio view.
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Partnership ID"
// @Success      200  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/{id}/shared-portfolio [get]
func (h *PartnershipHandler) SharedPortfolio(c *gin.Context) {
	id := c.Param("id")

	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		response.Unauthorized(c, "tenant_id not found in context")
		return
	}

	props, err := h.service.SharedPortfolioForPartnership(c.Request.Context(), id, tenantID)
	if err != nil {
		switch {
		case err.Error() == "partnership not found":
			response.NotFound(c, err.Error())
		case err.Error() == "access denied":
			response.Forbidden(c, err.Error())
		case errors.Is(err, service.ErrSharedPortfolioUnavailable):
			response.Forbidden(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Shared portfolio retrieved successfully", props)
}

// AllSharedProperties godoc
// @Summary      List all partner-shared properties
// @Description  Aggregates the shared portfolio across every ACTIVE partnership the caller participates in (and that grants shared_portfolio view). Each property is privacy-projected; the response carries the originating partnership id and source company id.
// @Tags         partnerships
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=[]service.SharedProperty}
// @Failure      403  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /partnerships/shared/properties [get]
func (h *PartnershipHandler) AllSharedProperties(c *gin.Context) {
	tenantID := c.GetString("tenant_id")
	if tenantID == "" {
		response.Unauthorized(c, "tenant_id not found in context")
		return
	}

	props, err := h.service.AllSharedPropertiesForCompany(c.Request.Context(), tenantID)
	if err != nil {
		if err.Error() == "access denied" {
			response.Forbidden(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Shared properties retrieved successfully", props)
}

func respondAgreementError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAgreementAccess):
		response.Forbidden(c, "Sem permissão para alterar este acordo.")
	case errors.Is(err, partnership.ErrConflict), errors.Is(err, partnership.ErrDuplicate), errors.Is(err, service.ErrAgreementVersion):
		response.Conflict(c, err.Error())
	case errors.Is(err, partnership.ErrNotFound):
		response.NotFound(c, err.Error())
	case errors.Is(err, service.ErrAgreementValidation), errors.Is(err, partnership.ErrCompany):
		response.ValidationError(c, err.Error())
	default:
		response.InternalError(c, err)
	}
}
func (h *PartnershipHandler) ChangeAgreement(c *gin.Context) {
	var req service.AgreementChange
	if !validation.BindJSON(c, &req) {
		return
	}
	p, err := h.service.ChangeAgreement(c.Request.Context(), c.Param("id"), c.GetString("tenant_id"), c.GetString("user_id"), req)
	if err != nil {
		respondAgreementError(c, err)
		return
	}
	response.Success(c, "Acordo atualizado", p)
}
func (h *PartnershipHandler) LegacyMutation(c *gin.Context) {
	response.Conflict(c, "Recarregue a página para revisar e enviar a versão atual do acordo.")
}
