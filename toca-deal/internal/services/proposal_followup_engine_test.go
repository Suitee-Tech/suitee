package services

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

type fakeFollowUpRepo struct {
	deals   []*deal.Deal
	listErr error
	markErr error
	updates int
}

// List devolve CÓPIAS dos negócios, como faria uma leitura no banco: sem
// isso, um mark que não persistiu (Update com erro) apareceria no tick
// seguinte e mascararia justamente o caso que o teste cobre.
func (f *fakeFollowUpRepo) ListProposalsAwaitingResponse(_ context.Context, olderThan, notOlderThan time.Time, _ int) ([]*deal.Deal, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]*deal.Deal, 0, len(f.deals))
	for _, stored := range f.deals {
		if stored == nil || stored.Status == deal.StatusConverted || stored.Status == deal.StatusCancelled {
			continue
		}
		matched := false
		for _, p := range stored.Proposals {
			if p.AwaitingFor() == "" || p.CreatedAt.After(olderThan) || !p.CreatedAt.After(notOlderThan) {
				continue
			}
			matched = true
			break
		}
		if !matched {
			continue
		}
		clone, err := cloneDeal(stored)
		if err != nil {
			return nil, err
		}
		out = append(out, clone)
	}
	return out, nil
}

func (f *fakeFollowUpRepo) MarkProposalFollowUp(_ context.Context, _, dealID, proposalID string, followUps int, at time.Time) error {
	if f.markErr != nil {
		return f.markErr
	}
	f.updates++
	for _, stored := range f.deals {
		if stored == nil || stored.ID != dealID {
			continue
		}
		if p := stored.ProposalByID(proposalID); p != nil {
			p.FollowUps = followUps
			stamp := at
			p.FollowUpLastAt = &stamp
		}
		return nil
	}
	return nil
}

func cloneDeal(d *deal.Deal) (*deal.Deal, error) {
	raw, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var out deal.Deal
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type capturedEvent struct {
	name    string
	eventID string
	payload any
}

func newCapturingEngine(t *testing.T, repo *fakeFollowUpRepo, sched deal.ProposalFollowUpSchedule) (*ProposalFollowUpEngine, *[]capturedEvent) {
	t.Helper()
	bus := events.NewInMemoryEventBus()
	var mu sync.Mutex
	var got []capturedEvent
	if err := bus.Subscribe("proposal.followup_due", func(_ context.Context, e events.Event) error {
		mu.Lock()
		got = append(got, capturedEvent{name: e.Name(), eventID: e.EventID(), payload: e.Payload()})
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	engine, err := NewProposalFollowUpEngine(repo, bus, nil, sched)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	engine.now = func() time.Time { return now }
	return engine, &got
}

func followUpDeal(proposalStatus deal.ProposalStatus, createdAt time.Time) *deal.Deal {
	return &deal.Deal{
		ID:       "deal-1",
		TenantID: "tenant-1",
		Status:   deal.StatusProposal,
		BrokerID: "broker-1",
		Value:    500000,
		Proposals: []deal.Proposal{{
			ID:        "prop-1",
			Status:    proposalStatus,
			Value:     500000,
			CreatedAt: createdAt,
		}},
	}
}

func TestProposalFollowUpEngine_EnqueuesGroupAnchoredEvent(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeFollowUpRepo{deals: []*deal.Deal{followUpDeal(deal.ProposalStatusSent, now.AddDate(0, 0, -10))}}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3})

	result, err := engine.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Enqueued != 1 || len(*got) != 1 {
		t.Fatalf("esperava 1 lembrete, result=%+v events=%d", result, len(*got))
	}
	evt := (*got)[0]
	wantID := events.DeriveNotificationID("prop-1", "proposal_followup", "1")
	if evt.eventID != wantID {
		t.Fatalf("event_id deveria ser estável por grupo: got %s want %s", evt.eventID, wantID)
	}
	if repo.updates != 1 {
		t.Fatalf("esperava 1 persistência do mark, got %d", repo.updates)
	}
}

// O ponto central da correção: dois ticks antes do mark produzem o MESMO
// event_id. Como o dedup do consumidor é por event_id em comm.sent_externals,
// o segundo tick não vira um segundo e-mail — o lembrete é por grupo, não por
// varredura.
func TestProposalFollowUpEngine_SameGroupNeverGetsASecondEventID(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeFollowUpRepo{
		deals:   []*deal.Deal{followUpDeal(deal.ProposalStatusSent, now.AddDate(0, 0, -10))},
		markErr: errors.New("mark indisponível (simula queda entre publish e mark)"),
	}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3})

	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if len(*got) != 2 {
		t.Fatalf("esperava 2 publicações (o outbox é at-least-once), got %d", len(*got))
	}
	if (*got)[0].eventID != (*got)[1].eventID {
		t.Fatalf("ticks do mesmo grupo devem reusar o event_id: %s != %s", (*got)[0].eventID, (*got)[1].eventID)
	}
}

