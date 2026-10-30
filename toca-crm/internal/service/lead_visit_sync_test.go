package service

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// syncTestEvent é um events.Event mínimo para os testes do handler.
type syncTestEvent struct {
	name    string
	payload interface{}
}

func (e *syncTestEvent) Name() string         { return e.name }
func (e *syncTestEvent) Payload() interface{} { return e.payload }
func (e *syncTestEvent) EventID() string      { return e.name }

func appointmentCreated(payload map[string]interface{}) events.Event {
	return &syncTestEvent{name: "appointment.created", payload: payload}
}

func seedLead(id, tenant string) *lead.Lead {
	return &lead.Lead{ID: id, TenantID: tenant, UpdatedAt: time.Now()}
}

func TestAppointmentVisitSyncAdoptsOperatorVisit(t *testing.T) {
	repo := newAcceptRepo(seedLead("lead-1", "tenant-a"))
	handler := NewAppointmentVisitSyncHandler(repo)

	err := handler(context.Background(), appointmentCreated(map[string]interface{}{
		"id": "appt-1", "tenant_id": "tenant-a", "lead_id": "lead-1",
		"type": "Visita", "date": "2026-10-05", "time": "14:00", "end_time": "15:00",
		"property_id": "", "description": "Visitar apartamento", "status": "Pendente",
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	l := repo.leads["lead-1"]
	if l.VisitRequest == nil {
		t.Fatal("visit_request não foi adotado")
	}
	if l.VisitRequest.AppointmentID != "appt-1" {
		t.Errorf("AppointmentID = %q, want appt-1", l.VisitRequest.AppointmentID)
	}
	if l.VisitRequest.Status != "Aguardando" {
		t.Errorf("Status = %q, want Aguardando (Pendente normalizado)", l.VisitRequest.Status)
	}
	if l.VisitRequest.Date != "2026-10-05" || l.VisitRequest.Time != "14:00" || l.VisitRequest.EndTime != "15:00" {
		t.Errorf("agenda não copiada: %+v", l.VisitRequest)
	}
}

func TestAppointmentVisitSyncIgnoresNonVisitTypes(t *testing.T) {
	repo := newAcceptRepo(seedLead("lead-1", "tenant-a"))
	handler := NewAppointmentVisitSyncHandler(repo)

	if err := handler(context.Background(), appointmentCreated(map[string]interface{}{
		"id": "appt-2", "tenant_id": "tenant-a", "lead_id": "lead-1", "type": "Ligacao", "status": "Pendente",
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if repo.leads["lead-1"].VisitRequest != nil {
		t.Fatal("tipo não-Visita não deveria virar visita")
	}
}

func TestAppointmentVisitSyncIgnoresInspectionAppointments(t *testing.T) {
	repo := newAcceptRepo(seedLead("lead-1", "tenant-a"))
	handler := NewAppointmentVisitSyncHandler(repo)

	if err := handler(context.Background(), appointmentCreated(map[string]interface{}{
		"id": "appt-3", "tenant_id": "tenant-a", "lead_id": "lead-1",
		"type": "Visita", "inspection_id": "insp-1", "status": "Pendente",
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if repo.leads["lead-1"].VisitRequest != nil {
		t.Fatal("vistoria de contrato não deveria virar visita de lead")
	}
}

func TestAppointmentVisitSyncDoesNotClobberOpenClientVisit(t *testing.T) {
	l := seedLead("lead-1", "tenant-a")
	l.VisitRequest = &lead.VisitRequest{AppointmentID: "client-appt", Status: "Aguardando", Date: "2026-10-01"}
	repo := newAcceptRepo(l)
	handler := NewAppointmentVisitSyncHandler(repo)

	if err := handler(context.Background(), appointmentCreated(map[string]interface{}{
		"id": "appt-9", "tenant_id": "tenant-a", "lead_id": "lead-1", "type": "Visita", "status": "Pendente",
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	if repo.leads["lead-1"].VisitRequest.AppointmentID != "client-appt" {
		t.Fatal("visita aberta do cliente foi sobrescrita")
	}
}

func TestAppointmentVisitSyncAdoptsAfterClosedVisit(t *testing.T) {
	l := seedLead("lead-1", "tenant-a")
	l.VisitRequest = &lead.VisitRequest{AppointmentID: "old-appt", Status: "Cancelado"}
	repo := newAcceptRepo(l)
	handler := NewAppointmentVisitSyncHandler(repo)

	if err := handler(context.Background(), appointmentCreated(map[string]interface{}{
		"id": "appt-10", "tenant_id": "tenant-a", "lead_id": "lead-1", "type": "Visita", "status": "Confirmado",
	})); err != nil {
		t.Fatalf("handler: %v", err)
	}
	got := repo.leads["lead-1"].VisitRequest
	if got == nil || got.AppointmentID != "appt-10" {
		t.Fatalf("visita encerrada não foi substituída: %+v", got)
	}
	if got.Status != "Confirmado" {
		t.Errorf("Status = %q, want Confirmado", got.Status)
	}
}

func TestAppointmentVisitSyncIsIdempotent(t *testing.T) {
	repo := newAcceptRepo(seedLead("lead-1", "tenant-a"))
	handler := NewAppointmentVisitSyncHandler(repo)
	evt := appointmentCreated(map[string]interface{}{
		"id": "appt-1", "tenant_id": "tenant-a", "lead_id": "lead-1",
		"type": "Visita", "date": "2026-10-05", "time": "14:00", "end_time": "15:00", "status": "Pendente",
	})

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("primeira entrega: %v", err)
	}
	seqAfterFirst := repo.leads["lead-1"].UpdatedSeq

	if err := handler(context.Background(), evt); err != nil {
		t.Fatalf("segunda entrega: %v", err)
	}
	if repo.leads["lead-1"].UpdatedSeq != seqAfterFirst {
		t.Fatal("redelivery regravou o lead (não idempotente)")
	}
}
