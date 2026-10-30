package handlers

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
)

// P1/A8 — toca-ai: rotas do assistente sem nenhuma verificação de permissão.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A8) registrou `/ai/chat`, `/ai/tools` e as rotas de conversa acessíveis
// a qualquer autenticado não-`cliente`. Agora exigem `ai:use` — capacidade
// nomeada, com o default espelhando a população que já tem o painel no layout
// (então ninguém perde acesso) e revogável por tenant.
//
// O router é o real (`SetupRouter`) e o provider é o real, alimentado por um
// toca-core dublê. Os guards rodam antes dos handlers, então nenhum serviço
// (MCP, DeepSeek, repositório) é necessário.

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
	j := &testJWKS{key: key, kid: "toca-ai-test-kid"}
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

func (j *testJWKS) token(t *testing.T, tenantID string) string {
	t.Helper()
	claims := auth.Claims{
		AccountID:      "acc-1",
		Email:          "corretor@example.com",
		Name:           "Corretor",
		Role:           "member",
		Permission:     "member",
		FunctionalRole: "corretor",
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

func aiRouter(t *testing.T, jwksURL string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwksURL
	return SetupRouter(cfg, NewChatHandler(nil, nil), NewConversationHandler(nil), NewSuggestionsHandler(nil, nil), NewExtractionHandler(nil))
}

func doAIRequest(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// Sem `ai:use`, nenhuma rota do assistente responde: 403 antes do handler.
func TestAIRoutes_RequireAIUse(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := aiRouter(t, jwks.server.URL)
	core.set() // nenhuma permissão
	token := jwks.token(t, "tenant-1")

	cases := []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/ai/tools", ""},
		{http.MethodPost, "/api/v1/ai/chat", `{"message":"oi"}`},
		{http.MethodGet, "/api/v1/ai/suggestions?deal_id=d1", ""},
		{http.MethodGet, "/api/v1/ai/conversations", ""},
		{http.MethodDelete, "/api/v1/ai/conversations", ""},
		{http.MethodDelete, "/api/v1/ai/conversations/c1", ""},
		{http.MethodGet, "/api/v1/ai/conversations/c1/messages", ""},
	}
	for _, tc := range cases {
		w := doAIRequest(router, tc.method, tc.path, tc.body, token)
		if w.Code != http.StatusForbidden {
			t.Fatalf("%s %s sem `ai:use` respondeu %d: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
}

// Com `ai:use` o guarda deixa passar (o handler responde o que puder sem MCP).
func TestAIRoutes_AIUseLetsTheGuardThrough(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()
	router := aiRouter(t, jwks.server.URL)
	core.set("ai:use")
	token := jwks.token(t, "tenant-1")

	for _, path := range []string{"/api/v1/ai/tools", "/api/v1/ai/conversations"} {
		w := doAIRequest(router, http.MethodGet, path, "", token)
		if w.Code == http.StatusForbidden || w.Code == http.StatusUnauthorized {
			t.Fatalf("GET %s com `ai:use` foi barrado no guarda: %d %s", path, w.Code, w.Body.String())
		}
	}

	// `/ai/health` é sonda do serviço: continua aberta a qualquer autenticado.
	if w := doAIRequest(router, http.MethodGet, "/api/v1/ai/health", "", token); w.Code != http.StatusOK {
		t.Fatalf("GET /ai/health deveria seguir 200, veio %d", w.Code)
	}
}
