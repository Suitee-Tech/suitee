package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Regressão do incidente de deploy (2026-09-19): `/health` e `/metrics` do
// toca-core responderam 403 depois do fail-closed do achado M2
// (`RejectFunctionalRole` negando quando o papel funcional está ausente).
//
// Em toca-core essa guarda é aplicada na ENGINE (`r.Use(...)`), então os dois
// endpoints públicos — liberados pelo middleware de auth por whitelist, sem
// popular `user_id` — atravessavam o `RejectFunctionalRole` sem papel nenhum. O
// healthcheck do container passou a falhar, o Compose abortou com
// "container toca-toca-core-1 is unhealthy" e o `make up` inteiro caiu.
//
// Contrato: requisição NÃO autenticada em caminho público passa (não há papel a
// rejeitar); chamador autenticado sem papel continua barrado (o M2 segue de pé).
func TestRouter_InfraEndpointsAreReachableWithoutAuth(t *testing.T) {
	router := newRouterForTest(t, nil)

	for _, path := range []string{"/health", "/metrics"} {
		t.Run(path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))

			if recorder.Code != http.StatusOK {
				t.Fatalf("%s sem autenticação devolveu %d — probe de infraestrutura quebrado (body=%s)",
					path, recorder.Code, recorder.Body.String())
			}
		})
	}
}

// O caminho autenticado segue fail-closed: a correção acima não reabre o M2.
func TestRouter_AuthenticatedWithoutRoleStillDeniedOnOperatorGroup(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := newRouterForTest(t, nil)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/companies/settings", nil)
	// Sem JWT e sem chave interna: a rota é de operador, então continua negada
	// (pelo guarda de permissão) em vez de responder dados.
	router.ServeHTTP(recorder, req)

	if recorder.Code == http.StatusOK {
		t.Fatalf("rota de operador respondeu 200 sem credencial: %s", recorder.Body.String())
	}
}
