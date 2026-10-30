package middleware

import "testing"

func TestOriginAllowedExactAndWildcard(t *testing.T) {
	allowed := []string{
		"http://localhost:5173",
		"https://*.imoveis.suitee.com.br",
	}
	cases := []struct {
		origin string
		want   bool
	}{
		{"http://localhost:5173", true},
		{"https://imobiliaria-exemplo.imoveis.suitee.com.br", true},
		{"https://a.b.imoveis.suitee.com.br", true},
		{"https://IMOBILIARIA-exemplo.imoveis.SUITEE.COM.BR", true},
		{"http://imobiliaria-exemplo.imoveis.suitee.com.br", false}, // scheme
		{"https://imoveis.suitee.com.br", false},                   // apex
		{"https://evil-imoveis.suitee.com.br", false},
		{"https://a.imoveis.suitee.com.br.evil.com", false},
		{"https://example.com", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := OriginAllowed(tc.origin, allowed); got != tc.want {
			t.Errorf("OriginAllowed(%q)=%v want %v", tc.origin, got, tc.want)
		}
	}
}

func TestOriginAllowedWildcardAll(t *testing.T) {
	if !OriginAllowed("https://anything.example", []string{"*"}) {
		t.Fatal("padrão * deve aceitar qualquer origem")
	}
	if OriginAllowed("https://anything.example", []string{"https://app.example"}) {
		t.Fatal("origem fora da allowlist exata não pode passar")
	}
}

func TestHasWildcardOrigin(t *testing.T) {
	if !HasWildcardOrigin([]string{"https://app.example", "https://*.imoveis.example"}) {
		t.Fatal("padrão com * deve ser detectado")
	}
	if HasWildcardOrigin([]string{"https://app.example"}) {
		t.Fatal("lista exata não tem wildcard")
	}
}
