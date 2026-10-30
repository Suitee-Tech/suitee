package propertymatch

import (
	"math"
	"testing"
)

func TestScoreRedistributesWeightsForPartialPreferences(t *testing.T) {
	p := Preferences{PriceRange: PriceRange{Min: 300_000, Max: 500_000}, PreferredLocation: "Pinheiros"}
	b := Score(p, Property{ListingType: "sale", SalePrice: 400_000, Neighborhood: "Pinheiros"}, DefaultWeights)
	if math.Abs(b.Total()-100) > 0.01 {
		t.Fatalf("expected complete score after redistribution, got %.2f", b.Total())
	}
	if b.UsedWeights.Area != 0 || b.UsedWeights.Bedrooms != 0 {
		t.Fatalf("inactive dimensions should have zero weight: %+v", b.UsedWeights)
	}
}

func TestBusinessTypeIsDominantAndUsesOperationPrice(t *testing.T) {
	p := Preferences{
		BusinessType:      "rent",
		PreferredLocation: "Pinheiros",
		PriceRange:        PriceRange{Min: 300_000, Max: 500_000},
		Bedrooms:          intPointer(3),
		AreaMin:           floatPointer(80),
		AreaMax:           floatPointer(150),
	}
	candidate := Property{
		ListingType:  "sale",
		AvailableFor: []string{"sale", "rent"},
		SalePrice:    900_000,
		RentPrice:    400_000,
		Neighborhood: "Pinheiros",
		AreaTotal:    120,
		Bedrooms:     3,
	}
	b := Score(p, candidate, DefaultWeights)
	if b.Business != 50 || b.UsedWeights.Business != 50 {
		t.Fatalf("business modality should carry the dominant weight: %+v", b)
	}
	if b.Total() != 100 {
		t.Fatalf("rent price should be used when the primary listing type is sale, got %.2f", b.Total())
	}
}

func intPointer(value int) *int { return &value }

func floatPointer(value float64) *float64 { return &value }

func TestBusinessTypeMismatchIsNotCompatible(t *testing.T) {
	candidate := Property{ListingType: "sale", AvailableFor: []string{"sale"}}
	if BusinessTypeMatches("rent", candidate) {
		t.Fatal("sale-only property must not match a rental lead")
	}
	if got := BusinessTypeScore("rent", candidate); got != 0 {
		t.Fatalf("business mismatch should score zero, got %.2f", got)
	}
}

func TestBusinessTypeFallsBackToLegacyListingType(t *testing.T) {
	if !BusinessTypeMatches("rent", Property{ListingType: "RENT"}) {
		t.Fatal("legacy property without available_for should use listing_type")
	}
}

func TestDecodePreferencesSupportsLegacyKeys(t *testing.T) {
	p, err := DecodePreferences([]byte(`{"Type":"APARTMENT","BusinessType":"rent","PreferredLocation":"Pinheiros","PriceRange":{"Min":300000,"Max":500000},"Features":{"Bedrooms":3,"AreaMin":80}}`))
	if err != nil {
		t.Fatal(err)
	}
	if p.Type != "APARTMENT" || p.BusinessType != "rent" || p.PreferredLocation != "Pinheiros" || p.PriceRange.Max != 500000 || p.Bedrooms == nil || *p.Bedrooms != 3 {
		t.Fatalf("legacy preferences were not decoded: %+v", p)
	}
}

func TestSortScoresUsesPriceAsTieBreaker(t *testing.T) {
	got := SortScores([]float64{80, 80, 70}, []float64{500, 300, 100})
	want := []int{1, 0, 2}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order: got %v want %v", got, want)
		}
	}
}
