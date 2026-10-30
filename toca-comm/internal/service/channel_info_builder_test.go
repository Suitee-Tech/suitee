package service

import (
	"errors"
	"testing"
)

func TestBuildChannelInfo_CompletePayload(t *testing.T) {
	p := DealPayload{
		DealID:     "deal-123",
		PropertyID: "prop-456",
		Title:      "#789",
		DealType:   "SALE",
		DealValue:  350000.00,
		BrokerID:   "broker-1",
		BrokerName: "Carlos Correia",
		Parties: []PartyPayload{
			{Name: "João Silva", Role: "buyer", Type: "cliente", LeadID: "lead-001"},
			{Name: "Maria Souza", Role: "owner", Type: "proprietario", LeadID: ""},
		},
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}
	if info == nil {
		t.Fatal("expected non-nil ChannelInfo")
	}

	if len(info.Sections) != 3 {
		t.Fatalf("expected 3 sections, got %d", len(info.Sections))
	}

	// Section 0: Negócio
	neg := info.Sections[0]
	if neg.Label != "Negócio" {
		t.Errorf("section 0 label: expected 'Negócio', got %q", neg.Label)
	}
	if len(neg.Items) != 3 {
		t.Errorf("section 0: expected 3 items (ID, Tipo, Valor), got %d", len(neg.Items))
	}
	if neg.Items[0].Label != "ID" || neg.Items[0].Value != "#789" {
		t.Errorf("deal ID item: %s=%s", neg.Items[0].Label, neg.Items[0].Value)
	}
	if neg.Items[0].Link != "/esteira/deals/deal-123" {
		t.Errorf("deal link: expected /esteira/deals/deal-123, got %q", neg.Items[0].Link)
	}
	if neg.Items[1].Label != "Tipo" || neg.Items[1].Value != "Venda" {
		t.Errorf("deal type item: %s=%s (expected Venda for SALE)", neg.Items[1].Label, neg.Items[1].Value)
	}
	// Valor do negócio no padrão brasileiro: o painel de contexto do chat mostra
	// o mesmo texto do e-mail e do painel ("R$ 350.000,00").
	if neg.Items[2].Label != "Valor" || neg.Items[2].Value != "R$ 350.000,00" {
		t.Errorf("deal value item: %s=%s", neg.Items[2].Label, neg.Items[2].Value)
	}

	// Section 1: Imóvel
	prop := info.Sections[1]
	if prop.Label != "Imóvel" {
		t.Errorf("section 1 label: expected 'Imóvel', got %q", prop.Label)
	}
	if prop.Items[0].Label != "Imóvel" || prop.Items[0].Value != "prop-456" {
		t.Errorf("property item: %s=%s", prop.Items[0].Label, prop.Items[0].Value)
	}
	if prop.Items[0].Link != "/imoveis/prop-456" {
		t.Errorf("property link: expected /imoveis/prop-456, got %q", prop.Items[0].Link)
	}

	// Section 2: Partes
	part := info.Sections[2]
	if part.Label != "Partes" {
		t.Errorf("section 2 label: expected 'Partes', got %q", part.Label)
	}
	if len(part.Items) != 2 {
		t.Fatalf("expected 2 party items, got %d", len(part.Items))
	}
	if part.Items[0].Label != "João Silva" || part.Items[0].Value != "Comprador · Cliente" {
		t.Errorf("party[0]: %s=%s (expected Comprador · Cliente)", part.Items[0].Label, part.Items[0].Value)
	}
	if part.Items[0].Link != "/leads/lead-001" {
		t.Errorf("party[0] link: expected /leads/lead-001, got %q", part.Items[0].Link)
	}
	if part.Items[1].Label != "Maria Souza" || part.Items[1].Value != "Proprietário · Proprietário" {
		t.Errorf("party[1]: %s=%s (expected Proprietário · Proprietário)", part.Items[1].Label, part.Items[1].Value)
	}
	if part.Items[1].Link != "" {
		t.Errorf("party[1] link: expected empty (no LeadID), got %q", part.Items[1].Link)
	}
}

func TestBuildChannelInfo_MissingOptionalFields(t *testing.T) {
	p := DealPayload{
		DealID:     "deal-min",
		PropertyID: "prop-min",
		Title:      "",
		Parties:    nil,
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}

	neg := info.Sections[0]
	if len(neg.Items) != 1 {
		t.Errorf("expected 1 deal item (ID only), got %d", len(neg.Items))
	}
	if neg.Items[0].Link != "/esteira/deals/deal-min" {
		t.Errorf("expected link /esteira/deals/deal-min, got %q", neg.Items[0].Link)
	}

	// Partes section should be omitted when no parties
	for _, s := range info.Sections {
		if s.Label == "Partes" {
			t.Error("expected no Partes section when parties list is nil")
		}
	}
}

