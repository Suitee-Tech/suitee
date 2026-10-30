package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/toca-calendar/internal/services"
)

// P1/A8 — toca-calendar: 11 rotas, NENHUMA com verificação de permissão.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A8) registrou o serviço assim: "criar/ler/editar/excluir **qualquer**
// compromisso" por qualquer autenticado não-`cliente` — inclusive `marketing`,
// que a matriz documenta sem acesso a deals.
//
// A agenda é operada pelo contexto de negócio/esteira (`module:crm` no cliente,
// `CalendarContext` chamando `/appointments`), então o mapa agora é o do catálogo
// para deals: ver → `deals:read`, escrever → `deals:write`.
//
// O router é o real (`handlers.SetupRouter`) e o provider é o real
// (`auth.NewCorePermissionProvider`) alimentado por um toca-core dublê.

type permissionsServer struct {
	mu          sync.Mutex
	permissions []string
}

func (p *permissionsServer) set(perms ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.permissions = perms
}

func (p *permissionsServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		perms := append([]string(nil), p.permissions...)
		p.mu.Unlock()
		if r.URL.Path != "/api/v1/me/permissions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"functional_role": "corretor",
			"profile":         "member",
			"permissions":     perms,
		})
	}))
}

// appointmentsGuardRouter monta o router real com o provider real apontando para
// o core dublê.
func appointmentsGuardRouter(t *testing.T, jwksURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwksURL
	return SetupRouter(cfg, services.NewAppointmentService(&recordingAppointmentRepo{}, nil))
}

func doGuardRequest(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Sem `deals:write`, criar/editar/excluir compromisso é 403 (o handler não roda).
func TestCalendarRoutes_MutationsRequireDealsWrite(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newCalendarTestJWKS(t)
	defer jwks.server.Close()
	router := appointmentsGuardRouter(t, jwks.server.URL)
	core.set("deals:read") // só leitura
	token := jwks.token(t, "tenant-1")

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/api/v1/appointments", `{"title":"Visita","date":"2099-10-20","time":"09:00","end_time":"10:00","type":"Visita","priority":"Media"}`},
		{http.MethodPut, "/api/v1/appointments/a1", `{"title":"Visita"}`},
		{http.MethodDelete, "/api/v1/appointments/a1", ""},
	} {
		w := doGuardRequest(router, tc.method, tc.path, tc.body, token)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s sem `deals:write` respondeu %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// Sem `deals:read`, ler a agenda é 403.
func TestCalendarRoutes_ReadsRequireDealsRead(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newCalendarTestJWKS(t)
	defer jwks.server.Close()
	router := appointmentsGuardRouter(t, jwks.server.URL)
	core.set() // nenhuma permissão
	token := jwks.token(t, "tenant-1")

	for _, path := range []string{"/api/v1/appointments", "/api/v1/appointments/a1"} {
		w := doGuardRequest(router, http.MethodGet, path, "", token)
		if w.Code != http.StatusForbidden {
			t.Fatalf("GET %s sem `deals:read` respondeu %d: %s", path, w.Code, w.Body.String())
		}
	}
}

// A decisão acompanha a matriz do tenant: com a permissão, o guarda deixa passar
// (o handler responde o que puder com o dublê).
func TestCalendarRoutes_GuardFollowsTenantMatrix(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newCalendarTestJWKS(t)
	defer jwks.server.Close()
	router := appointmentsGuardRouter(t, jwks.server.URL)
	token := jwks.token(t, "tenant-1")

	core.set()
	if w := doGuardRequest(router, http.MethodGet, "/api/v1/appointments", "", token); w.Code != http.StatusForbidden {
		t.Fatalf("sem permissão deveria ser 403, veio %d", w.Code)
	}

	core.set("deals:read")
	if w := doGuardRequest(router, http.MethodGet, "/api/v1/appointments", "", token); w.Code == http.StatusForbidden {
		t.Fatalf("com `deals:read` o guarda não deveria recusar: %s", w.Body.String())
	}

	core.set("deals:read", "deals:write")
	w := doGuardRequest(router, http.MethodPost, "/api/v1/appointments",
		`{"title":"Visita","date":"2099-10-20","time":"09:00","end_time":"10:00","type":"Visita","priority":"Media"}`, token)
	if w.Code == http.StatusForbidden {
		t.Fatalf("com `deals:write` o guarda não deveria recusar: %s", w.Body.String())
	}
}
