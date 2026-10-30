package handlers

import (
	"errors"
	"github.com/google/uuid"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"github.com/oginoino/toca/toca-calendar/internal/services"
)

type AppointmentHandler struct {
	service *services.AppointmentService
}

func NewAppointmentHandler(svc *services.AppointmentService) *AppointmentHandler {
	return &AppointmentHandler{service: svc}
}

type CreateAppointmentRequest struct {
	ID                string                 `json:"id,omitempty"`
	TenantID          string                 `json:"tenant_id,omitempty"`
	Title             string                 `json:"title" binding:"required"`
	Date              string                 `json:"date" binding:"required"`
	Time              string                 `json:"time" binding:"required"`
	EndTime           string                 `json:"end_time" binding:"required"`
	Type              appointment.Type       `json:"type" binding:"required"`
	Priority          appointment.Priority   `json:"priority"`
	Client            string                 `json:"client"`
	LeadID            string                 `json:"lead_id,omitempty"`
	Property          string                 `json:"property"`
	PropertyID        string                 `json:"property_id,omitempty"`
	Agent             string                 `json:"agent"`
	AgentID           string                 `json:"agent_id,omitempty"`
	DealID            string                 `json:"deal_id,omitempty"`
	CustomerAccountID string                 `json:"customer_account_id,omitempty"`
	Description       string                 `json:"description,omitempty"`
	Status            appointment.Status     `json:"status,omitempty"`
	Visibility        appointment.Visibility `json:"visibility,omitempty"`
}

type UpdateAppointmentRequest struct {
	TenantID          string                 `json:"tenant_id,omitempty"`
	CustomerAccountID string                 `json:"customer_account_id,omitempty"`
	Title             string                 `json:"title"`
	Date              string                 `json:"date"`
	Time              string                 `json:"time"`
	EndTime           string                 `json:"end_time"`
	Type              appointment.Type       `json:"type"`
	Priority          appointment.Priority   `json:"priority"`
	Client            string                 `json:"client"`
	LeadID            string                 `json:"lead_id,omitempty"`
	Property          *string                `json:"property"`
	PropertyID        *string                `json:"property_id,omitempty"`
	Agent             string                 `json:"agent"`
	AgentID           string                 `json:"agent_id,omitempty"`
	Status            appointment.Status     `json:"status"`
	DealID            *string                `json:"deal_id,omitempty"`
	ExpectedUpdatedAt *time.Time             `json:"expected_updated_at,omitempty"`
	Description       *string                `json:"description,omitempty"`
	Visibility        appointment.Visibility `json:"visibility,omitempty"`
}

func appointmentTeamScope(c *gin.Context) bool {
	return auth.HasTenantWideScope(c.GetString("permission"), c.GetString("functional_role"), c.GetString("base_role"))
}

func isLegacyPrivate(appt *appointment.Appointment) bool {
	return appt.Visibility == "" && appt.LeadID == "" && appt.DealID == "" &&
		appt.CustomerAccountID == "" && appt.PropertyID == ""
}

func canAccessAppointment(c *gin.Context, appt *appointment.Appointment) bool {
	accountID, err := context.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" || appt == nil {
		return false
	}
	if appt.AgentID == accountID {
		return true
	}
	return appointmentTeamScope(c) && appt.Visibility != appointment.VisibilityPrivate && !isLegacyPrivate(appt)
}

func (h *AppointmentHandler) Create(c *gin.Context) {
	h.create(c, false)
}

func (h *AppointmentHandler) InternalCreate(c *gin.Context) {
	h.create(c, true)
}

