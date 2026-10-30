package deal_test

import (
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

func TestDeal_AddSigner(t *testing.T) {
	tests := []struct {
		name    string
		dType   deal.Type
		role    deal.SignatoryRole
		wantErr bool
	}{
		{
			name:    "Sale - Valid Comprador",
			dType:   deal.TypeSale,
			role:    deal.SignatoryRoleComprador,
			wantErr: false,
		},
		{
			name:    "Sale - Valid Vendedor",
			dType:   deal.TypeSale,
			role:    deal.SignatoryRoleVendedor,
			wantErr: false,
		},
		{
			name:    "Sale - Invalid Inquilino",
			dType:   deal.TypeSale,
			role:    deal.SignatoryRoleInquilino,
			wantErr: true,
		},
		{
			name:    "Rent - Valid Inquilino",
			dType:   deal.TypeRent,
			role:    deal.SignatoryRoleInquilino,
			wantErr: false,
		},
		{
			name:    "Rent - Valid Proprietario",
			dType:   deal.TypeRent,
			role:    deal.SignatoryRoleProprietario,
			wantErr: false,
		},
		{
			name:    "Rent - Invalid Comprador",
			dType:   deal.TypeRent,
			role:    deal.SignatoryRoleComprador,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, _ := deal.New(tt.dType, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")
			err := d.AddSigner("John Doe", "john@example.com", tt.role)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddSigner() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDeal_AddParty_Validation(t *testing.T) {
	dSale, _ := deal.New(deal.TypeSale, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")
	if err := dSale.AddParty(deal.Party{Role: deal.PartyRoleTenant, IsPrimary: true}); err == nil {
		t.Fatalf("expected error for tenant party in sale deal")
	}
	if err := dSale.AddParty(deal.Party{Role: deal.PartyRoleBuyer, IsPrimary: true}); err != nil {
		t.Fatalf("expected no error for buyer party in sale deal, got %v", err)
	}

	dRent, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")
	if err := dRent.AddParty(deal.Party{Role: deal.PartyRoleBuyer, IsPrimary: true}); err == nil {
		t.Fatalf("expected error for buyer party in rent deal")
	}
	if err := dRent.AddParty(deal.Party{Role: deal.PartyRoleTenant, IsPrimary: true}); err != nil {
		t.Fatalf("expected no error for tenant party in rent deal, got %v", err)
	}
}

func TestDeal_PartyPrimaryManagement(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")

	if err := d.AddParty(deal.Party{ID: "p1", Role: deal.PartyRoleTenant, Email: "a@a.com", IsPrimary: true}); err != nil {
		t.Fatalf("AddParty error: %v", err)
	}
	if err := d.AddParty(deal.Party{ID: "p2", Role: deal.PartyRoleTenant, Email: "b@b.com"}); err != nil {
		t.Fatalf("AddParty error: %v", err)
	}

	if p := d.PrimaryByRole(deal.PartyRoleTenant); p == nil || p.ID != "p1" {
		t.Fatalf("expected primary tenant p1, got %+v", p)
	}

	if err := d.SetPrimaryParty("p2"); err != nil {
		t.Fatalf("SetPrimaryParty error: %v", err)
	}
	if p := d.PrimaryByRole(deal.PartyRoleTenant); p == nil || p.ID != "p2" {
		t.Fatalf("expected primary tenant p2, got %+v", p)
	}
}

func TestDeal_RemoveParty_OnlyPrimaryFails(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")

	if err := d.AddParty(deal.Party{ID: "p1", Role: deal.PartyRoleTenant, Email: "a@a.com", IsPrimary: true}); err != nil {
		t.Fatalf("AddParty error: %v", err)
	}
	if err := d.RemoveParty("p1"); err == nil {
		t.Fatalf("expected error when removing the only primary party")
	}
}

func TestDeal_AddSigner_CreatesParty(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")

	if err := d.AddSigner("John Doe", "john@example.com", deal.SignatoryRoleLocatario); err != nil {
		t.Fatalf("AddSigner error: %v", err)
	}
	if len(d.Signers) != 1 {
		t.Fatalf("expected 1 signer, got %d", len(d.Signers))
	}
	if len(d.Parties) == 0 {
		t.Fatalf("expected parties to be populated")
	}
	found := false
	for _, p := range d.Parties {
		if p.ID == d.Signers[0].ID {
			found = true
			if p.Role != deal.PartyRoleTenant {
				t.Fatalf("expected party role tenant, got %s", p.Role)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected a party with same id as signer")
	}
}

func TestDeal_AggregatedDueDiligence(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")

	d.Parties = []deal.Party{
		{ID: "p1", Role: deal.PartyRoleTenant, IsPrimary: true, DueDiligence: &deal.DueDiligence{Status: "APPROVED"}},
		{ID: "p2", Role: deal.PartyRoleGuarantor, IsPrimary: true, DueDiligence: &deal.DueDiligence{Status: "PENDING"}},
	}
	if agg := d.AggregatedDueDiligence(); agg == nil || agg.Status != "PENDING" {
		t.Fatalf("expected PENDING, got %+v", agg)
	}

	d.Parties[1].DueDiligence.Status = "APPROVED"
	if agg := d.AggregatedDueDiligence(); agg == nil || agg.Status != "APPROVED" {
		t.Fatalf("expected APPROVED, got %+v", agg)
	}

	d.Parties[1].DueDiligence.Status = "REJECTED"
	if agg := d.AggregatedDueDiligence(); agg == nil || agg.Status != "REJECTED" {
		t.Fatalf("expected REJECTED, got %+v", agg)
	}
}

func TestDeal_UpdatePartyDueDiligence(t *testing.T) {
	d, _ := deal.New(deal.TypeRent, "prop-1", "owner-1", "cust-1", "broker-1", 1000, "tenant-1")
	_ = d.AddParty(deal.Party{ID: "p1", Role: deal.PartyRoleTenant, IsPrimary: true})

	oldStatus, newStatus, err := d.UpdatePartyDueDiligence("p1", nil, "", "", nil, "PENDING")
	if err != nil {
		t.Fatalf("UpdatePartyDueDiligence error: %v", err)
	}
	if oldStatus != "" || newStatus != "PENDING" {
		t.Fatalf("unexpected statuses old=%s new=%s", oldStatus, newStatus)
	}

	oldStatus, newStatus, err = d.UpdatePartyDueDiligence("p1", nil, "", "", nil, "APPROVED")
	if err != nil {
		t.Fatalf("UpdatePartyDueDiligence error: %v", err)
	}
	if oldStatus != "PENDING" || newStatus != "APPROVED" {
		t.Fatalf("unexpected statuses old=%s new=%s", oldStatus, newStatus)
	}
}
