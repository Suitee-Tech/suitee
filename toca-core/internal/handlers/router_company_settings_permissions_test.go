package handlers

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca-core/internal/app"
	"github.com/oginoino/toca-core/internal/domain/core/settings"
	service "github.com/oginoino/toca-core/internal/services"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
)

// P0 / A1 — as seis leituras de /company-settings não tinham NENHUMA guarda de
// permissão, enquanto as escritas correspondentes tinham. Este teste sobe o
// router real (SetupRouter) e prova duas coisas por rota:
//
//  1. sem a permissão exigida => 403 (antes: 200 com o segredo/matriz no corpo);
//  2. com a permissão exigida => 200 (a guarda não quebrou a rota legítima).
//
// O request usa a credencial interna (INTERNAL_API_KEY + X-Tenant-Id), que
// entra no middleware autenticado com functional_role "service" — nunca root.
// Assim o teste mede exatamente o RequirePermission, sem precisar forjar JWT.

const (
	internalKeyTest = "test-internal-key"
	tenantTeste     = "11111111-1111-1111-1111-111111111111"
)

// --- fakes adicionais (os de webhook/api-key/matriz ficam no arquivo de
// isolamento de tenant, mesmo pacote) ---------------------------------------

type fakeCompanyRepo struct{}

func (fakeCompanyRepo) GetByTenantID(context.Context, string) (*settings.CompanySettings, error) {
	return nil, nil
}
func (fakeCompanyRepo) Upsert(context.Context, *settings.CompanySettings) error { return nil }

type fakeAppearanceRepo struct{}

func (fakeAppearanceRepo) GetByTenantID(context.Context, string) (*settings.AppearanceSettings, error) {
	return nil, nil
}
func (fakeAppearanceRepo) Upsert(context.Context, *settings.AppearanceSettings) error { return nil }

type fakeNotificationRepo struct{}

func (fakeNotificationRepo) GetByTenantID(context.Context, string) (*settings.NotificationSettings, error) {
	return nil, nil
}
func (fakeNotificationRepo) Upsert(context.Context, *settings.NotificationSettings) error { return nil }

// permissionMatrix monta a matriz do tenant a partir da matriz default e
// concede as permissões pedidas ao papel "service" — usado nos casos de
// NEGAÇÃO do chamador interno (T-10: chave interna nunca passa por rota de
// permissão de usuário, mesmo com o grant).
func permissionMatrix(granted ...string) *settings.RolePermissions {
	return permissionMatrixForRole("service", granted...)
}

// permissionMatrixForRole concede as permissões ao papel indicado — usado
// nos casos de PERMISSÃO do chamador usuário (JWT com functional_role).
func permissionMatrixForRole(role string, granted ...string) *settings.RolePermissions {
	perms := settings.BuildDefaultMatrix()
	for i := range perms {
		for _, g := range granted {
			if perms[i].ID == g {
				perms[i].Roles[role] = true
			}
		}
	}
	return &settings.RolePermissions{
		TenantID:    tenantTeste,
		DefaultRole: "corretor",
		Permissions: perms,
	}
}

func newRouterForTest(t *testing.T, matrix *settings.RolePermissions) *gin.Engine {
	t.Helper()
	svc := service.NewSettingsService(
		fakeCompanyRepo{},
		fakeAppearanceRepo{},
		&fakePermissionsRepo{matrix: matrix},
		fakeNotificationRepo{},
		newFakeWebhookRepo(settings.WebhookEndpoint{
			ID: "wh-teste", TenantID: tenantTeste, URL: "https://a.example/hook", Secret: "whsec_nao_pode_vazar",
		}),
		newFakeAPIKeyRepo(settings.APIKey{ID: "key-teste", TenantID: tenantTeste, Name: "chave", IsActive: true}),
		nil,
	)
	cfg := &app.Config{
		BaseConfig: config.BaseConfig{
			AppMode:   "test",
			JWT:       config.JWTConfig{SupabaseJWKSURL: ""},
			RateLimit: config.RateLimitConfig{RPS: 1000, Burst: 1000},
		},
	}
	return SetupRouter(cfg, nil, nil, svc, nil, nil)
}

// testJWKSForCompanySettings serve o JWKS local e inicializa o validador —
// permite exercitar o caminho de USUÁRIO (JWT) do RequirePermission, que o
// T-10 separou do caminho de chave interna.
type testJWKSForCompanySettings struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

