package services_test

import (
	"context"
	"testing"

	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

// Production scenario the user reported: property has an owner registered as
// a CRM lead, but when a deal is created the owner Party comes empty because
// the CRM hydration silently no-ops (missing INTERNAL_API_KEY, network, etc.).
// Inline owner hints from the request bypass that fragility — caller already
// has the lead profile loaded, no service-to-service call needed.
func TestCreate_PopulatesOwnerPartyFromHints(t *testing.T) {
	propRepo := &stubPropertyRepo{tenantID: "tenant-A", propertyID: "prop-A"}
	leadRepo := &stubLeadRepo{existing: map[string]map[string]bool{
		"tenant-A": {"owner-lead": true, "buyer-lead": true},
	}}
	svc := service.NewDealService(&mockDealRepo{}, propRepo, leadRepo, &mockEventBus{}, nil)
	// Disable the CRM hydrator entirely to prove the fix works without it.
	svc.WithLeadHydrator(nil)

	d, err := svc.Create(
		context.Background(),
		deal.TypeRent,
		"prop-A", "owner-lead", "buyer-lead", "broker-1", "",
		3000, "tenant-A",
		"Larissa Oliveira", "larissa@example.com", "acc-larissa", "locatario",
		service.PartyProfileHints{
			Name:      "Primouse Everdim",
			Email:     "primose.e@email.com",
			Phone:     "11949038328",
			AccountID: "acc-primouse",
		},
		false,
	)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	var ownerParty *deal.Party
	for i := range d.Parties {
		if d.Parties[i].Role == deal.PartyRoleOwner {
			ownerParty = &d.Parties[i]
			break
		}
	}
	if ownerParty == nil {
		t.Fatal("owner party should be present")
	}
	if ownerParty.Name != "Primouse Everdim" {
		t.Errorf("owner Name = %q, want %q", ownerParty.Name, "Primouse Everdim")
	}
	if ownerParty.Email != "primose.e@email.com" {
		t.Errorf("owner Email = %q, want %q", ownerParty.Email, "primose.e@email.com")
	}
	if ownerParty.Phone != "11949038328" {
		t.Errorf("owner Phone = %q, want %q", ownerParty.Phone, "11949038328")
	}
	if ownerParty.AccountID != "acc-primouse" {
		t.Errorf("owner AccountID = %q, want %q", ownerParty.AccountID, "acc-primouse")
	}
}

func TestCreateWithParties_PopulatesOwnerPartyFromHints(t *testing.T) {
	propRepo := &stubPropertyRepo{tenantID: "tenant-A", propertyID: "prop-A"}
	leadRepo := &stubLeadRepo{existing: map[string]map[string]bool{
		"tenant-A": {"owner-lead": true, "buyer-lead": true},
	}}
	svc := service.NewDealService(&mockDealRepo{}, propRepo, leadRepo, &mockEventBus{}, nil)
	svc.WithLeadHydrator(nil)

	// No owner Party supplied via parties[] — fallback path adds it. Hints
	// must apply there too.
	d, err := svc.CreateWithParties(
		context.Background(),
		deal.TypeRent,
		"prop-A", "owner-lead", "buyer-lead", "broker-1", "",
		3000, "tenant-A",
		nil,
		service.PartyProfileHints{
			Name:  "Primouse Everdim",
			Email: "primose.e@email.com",
		},
		false,
	)
	if err != nil {
		t.Fatalf("CreateWithParties: %v", err)
	}
	var ownerParty *deal.Party
	for i := range d.Parties {
		if d.Parties[i].Role == deal.PartyRoleOwner {
			ownerParty = &d.Parties[i]
			break
		}
	}
	if ownerParty == nil {
		t.Fatal("owner party should be present")
	}
	if ownerParty.Name != "Primouse Everdim" || ownerParty.Email != "primose.e@email.com" {
		t.Errorf("owner hints not applied: %+v", ownerParty)
	}
}

// When CRM hydration succeeds (the existing fallback), it must NOT overwrite
// hints the caller already provided.
func TestCreate_HintsTakePrecedenceOverCRMHydration(t *testing.T) {
	propRepo := &stubPropertyRepo{tenantID: "tenant-A", propertyID: "prop-A"}
	leadRepo := &stubLeadRepo{existing: map[string]map[string]bool{
		"tenant-A": {"owner-lead": true, "buyer-lead": true},
	}}
	svc := service.NewDealService(&mockDealRepo{}, propRepo, leadRepo, &mockEventBus{}, nil)

	d, err := svc.Create(
		context.Background(),
		deal.TypeRent,
		"prop-A", "owner-lead", "buyer-lead", "broker-1", "",
		3000, "tenant-A",
		"", "", "", "",
		service.PartyProfileHints{
			Name:  "Hint Name",
			Email: "hint@example.com",
		},
		false,
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	for i := range d.Parties {
		if d.Parties[i].Role != deal.PartyRoleOwner {
			continue
		}
		if d.Parties[i].Name != "Hint Name" {
			t.Errorf("hint name should be preserved, got %q", d.Parties[i].Name)
		}
	}
}
