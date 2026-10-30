package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

// P0 / C4 — IDOR nas integrações do portal.
//
// O `id` vem da URL e o tenant do chamador tem de ser aplicado na busca, na
// atualização e na exclusão. Divergência de tenant = 404 (nunca 403), para não
// revelar que o recurso existe em outro tenant.

const (
	p0AttackerTenant = "tenant-atacante"
	p0VictimTenant   = "tenant-vitima"
	p0VictimIntegID  = "11111111-1111-1111-1111-111111111111"
	p0OwnIntegID     = "22222222-2222-2222-2222-222222222222"
)

func p0InitValidation(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if err := validation.InitGin(); err != nil {
		t.Fatalf("validation.InitGin: %v", err)
	}
}

func newP0IntegrationEngine(t *testing.T, repo *fakeIntegrationRepo) *gin.Engine {
	t.Helper()
	p0InitValidation(t)
	h := NewIntegrationHandler(service.NewIntegrationService(repo, "https://portal.toca.test"))
	engine := gin.New()
	g := engine.Group("/api/v1/integrations")
	g.GET("/:id", h.GetByID)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	return engine
}

// p0TenantRequest dispara a requisição com o tenant do chamador no request
// context — exatamente de onde o handler deve tirá-lo (pkg/context).
func p0TenantRequest(t *testing.T, engine *gin.Engine, method, path, tenantID, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(appcontext.WithTenantID(req.Context(), tenantID))
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func p0SeedTwoTenants(repo *fakeIntegrationRepo) {
	repo.seed(&integration.PortalIntegration{
		ID:           p0VictimIntegID,
		TenantID:     p0VictimTenant,
		Portal:       integration.PortalZAP,
		Status:       integration.IntegrationActive,
		ContactName:  "Titular Vitima",
		ContactEmail: "vitima@example.com",
		ContactPhone: "11988887777",
		FeedToken:    "feed-token-da-vitima",
	})
	repo.seed(&integration.PortalIntegration{
		ID:           p0OwnIntegID,
		TenantID:     p0AttackerTenant,
		Portal:       integration.PortalZAP,
		Status:       integration.IntegrationActive,
		ContactName:  "Titular Atacante",
		ContactEmail: "atacante@example.com",
		ContactPhone: "11977776666",
		FeedToken:    "feed-token-do-atacante",
	})
}

func TestIntegrationP0_GetByIDCrossTenantIsNotFound(t *testing.T) {
	repo := newFakeIntegrationRepo()
	p0SeedTwoTenants(repo)
	engine := newP0IntegrationEngine(t, repo)

	w := p0TenantRequest(t, engine, http.MethodGet, "/api/v1/integrations/"+p0VictimIntegID, p0AttackerTenant, "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("LEITURA CROSS-TENANT: tenant %q leu a integração %q do tenant %q → HTTP %d (corpo=%s)",
			p0AttackerTenant, p0VictimIntegID, p0VictimTenant, w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "vitima@example.com") || strings.Contains(w.Body.String(), "Titular Vitima") {
		t.Fatalf("LEITURA CROSS-TENANT: PII da vítima vazou no corpo: %s", w.Body.String())
	}

	// Sanidade: o dono continua enxergando a própria integração.
	own := p0TenantRequest(t, engine, http.MethodGet, "/api/v1/integrations/"+p0OwnIntegID, p0AttackerTenant, "")
	if own.Code != http.StatusOK {
		t.Fatalf("leitura legítima falhou: HTTP %d (corpo=%s)", own.Code, own.Body.String())
	}
}

func TestIntegrationP0_UpdateCrossTenantIsNotFoundAndDoesNotWrite(t *testing.T) {
	repo := newFakeIntegrationRepo()
	p0SeedTwoTenants(repo)
	engine := newP0IntegrationEngine(t, repo)

	w := p0TenantRequest(t, engine, http.MethodPut, "/api/v1/integrations/"+p0VictimIntegID, p0AttackerTenant,
		`{"contact_name":"invadido","contact_email":"invasor@example.com"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("ESCRITA CROSS-TENANT: tenant %q atualizou a integração do tenant %q → HTTP %d (corpo=%s)",
			p0AttackerTenant, p0VictimTenant, w.Code, w.Body.String())
	}
	victim := repo.raw(p0VictimIntegID)
	if victim == nil {
		t.Fatal("linha da vítima desapareceu")
	}
	if victim.ContactName != "Titular Vitima" || victim.ContactEmail != "vitima@example.com" {
		t.Fatalf("ESCRITA CROSS-TENANT: registro da vítima mudou → name=%q email=%q", victim.ContactName, victim.ContactEmail)
	}
	if victim.TenantID != p0VictimTenant {
		t.Fatalf("ESCRITA CROSS-TENANT: tenant da vítima mudou para %q", victim.TenantID)
	}
}

func TestIntegrationP0_DeleteCrossTenantIsNotFoundAndPreservesRow(t *testing.T) {
	repo := newFakeIntegrationRepo()
	p0SeedTwoTenants(repo)
	engine := newP0IntegrationEngine(t, repo)

	w := p0TenantRequest(t, engine, http.MethodDelete, "/api/v1/integrations/"+p0VictimIntegID, p0AttackerTenant, "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("EXCLUSÃO CROSS-TENANT: tenant %q apagou a integração do tenant %q → HTTP %d (corpo=%s)",
			p0AttackerTenant, p0VictimTenant, w.Code, w.Body.String())
	}
	if repo.isDeleted(p0VictimIntegID) {
		t.Fatalf("EXCLUSÃO CROSS-TENANT: integração %q foi soft-deleted por outro tenant", p0VictimIntegID)
	}
	if repo.raw(p0VictimIntegID) == nil {
		t.Fatal("linha da vítima desapareceu")
	}
}

// TestIntegrationP0_NoForbiddenOnCrossTenant garante que a resposta é 404 e
// nunca 403 — 403 revelaria a existência do recurso em outro tenant.
func TestIntegrationP0_NoForbiddenOnCrossTenant(t *testing.T) {
	repo := newFakeIntegrationRepo()
	p0SeedTwoTenants(repo)
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
		if w.Code == http.StatusForbidden {
			t.Fatalf("%s cross-tenant devolveu 403 (revela existência); esperado 404", tc.method)
		}
		// Erro genérico: nenhuma mensagem de driver SQL pode chegar ao cliente.
		if strings.Contains(w.Body.String(), "sql:") || strings.Contains(w.Body.String(), "pgx") {
			t.Fatalf("%s cross-tenant vazou erro de driver: %s", tc.method, w.Body.String())
		}
	}
}

// TestIntegrationP0_ListIsTenantScoped mantém a checagem de que a listagem já
// era (e continua) por tenant.
func TestIntegrationP0_ListIsTenantScoped(t *testing.T) {
	repo := newFakeIntegrationRepo()
	p0SeedTwoTenants(repo)
	engine := newP0IntegrationEngine(t, repo)

	h := NewIntegrationHandler(service.NewIntegrationService(repo, "https://portal.toca.test"))
	engine.GET("/api/v1/integrations", h.List)
	w := p0TenantRequest(t, engine, http.MethodGet, "/api/v1/integrations", p0AttackerTenant, "")

	if w.Code != http.StatusOK {
		t.Fatalf("listagem falhou: HTTP %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "vitima@example.com") {
		t.Fatalf("LISTAGEM CROSS-TENANT: PII de outro tenant no corpo: %s", w.Body.String())
	}
	var parsed struct {
		Data []integration.PortalIntegration `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &parsed); err != nil {
		t.Fatalf("resposta inesperada: %v (%s)", err, w.Body.String())
	}
	if len(parsed.Data) != 1 || parsed.Data[0].ID != p0OwnIntegID {
		t.Fatalf("listagem deveria trazer só a integração do próprio tenant, veio %+v", parsed.Data)
	}
}
