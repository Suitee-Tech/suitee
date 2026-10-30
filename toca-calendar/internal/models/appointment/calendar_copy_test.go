package appointment

import (
	"testing"
	"time"
)

func saoPaulo(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("fuso indisponível: %v", err)
	}
	return loc
}

func TestCalendarCopyEligibility(t *testing.T) {
	loc := saoPaulo(t)
	now := time.Date(2026, 10, 3, 9, 0, 0, 0, loc)
	base := func() *Appointment {
		return &Appointment{ID: "appt-1", TenantID: "tenant", Title: "Visita ao imóvel",
			Date: "2026-10-10", Time: "14:00", EndTime: "15:00", Type: TypeVisita,
			Property: "Apto Jardins", Status: StatusAgendado}
	}

	appt := base()
	appt.Status = StatusPendente
	if _, ok := appt.CalendarCopyFor(now, loc); ok {
		t.Error("pedido pendente não pode virar evento de calendário")
	}
	for _, status := range []Status{StatusAgendado, StatusConfirmado, StatusReagendado} {
		appt := base()
		appt.Status = status
		copy, ok := appt.CalendarCopyFor(now, loc)
		if !ok {
			t.Errorf("status %s deveria ser publicável", status)
			continue
		}
		if copy.UID != "appt-1@suitee" {
			t.Errorf("UID instável: %s", copy.UID)
		}
		if copy.Start.Hour() != 14 || copy.End.Hour() != 15 {
			t.Errorf("janela errada: %s → %s", copy.Start, copy.End)
		}
	}
	for _, status := range []Status{StatusCancelado, StatusConcluido, StatusEmAndamento} {
		appt := base()
		appt.Status = status
		if _, ok := appt.CalendarCopyFor(now, loc); ok {
			t.Errorf("status %s não pode ser publicável", status)
		}
	}

	// Sem hora final não há janela: um evento de calendário precisa de término.
	noEnd := base()
	noEnd.EndTime = ""
	if _, ok := noEnd.CalendarCopyFor(now, loc); ok {
		t.Error("compromisso sem hora final não é publicável")
	}
	// Horário já passado não vira evento novo.
	past := base()
	past.Date = "2026-10-01"
	if _, ok := past.CalendarCopyFor(now, loc); ok {
		t.Error("compromisso passado não é publicável")
	}
	// O nome do imóvel é a única localização publicada.
	if copy, _ := base().CalendarCopyFor(now, loc); copy.Location != "Apto Jardins" {
		t.Errorf("local esperado do imóvel, veio %q", copy.Location)
	}
}
