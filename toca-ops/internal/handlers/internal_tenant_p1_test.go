package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/toca-ops/internal/handlers"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"github.com/oginoino/toca/toca-ops/internal/models/maintenance"
	"github.com/oginoino/toca/toca-ops/internal/models/ticket"
	"github.com/oginoino/toca/toca-ops/internal/services"
	"github.com/stretchr/testify/assert"
)

// P1 / A3 — as rotas /api/v1/ops/internal/* não aceitam mais o tenant escolhido
// pelo chamador.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A3) mostrou que os handlers internos liam o tenant do `?tenant_id=`
// (ou do corpo) com a chave interna compartilhada como única credencial:
// qualquer portador da chave lia/criava dados de qualquer tenant.
//
// Agora o grupo usa `auth.InternalOrUserMiddleware` e os handlers resolvem o
// tenant com `auth.ResolveTenantForInternalRequest` (helper internalTenant):
// credencial de serviço com allowlist só atende os tenants dela; chamada com JWT
// resolve pelo token e pedir outro tenant é 403. O dublê de repositório registra
// o tenant com que foi consultado — a asserção não é só o status, é a prova de
// que o tenant estrangeiro nunca chegou ao repositório.

// recordingInspectionRepo registra cada tenant que chegou ao repositório de
// vistorias (dublê por embedding: só o método observado é sobrescrito).
type recordingInspectionRepo struct {
	inspection.Repository
	mu      sync.Mutex
	tenants []string
}

func (r *recordingInspectionRepo) record(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Dedupe consecutivo: um mesmo request pode consultar o repositório duas
	// vezes (ex.: GetByID + Update) e as asserções continuam exatas.
	if n := len(r.tenants); n > 0 && r.tenants[n-1] == tenantID {
		return
	}
	r.tenants = append(r.tenants, tenantID)
}

func (r *recordingInspectionRepo) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.tenants...)
}

func (r *recordingInspectionRepo) List(_ context.Context, f inspection.ListFilter) ([]*inspection.Inspection, int64, error) {
	r.record(f.TenantID)
	return []*inspection.Inspection{}, 0, nil
}

// recordingTicketRepo registra o tenant de toda leitura/escrita de chamado.
type recordingTicketRepo struct {
	ticket.Repository
	mu      sync.Mutex
	tenants []string
}

func (r *recordingTicketRepo) record(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Dedupe consecutivo: um mesmo request pode consultar o repositório duas
	// vezes (ex.: GetByID + Update) e as asserções continuam exatas.
	if n := len(r.tenants); n > 0 && r.tenants[n-1] == tenantID {
		return
	}
	r.tenants = append(r.tenants, tenantID)
}

func (r *recordingTicketRepo) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.tenants...)
}

func (r *recordingTicketRepo) Create(_ context.Context, t *ticket.Ticket) error {
	r.record(t.TenantID)
	return nil
}

func (r *recordingTicketRepo) GetByID(_ context.Context, tenantID, id string) (*ticket.Ticket, error) {
	r.record(tenantID)
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("ticket not found: tenant context required")
	}
	return &ticket.Ticket{ID: id, TenantID: tenantID}, nil
}

func (r *recordingTicketRepo) Update(_ context.Context, t *ticket.Ticket) error {
	r.record(t.TenantID)
	return nil
}

func (r *recordingTicketRepo) ListByRequester(_ context.Context, tenantID, _ string) ([]*ticket.Ticket, error) {
	r.record(tenantID)
	return []*ticket.Ticket{}, nil
}

// recordingMaintenanceRepo registra o tenant de toda leitura/escrita de
// manutenção.
type recordingMaintenanceRepo struct {
	maintenance.Repository
	mu      sync.Mutex
	tenants []string
}

func (r *recordingMaintenanceRepo) record(tenantID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Dedupe consecutivo: um mesmo request pode consultar o repositório duas
	// vezes (ex.: GetByID + Update) e as asserções continuam exatas.
	if n := len(r.tenants); n > 0 && r.tenants[n-1] == tenantID {
		return
	}
	r.tenants = append(r.tenants, tenantID)
}

func (r *recordingMaintenanceRepo) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.tenants...)
}

func (r *recordingMaintenanceRepo) Create(_ context.Context, t *maintenance.MaintenanceTicket) error {
	r.record(t.TenantID)
	return nil
}

func (r *recordingMaintenanceRepo) ListByProperty(_ context.Context, tenantID, _ string, _, _ int64) ([]*maintenance.MaintenanceTicket, int64, error) {
	r.record(tenantID)
	return []*maintenance.MaintenanceTicket{}, 0, nil
}

