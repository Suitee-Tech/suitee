package handlers_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/handlers"
	gwmw "github.com/oginoino/toca/gateway/internal/middleware"
	"github.com/oginoino/toca/gateway/internal/services/proxy"
)

// P0/C10 — prova no roteador REAL do gateway (mesmos registros de rota e mesma
// ordem de middleware do cmd/gateway/main.go) de que as rotas /internal/ não são
// mais alcançáveis pela borda pública.
//
// Antes da correção: `RegisterDeal` registrava
// `/api/v1/deal/internal/*path` e `RegisterSignature` registrava
// `/api/v1/signature/*path` (que casa `/signature/internal/...`), ambos sem
// denylist — e o proxy repassa headers do cliente sem filtrar. Com a
// INTERNAL_API_KEY, qualquer um lia deals/contratos/documentos de qualquer
// tenant informando `?tenant_id=`.
func TestGatewayDoesNotServeInternalPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Backend de mentira: responde 200 em tudo o que chegar. Serve para provar
	// que a requisição NÃO chega ao serviço quando o caminho é interno.
	hit := 0
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer backend.Close()

	cfg := gwconfig.Config{
		DealServiceURL:      backend.URL,
		SignatureServiceURL: backend.URL,
		CRMServiceURL:       backend.URL,
		FinancialServiceURL: backend.URL,
		CoreServiceURL:      backend.URL,
	}

	// O reverse proxy precisa de um http.ResponseWriter real (CloseNotifier),
	// então o gateway roda em um servidor de teste de verdade — mais próximo
	// do comportamento em produção do que um ResponseRecorder.
	r := gin.New()
	r.Use(gwmw.RejectInternalPaths())
	pxy := proxy.New()
	handlers.RegisterDeal(r, cfg, pxy)
	handlers.RegisterSignature(r, cfg, pxy)
	handlers.RegisterCRM(r, cfg, pxy)

	gw := httptest.NewServer(r)
	defer gw.Close()

	send := func(method, path string) int {
		t.Helper()
		req, err := http.NewRequest(method, gw.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Internal-Key", "chave-vazada")
		req.Header.Set("X-Tenant-Id", "tenant-vitima")
		resp, err := gw.Client().Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	internal := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/deal/internal/deals/abc?tenant_id=victim"},
		{http.MethodGet, "/api/v1/deal/internal/contracts/abc?tenant_id=victim"},
		{http.MethodGet, "/api/v1/deal/internal/documents/abc/content?tenant_id=victim"},
		{http.MethodGet, "/api/v1/signature/internal/envelopes/by-contract/abc?tenant_id=victim"},
		{http.MethodPost, "/api/v1/signature/internal/envelopes/by-contract/abc"},
		{http.MethodGet, "/api/v1/crm/internal/leads/abc?tenant_id=victim"},
		{http.MethodGet, "/api/v1/deal/INTERNAL/deals/abc"},
	}
	for _, tc := range internal {
		before := hit
		if got := send(tc.method, tc.path); got != http.StatusNotFound {
			t.Errorf("%s %s: status = %d, want 404", tc.method, tc.path, got)
		}
		if hit != before {
			t.Errorf("%s %s: a requisição alcançou o serviço de destino (não deveria)", tc.method, tc.path)
		}
	}

	// As rotas públicas continuam funcionando (não quebramos funcionalidade).
	public := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/deals/abc"},
		{http.MethodGet, "/api/v1/deals"},
		{http.MethodGet, "/api/v1/contracts/abc"},
		{http.MethodGet, "/api/v1/signature/envelopes"},
		{http.MethodGet, "/api/v1/leads"},
	}
	for _, tc := range public {
		before := hit
		if got := send(tc.method, tc.path); got != http.StatusOK {
			t.Errorf("%s %s: status = %d, want 200 (rota pública)", tc.method, tc.path, got)
		}
		if hit == before {
			t.Errorf("%s %s: não chegou ao serviço de destino (rota pública quebrada)", tc.method, tc.path)
		}
	}
}
