package handlers

import "testing"

func TestHeaderSafeText_RemoveAcentosDoAviso(t *testing.T) {
	cases := map[string]string{
		"locação 1 sem CPF/CNPJ do locador — a RFB pode exigir o documento": "locacao 1 sem CPF/CNPJ do locador - a RFB pode exigir o documento",
		"código de município ausente (São Paulo)":                           "codigo de municipio ausente (Sao Paulo)",
		"sem acento": "sem acento",
	}
	for input, want := range cases {
		if got := headerSafeText(input); got != want {
			t.Errorf("headerSafeText(%q) = %q, esperado %q", input, got, want)
		}
	}
	// Todo caractere do resultado precisa ser ASCII imprimível: é o que o
	// cabeçalho HTTP aceita (e o que o toast do operador mostra legível).
	for _, r := range headerSafeText("acentuação — travessão e emoji 🚀") {
		if r < 0x20 || r > 0x7E {
			t.Fatalf("caractere não imprimível no cabeçalho: %q", r)
		}
	}
}
