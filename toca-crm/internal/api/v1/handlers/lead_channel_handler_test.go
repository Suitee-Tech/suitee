package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

type channelLeadRepo struct {
	lead.Repository
	item *lead.Lead
}

func (r *channelLeadRepo) GetByID(_ context.Context, tenantID, _ string) (*lead.Lead, error) {
	// Espelha o contrato tenant-scoped do repositório real (achado C9).
	if r.item == nil {
		return nil, nil
	}
	if tenantID != "" && r.item.TenantID != "" && r.item.TenantID != tenantID {
		return nil, nil
	}
	return r.item, nil
}

func (r *channelLeadRepo) GetByIDAnyTenant(context.Context, string) (*lead.Lead, error) {
	return r.item, nil
}

func TestOpenDirectChannelOwnershipHTTPStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name   string
		item   *lead.Lead
		status int
	}{
		{name: "unrelated client", item: &lead.Lead{AccountID: "owner"}, status: http.StatusForbidden},
		{name: "missing lead", status: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := service.NewLeadService(&channelLeadRepo{item: tc.item}, nil, nil, nil, nil, nil)
			handler := NewLeadHandler(svc, nil, nil)
			router := gin.New()
			router.POST("/client-portal/leads/:id/channel", handler.OpenDirectChannel)
			req := httptest.NewRequest(http.MethodPost, "/client-portal/leads/lead/channel", nil)
			req = req.WithContext(pkgcontext.WithAccountID(req.Context(), "stranger"))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, req)
			if response.Code != tc.status {
				t.Fatalf("got status %d: %s", response.Code, response.Body.String())
			}
		})
	}
}
