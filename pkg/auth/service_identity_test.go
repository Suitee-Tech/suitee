package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// P1 / A3 — o tenant de uma requisição interna não pode ser escolhido pelo
// chamador.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A3) mostrou rotas internas lendo `?tenant_id=` do chamador
// (toca-signature, toca-ops, toca-calendar, toca-comm, toca-financial,
// toca-crm). Com a chave interna compartilhada, qualquer portador lia/criava
// dados de qualquer tenant informando o id desejado.
//
// Correção: `ResolveTenantForInternalRequest` decide o tenant efetivo:
//   - chamador com JWT de usuário → tenant do TOKEN (pedir outro é 403);
//   - chamador de serviço → tenant pedido, obrigatório, restrito à allowlist da
//     credencial quando ela existe (INTERNAL_SERVICE_KEYS).

func parseRegistry(t *testing.T, raw string) *ServiceKeyRegistry {
	t.Helper()
	return ParseServiceKeyRegistry(raw)
}

func TestServiceKeyRegistry_Resolve(t *testing.T) {
	reg := parseRegistry(t, "toca-financial:key-fin:t1|t2,toca-crm:key-crm")

	fin, ok := reg.Resolve("key-fin")
	if !ok || fin.Name != "toca-financial" {
		t.Fatalf("chave nomeada não resolveu: %+v ok=%v", fin, ok)
	}
	if !fin.AllowsTenant("t1") || !fin.AllowsTenant("t2") {
		t.Fatal("allowlist não reconheceu tenant permitido")
	}
	if fin.AllowsTenant("t3") {
		t.Fatal("allowlist liberou tenant fora da lista")
	}

	crm, ok := reg.Resolve("key-crm")
	if !ok || crm.Name != "toca-crm" {
		t.Fatalf("segunda chave não resolveu: %+v", crm)
	}
	if !crm.AllowsTenant("qualquer") {
		t.Fatal("sem allowlist a credencial atende qualquer tenant (documentado)")
	}

	if _, ok := reg.Resolve("nao-existe"); ok {
		t.Fatal("chave desconhecida resolveu")
	}
	if _, ok := reg.Resolve(""); ok {
		t.Fatal("chave vazia resolveu")
	}
}

func TestServiceKeyRegistry_MalformedEntriesAreIgnored(t *testing.T) {
	reg := parseRegistry(t, "semchave,toca-ok:key-ok,:vazio,,  ")

	if _, ok := reg.Resolve("semchave"); ok {
		t.Fatal("entrada sem par nome:chave virou credencial")
	}
	if _, ok := reg.Resolve(""); ok {
		t.Fatal("chave vazia aceita")
	}
	if _, ok := reg.Resolve("key-ok"); !ok {
		t.Fatal("entrada válida foi descartada junto com as malformadas")
	}
}

func internalCtx(kind, tokenTenant, serviceName string, allowed []string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/signature/internal/envelopes/by-contract/c1", nil)
	if kind != "" {
		c.Set(CtxInternalAuthKind, kind)
	}
	if tokenTenant != "" {
		c.Set("tenant_id", tokenTenant)
	}
	if serviceName != "" {
		c.Set(CtxInternalServiceName, serviceName)
	}
	if allowed != nil {
		c.Set(CtxInternalAllowedScope, allowed)
	}
	return c
}

func TestResolveTenantForInternalRequest_UserTokenWins(t *testing.T) {
	c := internalCtx(InternalAuthUser, "tenant-do-token", "", nil)

	// Pedido vazio ou igual: usa o tenant do token.
	for _, requested := range []string{"", "tenant-do-token"} {
		got, err := ResolveTenantForInternalRequest(c, requested)
		if err != nil || got != "tenant-do-token" {
			t.Fatalf("requested=%q: got=%q err=%v", requested, got, err)
		}
	}

	// Pedir OUTRO tenant é recusado: era o achado A3.
	if _, err := ResolveTenantForInternalRequest(c, "tenant-vitima"); !errors.Is(err, ErrInternalCrossTenant) {
		t.Fatalf("VAZAMENTO: chamador com token de tenant-do-token pediu tenant-vitima e passou (err=%v)", err)
	}
}

