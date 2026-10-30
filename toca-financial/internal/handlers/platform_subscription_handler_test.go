package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	appctx "github.com/oginoino/toca/pkg/context"
	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	service "github.com/oginoino/toca/toca-financial/internal/services"
	"github.com/stretchr/testify/require"
)

type webhookFailingRepo struct {
	ps.Repository
	failure error
}

func (r webhookFailingRepo) ResolveTenant(context.Context, string, string, string) (string, error) {
	return "", r.failure
}
func TestPlatformSubscriptionWebhookAuthenticationAndRetry(t *testing.T) {
	gin.SetMode(gin.TestMode)
	raw := `{"id":"log_test","event":"subscription.cancelled","apiVersion":2,"devMode":true,"data":{"subscription":{"id":"subs_test","updatedAt":"2026-10-03T18:00:00Z"}}}`
	mac := hmac.New(sha256.New, []byte(abacatePublicKey))
	_, _ = mac.Write([]byte(raw))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for _, test := range []struct {
		name, body, secret, sig, ip string
		failure                     error
		status                      int
	}{
		{name: "missing secret", body: raw, sig: signature, status: 401},
		{name: "wrong signature", body: raw, secret: "test-secret", sig: "fake", status: 401},
		{name: "tampered body", body: strings.Replace(raw, "subs_test", "subs_other", 1), secret: "test-secret", sig: signature, status: 401},
		{name: "wrong IP", body: raw, secret: "test-secret", sig: signature, ip: "203.0.113.1", status: 403},
		{name: "database outage retries", body: raw, secret: "test-secret", sig: signature, failure: errors.New("database outage"), status: 503},
		{name: "foreign product acknowledged", body: raw, secret: "test-secret", sig: signature, failure: ps.ErrNotFound, status: 200},
		{name: "conflicting event stays retryable", body: raw, secret: "test-secret", sig: signature, failure: ps.ErrConflict, status: 503},
	} {
		t.Run(test.name, func(t *testing.T) {
			h := &PlatformSubscriptionHandler{Service: &service.PlatformSubscriptionService{Repo: webhookFailingRepo{failure: test.failure}, Enabled: true, DevMode: true}, Secret: "test-secret"}
			if test.ip != "" {
				h.AllowedIPs = []string{"192.0.2.0/24"}
			}
			r := gin.New()
			_ = r.SetTrustedProxies(nil)
			r.POST("/webhooks/abacatepay", h.Webhook)
			req := httptest.NewRequest(http.MethodPost, "/webhooks/abacatepay?webhookSecret="+test.secret, strings.NewReader(test.body))
			req.Header.Set("X-Webhook-Signature", test.sig)
			if test.ip != "" {
				req.RemoteAddr = test.ip + ":1234"
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			require.Equal(t, test.status, w.Code)
			require.NotContains(t, w.Body.String(), "test-secret")
		})
	}
}

type receiptRepo struct {
	ps.Repository
	payment *ps.Payment
	state   *ps.State
}

func (r receiptRepo) Payment(context.Context, string, string) (*ps.Payment, error) {
	return r.payment, nil
}
func (r receiptRepo) Get(context.Context, string) (*ps.State, error) { return r.state, nil }
func (r receiptRepo) TenantIdentity(context.Context, string) (string, string, error) {
	return "Imobiliária Teste", "12.345.678/0001-90", nil
}

func TestPlatformSubscriptionReceiptIsATenantDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	plan := &ps.Plan{Tier: "starter", Name: "Starter", Cycle: "MONTHLY", AmountCents: 49700}
	paid := &ps.Payment{ID: "card_1", SubscriptionID: "subs_1", AmountCents: 49700, PaidAt: time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)}
	handler := func(repo ps.Repository) *PlatformSubscriptionHandler {
		return &PlatformSubscriptionHandler{Service: &service.PlatformSubscriptionService{Repo: repo, Enabled: true, DevMode: true}}
	}
	r := gin.New()
	r.GET("/payments/:id/receipt", handler(receiptRepo{
		payment: paid,
		state:   &ps.State{Status: "ACTIVE", Plan: plan, PaymentMethod: &ps.PaymentMethod{Kind: "CARD", Brand: "VISA", Last4: "4242"}},
	}).GetPaymentReceipt)
	req := httptest.NewRequest(http.MethodGet, "/payments/card_1/receipt", nil)
	req = req.WithContext(appctx.WithTenantID(req.Context(), "tenant-a"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/pdf", w.Header().Get("Content-Type"))
	require.Contains(t, w.Header().Get("Content-Disposition"), "comprovante-2026-10-03.pdf")
	require.True(t, bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF")), "esperado PDF")

	// A payment that is not this tenant's is reported as missing.
	missing := gin.New()
	missing.GET("/payments/:id/receipt", handler(receiptRepo{state: &ps.State{}}).GetPaymentReceipt)
	req = httptest.NewRequest(http.MethodGet, "/payments/card_de_outro/receipt", nil)
	req = req.WithContext(appctx.WithTenantID(req.Context(), "tenant-a"))
	w = httptest.NewRecorder()
	missing.ServeHTTP(w, req)
	require.Equal(t, http.StatusNotFound, w.Code)
}

func TestPlatformSubscriptionHandlersRequireTenantContext(t *testing.T) {
	h := &PlatformSubscriptionHandler{}
	r := gin.New()
	r.GET("/", h.Get)
	r.POST("/checkout", h.Select)
	r.POST("/cancel", h.Cancel)
	for _, request := range []struct{ method, path string }{{"GET", "/"}, {"POST", "/checkout"}, {"POST", "/cancel"}} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(request.method, request.path, strings.NewReader(`{"tier":"pro","cycle":"MONTHLY","tenant_id":"victim"}`)))
		require.Equal(t, 403, w.Code)
	}
}
