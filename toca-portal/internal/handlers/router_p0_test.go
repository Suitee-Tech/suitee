package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

// p0CoreStub finge o toca-core para o provider de permissão: responde
// /api/v1/me/permissions com a lista configurada. Com a lista vazia o provider
// RESPONDE (sem erro) e o middleware nega com 403 — que é a asserção de
// permission destes testes. Com o core inalcançável, o middleware responde 503
// fail-closed (semântica do incidente de agenda de 2026-09-21: falha de
// infraestrutura nunca vira "sem permissão").
type p0CoreStub struct {
	permissions []string
}

func (s *p0CoreStub) start(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/me/permissions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"functional_role": "operacoes",
			"profile":         "member",
			"permissions":     s.permissions,
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// P0 / A8 (C4) — enforcement de permission no router.
//
// O router real é montado com repos falsos. Um chamador com X-Internal-Key
// válido NÃO tem identidade de usuário: o RequirePermission nega com 403
// explícito ("internal service callers cannot use user permission routes" —
// T-10), sem chamar o provider — rotas de USUÁRIO nunca autorizam chave
// interna; as rotas internas usam RequireInternalKey. O 503 fail-closed do
// provider inalcançável vale para usuários reais e é coberto em
// pkg/auth/permission_provider_failure_test.go.
//
// Antes da correção estas mesmas requisições chegavam ao handler (200/201).

const p0RouterInternalKey = "p0-internal-key"

// p0RouterEngine monta o router com o core stub (permissões vazias → 403).
func p0RouterEngine(t *testing.T) *gin.Engine {
	t.Helper()
	stub := (&p0CoreStub{}).start(t)
	return p0RouterEngineWithCoreURL(t, stub.URL)
}

// p0RouterEngineWithCoreURL permite o caso de provider inalcançável (503).
func p0RouterEngineWithCoreURL(t *testing.T, coreURL string) *gin.Engine {
	t.Helper()
	p0InitValidation(t)
	t.Setenv("INTERNAL_API_KEY", p0RouterInternalKey)
	t.Setenv("CORE_SERVICE_URL", coreURL)

	integRepo := newFakeIntegrationRepo()
	p0SeedTwoTenants(integRepo)
	leadRepo := newFakeLeadRepo()

	integService := service.NewIntegrationService(integRepo, "https://portal.toca.test")
	feedService := service.NewFeedService(nil, "http://127.0.0.1:1", p0RouterInternalKey)
	webhookService := service.NewWebhookService(leadRepo, integRepo, nil, "", p0RouterInternalKey)
	syncService := service.NewSyncService(newFakeSyncRepo(), integRepo, feedService, nil)

	return SetupRouter(
		config.BaseConfig{},
		NewIntegrationHandler(integService),
		NewFeedHandler(integService, feedService),
		NewWebhookHandler(webhookService),
		NewSyncHandler(syncService),
		NewStatsHandler(integService, leadRepo),
	)
}

func p0RouterRequest(engine *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

// TestRouterP0_IntegrationRoutesRequirePermission cobre CRUD + sync.
func TestRouterP0_IntegrationRoutesRequirePermission(t *testing.T) {
	engine := p0RouterEngine(t)
	auth := map[string]string{
		"X-Internal-Key": p0RouterInternalKey,
		"X-Tenant-Id":    p0AttackerTenant,
	}

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"create", http.MethodPost, "/api/v1/integrations", `{"portal":"zap_imoveis","contact_name":"x","contact_email":"x@example.com","contact_phone":"1","sync_frequency":"6h","publication_type":"STANDARD"}`},
		{"list", http.MethodGet, "/api/v1/integrations", ""},
		{"get", http.MethodGet, "/api/v1/integrations/" + p0OwnIntegID, ""},
		{"update", http.MethodPut, "/api/v1/integrations/" + p0OwnIntegID, `{"contact_name":"x"}`},
		{"delete", http.MethodDelete, "/api/v1/integrations/" + p0OwnIntegID, ""},
		{"sync", http.MethodPost, "/api/v1/integrations/" + p0OwnIntegID + "/sync", ""},
		{"syncs", http.MethodGet, "/api/v1/integrations/" + p0OwnIntegID + "/syncs", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := p0RouterRequest(engine, tc.method, tc.path, tc.body, auth)
			if w.Code != http.StatusForbidden {
				t.Fatalf("SEM identidade de usuário: %s %s deveria ser 403 (chave interna não autoriza rota de usuário), veio %d (corpo=%s)",
					tc.method, tc.path, w.Code, w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "permission") {
				t.Fatalf("403 inesperado (não veio do RequirePermission): %s", w.Body.String())
			}
		})
	}
}

