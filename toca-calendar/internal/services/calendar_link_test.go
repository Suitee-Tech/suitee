package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"github.com/stretchr/testify/require"
)

func TestCalendarLinkRoundTrip(t *testing.T) {
	link := NewCalendarLink("segredo-de-teste", "https://api.example.com/")
	require.True(t, link.Enabled())

	expires := time.Now().Add(CalendarLinkTTL)
	url := link.URL("appt-1", "tenant-a", expires)
	require.Contains(t, url, "/api/v1/appointments/public/appt-1/calendar.ics?token=")
	// A URL usa a base pública sem barra duplicada.
	require.Contains(t, url, "https://api.example.com/api/v1/")

	// Um token qualquer não passa.
	if _, ok := link.Verify("nao-e-um-token", time.Now()); ok {
		t.Error("token inválido foi aceito")
	}
	tenantID, ok := link.Verify(tokenOf(t, url), time.Now())
	require.True(t, ok)
	require.Equal(t, "tenant-a", tenantID)
}

func TestCalendarLinkRejectsTamperingAndExpiry(t *testing.T) {
	link := NewCalendarLink("segredo-de-teste", "https://api.example")
	url := link.URL("appt-1", "tenant-a", time.Now().Add(time.Hour))
	token := tokenOf(t, url)

	// Assinatura de outro segredo não passa.
	other := NewCalendarLink("outro-segredo", "https://api.example")
	if _, ok := other.Verify(token, time.Now()); ok {
		t.Error("token de outro segredo foi aceito")
	}
	// Adulterar o tenant invalida a assinatura.
	tampered := "dGVuYW50LWI=" + token[len("dGVuYW50LWE="):]
	if _, ok := link.Verify(tampered, time.Now()); ok {
		t.Error("token adulterado foi aceito")
	}
	// Vencido, não vale mais.
	if _, ok := link.Verify(token, time.Now().Add(2*time.Hour)); ok {
		t.Error("token vencido foi aceito")
	}
	// Sem segredo ou sem base o assinador fica inerte, e o e-mail sai sem CTA.
	require.False(t, NewCalendarLink("", "https://api.example").Enabled())
	require.False(t, NewCalendarLink("segredo", "").Enabled())
	require.Empty(t, NewCalendarLink("", "https://api.example").URL("appt-1", "tenant-a", time.Now()))
}

type calendarRepoStub struct {
	appointment.Repository
	appt *appointment.Appointment
}

func (r calendarRepoStub) GetByID(context.Context, string) (*appointment.Appointment, error) {
	return r.appt, nil
}

// O caminho do link assinado lê sem claims no contexto.
func (r calendarRepoStub) GetByIDForLink(context.Context, string) (*appointment.Appointment, error) {
	return r.appt, nil
}

func TestCalendarFileScopesTheTenant(t *testing.T) {
	svc := NewAppointmentService(calendarRepoStub{appt: &appointment.Appointment{
		ID: "appt-1", TenantID: "tenant-a", Title: "Visita", Date: "2099-10-10",
		Time: "14:00", EndTime: "15:00", Type: appointment.TypeVisita, Status: appointment.StatusConfirmado,
	}}, nil)

	file, name, err := svc.CalendarFile(context.Background(), "tenant-a", "appt-1")
	require.NoError(t, err)
	require.Contains(t, string(file), "BEGIN:VCALENDAR")
	require.Equal(t, "visita-2099-10-10.ics", name)

	// Outra imobiliária não recebe o arquivo nem descobre que ele existe.
	_, _, err = svc.CalendarFile(context.Background(), "tenant-b", "appt-1")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCalendarFileRefusesPendingAndFinished(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status appointment.Status
	}{
		{"pendente", appointment.StatusPendente},
		{"cancelado", appointment.StatusCancelado},
		{"concluido", appointment.StatusConcluido},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewAppointmentService(calendarRepoStub{appt: &appointment.Appointment{
				ID: "appt-1", TenantID: "tenant-a", Date: "2099-10-10", Time: "14:00", EndTime: "15:00",
				Status: tc.status,
			}}, nil)
			_, _, err := svc.CalendarFile(context.Background(), "tenant-a", "appt-1")
			require.ErrorIs(t, err, ErrCalendarNotAvailable)
		})
	}
}

func TestPublicCalendarFileUsesTheTokenTenant(t *testing.T) {
	appt := &appointment.Appointment{
		ID: "appt-1", TenantID: "tenant-a", Title: "Visita", Date: "2099-10-10",
		Time: "14:00", EndTime: "15:00", Type: appointment.TypeVisita, Status: appointment.StatusAgendado,
	}
	link := NewCalendarLink("segredo-de-teste", "https://api.example")
	svc := NewAppointmentService(calendarRepoStub{appt: appt}, nil).WithCalendarLinks(link)

	url := link.URL("appt-1", "tenant-a", time.Now().Add(time.Hour))
	file, _, err := svc.PublicCalendarFile(context.Background(), "appt-1", tokenOf(t, url))
	require.NoError(t, err)
	require.Contains(t, string(file), "UID:appt-1@suitee")

	// Token de outra imobiliária continua restrito ao compromisso dela.
	foreign := NewCalendarLink("segredo-de-teste", "https://api.example").URL("appt-1", "tenant-b", time.Now().Add(time.Hour))
	_, _, err = svc.PublicCalendarFile(context.Background(), "appt-1", tokenOf(t, foreign))
	require.ErrorIs(t, err, ErrNotFound)

	// Sem assinador configurado a rota pública não existe para o cliente.
	offline := NewAppointmentService(calendarRepoStub{appt: appt}, nil)
	_, _, err = offline.PublicCalendarFile(context.Background(), "appt-1", tokenOf(t, url))
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCalendarURLOnlyForPublishableAppointments(t *testing.T) {
	link := NewCalendarLink("segredo-de-teste", "https://api.example")
	svc := NewAppointmentService(calendarRepoStub{}, nil).WithCalendarLinks(link)

	eligible := &appointment.Appointment{ID: "appt-1", TenantID: "tenant-a", Date: "2099-10-10", Time: "14:00",
		EndTime: "15:00", Status: appointment.StatusConfirmado, Type: appointment.TypeVisita}
	require.Contains(t, svc.CalendarURL(context.Background(), eligible), "/calendar.ics?token=")

	pending := &appointment.Appointment{ID: "appt-2", TenantID: "tenant-a", Date: "2099-10-10", Time: "14:00",
		EndTime: "15:00", Status: appointment.StatusPendente}
	require.Empty(t, svc.CalendarURL(context.Background(), pending), "pedido pendente não gera link")
}

// tokenOf extrai o valor de ?token= da URL assinada.
func tokenOf(t *testing.T, raw string) string {
	t.Helper()
	_, token, found := strings.Cut(raw, "token=")
	require.True(t, found, "url sem token: %s", raw)
	return token
}
