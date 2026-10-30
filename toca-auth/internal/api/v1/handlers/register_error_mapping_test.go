package handlers

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/oginoino/toca-auth/internal/domain/auth"
)

func TestIsDuplicateRegistrationError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"sentinel de e-mail", auth.ErrEmailAlreadyExists, true},
		{"sentinel de telefone", auth.ErrPhoneAlreadyExists, true},
		{"sentinel embrulhado", fmt.Errorf("register: %w", auth.ErrEmailAlreadyExists), true},
		{"string legada de e-mail", errors.New("email already exists"), true},
		{"string legada de telefone", errors.New("phone already exists"), true},
		{"email_exists bruto", errors.New(`supabase request failed: status 422: {"error_code":"email_exists"}`), true},
		{"user_already_exists bruto", errors.New("user_already_exists"), true},
		{"already been registered bruto", errors.New("A user with this email address has already been registered"), true},
		{"falha de sign-up", auth.ErrSignUpFailed, false},
		{"erro do provedor", errors.New(`supabase request failed: status 422: {"error_code":"email_provider_disabled"}`), false},
		{"nil", nil, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDuplicateRegistrationError(tc.err); got != tc.want {
				t.Fatalf("isDuplicateRegistrationError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// O 409 de conflito não pode confirmar qual dado colidiu nem sugerir que a
// conta existe: o texto é o mesmo para e-mail e telefone.
func TestRegistrationConflictMessageAvoidsAccountHints(t *testing.T) {
	msg := strings.ToLower(registrationConflictMessage)
	for _, hint := range []string{"e-mail", "email", "telefone", "phone", "cadastrado", "existe", "conta"} {
		if strings.Contains(msg, hint) {
			t.Fatalf("mensagem de conflito contém indício de conta (%q): %q", hint, registrationConflictMessage)
		}
	}
}