func TestProposalFollowUpEngine_NoRepeatAfterMark(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeFollowUpRepo{deals: []*deal.Deal{followUpDeal(deal.ProposalStatusSent, now.AddDate(0, 0, -10))}}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3})

	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick 1: %v", err)
	}
	// Um dia depois, ainda dentro do gap: nenhum lembrete novo.
	engine.now = func() time.Time { return now.AddDate(0, 0, 1) }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick 2: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("não pode haver lembrete novo dentro do gap, got %d", len(*got))
	}

	// Passado o gap, o estágio 2 é um grupo novo (event_id diferente).
	engine.now = func() time.Time { return now.AddDate(0, 0, 7) }
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick 3: %v", err)
	}
	if len(*got) != 2 {
		t.Fatalf("esperava estágio 2 depois do gap, got %d", len(*got))
	}
	if (*got)[0].eventID == (*got)[1].eventID {
		t.Fatalf("estágios diferentes devem ter event_ids diferentes")
	}
}

func TestProposalFollowUpEngine_SkipsAnsweredProposal(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeFollowUpRepo{deals: []*deal.Deal{followUpDeal(deal.ProposalStatusAccepted, now.AddDate(0, 0, -30))}}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3})

	result, err := engine.Tick(context.Background())
	if err != nil {
		t.Fatalf("tick: %v", err)
	}
	if result.Enqueued != 0 || len(*got) != 0 {
		t.Fatalf("proposta respondida não gera lembrete, result=%+v events=%d", result, len(*got))
	}
}

func TestProposalFollowUpEngine_CounterOfferChasesBroker(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeFollowUpRepo{deals: []*deal.Deal{followUpDeal(deal.ProposalStatusCounter, now.AddDate(0, 0, -5))}}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 3})

	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("esperava 1 lembrete de contraproposta, got %d", len(*got))
	}
	payload, ok := (*got)[0].payload.(contracts.ProposalFollowUpDueEventPayload)
	if !ok {
		t.Fatalf("payload inesperado: %T", (*got)[0].payload)
	}
	if payload.AwaitingFor != "broker" || payload.ProposalStatus != "COUNTER_OFFER" {
		t.Fatalf("contraproposta deve aguardar o corretor, payload=%+v", payload)
	}
	if payload.BrokerID != "broker-1" || payload.Stage != 1 {
		t.Fatalf("payload sem corretor/estágio esperados: %+v", payload)
	}
}

func TestProposalFollowUpEngine_MaxStagesStops(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	d := followUpDeal(deal.ProposalStatusSent, now.AddDate(0, 0, -60))
	repo := &fakeFollowUpRepo{deals: []*deal.Deal{d}}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{Stage1Days: 3, GapDays: 7, MaxStages: 2, MaxAgeDays: 3650})

	cursor := now
	for i := 0; i < 5; i++ {
		engine.now = func() time.Time { return cursor }
		if _, err := engine.Tick(context.Background()); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
		cursor = cursor.AddDate(0, 0, 30)
	}
	if len(*got) != 2 {
		t.Fatalf("régua deve parar em MaxStages=2, got %d", len(*got))
	}
}

func TestProposalFollowUpEngine_DisabledWhenNoStages(t *testing.T) {
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeFollowUpRepo{deals: []*deal.Deal{followUpDeal(deal.ProposalStatusSent, now.AddDate(0, 0, -60))}}
	engine, got := newCapturingEngine(t, repo, deal.ProposalFollowUpSchedule{MaxStages: 0})

	if engine.Enabled() {
		t.Fatal("MaxStages=0 deve desligar a régua")
	}
	if _, err := engine.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(*got) != 0 {
		t.Fatalf("régua desligada não publica, got %d", len(*got))
	}
}

func TestNewProposalFollowUpEngine_RequiresDependencies(t *testing.T) {
	if _, err := NewProposalFollowUpEngine(nil, events.NewInMemoryEventBus(), nil, deal.ProposalFollowUpSchedule{}); err == nil {
		t.Fatal("esperava erro sem repositório")
	}
	if _, err := NewProposalFollowUpEngine(&fakeFollowUpRepo{}, nil, nil, deal.ProposalFollowUpSchedule{}); err == nil {
		t.Fatal("esperava erro sem barramento")
	}
}
