package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

// P1 / A26 — posse nas MUTAÇÕES de lead e gate de reatribuição.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achados A26 e F5/F6) mostrou que as rotas de mutação exigiam apenas
// `leads:write` e não checavam posse: um corretor alterava o lead de outro
// corretor; e `assigned_broker_id` era aceito no corpo do PATCH sem exigir
// `leads:reassign` (permissão que, aliás, não tinha nenhum enforcement).

type mutatingLeadRepo struct {
	lead.Repository
	item    *lead.Lead
	patched bool
	updated bool
}

func (r *mutatingLeadRepo) GetByID(_ context.Context, tenantID, id string) (*lead.Lead, error) {
	if r.item == nil || r.item.ID != id || (tenantID != "" && r.item.TenantID != tenantID) {
		return nil, nil
	}
	return r.item, nil
}

func (r *mutatingLeadRepo) Patch(_ context.Context, leadID, tenantID string, _ *lead.LeadExpectedVersion, _ map[string]interface{}) (*lead.Lead, bool, error) {
	r.patched = true
	return r.item, true, nil
}

func (r *mutatingLeadRepo) Update(_ context.Context, l *lead.Lead) error {
	r.updated = true
	return nil
}

func mutationRouter(repo lead.Repository, provider authProvider, accountID, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewLeadService(repo, nil, nil, nil, nil, nil)
	h := NewLeadHandler(svc, nil, nil).WithPermissionProvider(provider)

	r := gin.New()
	r.Use(authMiddlewareForTest(accountID, role))
	r.PATCH("/leads/:id", h.RequireLeadOwnership(), h.Patch)
	return r
}

func doMutation(r *gin.Engine, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/leads/lead-alheio", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestLeadMutation_CorretorNaoAlteraLeadDeOutroCorretor(t *testing.T) {
	repo := &mutatingLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", Name: "Cliente de outro corretor",
		AssignedBrokerID: "corretor-2",
	}}
	r := mutationRouter(repo, brokerProvider, "corretor-1", "corretor")

	w := doMutation(r, `{"name":"invadido"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("VAZAMENTO: corretor-1 alterou lead de corretor-2 (HTTP %d): %s", w.Code, w.Body.String())
	}
	if repo.patched || repo.updated {
		t.Fatal("MUTAÇÃO CROSS-BROKER PERSISTIDA: o repositório foi chamado apesar da recusa")
	}
}

func TestLeadMutation_CorretorAlteraProprioLead(t *testing.T) {
	repo := &mutatingLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", AssignedBrokerID: "corretor-1",
	}}
	r := mutationRouter(repo, brokerProvider, "corretor-1", "corretor")

	w := doMutation(r, `{"name":"meu cliente"}`)

	if w.Code == http.StatusNotFound {
		t.Fatalf("dono do lead foi recusado (HTTP %d): %s", w.Code, w.Body.String())
	}
	if !repo.patched {
		t.Fatal("mutação legítima não chegou ao repositório")
	}
}

// O pool NÃO atribuído continua mutável para quem não é elevado — a listagem do
// corretor inclui esses leads (repo filtra `AssignedBrokerID = caller OR ”`).
func TestLeadMutation_PoolNaoAtribuidoContinuaMutavel(t *testing.T) {
	repo := &mutatingLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", AssignedBrokerID: "",
	}}
	r := mutationRouter(repo, brokerProvider, "corretor-1", "corretor")

	w := doMutation(r, `{"name":"pool"}`)

	if w.Code == http.StatusNotFound {
		t.Fatalf("lead do pool foi recusado para corretor (HTTP %d) — a listagem dele inclui esses leads", w.Code)
	}
}

func TestLeadMutation_GestorAlteraQualquerLead(t *testing.T) {
	repo := &mutatingLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", AssignedBrokerID: "corretor-2",
	}}
	r := mutationRouter(repo, elevatedProvider, "gestor-1", "gestor")

	w := doMutation(r, `{"name":"ajuste do gestor"}`)

	if w.Code == http.StatusNotFound {
		t.Fatalf("gestor (leads:reassign) foi recusado (HTTP %d): %s", w.Code, w.Body.String())
	}
}

func TestLeadMutation_LeadInexistente404(t *testing.T) {
	repo := &mutatingLeadRepo{item: nil}
	r := mutationRouter(repo, elevatedProvider, "gestor-1", "gestor")

	if w := doMutation(r, `{"name":"x"}`); w.Code != http.StatusNotFound {
		t.Fatalf("lead inexistente: HTTP %d, want 404", w.Code)
	}
	if repo.patched {
		t.Fatal("mutação chegou ao repositório sem lead existente")
	}
}

// O middleware precisa ver o tenant do token; sem ele o serviço não acha o lead.
func TestLeadMutation_SemTenantNoContexto404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &mutatingLeadRepo{item: &lead.Lead{ID: "lead-alheio", TenantID: "tenant-1"}}
	svc := service.NewLeadService(repo, nil, nil, nil, nil, nil)
	h := NewLeadHandler(svc, nil, nil).WithPermissionProvider(elevatedProvider)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithAccountID(c.Request.Context(), "gestor-1")
		c.Request = c.Request.WithContext(ctx)
		c.Set("user_id", "gestor-1")
		c.Next()
	})
	r.PATCH("/leads/:id", h.RequireLeadOwnership(), h.Patch)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/leads/lead-alheio", strings.NewReader(`{"name":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("sem tenant no contexto: HTTP %d, want 404 (fail-closed)", w.Code)
	}
}
