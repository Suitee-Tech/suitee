package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// payRequestWith mounts the pay route and performs one request with the given
// functional role in the authenticated context.
func payRequestWith(t *testing.T, role string, payFn func(context.Context, string, string, string, int64, string, string, time.Time) (*service.ManagedBillMutationResult, error)) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{payFn: payFn}
	router := gin.New()
	router.POST("/managed-bills/:id/pay", NewManagedBillHandler(stub).Pay)

	req := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/pay",
		bytes.NewReader([]byte(`{"expected_version":2,"payment_reference":"pix-1"}`)))
	req.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "operator-1")
	if role != "" {
		ctx = appcontext.WithFunctionalRole(ctx, role)
	}
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func approveOwnBillPayFn(_ context.Context, _ string, _ string, _ string, _ int64, _ string, _ string, _ time.Time) (*service.ManagedBillMutationResult, error) {
	return nil, service.ErrManagedBillActorConflict
}

// Política nova: o administrador da imobiliária conclui aprovação e pagamento
// sozinho. A dispensa é concedida a partir do papel AUTENTICADO — nunca do corpo
// da requisição —, então um cliente não pode se autoconceder.
func TestManagedBillPayWaivesSegregationForFinanceAuthority(t *testing.T) {
	for _, tc := range []struct {
		role        string
		wantWaived  bool
		description string
	}{
		{role: "admin", wantWaived: true, description: "administrador responde pela operação financeira"},
		{role: "gestor", wantWaived: true, description: "gestor tem as mesmas permissões de aprovar e escrever"},
		{role: "financeiro", wantWaived: false, description: "operador financeiro não aprova"},
		{role: "corretor", wantWaived: false, description: "corretor não responde pela operação"},
		// Sem papel identificado a segregação continua valendo: liberar por
		// ausência de dado seria o pior dos defaults.
		{role: "", wantWaived: false, description: "sem papel atribuído não libera"},
	} {
		t.Run(tc.role, func(t *testing.T) {
			var sawWaiver bool
			recorder := payRequestWith(t, tc.role, func(ctx context.Context, tenantID, id, actor string, expectedVersion int64, reference, proof string, at time.Time) (*service.ManagedBillMutationResult, error) {
				sawWaiver = service.SeparationOfDutiesWaived(ctx)
				// A dispensa remove a exigência de dois atores; o resto das regras
				// segue valendo, então o handler ainda precisa tratar o conflito.
				return approveOwnBillPayFn(ctx, tenantID, id, actor, expectedVersion, reference, proof, at)
			})

			if sawWaiver != tc.wantWaived {
				t.Fatalf("%s: waiver = %v, want %v", tc.description, sawWaiver, tc.wantWaived)
			}
			// Independente da dispensa, o conflito de ator é reportado como 409.
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

// A dispensa não pode vir do corpo: um payload que tente forçá-la é ignorado.
func TestManagedBillPayIgnoresWaiverSuppliedInBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var sawWaiver bool
	stub := &managedBillHandlerStub{
		payFn: func(ctx context.Context, _ string, _ string, _ string, _ int64, _ string, _ string, _ time.Time) (*service.ManagedBillMutationResult, error) {
			sawWaiver = service.SeparationOfDutiesWaived(ctx)
			return &service.ManagedBillMutationResult{Bill: &managed_bill.ManagedBill{ID: "bill-1", Status: managed_bill.StatusPaid}}, nil
		},
	}
	router := gin.New()
	router.POST("/managed-bills/:id/pay", NewManagedBillHandler(stub).Pay)

	body := []byte(`{"expected_version":2,"payment_reference":"pix-1","separation_of_duties_waived":true,"waived":true,"role":"admin"}`)
	req := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/pay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "operator-1")
	ctx = appcontext.WithFunctionalRole(ctx, "financeiro")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if sawWaiver {
		t.Fatal("o corpo da requisição não pode conceder a dispensa da segregação")
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
