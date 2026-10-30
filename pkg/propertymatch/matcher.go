// Package propertymatch contains the deterministic, explainable ranking rules
// used by both the CRM and the client portal. Keeping the matcher independent
// from either service prevents the two surfaces from silently recommending
// different properties for the same lead.
package propertymatch

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
)

type PriceRange struct {
	Min float64
	Max float64
}

// Preferences is the small, transport-neutral subset of a lead profile used
// for property matching. BusinessType is the operation the lead wants
// (sale/rent); it is separate from Type, which is the physical property type.
type Preferences struct {
	Type              string
	BusinessType      string
	PreferredLocation string
	PriceRange        PriceRange
	Bedrooms          *int
	AreaMin           *float64
	AreaMax           *float64
}

type Property struct {
	ID           string
	PropertyType string
	ListingType  string
	AvailableFor []string
	Street       string
	Neighborhood string
	City         string
	State        string
	SalePrice    float64
	RentPrice    float64
	AreaTotal    float64
	Bedrooms     int
}

type Weights struct {
	Business float64 `json:"business"`
	Price    float64 `json:"price"`
	Location float64 `json:"location"`
	Area     float64 `json:"area"`
	Bedrooms float64 `json:"bedrooms"`
}

// Business is intentionally the largest dimension. A property available only
// for sale should not rank as a strong recommendation for a rental lead even
// when its price or location happens to be a good fit.
var DefaultWeights = Weights{Business: 50, Price: 25, Location: 15, Area: 5, Bedrooms: 5}

type Breakdown struct {
	Business    float64 `json:"business"`
	Price       float64 `json:"price"`
	Location    float64 `json:"location"`
	Area        float64 `json:"area"`
	Bedrooms    float64 `json:"bedrooms"`
	UsedWeights Weights `json:"used_weights"`
}

func (b Breakdown) Total() float64 {
	return math.Round((b.Business+b.Price+b.Location+b.Area+b.Bedrooms)*100) / 100
}

func HasAnyPreference(p Preferences) bool {
	return p.PriceRange.Min > 0 || p.PriceRange.Max > 0 ||
		strings.TrimSpace(p.PreferredLocation) != "" ||
		positive(p.AreaMin) || positive(p.AreaMax) ||
		(p.Bedrooms != nil && *p.Bedrooms > 0)
}

func Score(p Preferences, candidate Property, weights Weights) Breakdown {
	hasBusiness := normalizeBusinessType(p.BusinessType) != ""
	hasPrice := p.PriceRange.Min > 0 || p.PriceRange.Max > 0
	hasLocation := strings.TrimSpace(p.PreferredLocation) != ""
	hasArea := positive(p.AreaMin) || positive(p.AreaMax)
	hasBedrooms := p.Bedrooms != nil && *p.Bedrooms > 0

	used := RedistributeWeightsWithBusiness(weights, hasBusiness, hasPrice, hasLocation, hasArea, hasBedrooms)
	b := Breakdown{UsedWeights: used}
	if hasBusiness {
		b.Business = BusinessTypeScore(p.BusinessType, candidate) * used.Business
	}
	if hasPrice {
		b.Price = PriceScore(p.PriceRange, EffectivePriceForOperation(candidate, p.BusinessType)) * used.Price
	}
	if hasLocation {
		b.Location = LocationScore(p.PreferredLocation, candidate) * used.Location
	}
	if hasArea {
		b.Area = AreaScore(p.AreaMin, p.AreaMax, candidate.AreaTotal) * used.Area
	}
	if hasBedrooms {
		b.Bedrooms = BedroomsScore(*p.Bedrooms, candidate.Bedrooms) * used.Bedrooms
	}
	return b
}

func EffectivePrice(p Property) float64 {
	if normalizeBusinessType(p.ListingType) == "rent" && p.RentPrice > 0 {
		return p.RentPrice
	}
	if p.SalePrice > 0 {
		return p.SalePrice
	}
	return p.RentPrice
}

// EffectivePriceForOperation selects the price for the lead's requested
// business type. This matters for a property that is primarily listed for
// sale but is also available for rent.
func EffectivePriceForOperation(p Property, businessType string) float64 {
	switch normalizeBusinessType(businessType) {
	case "rent":
		return p.RentPrice
	case "sale":
		return p.SalePrice
	}
	return EffectivePrice(p)
}

