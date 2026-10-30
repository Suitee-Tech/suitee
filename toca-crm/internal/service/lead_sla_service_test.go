package service

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// captureRepo only implements the lead.Repository methods that triggerEscalation
// and Patch tests touch. All other methods inherit a nil embedded interface and
// will panic if accidentally invoked — that's a useful guard during refactors.
type captureRepo struct {
	lead.Repository
	updated     []*lead.Lead
	created     []*lead.Lead
	existing    *lead.Lead
	patchCalls  []map[string]interface{}
	patchResult *lead.Lead
	patchOk     bool
}

// Create registra o agregado como o serviço o entregou ao repositório — é o que
// permite afirmar que o telefone já chega normalizado.
func (r *captureRepo) Create(_ context.Context, l *lead.Lead) error {
	clone := *l
	r.created = append(r.created, &clone)
	return nil
}

func (r *captureRepo) Update(_ context.Context, l *lead.Lead) error {
	clone := *l
	r.updated = append(r.updated, &clone)
	return nil
}

func (r *captureRepo) GetByID(_ context.Context, _, _ string) (*lead.Lead, error) {
	if r.existing == nil {
		return &lead.Lead{}, nil
	}
	clone := *r.existing
	return &clone, nil
}

func (r *captureRepo) Patch(_ context.Context, _, _ string, _ *lead.LeadExpectedVersion, updates map[string]interface{}) (*lead.Lead, bool, error) {
	// Copy the updates map so the assertion sees what the service actually
	// passed in (the caller is allowed to mutate after the call returns).
	clone := make(map[string]interface{}, len(updates))
	for k, v := range updates {
		clone[k] = v
	}
	r.patchCalls = append(r.patchCalls, clone)
	if r.patchResult != nil {
		return r.patchResult, r.patchOk, nil
	}
	return &lead.Lead{}, true, nil
}

// captureBus records every event passed to PublishReliable so tests can assert
// the escalation level transition.
type captureBus struct {
	events.EventBus
	reliable []events.Event
}

func (b *captureBus) PublishReliable(_ context.Context, evt events.Event) error {
	b.reliable = append(b.reliable, evt)
	return nil
}

func TestTriggerEscalation_IncrementsLevelAndEmitsBothBoundaries(t *testing.T) {
	repo := &captureRepo{}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, eventBus: bus}

	l := &lead.Lead{ID: "lead-1", TenantID: "t-1", EscalationLevel: 0}
	cfg := &lead.RouletteConfig{
		MaxEscalations: 3,
		Escalation: []lead.EscalationStep{
			{ID: "s1", Label: "Step 1", Action: "notify_manager"},
			{ID: "s2", Label: "Step 2", Action: "escalate"},
			{ID: "s3", Label: "Step 3", Action: "mark_urgent"},
		},
	}

	for tick := 1; tick <= 3; tick++ {
		svc.triggerEscalation(context.Background(), l, cfg)
	}

	if l.EscalationLevel != 3 {
		t.Fatalf("expected level 3 after 3 ticks, got %d", l.EscalationLevel)
	}
	if len(bus.reliable) != 3 {
		t.Fatalf("expected 3 published events, got %d", len(bus.reliable))
	}
	for i, evt := range bus.reliable {
		e, ok := evt.(LeadEscalatedEvent)
		if !ok {
			t.Fatalf("event %d wrong type %T", i, evt)
		}
		wantPrev, wantNew := i, i+1
		if e.PreviousLevel != wantPrev || e.Level != wantNew {
			t.Fatalf("event %d: prev=%d new=%d, want prev=%d new=%d", i, e.PreviousLevel, e.Level, wantPrev, wantNew)
		}
	}
}

func TestTriggerEscalation_CapsAtMaxAndMarksLost(t *testing.T) {
	repo := &captureRepo{}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, eventBus: bus}

	l := &lead.Lead{ID: "lead-2", TenantID: "t-1", EscalationLevel: 3, Status: lead.LeadStatusPendingAcceptance}
	cfg := &lead.RouletteConfig{
		MaxEscalations: 3,
		Escalation:     []lead.EscalationStep{{ID: "s1", Label: "Step", Action: "notify"}},
	}

	svc.triggerEscalation(context.Background(), l, cfg)

	if l.Status != lead.LeadStatusLost {
		t.Fatalf("expected status LOST after cap, got %s", l.Status)
	}
	if l.EscalationLevel != 3 {
		t.Fatalf("expected level unchanged at cap, got %d", l.EscalationLevel)
	}
	if len(bus.reliable) != 0 {
		t.Fatalf("expected no events when capped, got %d", len(bus.reliable))
	}
}

