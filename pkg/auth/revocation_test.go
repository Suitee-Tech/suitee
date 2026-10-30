package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/redis/go-redis/v9"
)

// P1/A15 — vínculo revogado precisa derrubar o access token.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A15) registrou que remover um membro da empresa não invalidava o token
// dele: o acesso continuava até o token expirar. O mecanismo testado aqui é o par
// gravador (toca-auth, no ato da remoção) × verificador (middleware, por
// requisição), com TTL do tamanho do token.

// ───────────────────────── helpers de JWT/JWKS ─────────────────────────
//
// O middleware valida a assinatura contra o JWKS do Supabase: o teste sobe um
// JWKS local e assina o token com a mesma chave, como fazem os testes de
// serviço (toca-ops/toca-comm/toca-calendar).

// clientRevoker adapta o *redis.Client (que devolve *StatusCmd) à interface do
// gravador — em produção quem implementa é o wrapper de Redis do toca-auth.
type clientRevoker struct{ client *redis.Client }

func (c clientRevoker) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

type authTestJWKS struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

var authTestJWKSOnce sync.Once

func initAuthTestJWKS(t *testing.T) *authTestJWKS {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	j := &authTestJWKS{key: key, kid: "pkg-auth-test-kid"}
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
	if err := InitJWKS(j.server.URL); err != nil {
		t.Fatalf("InitJWKS: %v", err)
	}
	return j
}

var authTestJWKSInstance *authTestJWKS

func authTestMiddleware(t *testing.T) gin.HandlerFunc {
	t.Helper()
	authTestJWKSOnce.Do(func() { authTestJWKSInstance = initAuthTestJWKS(t) })
	return Middleware(Config{SupabaseJWKSURL: authTestJWKSInstance.server.URL})
}

func signTestTokenWithRole(t *testing.T, tenantID, accountID, functionalRole string) string {
	t.Helper()
	return signTestTokenWithRoleAndProfile(t, tenantID, accountID, functionalRole, "member")
}

