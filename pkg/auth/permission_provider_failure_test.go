package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
)

// Incidente dos agendamentos (2026-09-21): o operador recebia **403 "permissão
// negada"** ao abrir/salvar uma visita, e o motivo real era o provider de
// permissões falhando (erro engolido pelo ramo que juntava `err != nil` com
// `rolesMap == nil`). Sem o erro no log, o 403 mandou a investigação para o
// lado errado por quase uma hora.
//
// Agora os dois casos são distintos:
//   - erro do provider = INFRAESTRUTURA -> 503 (nada passa, mas o cliente sabe
//     que pode tentar de novo) + `authz.provider_failed` com a causa;
//   - mapa vazio/nil devolvido SEM erro = decisão -> 403, como sempre.

type failingProvider struct{ err error }

func (f failingProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	return nil, f.err
}

type nilMapProvider struct{}

func (nilMapProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	return nil, nil
}

func authenticatedEngine(provider PermissionProvider, permission string) *gin.Engine {
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		// O que o middleware de auth publica para uma requisição autenticada.
		c.Set("user_id", "acc-1")
		c.Set("tenant_id", "tenant-a")
		c.Set("permission", "member")
		c.Set("functional_role", "corretor")
		c.Next()
	})
	engine.GET("/appointments", RequirePermission(provider, permission), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return engine
}

func TestRequirePermission_ProviderFailureIs503WithCause(t *testing.T) {
	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	causa := errors.New("core client: dial tcp 172.19.0.16:8082: connect: connection refused")
	engine := authenticatedEngine(failingProvider{err: causa}, "deals:read")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("falha do provider deveria ser 503 (infraestrutura), veio %d: %s", w.Code, w.Body.String())
	}
	// A trilha registra o motivo, não "sem permissão".
	found := false
	for _, e := range entries {
		if e.Action == "authz.denied" && e.Metadata["reason"] == "permission provider failed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("falha do provider não deixou trilha com o motivo próprio: %+v", entries)
	}
}

func TestRequirePermission_EmptyRolesMapIsStillForbidden(t *testing.T) {
	engine := authenticatedEngine(denyAllProvider{}, "deals:read")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("papel sem a permissão continua 403, veio %d", w.Code)
	}
}

func TestRequirePermission_NilRolesMapIsForbiddenNotUnavailable(t *testing.T) {
	engine := authenticatedEngine(nilMapProvider{}, "deals:read")

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("mapa nil sem erro é decisão (403), veio %d", w.Code)
	}
}

// RequireAnyPermission segue o mesmo critério: erro do provider é 503, não
// "nenhuma das permissões".
func TestRequireAnyPermission_ProviderFailureIs503(t *testing.T) {
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("user_id", "acc-1")
		c.Set("tenant_id", "tenant-a")
		c.Set("permission", "member")
		c.Set("functional_role", "corretor")
		c.Next()
	})
	engine.GET(
		"/exports",
		RequireAnyPermission(failingProvider{err: errors.New("boom")}, "leads:import_export", "reports:export"),
		func(c *gin.Context) { c.Status(http.StatusOK) },
	)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/exports", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("falha do provider no RequireAnyPermission deveria ser 503, veio %d: %s", w.Code, w.Body.String())
	}
}

// T-10 — um chamador INTERNO (X-Internal-Key resolve para permission=service)
// em rota de permissão de USUÁRIO é uma decisão de autorização (403), não uma
// falha de infraestrutura (503): ele não tem identidade de usuário e nunca terá.
// As rotas internas de verdade usam RequireInternalKey, que não passa por aqui.
func TestRequirePermission_InternalCallerIsExplicitlyForbidden(t *testing.T) {
	var entries []audit.Entry
	cleanup := audit.SetTestSink(func(e audit.Entry) { entries = append(entries, e) })
	t.Cleanup(cleanup)

	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		// O que o InternalOrMiddleware publica para uma credencial interna válida.
		c.Set("user_id", "internal")
		c.Set("tenant_id", "tenant-a")
		c.Set("permission", "service")
		c.Set("functional_role", "service")
		c.Set("type", "service")
		c.Next()
	})
	// Provider que FALHARIA se fosse chamado: o ramo de chamador interno nega
	// antes, então a resposta não pode ser 503.
	engine.GET("/appointments", RequirePermission(failingProvider{err: errors.New("no user token")}, "deals:read"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/appointments", nil))

	if w.Code != http.StatusForbidden {
		t.Fatalf("chamador interno deveria ser 403 explícito, veio %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "permission") {
		t.Fatalf("403 sem mensagem de permissão: %s", w.Body.String())
	}
	found := false
	for _, e := range entries {
		if e.Action == "authz.denied" && e.Metadata["reason"] == "internal service callers cannot use user permission routes" {
			found = true
		}
	}
	if !found {
		t.Fatalf("negação sem trilha própria: %+v", entries)
	}
}