// withNamedServiceKey publica uma credencial NOMEADA com allowlist de tenants no
// registro global lido pelo middleware (mesmo caminho de INTERNAL_SERVICE_KEYS em
// produção) e devolve a chave a apresentar no header X-Internal-Key.
func withNamedServiceKey(t *testing.T, service, key string, tenants ...string) string {
	t.Helper()
	entry := service + ":" + key
	if len(tenants) > 0 {
		entry += ":" + strings.Join(tenants, "|")
	}
	prev, had := os.LookupEnv("INTERNAL_SERVICE_KEYS")
	if err := os.Setenv("INTERNAL_SERVICE_KEYS", entry); err != nil {
		t.Fatalf("set INTERNAL_SERVICE_KEYS: %v", err)
	}
	auth.ReloadServiceKeyRegistry()
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("INTERNAL_SERVICE_KEYS", prev)
		} else {
			_ = os.Unsetenv("INTERNAL_SERVICE_KEYS")
		}
		auth.ReloadServiceKeyRegistry()
	})
	return key
}

// opsRouter sobe o router REAL do serviço (handlers.SetupRouter): rotas,
// middlewares e handlers de produção, com repositórios dublês.
func opsRouter(t *testing.T, jwksURL string, insp *recordingInspectionRepo, tkt *recordingTicketRepo, maint *recordingMaintenanceRepo) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := config.BaseConfig{}
	cfg.JWT.SupabaseJWKSURL = jwksURL
	inspSvc := services.NewInspectionService(insp, nil)
	return handlers.SetupRouter(
		cfg,
		services.NewMaintenanceService(maint, nil),
		services.NewKeyService(nil),
		inspSvc,
		services.NewTicketService(tkt, nil),
		services.NewSupplierService(nil),
	)
}

func doInternalRequest(t *testing.T, r *gin.Engine, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	} else {
		reader = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

const inspectionsPath = "/api/v1/ops/internal/inspections?contract_id=c1"

func newOpsFakes() (*recordingInspectionRepo, *recordingTicketRepo, *recordingMaintenanceRepo) {
	return &recordingInspectionRepo{}, &recordingTicketRepo{}, &recordingMaintenanceRepo{}
}

// Sem chave interna e sem JWT a rota interna não responde nada de negócio:
// prova que o tenant não vem do query param.
func TestInternalOps_RejectsRequestWithoutInternalIdentity(t *testing.T) {
	insp, tkt, maint := newOpsFakes()
	r := opsRouter(t, "", insp, tkt, maint)

	w := doInternalRequest(t, r, http.MethodGet, inspectionsPath+"&tenant_id=tenant-vitima", "", nil)

	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, w.Code,
		"rota interna respondeu %d sem identidade de serviço/usuário: %s", w.Code, w.Body.String())
	assert.Empty(t, insp.seen(), "o repositório não pode ser consultado sem identidade")
}

// (a) Credencial de SERVIÇO restrita a tenant-a pedindo tenant-vitima: 403 e o
// repositório NUNCA vê o tenant estrangeiro — era exatamente o cenário do A3.
func TestInternalInspections_ServiceCredentialCannotPickForeignTenant(t *testing.T) {
	insp, tkt, maint := newOpsFakes()
	r := opsRouter(t, "", insp, tkt, maint)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

	w := doInternalRequest(t, r, http.MethodGet, inspectionsPath+"&tenant_id=tenant-vitima", "",
		map[string]string{"X-Internal-Key": key})

	assert.Equal(t, http.StatusForbidden, w.Code,
		"VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a pediu tenant-vitima e recebeu %d (tenants consultados: %v): %s",
		w.Code, insp.seen(), w.Body.String())
	assert.Empty(t, insp.seen(), "o repositório foi consultado com %v — a requisição nem deveria chegar lá", insp.seen())
}

// (a-legítimo) A mesma credencial no tenant DA ALLOWLIST continua funcionando e
// chega ao repositório com o tenant certo.
func TestInternalInspections_ServiceCredentialOwnTenant_ReachesRepository(t *testing.T) {
	insp, tkt, maint := newOpsFakes()
	r := opsRouter(t, "", insp, tkt, maint)
	key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

	w := doInternalRequest(t, r, http.MethodGet, inspectionsPath+"&tenant_id=tenant-a", "",
		map[string]string{"X-Internal-Key": key})

	assert.Equal(t, http.StatusOK, w.Code, "credencial no próprio tenant deve funcionar: %s", w.Body.String())
	assert.Equal(t, []string{"tenant-a"}, insp.seen(), "o repositório deve ser consultado com o tenant da credencial")
}