func signTestTokenWithRoleAndProfile(t *testing.T, tenantID, accountID, functionalRole, profile string) string {
	t.Helper()
	if authTestJWKSInstance == nil {
		t.Fatal("JWKS de teste não inicializado")
	}
	claims := Claims{
		AccountID:      accountID,
		Email:          "user@example.com",
		Name:           "Usuário",
		Role:           "member",
		Permission:     profile,
		FunctionalRole: functionalRole,
		Type:           "company",
		TenantID:       tenantID,
		TokenType:      TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accountID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = authTestJWKSInstance.kid
	signed, err := tok.SignedString(authTestJWKSInstance.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func signTestToken(t *testing.T, tenantID, accountID string) string {
	t.Helper()
	if authTestJWKSInstance == nil {
		t.Fatal("JWKS de teste não inicializado")
	}
	claims := Claims{
		AccountID:      accountID,
		Email:          "user@example.com",
		Name:           "Usuário",
		Role:           "member",
		Permission:     "member",
		FunctionalRole: "corretor",
		Type:           "company",
		TenantID:       tenantID,
		TokenType:      TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   accountID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = authTestJWKSInstance.kid
	signed, err := tok.SignedString(authTestJWKSInstance.key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

type fakeRevocationChecker struct {
	revoked map[string]bool
	err     error
	calls   int
}

func (f *fakeRevocationChecker) IsRevoked(_ context.Context, tenantID, accountID string) (bool, error) {
	f.calls++
	if f.err != nil {
		return false, f.err
	}
	return f.revoked[tenantID+":"+accountID], nil
}

type fakeRevoker struct {
	keys []string
	ttl  time.Duration
	err  error
}

func (f *fakeRevoker) Set(_ context.Context, key string, _ interface{}, ttl time.Duration) error {
	if f.err != nil {
		return f.err
	}
	f.keys = append(f.keys, key)
	f.ttl = ttl
	return nil
}

// A chave usada pelo gravador é a mesma consultada pelo verificador — se as duas
// pontas divergirem, a revogação simplesmente não acontece.
func TestRevocationKeyIsSharedByWriterAndChecker(t *testing.T) {
	revoker := &fakeRevoker{}
	if err := RevokeMembership(context.Background(), revoker, "tenant-a", "acc-1"); err != nil {
		t.Fatalf("RevokeMembership: %v", err)
	}
	if len(revoker.keys) != 1 || revoker.keys[0] != MembershipRevocationKey("tenant-a", "acc-1") {
		t.Fatalf("chave gravada divergente: %+v", revoker.keys)
	}
	if revoker.ttl != MembershipRevocationTTL() {
		t.Fatalf("TTL gravado %v, esperado %v", revoker.ttl, MembershipRevocationTTL())
	}
}

// O TTL acompanha a vida do token (default 1h) e aceita configuração por env.
func TestMembershipRevocationTTL(t *testing.T) {
	t.Setenv("MEMBERSHIP_REVOCATION_TTL", "")
	if got := MembershipRevocationTTL(); got != DefaultMembershipRevocationTTL {
		t.Fatalf("TTL default = %v, esperado %v", got, DefaultMembershipRevocationTTL)
	}
	t.Setenv("MEMBERSHIP_REVOCATION_TTL", "15m")
	if got := MembershipRevocationTTL(); got != 15*time.Minute {
		t.Fatalf("TTL configurado = %v, esperado 15m", got)
	}
	t.Setenv("MEMBERSHIP_REVOCATION_TTL", "torto")
	if got := MembershipRevocationTTL(); got != DefaultMembershipRevocationTTL {
		t.Fatalf("TTL inválido deveria cair no default, veio %v", got)
	}
}

// Vínculo revogado → 401 e o handler NÃO roda.
func TestMiddleware_RejectsRevokedMembership(t *testing.T) {
	checker := &fakeRevocationChecker{revoked: map[string]bool{"tenant-a:acc-1": true}}
	SetRevocationChecker(checker)
	t.Cleanup(func() { SetRevocationChecker(nil) })

	handlerRan := false
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/leads", func(c *gin.Context) { handlerRan = true; c.Status(http.StatusOK) })

	token := signTestToken(t, "tenant-a", "acc-1")
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("vínculo revogado deveria dar 401, veio %d: %s", w.Code, w.Body.String())
	}
	if handlerRan {
		t.Fatal("o handler rodou apesar da revogação")
	}
	if checker.calls == 0 {
		t.Fatal("o verificador não foi consultado")
	}
}

// Vínculo válido → segue normalmente.
func TestMiddleware_AllowsNonRevokedMembership(t *testing.T) {
	SetRevocationChecker(&fakeRevocationChecker{revoked: map[string]bool{}})
	t.Cleanup(func() { SetRevocationChecker(nil) })

	handlerRan := false
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/leads", func(c *gin.Context) { handlerRan = true; c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, "tenant-a", "acc-1"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !handlerRan {
		t.Fatalf("vínculo válido deveria passar: status=%d handlerRan=%v", w.Code, handlerRan)
	}
}

// Falha do Redis: 200 (fail-open deliberado — é mitigação suplementar; derrubar
// todo o tráfego autenticado por indisponibilidade de cache seria pior que a
// janela de até 1h que o mecanismo cobre).
func TestMiddleware_FailsOpenWhenRevocationStoreIsDown(t *testing.T) {
	SetRevocationChecker(&fakeRevocationChecker{err: errors.New("redis down")})
	t.Cleanup(func() { SetRevocationChecker(nil) })

	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/leads", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, "tenant-a", "acc-1"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("falha do store de revogação deveria ser fail-open, veio %d", w.Code)
	}
}

// TestRedisRevocationChecker_RoundTrip exercita a implementação REAL (go-redis)
// contra um Redis de verdade. É gated por REDIS_URI, como os testes de banco
// descartável: sem Redis configurado, o teste é pulado.
//
//	REDIS_URI=redis://127.0.0.1:6379/ go test ./auth/ -run TestRedisRevocationChecker -v
func TestRedisRevocationChecker_RoundTrip(t *testing.T) {
	uri := os.Getenv("REDIS_URI")
	if uri == "" {
		t.Skip("set REDIS_URI to run the Redis-backed revocation check")
	}
	opts, err := redis.ParseURL(uri)
	if err != nil {
		t.Fatalf("parse REDIS_URI: %v", err)
	}
	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })

	checker := NewRedisRevocationChecker(client)
	ctx := context.Background()
	tenantID := "qa-tenant-a15"
	accountID := "qa-account-a15"
	t.Cleanup(func() { _ = client.Del(ctx, MembershipRevocationKey(tenantID, accountID)).Err() })

	revoked, err := checker.IsRevoked(ctx, tenantID, accountID)
	if err != nil {
		t.Fatalf("IsRevoked (limpo): %v", err)
	}
	if revoked {
		t.Fatal("vínculo marcado como revogado antes de qualquer remoção")
	}

	if err := RevokeMembership(ctx, clientRevoker{client}, tenantID, accountID); err != nil {
		t.Fatalf("RevokeMembership: %v", err)
	}
	revoked, err = checker.IsRevoked(ctx, tenantID, accountID)
	if err != nil {
		t.Fatalf("IsRevoked (após revogação): %v", err)
	}
	if !revoked {
		t.Fatal("o verificador não enxergou a marca gravada — gravador e verificador divergem")
	}

	// TTL: a marca precisa expirar sozinha (o mecanismo cobre a vida do token).
	ttl, err := client.TTL(ctx, MembershipRevocationKey(tenantID, accountID)).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 || ttl > MembershipRevocationTTL() {
		t.Fatalf("TTL da marca = %v, esperado (0, %v]", ttl, MembershipRevocationTTL())
	}
}

// P2/M4 — negação de autorização deixa trilha.
//
// A revisão de permissões (achado M4) registrou que falhas de autorização não
// tinham trilha: não havia como reconstituir quem tentou o quê e foi barrado. O
// ponto único é `denyAuthz`, usado por RequirePermission/RequireAnyPermission.
func TestRequirePermission_RecordsDenialInAudit(t *testing.T) {
	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	provider := &denyAllProvider{}
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/leads", RequirePermission(provider, "leads:write"), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, "tenant-a", "acc-1"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("sem a permissão deveria dar 403, veio %d", w.Code)
	}
	if len(entries) != 1 {
		t.Fatalf("esperava 1 evento de auditoria da negação, veio %d: %+v", len(entries), entries)
	}
	e := entries[0]
	if e.Action != "authz.denied" || e.EntityType != "http_route" {
		t.Fatalf("trilha da negação com ação/entidade erradas: %+v", e)
	}
	if e.EntityID != "GET /leads" {
		t.Fatalf("trilha sem a rota: %q", e.EntityID)
	}
	if e.Metadata["required"] != "leads:write" || e.Metadata["reason"] == nil {
		t.Fatalf("trilha sem permissão exigida/motivo: %+v", e.Metadata)
	}
}

