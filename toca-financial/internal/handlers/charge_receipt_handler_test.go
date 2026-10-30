package handlers

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// stubTenantCompany is the narrow lookup used by the receipt handlers.
type stubTenantCompany struct {
	name      string
	err       error
	gotTenant string
}

func (s *stubTenantCompany) CompanyName(_ context.Context, tenantID string) (string, error) {
	s.gotTenant = tenantID
	return s.name, s.err
}

func setupReceiptRouter(repo *mockChargeRepo, lookup *stubTenantCompany) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := NewChargeHandler(service.NewChargeService(repo, nil)).WithTenantCompanyLookup(lookup)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithTenantID(c.Request.Context(), "tenant-123"))
		c.Next()
	})
	r.GET("/charges/:id/receipt", handler.GetReceipt)
	return r
}

func TestChargeHandler_GetReceipt_PrintsLoggedCompanyName(t *testing.T) {
	repo := &mockChargeRepo{charges: map[string]*charge.Charge{}}
	seedCharge(repo, "tenant-123", "c-paid", charge.StatusPaid)
	lookup := &stubTenantCompany{name: "Exemplo Imóveis"}

	w := httptest.NewRecorder()
	setupReceiptRouter(repo, lookup).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/charges/c-paid/receipt", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("receipt status = %d, body = %s", w.Code, w.Body.String())
	}
	if lookup.gotTenant != "tenant-123" {
		t.Fatalf("lookup tenant = %q, want tenant-123", lookup.gotTenant)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatalf("receipt is not a PDF: %q", w.Body.String()[:min(16, w.Body.Len())])
	}
}

func TestChargeHandler_GetReceipt_KeepsFallbackWhenLookupFails(t *testing.T) {
	repo := &mockChargeRepo{charges: map[string]*charge.Charge{}}
	seedCharge(repo, "tenant-123", "c-paid", charge.StatusPaid)
	lookup := &stubTenantCompany{err: errors.New("tenant registry unavailable")}

	w := httptest.NewRecorder()
	setupReceiptRouter(repo, lookup).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/charges/c-paid/receipt", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("receipt status = %d, want 200 even when enrichment fails", w.Code)
	}
	if !bytes.HasPrefix(w.Body.Bytes(), []byte("%PDF-")) {
		t.Fatal("receipt is not a PDF")
	}
}
