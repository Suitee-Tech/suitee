package site

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSlug(t *testing.T) {
	valid := []string{"exemplo", "imobiliaria-exemplo", "a1b", "casa-2", "x9y"}
	for _, slug := range valid {
		if err := ValidateSlug(slug); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, quer nil", slug, err)
		}
	}

	invalid := []string{
		"ab", "exemplo-", "-exemplo", "ca--riri", "cariri_2",
		"exemplo.imoveis", "Exemplo ", "www", "app", "api", "admin", "portal",
		"sites", "mail", "cdn", "imoveis", "suitee", "login", "painel", "teste",
		"demo", "imobiliaria", "imovel", "123", "2026",
		strings.Repeat("a", 49),
	}
	for _, slug := range invalid {
		if err := ValidateSlug(slug); err == nil {
			t.Errorf("ValidateSlug(%q) aceitou slug inválido", slug)
		} else if !errors.Is(err, ErrSlugInvalid) {
			t.Errorf("ValidateSlug(%q) erro não é ErrSlugInvalid: %v", slug, err)
		}
	}
}

func TestSlugifyFoldsPortugueseAndSymbols(t *testing.T) {
	cases := map[string]string{
		"Imobiliária Exemplo":      "imobiliaria-exemplo",
		"  Ção & Cia Imóveis  ":   "cao-cia-imoveis",
		"João's Imóveis!!":        "joao-s-imoveis",
		"---já---":                "ja",
		"Imobiliária   do   Vale": "imobiliaria-do-vale",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBuildPublicBaseURLUsesSingleSuffix(t *testing.T) {
	got := BuildPublicBaseURL("https", "imoveis.suitee.com.br", "exemplo")
	if got != "https://exemplo.imoveis.suitee.com.br" {
		t.Fatalf("got %q", got)
	}
	if BuildPublicBaseURL("", "", "exemplo") != "" {
		t.Fatal("sem sufixo não há URL pública")
	}
	if got := BuildPublicBaseURLWithPort("http", "imoveis.localhost", "exemplo", "3002"); got != "http://exemplo.imoveis.localhost:3002" {
		t.Fatalf("porta local: %q", got)
	}
	if got := BuildPublicBaseURLWithPort("https", "imoveis.suitee.com.br", "exemplo", ""); got != "https://exemplo.imoveis.suitee.com.br" {
		t.Fatalf("produção sem porta: %q", got)
	}
}

func TestSeoSlugStableAndCollisionSuffix(t *testing.T) {
	if got := SeoSlug("Apartamento 3 quartos no Centro", "prop-1", 1); got != "apartamento-3-quartos-no-centro" {
		t.Fatalf("got %q", got)
	}
	if got := SeoSlug("Apartamento 3 quartos no Centro", "prop-1", 2); got != "apartamento-3-quartos-no-centro-2" {
		t.Fatalf("got %q", got)
	}
	got := SeoSlug("!!!", "abcdef12-3456-7890-abcd-ef1234567890", 1)
	if got != "imovel-abcdef12" {
		t.Fatalf("fallback de título vazio = %q", got)
	}
	if got := SeoSlug("123", "abcdef12-3456-7890-abcd-ef1234567890", 1); got != "imovel-abcdef12" {
		t.Fatalf("fallback de título numérico = %q", got)
	}
	long := strings.Repeat("palavra ", 20)
	withSuffix := SeoSlug(long, "prop-1", 12)
	if len(withSuffix) > 80 || !strings.HasSuffix(withSuffix, "-12") {
		t.Fatalf("sufixo estourou o limite: len=%d slug=%q", len(withSuffix), withSuffix)
	}
}

func TestSlugFromHost(t *testing.T) {
	suffix := "imoveis.suitee.com.br"
	cases := []struct {
		host    string
		want    string
		wantErr bool
	}{
		{"exemplo.imoveis.suitee.com.br", "exemplo", false},
		{"exemplo.imoveis.Suitee.Com.Br:443", "exemplo", false},
		{"exemplo.imoveis.suitee.com.br.", "exemplo", false},
		{"https://exemplo.imoveis.suitee.com.br/imoveis/x", "exemplo", false},
		{"imoveis.suitee.com.br", "", true},
		{"a.b.imoveis.suitee.com.br", "", true},
		{"Exemplo.sites.suitee.com.br", "", true},
		{"exemplo.imoveis.suitee.com.br.evil.com", "", true},
		{"admin.imoveis.suitee.com.br", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		got, err := SlugFromHost(tc.host, suffix)
		if tc.wantErr {
			if err == nil {
				t.Errorf("SlugFromHost(%q) aceitou host inválido (%q)", tc.host, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("SlugFromHost(%q)=(%q,%v) want %q", tc.host, got, err, tc.want)
		}
	}

	if got, err := SlugFromHost("exemplo.imoveis.localhost:3000", "imoveis.localhost"); err != nil || got != "exemplo" {
		t.Fatalf("local dev: got %q err %v", got, err)
	}
}

func TestPropertyPublishRule(t *testing.T) {
	cases := []struct {
		status, publication string
		want                bool
	}{
		{"published", "published", true},
		{"published", "draft", false},
		{"prospecting", "published", false},
		{"sold", "published", false},
		{"draft", "draft", false},
	}
	for _, tc := range cases {
		p := PropertySnapshot{Status: tc.status, PublicationStatus: tc.publication}
		if got := p.IsPublishableOnSite(); got != tc.want {
			t.Errorf("status=%s publication=%s => %v want %v", tc.status, tc.publication, got, tc.want)
		}
	}
}

func TestPageBodyTextHandlesPlainAndLegacyJSON(t *testing.T) {
	if got := (Page{Body: []byte(`"texto simples"`)}).BodyText(); got != "texto simples" {
		t.Fatalf("plain: %q", got)
	}
	if got := (Page{Body: []byte(`{"text":"legado"}`)}).BodyText(); got != `{"text":"legado"}` {
		t.Fatalf("legado: %q", got)
	}
	if got := (Page{}).BodyText(); got != "" {
		t.Fatalf("vazio: %q", got)
	}
}
