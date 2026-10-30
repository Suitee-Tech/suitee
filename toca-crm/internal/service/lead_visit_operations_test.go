package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func TestVisitDurableIntentRecoversAfterRestartWithoutDuplicating(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	calendar.fail = errors.New("response lost")
	details := VisitDetails{EndTime: "16:00", Note: "Garagem"}
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00", details); err == nil {
		t.Fatal("failure reported success")
	}
	op := repo.leads["lead"].VisitOperation
	if op == nil || repo.leads["lead"].VisitRequest != nil {
		t.Fatal("durable intent missing or false completed visit")
	}
	id := op.Desired.AppointmentID
	// Calendar committed the write, but the first process lost its response.
	calendar.fail = nil
	calendar.items[id] = &clients.Appointment{ID: id, TenantID: repo.leads["lead"].TenantID, LeadID: "lead", AgentID: "broker", CustomerAccountID: "client", PropertyID: "property", Date: "2099-10-20", Time: "14:00", EndTime: "16:00", Description: "Garagem", Status: "Pendente"}
	restarted := (&LeadService{repo: repo}).WithCalendarClient(calendar)
	if err := restarted.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00", details); err != nil {
		t.Fatal(err)
	}
	l := repo.leads["lead"]
	if l.VisitOperation != nil || l.VisitRequest == nil || l.VisitRequest.AppointmentID != id || len(calendar.items) != 1 {
		t.Fatal("recovery duplicated or lost visit")
	}
	if err := restarted.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00", VisitDetails{EndTime: "17:00", Note: "Nova nota"}); err != nil {
		t.Fatal(err)
	}
	if calendar.items[id].EndTime != "17:00" || l.VisitRequest.EndTime != "16:00" || repo.leads["lead"].VisitRequest.EndTime != "17:00" {
		t.Fatal("retry payload diverged from canonical calendar")
	}
}

func TestVisitAssignmentConflictCancelsOrphanAndAllowsFreshRequest(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	calendar.onCreate = func() { l := repo.leads["lead"]; l.AssignedBrokerID = "new-broker"; l.UpdatedSeq++ }
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); !errors.Is(err, ErrVisitConflict) {
		t.Fatalf("expected conflict: %v", err)
	}
	if repo.leads["lead"].VisitRequest != nil || repo.leads["lead"].VisitOperation != nil {
		t.Fatal("conflicted operation not compensated")
	}
	for _, appt := range calendar.items {
		if appt.Status != "Cancelado" {
			t.Fatal("orphan occupies slot")
		}
	}
	calendar.onCreate = nil
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
		t.Fatal(err)
	}
	if len(calendar.items) != 2 || repo.leads["lead"].VisitRequest.Status != "Aguardando" {
		t.Fatal("fresh request reused cancelled operation")
	}
}

func TestVisitLeasePreventsParallelMutation(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	calendar.fail = errors.New("network")
	_ = svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00")
	repo.leads["lead"].VisitOperation.LeaseUntil = time.Now().Add(time.Minute)
	calendar.fail = nil
	calls := calendar.creates
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); !errors.Is(err, ErrVisitConflict) {
		t.Fatal(err)
	}
	if calendar.creates != calls {
		t.Fatal("parallel request executed reserved operation")
	}
}

func TestVisitRecoveryDoesNotOverwriteNewerCalendarEdit(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
		t.Fatal(err)
	}
	id := repo.leads["lead"].VisitRequest.AppointmentID
	calendar.fail = errors.New("network")
	// Intent reservation needs its initial read to succeed; fail on mutation.
	before := visitSnapshot(repo.leads["lead"])
	desired := *before.VisitRequest
	desired.Time = "16:00"
	desired.EndTime = "17:00"
	calendar.fail = nil
	// Reserve a crashed operation, preserving the originally observed version.
	priorVersion := calendar.items[id].UpdatedAt
	before.VisitOperation = &lead.VisitOperation{ID: "op", Token: "crashed", Before: before.VisitRequest, Desired: &desired, BrokerID: before.AssignedBrokerID, AccountID: before.AccountID, ExpectedAgendaUpdatedAt: &priorVersion}
	repo.leads["lead"].VisitOperation = before.VisitOperation
	calendar.items[id].Time = "18:00"
	calendar.items[id].EndTime = "19:00"
	calendar.items[id].UpdatedAt = time.Now().Add(time.Second)
	err := svc.recoverVisitOperation(ctx, repo.leads["lead"])
	if !errors.Is(err, clients.ErrCalendarChanged) || calendar.items[id].Time != "18:00" || repo.leads["lead"].VisitOperation != nil {
		t.Fatalf("newer agenda edit overwritten: %v", err)
	}
}

func (r *inMemoryAcceptLeadRepo) ListPendingVisitOperations(ctx context.Context, limit int) ([]*lead.Lead, error) {
	var result []*lead.Lead
	for _, l := range r.leads {
		if l.VisitOperation != nil && !l.VisitOperation.LeaseUntil.After(time.Now()) {
			result = append(result, visitSnapshot(l))
		}
	}
	return result, nil
}

func TestVisitRecoveryWorkerCompletesIntentWithoutClientRetry(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	calendar.fail = errors.New("network")
	_ = svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00")
	id := repo.leads["lead"].VisitOperation.Desired.AppointmentID
	calendar.fail = nil
	restarted := (&LeadService{repo: repo}).WithCalendarClient(calendar)
	if err := restarted.recoverPendingVisits(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.leads["lead"].VisitOperation != nil || repo.leads["lead"].VisitRequest == nil || repo.leads["lead"].VisitRequest.AppointmentID != id || len(calendar.items) != 1 {
		t.Fatal("worker did not recover durable intent")
	}
}

func TestCreateRecoveryAdoptsBrokerEditedCanonicalAppointment(t *testing.T) {
	svc, repo, calendar, ctx := visitFixture()
	calendar.fail = errors.New("network")
	_ = svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00")
	id := repo.leads["lead"].VisitOperation.Desired.AppointmentID
	calendar.fail = nil
	calendar.items[id] = &clients.Appointment{ID: id, TenantID: repo.leads["lead"].TenantID, LeadID: "lead", AgentID: "another-agenda-agent", CustomerAccountID: "client", PropertyID: "property", Date: "2099-10-22", Time: "16:00", EndTime: "17:00", Description: "Corretor ajustou", Status: "Reagendado"}
	restarted := (&LeadService{repo: repo}).WithCalendarClient(calendar)
	if err := restarted.recoverPendingVisits(context.Background()); err != nil {
		t.Fatal(err)
	}
	v := repo.leads["lead"].VisitRequest
	if repo.leads["lead"].VisitOperation != nil || v == nil || v.Date != "2099-10-22" || v.EndTime != "17:00" || v.Note != "Corretor ajustou" || v.Status != "Reagendado" || len(calendar.items) != 1 {
		t.Fatal("canonical edit lost or intent discarded")
	}
}
