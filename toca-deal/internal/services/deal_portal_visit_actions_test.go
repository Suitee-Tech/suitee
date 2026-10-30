package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
)

func visitActionFixture(t *testing.T, a *clients.Appointment, updateErr error) (*DealService, *clients.UpdateAppointmentRequest, *int) {
	t.Helper()
	d := &deal.Deal{ID: "deal", TenantID: "agency", CustomerID: "lead", Parties: []deal.Party{{AccountID: "client", LeadID: "lead", Role: deal.PartyRoleBuyer}}}
	payload := &clients.UpdateAppointmentRequest{}
	writes := new(int)
	base := clients.NewBaseClient("http://calendar.test")
	base.InternalAPIKey = "test-key"
	base.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == http.MethodGet {
			if r.URL.Query().Get("tenant_id") != d.TenantID {
				t.Fatal("missing tenant")
			}
		} else if r.Method == http.MethodPut {
			*writes++
			if err := json.NewDecoder(r.Body).Decode(payload); err != nil {
				t.Fatal(err)
			}
			if updateErr != nil {
				return nil, updateErr
			}
			if payload.ExpectedUpdatedAt == nil || !payload.ExpectedUpdatedAt.Equal(a.UpdatedAt) {
				t.Fatal("missing concurrency guard")
			}
			if payload.TenantID != a.TenantID || payload.DealID != "" || payload.PropertyID != "" || payload.AgentID != "" || payload.LeadID != "" {
				t.Fatalf("changed appointment relationships: %+v", payload)
			}
			if payload.Date != "" {
				a.Date, a.Time, a.EndTime = payload.Date, payload.Time, payload.EndTime
			}
			if payload.Description != nil {
				a.Description = *payload.Description
			}
			a.Status = payload.Status
			a.UpdatedAt = a.UpdatedAt.Add(time.Second)
		} else {
			t.Fatalf("unexpected request %s", r.Method)
		}
		body, _ := json.Marshal(map[string]interface{}{"data": a})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}}, nil
	})
	return &DealService{repo: portalVisitDeals{newInMemoryDealRepo(d), d}, calendarClient: &clients.CalendarClient{BaseClient: base}}, payload, writes
}

func futureVisit() *clients.Appointment {
	return &clients.Appointment{ID: "visit", TenantID: "agency", DealID: "deal", LeadID: "lead", CustomerAccountID: "client", PropertyID: "property", AgentID: "broker", Type: "Visita", Status: "Confirmado", Date: "2099-10-20", Time: "14:00", EndTime: "15:00", UpdatedAt: time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC)}
}

func TestPortalVisitRescheduleKeepsSameAgendaAppointment(t *testing.T) {
	for _, preDeal := range []bool{false, true} {
		t.Run(map[bool]string{false: "deal", true: "lead"}[preDeal], func(t *testing.T) {
			a := futureVisit()
			if preDeal {
				a.DealID = ""
			}
			svc, payload, writes := visitActionFixture(t, a, nil)
			note := "Entrada pela portaria"
			updated, err := svc.UpdateMyDealAppointment(context.Background(), "client", "", "deal", "visit", portal.UpdateAppointmentRequest{ExpectedUpdatedAt: a.UpdatedAt, Date: "2099-10-21", Time: "16:00", EndTime: "17:00", Description: &note})
			if err != nil {
				t.Fatal(err)
			}
			if *writes != 1 || updated.ID != "visit" || updated.PropertyID != "property" || updated.AgentID != "broker" || updated.Time != "16:00" || updated.Status != "Reagendado" || !updated.CanManage || payload.CustomerAccountID != "client" {
				t.Fatalf("incorrect reschedule: %+v", updated)
			}
		})
	}
}

func TestPortalVisitCancellationPreservesTracking(t *testing.T) {
	a := futureVisit()
	a.CustomerAccountID = "" // Broker-created visit resolves the client via its lead.
	svc, payload, writes := visitActionFixture(t, a, nil)
	updated, err := svc.CancelMyDealAppointment(context.Background(), "client", "", "deal", "visit", portal.CancelAppointmentRequest{ExpectedUpdatedAt: a.UpdatedAt})
	if err != nil {
		t.Fatal(err)
	}
	if *writes != 1 || updated.ID != "visit" || updated.Status != "Cancelado" || updated.CanManage || updated.Date != "2099-10-20" || payload.Date != "" || payload.CustomerAccountID != "client" {
		t.Fatalf("lost tracking on cancellation: %+v", updated)
	}
}

