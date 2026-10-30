package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/invoice"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type invoicePaymentRepo struct {
	inv         *invoice.Invoice
	getErr      error
	createCalls int
	updateCalls int
}

func (r *invoicePaymentRepo) Create(_ context.Context, inv *invoice.Invoice) error {
	r.createCalls++
	r.inv = inv
	return nil
}
func (r *invoicePaymentRepo) GetByID(_ context.Context, tenantID, id string) (*invoice.Invoice, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.inv == nil || r.inv.TenantID != tenantID || r.inv.ID != id {
		return nil, nil
	}
	return r.inv, nil
}
func (r *invoicePaymentRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*invoice.Invoice, error) {
	if r.inv == nil || r.inv.TenantID != tenantID || r.inv.SourceEventID != sourceEventID {
		return nil, nil
	}
	return r.inv, nil
}
func (r *invoicePaymentRepo) SumOpenAmountCentsByChargeID(_ context.Context, tenantID, chargeID, excludedInvoiceID string) (int64, error) {
	if r.inv == nil || r.inv.ID == excludedInvoiceID || r.inv.TenantID != tenantID || r.inv.ChargeID != chargeID || r.inv.Status == invoice.StatusPaid || r.inv.Status == invoice.StatusCancelled {
		return 0, nil
	}
	return int64(r.inv.Amount * 100), nil
}
func (r *invoicePaymentRepo) GetByReferenceID(context.Context, string, string) ([]*invoice.Invoice, error) {
	return nil, nil
}
func (r *invoicePaymentRepo) Update(_ context.Context, inv *invoice.Invoice) error {
	r.updateCalls++
	r.inv = inv
	return nil
}
func (r *invoicePaymentRepo) List(context.Context, string, *invoice.Status, *string) ([]*invoice.Invoice, error) {
	return nil, nil
}
func (r *invoicePaymentRepo) Search(context.Context, string, invoice.SearchFilter) ([]*invoice.Invoice, int64, error) {
	return nil, 0, nil
}

func setupInvoicePaymentRouter(t *testing.T) (*gin.Engine, *invoicePaymentRepo, *charge.Charge) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	chargeRepo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	receivable, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Aluguel", "2026-08", 100, time.Now())
	if err != nil {
		t.Fatalf("create charge: %v", err)
	}
	receivable.ID = "charge-1"
	chargeRepo.charges[receivable.ID] = receivable

	inv, err := invoice.New("contract-1", "payer-1", 40, time.Now(), invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	inv.ID = "invoice-1"
	inv.Status = invoice.StatusIssued
	inv.ChargeID = receivable.ID
	invoiceRepo := &invoicePaymentRepo{inv: inv}
	invoiceService := service.NewInvoiceService(invoiceRepo, nil).
		WithPaymentApplication(service.NewChargeService(chargeRepo, nil))
	handler := NewInvoiceHandler(invoiceService)

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithTenantID(c.Request.Context(), "tenant-1"))
		c.Next()
	})
	router.POST("/invoices", handler.Create)
	router.GET("/invoices/:id", handler.GetByID)
	router.POST("/invoices/:id/pay", handler.Pay)
	return router, invoiceRepo, receivable
}

func TestInvoiceCreateRequiresOneStableIdempotencyCommand(t *testing.T) {
	router, repo, _ := setupInvoicePaymentRouter(t)
	payload := map[string]any{
		"reference_id": "contract-1",
		"charge_id":    "charge-1",
		"payer_id":     "payer-1",
		"amount":       40,
		"due_date":     "2026-09-10T00:00:00Z",
		"type":         "RENT",
	}
	body, _ := json.Marshal(payload)

	missingKey := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
	missingKey.Header.Set("Content-Type", "application/json")
	missingRecorder := httptest.NewRecorder()
	router.ServeHTTP(missingRecorder, missingKey)
	if missingRecorder.Code != http.StatusBadRequest || repo.createCalls != 0 {
		t.Fatalf("missing key status=%d creates=%d body=%s", missingRecorder.Code, repo.createCalls, missingRecorder.Body.String())
	}

	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "invoice-create-command-1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("attempt %d status=%d body=%s", attempt+1, w.Code, w.Body.String())
		}
	}
	if repo.createCalls != 1 {
		t.Fatalf("identical retry created %d rows, want 1", repo.createCalls)
	}

	payload["amount"] = 41
	changedBody, _ := json.Marshal(payload)
	conflict := httptest.NewRequest(http.MethodPost, "/invoices", bytes.NewReader(changedBody))
	conflict.Header.Set("Content-Type", "application/json")
	conflict.Header.Set("Idempotency-Key", "invoice-create-command-1")
	conflictRecorder := httptest.NewRecorder()
	router.ServeHTTP(conflictRecorder, conflict)
	if conflictRecorder.Code != http.StatusConflict || repo.createCalls != 1 {
		t.Fatalf("changed retry status=%d creates=%d body=%s", conflictRecorder.Code, repo.createCalls, conflictRecorder.Body.String())
	}
}

func TestInvoicePaymentRequiresIdempotencyKeyBeforeAllocatingCash(t *testing.T) {
	router, repo, receivable := setupInvoicePaymentRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/invoices/invoice-1/pay", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", w.Code, w.Body.String())
	}
	if repo.updateCalls != 0 || repo.inv.Status != invoice.StatusIssued || len(receivable.Payments) != 0 {
		t.Fatalf("missing key mutated financial state: updates=%d invoice=%s payments=%d", repo.updateCalls, repo.inv.Status, len(receivable.Payments))
	}
}

func TestInvoicePaymentUsesChargeAppendOnlyApplicationExactlyOnce(t *testing.T) {
	router, repo, receivable := setupInvoicePaymentRouter(t)

	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/invoices/invoice-1/pay", nil)
		req.Header.Set("Idempotency-Key", "invoice-command-1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d: %s", attempt+1, w.Code, w.Body.String())
		}
	}

	if repo.updateCalls != 1 || repo.inv.Status != invoice.StatusPaid {
		t.Fatalf("invoice projection writes=%d status=%s, want one PAID update", repo.updateCalls, repo.inv.Status)
	}
	if len(receivable.Payments) != 1 || receivable.PaidAmount == nil || receivable.PaidAmount.Cents() != 4_000 || receivable.Status != charge.StatusPartiallyPaid {
		t.Fatalf("canonical charge allocation = %+v", receivable)
	}
}

func TestInvoiceHandlersMapWrappedNotFoundSentinelTo404(t *testing.T) {
	router, repo, _ := setupInvoicePaymentRouter(t)
	repo.getErr = fmt.Errorf("postgres invoice lookup: %w", invoice.ErrInvoiceNotFound)

	getReq := httptest.NewRequest(http.MethodGet, "/invoices/missing", nil)
	getRecorder := httptest.NewRecorder()
	router.ServeHTTP(getRecorder, getReq)
	if getRecorder.Code != http.StatusNotFound {
		t.Fatalf("GET missing invoice status=%d body=%s", getRecorder.Code, getRecorder.Body.String())
	}

	payReq := httptest.NewRequest(http.MethodPost, "/invoices/missing/pay", nil)
	payReq.Header.Set("Idempotency-Key", "missing-invoice-payment")
	payRecorder := httptest.NewRecorder()
	router.ServeHTTP(payRecorder, payReq)
	if payRecorder.Code != http.StatusNotFound {
		t.Fatalf("PAY missing invoice status=%d body=%s", payRecorder.Code, payRecorder.Body.String())
	}
	if repo.updateCalls != 0 {
		t.Fatalf("missing invoice attempted %d updates", repo.updateCalls)
	}
}
