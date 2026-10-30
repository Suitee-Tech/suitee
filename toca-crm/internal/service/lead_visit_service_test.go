package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

type visitCalendarFake struct {
	items              map[string]*clients.Appointment
	creates, updates   int
	fail               error
	lastRequest        clients.CreateAppointmentRequest
	onCreate, onUpdate func()
}

func (f *visitCalendarFake) CreateAppointment(_ context.Context, r clients.CreateAppointmentRequest) (*clients.Appointment, error) {
	f.creates++
	f.lastRequest = r
	if f.fail != nil {
		return nil, f.fail
	}
	if existing := f.items[r.ID]; existing != nil {
		return existing, nil
	}
	if f.onCreate != nil {
		f.onCreate()
	}
	appt := &clients.Appointment{ID: r.ID, TenantID: r.TenantID.String(), LeadID: r.LeadID, AgentID: r.AgentID, CustomerAccountID: r.CustomerAccountID, PropertyID: r.PropertyID, Date: r.Date, Time: r.Time, EndTime: r.EndTime, Status: r.Status, Description: r.Description, UpdatedAt: time.Now()}
	f.items[r.ID] = appt
	return appt, nil
}
func (f *visitCalendarFake) UpdateAppointment(_ context.Context, id string, r clients.UpdateAppointmentRequest) (*clients.Appointment, error) {
	f.updates++
	if f.fail != nil {
		return nil, f.fail
	}
	appt := f.items[id]
	if appt == nil {
		return nil, clients.ErrCalendarNotFound
	}
	if r.ExpectedUpdatedAt != nil && !r.ExpectedUpdatedAt.Equal(appt.UpdatedAt) {
		return nil, clients.ErrCalendarChanged
	}
	copy := *appt
	copy.UpdatedAt = time.Now()
	if r.Date != "" {
		copy.Date = r.Date
	}
	if r.Time != "" {
		copy.Time = r.Time
	}
	if r.EndTime != "" {
		copy.EndTime = r.EndTime
	}
	if r.Status != "" {
		copy.Status = r.Status
	}
	if r.PropertyID != "" {
		copy.PropertyID = r.PropertyID
	}
	if r.Description != nil {
		copy.Description = *r.Description
	}
	f.items[id] = &copy
	if f.onUpdate != nil {
		f.onUpdate()
	}
	return &copy, nil
}
func (f *visitCalendarFake) GetAppointment(_ context.Context, id, tenantID string) (*clients.Appointment, error) {
	if f.fail != nil {
		return nil, f.fail
	}
	if appt := f.items[id]; appt != nil && appt.TenantID == tenantID {
		copy := *appt
		return &copy, nil
	}
	return nil, clients.ErrCalendarNotFound
}
func visitFixture() (*LeadService, *inMemoryAcceptLeadRepo, *visitCalendarFake, context.Context) {
	l := &lead.Lead{ID: "lead", TenantID: "11111111-1111-4111-8111-111111111111", AccountID: "client", Name: "Lucas", Email: "client@example.test", AssignedBrokerID: "broker", RecommendedPropertyID: "property"}
	repo := newAcceptRepo(l)
	calendar := &visitCalendarFake{items: map[string]*clients.Appointment{}}
	ctx := pkgcontext.WithAccountID(context.Background(), "client")
	return (&LeadService{repo: repo}).WithCalendarClient(calendar), repo, calendar, ctx
}

