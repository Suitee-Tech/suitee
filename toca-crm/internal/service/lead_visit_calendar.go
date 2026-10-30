package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/icalendar"
	"github.com/oginoino/toca/pkg/timeutil"
)

// ErrVisitNotMarked cobre a visita que ainda não virou horário marcado: pedido
// aguardando o corretor, visita cancelada, concluída ou já passada.
var ErrVisitNotMarked = errors.New("visita ainda não está marcada")

// ClientVisitCalendar devolve o .ics da visita marcada do próprio cliente.
// A autorização é a mesma do restante do portal: o lead precisa ser dele.
func (s *LeadService) ClientVisitCalendar(ctx context.Context, leadID string) ([]byte, string, error) {
	l, err := s.visitLead(ctx, leadID, true)
	if err != nil {
		return nil, "", err
	}
	if l.VisitRequest == nil || l.VisitRequest.AppointmentID == "" {
		return nil, "", ErrVisitNotMarked
	}
	if s.calendarClient == nil {
		return nil, "", clients.ErrCalendarUnavailable
	}
	appt, err := s.calendarClient.GetAppointment(ctx, l.VisitRequest.AppointmentID, l.TenantID)
	if err != nil {
		return nil, "", err
	}
	projection, ok := visitCalendarProjection(appt, timeutil.LocationForTenant(ctx, l.TenantID), time.Now())
	if !ok {
		return nil, "", ErrVisitNotMarked
	}
	file, err := icalendar.Render(projection)
	if err != nil {
		return nil, "", fmt.Errorf("gerar arquivo de calendário: %w", err)
	}
	return file, visitCalendarFileName(appt), nil
}

// visitCalendarProjection publica o que o cliente já vê no cartão da visita:
// título, janela de horário e o imóvel da visita. Notas e descrição ficam fora.
func visitCalendarProjection(appt *clients.Appointment, loc *time.Location, now time.Time) (icalendar.Copy, bool) {
	if appt == nil || loc == nil {
		return icalendar.Copy{}, false
	}
	switch appt.Status {
	case "Agendado", "Confirmado", "Reagendado":
	default:
		return icalendar.Copy{}, false
	}
	start, err := visitCivilTime(appt.Date, appt.Time, loc)
	if err != nil {
		return icalendar.Copy{}, false
	}
	end, err := visitCivilTime(appt.Date, appt.EndTime, loc)
	if err != nil || !end.After(start) || start.Before(now) {
		return icalendar.Copy{}, false
	}
	title := strings.TrimSpace(appt.Title)
	if title == "" {
		title = "Visita"
	}
	return icalendar.Copy{
		UID:      appt.ID + "@suitee",
		Title:    title,
		Location: strings.TrimSpace(appt.Property),
		Start:    start,
		End:      end,
		TimeZone: loc.String(),
		Stamp:    now.UTC(),
	}, true
}

// visitCivilTime interpreta data e hora civis no fuso da imobiliária.
func visitCivilTime(date, clock string, loc *time.Location) (time.Time, error) {
	day, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(date), loc)
	if err != nil {
		return time.Time{}, err
	}
	hour, minute := 0, 0
	if trimmed := strings.TrimSpace(clock); trimmed != "" {
		if _, err := fmt.Sscanf(strings.Replace(trimmed, ":", " ", 1), "%d %d", &hour, &minute); err != nil {
			return time.Time{}, err
		}
	}
	return time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc), nil
}

// visitCalendarFileName delega ao pacote compartilhado: um nome só para as três superfícies.
func visitCalendarFileName(appt *clients.Appointment) string {
	return icalendar.FileName(appt.Title, appt.Date)
}
