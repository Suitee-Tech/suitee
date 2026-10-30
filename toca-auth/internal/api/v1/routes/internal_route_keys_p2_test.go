package routes_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oginoino/toca/pkg/auth"
)

// M8 — as rotas internas do toca-auth precisam da MESMA guarda das demais
// (`auth.RequireInternalKey()`), não de uma comparação `!=` caseira.
//
// Dois efeitos observáveis provam a troca:
//
//  1. chave NOMEADA (`INTERNAL_SERVICE_KEYS=nome:chave`) passa a valer nessas
//     rotas — antes só a `INTERNAL_API_KEY` compartilhada era aceita, o que
//     travava a migração do A2 justamente nas rotas mais sensíveis (criação de
//     conta de cliente e listagem de membros da empresa);
//  2. o limite de 60/min por IP da guarda passa a existir (antes: nenhum).
func TestInternalRoutesUseRequireInternalKeyGuard(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "chave-compartilhada-legada")
	t.Setenv("INTERNAL_SERVICE_KEYS", "toca-deal:chave-nomeada-do-deal")
	auth.ReloadServiceKeyRegistry()
	defer auth.SetAdminRateLimitForTest(3)()

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)

	namedKey := func(path string, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "198.51.100.60:4444"
		req.Header.Set("X-Internal-Key", key)
		req.Header.Set("X-Tenant-Id", "tenant-1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	// A chave compartilhada antiga NÃO pode mais ser usada por um cliente que
	// já tem identidade nomeada... e a nomeada precisa funcionar.
	denied := namedKey("/api/v1/auth/internal/companies/c1/members", "chave-errada")
	if denied.Code != http.StatusUnauthorized {
		t.Fatalf("chave inválida respondeu %d, want 401", denied.Code)
	}

	accepted := namedKey("/api/v1/auth/internal/companies/c1/members", "chave-nomeada-do-deal")
	if accepted.Code == http.StatusUnauthorized {
		t.Fatalf("chave nomeada recusada (%d): a rota ainda usa a comparação caseira com INTERNAL_API_KEY", accepted.Code)
	}

	// Terceira e quarta chamadas do mesmo IP estouram o limite de 3/min: prova
	// que a guarda aplica rate limit (a comparação caseira não aplicava nenhum).
	limited := false
	for i := 0; i < 3; i++ {
		w := namedKey("/api/v1/auth/internal/companies/c1/members", "chave-nomeada-do-deal")
		if w.Code == http.StatusForbidden && strings.Contains(w.Body.String(), "rate limit") {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatalf("a rota interna não aplicou o rate limit da guarda RequireInternalKey")
	}
}