func TestPortalVisitActionsRejectUnauthorizedClosedAndStaleVisits(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*clients.Appointment)
		expected error
	}{
		{"another deal", func(a *clients.Appointment) { a.DealID = "other" }, ErrPortalAppointmentAccess},
		{"another client", func(a *clients.Appointment) { a.CustomerAccountID = "other" }, ErrPortalAppointmentAccess},
		{"another pre-deal lead", func(a *clients.Appointment) { a.DealID = ""; a.LeadID = "other" }, ErrPortalAppointmentAccess},
		{"completed", func(a *clients.Appointment) { a.Status = "Concluido" }, ErrPortalAppointmentClosed},
		{"cancelled", func(a *clients.Appointment) { a.Status = "Cancelado" }, ErrPortalAppointmentClosed},
		{"started", func(a *clients.Appointment) { a.Status = "Em Andamento" }, ErrPortalAppointmentClosed},
		{"past", func(a *clients.Appointment) { a.Date = "2000-01-01" }, ErrPortalAppointmentClosed},
		{"inspection", func(a *clients.Appointment) { a.Type = "Vistoria" }, ErrPortalAppointmentClosed},
		{"stale", func(a *clients.Appointment) { a.UpdatedAt = a.UpdatedAt.Add(time.Second) }, clients.ErrCalendarChanged},
		{"another tenant", func(a *clients.Appointment) { a.TenantID = "other" }, clients.ErrCalendarUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := futureVisit()
			expected := a.UpdatedAt
			tc.change(a)
			svc, _, writes := visitActionFixture(t, a, nil)
			_, err := svc.CancelMyDealAppointment(context.Background(), "client", "", "deal", "visit", portal.CancelAppointmentRequest{ExpectedUpdatedAt: expected})
			if !errors.Is(err, tc.expected) || *writes != 0 {
				t.Fatalf("err=%v writes=%d", err, *writes)
			}
		})
	}
}

func TestPortalVisitRescheduleRejectsInvalidSlotsAndAgendaFailure(t *testing.T) {
	for _, tc := range []struct{ date, start, end string }{{"2000-01-01", "14:00", "15:00"}, {"2099-02-30", "14:00", "15:00"}, {"2099-10-21", "17:00", "16:00"}, {"2099-10-21", "14:00", "14:00"}} {
		a := futureVisit()
		svc, _, writes := visitActionFixture(t, a, nil)
		_, err := svc.UpdateMyDealAppointment(context.Background(), "client", "", "deal", "visit", portal.UpdateAppointmentRequest{ExpectedUpdatedAt: a.UpdatedAt, Date: tc.date, Time: tc.start, EndTime: tc.end})
		if !errors.Is(err, clients.ErrCalendarInvalid) || *writes != 0 {
			t.Fatalf("invalid slot written: err=%v", err)
		}
	}
	a := futureVisit()
	svc, _, _ := visitActionFixture(t, a, errors.New("offline"))
	_, err := svc.CancelMyDealAppointment(context.Background(), "client", "", "deal", "visit", portal.CancelAppointmentRequest{ExpectedUpdatedAt: a.UpdatedAt})
	if !errors.Is(err, clients.ErrCalendarUnavailable) || a.Status != "Confirmado" {
		t.Fatalf("false cancellation: %v", err)
	}
}

func TestPortalVisitRescheduleStatusFollowsAgendaConfirmation(t *testing.T) {
	for _, tc := range []struct{ status, start, expected string }{
		{"Pendente", "16:00", "Pendente"}, {"Agendado", "16:00", "Pendente"}, {"Confirmado", "14:00", "Confirmado"}, {"Reagendado", "16:00", "Reagendado"},
	} {
		t.Run(tc.status+tc.start, func(t *testing.T) {
			a := futureVisit()
			a.Status = tc.status
			svc, _, _ := visitActionFixture(t, a, nil)
			end := "17:00"
			if tc.start == "14:00" {
				end = "15:00"
			}
			updated, err := svc.UpdateMyDealAppointment(context.Background(), "client", "", "deal", "visit", portal.UpdateAppointmentRequest{ExpectedUpdatedAt: a.UpdatedAt, Date: a.Date, Time: tc.start, EndTime: end})
			if err != nil || updated.Status != tc.expected {
				t.Fatalf("incorrect confirmation status: updated=%+v err=%v", updated, err)
			}
		})
	}
}
