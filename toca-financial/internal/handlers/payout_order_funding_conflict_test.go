package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

type payoutFundingConflictRepo struct {
	payout_order.Repository
	item *payout_order.PayoutOrder
}

func (r *payoutFundingConflictRepo) GetByID(_ context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	clone := *r.item
	return &clone, nil
}

func TestPayoutFundingIncompleteMapsToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	item, err := payout_order.New(
		"tenant-rent", payout_order.SourceManual, "contract-rent",
		payout_order.RecipientBroker, "broker-rent", "Corretor", 10000, "system",
	)
	if err != nil {
		t.Fatal(err)
	}
	item.ID = "payout-rent"
	item.Status = payout_order.StatusDraft
	item.FundingRequiredCents = item.AmountCents
	repo := &payoutFundingConflictRepo{item: item}
	handler := NewPayoutOrderHandler(service.NewPayoutOrderService(repo), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithTenantID(c.Request.Context(), item.TenantID))
		c.Next()
	})
	router.POST("/payout-orders/:id/approve", handler.Approve)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/payout-orders/payout-rent/approve", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", recorder.Code, recorder.Body.String())
	}
}

func TestLegacyRentalPayoutReconciliationMapsToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	item, err := payout_order.New(
		"tenant-legacy-rent", payout_order.SourceManual, "contract-legacy-rent",
		payout_order.RecipientBroker, "broker-legacy-rent", "Corretor", 10000, "system",
	)
	if err != nil {
		t.Fatal(err)
	}
	item.ID = "payout-legacy-rent"
	item.SourceRef = map[string]any{"deal_type": "locação"}
	repo := &payoutFundingConflictRepo{item: item}
	handler := NewPayoutOrderHandler(service.NewPayoutOrderService(repo), nil)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(pkgcontext.WithTenantID(c.Request.Context(), item.TenantID))
		c.Next()
	})
	router.POST("/payout-orders/:id/approve", handler.Approve)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/payout-orders/payout-legacy-rent/approve", nil)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", recorder.Code, recorder.Body.String())
	}
}
