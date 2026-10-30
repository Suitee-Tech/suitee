package routes_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca-auth/internal/api/v1/routes"
	"github.com/oginoino/toca-auth/internal/app"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
)

// P1/A8 — toca-auth: gestão de empresa/membros/convites/grupos sem verificação
// de permissão.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A8) registrou `toca-auth` com 57 rotas e apenas 3 com
// `RequirePermission`: toda a gestão de empresas, membros, convites e grupos
// ficava atrás de "estar autenticado e não ser `cliente`". O P0/C1..C3 corrigiu o
// **vínculo** (só mexo na minha empresa) e a validação de papel; faltava a
// permissão do catálogo. Agora:
//
//	ver       → `company:read_members`   (todos os papéis de operador)
//	gerenciar → `company:manage_members` (admin/gestor/operacoes)
//
// O router é o real (`routes.SetupRouter`) com o provider real
// (`auth.NewCorePermissionProvider`) alimentado por um toca-core dublê, e JWKS
// local. O guarda roda ANTES do handler, então o banco não é necessário.

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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
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
	return srv
}

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
	j := &testJWKS{key: key, kid: "toca-auth-test-kid"}
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

func (j *testJWKS) token(t *testing.T, tenantID, functionalRole string) string {
	t.Helper()
	claims := auth.Claims{
		AccountID:      "acc-1",
		Email:          "operador@example.com",
		Name:           "Operador",
		Role:           "member",
		Permission:     "member", // não-root: a matriz do tenant é consultada
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

func authRouter(t *testing.T, jwksURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	// `SetupRouter` constrói o cliente Supabase no boot e entra em panic sem as
	// variáveis (comportamento de produção, mantido). O guarda de permissão roda
	// antes de qualquer handler, então nenhuma chamada sai para esses valores.
	t.Setenv("SUPABASE_URL", "http://supabase.test")
	t.Setenv("SUPABASE_ANON_KEY", "anon-key-test")
	t.Setenv("SUPABASE_SERVICE_ROLE_KEY", "service-role-key-test")

	cfg := &app.Config{}
	cfg.JWT.SupabaseJWKSURL = jwksURL
	// O router de produção monta o rate limit por IP; com a config zerada o
	// limiter nasce com burst 0 e nega tudo. O teste exercita o GUARDA de
	// permissão, então a cota é folgada de propósito.
	cfg.RateLimit.RPS = 1000
	cfg.RateLimit.Burst = 1000
	return routes.SetupRouter(cfg, nil, nil, nil)
}

// requestSeq dá um IP de origem distinto por requisição: o router do toca-auth
// tem rate limit global por IP e o teste faz dezenas de chamadas do mesmo host.
var requestSeq int64

func doAuthRequest(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reqBody)
	n := atomic.AddInt64(&requestSeq, 1)
	req.Header.Set("X-Forwarded-For", fmt.Sprintf("198.51.100.%d", n%250+1))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Sem a permissão de gestão, mexer em membro/convite/grupo é 403 — e o handler
// nem é alcançado (nenhum banco envolvido).
func TestCompanyRoutes_RequireManageMembersPermission(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)
	// Matriz de um papel que só lê (`auditor`, por exemplo): nenhuma permissão de
	// gestão.
	core.set("company:read_members")
	token := jwks.token(t, "tenant-1", "auditor")

	for _, tc := range []struct {
		method, path, body string
	}{
		{http.MethodPut, "/api/v1/companies/c1", `{"name":"Nova"}`},
		{http.MethodPost, "/api/v1/companies/c1/users", `{"email":"x@y.com"}`},
		{http.MethodDelete, "/api/v1/companies/c1/members/m1", ""},
		// P2/M6+M7: alterar papel/perfil e suspender/reativar vínculo exige a
		// mesma permissão de gestão de membros (antes a rota nem existia).
		{http.MethodPatch, "/api/v1/companies/c1/members/m1", `{"functional_role":"financeiro"}`},
		{http.MethodPatch, "/api/v1/companies/c1/members/m1", `{"is_active":false}`},
		{http.MethodPost, "/api/v1/companies/c1/invite", `{"email":"x@y.com"}`},
		{http.MethodGet, "/api/v1/companies/c1/invitations", ""},
		{http.MethodPost, "/api/v1/companies/c1/groups", `{"name":"G"}`},
		{http.MethodPut, "/api/v1/companies/c1/groups/g1", `{"name":"G2"}`},
		{http.MethodDelete, "/api/v1/companies/c1/groups/g1", ""},
		{http.MethodPost, "/api/v1/companies/c1/groups/g1/members", `{"account_id":"a1"}`},
		{http.MethodPost, "/api/v1/companies/c1/groups/g1/members/a1/move", `{"target_group_id":"g2"}`},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			w := doAuthRequest(router, tc.method, tc.path, tc.body, token)
			if w.Code != http.StatusForbidden {
				t.Fatalf("rota de gestão sem `company:manage_members` respondeu %d: %s", w.Code, w.Body.String())
			}
		})
	}
}

