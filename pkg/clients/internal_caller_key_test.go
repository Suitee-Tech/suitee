package clients

import "testing"

// Regressão da migração de credencial interna: enquanto INTERNAL_CALLER_KEY não
// existir, nada muda (chave compartilhada); quando existir, ela é a chave
// apresentada nas chamadas internas.
func TestInternalCallerKeyPrecedence(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "chave-compartilhada")
	t.Setenv("INTERNAL_CALLER_KEY", "")
	if got := internalCallerKey(); got != "chave-compartilhada" {
		t.Errorf("sem INTERNAL_CALLER_KEY: %q, want chave-compartilhada", got)
	}

	t.Setenv("INTERNAL_CALLER_KEY", "chave-do-deal")
	if got := internalCallerKey(); got != "chave-do-deal" {
		t.Errorf("com INTERNAL_CALLER_KEY: %q, want chave-do-deal", got)
	}

	// Espaços não podem virar uma credencial "válida" por acidente.
	t.Setenv("INTERNAL_CALLER_KEY", "   ")
	if got := internalCallerKey(); got != "chave-compartilhada" {
		t.Errorf("INTERNAL_CALLER_KEY em branco deveria cair para a compartilhada: %q", got)
	}
}
