package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/oginoino/toca/toca-calendar/internal/models/appointment"
)

type fakeApptRepo struct {
	mu            sync.Mutex
	items         map[string]*appointment.Appointment
	conflictMatch func(tenantID, agentID, date, startTime, endTime, excludeID string) []*appointment.Appointment
}

func newFakeApptRepo() *fakeApptRepo {
	return &fakeApptRepo{items: map[string]*appointment.Appointment{}}
}

func (f *fakeApptRepo) Create(ctx context.Context, a *appointment.Appointment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a.ID == "" {
		return errors.New("missing ID")
	}
	f.items[a.ID] = a
	return nil
}

// O link assinado lê sem claims no contexto; o fake responde igual.
func (f *fakeApptRepo) GetByIDForLink(ctx context.Context, id string) (*appointment.Appointment, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeApptRepo) GetByID(ctx context.Context, id string) (*appointment.Appointment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.items[id], nil
}

func (f *fakeApptRepo) Update(ctx context.Context, a *appointment.Appointment) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items[a.ID] = a
	return nil
}

func (f *fakeApptRepo) Delete(ctx context.Context, tenantID, id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.items, id)
	return nil
}

func (f *fakeApptRepo) List(ctx context.Context, tenantID string, filters appointment.ListFilters) ([]*appointment.Appointment, int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*appointment.Appointment{}
	for _, a := range f.items {
		if a.TenantID == tenantID {
			out = append(out, a)
		}
	}
	return out, int64(len(out)), nil
}

func (f *fakeApptRepo) FindConflicts(ctx context.Context, tenantID, agentID, date, startTime, endTime, excludeID string) ([]*appointment.Appointment, error) {
	if f.conflictMatch != nil {
		return f.conflictMatch(tenantID, agentID, date, startTime, endTime, excludeID), nil
	}
	return nil, nil
}

func newAppointment(id, tenantID, agentID, date, time, endTime string) *appointment.Appointment {
	return &appointment.Appointment{
		ID:       id,
		TenantID: tenantID,
		AgentID:  agentID,
		Date:     date,
		Time:     time,
		EndTime:  endTime,
		Status:   appointment.StatusAgendado,
		Type:     appointment.TypeVisita,
		Priority: appointment.PriorityMedia,
	}
}

func TestAppointmentService_Create_Succeeds(t *testing.T) {
	repo := newFakeApptRepo()
	svc := NewAppointmentService(repo, nil)

	appt := newAppointment("a1", "t1", "agent-1", "2026-05-20", "14:00", "15:00")
	if err := svc.Create(context.Background(), appt); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if repo.items["a1"] == nil {
		t.Error("appointment was not persisted")
	}
}

func TestAppointmentService_Create_RejectsConflict(t *testing.T) {
	repo := newFakeApptRepo()
	repo.conflictMatch = func(_, _, _, _, _, _ string) []*appointment.Appointment {
		return []*appointment.Appointment{{ID: "existing"}}
	}
	svc := NewAppointmentService(repo, nil)

	appt := newAppointment("a1", "t1", "agent-1", "2026-05-20", "14:00", "15:00")
	err := svc.Create(context.Background(), appt)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict, got %v", err)
	}
	if repo.items["a1"] != nil {
		t.Error("appointment should NOT be persisted on conflict")
	}
}

func TestAppointmentService_Create_SkipsConflictCheckWhenIncomplete(t *testing.T) {
	repo := newFakeApptRepo()
	called := false
	repo.conflictMatch = func(_, _, _, _, _, _ string) []*appointment.Appointment {
		called = true
		return []*appointment.Appointment{{ID: "x"}}
	}
	svc := NewAppointmentService(repo, nil)

	appt := &appointment.Appointment{
		ID:       "a1",
		TenantID: "t1",
		Date:     "2026-05-20",
		Time:     "14:00",
		EndTime:  "15:00",
		Type:     appointment.TypeVisita,
		Priority: appointment.PriorityMedia,
		Status:   appointment.StatusAgendado,
	}
	if err := svc.Create(context.Background(), appt); err != nil {
		t.Fatalf("Create error: %v", err)
	}
	if called {
		t.Error("FindConflicts should not have been called without AgentID")
	}
}

