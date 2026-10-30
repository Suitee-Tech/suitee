package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"github.com/oginoino/toca/toca-deal/internal/services"
)

type portalChargesAccessRepo struct{ deal.Repository }

func (r portalChargesAccessRepo) FindBySignerIdentity(context.Context, string, string) ([]*deal.Deal, error) {
	return []*deal.Deal{{ID: "deal-charges", TenantID: "11111111-1111-4111-8111-111111111111"}}, nil
}

// Incidente 2026-09-19: a tela Financeiro do portal do cliente mostrava
// "Não foi possível carregar" com 403 no console. O 403 era o handler do portal
// traduzindo QUALQUER erro — inclusive o 400 do toca-financial por falta de
// `tenant_id` na rota interna — como acesso negado.
//
// Este teste fixa o mapeamento: 200 quando o destino responde, 503 quando o
// destino falha (sem vazar o texto interno) e 403 apenas para negócio alheio.
func TestPortalChargesHTTPStatusMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("INTERNAL_API_KEY", "test-key")

	tests := []struct {
		name        string
		financial   func(w http.ResponseWriter, r *http.Request)
		dealID      string
		wantStatus  int
		wantContain string
		wantAbsent  string
		// G32: encargos discriminados têm de chegar ao corpo que o portal lê.
		wantAll []string
	}{
		{
			name: "cobranças do próprio negócio",
			financial: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("tenant_id") != "11111111-1111-4111-8111-111111111111" {
					t.Errorf("tenant_id ausente na chamada interna: %s", r.URL.RawQuery)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":"charge-1","status":"OVERDUE","amount":3200,"late_fee":64,"interest":12.8,"discount":0,"total_amount":3276.8,"days_overdue":23}]}`))
			},
			dealID:      "deal-charges",
			wantStatus:  http.StatusOK,
			wantContain: "charge-1",
			wantAll:     []string{`"late_fee":64`, `"interest":12.8`, `"discount":0`, `"total_amount":3276.8`, `"days_overdue":23`},
		},
		{
			name: "destino devolve 400 (tenant ausente) não é acesso negado",
			financial: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"message":"tenant id required"}`))
			},
			dealID:      "deal-charges",
			wantStatus:  http.StatusServiceUnavailable,
			wantContain: "upstream service unavailable",
			wantAbsent:  "returned status",
		},
		{
			name:        "negócio de terceiro",
			financial:   func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(200) },
			dealID:      "deal-of-someone-else",
			wantStatus:  http.StatusForbidden,
			wantContain: "access denied",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			financial := httptest.NewServer(http.HandlerFunc(tt.financial))
			t.Cleanup(financial.Close)
			t.Setenv("FINANCIAL_SERVICE_URL", financial.URL)
			t.Setenv("FINANCIAL_SERVICE_URL_INTERNAL", financial.URL)

			service := services.NewDealService(portalChargesAccessRepo{}, nil, nil, nil, nil)
			handler := NewDealHandler(service)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithAccountID(c.Request.Context(), "account-1")
				c.Request = c.Request.WithContext(ctx)
			})
			router.GET("/client-portal/deals/:id/charges", handler.GetMyDealCharges)

			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/client-portal/deals/"+tt.dealID+"/charges", nil))

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d (%s), want %d", recorder.Code, recorder.Body.String(), tt.wantStatus)
			}
			if tt.wantContain != "" && !strings.Contains(recorder.Body.String(), tt.wantContain) {
				t.Fatalf("body = %s, want conter %q", recorder.Body.String(), tt.wantContain)
			}
			if tt.wantAbsent != "" && strings.Contains(recorder.Body.String(), tt.wantAbsent) {
				t.Fatalf("body = %s não pode conter %q (vazamento de erro interno)", recorder.Body.String(), tt.wantAbsent)
			}
			for _, want := range tt.wantAll {
				if !strings.Contains(recorder.Body.String(), want) {
					t.Fatalf("body = %s, want conter %q", recorder.Body.String(), want)
				}
			}
		})
	}
}
