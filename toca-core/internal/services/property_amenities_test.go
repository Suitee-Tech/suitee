package services

import (
	"errors"
	"strings"
	"testing"

	"github.com/oginoino/toca-core/internal/domain/core/property"
	"github.com/oginoino/toca/pkg/validation"
)

// A lista de comodidades é validada e normalizada no serviço (create e update):
// sem teto, um cliente grava milhares de itens no JSONB do imóvel; sem
// normalização, "Piscina" e "piscina " viram duas comodidades na busca e no
// contrato. A mensagem precisa ser a que o operador lê no formulário.
func TestValidateAmenities(t *testing.T) {
	t.Run("normaliza espaços, vazios e duplicatas", func(t *testing.T) {
		p := &property.Property{Amenities: []string{"  Piscina  ", "piscina", "", " Academia "}}
		if err := validateAmenities(p); err != nil {
			t.Fatalf("não esperava erro: %v", err)
		}
		want := []string{"Piscina", "Academia"}
		if len(p.Amenities) != len(want) {
			t.Fatalf("amenities = %v, want %v", p.Amenities, want)
		}
		for i := range want {
			if p.Amenities[i] != want[i] {
				t.Fatalf("amenities = %v, want %v", p.Amenities, want)
			}
		}
	})

	t.Run("acima do teto de itens", func(t *testing.T) {
		many := make([]string, validation.MaxTagItems+1)
		for i := range many {
			many[i] = strings.Repeat("a", 3) + string(rune('a'+i%26)) + strings.Repeat("b", i)
		}
		err := validateAmenities(&property.Property{Amenities: many})
		var target *AmenitiesValidationError
		if !errors.As(err, &target) {
			t.Fatalf("esperava AmenitiesValidationError, veio %v", err)
		}
		if !strings.Contains(target.Message, "máximo de") {
			t.Fatalf("mensagem deveria citar o teto: %q", target.Message)
		}
	})

	t.Run("item acima do teto de caracteres", func(t *testing.T) {
		err := validateAmenities(&property.Property{Amenities: []string{strings.Repeat("a", validation.MaxTagRunes+1)}})
		var target *AmenitiesValidationError
		if !errors.As(err, &target) {
			t.Fatalf("esperava AmenitiesValidationError, veio %v", err)
		}
	})

	t.Run("lista vazia vira nulo", func(t *testing.T) {
		p := &property.Property{Amenities: []string{"  ", ""}}
		if err := validateAmenities(p); err != nil {
			t.Fatalf("não esperava erro: %v", err)
		}
		if p.Amenities != nil {
			t.Fatalf("lista só com vazios deveria virar nulo, veio %v", p.Amenities)
		}
	})
}
