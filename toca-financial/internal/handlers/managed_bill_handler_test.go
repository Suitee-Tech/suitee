package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type managedBillHandlerStub struct {
	listFn      func(context.Context, string, managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error)
	getFn       func(context.Context, string, string) (*managed_bill.ManagedBill, error)
	approveFn   func(context.Context, string, string, string, int64) (*service.ManagedBillMutationResult, error)
	payFn       func(context.Context, string, string, string, int64, string, string, time.Time) (*service.ManagedBillMutationResult, error)
	reconcileFn func(context.Context, string, string, string, int64, string, time.Time) (*service.ManagedBillMutationResult, error)
	reverseFn   func(context.Context, string, string, string, int64, string, string, string, time.Time) (*service.ManagedBillMutationResult, error)
	cancelFn    func(context.Context, string, string, string, int64, string) (*service.ManagedBillMutationResult, error)
}

func (s *managedBillHandlerStub) GetByID(ctx context.Context, tenantID, id string) (*managed_bill.ManagedBill, error) {
	if s.getFn == nil {
		return nil, managed_bill.ErrNotFound
	}
	return s.getFn(ctx, tenantID, id)
}

func (s *managedBillHandlerStub) List(ctx context.Context, tenantID string, filter managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error) {
	return s.listFn(ctx, tenantID, filter)
}

func (s *managedBillHandlerStub) Approve(ctx context.Context, tenantID, id, actor string, expectedVersion int64) (*service.ManagedBillMutationResult, error) {
	return s.approveFn(ctx, tenantID, id, actor, expectedVersion)
}

func (s *managedBillHandlerStub) Pay(ctx context.Context, tenantID, id, actor string, expectedVersion int64, paymentReference, proofURL string, paidAt time.Time) (*service.ManagedBillMutationResult, error) {
	return s.payFn(ctx, tenantID, id, actor, expectedVersion, paymentReference, proofURL, paidAt)
}

func (s *managedBillHandlerStub) Reconcile(ctx context.Context, tenantID, id, actor string, expectedVersion int64, reference string, reconciledAt time.Time) (*service.ManagedBillMutationResult, error) {
	return s.reconcileFn(ctx, tenantID, id, actor, expectedVersion, reference, reconciledAt)
}

func (s *managedBillHandlerStub) ReversePayment(ctx context.Context, tenantID, id, actor string, expectedVersion int64, reversalReference, reason, proofURL string, reversedAt time.Time) (*service.ManagedBillMutationResult, error) {
	return s.reverseFn(ctx, tenantID, id, actor, expectedVersion, reversalReference, reason, proofURL, reversedAt)
}

func (s *managedBillHandlerStub) Cancel(ctx context.Context, tenantID, id, actor string, expectedVersion int64, reason string) (*service.ManagedBillMutationResult, error) {
	return s.cancelFn(ctx, tenantID, id, actor, expectedVersion, reason)
}