func TestResolveTenantForInternalRequest_UserWithoutTenantIsRejected(t *testing.T) {
	c := internalCtx(InternalAuthUser, "", "", nil)
	if _, err := ResolveTenantForInternalRequest(c, ""); !errors.Is(err, ErrInternalTenantRequired) {
		t.Fatalf("token sem tenant deveria falhar fechado (err=%v)", err)
	}
}

func TestResolveTenantForInternalRequest_ServiceNeedsExplicitTenant(t *testing.T) {
	c := internalCtx(InternalAuthService, "", "toca-deal", nil)
	if _, err := ResolveTenantForInternalRequest(c, ""); !errors.Is(err, ErrInternalTenantRequired) {
		t.Fatalf("serviço sem tenant explícito deveria ser recusado (err=%v)", err)
	}
	got, err := ResolveTenantForInternalRequest(c, "tenant-x")
	if err != nil || got != "tenant-x" {
		t.Fatalf("serviço sem allowlist deveria poder atender tenant-x: got=%q err=%v", got, err)
	}
}

func TestResolveTenantForInternalRequest_ServiceAllowlistIsEnforced(t *testing.T) {
	c := internalCtx(InternalAuthService, "", "toca-financial", []string{"t1", "t2"})

	if got, err := ResolveTenantForInternalRequest(c, "t1"); err != nil || got != "t1" {
		t.Fatalf("tenant na allowlist deveria passar: got=%q err=%v", got, err)
	}
	if _, err := ResolveTenantForInternalRequest(c, "t9"); !errors.Is(err, ErrInternalCrossTenant) {
		t.Fatalf("VAZAMENTO: credencial restrita a t1|t2 atendeu t9 (err=%v)", err)
	}
}

func TestResolveTenantForInternalRequest_UnmarkedRouteIsRejected(t *testing.T) {
	c := internalCtx("", "", "", nil)
	if _, err := ResolveTenantForInternalRequest(c, "tenant-x"); !errors.Is(err, ErrInternalUnauthenticated) {
		t.Fatalf("rota sem middleware interno deveria falhar fechado (err=%v)", err)
	}
}

// O middleware aceita as duas formas de identidade e marca qual foi usada.
func TestInternalOrUserMiddleware_ServiceKeyPathMarksService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", "")
	t.Setenv("INTERNAL_SERVICE_KEYS", "toca-signature:svc-key:t1")
	// `ReloadServiceKeyRegistry` em vez de depender do sync.Once: sem isto o
	// resultado dependeria da ordem de execução dos testes do pacote.
	ReloadServiceKeyRegistry()
	t.Cleanup(func() { ReloadServiceKeyRegistry() })

	r := gin.New()
	r.GET("/internal", InternalOrUserMiddleware(Config{}), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"kind":    c.GetString(CtxInternalAuthKind),
			"service": c.GetString(CtxInternalServiceName),
			"allowed": c.GetStringSlice(CtxInternalAllowedScope),
		})
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	req.Header.Set("X-Internal-Key", "svc-key")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("chave nomeada válida: HTTP %d (%s)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"kind":"service"`) || !strings.Contains(body, `"service":"toca-signature"`) {
		t.Fatalf("contexto do serviço não foi marcado: %s", body)
	}
	// A allowlist vincula a credencial aos tenants t1 — e só a eles.
	c := internalCtx(InternalAuthService, "", "toca-signature", []string{"t1"})
	if _, err := ResolveTenantForInternalRequest(c, "t2"); !errors.Is(err, ErrInternalCrossTenant) {
		t.Fatalf("credencial com allowlist t1 atendeu t2 (err=%v)", err)
	}
}