func (h *AppointmentHandler) create(c *gin.Context, internal bool) {
	var req CreateAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, ok := tenantForRequest(c, req.TenantID)
	if !ok {
		return
	}
	if internal {
		c.Request = c.Request.WithContext(context.WithTenantID(c.Request.Context(), tenantID))
	}

	priority := req.Priority
	if priority == "" {
		priority = appointment.PriorityMedia
	}

	appt := appointment.New(tenantID, req.Title, req.Date, req.Time, req.EndTime, req.Type, priority, req.Client, req.Agent)
	appt.LeadID = req.LeadID
	appt.Property = req.Property
	appt.PropertyID = req.PropertyID
	appt.AgentID = req.AgentID
	appt.DealID = req.DealID
	appt.CustomerAccountID = req.CustomerAccountID
	appt.Description = req.Description
	appt.Visibility = req.Visibility
	if appt.Visibility == "" {
		appt.Visibility = appointment.VisibilityTeam
	}
	if !internal {
		accountID, err := context.GetAccountID(c.Request.Context())
		if err != nil || accountID == "" {
			response.Forbidden(c, "account context required")
			return
		}
		if !appointmentTeamScope(c) || appt.AgentID == "" {
			appt.AgentID = accountID
		}
	}
	if req.Status != "" {
		appt.Status = req.Status
	}
	if internal && req.ID != "" {
		if _, err := uuid.Parse(req.ID); err != nil {
			response.ValidationError(c, "invalid appointment id")
			return
		}
		appt.ID = req.ID
	}

	var createErr error
	if internal && req.ID != "" {
		createErr = h.service.CreateIdempotent(c.Request.Context(), appt)
	} else {
		createErr = h.service.Create(c.Request.Context(), appt)
	}
	if err := createErr; err != nil {
		if errors.Is(err, appointment.ErrInvalid) {
			response.ValidationError(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrConflict) || errors.Is(err, appointment.ErrConcurrentModification) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Appointment created successfully", appt)
}

func (h *AppointmentHandler) InternalGetByID(c *gin.Context) {
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	c.Request = c.Request.WithContext(context.WithTenantID(c.Request.Context(), tenantID))
	appt, err := h.service.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			response.NotFound(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}
	if appt.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}
	response.Success(c, "Appointment retrieved", appt)
}

func (h *AppointmentHandler) GetByID(c *gin.Context) {
	id := c.Param("id")
	appt, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	if appt.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}
	if !canAccessAppointment(c, appt) {
		response.Forbidden(c, "access denied")
		return
	}

	response.Success(c, "Appointment retrieved successfully", appt)
}

