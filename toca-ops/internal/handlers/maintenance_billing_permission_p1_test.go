package handlers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/toca-ops/internal/handlers"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
	"github.com/oginoino/toca/toca-ops/internal/services"
)

// P1/A12 — manutenção move dinheiro sem permissão financeira.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A12) mostrou que `CreateMaintenanceRequest`/`Update` aceitam
// `estimated/actual_cost`, `payer_role` e `billing_target`
// (`next_charge` | `next_transfer` | `standalone`), e o toca-financial converte
// isso em **cobrança do inquilino** ou **dedução no repasse do proprietário**
// (`maintenance_event_handler.go`; a conclusão reconcilia a diferença em
// `maintenance_completed_handler.go`) — sem nenhuma permissão financeira na rota:
// qualquer autenticado não-`cliente` movia dinheiro.
//
// O gate agora é: efeito financeiro ⇒ `charges:write` ou `finance:write`
// (admin/financeiro no catálogo). Chamado operacional sem custo continua livre
// para quem edita chamados, e a negação é fail-closed.
//
// O router é o REAL (`handlers.SetupRouter`) e o provider de permissões é o real
// (`auth.NewCorePermissionProvider`), alimentado por um toca-core dublê — o mesmo
// harness de `router_inspection_permission_test.go`.

// billingRepo registra se a escrita chegou ao repositório: a prova de que o
// dinheiro NÃO se moveu é esta, não só o status HTTP.
type billingRepo struct {
	maintenance.Repository
	mu      sync.Mutex
	creates []*maintenance.MaintenanceTicket
	updates []*maintenance.MaintenanceTicket
	ticket  *maintenance.MaintenanceTicket
}

func (r *billingRepo) Create(_ context.Context, t *maintenance.MaintenanceTicket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.creates = append(r.creates, t)
	return nil
}

func (r *billingRepo) Update(_ context.Context, t *maintenance.MaintenanceTicket) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updates = append(r.updates, t)
	return nil
}

func (r *billingRepo) GetByID(_ context.Context, tenantID, id string) (*maintenance.MaintenanceTicket, error) {
	if r.ticket == nil {
		return nil, nil
	}
	copied := *r.ticket
	copied.TenantID = tenantID
	copied.ID = id
	return &copied, nil
}

func (r *billingRepo) writes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.creates) + len(r.updates)
}

func billingRouter(t *testing.T, jwksURL string, repo *billingRepo) *gin.Engine {
	t.Helper()
	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwksURL

	return handlers.SetupRouter(
		cfg,
		services.NewMaintenanceService(repo, nil),
		services.NewKeyService(nil),
		services.NewInspectionService(nil, nil),
		services.NewTicketService(nil, nil),
		services.NewSupplierService(nil),
	)
}

