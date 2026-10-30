package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/auth"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// M11 — waive/adjust de valores sem limiar nem aprovador.
//
// A revisão de permissões de 2026-09-19 registrou que a MESMA permissão de
// escrita do dia a dia (`charges:write`) concedia desconto, perdoava multa/juros
// e criava dedução de comissão de qualquer valor. O teste cobre as três camadas:
// a aritmética do ajuste, a decisão do limiar e o caminho HTTP (403 sem a
// permissão dedicada, 200 com ela, e trilha de auditoria nos dois casos).

type denyingAdjustmentProvider struct{}

func (denyingAdjustmentProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	return map[string]bool{}, nil
}

type allowingAdjustmentProvider struct{}

func (allowingAdjustmentProvider) GetRolesMap(context.Context, string, string) (map[string]bool, error) {
	// `HasPermission` consulta o mapa pelo PAPEL FUNCIONAL do chamador.
	return map[string]bool{"admin": true, "financeiro": true, "gestor": true}, nil
}

func adjustmentCtx() *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/charges/c-1", nil)
	ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-1")
	ctx = pkgcontext.WithAccountID(ctx, "user-1")
	ctx = pkgcontext.WithFunctionalRole(ctx, "financeiro")
	c.Request = c.Request.WithContext(ctx)
	c.Set("tenant_id", "tenant-1")
	c.Set("user_id", "user-1")
	c.Set("functional_role", "financeiro")
	c.Set("permission", "")
	c.Set("type", "individual")
	return c
}

func TestActorCanApproveAdjustment_FailsClosed(t *testing.T) {
	if actorCanApproveAdjustment(adjustmentCtx(), nil) {
		t.Fatal("provider nil liberou aprovação (fail-open) — a segunda assinatura não pode depender de wiring")
	}
	if actorCanApproveAdjustment(adjustmentCtx(), denyingAdjustmentProvider{}) {
		t.Fatal("provider sem a permissão liberou aprovação")
	}
	if !actorCanApproveAdjustment(adjustmentCtx(), allowingAdjustmentProvider{}) {
		t.Fatal("permissão presente não liberou aprovação (fluxo legítimo quebrado)")
	}
}

func TestAdjustmentThresholdFromEnv(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "")
	if got := adjustmentApprovalThresholdBRL(); got != defaultAdjustmentApprovalThresholdBRL {
		t.Fatalf("default = %v, want %v", got, defaultAdjustmentApprovalThresholdBRL)
	}
	t.Setenv(adjustmentThresholdEnv, "250")
	if got := adjustmentApprovalThresholdBRL(); got != 250 {
		t.Fatalf("valor configurado = %v, want 250", got)
	}
	t.Setenv(adjustmentThresholdEnv, "0")
	if got := adjustmentApprovalThresholdBRL(); got != 0 {
		t.Fatalf("zero deveria significar 'toda alteração exige aprovação'; veio %v", got)
	}
	t.Setenv(adjustmentThresholdEnv, "valor-torto")
	if got := adjustmentApprovalThresholdBRL(); got != defaultAdjustmentApprovalThresholdBRL {
		t.Fatalf("env inválido deveria cair no default (nunca em 'sem limiar'); veio %v", got)
	}
	t.Setenv(adjustmentThresholdEnv, "-10")
	if got := adjustmentApprovalThresholdBRL(); got != defaultAdjustmentApprovalThresholdBRL {
		t.Fatalf("env negativo deveria cair no default; veio %v", got)
	}
}

