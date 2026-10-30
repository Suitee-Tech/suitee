package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
)

// P1 / A23 — o gate de aprovação por valor não pode falhar ABERTO.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A23) mostrou que o handler defaultava `actorCanApprove := true` quando o
// provider de permissão não estava wired. O próprio comentário admitia o
// cenário ("dev/test setups antigos"): uma regressão de wiring desligava em
// silêncio a exigência de aprovação para deals acima do threshold, deixando um
// corretor empurrar negócios altos.
//
// Contrato novo: `actorCanApproveDeal` falha FECHADO — sem provider, ninguém
// aprova.

type denyingProvider struct{}

func (denyingProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

type allowingProvider struct{}

func (allowingProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	return map[string]bool{"corretor": true}, nil
}

func approverCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/deals/deal-1/status", nil)
	ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
	ctx = pkgcontext.WithAccountID(ctx, "user-1")
	ctx = pkgcontext.WithFunctionalRole(ctx, "corretor")
	c.Request = c.Request.WithContext(ctx)
	// `auth.HasPermission` lê as chaves do contexto do GIN (é o que o
	// middleware de JWT popula), não apenas o request context — o teste precisa
	// espelhar isso para exercitar o caminho real.
	c.Set("tenant_id", "tenant-1")
	c.Set("user_id", "user-1")
	c.Set("functional_role", "corretor")
	c.Set("permission", "")
	c.Set("type", "individual")
	return c
}

func TestActorCanApproveDeal_FailsClosedWithoutProvider(t *testing.T) {
	if actorCanApproveDeal(approverCtx(), nil) {
		t.Fatal("GATE FAIL-OPEN: provider nil liberou aprovação — era exatamente o achado A23")
	}
}

func TestActorCanApproveDeal_FailsClosedWithoutPermission(t *testing.T) {
	if actorCanApproveDeal(approverCtx(), denyingProvider{}) {
		t.Fatal("provider sem a permissão liberou aprovação")
	}
}

func TestActorCanApproveDeal_AllowsWhenPermissionPresent(t *testing.T) {
	if !actorCanApproveDeal(approverCtx(), allowingProvider{}) {
		t.Fatal("permissão presente não liberou aprovação (fluxo legítimo quebrado)")
	}
}

// Um contexto sem tenant não pode aprovar nem com provider que allowlistaria.
func TestActorCanApproveDeal_FailsClosedWithoutTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPatch, "/api/v1/deals/deal-1/status", nil)

	if actorCanApproveDeal(c, allowingProvider{}) {
		t.Fatal("sem tenant no contexto a aprovação deveria ser negada (auth.HasPermission falha fechado)")
	}
}

// Guarda contra a reintrodução do caminho "trusted" no handler HTTP.
func TestHTTPHandlerDoesNotUseTrustedStatusPath(t *testing.T) {
	// O handler de status precisa passar a permissão real do JWT
	// (UpdateStatusAuthorized). O caminho `UpdateStatusTrusted` é para
	// workers/subscribers e não pode aparecer em internal/api.
	src, err := os.ReadFile("deal_handler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "UpdateStatusTrusted(") {
		t.Fatal("handler HTTP chamando UpdateStatusTrusted — isso desliga o gate de aprovação por valor")
	}
	if !strings.Contains(body, "UpdateStatusAuthorized(") {
		t.Fatal("handler HTTP deveria usar UpdateStatusAuthorized com a permissão do JWT")
	}
	if !strings.Contains(body, "actorCanApproveDeal(") {
		t.Fatal("handler HTTP deveria decidir a aprovação por actorCanApproveDeal (fail-closed)")
	}
}

var _ auth.PermissionProvider = denyingProvider{}
