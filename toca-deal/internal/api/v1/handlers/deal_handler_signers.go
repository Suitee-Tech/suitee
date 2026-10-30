package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Signer endpoints own signer mutation and portal invitation orchestration.

type AddSignerRequest struct {
	Name       string             `json:"name" binding:"required"`
	Email      string             `json:"email" binding:"required,email"`
	Role       deal.SignatoryRole `json:"role" binding:"required"`
	IsRequired *bool              `json:"is_required,omitempty"`
}

// AddSigner godoc
// @Summary      Add signer
// @Description  Add a signer to the deal
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        request body handlers.AddSignerRequest true "Add Signer Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/signers [post]
func (h *DealHandler) AddSigner(c *gin.Context) {
	id := c.Param("id")
	var req AddSignerRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.AddSigner(c.Request.Context(), tenantID, id, brokerID, req.Name, req.Email, req.Role, req.IsRequired, userID)
	if err != nil {
		respondSignerError(c, err)
		return
	}

	response.Success(c, "Signer added successfully", d)
}

type UpdateSignerRequest struct {
	IsRequired *bool               `json:"is_required,omitempty"`
	Name       *string             `json:"name,omitempty"`
	Email      *string             `json:"email,omitempty"`
	Role       *deal.SignatoryRole `json:"role,omitempty"`
}

// UpdateSigner updates editable signer fields (configuration). Distinct from
// UpdateSignerStatus which is reserved for the signature flow.
// @Router       /deals/{id}/signers/{signerId} [put]
func (h *DealHandler) UpdateSigner(c *gin.Context) {
	id := c.Param("id")
	signerID := c.Param("signerId")
	var req UpdateSignerRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.UpdateSigner(c.Request.Context(), tenantID, id, brokerID, signerID, service.SignerUpdate{
		IsRequired: req.IsRequired,
		Name:       req.Name,
		Email:      req.Email,
		Role:       req.Role,
	}, userID)
	if err != nil {
		respondSignerError(c, err)
		return
	}

	response.Success(c, "Signer updated successfully", d)
}

type UpdateSignerStatusRequest struct {
	Status       deal.SignerStatus `json:"status" binding:"required"`
	SignatureURL string            `json:"signature_url"`
}

// UpdateSignerStatus godoc
// @Summary      Update signer status
// @Description  Update the status of a signer
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        signerId path string true "Signer ID"
// @Param        request body handlers.UpdateSignerStatusRequest true "Update Signer Status Request"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/signers/{signerId} [patch]
func (h *DealHandler) UpdateSignerStatus(c *gin.Context) {
	id := c.Param("id")
	signerID := c.Param("signerId")
	var req UpdateSignerStatusRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.UpdateSignerStatus(c.Request.Context(), tenantID, id, brokerID, signerID, req.Status, req.SignatureURL, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Signer status updated successfully", d)
}

// RemoveSigner godoc
// @Summary      Remove signer
// @Description  Remove a signer from the deal
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        signerId path string true "Signer ID"
// @Success      200  {object}  response.Response{data=deal.Deal}
// @Failure      400  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/signers/{signerId} [delete]
func (h *DealHandler) RemoveSigner(c *gin.Context) {
	id := c.Param("id")
	signerID := c.Param("signerId")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	brokerID, userID, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	d, err := h.service.RemoveSigner(c.Request.Context(), tenantID, id, brokerID, signerID, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Signer removed successfully", d)
}

// InviteSigner godoc
// @Summary      Invite signer
// @Description  Send an invitation to a deal signer
// @Tags         deals
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Deal ID"
// @Param        signerId path string true "Signer ID"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /deals/{id}/signers/{signerId}/invite [post]
func (h *DealHandler) InviteSigner(c *gin.Context) {
	id := c.Param("id")
	signerID := c.Param("signerId")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	brokerID, _, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}

	if err := h.service.InviteSigner(c.Request.Context(), tenantID, id, brokerID, signerID); err != nil {
		if err.Error() == "deal not found" || err.Error() == "signer not found" {
			response.NotFound(c, err.Error())
		} else if err.Error() == "signer must have an email to be invited" {
			response.Error(c, 400, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Signer invited successfully", nil)
}

// respondSignerError traduz os erros do cadastro de signatário. Nome sem
// sobrenome é erro de ENTRADA (o operador corrige o campo), não 500: era o que
// acontecia ao adicionar um signatário vindo de um lead monônimo — a esteira
// devolvia "erro interno" sem dizer qual campo ajustar.
func respondSignerError(c *gin.Context, err error) {
	if validation.IsPersonNameError(err) {
		response.ValidationError(c, err.Error())
		return
	}
	response.InternalError(c, err)
}
