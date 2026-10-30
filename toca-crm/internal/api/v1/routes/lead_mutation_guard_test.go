package routes

import (
	"os"
	"strings"
	"testing"
)

// P1 / A26 — o registro das rotas de mutação de lead precisa manter o
// middleware de posse.
//
// Os testes de handler montam o próprio router, então remover
// `RequireLeadOwnership()` de `routes.go` NÃO seria detectado por eles (foi o
// que a verificação por mutação mostrou). Este teste trava o registro real: toda
// rota de mutação em `/leads/:id` que aceita corpo/recurso do lead tem de passar
// pelo middleware de posse.
func TestLeadMutationRoutesKeepOwnershipMiddleware(t *testing.T) {
	src, err := os.ReadFile("routes.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	mutating := []string{
		`leadsProtected.PUT("/:id",`,
		`leadsProtected.PATCH("/:id",`,
		`leadsProtected.POST("/:id/notes",`,
		`leadsProtected.PUT("/:id/recommended-property",`,
		`leadsProtected.PUT("/:id/visit/confirm",`,
		`leadsProtected.POST("/:id/first-contact",`,
		`leadsProtected.POST("/:id/disqualify",`,
		`leadsProtected.POST("/:id/convert",`,
		`leadsProtected.POST("/:id/invite",`,
	}

	lines := strings.Split(body, "\n")
	for _, prefix := range mutating {
		found := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if !strings.HasPrefix(trimmed, prefix) {
				continue
			}
			found = true
			if !strings.Contains(trimmed, "RequireLeadOwnership()") {
				t.Errorf("P0/P1-A26: a rota %q perdeu o middleware de posse:\n  %s", prefix, trimmed)
			}
			break
		}
		if !found {
			t.Errorf("rota esperada não encontrada em routes.go: %q (foi renomeada? atualize este teste e garanta a posse)", prefix)
		}
	}
}

// O gate de reatribuição vive no handler (depende do CORPO), então é testado
// pela presença da checagem de `leads:reassign` no caminho do PATCH.
func TestLeadPatchRequiresReassignPermission(t *testing.T) {
	src, err := os.ReadFile("../handlers/lead_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)

	if !strings.Contains(body, `"leads:reassign"`) {
		t.Fatal("P0/P1-A26: o PATCH de lead não exige `leads:reassign` para trocar o corretor atribuído")
	}
	if !strings.Contains(body, "leads:reassign is required to change the assigned broker") {
		t.Fatal("P0/P1-A26: a recusa de reatribuição sem permissão desapareceu")
	}
}
