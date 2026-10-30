package handlers

import (
	"os"
	"strings"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/logger"
)

// P2/M10 — webhook de canal falhava ABERTO quando o segredo não estava
// configurado.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado M10) mostrou que `whatsapp_webhook_handler` e `sms_webhook_handler`
// pulavam a validação de assinatura quando a variável de segredo estava ausente:
// qualquer um podia injetar eventos/status forjados na plataforma — e a ausência
// do segredo é justamente o estado de um deploy mal configurado.
//
// Política agora:
//
//   - segredo presente  → assinatura OBRIGATÓRIA (comportamento já existente);
//   - segredo ausente em produção (APP_MODE=release ou APP_ENV=production/prod)
//     → RECUSA o evento (fail-closed, com ERROR no log);
//   - segredo ausente fora de produção → aceita com WARN, para não quebrar o
//     desenvolvimento local (mesmo critério de "release" usado no restante da
//     frota).
func webhookSignatureMode(secretEnv string) (secret string, require bool, release bool) {
	secret = strings.TrimSpace(os.Getenv(secretEnv))
	release = isReleaseEnv()
	if secret != "" {
		return secret, true, release
	}
	// Sem segredo: NÃO há assinatura a exigir (não há chave); quem decide entre
	// recusar e aceitar em modo dev é o `release` devolvido.
	return "", false, release
}

// isReleaseEnv usa o critério único da frota (pkg/config) — antes era uma
// cópia que precisava "espelhar" gateway/toca-mcp à mão (P2/M16).
func isReleaseEnv() bool {
	return config.IsReleaseEnv(os.Getenv("APP_ENV"), os.Getenv("APP_MODE"))
}

// rejectUnsignedWebhook registra e responde o status combinado com o provedor.
// Sempre 200 para o provedor (non-2xx vira retry infinito); o que muda é NÃO
// processar o evento.
func rejectUnsignedWebhook(channel, secretEnv, reason string) {
	if reason == "missing_secret_in_release" {
		logger.Error("webhook."+channel+".missing_secret",
			"event", "webhook.missing_secret",
			"channel", channel,
			"env_var", secretEnv,
			"hint", "configure o segredo: sem ele o webhook recusa todo evento em produção",
		)
		return
	}
	logger.Warn("webhook."+channel+".unsigned_accepted",
		"event", "webhook.unsigned_accepted",
		"channel", channel,
		"env_var", secretEnv,
		"hint", "sem o segredo o evento é aceito APENAS fora de produção",
	)
}
