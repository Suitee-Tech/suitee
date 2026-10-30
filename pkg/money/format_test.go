package money

import "testing"

func TestBRL(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want string
	}{
		{"milhar com centavos", 6000, "R$ 6.000,00"},
		{"dezena de milhar", 60000, "R$ 60.000,00"},
		{"centavos", 1234.5, "R$ 1.234,50"},
		{"zero", 0, "R$ 0,00"},
		{"centavo isolado", 0.05, "R$ 0,05"},
		{"abaixo de mil", 999.99, "R$ 999,99"},
		{"milhão", 1234567.89, "R$ 1.234.567,89"},
		{"negativo", -1500.75, "R$ -1.500,75"},
		{"arredonda para cima no centavo", 1.006, "R$ 1,01"},
		{"arredonda para baixo no centavo", 1.004, "R$ 1,00"},
		{"negativo que arredonda para zero não vira -0", -0.001, "R$ 0,00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := BRL(tc.in); got != tc.want {
				t.Fatalf("BRL(%v) = %q, esperado %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBRLWhole(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{60000, "R$ 60.000"},
		{59999.6, "R$ 60.000"},
		{1500, "R$ 1.500"},
		{0, "R$ 0"},
		{-2500.4, "R$ -2.500"},
	}
	for _, tc := range cases {
		if got := BRLWhole(tc.in); got != tc.want {
			t.Fatalf("BRLWhole(%v) = %q, esperado %q", tc.in, got, tc.want)
		}
	}
}

// O defeito relatado: "R$ 6000.00" no e-mail de proposta.
func TestBRLNaoUsaSeparadorIngles(t *testing.T) {
	got := BRL(6000)
	if got == "R$ 6000.00" {
		t.Fatal("formatação inglesa ainda presente")
	}
	if got != "R$ 6.000,00" {
		t.Fatalf("inesperado: %q", got)
	}
}
