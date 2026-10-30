package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
)

// Reauditoria de 2026-09-20 — achados N1, N2 e N8 em `pkg/auth`.
//
// Cada caso aqui falhava ANTES da correção, e a mutação correspondente foi
// verificada: N1 = negação sem trilha, N2 = chave nomeada recusada e allowlist de
// tenant ignorada, N8 = lista de permissões vazia liberando a rota.

func newAuthTestRouter(handlers ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	for _, h := range handlers {
		r.Use(h)
	}
	r.NoRoute(func(c *gin.Context) { c.Status(http.StatusOK) })
	r.Any("/*any", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

// authenticatedAs publica o contexto que o Middleware monta a partir das claims.
func authenticatedAs(profile, role, accountID, tenantID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("user_id", accountID)
		c.Set("permission", profile)
		c.Set("functional_role", role)
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

// ---------- N1: negação das guardas legadas deixa trilha ----------

func TestN1_RejectFunctionalRoleDenialIsAudited(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	// Papel `cliente` numa superfície de operador: exatamente a fronteira do
	// P0/A11, que antes respondia 403 sem deixar rastro.
	r := newAuthTestRouter(authenticatedAs("member", "cliente", "acc-1", "tenant-1"), RejectFunctionalRole("cliente"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, quer 403", w.Code)
	}
	if len(denials) != 1 {
		t.Fatalf("negação de `cliente` sem trilha de auditoria: %+v", denials)
	}
	if denials[0].Metadata["reason"] != "role rejected on operator surface" {
		t.Fatalf("motivo inesperado na trilha: %v", denials[0].Metadata["reason"])
	}
	if denials[0].Metadata["functional_role"] != "cliente" {
		t.Fatalf("trilha sem o papel do chamador: %v", denials[0].Metadata)
	}
	if denials[0].EntityID != "GET /api/v1/deals" {
		t.Fatalf("trilha sem a rota: %q", denials[0].EntityID)
	}
}

func TestN1_RequireFunctionalRoleDenialIsAudited(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	r := newAuthTestRouter(authenticatedAs("member", "corretor", "acc-1", "tenant-1"), RequireFunctionalRole("cliente"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/client-portal/overview", nil))

	if w.Code != http.StatusForbidden || len(denials) != 1 {
		t.Fatalf("status = %d, trilha = %+v", w.Code, denials)
	}
	if denials[0].Metadata["required"] != "cliente" {
		t.Fatalf("trilha sem a exigência: %v", denials[0].Metadata)
	}
}

// A negação de caminho público (não autenticado) NÃO pode virar trilha: é
// probe de infraestrutura, não evento de autorização.
func TestN1_UnauthenticatedRejectFunctionalRoleLeavesNoTrail(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	r := newAuthTestRouter(RejectFunctionalRole("cliente"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("probe de infraestrutura: status = %d, quer 200", w.Code)
	}
	if len(denials) != 0 {
		t.Fatalf("probe não autenticado gerou trilha de autorização: %+v", denials)
	}
}

// ---------- N2: chave interna ----------

func TestN2_NamedServiceKeyIsAccepted(t *testing.T) {
	// Antes, este ramo comparava com `==` a INTERNAL_API_KEY e nada mais: uma
	// chave NOMEADA caía no JWT e a chamada de serviço legítima virava 401.
	t.Setenv("INTERNAL_API_KEY", "chave-compartilhada-legada")
	t.Setenv("INTERNAL_SERVICE_KEYS", "toca-deal:chave-do-deal:tenant-1")
	t.Cleanup(func() { ReloadServiceKeyRegistry() })
	ReloadServiceKeyRegistry()

	r := newAuthTestRouter(InternalOrMiddleware(Config{}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil)
	req.Header.Set("X-Internal-Key", "chave-do-deal")
	req.Header.Set("X-Tenant-Id", "tenant-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("chave nomeada recusada: status = %d", w.Code)
	}
}

func TestN2_NamedKeyCannotActOnTenantOutsideItsAllowlist(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "chave-compartilhada-legada")
	t.Setenv("INTERNAL_SERVICE_KEYS", "toca-deal:chave-do-deal:tenant-1")
	t.Cleanup(func() { ReloadServiceKeyRegistry() })
	ReloadServiceKeyRegistry()

	r := newAuthTestRouter(InternalOrMiddleware(Config{}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil)
	req.Header.Set("X-Internal-Key", "chave-do-deal")
	req.Header.Set("X-Tenant-Id", "tenant-2") // fora da allowlist
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("credencial restrita ao tenant-1 atuou no tenant-2: status = %d", w.Code)
	}
}

func TestN2_WrongKeyFallsThroughToJWT(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "chave-compartilhada-legada")
	t.Setenv("INTERNAL_SERVICE_KEYS", "")
	t.Cleanup(func() { ReloadServiceKeyRegistry() })
	ReloadServiceKeyRegistry()

	r := newAuthTestRouter(InternalOrMiddleware(Config{}))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil)
	req.Header.Set("X-Internal-Key", "chave-errada")
	req.Header.Set("X-Tenant-Id", "tenant-1")
	r.ServeHTTP(w, req)

	// Sem JWT válido, o caminho tem de continuar fechando (401), e não passar.
	if w.Code == http.StatusOK {
		t.Fatalf("chave inválida passou pela borda do middleware")
	}
}

// ---------- N8: RequireAnyPermission com exigência vazia ----------

func TestN8_RequireAnyPermissionWithNoPermissionsDenies(t *testing.T) {
	var denials []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) {
		if e.Action == "authz.denied" {
			denials = append(denials, e)
		}
	})
	t.Cleanup(cleanup)

	// `RequireAnyPermission(p)` sem nenhuma permissão liberava todo papel.
	r := newAuthTestRouter(authenticatedAs("member", "corretor", "acc-1", "tenant-1"), RequireAnyPermission(nil))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/reports/financial/dre", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("exigência vazia liberou a rota: status = %d", w.Code)
	}
	if len(denials) != 1 || denials[0].Metadata["reason"] != "empty permission requirement" {
		t.Fatalf("negação sem trilha/motivo: %+v", denials)
	}
}

func TestN8_EmptyPermissionInsideListDoesNotShortCircuit(t *testing.T) {
	// A entrada vazia liberava ANTES de avaliar as outras permissões: com
	// `("a", "")` qualquer papel passava, mesmo sem "a".
	r := newAuthTestRouter(authenticatedAs("member", "corretor", "acc-1", "tenant-1"), RequireAnyPermission(nil, "reports:read", ""))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/reports/financial/dre", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("entrada vazia na lista liberou a rota: status = %d", w.Code)
	}
}

// Claim com tipo inesperado nega em vez de derrubar a requisição com 500.
func TestN8_UnexpectedClaimTypeDeniesInsteadOfPanicking(t *testing.T) {
	brokenCtx := func(c *gin.Context) {
		c.Set("user_id", "acc-1")
		c.Set("tenant_id", 12345) // não é string
		c.Set("permission", "member")
		c.Set("functional_role", "corretor")
		c.Next()
	}

	r := newAuthTestRouter(brokenCtx, RequireAnyPermission(nil, "reports:read"))

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/reports/financial/dre", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("claim de tipo inesperado: status = %d, quer 403", w.Code)
	}
}

// ---------- N9: identidade incompleta na marca de revogação ----------

// O skip continua sendo skip (não é erro: a remoção já aconteceu na fonte de
// verdade), mas deixa de ser indistinguível de sucesso — agora registra
// `auth.membership_revocation_skipped_empty_identity` para alarme.
func TestN9_RevocationWithEmptyIdentitySkipsWithoutWriting(t *testing.T) {
	revoker := &fakeRevoker{}

	if err := RevokeMembership(context.Background(), revoker, "", "acc-1"); err != nil {
		t.Fatalf("identidade vazia: esperava skip sem erro, veio %v", err)
	}
	if err := SuspendMembership(context.Background(), revoker, "tenant-1", ""); err != nil {
		t.Fatalf("identidade vazia: esperava skip sem erro, veio %v", err)
	}
	if err := ClearMembershipRevocation(context.Background(), nil, "", ""); err != nil {
		t.Fatalf("identidade vazia: esperava skip sem erro, veio %v", err)
	}
	if len(revoker.keys) != 0 {
		t.Fatalf("marca gravada com identidade incompleta: %v", revoker.keys)
	}
}