// BusinessTypeMatches returns whether a property can be used for the
// requested operation. An explicit AvailableFor list is authoritative; the
// primary ListingType is only a fallback for legacy properties without it.
func BusinessTypeMatches(requested string, candidate Property) bool {
	wanted := normalizeBusinessType(requested)
	if wanted == "" {
		return true
	}
	if len(candidate.AvailableFor) > 0 {
		for _, available := range candidate.AvailableFor {
			if normalizeBusinessType(available) == wanted {
				return true
			}
		}
		return false
	}
	return normalizeBusinessType(candidate.ListingType) == wanted
}

func BusinessTypeScore(requested string, candidate Property) float64 {
	if normalizeBusinessType(requested) == "" {
		return 0
	}
	if BusinessTypeMatches(requested, candidate) {
		return 1
	}
	return 0
}

func PriceScore(r PriceRange, price float64) float64 {
	if price <= 0 {
		return 0
	}
	min, max := r.Min, r.Max
	if min == 0 && max > 0 {
		if price <= max {
			return 1
		}
		return clamp01(1 - ((price-max)/max)/0.5)
	}
	if max == 0 && min > 0 {
		if price >= min {
			return 1
		}
		return clamp01(1 - ((min-price)/min)/0.5)
	}
	if min <= 0 || max <= 0 {
		return 0
	}
	if price >= min && price <= max {
		return 1
	}
	if price < min {
		return clamp01(1 - ((min-price)/min)/0.5)
	}
	return clamp01(1 - ((price-max)/max)/0.5)
}

func LocationScore(preferred string, p Property) float64 {
	terms := SplitLocationTerms(preferred)
	if len(terms) == 0 {
		return 0
	}
	haystack := strings.ToLower(strings.Join([]string{p.Neighborhood, p.City, p.Street, p.State}, " "))
	best := 0.0
	for _, term := range terms {
		t := strings.ToLower(strings.TrimSpace(term))
		if t == "" {
			continue
		}
		if strings.Contains(haystack, t) {
			best = 1
			continue
		}
		if len(t) >= 5 {
			for _, word := range strings.Fields(haystack) {
				if strings.HasPrefix(word, t[:5]) && best < 0.7 {
					best = 0.7
				}
			}
		}
	}
	return best
}

func AreaScore(minPtr, maxPtr *float64, area float64) float64 {
	if area <= 0 {
		return 0
	}
	var min, max float64
	if positive(minPtr) {
		min = *minPtr
	}
	if positive(maxPtr) {
		max = *maxPtr
	}
	switch {
	case min > 0 && max > 0:
		if area >= min && area <= max {
			return 1
		}
		if area < min {
			return clamp01(1 - ((min-area)/min)/0.5)
		}
		return clamp01(1 - ((area-max)/max)/0.5)
	case max > 0:
		if area <= max {
			return 1
		}
		return clamp01(1 - ((area-max)/max)/0.5)
	case min > 0:
		if area >= min {
			return 1
		}
		return clamp01(1 - ((min-area)/min)/0.5)
	default:
		return 0
	}
}

func BedroomsScore(requested, available int) float64 {
	if available <= 0 {
		return 0
	}
	if available >= requested {
		extra := float64(available - requested)
		if extra <= 1 {
			return 1
		}
		return 1 - math.Min(0.3, (extra-1)*0.15)
	}
	return clamp01(1 - float64(requested-available)*0.5)
}

func RedistributeWeights(base Weights, hasPrice, hasLocation, hasArea, hasBedrooms bool) Weights {
	return RedistributeWeightsWithBusiness(base, false, hasPrice, hasLocation, hasArea, hasBedrooms)
}