func TestLeadVisitLifecycleUsesOneAppointmentAndAgendaState(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	details := VisitDetails{EndTime: "15:30", Note: "Preciso de acesso à garagem"}
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00", details); err != nil {
		t.Fatal(err)
	}
	original := repo.leads["lead"].VisitRequest.AppointmentID
	if calendar.lastRequest.Status != "Pendente" || calendar.lastRequest.CustomerAccountID != "client" || calendar.lastRequest.AgentID != "broker" || calendar.lastRequest.EndTime != "15:30" || calendar.lastRequest.Description != details.Note {
		t.Fatalf("incomplete appointment: %#v", calendar.lastRequest)
	}
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00", details); err != nil {
		t.Fatal(err)
	}
	if calendar.creates != 1 || calendar.updates != 0 {
		t.Fatal("identical request duplicated or changed the appointment")
	}
	// Simulate the broker confirming in /agenda, then changing its date/time.
	calendar.items[original].Status = "Confirmado"
	overview, err := svc.ClientPortalOverview(ctx, "client", "")
	if err != nil || overview[0].VisitRequest.Status != "Confirmado" {
		t.Fatalf("agenda confirmation not reflected: %#v %v", overview, err)
	}
	calendar.items[original].Status = "Reagendado"
	calendar.items[original].Date = "2099-10-22"
	calendar.items[original].Time = "16:00"
	calendar.items[original].EndTime = "17:00"
	overview, err = svc.ClientPortalOverview(ctx, "client", "")
	if err != nil || overview[0].VisitRequest.Date != "2099-10-22" || overview[0].VisitRequest.Time != "16:00" || overview[0].VisitRequest.Status != "Reagendado" {
		t.Fatalf("agenda rescheduling not reflected: %#v %v", overview, err)
	}
	if err := svc.UpdateLeadVisit(ctx, "lead", "2099-10-23", "10:00", VisitDetails{EndTime: "11:30"}); err != nil {
		t.Fatal(err)
	}
	if calendar.creates != 1 || repo.leads["lead"].VisitRequest.AppointmentID != original || calendar.items[original].Status != "Pendente" {
		t.Fatal("client rescheduling lost the original appointment")
	}
	if err := svc.CancelLeadVisit(ctx, "lead"); err != nil {
		t.Fatal(err)
	}
	if calendar.items[original] == nil || calendar.items[original].Status != "Cancelado" {
		t.Fatal("cancelled appointment history was lost")
	}
	if err := svc.CancelLeadVisit(ctx, "lead"); err != nil {
		t.Fatal("repeated cancellation failed:", err)
	}
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-24", "10:00", VisitDetails{EndTime: "11:00"}); err != nil {
		t.Fatal(err)
	}
	if repo.leads["lead"].VisitRequest.AppointmentID == original || calendar.items[original].Status != "Cancelado" {
		t.Fatal("new visit overwrote cancelled visit history")
	}
}

func TestLeadVisitDoesNotPersistFalseSuccessOnCalendarFailure(t *testing.T) {
	for _, failure := range []error{clients.ErrCalendarConflict, errors.New("network error")} {
		svc, repo, calendar, ctx := visitFixture()
		calendar.fail = failure
		err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00")
		if err == nil || repo.leads["lead"].VisitRequest != nil {
			t.Fatalf("failed agenda call persisted visit: %v", err)
		}
		if failure == clients.ErrCalendarConflict && !errors.Is(err, failure) {
			t.Fatal("conflict was not propagated")
		}
	}
	svc, repo, calendar, ctx := visitFixture()
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
		t.Fatal(err)
	}
	calendar.fail = clients.ErrCalendarConflict
	if err := svc.UpdateLeadVisit(ctx, "lead", "2099-10-21", "15:00"); err == nil {
		t.Fatal("failed update reported success")
	}
	if repo.leads["lead"].VisitRequest.Date != "2099-10-20" {
		t.Fatal("failed update modified the stored request")
	}
	if err := svc.CancelLeadVisit(ctx, "lead"); err == nil || repo.leads["lead"].VisitRequest.Status == "Cancelado" {
		t.Fatal("failed cancellation reported success")
	}
}