func TestInternalOrUserMiddleware_UnknownKeyDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", "chave-legada")

	r := gin.New()
	r.GET("/internal", InternalOrUserMiddleware(Config{}), func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal", nil)
	req.Header.Set("X-Internal-Key", "chave-errada")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("chave errada: HTTP %d, want 401", w.Code)
	}
}

// A chave compartilhada continua funcionando (migração) e é marcada como legacy.
func TestResolveServiceIdentity_LegacyKeyStillWorks(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "legacy-key-shared")
	identity, ok := ResolveServiceIdentity("legacy-key-shared")
	if !ok {
		t.Fatal("chave compartilhada deixou de funcionar — quebraria a frota inteira")
	}
	if !identity.Legacy {
		t.Fatalf("chave compartilhada deveria ser marcada como legacy: %+v", identity)
	}
}

// O middleware interno preserva o rate limit do RequireInternalKey: trocar o
// middleware nas rotas internas não pode remover a única barreira de taxa da
// superfície service-to-service (60/min por IP por padrão).
//
// Usa um IP dedicado para não consumir o orçamento do limiter (singleton de
// processo) dos demais testes.
func TestInternalOrUserMiddleware_RateLimitsBursts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", "")
	t.Setenv("INTERNAL_SERVICE_KEYS", "svc:rate-probe-key")
	ReloadServiceKeyRegistry()
	t.Cleanup(func() { ReloadServiceKeyRegistry() })

	r := gin.New()
	r.GET("/internal", InternalOrUserMiddleware(Config{}), func(c *gin.Context) { c.Status(http.StatusOK) })

	codes := map[int]int{}
	for i := 0; i < 80; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/internal", nil)
		req.Header.Set("X-Internal-Key", "rate-probe-key")
		req.RemoteAddr = "203.0.113.99:1234" // IP dedicado a este teste
		r.ServeHTTP(w, req)
		codes[w.Code]++
	}

	if codes[http.StatusForbidden] == 0 {
		t.Fatalf("SEM RATE LIMIT: 80 chamadas internas passaram (códigos=%v) — o RequireInternalKey limitava em 60/min", codes)
	}
	if codes[http.StatusOK] == 0 {
		t.Fatalf("o limiter barrou tudo antes do limite (códigos=%v)", codes)
	}
}

// P1/A2 — retrato da migração para chave por serviço.
//
// A migração é configuração (INTERNAL_SERVICE_KEYS nos receptores; cada chamador
// com a própria chave em INTERNAL_API_KEY), mas sem sinal no boot um serviço fica
// no modelo antigo em silêncio. O retrato abaixo é o que dispara o alarme em
// produção.
func TestServiceKeyRolloutStatus(t *testing.T) {
	cases := []struct {
		nome   string
		keys   string
		legacy string
		mode   string
		env    string
		named  int
		needs  bool
	}{
		{"release com chave compartilhada e sem chaves nomeadas", "", "shared-key", "release", "", 0, true},
		{"production por APP_ENV", "", "shared-key", "", "production", 0, true},
		{"release com chaves nomeadas", "toca-deal:k1:t1", "shared-key", "release", "", 1, false},
		{"dev com chave compartilhada (sem alarme)", "", "shared-key", "debug", "development", 0, false},
		{"release sem nenhuma chave (fecha tudo)", "", "", "release", "", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			t.Setenv("INTERNAL_SERVICE_KEYS", tc.keys)
			t.Setenv("INTERNAL_API_KEY", tc.legacy)
			t.Setenv("APP_MODE", tc.mode)
			t.Setenv("APP_ENV", tc.env)
			ReloadServiceKeyRegistry()
			t.Cleanup(func() {
				_ = os.Unsetenv("INTERNAL_SERVICE_KEYS")
				ReloadServiceKeyRegistry()
			})

			got := ServiceKeyRolloutFromEnv()
			if got.NamedKeys != tc.named {
				t.Errorf("NamedKeys = %d, esperado %d", got.NamedKeys, tc.named)
			}
			if got.NeedsMigration() != tc.needs {
				t.Errorf("NeedsMigration() = %v, esperado %v (retrato: %+v)", got.NeedsMigration(), tc.needs, got)
			}
		})
	}
}

