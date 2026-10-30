package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ops/internal/models/key"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

type KeyHandler struct {
	service *services.KeyService
}

func NewKeyHandler(svc *services.KeyService) *KeyHandler {
	return &KeyHandler{service: svc}
}

type CreateKeyRequest struct {
	PropertyID string `json:"property_id" binding:"required"`
	Code       string `json:"code" binding:"required"`
	Location   string `json:"location" binding:"required"`
	Notes      string `json:"notes"`
}

type CheckOutRequest struct {
	CheckedOutBy string `json:"checked_out_by" binding:"required"`
}

func (h *KeyHandler) Create(c *gin.Context) {
	var req CreateKeyRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	k := key.New(req.PropertyID, req.Code, req.Location, req.Notes, tenantID)

	if err := h.service.Create(c.Request.Context(), k); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Key created successfully", k)
}

func (h *KeyHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	k, err := h.service.GetByID(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Key retrieved successfully", k)
}

func (h *KeyHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if err := h.service.Delete(c.Request.Context(), tenantID, id); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Key deleted successfully", nil)
}

func (h *KeyHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	propertyID := c.Query("property_id")
	status := c.Query("status")

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	keys, total, err := h.service.List(c.Request.Context(), tenantID, propertyID, status, limit, offset)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Keys retrieved successfully", []*key.Key{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Keys retrieved successfully", keys, page, int(limit), total)
}

func (h *KeyHandler) CheckOut(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req CheckOutRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	k, err := h.service.CheckOut(c.Request.Context(), tenantID, id, req.CheckedOutBy)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	response.Success(c, "Key checked out successfully", k)
}

func (h *KeyHandler) CheckIn(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id); err != nil {
		response.NotFound(c, err.Error())
		return
	}

	k, err := h.service.CheckIn(c.Request.Context(), tenantID, id)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	response.Success(c, "Key checked in successfully", k)
}
