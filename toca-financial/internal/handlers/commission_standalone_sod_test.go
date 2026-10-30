package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// SECURITY (achado A13 da revisão de permissões de 2026-09-19): comissão avulsa
// — o mesmo ator que pede não aprova, e quem aprova não paga. Antes, as três
// rotas exigiam apenas `finance:write`, que o `financeiro` tem: um único usuário
// criava, aprovava e pagava a própria comissão.
//
// O teste passa pelo handler de propósito: prova que a identidade usada na
// comparação é a do contexto autenticado (`account_id` do middleware), não um
// campo do corpo — e que a recusa sai como 409 com a causa visível, e não como
// 500/400 genérico.
func TestCommissionStandaloneSegregationOfDuties(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		path       string
		account    string
		role       string
		item       *commission.Commission
		wantStatus int
		wantBody   string
		wantBy     string
	}{
		{
			name:       "solicitante não aprova a própria comissão avulsa",
			path:       "/commissions/commission-sod/approve",
			account:    "user-requester",
			item:       standalonePendingCommission("user-requester"),
			wantStatus: http.StatusConflict,
			wantBody:   "requester cannot approve",
		},
		{
			name:       "outro operador aprova e fica registrado como aprovador",
			path:       "/commissions/commission-sod/approve",
			account:    "user-approver",
			item:       standalonePendingCommission("user-requester"),
			wantStatus: http.StatusOK,
			wantBy:     "user-approver",
		},
		{
			name:       "aprovador não paga o que aprovou",
			path:       "/commissions/commission-sod/pay",
			account:    "user-approver",
			item:       approvedStandaloneCommission("user-requester", "user-approver"),
			wantStatus: http.StatusConflict,
			wantBody:   "approver cannot settle the payment",
		},
		{
			name:       "terceiro operador conclui o pagamento",
			path:       "/commissions/commission-sod/pay",
			account:    "user-payer",
			item:       approvedStandaloneCommission("user-requester", "user-approver"),
			wantStatus: http.StatusOK,
		},
		{
			name:       "registro sem autor não cria conflito",
			path:       "/commissions/commission-sod/pay",
			account:    "user-payer",
			item:       standalonePendingCommission(""),
			wantStatus: http.StatusOK,
		},
		{
			name:       "dispensa do admin libera o ator único",
			path:       "/commissions/commission-sod/approve",
			account:    "user-requester",
			role:       "admin",
			item:       standalonePendingCommission("user-requester"),
			wantStatus: http.StatusOK,
			wantBy:     "user-requester",
		},
		{
			name:       "financeiro NÃO tem a dispensa",
			path:       "/commissions/commission-sod/approve",
			account:    "user-requester",
			role:       "financeiro",
			item:       standalonePendingCommission("user-requester"),
			wantStatus: http.StatusConflict,
			wantBody:   "requester cannot approve",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &commissionLifecycleConflictRepo{item: tt.item}
			handler := NewCommissionHandler(service.NewCommissionService(repo))
			router := gin.New()
			router.Use(func(c *gin.Context) {
				ctx := pkgcontext.WithTenantID(c.Request.Context(), tt.item.TenantID)
				ctx = pkgcontext.WithAccountID(ctx, tt.account)
				if tt.role != "" {
					ctx = pkgcontext.WithFunctionalRole(ctx, tt.role)
				}
				c.Request = c.Request.WithContext(ctx)
				c.Next()
			})
			router.POST("/commissions/:id/approve", handler.ApproveCommission)
			router.POST("/commissions/:id/pay", handler.MarkPaid)

			request := httptest.NewRequest(http.MethodPost, tt.path,
				strings.NewReader(`{"proof_url":"https://files.example/comissao.pdf"}`))
			request.Header.Set("Content-Type", "application/json")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status=%d body=%s, want %d", recorder.Code, recorder.Body.String(), tt.wantStatus)
			}
			if tt.wantBody != "" && !strings.Contains(recorder.Body.String(), tt.wantBody) {
				t.Fatalf("body=%s, want mensagem contendo %q", recorder.Body.String(), tt.wantBody)
			}
			if tt.wantBy != "" && repo.item.ApprovedBy != tt.wantBy {
				t.Fatalf("approved_by=%q, want %q", repo.item.ApprovedBy, tt.wantBy)
			}
			if tt.wantStatus == http.StatusConflict && repo.item.Status != tt.item.Status {
				t.Fatalf("status do agregado mudou para %s apesar da recusa", repo.item.Status)
			}
		})
	}
}

func standalonePendingCommission(requestedBy string) *commission.Commission {
	item := commission.New("deal-sod", "broker-sod", 100, "tenant-sod")
	item.ID = "commission-sod"
	item.RequestedBy = requestedBy
	return item
}

func approvedStandaloneCommission(requestedBy, approvedBy string) *commission.Commission {
	item := standalonePendingCommission(requestedBy)
	item.Status = commission.StatusApproved
	item.ApprovedBy = approvedBy
	return item
}
