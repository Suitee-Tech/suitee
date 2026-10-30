package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// P0/C10 — a borda pública não pode servir rotas /internal/.
func TestRejectInternalPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cases := []struct {
		path       string
		wantStatus int
	}{
		// Rotas internas expostas antes da correção (deal.go:25-26 e o
		// wildcard de signature.go:11-12).
		{"/api/v1/deal/internal/deals/abc", http.StatusNotFound},
		{"/api/v1/deal/internal/contracts/abc", http.StatusNotFound},
		{"/api/v1/deal/internal/documents/abc/content", http.StatusNotFound},
		{"/api/v1/signature/internal/envelopes/by-contract/abc", http.StatusNotFound},
		{"/api/v1/crm/internal/leads/abc", http.StatusNotFound},
		{"/api/v1/financial/internal/deals/abc/charges", http.StatusNotFound},
		{"/api/v1/comm/internal/channels", http.StatusNotFound},
		{"/api/v1/ai/internal/documents/extract", http.StatusNotFound},
		{"/api/v1/ops/internal/tickets", http.StatusNotFound},
		{"/api/v1/calendar/internal/appointments/abc", http.StatusNotFound},
		// Não pode ser contornado por caixa, barras duplicadas ou sufixo.
		{"/api/v1/deal/INTERNAL/deals/abc", http.StatusNotFound},
		{"/api/v1/deal//internal//deals/abc", http.StatusNotFound},
		{"/API/V1/DEAL/Internal/Deals/abc", http.StatusNotFound},
		// Rotas públicas legítimas seguem passando.
		{"/api/v1/deals/abc", http.StatusOK},
		{"/api/v1/deals", http.StatusOK},
		{"/api/v1/signature/envelopes", http.StatusOK},
		{"/api/v1/leads", http.StatusOK},
		{"/api/v1/properties", http.StatusOK},
		{"/api/v1/internals", http.StatusOK},      // não é o marcador
		{"/api/v1/x-internal", http.StatusOK},     // não é o marcador
		{"/api/v1/internal", http.StatusNotFound}, // segmento "internal" é sempre recusado
		{"/portal", http.StatusOK},
		// P1/A20 — caminhos retirados do produto continuam recusados na borda,
		// mesmo que alguém volte a registrá-los no serviço ou no proxy.
		{"/api/v1/portal/YWJj:ZGVm", http.StatusNotFound},
		{"/api/v1/portal/token-qualquer/checklist", http.StatusNotFound},
		{"/api/v1/portal", http.StatusNotFound},
		{"/API/V1/PORTAL/x", http.StatusNotFound},
		{"/api/v1/client-portal/deals/abc", http.StatusOK}, // superfície autenticada segue passando
		// P0/C10 (reauditoria de 2026-09-20) — CODIFICAÇÃO PERCENTUAL.
		//
		// A primeira correção comparava só `URL.EscapedPath()`, que preserva
		// `%69nternal`; o gin roteia pelo path decodificado e o ReverseProxy
		// reencaminha a forma escapada, que o serviço decodifica de novo. Estes
		// casos falhavam ANTES (o request era servido) e são o ponto cego que
		// deixou o bypass passar por duas revisões.
		{"/api/v1/signature/%69nternal/envelopes/by-contract/abc", http.StatusNotFound},
		{"/api/v1/auth/%69nternal/accounts/ensure-client", http.StatusNotFound},
		{"/api/v1/deal/%69nternal/deals/abc", http.StatusNotFound},
		{"/api/v1/%69nternal/deals/abc", http.StatusNotFound},
		// Qualquer byte do marcador, não só o "i" inicial.
		{"/api/v1/signature/i%6Eternal/envelopes/abc", http.StatusNotFound},
		{"/api/v1/signature/in%74ernal/envelopes/abc", http.StatusNotFound},
		{"/api/v1/signature/inter%6Eal/envelopes/abc", http.StatusNotFound},
		{"/api/v1/signature/internal%2Fenvelopes/abc", http.StatusNotFound},
		// Segmento inteiro escapado, e variações de caixa junto da codificação.
		{"/api/v1/signature/%69%6E%74%65%72%6E%61%6C/envelopes/abc", http.StatusNotFound},
		{"/API/V1/SIGNATURE/%49NTERNAL/envelopes/abc", http.StatusNotFound},
		// Codificação dupla: `%2569` decodifica para `%69` e só depois para `i`.
		// Não alcança a rota no upstream (ele decodifica uma vez só), mas a
		// denylist não deve depender disso.
		{"/api/v1/signature/%2569nternal/envelopes/abc", http.StatusNotFound},
		// O mesmo para a superfície retirada.
		{"/api/v1/%70ortal/YWJj:ZGVm", http.StatusNotFound},
		{"/api/v1/p%6Frtal/token", http.StatusNotFound},
		// Controle: caminho público que contém escape NÃO relacionado segue
		// passando — a denylist não pode virar um bloqueio geral de `%`.
		{"/api/v1/deals/abc%20def", http.StatusOK},
		{"/api/v1/properties?q=casa%20nova", http.StatusOK},
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			r := gin.New()
			r.Use(RejectInternalPaths())
			r.NoRoute(func(c *gin.Context) { c.Status(http.StatusOK) })
			r.Any("/*any", func(c *gin.Context) { c.Status(http.StatusOK) })

			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			r.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("path %q: status = %d, want %d", tc.path, w.Code, tc.wantStatus)
			}
		})
	}
}