func TestChargeAdjustmentMagnitude(t *testing.T) {
	base := func(total, discount, lateFee, interest float64) charge.Charge {
		var c charge.Charge
		c.TotalAmount = valueobjects.NewMoneyFromFloat(total)
		c.Discount = valueobjects.NewMoneyFromFloat(discount)
		c.LateFee = valueobjects.NewMoneyFromFloat(lateFee)
		c.Interest = valueobjects.NewMoneyFromFloat(interest)
		return c
	}

	tests := []struct {
		name           string
		before, after  charge.Charge
		wantMagnitude  float64
		wantApprovalAt float64 // limiar usado na checagem de decisão
		wantRequires   bool
	}{
		{
			name:   "desconto grande move o total",
			before: base(1000, 0, 0, 0), after: base(700, 300, 0, 0),
			wantMagnitude: 300, wantApprovalAt: 200, wantRequires: true,
		},
		{
			name:   "desconto compensado por multa não some da medida",
			before: base(1000, 0, 0, 0), after: base(1000, 400, 400, 0),
			wantMagnitude: 800, wantApprovalAt: 500, wantRequires: true,
		},
		{
			name:   "ajuste pequeno passa sem segunda assinatura",
			before: base(1000, 0, 0, 0), after: base(1040, 0, 40, 0),
			wantMagnitude: 40, wantApprovalAt: 200, wantRequires: false,
		},
		{
			name:   "sem mudança não exige aprovação nem com limiar zero",
			before: base(1000, 10, 5, 5), after: base(1000, 10, 5, 5),
			wantMagnitude: 0, wantApprovalAt: 0, wantRequires: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chargeAdjustmentMagnitude(tt.before, tt.after)
			if got != tt.wantMagnitude {
				t.Fatalf("magnitude = %v, want %v", got, tt.wantMagnitude)
			}
			if requires := adjustmentRequiresApproval(got, tt.wantApprovalAt); requires != tt.wantRequires {
				t.Fatalf("adjustmentRequiresApproval(%v, %v) = %v, want %v", got, tt.wantApprovalAt, requires, tt.wantRequires)
			}
		})
	}
}

// ───────────────────────── caminho HTTP ─────────────────────────

type adjustmentAuditSink struct {
	entries []audit.Entry
}

func withAdjustmentAudit(t *testing.T) *adjustmentAuditSink {
	t.Helper()
	sink := &adjustmentAuditSink{}
	cleanup := audit.SetTestSink(func(e audit.Entry) { sink.entries = append(sink.entries, e) })
	t.Cleanup(cleanup)
	return sink
}

func (s *adjustmentAuditSink) find(action string) (audit.Entry, bool) {
	for _, e := range s.entries {
		if e.Action == action {
			return e, true
		}
	}
	return audit.Entry{}, false
}

func setupChargeUpdateRouter(repo *mockChargeRepo, provider auth.PermissionProvider) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewChargeHandler(service.NewChargeService(repo, nil)).WithPermissionProvider(provider)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-123")
		ctx = pkgcontext.WithAccountID(ctx, "financeiro-1")
		ctx = pkgcontext.WithFunctionalRole(ctx, "financeiro")
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", "tenant-123")
		c.Set("user_id", "financeiro-1")
		c.Set("functional_role", "financeiro")
		c.Set("permission", "")
		c.Next()
	})
	r.PUT("/charges/:id", handler.Update)
	return r
}