func initCompanySettingsJWKS(t *testing.T) *testJWKSForCompanySettings {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	j := &testJWKSForCompanySettings{key: key, kid: "core-settings-test-kid"}
	payload := map[string]any{"keys": []map[string]any{{
		"kty": "RSA", "kid": j.kid, "use": "sig", "alg": "RS256",
		"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	j.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	t.Cleanup(j.server.Close)
	if err := auth.InitJWKS(j.server.URL); err != nil {
		t.Fatalf("InitJWKS: %v", err)
	}
	return j
}

func signCompanySettingsUser(t *testing.T, j *testJWKSForCompanySettings, functionalRole string) string {
	t.Helper()
	claims := auth.Claims{
		AccountID:      "user-settings-1",
		Email:          "user@example.com",
		Name:           "Usuário de Teste",
		Role:           "member",
		Permission:     "member",
		FunctionalRole: functionalRole,
		Type:           "company",
		TenantID:       tenantTeste,
		TokenType:      auth.TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user-settings-1",
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

// doUserGet dispara a rota como USUÁRIO (JWT) com o functional_role pedido.
func doUserGet(t *testing.T, r *gin.Engine, path, functionalRole string) *httptest.ResponseRecorder {
	t.Helper()
	j := initCompanySettingsJWKS(t)
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+signCompanySettingsUser(t, j, functionalRole))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func doInternalGet(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Internal-Key", internalKeyTest)
	req.Header.Set("X-Tenant-Id", tenantTeste)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestCompanySettingsReads_ExigemPermissao(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", internalKeyTest)

	// Somente as leituras que expõem SEGREDO ou o MAPA DE AUTORIZAÇÃO.
	//
	// `GET ""` (dados gerais) e `GET /appearance` são legíveis por qualquer
	// membro de propósito: o ThemeContext do frontend
	// (toka/src/shared/context/ThemeContext.tsx:207-208) carrega ambos para
	// TODO usuário autenticado para aplicar o white-label. Guardá-los apagaria
	// logo/cores para corretor/financeiro/auditor — regressão funcional que a
	// revisão não pede. Cobertos pelo teste abaixo.
	cases := []struct {
		nome       string
		path       string
		permission string
	}{
		{"matriz de permissões", "/api/v1/company-settings/permissions", "settings:manage_permissions"},
		{"notifications", "/api/v1/company-settings/notifications", "settings:manage_notifications"},
		{"webhooks (expõe o secret)", "/api/v1/company-settings/webhooks", "settings:manage_integrations"},
		{"api keys", "/api/v1/company-settings/api-keys", "settings:manage_integrations"},
	}

	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			// (1) usuário (corretor) sem a permissão => 403 pelo provider
			// (corretor NÃO tem settings:manage_* por padrão na matriz).
			rSemPermissao := newRouterForTest(t, permissionMatrixForRole("corretor"))
			rec := doUserGet(t, rSemPermissao, tc.path, "corretor")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("LEITURA SEM GUARDA: GET %s sem %q devolveu %d: %s",
					tc.path, tc.permission, rec.Code, rec.Body.String())
			}

			// (2) usuário (corretor) com a permissão correta => 200.
			rComPermissao := newRouterForTest(t, permissionMatrixForRole("corretor", tc.permission))
			rec = doUserGet(t, rComPermissao, tc.path, "corretor")
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s com %q deveria ser 200, veio %d: %s",
					tc.path, tc.permission, rec.Code, rec.Body.String())
			}

			// (3) T-10: a chave interna segue PROIBIDA nas rotas de permissão
			// de usuário mesmo quando o papel "service" tem o grant.
			rInterno := newRouterForTest(t, permissionMatrix(tc.permission))
			rec = doInternalGet(t, rInterno, tc.path)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("T-10: GET %s com X-Internal-Key e grant ao papel service deveria ser 403, veio %d: %s",
					tc.path, rec.Code, rec.Body.String())
			}
		})
	}
}

// A permissão exigida tem de ser ESPECÍFICA: ter settings:manage_general não
// pode abrir a matriz de permissões nem a listagem de integrações.
func TestCompanySettingsReads_NaoAceitamPermissaoErrada(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", internalKeyTest)

	r := newRouterForTest(t, permissionMatrixForRole("corretor", "settings:manage_general"))

	// `/appearance` saiu da lista: é branding legível por membro (ver
	// TestCompanySettingsBranding_AcessivelPorMembro).
	for _, path := range []string{
		"/api/v1/company-settings/permissions",
		"/api/v1/company-settings/webhooks",
		"/api/v1/company-settings/api-keys",
		"/api/v1/company-settings/notifications",
	} {
		rec := doUserGet(t, r, path, "corretor")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("GET %s aceitou settings:manage_general como credencial suficiente (%d): %s",
				path, rec.Code, rec.Body.String())
		}
	}
}

// Mesmo com a permissão de integrações, a resposta da listagem não pode
// carregar o signing secret (defesa em profundidade do P0/A1).
func TestListarWebhooks_ComPermissaoNaoVazaSecret(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", internalKeyTest)

	r := newRouterForTest(t, permissionMatrixForRole("corretor", "settings:manage_integrations"))
	rec := doUserGet(t, r, "/api/v1/company-settings/webhooks", "corretor")

	if rec.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d: %s", rec.Code, rec.Body.String())
	}
	var lista []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &lista); err != nil {
		t.Fatalf("resposta inválida: %v (%s)", err, rec.Body.String())
	}
	if len(lista) != 1 {
		t.Fatalf("esperava 1 webhook, veio %d: %s", len(lista), rec.Body.String())
	}
	if _, existe := lista[0]["secret"]; existe {
		t.Fatalf("o secret não pode ser serializado: %s", rec.Body.String())
	}
}

// TestCompanySettingsBranding_AcessivelPorMembro garante que os dois GETs de
// branding NÃO regridem para quem não tem `settings:manage_*`: o tema
// white-label é aplicado a todos os usuários da empresa.
func TestCompanySettingsBranding_AcessivelPorMembro(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", internalKeyTest)

	r := newRouterForTest(t, permissionMatrix())

	for _, path := range []string{"/api/v1/company-settings", "/api/v1/company-settings/appearance"} {
		rec := doInternalGet(t, r, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("REGRESSÃO DE WHITE-LABEL: GET %s sem permissão de settings devolveu %d: %s",
				path, rec.Code, rec.Body.String())
		}
	}
}
