package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/broker"
)

// P1 / A19 — leitura de corretor é tenant-scoped.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A19) mostrou `GET /brokers/:id` com `WHERE id = $1` (sem tenant) e
// guarda `RequirePermission(provider, "")` — qualquer autenticado não-`cliente`
// lia PII de corretor (nome, e-mail, telefone, papel, pesos de roleta) de
// qualquer tenant pelo UUID.

// observandoBrokerRepo registra com que tenant o handler chamou o repositório e
// devolve um corretor do tenant "vítima" quando o id é o alvo.
type observandoBrokerRepo struct {
	broker.Repository
	tenantRecebido string
}

func (r *observandoBrokerRepo) GetByID(_ context.Context, tenantID, id string) (*broker.Broker, error) {
	r.tenantRecebido = tenantID
	if id != "broker-da-vitima" {
		return nil, nil
	}
	// Espelha o SQL real (`WHERE id = $1 AND tenant_id = $2`): tenant diferente
	// do dono não encontra. Tenant VAZIO reproduz o comportamento ANTERIOR
	// (query só por id) — é o que a mutação do handler exercita.
	if tenantID != "" && tenantID != "tenant-vitima" {
		return nil, nil
	}
	return &broker.Broker{ID: id, TenantID: "tenant-vitima", Name: "Corretor da vítima", Email: "v@example.com"}, nil
}

func brokerRouter(repo broker.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/brokers/:id", NewBrokerHandler(repo).GetByID)
	return r
}

func doBrokerGet(r *gin.Engine, tenantID string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/brokers/broker-da-vitima", nil)
	ctx := req.Context()
	if tenantID != "" {
		ctx = pkgcontext.WithTenantID(ctx, tenantID)
	}
	r.ServeHTTP(w, req.WithContext(ctx))
	return w
}

func TestBrokerGetByID_PassaOTenantDoChamador(t *testing.T) {
	repo := &observandoBrokerRepo{}
	r := brokerRouter(repo)

	w := doBrokerGet(r, "tenant-atacante")

	if repo.tenantRecebido != "tenant-atacante" {
		t.Fatalf("handler chamou o repositório com tenant %q, want tenant-atacante", repo.tenantRecebido)
	}
	if w.Code == http.StatusOK {
		t.Fatalf("VAZAMENTO CROSS-TENANT: GET /brokers/:id devolveu 200 para corretor de outro tenant: %s", w.Body.String())
	}
}

func TestBrokerGetByID_SemTenantNoContexto_403(t *testing.T) {
	repo := &observandoBrokerRepo{}
	r := brokerRouter(repo)

	w := doBrokerGet(r, "")

	if w.Code != http.StatusForbidden {
		t.Fatalf("sem tenant no contexto: HTTP %d, want 403 (fail-closed)", w.Code)
	}
	if repo.tenantRecebido != "" {
		t.Fatalf("repositório foi consultado sem tenant (%q) — deveria nem chegar lá", repo.tenantRecebido)
	}
}

func TestBrokerGetByID_MesmoTenant_200(t *testing.T) {
	repo := &observandoBrokerRepo{}
	r := brokerRouter(repo)

	w := doBrokerGet(r, "tenant-vitima")

	if w.Code != http.StatusOK {
		t.Fatalf("dono do tenant deveria ler o corretor: HTTP %d (%s)", w.Code, w.Body.String())
	}
}
