package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/validation"
)

// O nome sem sobrenome é erro de ENTRADA: o operador precisa da mensagem no
// campo, não de um "erro interno". Era o que acontecia ao adicionar signatário
// a partir de um lead monônimo — a esteira devolvia 500 sem dizer o que corrigir.
func TestRespondSignerErrorMapsPersonNameToValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"monônimo", validation.ValidateFullPersonName("João", "Nome do signatário"), http.StatusBadRequest},
		{"vazio", validation.ValidateFullPersonName("  ", "Nome do signatário"), http.StatusBadRequest},
		{"com número", validation.ValidateFullPersonName("João 2 Silva", "Nome do signatário"), http.StatusBadRequest},
		{"erro envolvido", fmt.Errorf("adicionar signatário: %w", validation.ValidateFullPersonName("João", "Nome do signatário")), http.StatusBadRequest},
		{"erro de infraestrutura", errors.New("failed to persist signer"), http.StatusInternalServerError},
		{"sem erro", nil, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			respondSignerError(c, tc.err)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (corpo: %s)", w.Code, tc.want, w.Body.String())
			}
		})
	}
}
