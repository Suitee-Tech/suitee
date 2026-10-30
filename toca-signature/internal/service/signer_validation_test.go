package service

import (
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-signature/internal/clicksign"
)

// validateSignerForClicksign is a thin shim around pkg/validation. We test it
// here only to assert that the shim wires the right field name into the error
// message and catches Email separately (Clicksign requires both).
func TestValidateSignerForClicksign(t *testing.T) {
	cases := []struct {
		name    string
		req     clicksign.SignerRequest
		wantErr string // empty = must pass
	}{
		{"happy", clicksign.SignerRequest{Name: "João Silva", Email: "j@x.com"}, ""},
		{"empty name", clicksign.SignerRequest{Name: "", Email: "j@x.com"}, "não pode estar vazio"},
		{"mononym", clicksign.SignerRequest{Name: "João", Email: "j@x.com"}, "sobrenome"},
		{"with digits", clicksign.SignerRequest{Name: "João 2 Silva", Email: "j@x.com"}, "não pode conter números"},
		{"empty email", clicksign.SignerRequest{Name: "João Silva", Email: ""}, "email is empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateSignerForClicksign(c.req)
			if c.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), c.wantErr)
			}
		})
	}
}
