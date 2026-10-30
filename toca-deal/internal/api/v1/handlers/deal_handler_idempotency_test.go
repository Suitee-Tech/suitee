package handlers

import "testing"

func TestNormalizeIdempotencyKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"vazio", "", ""},
		{"ascii puro", "deal-create:t1:p1:c1", "deal-create:t1:p1:c1"},
		{"utf8 válido", "deal-create:criação:joão", "deal-create:criação:joão"},
		// Navegadores enviam headers em Latin-1: "ção" vira 0xe7 0xe3 0x6f.
		{"latin1 ção", string([]byte{0xe7, 0xe3, 0x6f}), "ção"},
		{"latin1 misto", "deal-" + string([]byte{0xe3}) + "timo", "deal-ãtimo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeIdempotencyKey(tc.in); got != tc.want {
				t.Fatalf("normalizeIdempotencyKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
