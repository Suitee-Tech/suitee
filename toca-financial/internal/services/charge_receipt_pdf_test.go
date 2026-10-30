package services

import (
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

func chargeReceiptRentalFixture(t *testing.T) *charge.Charge {
	t.Helper()
	items := []charge.LineItem{
		{Description: "Aluguel", AmountCents: 288000, SourceType: "rental_base", SourceID: "cfg", EconomicOwnerID: "owner", SettlementMode: charge.SettlementModeCollection, RuleVersionID: "rule"},
		{Description: "Taxa de administração", AmountCents: 32000, SourceType: "rental_bundled_item", SourceID: "admin_fee", EconomicOwnerID: "agency", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue, RuleVersionID: "rule"},
		{Description: "Condomínio", AmountCents: 65000},
		{Description: "IPTU", AmountCents: 18000},
		{Description: "Seguro Fiança", AmountCents: 9800},
	}
	c, err := charge.NewWithLineItems("contract", "payer", "owner", "property", "agency", charge.TypeRent, "Aluguel + Encargos", "2026-10", 4128, time.Now(), items)
	if err != nil {
		t.Fatal(err)
	}
	c.SourceEventID = "rental.cfg.month.2026-10.r1"
	return c
}

func TestChargeReceiptCompositionIncludedAdministration(t *testing.T) {
	c := chargeReceiptRentalFixture(t)
	items, split := chargeReceiptComposition(c)
	if split == nil || split.GrossRentCents != 320000 || split.AdministrationCents != 32000 || split.OwnerRentCents != 288000 {
		t.Fatalf("unexpected rental split: %+v", split)
	}
	if len(items) != 4 || items[0].Description != "Aluguel" || items[0].AmountCents != 320000 {
		t.Fatalf("unexpected billing composition: %+v", items)
	}
	var total int64
	for _, item := range items {
		total += item.AmountCents
	}
	if total != c.Amount.Cents() || c.LineItems[0].AmountCents != 288000 {
		t.Fatalf("changed charge total or persisted items: total=%d base=%d", total, c.LineItems[0].AmountCents)
	}
}

func TestChargeReceiptCompositionDoesNotAssumeAdditionalAdministrationIncluded(t *testing.T) {
	c := chargeReceiptRentalFixture(t)
	c.LineItems[1].AllocationType = ""
	c.LineItems[0].AmountCents = 320000
	items, split := chargeReceiptComposition(c)
	if split != nil || len(items) != 5 || items[0].AmountCents != 320000 || items[1].AmountCents != 32000 {
		t.Fatalf("explicit additional admin changed: items=%+v split=%+v", items, split)
	}
}

func TestChargeReceiptCompositionRejectsUntrustedSplit(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*charge.Charge)
	}{
		{"manual source", func(c *charge.Charge) { c.SourceEventID = "manual" }},
		{"sale", func(c *charge.Charge) { c.Type = charge.TypeSale }},
		{"different rule", func(c *charge.Charge) { c.LineItems[1].RuleVersionID = "other" }},
		{"different owner", func(c *charge.Charge) { c.LineItems[1].EconomicOwnerID = "third-party" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := chargeReceiptRentalFixture(t)
			tc.edit(c)
			items, split := chargeReceiptComposition(c)
			if split != nil || len(items) != len(c.LineItems) {
				t.Fatalf("untrusted split recomposed: %+v", split)
			}
		})
	}
}

func TestRenderChargeReceiptPDFWithComposition(t *testing.T) {
	c := chargeReceiptRentalFixture(t)
	out, err := RenderChargeReceiptPDF(ChargeReceiptContext{Charge: c, OrgName: "Agency"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 1500 || !strings.HasPrefix(string(out), "%PDF-") {
		t.Fatalf("invalid PDF output: %d bytes", len(out))
	}
	if _, err := RenderChargeReceiptPDF(ChargeReceiptContext{}); err == nil {
		t.Fatal("nil charge should be rejected")
	}
}
