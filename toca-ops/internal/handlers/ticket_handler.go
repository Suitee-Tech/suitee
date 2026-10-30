package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-ops/internal/models/ticket"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

type TicketHandler struct {
	service *services.TicketService
}

func NewTicketHandler(svc *services.TicketService) *TicketHandler {
	return &TicketHandler{service: svc}
}

type CreateTicketRequest struct {
	TenantID           string                `json:"tenant_id,omitempty"`
	DealID             string                `json:"deal_id,omitempty"`
	PropertyID         string                `json:"property_id,omitempty"`
	Type               ticket.TicketType     `json:"type"`
	Category           string                `json:"category"`
	Subject            string                `json:"subject" binding:"required"`
	Description        string                `json:"description,omitempty"`
	Priority           ticket.TicketPriority `json:"priority"`
	RequesterAccountID string                `json:"requester_account_id" binding:"required"`
	RequesterName      string                `json:"requester_name,omitempty"`
	RequesterEmail     string                `json:"requester_email,omitempty"`
}

type UpdateStatusRequest struct {
	Status ticket.TicketStatus `json:"status" binding:"required"`
}

type AssignRequest struct {
	AssigneeID   string `json:"assignee_id" binding:"required"`
	AssigneeName string `json:"assignee_name,omitempty"`
}

type AddCommentRequest struct {
	AuthorID        string   `json:"author_id" binding:"required"`
	AuthorName      string   `json:"author_name,omitempty"`
	AuthorType      string   `json:"author_type" binding:"required"`
	Visibility      string   `json:"visibility,omitempty"`
	Content         string   `json:"content" binding:"required"`
	Attachments     []string `json:"attachments,omitempty"`
	ClientRequestID string   `json:"client_request_id,omitempty"`
}

func (h *TicketHandler) Create(c *gin.Context) {
	var req CreateTicketRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, ok := tenantForRequest(c, req.TenantID)
	if !ok {
		return
	}

	t, err := h.service.Create(c.Request.Context(), services.CreateTicketRequest{
		TenantID:           tenantID,
		DealID:             req.DealID,
		PropertyID:         req.PropertyID,
		Type:               req.Type,
		Category:           req.Category,
		Subject:            req.Subject,
		Description:        req.Description,
		Priority:           req.Priority,
		RequesterAccountID: req.RequesterAccountID,
		RequesterName:      req.RequesterName,
		RequesterEmail:     req.RequesterEmail,
	})
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Created(c, "Ticket created successfully", t)
}

func (h *TicketHandler) List(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}

	filters := ticket.ListFilters{
		Status:             c.Query("status"),
		Type:               c.Query("type"),
		AssigneeID:         c.Query("assignee_id"),
		RequesterAccountID: c.Query("requester_account_id"),
		DealID:             c.Query("deal_id"),
		Limit:              limit,
		Offset:             offset,
	}

	tickets, total, err := h.service.ListForOperator(c.Request.Context(), tenantID, filters)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Tickets retrieved successfully", []*ticket.Ticket{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}
	page := int((offset / limit) + 1)
	response.Paginated(c, "Tickets retrieved successfully", tickets, page, int(limit), total)
}

func (h *TicketHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}
	t, err := h.service.GetByIDForOperator(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Ticket retrieved successfully", t)
}

func (h *TicketHandler) GetByIDInternal(c *gin.Context) {
	id := c.Param("id")
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	t, err := h.service.GetByIDInternal(c.Request.Context(), tenantID, id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, "Ticket retrieved successfully", t)
}

func (h *TicketHandler) UpdateStatus(c *gin.Context) {
	id := c.Param("id")
	var req UpdateStatusRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}

	t, err := h.service.UpdateStatus(c.Request.Context(), tenantID, id, req.Status)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Ticket status updated successfully", t)
}

func (h *TicketHandler) Assign(c *gin.Context) {
	id := c.Param("id")
	var req AssignRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		tenantID = c.Query("tenant_id")
	}
	if tenantID == "" {
		response.Forbidden(c, "tenant context required")
		return
	}

	t, err := h.service.Assign(c.Request.Context(), tenantID, id, req.AssigneeID, req.AssigneeName)
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}
	response.Success(c, "Ticket assigned successfully", t)
}

func (h *TicketHandler) AddComment(c *gin.Context) {
	id := c.Param("id")
	var req AddCommentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, ok := tenantForRequest(c, c.Query("tenant_id"))
	if !ok {
		return
	}

	comment, err := h.service.AddComment(c.Request.Context(), tenantID, id, services.AddCommentRequest{
		AuthorID:        req.AuthorID,
		AuthorName:      req.AuthorName,
		AuthorType:      req.AuthorType,
		Visibility:      req.Visibility,
		Content:         req.Content,
		Attachments:     req.Attachments,
		ClientRequestID: req.ClientRequestID,
	})
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	response.Created(c, "Comment added successfully", comment)
}

func (h *TicketHandler) ListByRequester(c *gin.Context) {
	accountID := c.Param("accountID")
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}

	tickets, err := h.service.ListByRequester(c.Request.Context(), tenantID, accountID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Tickets retrieved successfully", tickets)
}
