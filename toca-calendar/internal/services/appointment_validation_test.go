package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
)

type validationEventSpy struct {
	events.EventBus
	published int
}

func (s *validationEventSpy) PublishReliable(context.Context, events.Event) error {
	s.published++
	return nil
}

func TestAppointmentValidationRejectsInvalidWritesBeforePersistenceOrEvents(t *testing.T) {
	cases := map[string]func(*appointment.Appointment){
		"missing date":      func(a *appointment.Appointment) { a.Date = "" },
		"impossible date":   func(a *appointment.Appointment) { a.Date = "2026-02-30" },
		"noncanonical date": func(a *appointment.Appointment) { a.Date = "2026-2-03" },
		"invalid start":     func(a *appointment.Appointment) { a.Time = "25:00" },
		"short start":       func(a *appointment.Appointment) { a.Time = "9:00" },
		"invalid end":       func(a *appointment.Appointment) { a.EndTime = "24:00" },
		"equal times":       func(a *appointment.Appointment) { a.EndTime = a.Time },
		"reversed times":    func(a *appointment.Appointment) { a.EndTime = "08:00" },
		"unknown type":      func(a *appointment.Appointment) { a.Type = "typo" },
		"unknown status":    func(a *appointment.Appointment) { a.Status = "typo" },
		"unknown priority":  func(a *appointment.Appointment) { a.Priority = "typo" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			for _, command := range []string{"create", "idempotent create", "update"} {
				t.Run(command, func(t *testing.T) {
					repo := newFakeApptRepo()
					bus := &validationEventSpy{}
					service := NewAppointmentService(repo, bus)
					original := newAppointment("appointment", "agency", "agent", "2026-05-20", "09:00", "10:00")
					if command == "update" {
						repo.items[original.ID] = original
					}
					candidate := *original
					change(&candidate)
					var err error
					switch command {
					case "create":
						err = service.Create(context.Background(), &candidate)
					case "idempotent create":
						err = service.CreateIdempotent(context.Background(), &candidate)
					case "update":
						err = service.Update(context.Background(), &candidate)
					}
					if !errors.Is(err, appointment.ErrInvalid) {
						t.Fatalf("expected validation error, got %v", err)
					}
					if bus.published != 0 {
						t.Fatal("invalid data emitted events")
					}
					if command == "update" {
						if *repo.items[original.ID] != *original {
							t.Fatal("invalid update changed stored record")
						}
					} else if len(repo.items) != 0 {
						t.Fatal("invalid appointment persisted")
					}
				})
			}
		})
	}
}

func TestAppointmentValidationPreservesHistoricalAndLegacyValues(t *testing.T) {
	repo := newFakeApptRepo()
	service := NewAppointmentService(repo, nil)
	a := newAppointment("appointment", "agency", "", "2000-02-29", "09:00", "10:00")
	a.Type, a.Status = appointment.TypeEntrega, appointment.StatusEmAndamento
	if err := service.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	repo.items[a.ID].Type, repo.items[a.ID].Status = "LegacyType", "LegacyStatus"
	edited := *repo.items[a.ID]
	edited.Title = "Título atualizado"
	if err := service.Update(context.Background(), &edited); err != nil {
		t.Fatal(err)
	}
	if repo.items[a.ID].Type != "LegacyType" || repo.items[a.ID].Status != "LegacyStatus" {
		t.Fatal("editing title changed legacy enums")
	}
	for _, typ := range []appointment.Type{"Ligacao", "Lembrete", "Outro", "Reunião"} {
		a.Type = typ
		a.Status = appointment.StatusAgendado
		if err := a.Validate(nil); err != nil {
			t.Fatalf("frontend type %s rejected: %v", typ, err)
		}
	}
}

func TestIdempotentCalendarCreateRejectsChangedPayload(t *testing.T) {
	repo := newFakeApptRepo()
	svc := NewAppointmentService(repo, nil)
	original := newAppointment("stable-id", "agency", "agent", "2099-10-20", "14:00", "15:00")
	original.Description = "Original"
	if err := svc.CreateIdempotent(context.Background(), original); err != nil {
		t.Fatal(err)
	}
	changed := *original
	changed.EndTime = "16:00"
	changed.Description = "Changed"
	if err := svc.CreateIdempotent(context.Background(), &changed); !errors.Is(err, appointment.ErrConcurrentModification) {
		t.Fatalf("changed idempotency payload accepted: %v", err)
	}
	saved := repo.items[original.ID]
	if saved.EndTime != "15:00" || saved.Description != "Original" {
		t.Fatal("existing request changed")
	}
}