func TestManagedBillHandlerListKeepsTenantAndPaginationExplicit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.listFn = func(_ context.Context, tenantID string, filter managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error) {
		if tenantID != "tenant-a" {
			t.Fatalf("tenant = %q, want tenant-a", tenantID)
		}
		if filter.ContractID != "contract-1" || filter.Status != managed_bill.StatusReserved || filter.Search != "condominio" || filter.Limit != 25 || filter.Offset != 50 {
			t.Fatalf("unexpected filter: %+v", filter)
		}
		return []*managed_bill.ManagedBill{{ID: "bill-1", TenantID: tenantID, Status: managed_bill.StatusReserved}}, 51, nil
	}

	router := gin.New()
	router.GET("/managed-bills", NewManagedBillHandler(stub).List)
	req := httptest.NewRequest(http.MethodGet, "/managed-bills?contract_id=contract-1&status=RESERVED&search=condominio&limit=25&offset=50", nil)
	req = req.WithContext(appcontext.WithTenantID(req.Context(), "tenant-a"))
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Data []managed_bill.ManagedBill `json:"data"`
		Meta struct {
			Page  int   `json:"page"`
			Limit int   `json:"limit"`
			Total int64 `json:"total"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != "bill-1" || payload.Meta.Page != 3 || payload.Meta.Limit != 25 || payload.Meta.Total != 51 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestManagedBillHandlerPayReturnsAuthoritativePocketSnapshots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.payFn = func(_ context.Context, tenantID, id, actor string, expectedVersion int64, paymentReference, proofURL string, paidAt time.Time) (*service.ManagedBillMutationResult, error) {
		if tenantID != "tenant-a" || id != "bill-1" || actor != "operator-2" || expectedVersion != 2 {
			t.Fatalf("unexpected identity/version: tenant=%q id=%q actor=%q version=%d", tenantID, id, actor, expectedVersion)
		}
		if paymentReference != "pix-e2e-123" || proofURL != "https://proof.example/123" || !paidAt.IsZero() {
			t.Fatalf("unexpected payment command: reference=%q proof=%q paidAt=%v", paymentReference, proofURL, paidAt)
		}
		return &service.ManagedBillMutationResult{
			Bill:             &managed_bill.ManagedBill{ID: id, TenantID: tenantID, Status: managed_bill.StatusPaid, Version: 3},
			BalanceBefore:    service.ManagedBillBalanceSnapshot{TotalCents: 560000, ReservedCents: 100000, AvailableCents: 460000},
			BalanceAfter:     service.ManagedBillBalanceSnapshot{TotalCents: 490000, ReservedCents: 30000, AvailableCents: 460000},
			CoverageComplete: true,
		}, nil
	}

	router := gin.New()
	router.POST("/managed-bills/:id/pay", NewManagedBillHandler(stub).Pay)
	body := []byte(`{"expected_version":2,"payment_reference":"pix-e2e-123","proof_url":"https://proof.example/123"}`)
	req := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/pay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "operator-2")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := payload["data"].(map[string]any)
	before, beforeOK := data["owner_balance_before"].(map[string]any)
	after, afterOK := data["owner_balance_after"].(map[string]any)
	if !beforeOK || !afterOK || before["available_cents"] != float64(460000) || after["available_cents"] != float64(460000) {
		t.Fatalf("authoritative snapshots missing or wrong: %s", recorder.Body.String())
	}
}

func TestManagedBillHandlerRequiresActorAndMapsCASConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.approveFn = func(context.Context, string, string, string, int64) (*service.ManagedBillMutationResult, error) {
		return nil, managed_bill.ErrConcurrentModification
	}
	handler := NewManagedBillHandler(stub)
	router := gin.New()
	router.POST("/managed-bills/:id/approve", handler.Approve)

	withoutActor := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/approve", bytes.NewBufferString(`{"expected_version":1}`))
	withoutActor.Header.Set("Content-Type", "application/json")
	withoutActor = withoutActor.WithContext(appcontext.WithTenantID(withoutActor.Context(), "tenant-a"))
	forbidden := httptest.NewRecorder()
	router.ServeHTTP(forbidden, withoutActor)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("missing actor status = %d, body = %s", forbidden.Code, forbidden.Body.String())
	}

	withActor := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/approve", bytes.NewBufferString(`{"expected_version":1}`))
	withActor.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(withActor.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "operator-1")
	withActor = withActor.WithContext(ctx)
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, withActor)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("CAS status = %d, body = %s", conflict.Code, conflict.Body.String())
	}
}

func TestManagedBillHandlerReversePaymentPassesVersionedAuditCommand(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.reverseFn = func(_ context.Context, tenantID, id, actor string, expectedVersion int64, reversalReference, reason, proofURL string, reversedAt time.Time) (*service.ManagedBillMutationResult, error) {
		if tenantID != "tenant-a" || id != "bill-1" || actor != "approver-9" || expectedVersion != 4 {
			t.Fatalf("unexpected identity/version: tenant=%q id=%q actor=%q version=%d", tenantID, id, actor, expectedVersion)
		}
		if reversalReference != "ESTORNO-PIX-700" || reason != "Pagamento devolvido pelo banco" || proofURL != "https://proof.example/reversal.pdf" || !reversedAt.IsZero() {
			t.Fatalf("unexpected reversal command: reference=%q reason=%q proof=%q reversedAt=%v", reversalReference, reason, proofURL, reversedAt)
		}
		return &service.ManagedBillMutationResult{
			Bill:             &managed_bill.ManagedBill{ID: id, TenantID: tenantID, Status: managed_bill.StatusApproved, Version: 5},
			CoverageComplete: true,
		}, nil
	}

	router := gin.New()
	router.POST("/managed-bills/:id/reverse-payment", NewManagedBillHandler(stub).ReversePayment)
	body := []byte(`{"expected_version":4,"reversal_reference":"ESTORNO-PIX-700","reason":"Pagamento devolvido pelo banco","proof_url":"https://proof.example/reversal.pdf"}`)
	req := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/reverse-payment", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "approver-9")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestManagedBillHandlerReversePaymentValidatesPayloadActorAndConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.reverseFn = func(context.Context, string, string, string, int64, string, string, string, time.Time) (*service.ManagedBillMutationResult, error) {
		return nil, managed_bill.ErrConcurrentModification
	}
	handler := NewManagedBillHandler(stub)
	router := gin.New()
	router.POST("/managed-bills/:id/reverse-payment", handler.ReversePayment)

	invalid := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/reverse-payment", bytes.NewBufferString(`{"expected_version":4,"reason":"sem referência"}`))
	invalid.Header.Set("Content-Type", "application/json")
	invalidCtx := appcontext.WithTenantID(invalid.Context(), "tenant-a")
	invalidCtx = appcontext.WithAccountID(invalidCtx, "approver-9")
	invalid = invalid.WithContext(invalidCtx)
	invalidRecorder := httptest.NewRecorder()
	router.ServeHTTP(invalidRecorder, invalid)
	if invalidRecorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid payload status = %d, body = %s", invalidRecorder.Code, invalidRecorder.Body.String())
	}

	withoutActor := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/reverse-payment", bytes.NewBufferString(`{"expected_version":4,"reversal_reference":"EST-1","reason":"devolvido"}`))
	withoutActor.Header.Set("Content-Type", "application/json")
	withoutActor = withoutActor.WithContext(appcontext.WithTenantID(withoutActor.Context(), "tenant-a"))
	forbidden := httptest.NewRecorder()
	router.ServeHTTP(forbidden, withoutActor)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("missing actor status = %d, body = %s", forbidden.Code, forbidden.Body.String())
	}

	withActor := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/reverse-payment", bytes.NewBufferString(`{"expected_version":4,"reversal_reference":"EST-1","reason":"devolvido"}`))
	withActor.Header.Set("Content-Type", "application/json")
	conflictCtx := appcontext.WithTenantID(withActor.Context(), "tenant-a")
	conflictCtx = appcontext.WithAccountID(conflictCtx, "approver-9")
	withActor = withActor.WithContext(conflictCtx)
	conflict := httptest.NewRecorder()
	router.ServeHTTP(conflict, withActor)
	if conflict.Code != http.StatusConflict {
		t.Fatalf("conflict status = %d, body = %s", conflict.Code, conflict.Body.String())
	}
}

func TestRespondManagedBillErrorDoesNotTurnUnknownFailuresIntoValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	respondManagedBillError(ctx, errors.New("storage unavailable"))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRespondManagedBillErrorMapsReversalActorConflictToConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	respondManagedBillError(ctx, service.ErrManagedBillReversalActorConflict)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

// Reproduz o 409 de "Registrar pagamento": a versão em mãos está defasada.
// Antes a resposta era o mesmo "modificado por outra operação" de qualquer
// outro conflito, sem dizer o que fazer. Agora carrega a versão vigente para a
// interface recarregar e refazer a operação.
func TestManagedBillHandlerStaleVersionReturnsCurrentVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.payFn = func(context.Context, string, string, string, int64, string, string, time.Time) (*service.ManagedBillMutationResult, error) {
		return nil, &service.ManagedBillStaleVersionError{Expected: 2, Current: 4, Status: managed_bill.StatusApproved}
	}

	router := gin.New()
	router.POST("/managed-bills/:id/pay", NewManagedBillHandler(stub).Pay)
	body := []byte(`{"expected_version":2,"payment_reference":"pix-1"}`)
	req := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/pay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "operator-2")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := payload["data"].(map[string]any)
	if data["reason"] != "stale_version" {
		t.Fatalf("reason = %v, want stale_version", data["reason"])
	}
	if data["current_version"] != float64(4) {
		t.Fatalf("current_version = %v, want 4", data["current_version"])
	}
	message, _ := payload["message"].(string)
	if !strings.Contains(message, "recarregue a lista") {
		t.Fatalf("mensagem sem orientação de recuperação: %q", message)
	}
}

// Conta já paga com outra referência: repetir não resolve, e a mensagem precisa
// dizer isso em vez de sugerir nova tentativa.
func TestManagedBillHandlerAlreadyPaidIsDistinctFromStaleVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &managedBillHandlerStub{}
	stub.payFn = func(context.Context, string, string, string, int64, string, string, time.Time) (*service.ManagedBillMutationResult, error) {
		return nil, &service.ManagedBillAlreadyPaidError{CurrentReference: "pix-original", Status: managed_bill.StatusPaid}
	}

	router := gin.New()
	router.POST("/managed-bills/:id/pay", NewManagedBillHandler(stub).Pay)
	body := []byte(`{"expected_version":3,"payment_reference":"pix-outro"}`)
	req := httptest.NewRequest(http.MethodPost, "/managed-bills/bill-1/pay", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx := appcontext.WithTenantID(req.Context(), "tenant-a")
	ctx = appcontext.WithAccountID(ctx, "operator-2")
	req = req.WithContext(ctx)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := payload["data"].(map[string]any)
	if data["reason"] != "already_paid" {
		t.Fatalf("reason = %v, want already_paid", data["reason"])
	}
	if data["payment_reference"] != "pix-original" {
		t.Fatalf("payment_reference = %v, want a referência registrada", data["payment_reference"])
	}
}
