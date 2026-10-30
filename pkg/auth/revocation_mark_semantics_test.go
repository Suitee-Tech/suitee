package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Incidente de 2026-09-21 — "depois de trocar o perfil de acesso de um usuário e
// sair, a página ficou em loop infinito".
//
// A marca do A15 dizia só "este vínculo mudou" e bloqueava QUALQUER token
// enquanto estivesse no Redis (TTL 1h). Trocar o papel/perfil de alguém,
// portanto, tirava o acesso da própria pessoa por até uma hora — mesmo ela
// entrando de novo, com um token emitido depois da mudança. O front não
// conseguia restaurar a sessão, voltava para o /login e repetia.
//
// A semântica correta: a marca corta as credenciais emitidas ANTES da mudança.
// Uma credencial emitida depois é, por definição, do contexto novo.

func TestRevocationMarkCutsOnlyTokensIssuedBefore(t *testing.T) {
	// Marca gravada às 12:00:00 (valor em segundos, arredondado para cima).
	const cut = int64(1_700_000_010)
	mark := "1700000010"

	if revocationMarkRevokesUntil(mark, time.Unix(cut-1, 0)) != true {
		t.Fatal("token emitido ANTES da troca deveria ser revogado (A15)")
	}
	if revocationMarkRevokesUntil(mark, time.Unix(cut, 0)) != false {
		t.Fatal("token emitido DEPOIS da troca foi revogado — é o defeito do loop infinito")
	}
	if revocationMarkRevokesUntil(mark, time.Unix(cut+60, 0)) != false {
		t.Fatal("token novo (login depois da troca) precisa passar")
	}
}

func TestRevocationMarkLegacyAndSuspensionAlwaysCut(t *testing.T) {
	issued := time.Unix(1_700_000_100, 0)
	// Suspensão é decisão administrativa durável: vale enquanto existir.
	if !revocationMarkRevokesUntil(SuspendedMembershipValue, issued) {
		t.Fatal("vínculo suspenso deveria bloquear qualquer token")
	}
	// Marca legada ("1"), gravada antes desta mudança, não tem timestamp:
	// conservador, bloqueia.
	if !revocationMarkRevokesUntil("1", issued) {
		t.Fatal("marca legada sem timestamp deveria continuar bloqueando")
	}
	if revocationMarkRevokesUntil("", issued) {
		t.Fatal("sem marca não há revogação")
	}
	// Sem saber quando o token foi emitido, falha FECHADO.
	if !revocationMarkRevokesUntil("1700000010", time.Time{}) {
		t.Fatal("token sem `iat` deveria ser tratado como revogado (fail-closed)")
	}
}

type recordingRevoker struct {
	key   string
	value interface{}
	ttl   time.Duration
}

func (r *recordingRevoker) Set(_ context.Context, key string, value interface{}, ttl time.Duration) error {
	r.key, r.value, r.ttl = key, value, ttl
	return nil
}

func TestRevokeMembershipWritesCeiledTimestamp(t *testing.T) {
	revoker := &recordingRevoker{}
	if err := RevokeMembership(context.Background(), revoker, "tenant-a", "acc-1"); err != nil {
		t.Fatalf("RevokeMembership: %v", err)
	}
	if revoker.key != MembershipRevocationKey("tenant-a", "acc-1") {
		t.Fatalf("chave = %q", revoker.key)
	}
	raw, ok := revoker.value.(string)
	if !ok {
		t.Fatalf("marca deveria ser o timestamp em texto, veio %T", revoker.value)
	}
	mark, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		t.Fatalf("marca não é numérica (%q): %v", raw, err)
	}
	now := time.Now().Unix()
	if mark < now || mark > now+2 {
		t.Fatalf("marca = %d, esperado ~%d (arredondado para cima)", mark, now+1)
	}
	// O arredondamento para cima é o que garante que um token emitido no MESMO
	// segundo da troca também seja cortado.
	if !revocationMarkRevokesUntil(raw, time.Unix(now, 0)) {
		t.Fatal("token emitido no mesmo segundo da troca deveria ser revogado")
	}
	if revocationMarkRevokesUntil(raw, time.Unix(mark, 0)) {
		t.Fatal("token do segundo seguinte já é do contexto novo e deveria passar")
	}
	if revoker.ttl != MembershipRevocationTTL() {
		t.Fatalf("ttl = %v, esperado %v", revoker.ttl, MembershipRevocationTTL())
	}
}

// O middleware precisa entregar o `iat` do token ao checker — sem isso a decisão
// volta a ser "bloqueia sempre".
type issuedAtChecker struct {
	seen       time.Time
	revokedFor func(time.Time) bool
}

func (c *issuedAtChecker) IsRevoked(context.Context, string, string) (bool, error) {
	return true, nil // se o middleware cair aqui, o token novo seria recusado
}

func (c *issuedAtChecker) IsRevokedSince(_ context.Context, _, _ string, issuedAt time.Time) (bool, error) {
	c.seen = issuedAt
	return c.revokedFor(issuedAt), nil
}

func revokedMiddlewareEngine(t *testing.T, checker RevocationChecker) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	previous := RevocationCheckerFromEnv()
	SetRevocationChecker(checker)
	t.Cleanup(func() { SetRevocationChecker(previous) })

	engine := gin.New()
	engine.GET("/appointments", func(c *gin.Context) {
		c.Set("user_id", "acc-1")
		c.Set("tenant_id", "tenant-a")
		c.Set("permission", "member")
		c.Set("functional_role", "corretor")
		c.Next()
	}, func(c *gin.Context) {
		// Espelha o ramo do middleware que consulta a marca.
		checker := RevocationCheckerFromEnv()
		issuedAt := time.Unix(1_700_000_000, 0)
		revoked, err := isRevokedForToken(c.Request.Context(), checker, "tenant-a", "acc-1", issuedAt)
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		if revoked {
			c.Status(http.StatusUnauthorized)
			return
		}
		c.Status(http.StatusOK)
	})
	return engine
}

func TestMiddlewarePassesIssuedAtToChecker(t *testing.T) {
	checker := &issuedAtChecker{revokedFor: func(issuedAt time.Time) bool { return issuedAt.IsZero() }}
	engine := revokedMiddlewareEngine(t, checker)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("token do contexto novo deveria passar, veio %d", w.Code)
	}
	if checker.seen.IsZero() {
		t.Fatal("o checker não recebeu o instante de emissão do token")
	}
}

func TestMiddlewareFallsBackWhenCheckerHasNoIssuedAtSupport(t *testing.T) {
	engine := revokedMiddlewareEngine(t, legacyOnlyChecker{err: errors.New("redis caiu")})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments", nil))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("erro do checker legado deveria propagar, veio %d", w.Code)
	}
}

type legacyOnlyChecker struct{ err error }

func (c legacyOnlyChecker) IsRevoked(context.Context, string, string) (bool, error) {
	return false, c.err
}
