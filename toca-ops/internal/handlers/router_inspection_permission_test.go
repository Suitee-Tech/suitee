package handlers_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/toca-ops/internal/handlers"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

type testJWKS struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

func newTestJWKS(t *testing.T) *testJWKS {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	j := &testJWKS{key: key, kid: "toca-ops-test-kid"}

	payload := map[string]any{"keys": []map[string]any{{
		"kty": "RSA",
		"kid": j.kid,
		"use": "sig",
		"alg": "RS256",
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	j.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	return j
}

// token gera um access token RS256 no mesmo formato que o auth.Middleware
// valida (Claims do pkg/auth), com o tenant e o papel funcional do chamador.
func (j *testJWKS) token(t *testing.T, tenantID, functionalRole string) string {
	t.Helper()
	claims := auth.Claims{
		AccountID:      "acc-1",
		Email:          "operador@example.com",
		Name:           "Operador",
		Role:           "member",
		Permission:     "member", // não-root: a matriz de permissões é consultada
		FunctionalRole: functionalRole,
		Type:           "company",
		TenantID:       tenantID,
		TokenType:      auth.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "acc-1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = j.kid
	signed, err := tok.SignedString(j.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// permissionsServer devolve GET /api/v1/me/permissions como o toca-core faria.
type permissionsServer struct {
	mu          sync.Mutex
	permissions []string
	requests    int
}

func (p *permissionsServer) set(perms ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.permissions = perms
}

func (p *permissionsServer) hitCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.requests
}

func (p *permissionsServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.requests++
		perms := append([]string(nil), p.permissions...)
		p.mu.Unlock()
		if r.URL.Path != "/api/v1/me/permissions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"functional_role": "operacoes",
			"profile":         "member",
			"permissions":     perms,
		})
	}))
}

// C8/A8: as rotas de vistoria não tinham NENHUM RequirePermission — qualquer
// autenticado não-`cliente` lia e regravava vistorias. Este teste sobe o router
// real (handlers.SetupRouter) com JWKS e permission provider reais e prova que
// `rentals:inspect` é exigido e que, uma vez autorizado, o portão de tenant
// continua valendo.
func TestInspectionRoutes_RequireRentalsInspectPermission(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "") // sem cache: cada request consulta o provider

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwks.server.URL

	victim := seedInspection("tenant-B")
	own := seedInspection("tenant-A")
	repo := newTenantScopedInspectionRepo(victim, own)

	router := handlers.SetupRouter(
		cfg,
		services.NewMaintenanceService(nil, nil),
		services.NewKeyService(nil),
		services.NewInspectionService(repo, nil).WithPDFRenderer(stubPDFRenderer{}).WithStorage(stubStorage{}),
		services.NewTicketService(nil, nil),
		services.NewSupplierService(nil),
	)

	token := jwks.token(t, "tenant-A", "operacoes")

	// (1) Sem `rentals:inspect`: o guard rejeita com 403 e o handler não roda.
	core.set() // nenhuma permissão
	w := doAuthRequest(router, http.MethodGet, "/api/v1/inspections/"+own.ID, token)
	assert.Equal(t, http.StatusForbidden, w.Code, "rota de vistoria sem a permissão do catálogo deve ser 403")
	assert.GreaterOrEqual(t, core.hitCount(), 1, "o provider de permissões precisa ter sido consultado")

	// (2) Com `rentals:inspect`: o guard passa; o isolamento de tenant responde
	// 404 para vistoria de outro tenant (em vez de 200 vazando o registro).
	core.set("rentals:inspect")
	w = doAuthRequest(router, http.MethodGet, "/api/v1/inspections/"+victim.ID, token)
	assert.Equal(t, http.StatusNotFound, w.Code, "autorizado, mas o registro é de outro tenant")
	assert.NotContains(t, w.Body.String(), "prop-tenant-B")

	// (3) Com `rentals:inspect` e vistoria do próprio tenant: 200 (sem regressão).
	w = doAuthRequest(router, http.MethodGet, "/api/v1/inspections/"+own.ID, token)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "prop-tenant-A")
}

