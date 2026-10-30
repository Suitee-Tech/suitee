package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	service "github.com/oginoino/toca/toca-financial/internal/services"
)

// A recusa por segregação de funções precisa chegar ao operador como conflito de
// autoridade (409) com a causa visível. Antes deste mapeamento, `transfer` e
// `withdrawal` caíam no default e devolviam **500** — e o sanitizador de erro
// interno (M17) apagava a mensagem, então o operador não tinha como saber que o
// impedimento era ele mesmo ser as duas pontas da operação. `payout` e comissão
// devolviam 400, culpando o corpo da requisição.
func TestSegregationOfDutiesErrorsRenderAsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	conflictRecorder := func(render func(*gin.Context, error)) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
		render(c, service.ErrSegregationOfDutiesRequesterCannotApprove)
		return recorder
	}

	tests := []struct {
		name   string
		render func(*gin.Context, error)
	}{
		{name: "comissão", render: respondCommissionLifecycleError},
		{name: "payout order", render: respondPayoutOrderLifecycleError},
		{name: "saque", render: func(c *gin.Context, err error) { (&WithdrawalHandler{}).writeErr(c, err) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := conflictRecorder(tt.render)
			if recorder.Code != http.StatusConflict {
				t.Fatalf("status=%d body=%s, want 409", recorder.Code, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "segregation of duties") {
				t.Fatalf("body=%s, want a causa explícita", recorder.Body.String())
			}
		})
	}

	// O mapeamento é cirúrgico: erro comum continua com o status de antes.
	otherErr := errors.New("commission has already been paid")
	for _, tt := range []struct {
		name   string
		render func(*gin.Context, error)
		want   int
	}{
		{name: "comissão mantém 400", render: respondCommissionLifecycleError, want: http.StatusBadRequest},
		{name: "payout order mantém 400", render: respondPayoutOrderLifecycleError, want: http.StatusBadRequest},
		{name: "saque mantém 500", render: func(c *gin.Context, err error) { (&WithdrawalHandler{}).writeErr(c, err) }, want: http.StatusInternalServerError},
	} {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/probe", nil)
			tt.render(c, otherErr)
			if recorder.Code != tt.want {
				t.Fatalf("status=%d, want %d", recorder.Code, tt.want)
			}
		})
	}
}

type transferSoDRepo struct {
	transfer.RepositoryInterface
	item *transfer.OwnerTransfer
}

func (r *transferSoDRepo) GetByID(_ context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	if r.item == nil || r.item.TenantID != tenantID || r.item.ID != id {
		return nil, nil
	}
	clone := *r.item
	return &clone, nil
}

func (r *transferSoDRepo) Update(_ context.Context, t *transfer.OwnerTransfer) error {
	clone := *t
	r.item = &clone
	return nil
}

// O repasse tem a mesma regra (aprovador ≠ pagador) e a recusa passa pelo
// handler: prova que a identidade comparada é a do contexto autenticado e que a
// resposta é 409 — antes era 500 sem causa visível.
func TestTransferProcessSoDMapsToHTTP409(t *testing.T) {
	gin.SetMode(gin.TestMode)
	item := transfer.New("contract-sod-http", "owner-sod-http", "prop-sod-http", "tenant-sod-http", "2026-09",
		5000, 10, 0, 0, nil, "")
	item.ID = "transfer-sod-http"
	if err := item.Approve("user-approver", "Aprovador"); err != nil {
		t.Fatalf("seed approve: %v", err)
	}
	handler := NewTransferHandler(service.NewTransferService(&transferSoDRepo{item: item}))
	router := gin.New()
	router.Use(func(c *gin.Context) {
		ctx := pkgcontext.WithTenantID(c.Request.Context(), item.TenantID)
		ctx = pkgcontext.WithAccountID(ctx, "user-approver")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	router.POST("/transfers/:id/process", handler.Process)

	request := httptest.NewRequest(http.MethodPost, "/transfers/transfer-sod-http/process",
		strings.NewReader(`{"proof_url":"https://files.example/repasse.pdf"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s, want 409", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "segregation of duties") {
		t.Fatalf("body=%s, want a causa explícita", recorder.Body.String())
	}
	if item.Status == transfer.StatusProcessed {
		t.Fatal("repasse não pode ser efetivado pelo próprio aprovador")
	}
}