func doChargeUpdate(t *testing.T, repo *mockChargeRepo, provider auth.PermissionProvider, body string) *httptest.ResponseRecorder {
	t.Helper()
	r := setupChargeUpdateRouter(repo, provider)
	req := httptest.NewRequest(http.MethodPut, "/charges/c-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func seededChargeRepo(t *testing.T) *mockChargeRepo {
	t.Helper()
	repo := &mockChargeRepo{charges: map[string]*charge.Charge{}}
	c, err := charge.New("contract-1", "party-1", "owner-1", "prop-1", "tenant-123", charge.TypeRent, "Aluguel", "2026-09", 1000, time.Now().Add(72*time.Hour))
	if err != nil {
		t.Fatalf("charge.New: %v", err)
	}
	c.ID = "c-1"
	c.Status = charge.StatusPending
	repo.charges[c.ID] = c
	return repo
}

// Desconto acima do limiar sem a permissão dedicada: 403, nada persistido e
// trilha de negação.
func TestChargeUpdate_LargeAdjustmentRequiresApproval(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "200")
	sink := withAdjustmentAudit(t)
	repo := seededChargeRepo(t)

	w := doChargeUpdate(t, repo, denyingAdjustmentProvider{}, `{"discount":900}`)

	if w.Code != http.StatusForbidden {
		t.Fatalf("ajuste acima do limiar sem aprovação respondeu %d, want 403: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), AdjustmentApprovalPermission) {
		t.Fatalf("mensagem não diz qual permissão falta: %s", w.Body.String())
	}
	if repo.updateCalls != 0 {
		t.Fatalf("o ajuste foi persistido apesar da recusa (updateCalls=%d)", repo.updateCalls)
	}
	entry, ok := sink.find("finance.charge_adjustment_denied")
	if !ok {
		t.Fatal("negação sem trilha de auditoria")
	}
	if entry.Metadata["required_permission"] != AdjustmentApprovalPermission {
		t.Fatalf("trilha sem a permissão exigida: %+v", entry.Metadata)
	}
	if entry.Metadata["magnitude_brl"].(float64) < 200 {
		t.Fatalf("trilha sem a magnitude do ajuste: %+v", entry.Metadata)
	}
}

// Com a permissão dedicada, o ajuste passa e fica auditado com a decisão.
func TestChargeUpdate_LargeAdjustmentApprovedWithPermission(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "200")
	sink := withAdjustmentAudit(t)
	repo := seededChargeRepo(t)

	w := doChargeUpdate(t, repo, allowingAdjustmentProvider{}, `{"discount":900}`)

	if w.Code != http.StatusOK {
		t.Fatalf("ajuste aprovado respondeu %d, want 200: %s", w.Code, w.Body.String())
	}
	if repo.updateCalls != 1 {
		t.Fatalf("update do ajuste aprovado não chegou ao serviço (updateCalls=%d)", repo.updateCalls)
	}
	entry, ok := sink.find("finance.charge_adjusted")
	if !ok {
		t.Fatal("ajuste aprovado sem trilha")
	}
	if approved, _ := entry.Metadata["approved"].(bool); !approved {
		t.Fatalf("trilha não registra a aprovação: %+v", entry.Metadata)
	}
	if required, _ := entry.Metadata["approval_required"].(bool); !required {
		t.Fatalf("trilha não registra que o limiar foi cruzado: %+v", entry.Metadata)
	}
}

// Ajuste pequeno continua no fluxo normal (sem segunda assinatura) — o
// endurecimento não pode parar a operação do dia a dia.
func TestChargeUpdate_SmallAdjustmentNeedsNoApproval(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "500")
	sink := withAdjustmentAudit(t)
	repo := seededChargeRepo(t)

	w := doChargeUpdate(t, repo, denyingAdjustmentProvider{}, `{"late_fee":50}`)

	if w.Code != http.StatusOK {
		t.Fatalf("ajuste pequeno respondeu %d, want 200: %s", w.Code, w.Body.String())
	}
	if got := repo.charges["c-1"].LateFee.Float(); got != 50 {
		t.Fatalf("multa não aplicada: %v", got)
	}
	entry, ok := sink.find("finance.charge_adjusted")
	if !ok {
		t.Fatal("ajuste pequeno sem trilha")
	}
	if required, _ := entry.Metadata["approval_required"].(bool); required {
		t.Fatalf("ajuste pequeno marcado como exigindo aprovação: %+v", entry.Metadata)
	}
}

// Sem provider wired (regressão de wiring) o gate falha FECHADO.
func TestChargeUpdate_GateFailsClosedWithoutProvider(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "100")
	repo := seededChargeRepo(t)

	w := doChargeUpdate(t, repo, nil, `{"amount":5000}`)

	if w.Code != http.StatusForbidden {
		t.Fatalf("provider ausente liberou ajuste grande (%d): gate fail-open", w.Code)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("provider ausente persistiu o ajuste (updateCalls=%d): gate fail-open", repo.updateCalls)
	}
}