func TestTriggerEscalation_UsesDefaultWhenCapZero(t *testing.T) {
	repo := &captureRepo{}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, eventBus: bus}

	l := &lead.Lead{ID: "lead-3", TenantID: "t-1", EscalationLevel: lead.DefaultMaxEscalations}
	cfg := &lead.RouletteConfig{
		Escalation: []lead.EscalationStep{{ID: "s1", Label: "Step", Action: "notify"}},
	}

	svc.triggerEscalation(context.Background(), l, cfg)

	if l.Status != lead.LeadStatusLost {
		t.Fatalf("expected status LOST when level >= DefaultMaxEscalations, got %s", l.Status)
	}
}

// quietRouletteRepo lets the worker tick without doing any DB work.
type quietRouletteRepo struct {
	lead.RouletteRepository
	tenants []string
	calls   int
}

func (r *quietRouletteRepo) ListEnabledTenantIDs(_ context.Context) ([]string, error) {
	r.calls++
	return r.tenants, nil
}

func TestRunSLAWorker_StopsOnContextCancel(t *testing.T) {
	prev := SLAWorkerInterval
	SLAWorkerInterval = 10 * time.Millisecond
	t.Cleanup(func() { SLAWorkerInterval = prev })

	repo := &quietRouletteRepo{}
	svc := &LeadSLAService{rouletteRepo: repo}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		svc.RunSLAWorker(ctx)
		close(done)
	}()

	// Let it tick a few times so we know the loop is alive.
	time.Sleep(40 * time.Millisecond)
	if repo.calls == 0 {
		t.Fatalf("worker never executed CheckSLAViolations")
	}

	cancel()

	select {
	case <-done:
		// expected: worker returned cleanly
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("worker did not stop within 200ms after context cancel")
	}
}

func TestTriggerEscalation_NoOpWithoutSteps(t *testing.T) {
	repo := &captureRepo{}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, eventBus: bus}

	l := &lead.Lead{ID: "lead-4", EscalationLevel: 0}
	cfg := &lead.RouletteConfig{MaxEscalations: 3, Escalation: nil}

	svc.triggerEscalation(context.Background(), l, cfg)

	if l.EscalationLevel != 0 {
		t.Fatalf("expected no change without steps, got %d", l.EscalationLevel)
	}
	if len(bus.reliable) != 0 || len(repo.updated) != 0 {
		t.Fatalf("expected no side effects without steps")
	}
}

// ─────────────── #427 First Contact SLA ───────────────

// firstContactRepo extends captureRepo to return a fixed slice of
// "breaching" leads from ListFirstContactSLABreaches.
type firstContactRepo struct {
	captureRepo
	breaches []*lead.Lead
}

func (r *firstContactRepo) ListFirstContactSLABreaches(_ context.Context, _ string, _ time.Time, _ int64) ([]*lead.Lead, error) {
	out := make([]*lead.Lead, len(r.breaches))
	for i, l := range r.breaches {
		clone := *l
		out[i] = &clone
	}
	return out, nil
}

