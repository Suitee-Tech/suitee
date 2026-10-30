package service

import (
	"reflect"
	"sort"
	"testing"
)

// Wave P2.11 — qualification routing helpers were uncovered. These are
// the pure functions that decide what tags/region a lead carries into
// the eligibility check. Bugs here silently skew distribution without
// any audit trail; unit tests are cheap insurance.

func TestExtractQualificationValues(t *testing.T) {
	cases := []struct {
		name     string
		q        map[string]interface{}
		fieldIDs []string
		want     []string
	}{
		{name: "nil map", q: nil, fieldIDs: []string{"f1"}, want: nil},
		{name: "empty fields", q: map[string]interface{}{"f1": "x"}, fieldIDs: nil, want: nil},
		{name: "single string", q: map[string]interface{}{"f1": "luxo"}, fieldIDs: []string{"f1"}, want: []string{"luxo"}},
		{name: "[]string slice", q: map[string]interface{}{"f1": []string{"luxo", "praia"}}, fieldIDs: []string{"f1"}, want: []string{"luxo", "praia"}},
		{name: "[]interface{} (json default)", q: map[string]interface{}{"f1": []interface{}{"luxo", "praia"}}, fieldIDs: []string{"f1"}, want: []string{"luxo", "praia"}},
		{name: "trims whitespace", q: map[string]interface{}{"f1": "  luxo  "}, fieldIDs: []string{"f1"}, want: []string{"luxo"}},
		{name: "dedup case-insensitive", q: map[string]interface{}{"f1": []string{"Luxo", "LUXO", "luxo"}}, fieldIDs: []string{"f1"}, want: []string{"Luxo"}},
		{name: "multiple fields merged", q: map[string]interface{}{"f1": "luxo", "f2": "praia"}, fieldIDs: []string{"f1", "f2"}, want: []string{"luxo", "praia"}},
		{name: "missing field id is silent", q: map[string]interface{}{"f1": "luxo"}, fieldIDs: []string{"f1", "missing"}, want: []string{"luxo"}},
		{name: "non-string value is skipped", q: map[string]interface{}{"f1": 42}, fieldIDs: []string{"f1"}, want: nil},
		{name: "blank fieldID skipped", q: map[string]interface{}{"f1": "luxo"}, fieldIDs: []string{"  ", "f1"}, want: []string{"luxo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractQualificationValues(tc.q, tc.fieldIDs)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !equalUnordered(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestExtractQualificationLocation(t *testing.T) {
	cases := []struct {
		name    string
		q       map[string]interface{}
		fieldID string
		want    string
	}{
		{name: "empty fieldID", q: map[string]interface{}{"f1": "Centro"}, fieldID: "", want: ""},
		{name: "nil map", q: nil, fieldID: "f1", want: ""},
		{name: "single string", q: map[string]interface{}{"f1": "Centro"}, fieldID: "f1", want: "Centro"},
		{name: "multi values joined by comma", q: map[string]interface{}{"f1": []string{"Centro", "Sul"}}, fieldID: "f1", want: "Centro, Sul"},
		{name: "missing field returns empty", q: map[string]interface{}{}, fieldID: "f1", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractQualificationLocation(tc.q, tc.fieldID); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRegionMatches_Cases(t *testing.T) {
	cases := []struct {
		name              string
		regions           []string
		preferredLocation string
		want              bool
	}{
		{name: "empty regions never matches", want: false},
		{name: "empty location never matches", regions: []string{"Centro"}, want: false},
		{name: "exact match", regions: []string{"Centro"}, preferredLocation: "Centro", want: true},
		{name: "case-insensitive", regions: []string{"centro"}, preferredLocation: "CENTRO", want: true},
		{name: "comma-split location", regions: []string{"Sul"}, preferredLocation: "Centro, Sul", want: true},
		{name: "dash-split location", regions: []string{"Mooca"}, preferredLocation: "Brás-Mooca", want: true},
		{name: "no overlap returns false", regions: []string{"Norte"}, preferredLocation: "Sul, Centro", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := regionMatches(tc.regions, tc.preferredLocation); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestHasIntersection(t *testing.T) {
	cases := []struct {
		name string
		a, b []string
		want bool
	}{
		{name: "empty both", want: false},
		{name: "empty a", b: []string{"x"}, want: false},
		{name: "empty b", a: []string{"x"}, want: false},
		{name: "exact overlap", a: []string{"x"}, b: []string{"x"}, want: true},
		{name: "case-insensitive overlap", a: []string{"X"}, b: []string{"x"}, want: true},
		{name: "trim whitespace", a: []string{" x "}, b: []string{"x"}, want: true},
		{name: "disjoint", a: []string{"x"}, b: []string{"y"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hasIntersection(tc.a, tc.b); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func equalUnordered(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ca := append([]string(nil), a...)
	cb := append([]string(nil), b...)
	sort.Strings(ca)
	sort.Strings(cb)
	return reflect.DeepEqual(ca, cb)
}
