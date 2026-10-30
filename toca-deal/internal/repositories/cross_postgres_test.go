package repositories

import (
	"encoding/json"
	"testing"

	"github.com/oginoino/toca/pkg/propertymatch"
)

func TestRecommendationPreferencesDecodesLeadInterest(t *testing.T) {
	interest := json.RawMessage(`{"preferred_location":"Pinheiros","price_range":{"max":500000},"features":{"bedrooms":2}}`)
	pref, ok := recommendationPreferences(interest)
	if !ok {
		t.Fatal("expected lead interest to be considered matchable")
	}
	if pref.PreferredLocation != "Pinheiros" || pref.PriceRange.Max != 500000 || pref.Bedrooms == nil || *pref.Bedrooms != 2 {
		t.Fatalf("unexpected decoded preferences: %+v", pref)
	}
}

func TestRecommendationPropertyTypeNormalizesLeadType(t *testing.T) {
	if got := recommendationPropertyType(" APARTMENT "); got != "apartment" {
		t.Fatalf("expected apartment, got %q", got)
	}
	if got := recommendationPropertyType("OTHER"); got != "" {
		t.Fatalf("unsupported type should not constrain query, got %q", got)
	}
}

func TestRecommendationUsesSharedMatcherContract(t *testing.T) {
	pref, ok := recommendationPreferences(json.RawMessage(`{"type":"apartment","preferred_location":"Pinheiros","price_range":{"min":300000,"max":500000}}`))
	if !ok {
		t.Fatal("expected preferences")
	}
	breakdown := propertymatch.Score(pref, propertymatch.Property{
		ListingType: "sale", Neighborhood: "Pinheiros", SalePrice: 400000,
	}, propertymatch.DefaultWeights)
	if breakdown.Total() != 100 {
		t.Fatalf("portal should use the shared score, got %.2f", breakdown.Total())
	}
}

func TestRecommendationMatcherIncludesBusinessWeight(t *testing.T) {
	pref, ok := recommendationPreferences(json.RawMessage(`{"preferred_location":"Pinheiros","price_range":{"max":500000}}`))
	if !ok {
		t.Fatal("expected preferences")
	}
	pref.BusinessType = "rent"
	breakdown := propertymatch.Score(pref, propertymatch.Property{
		ListingType: "sale", AvailableFor: []string{"sale", "rent"},
		Neighborhood: "Pinheiros", RentPrice: 400000,
	}, propertymatch.DefaultWeights)
	if breakdown.Business != 50 || breakdown.Total() != 100 {
		t.Fatalf("portal should give operation the dominant score, got %+v", breakdown)
	}
}