var _ = json.Marshal

// ───────────── deduções de comissão (M11: "cobranças e deduções") ─────────────

type deductionGateRepo struct {
	service.CommissionRepository
	item        *commission.Commission
	updateCalls int
}

func (r *deductionGateRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	clone := *r.item
	clone.Deductions = append([]commission.Deduction(nil), r.item.Deductions...)
	return &clone, nil
}

func (r *deductionGateRepo) Update(_ context.Context, updated *commission.Commission) error {
	r.updateCalls++
	clone := *updated
	clone.Deductions = append([]commission.Deduction(nil), updated.Deductions...)
	r.item = &clone
	return nil
}

func doAddDeduction(t *testing.T, repo *deductionGateRepo, provider auth.PermissionProvider, amount string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	handler := NewCommissionHandler(service.NewCommissionService(repo)).WithPermissionProvider(provider)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), repo.item.TenantID)
		ctx = pkgcontext.WithFunctionalRole(ctx, "financeiro")
		c.Request = c.Request.WithContext(ctx)
		c.Set("tenant_id", repo.item.TenantID)
		c.Set("functional_role", "financeiro")
		c.Set("permission", "")
		c.Next()
	})
	router.POST("/commissions/:id/deductions", handler.AddDeduction)

	req := httptest.NewRequest(http.MethodPost, "/commissions/"+repo.item.ID+"/deductions",
		strings.NewReader(`{"kind":"OUTROS","description":"abatimento","amount":`+amount+`}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestCommissionDeduction_AboveThresholdRequiresApproval(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "200")
	sink := withAdjustmentAudit(t)
	item := commission.New("deal-1", "broker-1", 1000, "tenant-1")
	item.ID = "commission-1"
	repo := &deductionGateRepo{item: item}

	w := doAddDeduction(t, repo, denyingAdjustmentProvider{}, "900")

	if w.Code != http.StatusForbidden {
		t.Fatalf("dedução acima do limiar sem aprovação respondeu %d, want 403: %s", w.Code, w.Body.String())
	}
	if repo.updateCalls != 0 {
		t.Fatalf("dedução persistida apesar da recusa (updateCalls=%d)", repo.updateCalls)
	}
	if entry, ok := sink.find("finance.commission_deduction_denied"); !ok {
		t.Fatal("negação de dedução sem trilha")
	} else if entry.Metadata["magnitude_brl"].(float64) != 900 {
		t.Fatalf("trilha sem a magnitude: %+v", entry.Metadata)
	}
}

func TestCommissionDeduction_ApprovedWithPermission(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "200")
	sink := withAdjustmentAudit(t)
	item := commission.New("deal-1", "broker-1", 1000, "tenant-1")
	item.ID = "commission-1"
	repo := &deductionGateRepo{item: item}

	w := doAddDeduction(t, repo, allowingAdjustmentProvider{}, "900")

	if w.Code != http.StatusOK {
		t.Fatalf("dedução aprovada respondeu %d, want 200: %s", w.Code, w.Body.String())
	}
	if repo.updateCalls != 1 {
		t.Fatalf("dedução aprovada não chegou ao serviço (updateCalls=%d)", repo.updateCalls)
	}
	if _, ok := sink.find("finance.commission_deduction_approved"); !ok {
		t.Fatal("dedução aprovada sem trilha")
	}
}

func TestCommissionDeduction_SmallAmountNeedsNoApproval(t *testing.T) {
	t.Setenv(adjustmentThresholdEnv, "500")
	item := commission.New("deal-1", "broker-1", 1000, "tenant-1")
	item.ID = "commission-1"
	repo := &deductionGateRepo{item: item}

	w := doAddDeduction(t, repo, denyingAdjustmentProvider{}, "100")

	if w.Code != http.StatusOK {
		t.Fatalf("dedução pequena respondeu %d, want 200: %s", w.Code, w.Body.String())
	}
}
