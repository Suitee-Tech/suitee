package handlers

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/logger"
)

// SMSWebhookHandler aceita callbacks da Zenvia para status de entrega
// de SMS (ACCEPTED → SENT → DELIVERED | NOT_DELIVERED | REJECTED). Issue
// #444 follow-up.
//
// Diferenças vs. WhatsApp webhook (#443):
//   - Zenvia NÃO faz handshake de verificação GET (não tem hub.challenge).
//     Só POST.
//   - Assinatura usa HMAC-SHA1 em header X-ZENVIA-SIGNATURE (formato
//     "sha1=HEX"), não SHA-256.
//   - Payload é flat (sem entry/changes/value nested), uma message por
//     callback. Pode vir em batches dependendo da config do subscription
//     na Zenvia — aceitamos tanto objeto único quanto array via duck-type
//     no Unmarshal.
//
// Env vars usadas:
//   SMS_ZENVIA_WEBHOOK_SECRET  — secret compartilhado no cadastro do
//                                 webhook na Zenvia. Quando ausente, em
//                                 dev/staging, aceita sem validar e loga
//                                 warning (mesma postura do WhatsApp).
//
// Status semântica (Zenvia → significado):
//   ACCEPTED       — Zenvia recebeu, ainda não enviou ao carrier
//   SENT           — entregue ao carrier
//   DELIVERED      — confirmado no aparelho (handset ACK)
//   NOT_DELIVERED  — falha no carrier (número off, fora de cobertura,
//                    bloqueio operador)
//   REJECTED       — Zenvia recusou (sender não aprovado, conteúdo
//                    bloqueado por regras Anatel)
//   READ           — raro em SMS (só alguns operadores suportam)

type SMSWebhookHandler struct{}

func NewSMSWebhookHandler() *SMSWebhookHandler {
	return &SMSWebhookHandler{}
}

// Receive trata POST de status updates. Mesmo padrão do WhatsApp: 200
// sempre que possível para evitar retry loop. Audit per-status para
// forensics + reaction futura.
func (h *SMSWebhookHandler) Receive(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "sms webhook: failed to read body", "error", err.Error())
		c.Status(http.StatusOK)
		return
	}

	// P2/M10: idem WhatsApp — sem segredo em produção o evento é recusado.
	secret, requireSignature, release := webhookSignatureMode("SMS_ZENVIA_WEBHOOK_SECRET")
	if !requireSignature {
		if release {
			rejectUnsignedWebhook("sms", "SMS_ZENVIA_WEBHOOK_SECRET", "missing_secret_in_release")
			c.Status(http.StatusOK)
			return
		}
		rejectUnsignedWebhook("sms", "SMS_ZENVIA_WEBHOOK_SECRET", "missing_secret_dev")
	} else {
		sig := c.GetHeader("X-ZENVIA-SIGNATURE")
		if !verifyZenviaSignature(secret, body, sig) {
			logger.WarnContext(c.Request.Context(), "sms webhook: signature mismatch", "received_sig", sig)
			c.Status(http.StatusOK)
			return
		}
	}

	// Zenvia pode mandar um objeto único OU um array, dependendo do tipo
	// de subscription (single vs. batch). Tentamos array primeiro e, se
	// falhar, decodificamos como objeto único.
	var events []zenviaWebhookEvent
	if err := json.Unmarshal(body, &events); err != nil {
		var single zenviaWebhookEvent
		if err2 := json.Unmarshal(body, &single); err2 != nil {
			logger.WarnContext(c.Request.Context(), "sms webhook: invalid JSON", "error", err.Error(), "fallback_error", err2.Error())
			c.Status(http.StatusOK)
			return
		}
		events = []zenviaWebhookEvent{single}
	}

	processed := 0
	for _, ev := range events {
		// Aceitamos apenas MESSAGE_STATUS (não MESSAGE — que é inbound,
		// recebido — esse fluxo é separado e não é o foco do #444).
		if ev.Type != "" && ev.Type != "MESSAGE_STATUS" {
			continue
		}
		processed++
		audit.Record(c.Request.Context(), audit.Entry{
			Action:     "sms.delivery_status",
			EntityType: "SMSMessage",
			EntityID:   ev.MessageID,
			Metadata: map[string]any{
				"status":          ev.MessageStatus.Code,
				"timestamp":       ev.MessageStatus.Timestamp,
				"description":     ev.MessageStatus.Description,
				"channel":         ev.Channel,
				"subscription_id": ev.SubscriptionID,
				"content_index":   ev.ContentIndex,
			},
		})
		// Próximo PR: lookup Charge.RemindersSent por messageID → atualiza
		// DeliveryStatus + DeliveredAt. Mesma issue que WhatsApp (#443
		// follow-up) — exige migração de schema para persistir o
		// providerMessageID no envio original.
	}

	logger.InfoContext(c.Request.Context(), "sms webhook processed",
		"event_count", len(events),
		"status_count", processed,
	)
	c.Status(http.StatusOK)
}

// verifyZenviaSignature compara o HMAC-SHA1(body, secret) com o header
// X-ZENVIA-SIGNATURE. Zenvia usa prefixo "sha1=" (Meta usa sha256). Por
// que SHA-1 ainda? Documentação atual da Zenvia. Não é o ideal
// criptograficamente mas — HMAC-SHA1 com chave forte e mensagem de
// alguns KB ainda é seguro pra esse use-case.
func verifyZenviaSignature(secret string, body []byte, header string) bool {
	if !strings.HasPrefix(header, "sha1=") {
		return false
	}
	expectedHex := strings.TrimPrefix(header, "sha1=")
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write(body)
	computed := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expectedHex), []byte(computed))
}

// zenviaWebhookEvent — shape do POST que Zenvia envia para MESSAGE_STATUS.
// Documentação:
// https://zenvia.github.io/zenvia-openapi-spec/v2/#operation/subscribeMessageStatus
type zenviaWebhookEvent struct {
	ID             string              `json:"id"`
	Type           string              `json:"type"` // "MESSAGE_STATUS"
	SubscriptionID string              `json:"subscriptionId"`
	Channel        string              `json:"channel"` // "sms"
	MessageID      string              `json:"messageId"`
	Direction      string              `json:"direction"`
	ContentIndex   int                 `json:"contentIndex"`
	MessageStatus  zenviaMessageStatus `json:"messageStatus"`
}

type zenviaMessageStatus struct {
	Timestamp   string `json:"timestamp"`
	Code        string `json:"code"` // ACCEPTED|SENT|DELIVERED|NOT_DELIVERED|REJECTED|READ
	Description string `json:"description"`
}
