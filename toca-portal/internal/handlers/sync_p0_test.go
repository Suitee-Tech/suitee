package handlers

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

// P0 / C4 — caminhos de sync e não-vazamento de erro de driver.

func newP0SyncEngine(t *testing.T, integRepo *fakeIntegrationRepo, syncRepo *fakeSyncRepo) *gin.Engine {
	t.Helper()
	p0InitValidation(t)
	h := NewSyncHandler(service.NewSyncService(syncRepo, integRepo, nil, nil))
	engine := gin.New()
	g := engine.Group("/api/v1/integrations")
	g.POST("/:id/sync", h.TriggerSync)
	g.GET("/:id/syncs", h.ListSyncs)
	return engine
}

func TestSyncP0_CrossTenantIsNotFound(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	syncRepo := newFakeSyncRepo()
	p0SeedTwoTenants(integRepo)
	engine := newP0SyncEngine(t, integRepo, syncRepo)

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{"trigger", http.MethodPost, "/api/v1/integrations/" + p0VictimIntegID + "/sync"},
		{"list", http.MethodGet, "/api/v1/integrations/" + p0VictimIntegID + "/syncs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := p0TenantRequest(t, engine, tc.method, tc.path, p0AttackerTenant, "")
			if w.Code != http.StatusNotFound {
				t.Fatalf("SYNC CROSS-TENANT (%s): tenant %q alcançou a integração do tenant %q → HTTP %d (corpo=%s)",
					tc.name, p0AttackerTenant, p0VictimTenant, w.Code, w.Body.String())
			}
			if w.Code == http.StatusForbidden {
				t.Fatal("403 revelaria a existência do recurso em outro tenant; esperado 404")
			}
			if strings.Contains(w.Body.String(), "sql:") || strings.Contains(w.Body.String(), "pgx") {
				t.Fatalf("SYNC CROSS-TENANT (%s): erro de driver vazou: %s", tc.name, w.Body.String())
			}
		})
	}

	// O dono continua conseguindo disparar/ler o próprio sync.
	own := p0TenantRequest(t, engine, http.MethodGet, "/api/v1/integrations/"+p0OwnIntegID+"/syncs", p0AttackerTenant, "")
	if own.Code != http.StatusOK {
		t.Fatalf("listagem legítima de syncs falhou: HTTP %d (corpo=%s)", own.Code, own.Body.String())
	}
}

func TestSyncP0_SyncRepoNeverSeesForeignTenant(t *testing.T) {
	integRepo := newFakeIntegrationRepo()
	syncRepo := newFakeSyncRepo()
	p0SeedTwoTenants(integRepo)
	engine := newP0SyncEngine(t, integRepo, syncRepo)

	_ = p0TenantRequest(t, engine, http.MethodGet, "/api/v1/integrations/"+p0VictimIntegID+"/syncs", p0AttackerTenant, "")

	if syncRepo.byTenant[p0VictimTenant] != 0 {
		t.Fatalf("repositório de sync foi consultado no tenant da vítima (%d vezes)", syncRepo.byTenant[p0VictimTenant])
	}
}

func TestIntegrationP0_DriverErrorIsNotLeaked(t *testing.T) {
	repo := newFakeIntegrationRepo()
	p0SeedTwoTenants(repo)
	// Erro cru de driver, do jeito que o handler antigo devolvia em err.Error().
	repo.failWith = errors.New("load integration: sql: no rows in result set (pgx: connection reset by peer)")
	engine := newP0IntegrationEngine(t, repo)

	for _, tc := range []struct {
		method string
		body   string
	}{
		{http.MethodGet, ""},
		{http.MethodPut, `{"contact_name":"x"}`},
		{http.MethodDelete, ""},
	} {
		w := p0TenantRequest(t, engine, tc.method, "/api/v1/integrations/"+p0VictimIntegID, p0AttackerTenant, tc.body)
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("%s: erro inesperado de banco deveria virar 500 genérico, veio %d", tc.method, w.Code)
		}
		body := w.Body.String()
		for _, forbidden := range []string{"sql:", "pgx", "no rows in result set", "connection reset"} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s: erro de driver vazou para o cliente (%q): %s", tc.method, forbidden, body)
			}
		}
	}
}