// (b) JWT de usuário: o tenant do TOKEN manda. Pedir outro é 403; o mesmo tenant
// (ou nenhum tenant no query) segue funcionando.
func TestInternalInspections_UserTokenTenantWins(t *testing.T) {
	jwks := newTestJWKS(t)
	defer jwks.server.Close()

	insp, tkt, maint := newOpsFakes()
	r := opsRouter(t, jwks.server.URL, insp, tkt, maint)
	token := jwks.token(t, "tenant-do-token", "operacoes")

	w := doInternalRequest(t, r, http.MethodGet, inspectionsPath+"&tenant_id=tenant-vitima", "",
		map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusForbidden, w.Code,
		"VAZAMENTO CROSS-TENANT: token de tenant-do-token pediu tenant-vitima e recebeu %d (tenants consultados: %v)",
		w.Code, insp.seen())
	assert.Empty(t, insp.seen(), "o repositório foi consultado com %v — a requisição nem deveria chegar lá", insp.seen())

	w = doInternalRequest(t, r, http.MethodGet, inspectionsPath, "", map[string]string{"Authorization": "Bearer " + token})
	assert.Equal(t, http.StatusOK, w.Code, "chamada de usuário sem tenant explícito usa o do token: %s", w.Body.String())
	assert.Equal(t, []string{"tenant-do-token"}, insp.seen(), "o repositório deve ser consultado com o tenant do TOKEN")
}

// Serviço precisa dizer o tenant: sem `?tenant_id=` o resolver recusa (era o caso
// do cliente que dependia do tenant vazio para "listar tudo").
func TestInternalInspections_ServiceWithoutTenantIsRejected(t *testing.T) {
	insp, tkt, maint := newOpsFakes()
	r := opsRouter(t, "", insp, tkt, maint)
	key := withNamedServiceKey(t, "toca-deal", "key-deal")

	w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/inspections?contract_id=c1", "",
		map[string]string{"X-Internal-Key": key})

	assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, w.Code,
		"serviço sem tenant explícito deveria ser recusado como validação, veio %d", w.Code)
	assert.Empty(t, insp.seen())
}

// POST /ops/internal/tickets — o tenant chega no CORPO. Credencial restrita a
// tenant-a pedindo tenant-vitima no JSON: 403 e nada gravado.
func TestInternalTickets_Create_BodyTenantIsEnforced(t *testing.T) {
	const body = `{"tenant_id":"%s","subject":"Ar-condicionado","requester_account_id":"acc-1"}`

	t.Run("serviço com tenant estrangeiro no corpo", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/tickets",
			strings.Replace(body, "%s", "tenant-vitima", 1), map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusForbidden, w.Code,
			"VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a criou chamado em tenant-vitima (%d, tenants consultados: %v)",
			w.Code, tkt.seen())
		assert.Empty(t, tkt.seen(), "nenhuma escrita pode chegar ao repositório com tenant estrangeiro")
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/tickets",
			strings.Replace(body, "%s", "tenant-a", 1), map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusCreated, w.Code, "criação legítima deve continuar funcionando: %s", w.Body.String())
		assert.Equal(t, []string{"tenant-a"}, tkt.seen(), "o chamado deve ser gravado no tenant da credencial")
	})

	t.Run("usuário pedindo outro tenant no corpo", func(t *testing.T) {
		jwks := newTestJWKS(t)
		defer jwks.server.Close()

		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, jwks.server.URL, insp, tkt, maint)
		token := jwks.token(t, "tenant-a", "operacoes")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/tickets",
			strings.Replace(body, "%s", "tenant-vitima", 1), map[string]string{"Authorization": "Bearer " + token})

		assert.Equal(t, http.StatusForbidden, w.Code,
			"VAZAMENTO CROSS-TENANT: token de tenant-a criou chamado em tenant-vitima (%d, tenants consultados: %v)",
			w.Code, tkt.seen())
		assert.Empty(t, tkt.seen())
	})
}

