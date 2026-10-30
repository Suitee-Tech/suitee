package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func validApprovalPolicy() ApprovalPolicy {
	return ApprovalPolicy{
		SchemaVersion:      1,
		Revision:           7,
		Enabled:            true,
		ThresholdBRL:       750_000,
		TargetStatuses:     []string{"ANALYSIS", "CONTRACT"},
		RequiredPermission: "deals:approve",
		UpdatedAt:          time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC),
		UpdatedBy:          "user-1",
		Source:             "tenant_settings",
	}
}

func newApprovalPolicyTestClient(baseURL string, now *time.Time) *AuthClient {
	base := NewBaseClient(baseURL)
	base.InternalAPIKey = "test-internal-key"
	base.HTTPClient.Timeout = 100 * time.Millisecond
	return &AuthClient{
		BaseClient:           base,
		approvalCache:        make(map[string]approvalPolicyCacheEntry),
		approvalCacheTTL:     time.Minute,
		approvalStaleTTL:     5 * time.Minute,
		approvalFetchTimeout: 75 * time.Millisecond,
		now: func() time.Time {
			return *now
		},
	}
}

func TestAuthClientGetApprovalPolicyRemoteThenCache(t *testing.T) {
	var calls atomic.Int32
	policy := validApprovalPolicy()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("X-Internal-Key"); got != "test-internal-key" {
			http.Error(w, "missing internal key", http.StatusUnauthorized)
			return
		}
		if got := r.URL.Path; got != "/api/v1/auth/internal/tenants/11111111-1111-1111-1111-111111111111/approval-policy" {
			http.Error(w, "unexpected path", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": policy})
	}))
	defer server.Close()

	now := time.Now()
	client := newApprovalPolicyTestClient(server.URL, &now)

	remote, err := client.GetApprovalPolicy(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("remote fetch: %v", err)
	}
	if remote.ResolutionSource != ApprovalPolicyResolutionRemote || remote.Revision != 7 {
		t.Fatalf("unexpected remote policy: %+v", remote)
	}

	// Mutating the returned slice must not mutate the cached contract.
	remote.TargetStatuses[0] = "BROKEN"
	cached, err := client.GetApprovalPolicy(context.Background(), "11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatalf("cache fetch: %v", err)
	}
	if cached.ResolutionSource != ApprovalPolicyResolutionCache {
		t.Fatalf("resolution = %q", cached.ResolutionSource)
	}
	if cached.TargetStatuses[0] != "ANALYSIS" {
		t.Fatalf("cached policy was mutated: %+v", cached.TargetStatuses)
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls.Load())
	}
}

func TestAuthClientGetApprovalPolicyUsesBoundedStaleCache(t *testing.T) {
	var fail atomic.Bool
	policy := validApprovalPolicy()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": policy})
	}))
	defer server.Close()

	now := time.Now()
	client := newApprovalPolicyTestClient(server.URL, &now)
	tenantID := "22222222-2222-2222-2222-222222222222"
	if _, err := client.GetApprovalPolicy(context.Background(), tenantID); err != nil {
		t.Fatalf("seed cache: %v", err)
	}

	fail.Store(true)
	now = now.Add(2 * time.Minute) // fresh expired, stale still valid
	stale, err := client.GetApprovalPolicy(context.Background(), tenantID)
	if err != nil {
		t.Fatalf("stale fetch: %v", err)
	}
	if stale.ResolutionSource != ApprovalPolicyResolutionStaleCache || stale.Revision != policy.Revision {
		t.Fatalf("unexpected stale policy: %+v", stale)
	}

	now = now.Add(10 * time.Minute) // stale window expired
	if _, err := client.GetApprovalPolicy(context.Background(), tenantID); err == nil {
		t.Fatal("expected upstream error after stale window")
	}
}

func TestAuthClientGetApprovalPolicyRejectsInvalidContract(t *testing.T) {
	policy := validApprovalPolicy()
	policy.RequiredPermission = "admin"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": policy})
	}))
	defer server.Close()

	now := time.Now()
	client := newApprovalPolicyTestClient(server.URL, &now)
	if _, err := client.GetApprovalPolicy(context.Background(), "33333333-3333-3333-3333-333333333333"); err == nil {
		t.Fatal("expected invalid policy contract error")
	}
}
