package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	appCtx "github.com/oginoino/toca/pkg/context"
)

// Incidente de 2026-09-21: "Operações não está vendo nada" e "gestor e
// administrador atribuído depois não tem acesso a leads e esteiras criadas
// anteriormente à mudança".
//
// O `toca-deal` decidia o recorte por corretor olhando SÓ o eixo de PERFIL
// (`admin/owner/manager`). Atribuir um Grupo de Usuários grava o PAPEL
// FUNCIONAL e deixa o perfil do vínculo como `member`: quem era promovido a
// gestor/operações continuava com o recorte da própria carteira e via a Esteira
// vazia — inclusive os negócios criados antes da promoção.
func scopedRequest(profile, role, accountID string) *gin.Context {
	return scopedRequestWithBase(profile, role, "", accountID)
}

func scopedRequestWithBase(profile, role, baseRole, accountID string) *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil)
	ctx := appCtx.WithAccountID(req.Context(), accountID)
	ctx = appCtx.WithUserPermission(ctx, profile)
	ctx = appCtx.WithFunctionalRole(ctx, role)
	ctx = appCtx.WithBaseRole(ctx, baseRole)
	ctx = appCtx.WithTenantID(ctx, "tenant-a")
	c.Request = req.WithContext(ctx)
	return c
}

func TestCallerBrokerScope_PromotedRolesSeeTheWholeTenant(t *testing.T) {
	// O caso do relato: perfil do vínculo é `member`, o papel veio do grupo.
	for _, role := range []string{"gestor", "operacoes", "admin"} {
		c := scopedRequest("member", role, "acc-1")
		if scope := callerBrokerScope(c); scope != "" {
			t.Fatalf("papel %q deveria ver o tenant inteiro, mas ficou recortado por %q", role, scope)
		}
	}
}

// Perfil de acesso personalizado: o id (`cr_…`) não casa com o catálogo, mas o
// PAPEL BASE viaja no token. Sem este eixo, "Gestor Comercial" (base gestor)
// via a Esteira vazia — foi o relato "o novo administrador não vê leads de
// corretores que deveria ver assim como gestor".
func TestCallerBrokerScope_AccessProfileUsesItsBaseRole(t *testing.T) {
	c := scopedRequestWithBase("member", "cr_3fd39ef5-6d9c-4f1a-af33-4a7cfa6f6301", "gestor", "acc-1")
	if scope := callerBrokerScope(c); scope != "" {
		t.Fatalf("perfil baseado em gestor deveria ver o tenant inteiro, veio %q", scope)
	}

	// Perfil baseado em corretor continua na própria carteira: o base role não é
	// um passe livre, é a identidade real do perfil.
	c = scopedRequestWithBase("member", "cr_corretor_senior", "corretor", "acc-1")
	if scope := callerBrokerScope(c); scope != "acc-1" {
		t.Fatalf("perfil baseado em corretor deveria seguir recortado, veio %q", scope)
	}
}

func TestCallerBrokerScope_ProfileAxisStillCounts(t *testing.T) {
	for _, profile := range []string{"admin", "owner", "manager"} {
		c := scopedRequest(profile, "corretor", "acc-1")
		if scope := callerBrokerScope(c); scope != "" {
			t.Fatalf("perfil %q deveria ver o tenant inteiro, mas ficou recortado por %q", profile, scope)
		}
	}
}

func TestCallerBrokerScope_BrokerStaysScopedToOwnWallet(t *testing.T) {
	c := scopedRequest("member", "corretor", "acc-corretor")
	if scope := callerBrokerScope(c); scope != "acc-corretor" {
		t.Fatalf("corretor deveria ser recortado pela própria carteira, veio %q", scope)
	}
	// Auditor segue sem visão de tenant (somente leitura NÃO é o mesmo que visão
	// ampla: a decisão de produto é a mesma do CRM, que também não o eleva).
	c = scopedRequest("member", "auditor", "acc-auditor")
	if scope := callerBrokerScope(c); scope != "acc-auditor" {
		t.Fatalf("auditor deveria continuar recortado, veio %q", scope)
	}
}

func TestCallerBrokerScope_AcceptsProfileAxisVariants(t *testing.T) {
	// O valor entra por JWT/app_metadata com casing e espaços inconsistentes.
	c := scopedRequest("  GESTOR  ", "", "acc-1")
	if scope := callerBrokerScope(c); scope != "" {
		t.Fatalf("eixo de perfil com caixa/espaços deveria ser reconhecido, veio %q", scope)
	}
	c = scopedRequest("", " OPERACOES ", "acc-1")
	if scope := callerBrokerScope(c); scope != "" {
		t.Fatalf("eixo de papel com caixa/espaços deveria ser reconhecido, veio %q", scope)
	}
}
