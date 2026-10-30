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

// P0/A10 (reauditoria de 2026-09-20) — o escopo de visibilidade da listagem de
// leads é decidido pelo SERVIDOR.
//
// Antes, `?scope=tenant` desligava o recorte por corretor (`brokerID` ficava
// vazio — e ele é o único predicado de corretor da consulta). Como `leads:read`
// é default de corretor, marketing e auditor, qualquer um deles paginava a base
// inteira do tenant, com PII, só trocando a query string.
//
// Os casos "não elevado" falhavam antes e passam depois.

// scopeRecordingRepo captura o filtro que chegou ao repositório.
type scopeRecordingRepo struct {
	lead.Repository
	filters []lead.LeadFilter
}

func (r *scopeRecordingRepo) List(_ context.Context, _ string, filter lead.LeadFilter) ([]*lead.Lead, int64, error) {
	r.filters = append(r.filters, filter)
	return nil, 0, nil
}

// operatorTokenMiddleware publica o contexto que o pkg/auth.Middleware monta a
// partir das claims: perfil (`permission`), papel funcional, conta e tenant.
func operatorTokenMiddleware(profile, role, accountID, tenantID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), tenantID)
		ctx = pkgcontext.WithAccountID(ctx, accountID)
		ctx = pkgcontext.WithFunctionalRole(ctx, role)
		ctx = pkgcontext.WithUserPermission(ctx, profile)
		c.Request = c.Request.WithContext(ctx)

		c.Set("user_id", accountID)
		c.Set("permission", profile)
		c.Set("functional_role", role)
		c.Set("tenant_id", tenantID)
		c.Next()
	}
}

func TestLeadHandler_List_VisibilityScopeIsServerDecided(t *testing.T) {
	const (
		tenant  = "tenant-1"
		account = "broker-1"
	)

	cases := []struct {
		name            string
		profile         string
		role            string
		query           string
		wantBrokerScope string
	}{
		{
			name:            "corretor pedindo scope=tenant é recortado para a própria carteira",
			profile:         "member",
			role:            "corretor",
			query:           "?scope=tenant&search=ab",
			wantBrokerScope: account,
		},
		{
			name:            "corretor sem scope é recortado",
			profile:         "member",
			role:            "corretor",
			query:           "",
			wantBrokerScope: account,
		},
		{
			name:            "marketing pedindo scope=tenant é recortado",
			profile:         "member",
			role:            "marketing",
			query:           "?scope=tenant&search=ab",
			wantBrokerScope: account,
		},
		{
			name:            "auditor pedindo scope=tenant é recortado",
			profile:         "member",
			role:            "auditor",
			query:           "?scope=tenant&search=ab",
			wantBrokerScope: account,
		},
		{
			name:            "gestor pedindo scope=tenant vê o tenant",
			profile:         "member",
			role:            "gestor",
			query:           "?scope=tenant&search=ab",
			wantBrokerScope: "",
		},
		{
			name:            "perfil admin vê o tenant",
			profile:         "admin",
			role:            "admin",
			query:           "?scope=tenant&search=ab",
			wantBrokerScope: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &scopeRecordingRepo{}
			svc := service.NewLeadService(repo, nil, nil, nil, nil, nil)
			h := NewLeadHandler(svc, nil, nil)

			gin.SetMode(gin.TestMode)
			router := gin.New()
			router.Use(operatorTokenMiddleware(tc.profile, tc.role, account, tenant))
			router.GET("/leads", h.List)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/leads"+tc.query, nil))

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, quer 200 (corpo: %s)", w.Code, w.Body.String())
			}
			if len(repo.filters) != 1 {
				t.Fatalf("consultas ao repositório = %d, quer 1", len(repo.filters))
			}
			if got := repo.filters[0].BrokerScopeID; got != tc.wantBrokerScope {
				t.Fatalf("BrokerScopeID = %q, quer %q", got, tc.wantBrokerScope)
			}
		})
	}
}