// GET /ops/internal/tickets/:id e /by-requester/:accountID — tenant no query.
func TestInternalTickets_QueryTenantIsEnforced(t *testing.T) {
	t.Run("by-id com tenant estrangeiro", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/tickets/tkt-1?tenant_id=tenant-vitima", "",
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusForbidden, w.Code,
			"VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a leu chamado de tenant-vitima (%d, %s)",
			w.Code, w.Body.String())
		assert.Empty(t, tkt.seen())
	})

	t.Run("by-id no próprio tenant", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/tickets/tkt-1?tenant_id=tenant-a", "",
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusOK, w.Code, "%s", w.Body.String())
		assert.Equal(t, []string{"tenant-a"}, tkt.seen())
	})

	t.Run("by-requester com tenant estrangeiro", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/tickets/by-requester/acc-1?tenant_id=tenant-vitima", "",
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusForbidden, w.Code, "%d", w.Code)
		assert.Empty(t, tkt.seen())
	})

	t.Run("by-requester no próprio tenant", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/tickets/by-requester/acc-1?tenant_id=tenant-a", "",
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusOK, w.Code, "%s", w.Body.String())
		assert.Equal(t, []string{"tenant-a"}, tkt.seen())
	})

	t.Run("comentário com tenant estrangeiro", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/tickets/tkt-1/comments?tenant_id=tenant-vitima",
			`{"author_id":"acc-1","author_type":"operator","content":"oi"}`,
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusForbidden, w.Code, "%d", w.Code)
		assert.Empty(t, tkt.seen(), "nenhum comentário pode ser escrito em chamado de outro tenant")
	})

	t.Run("comentário no próprio tenant", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/tickets/tkt-1/comments?tenant_id=tenant-a",
			`{"author_id":"acc-1","author_type":"operator","content":"oi"}`,
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusCreated, w.Code, "%s", w.Body.String())
		assert.Equal(t, []string{"tenant-a"}, tkt.seen())
	})
}

// POST /ops/internal/maintenance-tickets — tenant no corpo.
func TestInternalMaintenance_Create_BodyTenantIsEnforced(t *testing.T) {
	const body = `{"tenant_id":"%s","type":"corretiva","category":"hidraulica","property_id":"prop-1","requester":"Maria","profile":"inquilino","priority":"alta"}`

	t.Run("serviço com tenant estrangeiro no corpo", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/maintenance-tickets",
			strings.Replace(body, "%s", "tenant-vitima", 1), map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusForbidden, w.Code,
			"VAZAMENTO CROSS-TENANT: credencial restrita a tenant-a abriu manutenção em tenant-vitima (%d, tenants consultados: %v)",
			w.Code, maint.seen())
		assert.Empty(t, maint.seen())
	})

	t.Run("serviço no próprio tenant", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/maintenance-tickets",
			strings.Replace(body, "%s", "tenant-a", 1), map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusCreated, w.Code, "%s", w.Body.String())
		assert.Equal(t, []string{"tenant-a"}, maint.seen())
	})

	t.Run("usuário pedindo outro tenant no corpo", func(t *testing.T) {
		jwks := newTestJWKS(t)
		defer jwks.server.Close()

		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, jwks.server.URL, insp, tkt, maint)
		token := jwks.token(t, "tenant-a", "operacoes")

		w := doInternalRequest(t, r, http.MethodPost, "/api/v1/ops/internal/maintenance-tickets",
			strings.Replace(body, "%s", "tenant-vitima", 1), map[string]string{"Authorization": "Bearer " + token})

		assert.Equal(t, http.StatusForbidden, w.Code, "%d (tenants consultados: %v)", w.Code, maint.seen())
		assert.Empty(t, maint.seen())
	})
}

// GET /ops/internal/properties/:id/maintenance-tickets — o tenant vinha do query
// e o cliente interno (pkg/clients) não manda nenhum: passa a falhar fechado em
// vez de listar com tenant vazio/"escolhido".
func TestInternalMaintenance_ListByProperty_TenantIsEnforced(t *testing.T) {
	t.Run("tenant estrangeiro", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/properties/prop-1/maintenance-tickets?tenant_id=tenant-vitima", "",
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusForbidden, w.Code, "%d", w.Code)
		assert.Empty(t, maint.seen())
	})

	t.Run("próprio tenant", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal", "tenant-a")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/properties/prop-1/maintenance-tickets?tenant_id=tenant-a", "",
			map[string]string{"X-Internal-Key": key})

		assert.Equal(t, http.StatusOK, w.Code, "%s", w.Body.String())
		assert.Equal(t, []string{"tenant-a"}, maint.seen())
	})

	t.Run("serviço sem tenant explícito", func(t *testing.T) {
		insp, tkt, maint := newOpsFakes()
		r := opsRouter(t, "", insp, tkt, maint)
		key := withNamedServiceKey(t, "toca-deal", "key-deal")

		w := doInternalRequest(t, r, http.MethodGet, "/api/v1/ops/internal/properties/prop-1/maintenance-tickets", "",
			map[string]string{"X-Internal-Key": key})

		assert.Contains(t, []int{http.StatusBadRequest, http.StatusUnprocessableEntity}, w.Code,
			"serviço sem tenant explícito deveria ser recusado como validação, veio %d", w.Code)
		assert.Empty(t, maint.seen())
	})
}