func TestAppointmentService_GetByID_NotFound(t *testing.T) {
	svc := NewAppointmentService(newFakeApptRepo(), nil)
	_, err := svc.GetByID(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestAppointmentService_Update_PreservesTenantIDAndCreatedAt(t *testing.T) {
	repo := newFakeApptRepo()
	original := newAppointment("a1", "t1", "agent-1", "2026-05-20", "14:00", "15:00")
	_ = repo.Create(context.Background(), original)

	svc := NewAppointmentService(repo, nil)
	mod := *original
	mod.TenantID = "t2-evil"
	mod.Title = "Updated"
	if err := svc.Update(context.Background(), &mod); err != nil {
		t.Fatalf("Update error: %v", err)
	}

	got := repo.items["a1"]
	if got.TenantID != "t1" {
		t.Errorf("tenant_id should be immutable, got %q", got.TenantID)
	}
	if got.Title != "Updated" {
		t.Errorf("title should be updated, got %q", got.Title)
	}
}

func TestAppointmentService_Update_RejectsConflictOnReschedule(t *testing.T) {
	repo := newFakeApptRepo()
	original := newAppointment("a1", "t1", "agent-1", "2026-05-20", "14:00", "15:00")
	_ = repo.Create(context.Background(), original)
	repo.conflictMatch = func(_, _, _, _, _, _ string) []*appointment.Appointment {
		return []*appointment.Appointment{{ID: "other"}}
	}

	svc := NewAppointmentService(repo, nil)
	mod := *original
	mod.Time = "16:00"
	mod.EndTime = "17:00"
	err := svc.Update(context.Background(), &mod)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("expected ErrConflict on reschedule, got %v", err)
	}
}

func TestAppointmentService_Delete_RejectsCrossTenant(t *testing.T) {
	repo := newFakeApptRepo()
	original := newAppointment("a1", "t1", "agent-1", "2026-05-20", "14:00", "15:00")
	_ = repo.Create(context.Background(), original)

	svc := NewAppointmentService(repo, nil)
	err := svc.Delete(context.Background(), "a1", "t2-other")
	if err == nil {
		t.Error("expected access denied for cross-tenant delete")
	}
	if repo.items["a1"] == nil {
		t.Error("appointment should NOT be deleted across tenants")
	}
}

func TestAppointmentService_Delete_RemovesOwnTenantAppt(t *testing.T) {
	repo := newFakeApptRepo()
	original := newAppointment("a1", "t1", "agent-1", "2026-05-20", "14:00", "15:00")
	_ = repo.Create(context.Background(), original)

	svc := NewAppointmentService(repo, nil)
	if err := svc.Delete(context.Background(), "a1", "t1"); err != nil {
		t.Fatalf("Delete error: %v", err)
	}
	if repo.items["a1"] != nil {
		t.Error("appointment should be deleted")
	}
}

func TestAppointmentService_IdempotentPortalCreation(t *testing.T) {
	repo := newFakeApptRepo()
	svc := NewAppointmentService(repo, nil)
	first := newAppointment("same-request", "agency", "broker", "2099-10-20", "14:00", "15:00")
	first.LeadID = "lead"
	first.CustomerAccountID = "client"
	first.Status = appointment.StatusPendente
	if err := svc.CreateIdempotent(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	first.Status = appointment.StatusConfirmado
	retry := *first
	retry.Status = appointment.StatusPendente
	if err := svc.CreateIdempotent(context.Background(), &retry); err != nil {
		t.Fatal(err)
	}
	if len(repo.items) != 1 || retry.Status != appointment.StatusConfirmado {
		t.Fatal("replay duplicated or reset the confirmed appointment")
	}
	stranger := retry
	stranger.TenantID = "other-agency"
	if err := svc.CreateIdempotent(context.Background(), &stranger); err == nil {
		t.Fatal("idempotency key crossed tenant boundaries")
	}
}

func TestAppointmentService_PortalReschedulingNeedsNewConfirmation(t *testing.T) {
	repo := newFakeApptRepo()
	original := newAppointment("visit", "agency", "broker", "2099-10-20", "14:00", "15:00")
	original.CustomerAccountID = "client"
	original.LeadID = "lead"
	original.DealID = "deal"
	original.Status = appointment.StatusConfirmado
	repo.items[original.ID] = original
	svc := NewAppointmentService(repo, nil)
	changed := *original
	changed.Date = "2099-10-21"
	changed.CustomerAccountID = ""
	changed.LeadID = "another-lead"
	changed.DealID = ""
	if err := svc.Update(context.Background(), &changed); err != nil {
		t.Fatal(err)
	}
	if changed.Status != appointment.StatusReagendado || changed.CustomerAccountID != "client" || changed.LeadID != "lead" || changed.DealID != "deal" {
		t.Fatalf("reschedule lost confirmation or identity: %#v", changed)
	}
}

func TestAppointmentService_CancelDoesNotCheckSchedulingConflicts(t *testing.T) {
	repo := newFakeApptRepo()
	original := newAppointment("visit", "agency", "broker", "2099-10-20", "14:00", "15:00")
	repo.items[original.ID] = original
	repo.conflictMatch = func(_, _, _, _, _, _ string) []*appointment.Appointment {
		return []*appointment.Appointment{{ID: "conflict"}}
	}
	svc := NewAppointmentService(repo, nil)
	cancelled := *original
	cancelled.Status = appointment.StatusCancelado
	cancelled.Time = "14:30"
	if err := svc.Update(context.Background(), &cancelled); err != nil {
		t.Fatalf("cancellation failed due to scheduling conflict: %v", err)
	}
	reactivated := cancelled
	reactivated.Status = appointment.StatusConfirmado
	if err := svc.Update(context.Background(), &reactivated); !errors.Is(err, ErrConflict) {
		t.Fatalf("reactivated visit bypassed conflict check: %v", err)
	}
}
