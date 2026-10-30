package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan_draft"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// readinessDraftRepo implementa o mínimo do draft.Repository para o teste do
// endpoint de readiness (T-01).
type readinessDraftRepo struct {
	draft *sale_billing_plan_draft.Draft
	// serviceReads conta as leituras feitas pelo caminho de serviço: a rota é
	// interna, sem claims de JWT, e o pool do app não enxerga o draft.
	serviceReads int
}

func (r *readinessDraftRepo) Upsert(ctx context.Context, d *sale_billing_plan_draft.Draft) error {
	return nil
}
func (r *readinessDraftRepo) GetActiveByDealForService(_ context.Context, _, _ string) (*sale_billing_plan_draft.Draft, error) {
	r.serviceReads++
	return r.draft, nil
}

func (r *readinessDraftRepo) GetActiveByDeal(_ context.Context, _, _ string) (*sale_billing_plan_draft.Draft, error) {
	return r.draft, nil
}
func (r *readinessDraftRepo) MarkConsumed(ctx context.Context, tenantID, dealID string) error {
	return nil
}

func readinessEngine(repo sale_billing_plan_draft.Repository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	svc := service.NewSaleBillingPlanService(nil, nil).WithDraftRepo(repo)
	h := NewSaleBillingPlanHandler(svc)
	r := gin.New()
	r.GET("/financial/internal/deals/:id/sale-plan-readiness", func(c *gin.Context) {
		// Simula o middleware InternalOrUserMiddleware com um JWT de usuário
		// do tenant-1 (caminho de resolução de tenant pela credencial).
		c.Set(auth.CtxInternalAuthKind, auth.InternalAuthUser)
		c.Set("tenant_id", "tenant-1")
		h.SalePlanReadiness(c)
	})
	return r
}

func readinessGet(engine *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	return w
}

func TestSalePlanReadiness_NoDraftIsInactive(t *testing.T) {
	engine := readinessEngine(&readinessDraftRepo{})
	w := readinessGet(engine, "/financial/internal/deals/deal-1/sale-plan-readiness")
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
	var body struct {
		Data SalePlanReadinessResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Data.Active {
		t.Fatalf("sem draft deveria responder active=false: %+v", body.Data)
	}
}

func TestSalePlanReadiness_ActiveDraftCarriesTotalAndMode(t *testing.T) {
	draft := &sale_billing_plan_draft.Draft{
		ID:               "draft-1",
		TenantID:         "tenant-1",
		DealID:           "deal-1",
		Mode:             sale_billing_plan.ModeInstallments,
		TotalAmountCents: 100000,
		InstallmentCount: 10,
		FirstDueDate:     time.Now().Add(30 * 24 * time.Hour),
	}
	repo := &readinessDraftRepo{draft: draft}
	engine := readinessEngine(repo)
	w := readinessGet(engine, "/financial/internal/deals/deal-1/sale-plan-readiness")
	if w.Code != http.StatusOK {
		t.Fatalf("esperava 200, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
	var body struct {
		Data SalePlanReadinessResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.Data.Active || body.Data.TotalAmountCents != 100000 || body.Data.Mode != string(sale_billing_plan.ModeInstallments) {
		t.Fatalf("readiness divergente: %+v", body.Data)
	}
	// A leitura tem de sair pelo pool de serviço: sem claims de JWT, o pool do
	// app não enxerga o draft e o plano configurado era reportado como ausente
	// (o contrato de venda era recusado com 422).
	if repo.serviceReads != 1 {
		t.Fatalf("readiness não usou a leitura de serviço (%d leituras)", repo.serviceReads)
	}
}

func TestSalePlanReadiness_MissingDealIDIs400(t *testing.T) {
	engine := readinessEngine(&readinessDraftRepo{})
	w := readinessGet(engine, "/financial/internal/deals//sale-plan-readiness")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400, veio %d (corpo=%s)", w.Code, w.Body.String())
	}
}
