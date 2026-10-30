package services

import (
	"context"
	"errors"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
	"testing"
	"time"
)

func TestInspectionCalendarLifecycle(t *testing.T) {
	repo := newFakeApptRepo()
	handler := NewInspectionCalendarHandler(repo)
	stamp := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	p := contracts.InspectionCalendarPayload{InspectionID: "inspection", TenantID: "company", ContractID: "contract", PropertyID: "property", Type: "ENTRY", ScheduledDate: stamp, TimeZone: "America/Sao_Paulo", InspectorID: "supplier", InspectorName: "Ana Vistoriadora", ScheduledBy: "organizer", Status: "Agendada", UpdatedAt: stamp}
	deliver := func(p contracts.InspectionCalendarPayload) {
		t.Helper()
		if err := handler(context.Background(), contracts.InspectionCalendarEvent{ID: "event", Data: p}); err != nil {
			t.Fatal(err)
		}
	}
	deliver(p)
	deliver(p)
	if len(repo.items) != 1 {
		t.Fatalf("duplicated calendar entry: %d", len(repo.items))
	}
	a, _ := inspectionAppointment(p)
	current := repo.items[a.ID]
	if current.Time != "09:00" || current.EndTime != "10:00" || current.InspectorID != "supplier" || current.AgentID != "organizer" || current.InspectionContractID != "contract" {
		t.Fatalf("invalid projection: %+v", current)
	}
	newer := p
	newer.UpdatedAt = stamp.Add(time.Minute)
	newer.ScheduledDate = stamp.Add(2 * time.Hour)
	newer.InspectorName = "Bia"
	deliver(newer)
	deliver(p)
	if repo.items[a.ID].Time != "11:00" || repo.items[a.ID].InspectorName != "Bia" {
		t.Fatal("old delivery overwrote new schedule")
	}
	newer.Status = "Cancelada"
	newer.UpdatedAt = newer.UpdatedAt.Add(time.Minute)
	deliver(newer)
	deliver(p)
	if repo.items[a.ID].Status != appointment.StatusCancelado {
		t.Fatal("cancellation resurrected")
	}
	svc := NewAppointmentService(repo, nil)
	if err := svc.Update(context.Background(), repo.items[a.ID]); !errors.Is(err, ErrInspectionManaged) {
		t.Fatalf("manual edit allowed: %v", err)
	}
	if err := svc.Delete(context.Background(), a.ID, p.TenantID); !errors.Is(err, ErrInspectionManaged) {
		t.Fatalf("manual delete allowed: %v", err)
	}
	other := p
	other.TenantID = "other"
	deliver(other)
	if len(repo.items) != 2 {
		t.Fatal("companies shared a calendar record")
	}
	newer.Status = "Concluída"
	newer.UpdatedAt = newer.UpdatedAt.Add(time.Minute)
	deliver(newer)
	if repo.items[a.ID].Status != appointment.StatusConcluido {
		t.Fatal("completion not reflected")
	}
}

func TestInspectionCalendarRejectsIncompleteSnapshot(t *testing.T) {
	handler := NewInspectionCalendarHandler(newFakeApptRepo())
	if handler(context.Background(), contracts.InspectionCalendarEvent{}) == nil {
		t.Fatal("accepted incomplete event")
	}
}
