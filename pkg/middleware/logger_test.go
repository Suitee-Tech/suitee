package middleware

import "testing"

// O webhook da AbacatePay entrega o segredo combinado em `?webhookSecret=`; o
// log de acesso gravava o path cru e o segredo vazava para o Loki em toda
// entrega de evento (achado em produção em 2026-10-07).
func TestSafePathRedigeParametrosSensiveis(t *testing.T) {
	casos := []struct {
		nome     string
		path     string
		rawQuery string
		want     string
	}{
		{"webhookSecret", "/webhooks/abacatepay", "webhookSecret=segredo-real", "/webhooks/abacatepay?webhookSecret=REDACTED"},
		{"caixa alta", "/webhooks/abacatepay", "WebhookSecret=segredo-real", "/webhooks/abacatepay?WebhookSecret=REDACTED"},
		{"preserva os demais", "/api/v1/deals", "limit=50&token=abc", "/api/v1/deals?limit=50&token=REDACTED"},
		{"sem query", "/health", "", "/health"},
		{"sem sensível", "/api/v1/deals", "limit=50&offset=0", "/api/v1/deals?limit=50&offset=0"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := safePath(caso.path, caso.rawQuery); got != caso.want {
				t.Fatalf("safePath(%q, %q) = %q, esperado %q", caso.path, caso.rawQuery, got, caso.want)
			}
		})
	}
}
