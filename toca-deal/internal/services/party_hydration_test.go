package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// fakeLeadHydrator implements PartyLeadHydrator for unit-testing
// DealService.hydratePartiesFromLeads.
type fakeLeadHydrator struct {
	byID    map[string]*clients.LeadDTO
	err     error
	calls   []string
	failFor map[string]bool
}

func (f *fakeLeadHydrator) GetLead(ctx context.Context, tenantID, leadID string) (*clients.LeadDTO, error) {
	f.calls = append(f.calls, leadID)
	if f.failFor != nil && f.failFor[leadID] {
		return nil, errors.New("crm timeout")
	}
	if f.err != nil {
		return nil, f.err
	}
	if lead, ok := f.byID[leadID]; ok {
		return lead, nil
	}
	return nil, nil
}

func newServiceWithHydrator(h PartyLeadHydrator) *DealService {
	return (&DealService{}).WithLeadHydrator(h)
}

func TestHydratePartiesFromLeads_FillsBareLeadIDs(t *testing.T) {
	hydrator := &fakeLeadHydrator{
		byID: map[string]*clients.LeadDTO{
			"lead-owner": {
				ID:            "lead-owner",
				Name:          "Maria Silva",
				Email:         "maria@example.com",
				Phone:         "11999999999",
				Documentation: "123.456.789-00",
				AccountID:     "acc-1",
			},
		},
	}
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{ID: "p1", LeadID: "lead-owner", Role: deal.PartyRoleOwner, IsPrimary: true},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	got := d.Parties[0]
	if got.Name != "Maria Silva" {
		t.Errorf("Name = %q, want %q", got.Name, "Maria Silva")
	}
	if got.Email != "maria@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "maria@example.com")
	}
	if got.Phone != "11999999999" {
		t.Errorf("Phone = %q, want %q", got.Phone, "11999999999")
	}
	if got.Document != "123.456.789-00" {
		t.Errorf("Document = %q, want %q", got.Document, "123.456.789-00")
	}
	if got.AccountID != "acc-1" {
		t.Errorf("AccountID = %q, want %q", got.AccountID, "acc-1")
	}
}

func TestHydratePartiesFromLeads_PreservesExistingFields(t *testing.T) {
	hydrator := &fakeLeadHydrator{
		byID: map[string]*clients.LeadDTO{
			"lead-1": {Name: "FromCRM", Email: "crm@example.com", Phone: "9999"},
		},
	}
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{
				ID:     "p1",
				LeadID: "lead-1",
				Role:   deal.PartyRoleOwner,
				Name:   "Caller Set",
				Email:  "caller@example.com",
			},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	got := d.Parties[0]
	if got.Name != "Caller Set" {
		t.Errorf("Name was overwritten: %q", got.Name)
	}
	if got.Email != "caller@example.com" {
		t.Errorf("Email was overwritten: %q", got.Email)
	}
	// Phone was missing — should be filled from CRM.
	if got.Phone != "9999" {
		t.Errorf("Phone should be filled, got %q", got.Phone)
	}
}

func TestHydratePartiesFromLeads_SkipsPartiesWithoutLeadID(t *testing.T) {
	hydrator := &fakeLeadHydrator{}
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{ID: "p1", Role: deal.PartyRoleOwner, Name: "Manual"},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	if len(hydrator.calls) != 0 {
		t.Errorf("GetLead should not be called when LeadID is empty, got %v", hydrator.calls)
	}
}

func TestHydratePartiesFromLeads_SkipsFullyHydratedParties(t *testing.T) {
	hydrator := &fakeLeadHydrator{}
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{
				ID:        "p1",
				LeadID:    "lead-1",
				Name:      "X",
				Email:     "x@x.com",
				Phone:     "1",
				Document:  "doc",
				AccountID: "acc",
			},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	if len(hydrator.calls) != 0 {
		t.Errorf("GetLead should not be called when all fields are set, got %v", hydrator.calls)
	}
}

func TestHydratePartiesFromLeads_CRMErrorDoesNotPanic(t *testing.T) {
	hydrator := &fakeLeadHydrator{err: errors.New("crm down")}
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{ID: "p1", LeadID: "lead-1", Role: deal.PartyRoleOwner},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	// Party stays bare; deal creation must still proceed.
	if d.Parties[0].Name != "" {
		t.Errorf("Name should remain empty on CRM error, got %q", d.Parties[0].Name)
	}
}

func TestHydratePartiesFromLeads_HandlesMissingLead(t *testing.T) {
	hydrator := &fakeLeadHydrator{byID: map[string]*clients.LeadDTO{}} // no matches
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{ID: "p1", LeadID: "lead-missing", Role: deal.PartyRoleOwner},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	if d.Parties[0].Name != "" {
		t.Errorf("Name should remain empty when lead is not found, got %q", d.Parties[0].Name)
	}
	if len(hydrator.calls) != 1 {
		t.Errorf("expected 1 GetLead call, got %d", len(hydrator.calls))
	}
}

func TestHydratePartiesFromLeads_MultiplePartiesPartialFill(t *testing.T) {
	hydrator := &fakeLeadHydrator{
		byID: map[string]*clients.LeadDTO{
			"lead-owner":    {Name: "Owner X", Email: "owner@x.com"},
			"lead-customer": {Name: "Tenant Y", Email: "tenant@y.com", Phone: "888"},
		},
	}
	svc := newServiceWithHydrator(hydrator)

	d := &deal.Deal{
		TenantID: "t1",
		Parties: []deal.Party{
			{ID: "p1", LeadID: "lead-owner", Role: deal.PartyRoleOwner, IsPrimary: true},
			{ID: "p2", LeadID: "lead-customer", Role: deal.PartyRoleTenant, IsPrimary: true},
		},
	}
	svc.hydratePartiesFromLeads(context.Background(), d)

	if d.Parties[0].Name != "Owner X" || d.Parties[0].Email != "owner@x.com" {
		t.Errorf("owner not hydrated: %+v", d.Parties[0])
	}
	if d.Parties[1].Name != "Tenant Y" || d.Parties[1].Phone != "888" {
		t.Errorf("tenant not hydrated: %+v", d.Parties[1])
	}
}

func TestHydratePartiesFromLeads_NilServiceOrHydratorIsSafe(t *testing.T) {
	var nilSvc *DealService
	nilSvc.hydratePartiesFromLeads(context.Background(), &deal.Deal{}) // must not panic

	svc := newServiceWithHydrator(nil)
	d := &deal.Deal{Parties: []deal.Party{{LeadID: "x"}}}
	svc.hydratePartiesFromLeads(context.Background(), d) // must not panic
	if d.Parties[0].Name != "" {
		t.Error("nil hydrator should leave parties untouched")
	}
}
