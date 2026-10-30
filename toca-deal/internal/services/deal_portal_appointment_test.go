package services

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/domain/portal"
)

func TestPortalDealVisitRejectsPastAndInvalidDatesBeforeCalendar(t *testing.T) {
	d := &deal.Deal{ID: "deal", TenantID: "11111111-1111-4111-8111-111111111111", PropertyID: "property"}
	svc := &DealService{repo: portalVisitDeals{newInMemoryDealRepo(d), d}}
	for _, date := range []string{"2000-01-01", "2099-02-30", "invalid"} {
		t.Run(date, func(t *testing.T) {
			_, err := svc.CreateMyDealAppointment(context.Background(), "client", "client@example.test", "Cliente", "deal", portal.CreateAppointmentRequest{Date: date, Time: "10:00", EndTime: "11:00"})
			if !errors.Is(err, clients.ErrCalendarInvalid) {
				t.Fatalf("expected invalid slot, got %v", err)
			}
		})
	}
}

type portalVisitDeals struct {
	*inMemoryDealRepo
	selected *deal.Deal
}

func (r portalVisitDeals) FindBySignerIdentity(context.Context, string, string) ([]*deal.Deal, error) {
	return []*deal.Deal{r.selected}, nil
}

type portalVisitProperties struct {
	PropertyRepository
	tenant, id string
}

func (r *portalVisitProperties) GetSummaryByID(_ context.Context, tenant, id string, _ deal.Type) (*portal.PropertySummary, error) {
	r.tenant, r.id = tenant, id
	return &portal.PropertySummary{ID: id, Title: "Imóvel escolhido"}, nil
}

type portalVisitTransport func(*http.Request) (*http.Response, error)

func (f portalVisitTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPortalDealVisitUsesChosenPropertyAndPendingStatus(t *testing.T) {
	d := &deal.Deal{ID: "deal", TenantID: "11111111-1111-4111-8111-111111111111", PropertyID: "original", BrokerID: "broker", CustomerID: "lead"}
	properties := &portalVisitProperties{}
	base := clients.NewBaseClient("http://calendar.test")
	base.InternalAPIKey = "test-key"
	var payload clients.CreateAppointmentRequest
	base.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		return &http.Response{StatusCode: 201, Body: io.NopCloser(strings.NewReader(`{"data":{"id":"visit"}}`)), Header: http.Header{}}, nil
	})
	svc := &DealService{repo: portalVisitDeals{newInMemoryDealRepo(d), d}, propertyRepo: properties, calendarClient: &clients.CalendarClient{BaseClient: base}}
	_, err := svc.CreateMyDealAppointment(context.Background(), "client", "client@example.test", "Lucas", "deal", portal.CreateAppointmentRequest{PropertyID: "selected", Date: "2099-10-20", Time: "14:00", EndTime: "15:30", Description: "Garagem"})
	if err != nil {
		t.Fatal(err)
	}
	if payload.PropertyID != "selected" || properties.tenant != d.TenantID || properties.id != "selected" || payload.Status != "Pendente" || payload.EndTime != "15:30" || payload.Description != "Garagem" || payload.CustomerAccountID != "client" || payload.DealID != d.ID {
		t.Fatalf("incorrect portal appointment: %#v", payload)
	}
}

func TestPortalDealAppointmentsScopeUsesAuthenticatedClientLead(t *testing.T) {
	d := &deal.Deal{ID: "deal", TenantID: "agency", CustomerID: "buyer-lead", LeadID: "buyer-lead", Parties: []deal.Party{
		{AccountID: "client", LeadID: "buyer-lead", Role: deal.PartyRoleBuyer},
		{AccountID: "owner", LeadID: "owner-lead", Role: deal.PartyRoleOwner},
	}}
	base := clients.NewBaseClient("http://calendar.test")
	base.InternalAPIKey = "test-key"
	calls := 0
	base.HTTPClient.Transport = portalVisitTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Path != "/api/v1/calendar/internal/deals/deal/appointments" || r.URL.Query().Get("tenant_id") != "agency" || strings.Join(r.URL.Query()["lead_id"], ",") != "buyer-lead" {
			t.Fatalf("unscoped appointment query: %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"pre-deal-visit","tenant_id":"agency","lead_id":"buyer-lead"}]}`)), Header: http.Header{}}, nil
	})
	svc := &DealService{repo: portalVisitDeals{newInMemoryDealRepo(d), d}, calendarClient: &clients.CalendarClient{BaseClient: base}}
	appointments, err := svc.GetMyDealAppointments(context.Background(), "client", "client@example.test", "deal")
	if err != nil || len(appointments) != 1 || appointments[0].ID != "pre-deal-visit" {
		t.Fatalf("appointments=%+v err=%v", appointments, err)
	}
	_, err = svc.GetMyDealAppointments(context.Background(), "client", "client@example.test", "other-deal")
	if err == nil || calls != 1 {
		t.Fatal("agenda called without deal access")
	}
}

func TestPortalAppointmentLeadScopeDoesNotExposeOtherParticipants(t *testing.T) {
	d := &deal.Deal{LeadID: "buyer-lead", CustomerID: "buyer-lead", Signers: []deal.Signer{
		{AccountID: "client", Email: "client@example.test", Role: deal.SignatoryRoleComprador},
		{AccountID: "owner", Email: "owner@example.test", Role: deal.SignatoryRoleVendedor},
	}}
	if ids := portalAppointmentLeadIDs(d, "client", ""); len(ids) != 1 || ids[0] != "buyer-lead" {
		t.Fatalf("legacy buyer scope: %v", ids)
	}
	if ids := portalAppointmentLeadIDs(d, "owner", "owner@example.test"); len(ids) != 0 {
		t.Fatalf("owner saw buyer's lead visits: %v", ids)
	}
	d.Parties = []deal.Party{{Email: "client@example.test", LeadID: "specific-lead"}, {AccountID: "owner", LeadID: "owner-lead"}}
	if ids := portalAppointmentLeadIDs(d, "", "CLIENT@example.test"); len(ids) != 1 || ids[0] != "specific-lead" {
		t.Fatalf("email scope: %v", ids)
	}
}