// RequireAnyPermission segue o mesmo caminho.
func TestRequireAnyPermission_RecordsDenialInAudit(t *testing.T) {
	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/exports", RequireAnyPermission(&denyAllProvider{}, "leads:import_export", "reports:export"), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/exports", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, "tenant-a", "acc-1"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("esperado 403, veio %d", w.Code)
	}
	if len(entries) != 1 || entries[0].Action != "authz.denied" {
		t.Fatalf("negação do RequireAnyPermission sem trilha: %+v", entries)
	}
	if got := entries[0].Metadata["required"]; got != "leads:import_export|reports:export" {
		t.Fatalf("trilha sem as permissões exigidas: %v", got)
	}
}

// Autorizado NÃO gera evento de negação (a trilha registra a negação, não o
// tráfego normal).
func TestRequirePermission_AllowsWithoutDenialEntry(t *testing.T) {
	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/leads", RequirePermission(&allowAllProvider{}, "leads:write"), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, "tenant-a", "acc-1"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("autorizado deveria passar, veio %d", w.Code)
	}
	if len(entries) != 0 {
		t.Fatalf("requisição autorizada gerou evento de negação: %+v", entries)
	}
}

type denyAllProvider struct{}

func (denyAllProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (denyAllProvider) GetPermissions(context.Context, string, string) ([]string, error) {
	return nil, nil
}

type allowAllProvider struct{}

func (allowAllProvider) GetRolesMap(_ context.Context, _, _ string) (map[string]bool, error) {
	return map[string]bool{"corretor": true}, nil
}
func (allowAllProvider) GetPermissions(context.Context, string, string) ([]string, error) {
	return nil, nil
}

// P2/M2 — RejectFunctionalRole era FAIL-OPEN sem `functional_role`.
//
// Token sem papel funcional atravessava grupos marcados como "de operador".
// Agora nega — exceto perfil root (owner/admin), que legitimamente pode não
// carregar papel e tem bypass registrado.
func TestRejectFunctionalRole_FailsClosedWithoutRole(t *testing.T) {
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/companies", RejectFunctionalRole("cliente"), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	req.Header.Set("Authorization", "Bearer "+signTestTokenWithRole(t, "tenant-a", "acc-1", ""))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("token sem papel funcional deveria ser barrado (fail-closed), veio %d: %s", w.Code, w.Body.String())
	}
}

