package services

import (
	"context"
	"errors"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
)

var ErrPortalAppointmentAccess = errors.New("Esta visita não pertence ao cliente ou ao negócio.")
var ErrPortalAppointmentClosed = errors.New("Esta visita já foi encerrada ou iniciada e não pode ser alterada.")

func portalAppointmentOwned(d *deal.Deal, a *clients.Appointment, accountID, email string) bool {
	if a.TenantID != d.TenantID || (a.DealID != "" && a.DealID != d.ID) {
		return false
	}
	leadOwned := false
	for _, id := range portalAppointmentLeadIDs(d, accountID, email) {
		if id == a.LeadID {
			leadOwned = true
			break
		}
	}
	// A pre-deal appointment must belong to an authorized lead as well.
	if a.DealID == "" && !leadOwned {
		return false
	}
	if a.CustomerAccountID != "" {
		return accountID != "" && a.CustomerAccountID == accountID
	}
	return leadOwned
}

func portalAppointmentEditable(a *clients.Appointment) bool {
	return portalAppointmentEditableIn(timeutil.Location(), a)
}

// portalAppointmentEditableIn é a variante que respeita o fuso da imobiliária:
// "ainda dá tempo de editar" depende do dia civil DELA, não do fuso do processo.
func portalAppointmentEditableIn(loc *time.Location, a *clients.Appointment) bool {
	if a.Type != "Visita" || a.UpdatedAt.IsZero() {
		return false
	}
	switch a.Status {
	case "Pendente", "Agendado", "Confirmado", "Reagendado":
		start, err := time.ParseInLocation("2006-01-02 15:04", a.Date+" "+a.Time, loc)
		return err == nil && start.After(time.Now())
	}
	return false
}

// myEditableAppointment devolve o compromisso e o TENANT do negócio: o fuso
// configurado pela imobiliária é o que decide se ainda dá tempo de editar.
func (s *DealService) myEditableAppointment(ctx context.Context, accountID, email, dealID, appointmentID string, expected time.Time) (*clients.Appointment, string, error) {
	d, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, "", err
	}
	if s.calendarClient == nil {
		return nil, d.TenantID, clients.ErrCalendarUnavailable
	}
	a, err := s.calendarClient.GetAppointment(ctx, appointmentID, d.TenantID)
	if err != nil {
		return nil, d.TenantID, err
	}
	if !portalAppointmentOwned(d, a, accountID, email) {
		return nil, d.TenantID, ErrPortalAppointmentAccess
	}
	if expected.IsZero() {
		return nil, d.TenantID, clients.ErrCalendarInvalid
	}
	if !expected.Equal(a.UpdatedAt) {
		return nil, d.TenantID, clients.ErrCalendarChanged
	}
	if !portalAppointmentEditableIn(timeutil.LocationForTenant(ctx, d.TenantID), a) {
		return nil, d.TenantID, ErrPortalAppointmentClosed
	}
	return a, d.TenantID, nil
}

func (s *DealService) UpdateMyDealAppointment(ctx context.Context, accountID, email, dealID, appointmentID string, req portal.UpdateAppointmentRequest) (*clients.Appointment, error) {
	a, tenantID, err := s.myEditableAppointment(ctx, accountID, email, dealID, appointmentID, req.ExpectedUpdatedAt)
	if err != nil {
		return nil, err
	}
	// O dia civil do agendamento é o da imobiliária (dono do negócio).
	location := timeutil.LocationForTenant(ctx, tenantID)
	start, err := time.ParseInLocation("2006-01-02 15:04", req.Date+" "+req.Time, location)
	if err != nil || !start.After(time.Now()) {
		return nil, clients.ErrCalendarInvalid
	}
	end, err := time.ParseInLocation("2006-01-02 15:04", req.Date+" "+req.EndTime, location)
	if err != nil || !end.After(start) {
		return nil, clients.ErrCalendarInvalid
	}
	status := a.Status
	if req.Date != a.Date || req.Time != a.Time || req.EndTime != a.EndTime {
		status = "Pendente"
		if a.Status == "Confirmado" || a.Status == "Reagendado" {
			status = "Reagendado"
		}
	}
	updated, err := s.calendarClient.UpdateAppointment(ctx, a.ID, clients.UpdateAppointmentRequest{
		TenantID: a.TenantID, CustomerAccountID: accountID, ExpectedUpdatedAt: &req.ExpectedUpdatedAt,
		Date: req.Date, Time: req.Time, EndTime: req.EndTime, Description: req.Description, Status: status,
	})
	if err == nil {
		updated.CanManage = portalAppointmentEditableIn(timeutil.LocationForTenant(ctx, updated.TenantID), updated)
	}
	return updated, err
}

func (s *DealService) CancelMyDealAppointment(ctx context.Context, accountID, email, dealID, appointmentID string, req portal.CancelAppointmentRequest) (*clients.Appointment, error) {
	a, _, err := s.myEditableAppointment(ctx, accountID, email, dealID, appointmentID, req.ExpectedUpdatedAt)
	if err != nil {
		return nil, err
	}
	// Retain the appointment and its relationship to the lead/deal for tracking.
	return s.calendarClient.UpdateAppointment(ctx, a.ID, clients.UpdateAppointmentRequest{
		TenantID: a.TenantID, CustomerAccountID: accountID, ExpectedUpdatedAt: &req.ExpectedUpdatedAt, Status: "Cancelado",
	})
}