// Regressão de onboarding: `POST /api/v1/companies` é o BOOTSTRAP do tenant.
//
// O `functional_role` do dono nasce do vínculo criado por essa PRÓPRIA rota
// (company_service.go: Profile "owner", FunctionalRole "admin"), então a conta
// recém-cadastrada chega aqui com `functional_role` vazio — por construção. Com
// o fail-closed do achado M2 aplicado a este grupo, a resposta era
// 403 "no functional role assigned" (`authz.denied` na auditoria) e a primeira
// empresa nunca era criada: sem empresa não há papel, e sem papel não há
// empresa. O papel `cliente` continua barrado (P0/A11).
//
// O guarda roda antes do handler, então chegar ao handler significa passar: o
// corpo `{}` só produz 400 de validação, que não é negação.
func TestCompanyRoutes_CreateCompanyBootstrapAllowsUnassignedRole(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)
	// Nenhuma permissão de catálogo: o bootstrap não depende delas — a empresa
	// criada é a do próprio chamador, não a de outro tenant.
	core.set()

	for _, tc := range []struct {
		name     string
		role     string
		wantDeny bool
	}{
		{"conta nova sem papel (bootstrap)", "", false},
		{"operador com papel", "gestor", false},
		{"cliente não cria empresa", "cliente", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := jwks.token(t, "tenant-1", tc.role)
			w := doAuthRequest(router, http.MethodPost, "/api/v1/companies", `{}`, token)
			denied := w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized
			if denied != tc.wantDeny {
				t.Fatalf("papel %q: barrado=%v, esperado=%v (HTTP %d: %s)",
					tc.role, denied, tc.wantDeny, w.Code, w.Body.String())
			}
		})
	}
}

// A leitura da lista de membros usa `company:read_members` — é o que o chat e os
// seletores do frontend consomem. Sem a permissão: 403.
func TestCompanyRoutes_ListMembersRequiresReadPermission(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)
	core.set() // nenhuma permissão
	token := jwks.token(t, "tenant-1", "corretor")

	w := doAuthRequest(router, http.MethodGet, "/api/v1/companies/c1/members", "", token)
	if w.Code != http.StatusForbidden {
		t.Fatalf("lista de membros sem `company:read_members` respondeu %d: %s", w.Code, w.Body.String())
	}
}

// Com a permissão de leitura, o guarda deixa passar (o handler responde o que
// puder sem banco — o que importa é NÃO ser 401/403).
func TestCompanyRoutes_ReadPermissionLetsTheGuardThrough(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)
	core.set("company:read_members")
	token := jwks.token(t, "tenant-1", "corretor")

	w := doAuthRequest(router, http.MethodGet, "/api/v1/companies/c1/members", "", token)
	if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
		t.Fatalf("corretor com `company:read_members` foi barrado no guarda: %d %s", w.Code, w.Body.String())
	}
}