// O alarme que faltava em 2026-10-05.
//
// A linha `INTERNAL_SERVICE_KEYS` do .env.beta ficou com o texto da linha
// seguinte colado na última entrada e a chave do toca-signature deixou de ser
// reconhecida por qualquer receptor. O registro continuou com 14 entradas, então
// o alarme de contagem não tinha o que reclamar; o serviço seguiu subindo
// saudável e só se descobriu o problema quando o documento de assinatura não
// chegou a nenhum signatário (401 na chamada interna, evento para a DLQ).
//
// CallerKeyRegistration compara a chave que ESTE processo apresenta com o
// registro que ele mesmo carrega — o único ponto em que a inconsistência é
// visível antes do primeiro 401.
func TestCallerKeyRegistration_DetectsUnregisteredCallerKey(t *testing.T) {
	const (
		signatureKey = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		otherKey     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		sharedKey    = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	)

	cases := []struct {
		nome       string
		keys       string
		callerKey  string
		sharedKey  string
		registered bool
		legacy     bool
		recognized bool
	}{
		{
			nome:       "chave do chamador presente no registro",
			keys:       "toca-signature:" + signatureKey + ",toca-deal:" + otherKey,
			callerKey:  signatureKey,
			sharedKey:  sharedKey,
			registered: true,
			recognized: true,
		},
		{
			nome:       "chave do chamador fora do registro (defeito do beta)",
			keys:       "toca-signature:" + signatureKey + "INTERNAL_CALLER_KEY_GATEWAY=" + otherKey,
			callerKey:  signatureKey,
			sharedKey:  sharedKey,
			registered: false,
			recognized: false,
		},
		{
			nome:       "registro com a chave de outro serviço",
			keys:       "toca-signature:" + otherKey,
			callerKey:  signatureKey,
			sharedKey:  sharedKey,
			registered: false,
			recognized: false,
		},
		{
			nome:       "chamador na chave compartilhada (legado aceito)",
			keys:       "toca-signature:" + signatureKey,
			callerKey:  sharedKey,
			sharedKey:  sharedKey,
			registered: false,
			legacy:     true,
			recognized: true,
		},
		{
			nome:       "sem chave própria configurada",
			keys:       "toca-signature:" + signatureKey,
			callerKey:  "",
			sharedKey:  sharedKey,
			registered: false,
			recognized: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			t.Setenv("INTERNAL_SERVICE_KEYS", tc.keys)
			t.Setenv("INTERNAL_API_KEY", tc.sharedKey)
			t.Setenv("INTERNAL_CALLER_KEY", tc.callerKey)
			ReloadServiceKeyRegistry()
			t.Cleanup(func() {
				_ = os.Unsetenv("INTERNAL_SERVICE_KEYS")
				ReloadServiceKeyRegistry()
			})

			got := CallerKeyRegistrationFromEnv()
			if got.RegisteredKey != tc.registered {
				t.Errorf("RegisteredKey = %v, esperado %v (retrato: %+v)", got.RegisteredKey, tc.registered, got)
			}
			if got.LegacySharedKey != tc.legacy {
				t.Errorf("LegacySharedKey = %v, esperado %v", got.LegacySharedKey, tc.legacy)
			}
			if got.Recognized() != tc.recognized {
				t.Errorf("Recognized() = %v, esperado %v", got.Recognized(), tc.recognized)
			}
			// A chave em si nunca pode aparecer no retrato (só a impressão
			// digital): é o mesmo contrato dos logs de admin.access.
			if len(tc.callerKey) >= 8 && strings.Contains(got.Fingerprint, tc.callerKey[:8]) {
				t.Errorf("fingerprint vazou pedaço da chave: %s", got.Fingerprint)
			}
		})
	}
}
