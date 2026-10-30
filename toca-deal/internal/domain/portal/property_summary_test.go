package portal

import "testing"

// A vitrine (busca e recomendações) não pode entregar rua e número ao cliente
// antes de ele agendar a visita: o endereço sai mascarado do backend, e não
// apenas escondido na tela.
func TestMaskPropertyPrivacyBlanksAddressOnly(t *testing.T) {
	original := &PropertySummary{
		ID:            "imovel-1",
		Title:         "Casa Vitória Régia",
		Address:       "Rua Vitória Régia, 152",
		Neighborhood:  "Vila Gardênia",
		City:          "Atibaia",
		State:         "SP",
		Price:         450000,
		Bedrooms:      3,
		Suites:        4,
		Bathrooms:     3,
		ParkingSpaces: 8,
		TotalAreaM2:   300,
		UsableAreaM2:  300,
		ListingType:   "sale",
		Amenities:     []string{"piscina"},
		Description:   "Casa Vitória Régia",
		Images:        []PropertyMedia{{URL: "https://files.example/fachada.jpg", Description: "Fachada"}},
	}

	masked := MaskPropertyPrivacy(original)

	if masked.Address != "" {
		t.Fatalf("endereço deveria sair mascarado da vitrine, veio %q", masked.Address)
	}
	// A máscara não pode empobrecer o que qualifica a decisão do cliente.
	if masked.Title != "Casa Vitória Régia" || masked.Neighborhood != "Vila Gardênia" ||
		masked.City != "Atibaia" || masked.State != "SP" {
		t.Fatalf("localização por bairro/cidade/UF deveria permanecer, veio %+v", masked)
	}
	if masked.Suites != 4 || masked.Bedrooms != 3 || masked.Bathrooms != 3 || masked.ParkingSpaces != 8 {
		t.Fatalf("características deveriam permanecer, veio %+v", masked)
	}
	if masked.TotalAreaM2 != 300 || masked.UsableAreaM2 != 300 || masked.Price != 450000 {
		t.Fatalf("áreas e preço deveriam permanecer, veio %+v", masked)
	}
	if len(masked.Amenities) != 1 || masked.Amenities[0] != "piscina" {
		t.Fatalf("comodidades deveriam permanecer, veio %+v", masked.Amenities)
	}
	if masked.Description == "" || len(masked.Images) != 1 || masked.Images[0].Description != "Fachada" {
		t.Fatalf("descrição e galeria deveriam permanecer, veio %+v", masked)
	}
}

func TestMaskPropertyPrivacyNilIsSafe(t *testing.T) {
	if got := MaskPropertyPrivacy(nil); got != nil {
		t.Fatalf("máscara de resumo ausente deveria devolver nil, veio %+v", got)
	}
}