// RedistributeWeightsWithBusiness keeps the business weight dominant and
// fixed. Only the optional property criteria share the remaining score budget;
// this prevents a lead with no location/area/bedroom preference from
// accidentally turning a business-only match into 75+ points. The old
// RedistributeWeights signature is retained for callers without an operation
// dimension.
func RedistributeWeightsWithBusiness(base Weights, hasBusiness, hasPrice, hasLocation, hasArea, hasBedrooms bool) Weights {
	active := Weights{}
	total := 0.0
	if hasBusiness {
		active.Business = base.Business
	}
	if hasPrice {
		active.Price = base.Price
		total += base.Price
	}
	if hasLocation {
		active.Location = base.Location
		total += base.Location
	}
	if hasArea {
		active.Area = base.Area
		total += base.Area
	}
	if hasBedrooms {
		active.Bedrooms = base.Bedrooms
		total += base.Bedrooms
	}
	if total <= 0 {
		if hasBusiness && active.Business > 0 {
			active.Business = 100
		}
		return active
	}
	target := 100.0
	if hasBusiness {
		target -= active.Business
		if target < 0 {
			target = 0
		}
	}
	mult := target / total
	active.Price *= mult
	active.Location *= mult
	active.Area *= mult
	active.Bedrooms *= mult
	return active
}

func SplitLocationTerms(raw string) []string {
	parts := strings.Split(strings.NewReplacer("/", ",", ";", ",", "|", ",").Replace(raw), ",")
	terms := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			terms = append(terms, value)
		}
	}
	return terms
}

// DecodePreferences accepts both the current JSON-tagged representation and
// legacy metadata that used PascalCase keys (and no underscores).
func DecodePreferences(raw []byte) (Preferences, error) {
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return Preferences{}, err
	}
	var out Preferences
	out.Type = stringValue(root, "type")
	out.BusinessType = firstStringValue(root, "business_type", "listing_type", "operation", "deal_type")
	out.PreferredLocation = stringValue(root, "preferred_location")
	price := objectValue(root, "price_range")
	out.PriceRange.Min = numberValue(price, "min")
	out.PriceRange.Max = numberValue(price, "max")
	features := objectValue(root, "features")
	if value, ok := positiveIntValue(features, "bedrooms"); ok {
		out.Bedrooms = &value
	}
	if value, ok := positiveFloatValue(features, "area_min"); ok {
		out.AreaMin = &value
	}
	if value, ok := positiveFloatValue(features, "area_max"); ok {
		out.AreaMax = &value
	}
	return out, nil
}

func RedistributeAndScore(p Preferences, candidate Property) Breakdown {
	return Score(p, candidate, DefaultWeights)
}

func positive(value *float64) bool { return value != nil && *value > 0 }

func clamp01(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func normalizedKey(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	return strings.ReplaceAll(value, "-", "")
}

func firstStringValue(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringValue(values, key); value != "" {
			return value
		}
	}
	return ""
}

func normalizeBusinessType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "sale", "buy", "purchase", "compra", "venda", "compra_e_venda", "compra e venda":
		return "sale"
	case "rent", "lease", "rental", "locacao", "locação", "aluguel", "alugar":
		return "rent"
	case "exchange", "permuta":
		return "exchange"
	default:
		return ""
	}
}

func findValue(values map[string]any, key string) (any, bool) {
	wanted := normalizedKey(key)
	for actual, value := range values {
		if normalizedKey(actual) == wanted {
			return value, true
		}
	}
	return nil, false
}

func stringValue(values map[string]any, key string) string {
	value, _ := findValue(values, key)
	result, _ := value.(string)
	return result
}

func objectValue(values map[string]any, key string) map[string]any {
	value, _ := findValue(values, key)
	result, _ := value.(map[string]any)
	return result
}

func numberValue(values map[string]any, key string) float64 {
	value, _ := findValue(values, key)
	switch result := value.(type) {
	case float64:
		return result
	case json.Number:
		parsed, _ := result.Float64()
		return parsed
	default:
		return 0
	}
}

func positiveIntValue(values map[string]any, key string) (int, bool) {
	value := int(numberValue(values, key))
	return value, value > 0
}

func positiveFloatValue(values map[string]any, key string) (float64, bool) {
	value := numberValue(values, key)
	return value, value > 0
}

// SortScores orders a parallel slice of scores from best to worst. It is kept
// small and generic so service repositories can attach the result to their
// own response DTO without sharing domain packages.
func SortScores(scores []float64, prices []float64) []int {
	indices := make([]int, len(scores))
	for i := range scores {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		left, right := indices[i], indices[j]
		if scores[left] != scores[right] {
			return scores[left] > scores[right]
		}
		if left < len(prices) && right < len(prices) && prices[left] != prices[right] {
			return prices[left] < prices[right]
		}
		return left < right
	})
	return indices
}
