package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/icalendar"
	"github.com/oginoino/toca/pkg/timeutil"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
)

// ErrCalendarNotAvailable marca compromisso que não vira arquivo de calendário:
// pedido ainda pendente, cancelado, concluído, já passado ou sem hora final.
var ErrCalendarNotAvailable = errors.New("compromisso não gera arquivo de calendário")

// ErrCalendarLinkInvalid cobre assinatura adulterada e link expirado: quem
// recebeu o e-mail não precisa saber qual dos dois aconteceu.
var ErrCalendarLinkInvalid = errors.New("link de calendário inválido ou expirado")

// CalendarFile devolve o .ics de um compromisso da própria imobiliária.
func (s *AppointmentService) CalendarFile(ctx context.Context, tenantID, appointmentID string) ([]byte, string, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(appointmentID) == "" {
		return nil, "", ErrNotFound
	}
	appt, err := s.repo.GetByID(ctx, appointmentID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}
	// O escopo do tenant é revalidado aqui: um id de outra imobiliária não vira
	// arquivo nem revela que o compromisso existe.
	if appt == nil || appt.TenantID != tenantID {
		return nil, "", ErrNotFound
	}
	return s.renderCalendar(ctx, appt)
}

// PublicCalendarFile atende o link assinado do e-mail: sem sessão, o tenant vem
// do próprio token e o compromisso é revalidado (inclusive status e data).
func (s *AppointmentService) PublicCalendarFile(ctx context.Context, appointmentID, token string) ([]byte, string, error) {
	if s.links == nil || !s.links.Enabled() {
		return nil, "", ErrNotFound
	}
	tenantID, ok := s.links.Verify(token, time.Now())
	if !ok {
		return nil, "", ErrCalendarLinkInvalid
	}
	// Sem sessão não há claims no contexto: a leitura do link usa o pool de
	// serviço e o tenant do próprio token é comparado antes de responder.
	appt, err := s.repo.GetByIDForLink(ctx, appointmentID)
	if err != nil {
		return nil, "", ErrNotFound
	}
	if appt == nil || appt.TenantID != tenantID {
		return nil, "", ErrNotFound
	}
	return s.renderCalendar(ctx, appt)
}

// CalendarURL monta o link assinado que acompanha os eventos de agenda. Vazio
// quando o compromisso não é elegível ou o assinador não está configurado.
func (s *AppointmentService) CalendarURL(ctx context.Context, appt *appointment.Appointment) string {
	if s.links == nil || !s.links.Enabled() || appt == nil {
		return ""
	}
	projection, ok := s.calendarCopy(ctx, appt)
	if !ok {
		return ""
	}
	return s.links.URL(appt.ID, appt.TenantID, projection.End.Add(CalendarLinkTTL))
}

func (s *AppointmentService) renderCalendar(ctx context.Context, appt *appointment.Appointment) ([]byte, string, error) {
	projection, ok := s.calendarCopy(ctx, appt)
	if !ok {
		return nil, "", ErrCalendarNotAvailable
	}
	file, err := icalendar.Render(projection)
	if err != nil {
		return nil, "", fmt.Errorf("gerar arquivo de calendário: %w", err)
	}
	return file, calendarFileName(appt), nil
}

// calendarCopy projeta o compromisso no fuso da imobiliária e diz se ele é
// elegível. É o único lugar que decide isso na agenda; o portal usa a mesma
// regra sobre o que o cliente já enxerga.
func (s *AppointmentService) calendarCopy(ctx context.Context, appt *appointment.Appointment) (appointment.CalendarCopy, bool) {
	loc := timeutil.LocationForTenant(ctx, appt.TenantID)
	return appt.CalendarCopyFor(time.Now().In(loc), loc)
}

// calendarFileName delega ao pacote compartilhado: um nome só para as três
// superfícies que oferecem o arquivo.
func calendarFileName(appt *appointment.Appointment) string {
	return icalendar.FileName(appt.Title, appt.Date)
}
