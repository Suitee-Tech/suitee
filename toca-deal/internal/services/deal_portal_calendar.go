package services

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

// ErrPortalCalendarNotAvailable marca compromisso que não gera arquivo para o
// cliente: pedido ainda pendente, cancelado, concluído, já passado ou sem hora
// final — a mesma regra da agenda, aplicada ao que o portal mostra.
var ErrPortalCalendarNotAvailable = errors.New("compromisso não gera arquivo de calendário")

// portalPublishableStatus lista os status que representam horário marcado.
func portalPublishableStatus(status string) bool {
	switch status {
	case "Agendado", "Confirmado", "Reagendado":
		return true
	default:
		return false
	}
}

// MyDealAppointmentCalendar devolve o .ics de um compromisso do próprio cliente.
// A autorização é a do restante do portal: o negócio precisa ser dele e o
// compromisso precisa estar no escopo dos leads dele.
func (s *DealService) MyDealAppointmentCalendar(ctx context.Context, accountID, email, dealID, appointmentID string) ([]byte, string, error) {
	deal, err := s.findMyDeal(ctx, accountID, email, dealID)
	if err != nil {
		return nil, "", err
	}
	if s.calendarClient == nil {
		return nil, "", clients.ErrCalendarUnavailable
	}
	appointments, err := s.calendarClient.ListAppointmentsByDeal(ctx, deal.ID, clients.DealAppointmentScope{
		TenantID: deal.TenantID,
		LeadIDs:  portalAppointmentLeadIDs(deal, accountID, email),
	})
	if err != nil {
		return nil, "", err
	}
	for _, appt := range appointments {
		if appt == nil || appt.ID != appointmentID {
			continue
		}
		projection, ok := portalCalendarProjection(appt, timeutil.LocationForTenant(ctx, deal.TenantID), time.Now())
		if !ok {
			return nil, "", ErrPortalCalendarNotAvailable
		}
		file, err := icalendar.Render(projection)
		if err != nil {
			return nil, "", fmt.Errorf("gerar arquivo de calendário: %w", err)
		}
		return file, portalCalendarFileName(appt), nil
	}
	// Compromisso de outro negócio (ou de outro cliente) não existe para quem
	// está pedindo: a resposta é a mesma de um id inexistente.
	return nil, "", ErrPortalCalendarNotAvailable
}

// portalCalendarProjection publica o que o cliente já vê no portal: título,
// janela de horário e o imóvel que aparece no próprio cartão. Descrição, notas
// e participantes ficam de fora.
func portalCalendarProjection(appt *clients.Appointment, loc *time.Location, now time.Time) (icalendar.Copy, bool) {
	if appt == nil || loc == nil || !portalPublishableStatus(appt.Status) {
		return icalendar.Copy{}, false
	}
	start, err := portalCivilTime(appt.Date, appt.Time, loc)
	if err != nil {
		return icalendar.Copy{}, false
	}
	end, err := portalCivilTime(appt.Date, appt.EndTime, loc)
	if err != nil || !end.After(start) || start.Before(now) {
		return icalendar.Copy{}, false
	}
	title := strings.TrimSpace(appt.Title)
	if title == "" {
		title = strings.TrimSpace(appt.Type)
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

// portalCivilTime interpreta data e hora civis no fuso da imobiliária.
func portalCivilTime(date, clock string, loc *time.Location) (time.Time, error) {
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

// portalCalendarFileName delega ao pacote compartilhado: um nome só para as três superfícies.
func portalCalendarFileName(appt *clients.Appointment) string {
	return icalendar.FileName(appt.Title, appt.Date)
}