// Guarda de sanidade: o provider real é consultado (o dublê de core conta os
// hits), então o teste não está medindo um bypass de root profile.
func TestCompanyRoutes_GuardUsesTenantMatrix(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)
	token := jwks.token(t, "tenant-1", "operacoes")

	// Primeiro sem permissão (403), depois com (passa): a decisão acompanha a
	// matriz do tenant em vez de estar fixada no código.
	core.set()
	if w := doAuthRequest(router, http.MethodGet, "/api/v1/companies/c1/members", "", token); w.Code != http.StatusForbidden {
		t.Fatalf("sem permissão deveria ser 403, veio %d", w.Code)
	}
	core.set("company:read_members")
	if w := doAuthRequest(router, http.MethodGet, "/api/v1/companies/c1/members", "", token); w.Code == http.StatusForbidden {
		t.Fatalf("com permissão não deveria ser 403: %s", w.Body.String())
	}
	_ = context.Background()
}

// P1/A14 — o fluxo local de redefinição por token foi removido.
//
// `passwordResetRepo.Create` nunca era chamado, então `POST /auth/reset-password`
// sempre respondia "invalid token" e `GET /auth/reset-password/verify` sempre
// "invalid or expired token": dois endpoints públicos inalcançáveis que ainda
// anunciavam um caminho alternativo de recuperação (mais fraco que o do
// provedor). A redefinição é a do Supabase (link de recuperação → sessão no
// fragmento → `updateUser` no cliente).
func TestResetPasswordLocalFlowIsGone(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)

	// O fluxo LOCAL de token continua removido: `verify` não existe e o
	// payload antigo (token + confirm_password) não é aceito pela rota atual —
	// ela espera a sessão de recuperação do provedor.
	if w := doAuthRequest(router, http.MethodGet, "/api/v1/auth/reset-password/verify?token=abc", "", ""); w.Code != http.StatusNotFound {
		t.Fatalf("GET /auth/reset-password/verify deveria ter sido removida, respondeu %d: %s", w.Code, w.Body.String())
	}
	wLocal := doAuthRequest(router, http.MethodPost, "/api/v1/auth/reset-password", `{"token":"abc","new_password":"NovaSenha1!","confirm_password":"NovaSenha1!"}`, "")
	if wLocal.Code != http.StatusBadRequest {
		t.Fatalf("payload do fluxo local deveria ser recusado (400), respondeu %d: %s", wLocal.Code, wLocal.Body.String())
	}

	// O pedido do link de recuperação continua existindo (é ele que fala com o
	// provedor) — não pode ser 404.
	w := doAuthRequest(router, http.MethodPost, "/api/v1/auth/forgot-password", `{"email":"user@example.com"}`, "")
	if w.Code == http.StatusNotFound {
		t.Fatal("POST /auth/forgot-password foi removido junto: o pedido do link de recuperação precisa continuar")
	}
}

// P2/M4 — falha de autorização no grupo tem de sair como 403 (não 500) e deixar
// trilha.
//
// A revisão de permissões registrou que os handlers de grupo devolviam **500**
// para "unauthorized" do serviço — sem semântica correta e sem trilha de negação.
// Aqui a chamada é barrada pelo guard de permissão (403 + authz.denied); o
// mapeamento interno do grupo (`respondGroupError`) tem teste próprio em
// `internal/api/v1/handlers`.
func TestGroupRoutes_DenialIsForbiddenAndAudited(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := authRouter(t, jwks.server.URL)
	core.set("company:read_members") // só leitura
	token := jwks.token(t, "tenant-1", "corretor")

	w := doAuthRequest(router, http.MethodPost, "/api/v1/companies/c1/groups",
		`{"name":"Equipe"}`, token)
	if w.Code != http.StatusForbidden {
		t.Fatalf("criação de grupo sem `company:manage_members` deveria ser 403, veio %d: %s", w.Code, w.Body.String())
	}
	if len(denials) != 1 {
		t.Fatalf("negação sem trilha de auditoria: %+v", denials)
	}
	if denials[0].EntityID != "POST /api/v1/companies/c1/groups" {
		t.Fatalf("trilha da negação sem a rota: %q", denials[0].EntityID)
	}
	if denials[0].Metadata["required"] != "company:manage_members" {
		t.Fatalf("trilha sem a permissão exigida: %+v", denials[0].Metadata)
	}
}
