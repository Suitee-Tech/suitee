package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// Ao ligar o controle de acesso por plano em produção (2026-10-08) a conta com
// checkout pendente recebeu "Seu período de acesso terminou" — descrevendo um
// período que nunca existiu. Cada motivo tem mensagem própria.
func TestMensagemDoPaywallPorMotivo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	casos := []struct {
		motivo   string
		espera   string
		proibido string
	}{
		{"subscription_required", "Escolha um plano para liberar a criação e a edição de dados.", "período de acesso terminou"},
		{"trial_or_paid_period_ended", "Seu período de acesso terminou. Escolha um plano para continuar.", ""},
		{"payment_required", "Regularize a assinatura para continuar usando os recursos de gravação.", ""},
	}
	for _, caso := range casos {
		t.Run(caso.motivo, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/leads", nil)
			writePlanRequired(c, "subscription", nil, caso.motivo)

			if rec.Code != http.StatusPaymentRequired {
				t.Fatalf("status %d, esperado 402", rec.Code)
			}
			var corpo struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Data    struct {
					UpgradeURL string `json:"upgrade_url"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &corpo); err != nil {
				t.Fatalf("corpo inválido: %v", err)
			}
			if corpo.Message != caso.espera {
				t.Fatalf("mensagem %q, esperado %q", corpo.Message, caso.espera)
			}
			if caso.proibido != "" && corpo.Message == caso.proibido {
				t.Fatalf("mensagem ainda usa o texto genérico: %q", corpo.Message)
			}
			if corpo.Code != "plan_required" || corpo.Data.UpgradeURL != "/assinatura" {
				t.Fatalf("contrato do paywall mudou: code=%q upgrade_url=%q", corpo.Code, corpo.Data.UpgradeURL)
			}
		})
	}
}
