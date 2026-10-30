package services

import (
	"testing"
	"time"

	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// TestBuildContractSignedEventFor_RequiresDeal cobre o incidente do contrato
// de locação assinado sem Rental/comissão: quando o negócio não é carregado o
// payload sai sem type/value/parties e TODO consumidor rejeita (financeiro:
// "invalid contract value"; ops: "missing contract/tenant/property"). O
// helper precisa falhar em vez de emitir o evento corrompido.
func TestBuildContractSignedEventFor_RequiresDeal(t *testing.T) {
	c := &contract.Contract{
		ID:        "contract-1",
		DealID:    "deal-1",
		TenantID:  "tenant-1",
		StartDate: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	}
	now := time.Date(2026, 10, 2, 13, 9, 0, 0, time.UTC)

	if _, err := buildContractSignedEventFor(c, nil, now); err == nil {
		t.Fatal("negócio ausente deve devolver erro (nunca emitir payload vazio)")
	}
}

func TestBuildContractSignedEventFor_CarriesDealData(t *testing.T) {
	c := &contract.Contract{
		ID:        "contract-1",
		DealID:    "deal-1",
		TenantID:  "tenant-1",
		StartDate: time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC),
	}
	d := &deal.Deal{
		ID:         "deal-1",
		TenantID:   "tenant-1",
		Type:       deal.TypeRent,
		Value:      2800,
		CustomerID: "lead-tenant",
		OwnerID:    "lead-owner",
		PropertyID: "property-1",
		Commission: 0,
	}
	d.Parties = []deal.Party{
		{ID: "party-1", LeadID: "lead-tenant", AccountID: "acc-1", Email: "inquilino@example.com", Role: deal.PartyRoleTenant, IsPrimary: true},
		{ID: "party-2", LeadID: "lead-owner", AccountID: "acc-2", Email: "proprietario@example.com", Role: deal.PartyRoleOwner, IsPrimary: true},
	}
	d.BrokerParticipations = []deal.BrokerParticipation{
		{BrokerID: "broker-1", Role: deal.BrokerRoleCaptador, Percentage: 100},
	}

	evt, err := buildContractSignedEventFor(c, d, time.Date(2026, 10, 2, 13, 9, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildContractSignedEventFor: %v", err)
	}
	if evt.Data.Type != "RENT" {
		t.Errorf("type = %q, want RENT", evt.Data.Type)
	}
	if evt.Data.Value != 2800 {
		t.Errorf("value = %v, want 2800", evt.Data.Value)
	}
	if evt.Data.OwnerID != "lead-owner" || evt.Data.CustomerID != "lead-tenant" || evt.Data.PropertyID != "property-1" {
		t.Errorf("partes do negócio ausentes no payload: owner=%q customer=%q property=%q",
			evt.Data.OwnerID, evt.Data.CustomerID, evt.Data.PropertyID)
	}
	if len(evt.Data.Parties) != 2 {
		t.Errorf("parties = %d, want 2 (o financeiro exige tenant/owner)", len(evt.Data.Parties))
	}
	if len(evt.Data.BrokerParticipations) != 1 || evt.Data.BrokerParticipations[0].Percentage != 100 {
		t.Errorf("broker_participations = %+v", evt.Data.BrokerParticipations)
	}
}
