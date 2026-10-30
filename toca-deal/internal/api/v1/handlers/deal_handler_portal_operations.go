package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
	repository "github.com/oginoino/toca/toca-deal/internal/repositories"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

// Portal appointments, charges, attachments, maintenance and support tickets.

// DownloadMyDealAppointmentCalendar entrega o .ics de um compromisso marcado do
// próprio cliente. Mesma autorização do restante do portal: o negócio precisa
// ser dele e o compromisso estar no escopo dos leads dele.
func (h *DealHandler) DownloadMyDealAppointmentCalendar(c *gin.Context) {
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	file, name, err := h.service.MyDealAppointmentCalendar(c.Request.Context(), accountID, email, c.Param("id"), c.Param("appointmentId"))
	if err != nil {
		switch {
		case errors.Is(err, services.ErrPortalCalendarNotAvailable):
			c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Este compromisso ainda não está marcado. Assim que o corretor confirmar, o arquivo fica disponível."})
		case err.Error() == "deal not found or access denied":
			response.Forbidden(c, "access denied")
		default:
			response.Error(c, http.StatusServiceUnavailable, clients.ErrCalendarUnavailable.Error())
		}
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", file)
}

func (h *DealHandler) GetMyDealAppointments(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	appts, err := h.service.GetMyDealAppointments(c.Request.Context(), accountID, email, id)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		response.Error(c, http.StatusServiceUnavailable, clients.ErrCalendarUnavailable.Error())
		return
	}
	out := make([]*portal.Appointment, 0, len(appts))
	for _, a := range appts {
		if a == nil {
			continue
		}
		out = append(out, &portal.Appointment{
			CanManage:   a.CanManage,
			UpdatedAt:   a.UpdatedAt,
			ID:          a.ID,
			Title:       a.Title,
			Date:        a.Date,
			Time:        a.Time,
			EndTime:     a.EndTime,
			Type:        a.Type,
			Status:      a.Status,
			Description: a.Description,
			Property:    a.Property,
			CreatedAt:   a.CreatedAt,
		})
	}
	response.Success(c, "Appointments retrieved successfully", out)
}

func (h *DealHandler) GetMyDealCharges(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	charges, err := h.service.GetMyDealCharges(c.Request.Context(), accountID, email, id)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Charges retrieved successfully", charges)
}

type GetAttachmentUploadURLRequest struct {
	FileName    string `json:"file_name" binding:"required"`
	ContentType string `json:"content_type"`
	FileSize    int64  `json:"file_size"`
}

type CompleteAttachmentUploadRequest struct {
	StorageKey string `json:"storage_key" binding:"required"`
}

func (h *DealHandler) GetMyDealMaintenanceAttachmentUploadURL(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req GetAttachmentUploadURLRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	out, err := h.service.GetMyDealMaintenanceAttachmentUploadURL(c.Request.Context(), accountID, email, id, req.FileName, req.ContentType, req.FileSize)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "storage service not configured" || strings.Contains(err.Error(), "presigned PUT not supported") {
			response.Conflict(c, "direct upload unavailable")
			return
		}
		response.InternalServerError(c, "failed to generate upload URL")
		return
	}
	response.Success(c, "Presigned upload URL generated", out)
}

func (h *DealHandler) CompleteMyDealMaintenanceAttachmentUpload(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req CompleteAttachmentUploadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	out, err := h.service.CompleteMyDealMaintenanceAttachmentUpload(c.Request.Context(), accountID, email, id, req.StorageKey)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Attachment upload completed", out)
}

func (h *DealHandler) GetMyTicketAttachmentUploadURL(c *gin.Context) {
	ticketID := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req GetAttachmentUploadURLRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	out, err := h.service.GetMyTicketAttachmentUploadURL(c.Request.Context(), accountID, email, ticketID, req.FileName, req.ContentType, req.FileSize)
	if err != nil {
		if err.Error() == "ticket not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "storage service not configured" || strings.Contains(err.Error(), "presigned PUT not supported") {
			response.Conflict(c, "direct upload unavailable")
			return
		}
		response.InternalServerError(c, "failed to generate upload URL")
		return
	}
	response.Success(c, "Presigned upload URL generated", out)
}

func (h *DealHandler) CompleteMyTicketAttachmentUpload(c *gin.Context) {
	ticketID := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req CompleteAttachmentUploadRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	out, err := h.service.CompleteMyTicketAttachmentUpload(c.Request.Context(), accountID, email, ticketID, req.StorageKey)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Attachment upload completed", out)
}

func (h *DealHandler) SearchMyDealProperties(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	limit := int64(20)
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			limit = n
		}
	}
	offset := int64(0)
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			offset = n
		}
	}

	var minBedrooms *int
	if v := c.Query("min_bedrooms"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			minBedrooms = &n
		}
	}

	var minPrice *float64
	if v := c.Query("min_price"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			minPrice = &n
		}
	}

	var maxPrice *float64
	if v := c.Query("max_price"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil {
			maxPrice = &n
		}
	}

	params := repository.SearchParams{
		Query:       c.Query("q"),
		ListingType: c.Query("listing_type"),
		MinBedrooms: minBedrooms,
		MinPrice:    minPrice,
		MaxPrice:    maxPrice,
		Limit:       limit,
		Offset:      offset,
	}

	props, err := h.service.SearchMyDealProperties(c.Request.Context(), accountID, email, id, params)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Properties retrieved successfully", props)
}