func (h *AppointmentHandler) Update(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	var req UpdateAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}
	if !canAccessAppointment(c, existing) {
		response.Forbidden(c, "access denied")
		return
	}

	if req.ExpectedUpdatedAt != nil && !req.ExpectedUpdatedAt.Equal(existing.UpdatedAt) {
		response.Error(c, http.StatusConflict, appointment.ErrConcurrentModification.Error())
		return
	}
	copy := *existing
	existing = &copy
	if req.Title != "" {
		existing.Title = req.Title
	}
	if req.Date != "" {
		existing.Date = req.Date
	}
	if req.Time != "" {
		existing.Time = req.Time
	}
	if req.EndTime != "" {
		existing.EndTime = req.EndTime
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	if req.Priority != "" {
		existing.Priority = req.Priority
	}
	if req.Client != "" {
		existing.Client = req.Client
	}
	if req.LeadID != "" {
		existing.LeadID = req.LeadID
	}
	if req.PropertyID != nil {
		if existing.PropertyID != *req.PropertyID {
			existing.Property = ""
		}
		existing.PropertyID = *req.PropertyID
	}
	if req.Property != nil {
		existing.Property = *req.Property
	}
	if req.DealID != nil {
		existing.DealID = *req.DealID
	}
	if req.Agent != "" {
		existing.Agent = req.Agent
	}
	if req.AgentID != "" {
		if appointmentTeamScope(c) {
			existing.AgentID = req.AgentID
		}
	}
	if req.Status != "" {
		existing.Status = req.Status
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Visibility != "" {
		existing.Visibility = req.Visibility
	}

	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		if errors.Is(err, services.ErrInspectionManaged) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, appointment.ErrInvalid) {
			response.ValidationError(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrConflict) || errors.Is(err, appointment.ErrConcurrentModification) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Appointment updated successfully", existing)
}

func (h *AppointmentHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if existing.TenantID != tenantID || !canAccessAppointment(c, existing) {
		response.Forbidden(c, "access denied")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, tenantID); err != nil {
		if errors.Is(err, services.ErrInspectionManaged) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Appointment deleted successfully", nil)
}

func (h *AppointmentHandler) List(c *gin.Context) {
	tenantID, _ := context.GetTenantID(c.Request.Context())
	functionalRole, _ := c.Get("functional_role")
	roleStr, _ := functionalRole.(string)

	if tenantID == "" && roleStr != "cliente" {
		response.Forbidden(c, "tenant context required")
		return
	}

	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	if limit <= 0 {
		limit = 50
	}

	agentID := c.Query("agent_id")
	if agentID == "" {
		agentID = c.Query("agentId")
	}
	propertyID := c.Query("property_id")
	if propertyID == "" {
		propertyID = c.Query("propertyId")
	}
	startDate := c.Query("start_date")
	if startDate == "" {
		startDate = c.Query("startDate")
	}
	endDate := c.Query("end_date")
	if endDate == "" {
		endDate = c.Query("endDate")
	}

	leadID := c.Query("lead_id")
	if leadID == "" {
		leadID = c.Query("leadId")
	}

	filters := appointment.ListFilters{
		AgentID:    agentID,
		PropertyID: propertyID,
		LeadID:     leadID,
		StartDate:  startDate,
		EndDate:    endDate,
		Status:     c.Query("status"),
		Type:       c.Query("type"),
		ClientName: c.Query("client_name"),
		AgentName:  c.Query("agent_name"),
		Priority:   c.Query("priority"),
		Search:     c.Query("search"),
		Limit:      limit,
		Offset:     offset,
	}
	accountID, err := context.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" {
		response.Forbidden(c, "account context required")
		return
	}
	filters.AccessAgentID = accountID
	filters.TeamScope = c.Query("scope") == "team" && appointmentTeamScope(c)
	if !filters.TeamScope {
		// A visão padrão e todo corretor são sempre recortados para a própria agenda.
		filters.AgentID = accountID
	}

	appointments, total, err := h.service.List(c.Request.Context(), tenantID, filters)
	if err != nil {
		if isNotImplemented(err) {
			page := int((offset / limit) + 1)
			response.Paginated(c, "Appointments retrieved successfully", []*appointment.Appointment{}, page, int(limit), 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := int((offset / limit) + 1)
	response.Paginated(c, "Appointments retrieved successfully", appointments, page, int(limit), total)
}

// ListByDeal combines negotiation appointments with the client's earlier lead visits.
// Only the internally authenticated deal service supplies the authorized lead scope.
func (h *AppointmentHandler) ListByDeal(c *gin.Context) {
	dealID := c.Param("id")
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	filters := []appointment.ListFilters{{DealID: dealID}}
	leadIDs := map[string]bool{}
	for _, id := range c.QueryArray("lead_id") {
		id = strings.TrimSpace(id)
		if id != "" && !leadIDs[id] {
			filters = append(filters, appointment.ListFilters{LeadID: id})
			leadIDs[id] = true
		}
	}
	out := []*appointment.Appointment{}
	seen := map[string]bool{}
	for _, filter := range filters {
		filter.Limit = 100
		for {
			appointments, total, err := h.service.List(c.Request.Context(), tenantID, filter)
			if err != nil {
				response.InternalError(c, err)
				return
			}
			for _, a := range appointments {
				if a == nil || seen[a.ID] || a.Visibility == appointment.VisibilityPrivate {
					continue
				}
				// A lead can participate in several deals. Only bring unassigned earlier visits.
				if filter.LeadID != "" && a.DealID != "" {
					continue
				}
				out = append(out, a)
				seen[a.ID] = true
			}
			filter.Offset += int64(len(appointments))
			if filter.Offset >= total {
				break
			}
			if len(appointments) == 0 {
				response.InternalServerError(c, "incomplete appointment list")
				return
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.Date != right.Date {
			return left.Date > right.Date
		}
		if left.Time != right.Time {
			return left.Time > right.Time
		}
		return left.ID > right.ID
	})
	response.Success(c, "Appointments retrieved successfully", out)
}

// InternalUpdate atualiza um appointment via rota interna (admin key) — sem
// check de tenant de operador. Usado pelo toca-crm para refletir edição/
// confirmação da visita do lead na agenda do corretor.
func (h *AppointmentHandler) InternalUpdate(c *gin.Context) {
	id := c.Param("id")
	var req UpdateAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, ok := internalTenantRequested(c, req.TenantID)
	if !ok {
		return
	}
	c.Request = c.Request.WithContext(context.WithTenantID(c.Request.Context(), tenantID))
	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, services.ErrNotFound) {
			response.NotFound(c, err.Error())
		} else {
			response.InternalError(c, err)
		}
		return
	}
	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}
	if req.ExpectedUpdatedAt != nil && !req.ExpectedUpdatedAt.Equal(existing.UpdatedAt) {
		response.Error(c, http.StatusConflict, appointment.ErrConcurrentModification.Error())
		return
	}
	copy := *existing
	existing = &copy
	if existing.CustomerAccountID == "" {
		existing.CustomerAccountID = req.CustomerAccountID
	}
	if req.Title != "" {
		existing.Title = req.Title
	}
	if req.Date != "" {
		existing.Date = req.Date
	}
	if req.Time != "" {
		existing.Time = req.Time
	}
	if req.EndTime != "" {
		existing.EndTime = req.EndTime
	}
	if req.PropertyID != nil {
		if existing.PropertyID != *req.PropertyID {
			existing.Property = ""
		}
		existing.PropertyID = *req.PropertyID
	}
	if req.Property != nil {
		existing.Property = *req.Property
	}
	if req.Description != nil {
		existing.Description = *req.Description
	}
	if req.Status != "" {
		existing.Status = appointment.Status(req.Status)
	}
	if req.Type != "" {
		existing.Type = appointment.Type(req.Type)
	}
	if req.Priority != "" {
		existing.Priority = appointment.Priority(req.Priority)
	}
	if err := h.service.Update(c.Request.Context(), existing); err != nil {
		if errors.Is(err, services.ErrInspectionManaged) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		if errors.Is(err, appointment.ErrInvalid) {
			response.ValidationError(c, err.Error())
			return
		}
		if errors.Is(err, services.ErrConflict) || errors.Is(err, appointment.ErrConcurrentModification) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Appointment updated", existing)
}

// InternalDelete remove um appointment via rota interna (admin key).
func (h *AppointmentHandler) InternalDelete(c *gin.Context) {
	id := c.Param("id")
	tenantID, ok := internalTenant(c)
	if !ok {
		return
	}
	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	if existing.TenantID != tenantID {
		response.Forbidden(c, "access denied")
		return
	}
	if err := h.service.Delete(c.Request.Context(), id, tenantID); err != nil {
		if errors.Is(err, services.ErrInspectionManaged) {
			response.Error(c, http.StatusConflict, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Appointment deleted", nil)
}

// CalendarFile godoc
// @Summary      Baixar o compromisso em formato .ics
// @Description  Devolve o arquivo iCalendar de um compromisso da própria imobiliária. 409 quando o compromisso não gera arquivo (pendente, cancelado, concluído ou passado).
// @Tags         appointments
// @Produce      text/calendar
// @Security     BearerAuth
// @Param        id path string true "Appointment ID"
// @Success      200 {file} binary
// @Failure      403 {object} response.Response
// @Failure      404 {object} response.Response
// @Failure      409 {object} response.Response
// @Router       /appointments/{id}/calendar.ics [get]
func (h *AppointmentHandler) CalendarFile(c *gin.Context) {
	tenantID, err := context.GetTenantID(c.Request.Context())
	if err != nil {
		response.Forbidden(c, "tenant context required")
		return
	}
	file, name, err := h.service.CalendarFile(c.Request.Context(), tenantID, c.Param("id"))
	if err != nil {
		calendarError(c, err)
		return
	}
	writeCalendarFile(c, name, file)
}

// PublicCalendarFile atende o link assinado do e-mail: sem sessão, o token
// carrega o tenant e a validade, e o compromisso é revalidado na leitura.
func (h *AppointmentHandler) PublicCalendarFile(c *gin.Context) {
	file, name, err := h.service.PublicCalendarFile(c.Request.Context(), c.Param("id"), c.Query("token"))
	if err != nil {
		calendarError(c, err)
		return
	}
	writeCalendarFile(c, name, file)
}

func writeCalendarFile(c *gin.Context, name string, file []byte) {
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, "text/calendar; charset=utf-8", file)
}

// calendarError não distingue link expirado de adulterado: nos dois casos o
// download simplesmente não existe mais.
func calendarError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrNotFound), errors.Is(err, services.ErrCalendarLinkInvalid):
		response.NotFound(c, "appointment not found")
	case errors.Is(err, services.ErrCalendarNotAvailable):
		c.JSON(http.StatusConflict, gin.H{"success": false, "message": "Este compromisso não gera arquivo de calendário. Só horários marcados, futuros e com término definido podem ser adicionados."})
	default:
		response.InternalError(c, err)
	}
}
