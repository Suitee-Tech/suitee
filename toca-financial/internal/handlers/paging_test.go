package handlers

import "testing"

func TestNormalizePaging(t *testing.T) {
	cases := []struct {
		name                  string
		limit, offset         int64
		wantLimit, wantOffset int64
		wantPage              int
	}{
		{"defaults", 50, 0, 50, 0, 1},
		{"zero limit falls back (guards div-by-zero)", 0, 0, 50, 0, 1},
		{"negative limit falls back", -5, 100, 50, 100, 3},
		{"negative offset clamped to 0", 25, -10, 25, 0, 1},
		{"limit capped at max", 9999, 0, 200, 0, 1},
		{"page derived from offset", 20, 40, 20, 40, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, o, p := normalizePaging(tc.limit, tc.offset)
			if l != tc.wantLimit || o != tc.wantOffset || p != tc.wantPage {
				t.Fatalf("normalizePaging(%d, %d) = (%d, %d, %d), want (%d, %d, %d)",
					tc.limit, tc.offset, l, o, p, tc.wantLimit, tc.wantOffset, tc.wantPage)
			}
		})
	}
}
