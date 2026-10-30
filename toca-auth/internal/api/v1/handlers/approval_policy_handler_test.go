package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca-auth/internal/domain/tenantsettings"
	"github.com/oginoino/toca-auth/internal/service"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
)

type handlerApprovalPolicyRepo struct{}

func (handlerApprovalPolicyRepo) GetApprovalPolicy(context.Context, string) (tenantsettings.ApprovalPolicy, bool, error) {
	policy := tenantsettings.DefaultApprovalPolicy()
	policy.Revision = 3
	policy.ThresholdBRL = 650_000
	policy.Source = "tenant_settings"
	return policy, true, nil
}

func (handlerApprovalPolicyRepo) UpsertApprovalPolicy(context.Context, string, bool, float64, string) (tenantsettings.ApprovalPolicy, error) {
	return tenantsettings.ApprovalPolicy{}, nil
}

func TestApprovalPolicyInternalEndpointRequiresInternalKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", "approval-test-key")
	handler := NewApprovalPolicyHandler(service.NewApprovalPolicyService(handlerApprovalPolicyRepo{}))
	router := gin.New()
	router.GET("/internal/tenants/:tenant_id/approval-policy", pkgAuth.RequireInternalKey(), handler.GetInternal)

	unauthorized := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/internal/tenants/11111111-1111-1111-1111-111111111111/approval-policy", nil)
	router.ServeHTTP(unauthorized, req)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("missing key status = %d, want 401", unauthorized.Code)
	}

	authorized := httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/internal/tenants/11111111-1111-1111-1111-111111111111/approval-policy", nil)
	req.Header.Set("X-Internal-Key", "approval-test-key")
	router.ServeHTTP(authorized, req)
	if authorized.Code != http.StatusOK {
		t.Fatalf("valid key status = %d, body=%s", authorized.Code, authorized.Body.String())
	}
	if body := authorized.Body.String(); !containsAll(body, `"revision":3`, `"threshold_brl":650000`, `"source":"tenant_settings"`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
