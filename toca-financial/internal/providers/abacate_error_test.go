package providers

import (
	"encoding/json"
	"testing"
)

// Achado em produção (2026-10-07): a loja do AbacatePay não tinha cobrança
// recorrente habilitada e a API respondeu 400 "CARD is not available for this
// store". O cliente devolvia indisponibilidade genérica, então a tela mandava
// "atualize a assinatura" para um problema de CONTA — que repetir não resolve.
func TestRecusaDeCobrancaRecorrenteTemErroProprio(t *testing.T) {
	casos := []struct {
		mensagem string
		recusa   bool
	}{
		{"CARD is not available for this store", true},
		{"PIX Automático is not available for this store", true},
		{"Billing not found", false},
		{"internal server error", false},
		{"", false},
	}
	for _, caso := range casos {
		raw, _ := json.Marshal(caso.mensagem)
		if got := paymentMethodUnavailable(raw); got != caso.recusa {
			t.Fatalf("paymentMethodUnavailable(%q) = %v, esperado %v", caso.mensagem, got, caso.recusa)
		}
	}
}

// A mensagem do provedor entra no log, nunca no erro devolvido ao cliente.
func TestProviderMessageNaoVazaNoErro(t *testing.T) {
	raw, _ := json.Marshal("secret-test raw provider error")
	if got := providerMessage(raw); got != "secret-test raw provider error" {
		t.Fatalf("providerMessage = %q", got)
	}
	longo, _ := json.Marshal(string(make([]byte, 300)))
	if len(providerMessage(longo)) != 200 {
		t.Fatalf("mensagem longa deveria ser truncada em 200, veio %d", len(providerMessage(longo)))
	}
}