func TestLeadVisitOwnershipValidationAndClosedVisits(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	stranger := pkgcontext.WithAccountID(context.Background(), "stranger")
	if err := svc.RequestLeadVisit(stranger, "lead", "property", "2099-10-20", "14:00"); !errors.Is(err, ErrVisitForbidden) {
		t.Fatalf("ownership: %v", err)
	}
	for _, input := range []struct{ date, start, end string }{{"invalid", "14:00", "15:00"}, {"2000-01-01", "14:00", "15:00"}, {"2099-10-20", "24:00", "25:00"}, {"2099-10-20", "14:00", "13:00"}, {"2099-10-20", "23:30", ""}} {
		if err := svc.RequestLeadVisit(ctx, "lead", "property", input.date, input.start, VisitDetails{EndTime: input.end}); !errors.Is(err, ErrVisitInvalid) {
			t.Fatalf("invalid slot accepted: %#v %v", input, err)
		}
	}
	if calendar.creates != 0 {
		t.Fatal("invalid/unauthorized request reached calendar")
	}
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
		t.Fatal(err)
	}
	id := repo.leads["lead"].VisitRequest.AppointmentID
	calendar.items[id].Status = "Concluido"
	if err := svc.UpdateLeadVisit(ctx, "lead", "2099-10-21", "14:00"); !errors.Is(err, ErrVisitClosed) {
		t.Fatalf("completed visit reopened: %v", err)
	}
	// SECURITY (achado C9): operador de outro tenant agora é recusado já na
	// leitura tenant-scoped — respondemos "não encontrada" (não vaza sequer a
	// existência do lead), em vez do antigo Forbidden que exigia carregar a
	// linha alheia.
	wrongTenant := pkgcontext.WithTenantID(context.Background(), "other")
	if err := svc.ConfirmLeadVisit(wrongTenant, "lead"); !errors.Is(err, ErrVisitNotFound) {
		t.Fatalf("operator tenant check: esperava ErrVisitNotFound, veio %v", err)
	}
}

type visitSaveFailureRepo struct {
	*inMemoryAcceptLeadRepo
	fail bool
}

func (r *visitSaveFailureRepo) Patch(ctx context.Context, id, tenantID string, expected *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	if r.fail {
		return nil, false, errors.New("CRM temporarily unavailable")
	}
	return r.inMemoryAcceptLeadRepo.Patch(ctx, id, tenantID, expected, updates)
}
func TestLeadVisitRetryRecoversCalendarCreationAfterCRMFailure(t *testing.T) {
	svc, original, calendar, ctx := visitFixture()
	repo := &visitSaveFailureRepo{inMemoryAcceptLeadRepo: original, fail: true}
	svc.repo = repo
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err == nil {
		t.Fatal("expected CRM save failure")
	}
	repo.fail = false
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
		t.Fatal(err)
	}
	if len(calendar.items) != 1 || original.leads["lead"].VisitRequest.AppointmentID == "" {
		t.Fatal("retry duplicated or failed to link the appointment")
	}
}

// O overview é o cartão de entrada do portal do cliente e a agenda é apenas
// ENRIQUECIMENTO dele (reflete o que o corretor confirmou no calendário).
//
// Incidente de 2026-09-30: com o calendário inacessível — no beta, a variável
// CALENDAR_SERVICE_URL não existia no serviço do CRM, então TODA chamada
// falhava — o overview respondia 503 e o cliente via "Não foi possível
// carregar sua busca" mesmo com a visita intacta na agenda. O estado
// persistido da visita é a melhor informação disponível e o portal precisa
// continuar funcionando.
func TestPortalOverviewSurvivesCalendarOutageWithStoredVisit(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00", VisitDetails{EndTime: "15:00"}); err != nil {
		t.Fatal(err)
	}
	appointmentID := repo.leads["lead"].VisitRequest.AppointmentID

	calendar.fail = errors.New("connection refused")

	overview, err := svc.ClientPortalOverview(ctx, "client", "")
	if err != nil {
		t.Fatalf("falha do calendário derrubou o overview do portal: %v", err)
	}
	if len(overview) != 1 || overview[0].VisitRequest == nil {
		t.Fatalf("visita persistida sumiu do overview: %#v", overview)
	}
	visit := overview[0].VisitRequest
	if visit.AppointmentID != appointmentID || visit.Date != "2099-10-20" || visit.Time != "14:00" || visit.Status != "Aguardando" {
		t.Fatalf("estado persistido da visita não foi preservado: %#v", visit)
	}

	// Com o calendário de volta, o estado da agenda volta a mandar no overview.
	calendar.fail = nil
	calendar.items[appointmentID].Status = "Confirmado"
	overview, err = svc.ClientPortalOverview(ctx, "client", "")
	if err != nil || overview[0].VisitRequest.Status != "Confirmado" {
		t.Fatalf("overview não voltou a refletir a agenda: %#v %v", overview, err)
	}
}