// doBillingRequest envia corpo JSON com o JWT informado.
func doBillingRequest(r *gin.Engine, method, path, body, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const (
	createBillingBody = `{"type":"reparo","category":"hidraulica","property_id":"prop-1","requester":"Maria","profile":"inquilino","priority":"alta","description":"vazamento","estimated_cost":850,"payer_role":"tenant","billing_target":"next_charge"}`
	createPlainBody   = `{"type":"reparo","category":"hidraulica","property_id":"prop-1","requester":"Maria","profile":"inquilino","priority":"alta","description":"vazamento"}`
)

func billedTicket() *maintenance.MaintenanceTicket {
	return &maintenance.MaintenanceTicket{
		ID: "ticket-1", TenantID: "tenant-1", Type: "reparo", Category: "hidraulica",
		PropertyID: "prop-1", Requester: "Maria", Profile: "inquilino", Priority: "alta",
		Status: maintenance.StatusEmAnalise, EstimatedCost: 500, ActualCost: 700, PayerRole: "tenant",
	}
}

// Papel SEM permissão financeira tentando criar chamado com custo: 403 e nada
// chega ao repositório — o achado A12.
func TestMaintenanceBilling_OperatorWithoutFinancialPermissionCannotBill(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	repo := &billingRepo{}
	router := billingRouter(t, jwks.server.URL, repo)
	// matriz do `gestor`: edita chamado, não move dinheiro.
	core.set("rentals:inspect", "tickets:read", "tickets:write")

	w := doBillingRequest(router, http.MethodPost, "/api/v1/maintenance", createBillingBody, jwks.token(t, "tenant-1", "gestor"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("gestor (sem permissão financeira) criou chamado com cobrança: %d %s", w.Code, w.Body.String())
	}
	if repo.writes() != 0 {
		t.Fatalf("o chamado foi gravado mesmo com a recusa: %+v", repo.creates)
	}
}

// Papel COM permissão financeira: o chamado com custo é criado e o valor chega
// ao repositório (é o insumo do lançamento no financeiro).
func TestMaintenanceBilling_FinancialRoleCanBill(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	repo := &billingRepo{}
	router := billingRouter(t, jwks.server.URL, repo)
	core.set("rentals:inspect", "tickets:read", "tickets:write", "charges:write")

	w := doBillingRequest(router, http.MethodPost, "/api/v1/maintenance", createBillingBody, jwks.token(t, "tenant-1", "financeiro"))

	if w.Code != http.StatusCreated {
		t.Fatalf("financeiro deveria poder criar chamado com cobrança: %d %s", w.Code, w.Body.String())
	}
	if len(repo.creates) != 1 {
		t.Fatalf("esperava 1 criação, veio %d", len(repo.creates))
	}
	if repo.creates[0].EstimatedCost != 850 || repo.creates[0].PayerRole != "tenant" {
		t.Fatalf("custo/política não chegaram ao repositório: %+v", repo.creates[0])
	}
}

// Chamado operacional (sem custo nem política) continua permitido para quem
// edita chamados: o gate é sobre o dinheiro, não sobre a manutenção.
func TestMaintenanceBilling_OperationalTicketDoesNotRequireFinancialPermission(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	repo := &billingRepo{}
	router := billingRouter(t, jwks.server.URL, repo)
	core.set("rentals:inspect", "tickets:read", "tickets:write")

	w := doBillingRequest(router, http.MethodPost, "/api/v1/maintenance", createPlainBody, jwks.token(t, "tenant-1", "gestor"))

	if w.Code != http.StatusCreated {
		t.Fatalf("chamado sem efeito financeiro não deveria exigir permissão financeira: %d %s", w.Code, w.Body.String())
	}
	if len(repo.creates) != 1 {
		t.Fatalf("esperava 1 criação, veio %d", len(repo.creates))
	}
}

// Edição de texto passa; edição que introduz custo real (base do crédito/desconto
// na conclusão) não passa sem permissão financeira.
func TestMaintenanceBilling_UpdateCostRequiresFinancialPermission(t *testing.T) {
	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	t.Run("descrição", func(t *testing.T) {
		core := &permissionsServer{}
		coreSrv := core.start(t)
		defer coreSrv.Close()
		t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
		t.Setenv("REDIS_URI", "")
		core.set("rentals:inspect", "tickets:write")

		repo := &billingRepo{ticket: billedTicket()}
		router := billingRouter(t, jwks.server.URL, repo)
		w := doBillingRequest(router, http.MethodPut, "/api/v1/maintenance/ticket-1",
			`{"description":"vazamento no banheiro"}`, jwks.token(t, "tenant-1", "gestor"))
		if w.Code != http.StatusOK {
			t.Fatalf("edição sem efeito financeiro deveria passar: %d %s", w.Code, w.Body.String())
		}
	})

	t.Run("custo real", func(t *testing.T) {
		core := &permissionsServer{}
		coreSrv := core.start(t)
		defer coreSrv.Close()
		t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
		t.Setenv("REDIS_URI", "")
		core.set("rentals:inspect", "tickets:write")

		repo := &billingRepo{ticket: billedTicket()}
		router := billingRouter(t, jwks.server.URL, repo)
		w := doBillingRequest(router, http.MethodPut, "/api/v1/maintenance/ticket-1",
			`{"actual_cost":1200}`, jwks.token(t, "tenant-1", "gestor"))
		if w.Code != http.StatusForbidden {
			t.Fatalf("gestor lançou custo real sem permissão financeira: %d %s", w.Code, w.Body.String())
		}
		if repo.writes() != 0 {
			t.Fatal("o custo foi gravado mesmo com a recusa")
		}
	})
}

// Concluir chamado COM política de custo dispara `maintenance.completed` (o
// financeiro reconcilia o real contra o estimado). Com custo exige permissão
// financeira; sem custo, é operação de chamado.
func TestMaintenanceBilling_CompletingBilledTicketRequiresFinancialPermission(t *testing.T) {
	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	plain := *billedTicket()
	plain.EstimatedCost = 0
	plain.ActualCost = 0
	plain.PayerRole = ""

	newEnv := func(t *testing.T) *permissionsServer {
		t.Helper()
		core := &permissionsServer{}
		coreSrv := core.start(t)
		t.Cleanup(coreSrv.Close)
		t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
		t.Setenv("REDIS_URI", "")
		core.set("rentals:inspect", "tickets:write")
		return core
	}

	t.Run("com custo", func(t *testing.T) {
		newEnv(t)
		repo := &billingRepo{ticket: billedTicket()}
		router := billingRouter(t, jwks.server.URL, repo)
		w := doBillingRequest(router, http.MethodPatch, "/api/v1/maintenance/ticket-1/status",
			`{"status":"Concluído"}`, jwks.token(t, "tenant-1", "gestor"))
		if w.Code != http.StatusForbidden {
			t.Fatalf("conclusão de chamado com custo sem permissão financeira: %d %s", w.Code, w.Body.String())
		}
		if repo.writes() != 0 {
			t.Fatal("o status foi gravado mesmo com a recusa")
		}
	})

	t.Run("sem custo", func(t *testing.T) {
		newEnv(t)
		repo := &billingRepo{ticket: &plain}
		router := billingRouter(t, jwks.server.URL, repo)
		w := doBillingRequest(router, http.MethodPatch, "/api/v1/maintenance/ticket-1/status",
			`{"status":"Concluído"}`, jwks.token(t, "tenant-1", "gestor"))
		if w.Code != http.StatusOK {
			t.Fatalf("conclusão de chamado sem custo deveria passar: %d %s", w.Code, w.Body.String())
		}
	})
}

// Fail-closed: sem permissão financeira concedida (mapa do provider vazio), o
// efeito financeiro é negado em vez de liberado.
func TestMaintenanceBilling_NoGrantedPermissionDeniesBilling(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	repo := &billingRepo{}
	router := billingRouter(t, jwks.server.URL, repo)
	core.set() // nenhuma permissão

	w := doBillingRequest(router, http.MethodPost, "/api/v1/maintenance", createBillingBody, jwks.token(t, "tenant-1", "financeiro"))

	if w.Code != http.StatusForbidden {
		t.Fatalf("sem permissão concedida, o efeito financeiro deveria ser negado: %d %s", w.Code, w.Body.String())
	}
	if repo.writes() != 0 {
		t.Fatal("o chamado foi gravado sem autorização financeira")
	}
}

// A rota interna (portal do cliente → toca-ops) não expõe campos de cobrança no
// contrato do cliente; se alguém os enviar, a identidade de serviço não tem
// permissão financeira e a requisição é recusada (fail-closed).
func TestMaintenanceBilling_InternalServiceCallerCannotBill(t *testing.T) {
	core := &permissionsServer{}
	coreSrv := core.start(t)
	defer coreSrv.Close()
	t.Setenv("CORE_SERVICE_URL", coreSrv.URL)
	t.Setenv("REDIS_URI", "")

	repo := &billingRepo{}
	router := billingRouter(t, "", repo)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-1")

	body := `{"tenant_id":"tenant-1","type":"reparo","category":"hidraulica","property_id":"prop-1","requester":"Maria","profile":"cliente","priority":"alta","estimated_cost":9999,"payer_role":"tenant"}`
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ops/internal/maintenance-tickets", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Internal-Key", key)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusCreated || w.Code == http.StatusOK {
		t.Fatalf("SECURITY: identidade de serviço criou chamado com cobrança (%d): %s", w.Code, w.Body.String())
	}
	if repo.writes() != 0 {
		t.Fatal("o chamado foi gravado mesmo com a recusa")
	}
}
