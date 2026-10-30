package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

func respondPortalVisitAction(c *gin.Context, a *clients.Appointment, err error) {
	if err != nil {
		switch {
		case err.Error() == "deal not found or access denied", errors.Is(err, services.ErrPortalAppointmentAccess):
			response.Forbidden(c, "Acesso à visita negado.")
		case errors.Is(err, clients.ErrCalendarNotFound):
			response.NotFound(c, "Visita não encontrada.")
		case errors.Is(err, clients.ErrCalendarConflict), errors.Is(err, clients.ErrCalendarChanged), errors.Is(err, services.ErrPortalAppointmentClosed):
			response.Error(c, http.StatusConflict, err.Error())
		case errors.Is(err, clients.ErrCalendarInvalid):
			response.ValidationError(c, err.Error())
		default:
			response.Error(c, http.StatusServiceUnavailable, clients.ErrCalendarUnavailable.Error())
		}
		return
	}
	response.Success(c, "Visita atualizada", &portal.Appointment{
		ID: a.ID, Title: a.Title, Date: a.Date, Time: a.Time, EndTime: a.EndTime,
		Type: a.Type, Status: a.Status, Description: a.Description, Property: a.Property,
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt, CanManage: a.CanManage,
	})
}

func (h *DealHandler) UpdateMyDealAppointment(c *gin.Context) {
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())
	email, _ := pkgcontext.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req portal.UpdateAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	a, err := h.service.UpdateMyDealAppointment(c.Request.Context(), accountID, email, c.Param("id"), c.Param("appointmentId"), req)
	respondPortalVisitAction(c, a, err)
}

func (h *DealHandler) CancelMyDealAppointment(c *gin.Context) {
	accountID, _ := pkgcontext.GetAccountID(c.Request.Context())
	email, _ := pkgcontext.GetEmail(c.Request.Context())
	if accountID == "" && email == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	var req portal.CancelAppointmentRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	a, err := h.service.CancelMyDealAppointment(c.Request.Context(), accountID, email, c.Param("id"), c.Param("appointmentId"), req)
	respondPortalVisitAction(c, a, err)
}