// TestRouterP0_IntegrationRoutesRequireAuth garante que o JWT continua sendo
// exigido antes da permission.
func TestRouterP0_IntegrationRoutesRequireAuth(t *testing.T) {
	engine := p0RouterEngine(t)

	w := p0RouterRequest(engine, http.MethodGet, "/api/v1/integrations", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("sem credencial: esperava 401, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
}

// TestRouterP0_PublicRoutesStayPublic é a trava de regressão do feed público
// (GET /api/v1/portals/feed/:token) e do webhook público: nenhum dos dois pode
// ganhar JWT/permission.
func TestRouterP0_PublicRoutesStayPublic(t *testing.T) {
	engine := p0RouterEngine(t)

	// Feed: sem credencial, token inexistente → 404 do handler ("Feed not
	// found"); se o JWT tivesse sido aplicado, seria 401.
	feed := p0RouterRequest(engine, http.MethodGet, "/api/v1/portals/feed/token-inexistente", "", nil)
	if feed.Code != http.StatusNotFound {
		t.Fatalf("feed público deveria responder 404 do handler, veio %d (corpo=%s)", feed.Code, feed.Body.String())
	}
	if !strings.Contains(feed.Body.String(), "Feed not found") {
		t.Fatalf("feed público respondeu outra coisa: %s", feed.Body.String())
	}

	// Webhook: sem credencial e sem assinatura → 401 do HMAC (não do JWT).
	hook := p0RouterRequest(engine, http.MethodPost, "/api/v1/portals/webhook/"+p0OwnIntegID, `{"email":"x@example.com"}`, nil)
	if hook.Code != http.StatusUnauthorized {
		t.Fatalf("webhook público sem assinatura: esperava 401, veio %d (corpo=%s)", hook.Code, hook.Body.String())
	}
	if !strings.Contains(hook.Body.String(), "webhook signature") {
		t.Fatalf("webhook público não chegou ao handler (401 de outra origem): %s", hook.Body.String())
	}
}

// P1/A8 — leitura dos leads de uma integração e agregados do portal sem guarda.
//
// As três rotas ficaram de fora do gate do C4 com a justificativa de que
// "bloqueariam telas de papéis não-admin". A superfície foi conferida: o único
// consumidor de `/integrations/:id/leads` é a aba de integrações das
// configurações, que o cliente já gateia por `settings:manage_integrations` (a
// mesma permissão do resto do grupo); `/portal-stats/*` não tem consumidor no
// frontend. Agora as três exigem a permissão de gestão de integrações — e o
// chamador sem permissão recebe 403 em vez de 200.
func TestPortalStatsAndIntegrationLeadsRequireManageIntegrations(t *testing.T) {
	engine := p0RouterEngine(t)

	cases := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/integrations/integ-tenant-a/leads"},
		{http.MethodGet, "/api/v1/portal-stats/dashboard"},
		{http.MethodGet, "/api/v1/portal-stats/leads"},
	}
	for _, tc := range cases {
		w := p0RouterRequest(engine, tc.method, tc.path, "", map[string]string{
			"X-Internal-Key": p0RouterInternalKey,
			"X-Tenant-Id":    "tenant-a",
		})
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s sem identidade de usuário respondeu %d (esperado 403): %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