func TestCheckFirstContactSLAViolations_MarksAndPublishes(t *testing.T) {
	now := time.Now()
	assigned := now.Add(-90 * time.Minute) // 1h30 atrás — bem além do SLA

	breaching := &lead.Lead{
		ID:               "lead-fc-1",
		TenantID:         "t-1",
		Status:           lead.LeadStatusPendingAcceptance,
		AssignedAt:       assigned,
		AssignedBrokerID: "broker-1",
	}
	repo := &firstContactRepo{breaches: []*lead.Lead{breaching}}
	rouletteRepo := &quietRouletteRepo{tenants: []string{"t-1"}}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, rouletteRepo: rouletteRepo, eventBus: bus}

	if err := svc.CheckFirstContactSLAViolations(context.Background()); err != nil {
		t.Fatalf("CheckFirstContactSLAViolations: %v", err)
	}

	// 1 update marcando FirstContactSLAViolatedAt
	if len(repo.updated) != 1 {
		t.Fatalf("expected 1 Update call, got %d", len(repo.updated))
	}
	if repo.updated[0].FirstContactSLAViolatedAt == nil {
		t.Error("FirstContactSLAViolatedAt not set on lead")
	}

	// 1 evento publicado
	if len(bus.reliable) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(bus.reliable))
	}
	evt, ok := bus.reliable[0].(LeadFirstContactSLAViolatedEvent)
	if !ok {
		t.Fatalf("event wrong type %T", bus.reliable[0])
	}
	if evt.LeadID != "lead-fc-1" || evt.AssignedBrokerID != "broker-1" {
		t.Errorf("event payload wrong: %+v", evt)
	}
	if evt.SLAMinutes != FirstContactSLAMinutes {
		t.Errorf("SLAMinutes = %d, want %d", evt.SLAMinutes, FirstContactSLAMinutes)
	}
	if evt.MinutesOverdue < 25 {
		// 90min - 60min SLA = 30min overdue, com tolerância de timing
		t.Errorf("MinutesOverdue = %d, want >= 25", evt.MinutesOverdue)
	}
}

func TestCheckFirstContactSLAViolations_NoBreachesIsNoop(t *testing.T) {
	repo := &firstContactRepo{breaches: nil}
	rouletteRepo := &quietRouletteRepo{tenants: []string{"t-1"}}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, rouletteRepo: rouletteRepo, eventBus: bus}

	if err := svc.CheckFirstContactSLAViolations(context.Background()); err != nil {
		t.Fatalf("CheckFirstContactSLAViolations: %v", err)
	}
	if len(repo.updated) != 0 {
		t.Errorf("expected no Update calls, got %d", len(repo.updated))
	}
	if len(bus.reliable) != 0 {
		t.Errorf("expected no events, got %d", len(bus.reliable))
	}
}

// TestCheckFirstContactSLAViolations_GuardSkipsMarkedAndZeroAssigned trava o
// fix da enxurrada do #427: o filtro primário vive na query
// ListFirstContactSLABreaches, mas o worker tem uma defesa em profundidade.
// Se a query regredir e devolver um lead já notificado (FirstContactSLAViolatedAt
// setado) ou nunca atribuído (AssignedAt zero), o worker NÃO pode re-disparar
// o evento — caso contrário a Central de Notificações volta a inundar.
func TestCheckFirstContactSLAViolations_GuardSkipsMarkedAndZeroAssigned(t *testing.T) {
	now := time.Now()
	marked := now.Add(-30 * time.Minute)
	assigned := now.Add(-90 * time.Minute)

	// Lead já notificado: a query corrigida não deveria devolvê-lo, mas
	// se devolver (regressão de SQL), o guard do worker o ignora.
	alreadyMarked := &lead.Lead{
		ID:                        "lead-fc-marked",
		TenantID:                  "t-1",
		Status:                    lead.LeadStatusPendingAcceptance,
		AssignedAt:                assigned,
		AssignedBrokerID:          "broker-1",
		FirstContactSLAViolatedAt: &marked,
	}
	// Lead nunca atribuído: AssignedAt zero → now.Sub(zero) infla as "horas
	// de atraso" para ~2025 anos; o guard impede o evento.
	neverAssigned := &lead.Lead{
		ID:       "lead-fc-zero",
		TenantID: "t-1",
		Status:   lead.LeadStatusNew,
	}

	repo := &firstContactRepo{breaches: []*lead.Lead{alreadyMarked, neverAssigned}}
	rouletteRepo := &quietRouletteRepo{tenants: []string{"t-1"}}
	bus := &captureBus{}
	svc := &LeadSLAService{leadRepo: repo, rouletteRepo: rouletteRepo, eventBus: bus}

	if err := svc.CheckFirstContactSLAViolations(context.Background()); err != nil {
		t.Fatalf("CheckFirstContactSLAViolations: %v", err)
	}

	if len(repo.updated) != 0 {
		t.Errorf("expected no Update calls (both leads guarded), got %d", len(repo.updated))
	}
	if len(bus.reliable) != 0 {
		t.Errorf("expected no events (#427 flood guard), got %d", len(bus.reliable))
	}
}
