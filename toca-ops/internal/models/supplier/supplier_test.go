package supplier

import (
	"errors"
	"testing"
)

func TestContactAndDocumentValidation(t *testing.T) {
	for _, tc := range []struct {
		name, phone, email, document string
		want                         error
	}{
		{"mobile cpf", "(11) 98888-7777", "", "529.982.247-25", nil},
		{"landline cnpj", "11 3333 4444", "", "04.252.011/0001-10", nil},
		{"email only", "", "ana@example.com", "", nil},
		{"no contact", "", "", "", ErrContactRequired},
		{"incomplete phone", "11999", "ana@example.com", "", ErrInvalidPhone},
		{"invalid ddd", "00988887777", "", "", ErrInvalidPhone},
		{"invalid mobile", "11888887777", "", "", ErrInvalidPhone},
		{"invalid landline", "1163334444", "", "", ErrInvalidPhone},
		{"phone letters", "abc11988887777", "", "", ErrInvalidPhone},
		{"email invalid", "11988887777", "ana@", "", ErrInvalidEmail},
		{"cpf repeated", "11988887777", "", "11111111111", ErrInvalidDocument},
		{"cpf checksum", "11988887777", "", "52998224724", ErrInvalidDocument},
		{"cnpj checksum", "11988887777", "", "04252011000111", ErrInvalidDocument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := New("Empresa 123", "Vistoria", "externo", tc.document, tc.email, tc.phone, "", "", "tenant")
			if err := v.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("got %v want %v", err, tc.want)
			}
		})
	}
}
func TestCanonicalContacts(t *testing.T) {
	v := New(" Ana ", " Vistoria ", "externo", "529.982.247-25", " Ana@EXAMPLE.COM ", "(11) 98888-7777", "", "", "tenant")
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	if v.Document != "52998224725" || v.Phone != "+5511988887777" || v.Email != "ana@example.com" {
		t.Fatalf("not normalized: %+v", v)
	}
	v.Normalize()
	if v.Phone != "+5511988887777" {
		t.Fatal("country code duplicated")
	}
}
