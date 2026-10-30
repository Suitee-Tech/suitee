package response

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/logger"
)

// M17 — erro interno não vai para o cliente.
//
// A revisão de permissões de 2026-09-19 registrou 500 devolvendo `err.Error()`:
// nome de tabela/coluna, violação de constraint e mensagem do driver na resposta.
// O contrato do helper é: resposta genérica para o cliente, detalhe no log.
func TestInternalErrorHidesDetailFromClient(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/boom", func(c *gin.Context) {
		InternalError(c, errors.New(`pq: relation "toca_auth.accounts" does not exist (SQLSTATE 42P01)`))
	})

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", recorder.Code)
	}
	body := recorder.Body.String()
	for _, leak := range []string{"toca_auth.accounts", "SQLSTATE", "42P01", "pq:"} {
		if strings.Contains(body, leak) {
			t.Fatalf("resposta vazou detalhe interno (%q): %s", leak, body)
		}
	}
	if !strings.Contains(body, "erro interno") {
		t.Fatalf("resposta deveria trazer a mensagem genérica: %s", body)
	}
}

// O detalhe PRECISA continuar disponível para quem opera: vai para o log
// estruturado (com caminho e método), não para o cliente.
func TestInternalErrorLogsDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// O logger captura os.Stdout no Setup; redirecionar para um arquivo antes
	// dele permite observar a linha emitida.
	tmp, err := os.CreateTemp(t.TempDir(), "log-*.json")
	if err != nil {
		t.Fatalf("temp log: %v", err)
	}
	original := os.Stdout
	os.Stdout = tmp
	logger.Setup(logger.Config{Level: "error", Format: "json"})
	t.Cleanup(func() {
		os.Stdout = original
		logger.Setup(logger.Config{Level: "info", Format: "json"})
		_ = tmp.Close()
	})

	engine := gin.New()
	engine.GET("/boom", func(c *gin.Context) {
		InternalError(c, errors.New(`pq: duplicate key value violates unique constraint "accounts_email_key"`))
	})
	engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/boom", nil))

	emitted, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	line := string(emitted)
	if !strings.Contains(line, "http.internal_error") {
		t.Fatalf("evento de log ausente: %s", line)
	}
	if !strings.Contains(line, "accounts_email_key") {
		t.Fatalf("detalhe do erro não foi logado (operador perde o rastro): %s", line)
	}
	if !strings.Contains(line, "/boom") {
		t.Fatalf("log sem o caminho da requisição: %s", line)
	}
}
