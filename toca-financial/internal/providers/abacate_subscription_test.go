package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
	"github.com/stretchr/testify/require"
)

func TestAbacateSubscriptionCatalogPaginationDoesNotCreateDuplicate(t *testing.T) {
	creates := 0
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret-test", r.Header.Get("Authorization"))
		if r.Method == "POST" {
			creates++
			t.Error("must not create duplicate product")
			w.WriteHeader(500)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/products/get") {
			// The direct lookup misses: the catalog below has to answer.
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"success":false,"data":null,"error":"Product not found"}`))
			return
		}
		pages++
		if r.URL.Query().Get("after") == "" {
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []any{}, "pagination": map[string]any{"hasMore": true, "next": "page-two"}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{{"id": "prod_existing", "externalId": "suitee-starter-monthly-49700-t15", "price": 49700, "cycle": "MONTHLY", "currency": "BRL", "status": "ACTIVE", "devMode": true, "trialDays": 15}}, "pagination": map[string]any{"hasMore": false}})
	}))
	defer server.Close()
	p := NewAbacateSubscriptionProvider("secret-test", true)
	p.BaseURL = server.URL
	id, err := p.EnsureProduct(context.Background(), ps.Plan{Tier: "starter", Cycle: "MONTHLY", AmountCents: 49700, TrialDays: 15})
	require.NoError(t, err)
	require.Equal(t, "prod_existing", id)
	require.Equal(t, 2, pages)
	require.Zero(t, creates)
}
func TestAbacateSubscriptionProductLookupDoesNotTrustStaleCatalog(t *testing.T) {
	// The paginated catalog still returns the previous catalogue while a
	// product created moments ago already answers the direct lookup.
	lists := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			t.Error("must not create a product that already exists")
			w.WriteHeader(500)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/products/get") {
			// A product without the trial is a different offer and must be refused.
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{
				"id": "prod_created", "externalId": "suitee-starter-monthly-49700-t15", "price": 49700,
				"cycle": "MONTHLY", "currency": "BRL", "status": "ACTIVE", "devMode": true, "trialDays": 15}})
			return
		}
		lists++
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": []map[string]any{{"id": "prod_old", "externalId": "suitee-starter-monthly-49700", "price": 49700, "cycle": "MONTHLY", "currency": "BRL", "status": "ACTIVE", "devMode": true, "trialDays": nil}}, "pagination": map[string]any{"hasMore": false}})
	}))
	defer server.Close()
	p := NewAbacateSubscriptionProvider("secret-test", true)
	p.BaseURL = server.URL
	id, err := p.EnsureProduct(context.Background(), ps.Plan{Tier: "starter", Cycle: "MONTHLY", AmountCents: 49700, TrialDays: 15})
	require.NoError(t, err)
	require.Equal(t, "prod_created", id)
	require.Zero(t, lists)

	// An offer whose free period differs is a conflict, never a silent reuse.
	p.BaseURL = server.URL
	_, err = p.EnsureProduct(context.Background(), ps.Plan{Tier: "starter", Cycle: "MONTHLY", AmountCents: 49700, TrialDays: 7})
	require.ErrorIs(t, err, ps.ErrConflict)
}
func TestAbacateSubscriptionMissingResourceIsAbsenceNotOutage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		switch {
		case r.URL.RawQuery == "":
			_, _ = w.Write([]byte(`{"success":false,"data":null,"error":"Provide id, externalId or customerId"}`))
		case strings.Contains(r.URL.Path, "checkouts"):
			_, _ = w.Write([]byte(`{"success":false,"data":null,"error":"Billing not found"}`))
		default:
			_, _ = w.Write([]byte(`{"success":false,"data":null,"error":"Subscription not found"}`))
		}
	}))
	defer server.Close()
	p := NewAbacateSubscriptionProvider("secret-test", true)
	p.BaseURL = server.URL
	_, err := p.FindCheckout(context.Background(), "suitee-missing")
	require.ErrorIs(t, err, ps.ErrNotFound)
	_, err = p.GetSubscription(context.Background(), "subs_missing")
	require.ErrorIs(t, err, ps.ErrNotFound)
	// Deleting what no longer exists is the desired state, not a failure.
	require.NoError(t, p.DeleteCheckout(context.Background(), "bill_missing"))
	// A malformed request stays retryable instead of looking like absence.
	err = p.request(context.Background(), http.MethodGet, "/subscriptions/checkouts/get", nil, nil)
	require.ErrorIs(t, err, ps.ErrUnavailable)
	require.NotErrorIs(t, err, ps.ErrNotFound)
}
func TestAbacateSubscriptionCancelAcceptsProviderLag(t *testing.T) {
	// The provider's reads lag behind its writes: a cancelled subscription may
	// still read as ACTIVE right after the cancel. That must not report failure.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":"subs_1","status":"CANCELLED","devMode":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"subs_1","status":"ACTIVE","devMode":true}}`))
	}))
	defer server.Close()
	p := NewAbacateSubscriptionProvider("secret-test", true)
	p.BaseURL = server.URL
	require.NoError(t, p.Cancel(context.Background(), "subs_1"))

	// A cancel that stayed active everywhere is still a conflict.
	stale := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"subs_1","status":"ACTIVE","devMode":true}}`))
	}))
	defer stale.Close()
	p.BaseURL = stale.URL
	require.ErrorIs(t, p.Cancel(context.Background(), "subs_1"), ps.ErrConflict)

	// Production subscriptions are never cancelled through a sandbox client.
	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":"subs_1","status":"ACTIVE","devMode":false}}`))
	}))
	defer prod.Close()
	p.BaseURL = prod.URL
	require.ErrorIs(t, p.Cancel(context.Background(), "subs_1"), ps.ErrConflict)
}
func TestAbacateSubscriptionErrorsAreSanitizedAndLinksRestricted(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"secret-test raw provider error"}`))
	}))
	defer server.Close()
	p := NewAbacateSubscriptionProvider("secret-test", true)
	p.BaseURL = server.URL
	_, err := p.FindCheckout(context.Background(), "tenant-secret")
	require.ErrorIs(t, err, ps.ErrUnavailable)
	require.NotContains(t, err.Error(), "secret-test")
	require.NotContains(t, err.Error(), "tenant-secret")
	for _, u := range []string{"javascript:alert(1)", "https://app.abacatepay.com.evil.test/", "https://user:pass@app.abacatepay.com/", "http://app.abacatepay.com/"} {
		require.False(t, SafeAbacateURL(u))
	}
	require.True(t, SafeAbacateURL("https://app.abacatepay.com/pay/test"))
}
