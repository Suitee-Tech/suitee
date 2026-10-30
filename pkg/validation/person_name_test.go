package validation

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizePersonName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"João Silva", "João Silva"},
		{"  João Silva  ", "João Silva"},
		{"João   Silva", "João Silva"},
		{"\tJoão\tSilva\n", "João Silva"},
		{"", ""},
		{"   ", ""},
		{"José da Silva Jr", "José da Silva Jr"},
	}
	for _, c := range cases {
		if got := SanitizePersonName(c.in); got != c.want {
			t.Errorf("SanitizePersonName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestValidatePersonName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr string // empty = must pass
	}{
		{"happy", "João", ""},
		{"happy full", "João Silva", ""},
		{"empty", "", "não pode estar vazio"},
		{"whitespace only", "   ", "não pode estar vazio"},
		{"single char", "J", "muito curto"},
		{"with digits", "João2", "não pode conter números"},
		{"digits in middle", "Maria 3ª", "não pode conter números"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidatePersonName(c.input, "Nome")
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

func TestValidateFullPersonName(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"happy", "João Silva", ""},
		{"three names", "José da Silva", ""},
		{"mononym rejected", "João", "precisa incluir sobrenome"},
		{"with digits", "João 2 Silva", "não pode conter números"},
		{"empty", "", "não pode estar vazio"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateFullPersonName(c.input, "Nome")
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

// A camada de transporte usa este predicado para responder 400 com o motivo do
// campo em vez de 500. O texto das mensagens é contrato com o formulário.
func TestIsPersonNameError(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"monônimo", ValidateFullPersonName("João", "Nome"), true},
		{"vazio", ValidateFullPersonName("", "Nome"), true},
		{"curto", ValidateFullPersonName("J", "Nome"), true},
		{"com número", ValidateFullPersonName("João 2 Silva", "Nome"), true},
		{"sem erro", nil, false},
		{"erro de outro tipo", errors.New("invalid phone: 12345"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsPersonNameError(tc.err); got != tc.want {
				t.Fatalf("IsPersonNameError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
