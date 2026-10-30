package service

import (
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
)

func TestVisitCalendarProjectionOnlyPublishesMarkedVisits(t *testing.T) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("fuso indisponível: %v", err)
	}
	now := time.Date(2026, 10, 3, 9, 0, 0, 0, loc)
	base := func(status string) *clients.Appointment {
		return &clients.Appointment{ID: "appt-1", Title: "Visita ao imóvel", Property: "Apto Jardins",
			Date: "2026-10-10", Time: "14:00", EndTime: "15:00", Status: status}
	}
	for _, status := range []string{"Agendado", "Confirmado", "Reagendado"} {
		projection, ok := visitCalendarProjection(base(status), loc, now)
		if !ok {
			t.Errorf("status %s deveria ser publicável", status)
			continue
		}
		if projection.UID != "appt-1@suitee" || projection.Location != "Apto Jardins" {
			t.Errorf("projeção errada: %+v", projection)
		}
		if projection.Start.Hour() != 14 || projection.End.Hour() != 15 {
			t.Errorf("janela errada: %s → %s", projection.Start, projection.End)
		}
	}
	// Pendente ainda depende do corretor; cancelado e concluído são passado.
	for _, status := range []string{"Pendente", "Cancelado", "Concluido"} {
		if _, ok := visitCalendarProjection(base(status), loc, now); ok {
			t.Errorf("status %s não pode virar evento", status)
		}
	}
	// Sem hora final não há janela; visita passada não volta como evento novo.
	noEnd := base("Confirmado")
	noEnd.EndTime = ""
	if _, ok := visitCalendarProjection(noEnd, loc, now); ok {
		t.Error("visita sem hora final não é publicável")
	}
	past := base("Confirmado")
	past.Date = "2026-10-01"
	if _, ok := visitCalendarProjection(past, loc, now); ok {
		t.Error("visita passada não é publicável")
	}
	// O nome do arquivo precisa ser seguro para o cliente salvar.
	if name := visitCalendarFileName(base("Confirmado")); name != "visita-ao-imovel-2026-10-10.ics" {
		t.Errorf("nome do arquivo: %q", name)
	}
}
