package service

import (
	"strings"
	"testing"
)

func TestRecommendationCopy(t *testing.T) {
	if got := recommendationSubject(""); got != "Sugestão do seu corretor" {
		t.Errorf("subject without title: %q", got)
	}
	if got := recommendationSubject("Apto Centro"); got != "Sugestão do seu corretor: Apto Centro" {
		t.Errorf("subject with title: %q", got)
	}

	if got := recommendationPriceLine(0, 0); !strings.Contains(got, "sob consulta") {
		t.Errorf("price without values: %q", got)
	}
	// Preço em real brasileiro: o e-mail mostra o mesmo valor do anúncio.
	if got := recommendationPriceLine(350000, 0); !strings.Contains(got, "R$ 350.000,00") {
		t.Errorf("sale price: %q", got)
	}
	if got := recommendationPriceLine(350000, 1800); !strings.Contains(got, "Aluguel") || !strings.Contains(got, "R$ 1.800,00") {
		t.Errorf("rent wins: %q", got)
	}

	if got := recommendationLocationLine("", "", ""); got != "" {
		t.Errorf("empty location: %q", got)
	}
	if got := recommendationLocationLine("Centro", "São Paulo", "SP"); got != "Centro · São Paulo · SP" {
		t.Errorf("full location: %q", got)
	}

	body := recommendationClientBody("Apto Centro", "Centro", "São Paulo", "SP", 350000, 0)
	for _, want := range []string{"Apto Centro", "Centro · São Paulo · SP", "R$ 350.000,00"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}

	inapp := recommendationInAppContent("Apto Centro", "Centro", "São Paulo")
	if !strings.Contains(inapp, "Apto Centro") || !strings.Contains(inapp, "Centro") {
		t.Errorf("in-app content: %q", inapp)
	}
	inapp = recommendationInAppContent("", "", "")
	if !strings.Contains(inapp, "recomendado") {
		t.Errorf("in-app content fallback: %q", inapp)
	}
}
