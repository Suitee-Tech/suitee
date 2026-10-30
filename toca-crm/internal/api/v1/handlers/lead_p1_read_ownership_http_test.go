package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

// P1 / A10 — a rota `GET /leads/:id` aplica a posse de verdade.
//
// Este teste exercita o CAMINHO HTTP (handler → serviço → repositório), e não
// apenas a função de decisão: sem ele, remover a chamada a `canReadLead` do
// handler passaria despercebido (foi o que a verificação por mutação mostrou).

type ownershipLeadRepo struct {
	lead.Repository
	item *lead.Lead
}

func (r *ownershipLeadRepo) GetByID(_ context.Context, tenantID, id string) (*lead.Lead, error) {
	if r.item == nil || r.item.ID != id {
		return nil, nil
	}
	if tenantID != "" && r.item.TenantID != tenantID {
		return nil, nil
	}
	return r.item, nil
}

// authMiddlewareForTest reproduz o que o middleware de JWT faz: popula o
// REQUEST context (usado pelo handler) e as chaves do GIN context (usadas por
// `auth.HasPermission`). Sem as duas, o teste não exercita o caminho real.
func authMiddlewareForTest(accountID, role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
		ctx = pkgcontext.WithAccountID(ctx, accountID)
		ctx = pkgcontext.WithFunctionalRole(ctx, role)
		c.Request = c.Request.WithContext(ctx)

		c.Set("tenant_id", "tenant-1")
		c.Set("user_id", accountID)
		c.Set("functional_role", role)
		c.Set("permission", "")
		c.Set("type", "individual")
		c.Next()
	}
}

func leadReadRouter(repo lead.Repository, provider authProvider, accountID, role string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewLeadService(repo, nil, nil, nil, nil, nil)
	h := NewLeadHandler(svc, nil, nil).WithPermissionProvider(provider)

	r := gin.New()
	r.Use(authMiddlewareForTest(accountID, role))
	r.GET("/leads/:id", h.GetByID)
	return r
}

// authProvider é o mesmo fake de permissão usado nos testes da função pura.
type authProvider = roleProvider

func doLeadRead(r *gin.Engine) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/leads/lead-alheio", nil)
	r.ServeHTTP(w, req)
	return w
}

func TestGetLeadByID_CorretorNaoLeLeadDeOutroCorretor(t *testing.T) {
	repo := &ownershipLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", Name: "Cliente de outro corretor",
		Email: "cliente@example.com", Phone: "11999999999",
		AssignedBrokerID: "corretor-2",
	}}
	r := leadReadRouter(repo, brokerProvider, "corretor-1", "corretor")

	w := doLeadRead(r)

	if w.Code == http.StatusOK {
		t.Fatalf("VAZAMENTO: corretor-1 recebeu o lead de corretor-2 (200): %s", w.Body.String())
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("esperava 404 (não confirma existência), veio %d", w.Code)
	}
}

func TestGetLeadByID_CorretorLeProprioLead(t *testing.T) {
	repo := &ownershipLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", Name: "Meu cliente",
		AssignedBrokerID: "corretor-1",
	}}
	r := leadReadRouter(repo, brokerProvider, "corretor-1", "corretor")

	w := doLeadRead(r)

	if w.Code != http.StatusOK {
		t.Fatalf("dono do lead deveria ler (HTTP %d): %s", w.Code, w.Body.String())
	}
}

func TestGetLeadByID_GestorLeQualquerLead(t *testing.T) {
	repo := &ownershipLeadRepo{item: &lead.Lead{
		ID: "lead-alheio", TenantID: "tenant-1", Name: "Cliente de outro corretor",
		AssignedBrokerID: "corretor-2",
	}}
	r := leadReadRouter(repo, elevatedProvider, "gestor-1", "gestor")

	w := doLeadRead(r)

	if w.Code != http.StatusOK {
		t.Fatalf("quem tem leads:reassign deveria ler qualquer lead (HTTP %d): %s", w.Code, w.Body.String())
	}
}
