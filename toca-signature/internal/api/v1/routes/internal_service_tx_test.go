package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/database"
)

type ctxKey string

// fakeRunner simula o RunInServiceContext: injeta a marca da transação de
// serviço no contexto, como o PostgresDB faz com withServiceTx.
type fakeRunner struct {
	err   error
	ran   int
	serve bool
}

func (f *fakeRunner) RunInServiceContext(ctx context.Context, fn func(context.Context) error) error {
	f.ran++
	if f.err != nil && !f.serve {
		return f.err
	}
	return fn(context.WithValue(ctx, ctxKey("service_tx"), true))
}

func runInternal(t *testing.T, runner serviceTxRunner) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	sawServiceTx := false
	r := gin.New()
	r.GET("/internal/envelope", internalServiceTx(runner), func(c *gin.Context) {
		sawServiceTx, _ = c.Request.Context().Value(ctxKey("service_tx")).(bool)
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/internal/envelope", nil))
	return w, sawServiceTx
}

// A rota interna precisa rodar com o contexto da transação de serviço — sem
// isso o pool de aplicação fica sujeito à RLS e a chamada serviço-a-serviço
// (sem JWT) recebe 404 para uma linha que existe.
func TestInternalServiceTxRunsHandlerInsideServiceContext(t *testing.T) {
	runner := &fakeRunner{}
	w, saw := runInternal(t, runner)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !saw {
		t.Error("handler não recebeu o contexto da transação de serviço")
	}
	if runner.ran != 1 {
		t.Errorf("RunInServiceContext chamado %d vezes, want 1", runner.ran)
	}
}

// Sem pool de serviço configurado o middleware não pode derrubar a rota: avisa e
// segue (o mesmo trade-off do toca-deal).
func TestInternalServiceTxFallsBackWithoutServicePool(t *testing.T) {
	runner := &fakeRunner{err: database.ErrServiceRoleRequired}
	w, _ := runInternal(t, runner)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (fallback silencioso)", w.Code)
	}
}

// Falha real (banco fora) tem de virar 5xx, não resposta de sucesso.
func TestInternalServiceTxReportsRealFailures(t *testing.T) {
	runner := &fakeRunner{err: errors.New("banco indisponível")}
	w, _ := runInternal(t, runner)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
}

// Sem runner (banco não injetado) a rota continua respondendo.
func TestInternalServiceTxWithoutRunner(t *testing.T) {
	w, _ := runInternal(t, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}
