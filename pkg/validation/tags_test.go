package validation

import (
	"fmt"
	"strings"
	"testing"
)

func TestNormalizeTag(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{"espaços", "  Piscina  ", "Piscina"},
		{"espaços internos", "Salão   de    Festas", "Salão de Festas"},
		{"quebra de linha", "Playground\ncoberto", "Playground coberto"},
		{"caractere de controle", "Wi\u0000Fi", "WiFi"},
		{"vazio", "   ", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeTag(tc.input); got != tc.want {
				t.Fatalf("NormalizeTag(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestNormalizeTags(t *testing.T) {
	got := NormalizeTags([]string{"Piscina", " piscina ", "", "Academia", "ACADEMIA", "  Salão de Festas "})
	want := []string{"Piscina", "Academia", "Salão de Festas"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeTags = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeTags = %v, want %v", got, want)
		}
	}
}

func TestValidateTags(t *testing.T) {
	t.Run("lista normal", func(t *testing.T) {
		if err := ValidateTags([]string{"Piscina", "Academia"}, "Comodidades"); err != nil {
			t.Fatalf("não esperava erro: %v", err)
		}
	})

	t.Run("acima do teto de itens", func(t *testing.T) {
		tags := make([]string, MaxTagItems+1)
		for i := range tags {
			tags[i] = fmt.Sprintf("Item %d", i) // distintos: o teto é de itens únicos
		}
		err := ValidateTags(tags, "Comodidades")
		if err == nil || !strings.Contains(err.Error(), "máximo de") {
			t.Fatalf("esperava erro de quantidade, veio %v", err)
		}
	})

	t.Run("duplicatas não contam duas vezes", func(t *testing.T) {
		tags := make([]string, MaxTagItems)
		for i := range tags {
			tags[i] = "Piscina"
		}
		if err := ValidateTags(tags, "Comodidades"); err != nil {
			t.Fatalf("duplicatas deveriam colapsar em um item: %v", err)
		}
	})

	t.Run("item acima do teto de caracteres", func(t *testing.T) {
		long := strings.Repeat("a", MaxTagRunes+1)
		err := ValidateTags([]string{long}, "Comodidades")
		if err == nil || !strings.Contains(err.Error(), "até") {
			t.Fatalf("esperava erro de tamanho, veio %v", err)
		}
	})
}
