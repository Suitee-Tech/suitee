package service

import (
	"context"
	"errors"
	"testing"
	"time"

	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func TestLeadVisitPreservesUnrelatedEditDuringCalendarCall(t *testing.T) {
	for _, operation := range []string{"request", "reschedule", "confirm", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			svc, repo, calendar, ctx := visitFixture()
			if operation != "request" {
				if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
					t.Fatal(err)
				}
			}
			edit := func() {
				l := repo.leads["lead"]
				l.Phone, l.Name, l.UpdatedSeq, l.UpdatedAt = "11999999999", "Cliente atualizado", l.UpdatedSeq+1, time.Now()
			}
			calendar.onCreate, calendar.onUpdate = edit, edit
			var err error
			switch operation {
			case "request":
				err = svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00")
			case "reschedule":
				err = svc.UpdateLeadVisit(ctx, "lead", "2099-10-21", "15:00")
			case "confirm":
				err = svc.ConfirmLeadVisit(pkgcontext.WithTenantID(context.Background(), repo.leads["lead"].TenantID), "lead")
			case "cancel":
				err = svc.CancelLeadVisit(ctx, "lead")
			}
			if err != nil {
				t.Fatal(err)
			}
			l := repo.leads["lead"]
			if l.Phone != "11999999999" || l.Name != "Cliente atualizado" || l.VisitRequest == nil || l.VisitRequest.AppointmentID == "" {
				t.Fatalf("concurrent edit/visit lost: %+v", l)
			}
		})
	}
}

func TestVisitPersistenceRejectsChangedVisitOrBroker(t *testing.T) {
	for _, brokerChanged := range []bool{false, true} {
		svc, repo, _, ctx := visitFixture()
		if err := svc.RequestLeadVisit(ctx, "lead", "property", "2099-10-20", "14:00"); err != nil {
			t.Fatal(err)
		}
		before := visitSnapshot(repo.leads["lead"])
		current := repo.leads["lead"]
		if brokerChanged {
			current.AssignedBrokerID = "new-broker"
		} else {
			current.VisitRequest.Status = "Concluido"
		}
		current.UpdatedSeq++
		next := *before.VisitRequest
		next.Status = "Aguardando"
		err := svc.persistVisit(ctx, before, &next)
		if !errors.Is(err, ErrVisitConflict) {
			t.Fatalf("expected conflict: %v", err)
		}
		if !brokerChanged && repo.leads["lead"].VisitRequest.Status != "Concluido" {
			t.Fatal("newer visit was overwritten")
		}
		if brokerChanged && repo.leads["lead"].AssignedBrokerID != "new-broker" {
			t.Fatal("newer broker was overwritten")
		}
	}
}

func TestVisitPersistenceIdenticalRetryDoesNotWriteAgain(t *testing.T) {
	svc, repo, _, ctx := visitFixture()
	before := visitSnapshot(repo.leads["lead"])
	next := &lead.VisitRequest{AppointmentID: "appointment", Status: "Aguardando"}
	if err := svc.persistVisit(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	seq := repo.leads["lead"].UpdatedSeq
	if err := svc.persistVisit(ctx, before, next); err != nil {
		t.Fatal(err)
	}
	if repo.leads["lead"].UpdatedSeq != seq {
		t.Fatal("identical retry changed persisted version")
	}
}
