package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

// P1 / A10 — posse na leitura de lead por id.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A10) mostrou que as leituras de lead usavam
// `RequirePermission(provider, "")` (só "autenticado com papel") e que
// `GET /leads/:id` não checava posse: qualquer papel lia qualquer lead do
// tenant pelo UUID, enquanto a UI já filtrava por corretor no cliente
// (`isAdmin` em Leads.tsx). O servidor agora aplica a MESMA regra.

// roleProvider responde apenas sobre `leads:reassign` (a permissão que marca
// "lê qualquer lead do tenant"): devolve os papéis autorizados configurados.
type roleProvider struct{ reassignRoles map[string]bool }

func (p roleProvider) GetRolesMap(_ context.Context, _, requiredPermission string) (map[string]bool, error) {
	if requiredPermission == "leads:reassign" {
		return p.reassignRoles, nil
	}
	return map[string]bool{}, nil
}

var (
	elevatedProvider = roleProvider{reassignRoles: map[string]bool{"gestor": true}}
	brokerProvider   = roleProvider{reassignRoles: map[string]bool{}}
)

func leadReadCtx(accountID, functionalRole string, _ auth.PermissionProvider) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/leads/lead-1", nil)
	ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
	ctx = pkgcontext.WithAccountID(ctx, accountID)
	c.Request = c.Request.WithContext(ctx)
	// chaves do gin context (é o que auth.HasPermission lê)
	c.Set("tenant_id", "tenant-1")
	c.Set("user_id", accountID)
	c.Set("functional_role", functionalRole)
	c.Set("permission", "")
	return c
}

func TestCanReadLead_ElevatedReadsAnyLead(t *testing.T) {
	c := leadReadCtx("gestor-1", "gestor", elevatedProvider)
	other := &lead.Lead{ID: "lead-1", TenantID: "tenant-1", AssignedBrokerID: "corretor-9"}

	if !canReadLead(c, other, elevatedProvider) {
		t.Fatal("quem tem leads:reassign deveria ler qualquer lead do tenant")
	}
}

func TestCanReadLead_BrokerReadsOwnAndUnassigned(t *testing.T) {
	c := leadReadCtx("corretor-1", "corretor", brokerProvider)

	own := &lead.Lead{ID: "lead-1", TenantID: "tenant-1", AssignedBrokerID: "corretor-1"}
	if !canReadLead(c, own, brokerProvider) {
		t.Fatal("corretor deveria ler o próprio lead")
	}

	unassigned := &lead.Lead{ID: "lead-2", TenantID: "tenant-1", AssignedBrokerID: ""}
	if !canReadLead(c, unassigned, brokerProvider) {
		t.Fatal("corretor deveria ler o lead ainda não atribuído (pool da roleta)")
	}
}

func TestCanReadLead_BrokerCannotReadAnotherBrokersLead(t *testing.T) {
	c := leadReadCtx("corretor-1", "corretor", brokerProvider)
	foreign := &lead.Lead{ID: "lead-1", TenantID: "tenant-1", AssignedBrokerID: "corretor-2"}

	if canReadLead(c, foreign, brokerProvider) {
		t.Fatal("VAZAMENTO: corretor leu lead atribuído a outro corretor")
	}
}

// Sem provider (regressão de wiring) a decisão é conservadora: nada de
// "qualquer um lê qualquer coisa".
func TestCanReadLead_SemProviderNaoLiberaLeadAlheio(t *testing.T) {
	c := leadReadCtx("corretor-1", "corretor", nil)
	foreign := &lead.Lead{ID: "lead-1", TenantID: "tenant-1", AssignedBrokerID: "corretor-2"}

	if canReadLead(c, foreign, nil) {
		t.Fatal("sem provider o corretor não pode ler lead de outro")
	}
	unassigned := &lead.Lead{ID: "lead-2", TenantID: "tenant-1", AssignedBrokerID: ""}
	if !canReadLead(c, unassigned, nil) {
		t.Fatal("sem provider o pool não atribuído continua visível (fluxo da roleta)")
	}
}

func TestCanReadLead_SemContaNoContexto_Nega(t *testing.T) {
	c := leadReadCtx("", "corretor", nil)
	own := &lead.Lead{ID: "lead-1", TenantID: "tenant-1", AssignedBrokerID: "corretor-1"}
	if canReadLead(c, own, nil) {
		t.Fatal("sem account id no contexto a leitura deveria ser negada (fail-closed)")
	}
}

func TestCanReadLead_NilLead_Nega(t *testing.T) {
	c := leadReadCtx("corretor-1", "corretor", nil)
	if canReadLead(c, nil, nil) {
		t.Fatal("lead nil não pode ser liberado")
	}
}
