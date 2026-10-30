package handlers

import (
	"testing"
)

// P2/M10 — webhook de canal falhava ABERTO sem o segredo configurado.
//
// A revisão de permissões (achado M10) mostrou que os webhooks de WhatsApp e SMS
// pulavam a validação de assinatura quando a variável de segredo estava ausente:
// qualquer um injetava eventos forjados — e o segredo ausente é exatamente o
// estado de um deploy mal configurado.
//
// Política: segredo presente ⇒ assinatura obrigatória; ausente em produção ⇒
// RECUSA; ausente fora de produção ⇒ aceita com WARN (dev local).
func TestWebhookSignatureMode(t *testing.T) {
	cases := []struct {
		nome        string
		secret      string
		mode        string
		env         string
		wantRequire bool
		wantRelease bool
	}{
		{"segredo presente em produção", "s3cr3t", "release", "prod", true, true},
		{"segredo presente em dev", "s3cr3t", "debug", "development", true, false},
		{"sem segredo em produção (APP_MODE)", "", "release", "", false, true},
		{"sem segredo em produção (APP_ENV)", "", "", "production", false, true},
		{"sem segredo em dev", "", "debug", "development", false, false},
		{"segredo em branco conta como ausente em produção", "   ", "release", "prod", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.nome, func(t *testing.T) {
			t.Setenv("WHATSAPP_APP_SECRET", tc.secret)
			t.Setenv("APP_MODE", tc.mode)
			t.Setenv("APP_ENV", tc.env)

			secret, require, release := webhookSignatureMode("WHATSAPP_APP_SECRET")
			if require != tc.wantRequire {
				t.Errorf("require = %v, esperado %v", require, tc.wantRequire)
			}
			if release != tc.wantRelease {
				t.Errorf("release = %v, esperado %v", release, tc.wantRelease)
			}
			if require && secret == "" {
				t.Error("require=true com segredo vazio é incoerente")
			}
		})
	}
}

// Os dois handlers usam a MESMA política — a env de cada canal é o que muda.
func TestWebhookSignatureMode_PerChannelEnv(t *testing.T) {
	t.Setenv("APP_MODE", "release")
	t.Setenv("APP_ENV", "prod")
	t.Setenv("WHATSAPP_APP_SECRET", "wh-secret")
	t.Setenv("SMS_ZENVIA_WEBHOOK_SECRET", "")

	if _, require, _ := webhookSignatureMode("WHATSAPP_APP_SECRET"); !require {
		t.Error("WhatsApp com segredo configurado precisa exigir assinatura")
	}
	if _, require, _ := webhookSignatureMode("SMS_ZENVIA_WEBHOOK_SECRET"); require {
		t.Error("SMS sem segredo não pode exigir assinatura (e o handler recusa o evento em produção)")
	}
}