// A escrita mais perigosa do C8 (generate-laudo) também precisa do guard.
func TestGenerateLaudoRoute_RequiresRentalsInspectPermission(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwks.server.URL

	victim := seedInspection("tenant-B")
	originalURL := victim.ReportURL
	repo := newTenantScopedInspectionRepo(victim)

	router := handlers.SetupRouter(
		cfg,
		services.NewMaintenanceService(nil, nil),
		services.NewKeyService(nil),
		services.NewInspectionService(repo, nil).WithPDFRenderer(stubPDFRenderer{}).WithStorage(stubStorage{}),
		services.NewTicketService(nil, nil),
		services.NewSupplierService(nil),
	)

	token := jwks.token(t, "tenant-A", "operacoes")
	core.set() // sem permissão

	w := doAuthRequest(router, http.MethodPost, "/api/v1/inspections/"+victim.ID+"/generate-laudo", token)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Zero(t, repo.updateCount(), "requisição sem permissão não pode gravar laudo")
	if stored := repo.stored(victim.ID); assert.NotNil(t, stored) {
		assert.Equal(t, originalURL, stored.ReportURL)
	}
}

func doAuthRequest(r *gin.Engine, method, path, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// P1/A8 — manutenção, chaves e fornecedores sem verificação de permissão.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A8) registrou `toca-ops` com 41 rotas e apenas 6 com permissão (só as de
// vistoria): manutenção, chaves e fornecedores ficavam atrás de "autenticado e
// não-`cliente`". Com isso, `marketing`/`financeiro` criavam e apagavam chamado de
// manutenção e cadastro de prestador.
//
// Os três são a superfície operacional de Locação (a tela `/manutencao` exige
// `rentals:inspect` e tem as abas Manutenção e Vistoria), então o guard é o mesmo
// do grupo de vistorias — não muda quem opera hoje, fecha o "qualquer
// autenticado".
func TestMaintenanceKeysSuppliersRoutes_RequireRentalsInspect(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwks.server.URL

	router := handlers.SetupRouter(
		cfg,
		services.NewMaintenanceService(&recordingMaintenanceRepo{}, nil),
		services.NewKeyService(nil),
		services.NewInspectionService(nil, nil),
		services.NewTicketService(nil, nil),
		services.NewSupplierService(nil),
	)
	token := jwks.token(t, "tenant-1", "operacoes")

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/maintenance", ""},
		{http.MethodPost, "/api/v1/maintenance", `{"type":"reparo","category":"hidraulica","property_id":"p1","requester":"Maria","profile":"inquilino","priority":"alta"}`},
		{http.MethodDelete, "/api/v1/maintenance/t1", ""},
		{http.MethodGet, "/api/v1/keys", ""},
		{http.MethodPost, "/api/v1/keys", `{"property_id":"p1","code":"K-01","location":"Portaria"}`},
		{http.MethodPost, "/api/v1/keys/k1/checkout", `{"person":"Maria"}`},
		{http.MethodGet, "/api/v1/suppliers", ""},
		{http.MethodPost, "/api/v1/suppliers", `{"name":"Elétrica Silva"}`},
		{http.MethodDelete, "/api/v1/suppliers/s1", ""},
	}

	// (1) Sem `rentals:inspect`: 403 em todas.
	core.set()
	for _, tc := range routes {
		w := doInternalRequest(t, router, tc.method, tc.path, tc.body,
			map[string]string{"Authorization": "Bearer " + token})
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s sem `rentals:inspect` respondeu %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	// (2) Com `rentals:inspect`: o guarda deixa passar (o handler responde o que
	// puder com repositórios dublês/nulos — o que importa é não ser 401/403).
	core.set("rentals:inspect")
	for _, tc := range routes {
		w := doInternalRequest(t, router, tc.method, tc.path, tc.body,
			map[string]string{"Authorization": "Bearer " + token})
		if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
			t.Fatalf("%s %s com `rentals:inspect` foi barrado no guarda: %d %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}