func TestBuildChannelInfo_MissingPropertyID(t *testing.T) {
	p := DealPayload{
		DealID: "deal-no-prop",
	}

	info, err := BuildChannelInfo(p)
	if err == nil {
		t.Fatal("expected error for missing PropertyID")
	}
	if !errors.Is(err, ErrMissingPropertyID) {
		t.Errorf("expected ErrMissingPropertyID, got %v", err)
	}
	if info != nil {
		t.Error("expected nil ChannelInfo on error")
	}
}

func TestBuildChannelInfo_PartiesWithRoleOnly(t *testing.T) {
	p := DealPayload{
		DealID:     "d1",
		PropertyID: "p1",
		Parties: []PartyPayload{
			{Name: "Ana", Role: "guarantor", Type: ""},
		},
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}

	for _, s := range info.Sections {
		if s.Label == "Partes" {
			if s.Items[0].Value != "Fiador" {
				t.Errorf("expected party value='Fiador', got %q", s.Items[0].Value)
			}
		}
	}
}

func TestBuildChannelInfo_PartiesWithTypeOnly(t *testing.T) {
	p := DealPayload{
		DealID:     "d2",
		PropertyID: "p2",
		Parties: []PartyPayload{
			{Name: "Imobiliária X", Role: "", Type: "parceiro"},
		},
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}

	for _, s := range info.Sections {
		if s.Label == "Partes" {
			if s.Items[0].Value != "Parceiro" {
				t.Errorf("expected party value='Parceiro', got %q", s.Items[0].Value)
			}
		}
	}
}

func TestBuildChannelInfo_EmptyPartyNameFiltered(t *testing.T) {
	p := DealPayload{
		DealID:     "d3",
		PropertyID: "p3",
		Parties: []PartyPayload{
			{Name: "", Role: "seller"},
			{Name: "Pedro", Role: "buyer"},
			{Name: "", Role: "guarantor"},
		},
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}

	for _, s := range info.Sections {
		if s.Label == "Partes" {
			if len(s.Items) != 1 {
				t.Errorf("expected 1 party after filtering empties, got %d", len(s.Items))
			}
			if s.Items[0].Label != "Pedro" {
				t.Errorf("expected party 'Pedro', got %q", s.Items[0].Label)
			}
		}
	}
}

func TestBuildChannelInfo_ZeroValueOmitted(t *testing.T) {
	p := DealPayload{
		DealID:     "d4",
		PropertyID: "p4",
		DealValue:  0,
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}

	neg := info.Sections[0]
	for _, item := range neg.Items {
		if item.Label == "Valor" {
			t.Error("expected no Valor item when DealValue is 0")
		}
	}
}

func TestBuildChannelInfo_SectionsNotEmpty(t *testing.T) {
	p := DealPayload{
		DealID:     "d5",
		PropertyID: "p5",
		DealType:   "RENT",
		Parties:    nil,
	}

	info, err := BuildChannelInfo(p)
	if err != nil {
		t.Fatalf("BuildChannelInfo: %v", err)
	}

	// Check RENT is translated to Locação
	neg := info.Sections[0]
	for _, item := range neg.Items {
		if item.Label == "Tipo" && item.Value != "Locação" {
			t.Errorf("expected RENT translated to Locação, got %q", item.Value)
		}
	}

	for i, s := range info.Sections {
		if len(s.Items) == 0 {
			t.Errorf("section %d (%q) has no items", i, s.Label)
		}
	}
}

func TestBuildChannelInfo_TranslateDealType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"SALE", "Venda"},
		{"RENT", "Locação"},
		{"UNKNOWN", "UNKNOWN"},
	}

	for _, tt := range tests {
		got := translateDealType(tt.input)
		if got != tt.expected {
			t.Errorf("translateDealType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildChannelInfo_TranslatePartyRole(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"buyer", "Comprador"},
		{"seller", "Vendedor"},
		{"owner", "Proprietário"},
		{"tenant", "Locatário"},
		{"landlord", "Locador"},
		{"guarantor", "Fiador"},
		{"witness", "Testemunha"},
		{"broker", "Corretor"},
		{"unknown_role", "unknown_role"},
	}

	for _, tt := range tests {
		got := translatePartyRole(tt.input)
		if got != tt.expected {
			t.Errorf("translatePartyRole(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildChannelInfo_TranslatePartyType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"cliente", "Cliente"},
		{"proprietario", "Proprietário"},
		{"locador", "Locador"},
		{"locatario", "Locatário"},
		{"fiador", "Fiador"},
		{"parceiro", "Parceiro"},
		{"corretor", "Corretor"},
		{"desconhecido", "desconhecido"},
	}

	for _, tt := range tests {
		got := translatePartyType(tt.input)
		if got != tt.expected {
			t.Errorf("translatePartyType(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

var _ = BuildChannelInfo