// Regressão do deploy (2026-09-19): a guarda fail-closed do M2 derrubou os
// probes de infraestrutura. Em toca-core o `RejectFunctionalRole("cliente")` é
// aplicado na ENGINE, então `/health` e `/metrics` (públicos por whitelist do
// middleware de auth) passaram a responder 403 — o container ficou unhealthy e o
// `make up` abortou. Requisição não autenticada não tem papel a rejeitar: quem
// guarda rota de usuário é a permissão (que falha fechado).
func TestRejectFunctionalRole_PublicPathWithoutAuthPasses(t *testing.T) {
	engine := gin.New()
	// Sem o middleware de auth: é o que acontece com um probe em caminho público
	// (a auth libera /health e /metrics por whitelist, sem popular user_id).
	engine.GET("/health", RejectFunctionalRole("cliente"), func(c *gin.Context) { c.Status(http.StatusOK) })
	engine.GET("/metrics", RejectFunctionalRole("cliente"), func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, path := range []string{"/health", "/metrics"} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("%s sem autenticação deveria passar (probe de infraestrutura), veio %d: %s", path, w.Code, w.Body.String())
		}
	}
}

// O caminho autenticado continua fail-closed: token válido sem papel não passa.
// (É o contraste que garante que a correção do probe não reabriu o M2.)
func TestRejectFunctionalRole_AuthenticatedWithoutRoleStillDenied(t *testing.T) {
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		// Simula o que o middleware de auth faz quando o token é válido e o
		// caminho NÃO é público.
		c.Set("user_id", "acc-1")
		c.Set("tenant_id", "tenant-a")
		c.Next()
	})
	engine.GET("/companies", RejectFunctionalRole("cliente"), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/companies", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("chamador autenticado sem papel deveria ser barrado, veio %d", w.Code)
	}
}

// Perfil root sem papel funcional continua passando (o bypass é registrado).
func TestRejectFunctionalRole_RootProfileWithoutRolePasses(t *testing.T) {
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/companies", RejectFunctionalRole("cliente"), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/companies", nil)
	req.Header.Set("Authorization", "Bearer "+signTestTokenWithRoleAndProfile(t, "tenant-a", "acc-1", "", "owner"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("perfil root deveria passar, veio %d: %s", w.Code, w.Body.String())
	}
}

// Papel presente que NÃO está na lista de rejeição segue passando; o papel da
// lista é barrado (comportamento original preservado).
func TestRejectFunctionalRole_KeepsOriginalBehavior(t *testing.T) {
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	engine.GET("/companies", RejectFunctionalRole("cliente"), func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, tc := range []struct {
		role string
		want int
	}{
		{"corretor", http.StatusOK},
		{"gestor", http.StatusOK},
		{"cliente", http.StatusForbidden},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/companies", nil)
		req.Header.Set("Authorization", "Bearer "+signTestTokenWithRole(t, "tenant-a", "acc-1", tc.role))
		engine.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Fatalf("papel %q: esperado %d, veio %d", tc.role, tc.want, w.Code)
		}
	}
}

// P2/B3 — HasPermission com permissão vazia continua "só autenticação", mas deixa
// rastro (o engano de passar a constante errada não pode ser silencioso).
func TestHasPermission_EmptyPermissionIsAllowedAndLogged(t *testing.T) {
	engine := gin.New()
	engine.Use(authTestMiddleware(t))
	allowed := false
	engine.GET("/anything", func(c *gin.Context) {
		allowed = HasPermission(c, allowAllProvider{}, "")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/anything", nil)
	req.Header.Set("Authorization", "Bearer "+signTestToken(t, "tenant-a", "acc-1"))
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK || !allowed {
		t.Fatalf("permissão vazia deveria continuar permitindo (só autenticação): %d allowed=%v", w.Code, allowed)
	}
}

// Sem tenant, HasPermission nega (fail-closed) — o contrato documentado.
func TestHasPermission_WithoutTenantDenies(t *testing.T) {
	engine := gin.New()
	allowed := true
	engine.GET("/anything", func(c *gin.Context) {
		allowed = HasPermission(c, allowAllProvider{}, "leads:read")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/anything", nil))
	if allowed {
		t.Fatalf("sem tenant no contexto deveria negar (status %d)", w.Code)
	}
}
