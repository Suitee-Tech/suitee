package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

func setupChargePaymentRouter(repo *mockChargeRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewChargeHandler(service.NewChargeService(repo, nil))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), "tenant-payment")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/charges/:id/pay", handler.Pay)
	router.POST("/charges/:id/reconcile-manual", handler.ReconcileManual)
	return router
}

func newPaymentHandlerCharge(t *testing.T, repo *mockChargeRepo) *charge.Charge {
	t.Helper()
	c, err := charge.New(
		"contract-payment",
		"payer-payment",
		"owner-payment",
		"property-payment",
		"tenant-payment",
		charge.TypeRent,
		"Aluguel",
		"2026-08",
		100,
		time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("create charge fixture: %v", err)
	}
	c.ID = "charge-payment"
	repo.charges[c.ID] = c
	return c
}

func TestChargePaymentEndpointsRequireIdempotencyKey(t *testing.T) {
	paidAt := time.Date(2026, time.August, 10, 15, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		path string
		body any
	}{
		{
			name: "pay",
			path: "/charges/charge-payment/pay",
			body: PayChargeRequest{PaymentMethod: "PIX", PaidAmount: 40},
		},
		{
			name: "manual reconciliation",
			path: "/charges/charge-payment/reconcile-manual",
			body: ReconcileManualRequest{PaymentMethod: "TED", PaidAmount: 40, PaidAt: paidAt},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
			c := newPaymentHandlerCharge(t, repo)
			router := setupChargePaymentRouter(repo)
			payload, err := json.Marshal(tt.body)
			if err != nil {
				t.Fatalf("marshal request: %v", err)
			}

			for _, headerValue := range []string{"", "   "} {
				req := httptest.NewRequest(http.MethodPost, tt.path, bytes.NewReader(payload))
				req.Header.Set("Content-Type", "application/json")
				if headerValue != "" {
					req.Header.Set("Idempotency-Key", headerValue)
				}
				w := httptest.NewRecorder()

				router.ServeHTTP(w, req)

				if w.Code != http.StatusBadRequest {
					t.Fatalf("header %q: expected 400, got %d: %s", headerValue, w.Code, w.Body.String())
				}
				if len(c.Payments) != 0 || c.PaidAmount != nil || c.Status != charge.StatusPending {
					t.Fatalf("header %q mutated charge before validating idempotency key: %+v", headerValue, c)
				}
			}
		})
	}
}

func TestChargePayRetryWithSameHeaderDoesNotDuplicatePayment(t *testing.T) {
	repo := &mockChargeRepo{charges: make(map[string]*charge.Charge)}
	c := newPaymentHandlerCharge(t, repo)
	router := setupChargePaymentRouter(repo)
	payload, err := json.Marshal(PayChargeRequest{PaymentMethod: "PIX", PaidAmount: 40})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		req := httptest.NewRequest(http.MethodPost, "/charges/charge-payment/pay", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Idempotency-Key", "payment-command-1")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("attempt %d: expected 200, got %d: %s", attempt+1, w.Code, w.Body.String())
		}
	}

	if len(c.Payments) != 1 {
		t.Fatalf("expected one append-only payment after retry, got %d", len(c.Payments))
	}
	if c.PaidAmount == nil || c.PaidAmount.Cents() != 4000 {
		t.Fatalf("expected accumulated paid amount of 4000 cents, got %+v", c.PaidAmount)
	}
	if c.Status != charge.StatusPartiallyPaid {
		t.Fatalf("expected PARTIALLY_PAID, got %s", c.Status)
	}
}