func (h *DealHandler) GetMyDealMaintenanceTickets(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	tickets, err := h.service.GetMyDealMaintenanceTickets(c.Request.Context(), accountID, email, id)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Maintenance tickets retrieved successfully", tickets)
}

func (h *DealHandler) CreateMyDealMaintenanceTicket(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	name, _ := context.GetUserName(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req portal.CreateMaintenanceTicketRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	ticket, err := h.service.CreateMyDealMaintenanceTicket(c.Request.Context(), accountID, email, name, id, req)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Maintenance ticket created successfully", ticket)
}

func (h *DealHandler) GetMyDealTickets(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	tickets, err := h.service.GetMyDealTickets(c.Request.Context(), accountID, email, id)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Tickets retrieved successfully", tickets)
}

func (h *DealHandler) CreateMyDealTicket(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	name, _ := context.GetUserName(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req portal.CreateTicketRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	t, err := h.service.CreateMyDealTicket(c.Request.Context(), accountID, email, name, id, req)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Ticket created successfully", t)
}

func (h *DealHandler) GetMyTicket(c *gin.Context) {
	ticketID := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	t, err := h.service.GetMyTicket(c.Request.Context(), accountID, email, ticketID)
	if err != nil {
		respondPortalError(c, err)
		return
	}
	response.Success(c, "Ticket retrieved successfully", t)
}

func (h *DealHandler) AddMyTicketComment(c *gin.Context) {
	ticketID := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	name, _ := context.GetUserName(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req portal.AddTicketCommentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	comment, err := h.service.AddMyTicketComment(c.Request.Context(), accountID, email, name, ticketID, req)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Comment added successfully", comment)
}

func (h *DealHandler) CreateMyDealAppointment(c *gin.Context) {
	id := c.Param("id")
	accountID, _ := context.GetAccountID(c.Request.Context())
	email, _ := context.GetEmail(c.Request.Context())
	name, _ := context.GetUserName(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	var req portal.CreateAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	appt, err := h.service.CreateMyDealAppointment(c.Request.Context(), accountID, email, name, id, req)
	if err != nil {
		if err.Error() == "deal not found or access denied" {
			response.Forbidden(c, "access denied")
			return
		}
		if err.Error() == "date, time and end_time are required" {
			response.ValidationError(c, err.Error())
			return
		}
		if errors.Is(err, clients.ErrCalendarConflict) {
			response.Error(c, http.StatusConflict, clients.ErrCalendarConflict.Error())
			return
		}
		if errors.Is(err, clients.ErrCalendarInvalid) {
			response.ValidationError(c, err.Error())
			return
		}
		if errors.Is(err, clients.ErrCalendarUnavailable) || err.Error() == "property lookup unavailable" {
			response.Error(c, http.StatusServiceUnavailable, clients.ErrCalendarUnavailable.Error())
			return
		}
		if err.Error() == "property not found" {
			response.NotFound(c, "Imóvel não encontrado.")
			return
		}
		response.InternalServerError(c, "failed to create appointment")
		return
	}
	response.Success(c, "Appointment created successfully", appt)
}

// CreateDealAppointment agenda (operador/corretor) uma visita para um
// negócio. O cliente acompanha no portal (GET /client-portal/deals/:id/appointments).
func (h *DealHandler) CreateDealAppointment(c *gin.Context) {
	id := c.Param("id")
	var req portal.CreateAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	// N15 (reauditoria de 2026-09-20): a agenda é sub-recurso do negócio e o
	// plano de operador é recortado por corretor (achado A25). Este handler não
	// aplicava escopo nenhum e o serviço resolve o negócio só por tenant — um
	// `corretor` agendava visita em negócio de OUTRO corretor do mesmo tenant.
	// Mesma regra dos irmãos (contratos/documentos): confere o vínculo pelo
	// escopo antes de escrever.
	brokerScope, _, err := h.scopedBrokerID(c)
	if err != nil {
		response.Unauthorized(c, "unauthorized")
		return
	}
	if _, err := h.service.GetByID(c.Request.Context(), tenantID, id, brokerScope); err != nil {
		response.NotFound(c, "negócio não encontrado")
		return
	}
	appt, err := h.service.CreateDealAppointment(c.Request.Context(), tenantID, id, req)
	if err != nil {
		if err.Error() == "deal not found" {
			response.NotFound(c, "negócio não encontrado")
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Created(c, "Visita agendada com sucesso", appt)
}

// SetRecommendedProperty registra o imóvel recomendado pelo corretor ao cliente
// (o cliente vê como "Sugestão do seu corretor" no portal).
func (h *DealHandler) SetRecommendedProperty(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		PropertyID string `json:"property_id" binding:"required"`
	}
	if !validation.BindJSON(c, &req) {
		return
	}
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
	if err := h.service.SetRecommendedProperty(c.Request.Context(), tenantID, id, req.PropertyID, brokerID); err != nil {
		if err.Error() == "deal not found" {
			response.NotFound(c, "negócio não encontrado")
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Imóvel recomendado com sucesso", nil)
}
