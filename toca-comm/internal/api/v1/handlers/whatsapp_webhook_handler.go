package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/response"
)

// WhatsAppWebhookHandler aceita callbacks da Meta Cloud API para status
// de entrega de mensagens (sent → delivered → read → failed). Issue #443
// follow-up.
//
// Dois métodos:
//   - GET  /webhooks/whatsapp  — verification handshake da Meta. Echo
//     do hub.challenge quando o token bate.
//   - POST /webhooks/whatsapp  — recebe status updates. Valida
//     signature HMAC-SHA256 com app_secret, audit cada status, e por
//     enquanto LOGA — atualização de Charge.RemindersSent via wamid
//     fica para próximo PR (precisa de schema change p/ persistir o
//     wamid no envio original).
//
// Env vars usadas:
//   WHATSAPP_WEBHOOK_VERIFY_TOKEN  — token compartilhado com Meta no
//                                    cadastro do webhook. Setado em
//                                    ambos os lados.
//   WHATSAPP_APP_SECRET            — app secret da Meta Business. Meta
//                                    assina cada POST com HMAC-SHA256
//                                    desse secret + body bytes.

type WhatsAppWebhookHandler struct{}

func NewWhatsAppWebhookHandler() *WhatsAppWebhookHandler {
	return &WhatsAppWebhookHandler{}
}

// Verify trata o handshake inicial de assinatura do webhook na Meta.
// Meta chama GET /webhooks/whatsapp?hub.mode=subscribe&hub.challenge=N&
// hub.verify_token=TOKEN; respondemos com o challenge plain-text quando
// o token bate. Caso contrário 403.
func (h *WhatsAppWebhookHandler) Verify(c *gin.Context) {
	expected := os.Getenv("WHATSAPP_WEBHOOK_VERIFY_TOKEN")
	mode := c.Query("hub.mode")
	token := c.Query("hub.verify_token")
	challenge := c.Query("hub.challenge")

	if expected == "" {
		logger.WarnContext(c.Request.Context(), "whatsapp webhook verify: WHATSAPP_WEBHOOK_VERIFY_TOKEN not set; rejecting all")
		response.Forbidden(c, "webhook verification disabled")
		return
	}
	if mode != "subscribe" || token != expected {
		response.Forbidden(c, "invalid verification token")
		return
	}
	c.String(http.StatusOK, challenge)
}

// Receive trata o POST de status updates. Valida X-Hub-Signature-256
// quando WHATSAPP_APP_SECRET está setado (defesa contra impostor que
// envia callbacks forjados). Audit per-status para forensics + reaction
// future (próximo PR).
//
// Resposta sempre 200 — Meta reentenderá um non-2xx como falha e
// reenfileirará indefinidamente, criando ruído. Erros internos do
// processamento são absorvidos via audit + log; Meta não precisa saber.
func (h *WhatsAppWebhookHandler) Receive(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.WarnContext(c.Request.Context(), "whatsapp webhook: failed to read body", "error", err.Error())
		c.Status(http.StatusOK) // ack mesmo assim para evitar retry loop
		return
	}

	// P2/M10: segredo ausente em produção RECUSA o evento (antes aceitava sem
	// validar). Fora de produção mantém o modo dev, com WARN.
	secret, requireSignature, release := webhookSignatureMode("WHATSAPP_APP_SECRET")
	if !requireSignature {
		if release {
			rejectUnsignedWebhook("whatsapp", "WHATSAPP_APP_SECRET", "missing_secret_in_release")
			c.Status(http.StatusOK)
			return
		}
		rejectUnsignedWebhook("whatsapp", "WHATSAPP_APP_SECRET", "missing_secret_dev")
	} else {
		sig := c.GetHeader("X-Hub-Signature-256")
		if !verifyMetaSignature(secret, body, sig) {
			logger.WarnContext(c.Request.Context(), "whatsapp webhook: signature mismatch", "received_sig", sig)
			// Meta espera 200 quando rejeitamos por signature, senão
			// fica reentregando. Drop silencioso é melhor que loop.
			c.Status(http.StatusOK)
			return
		}
	}

	var payload metaWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		logger.WarnContext(c.Request.Context(), "whatsapp webhook: invalid JSON", "error", err.Error())
		c.Status(http.StatusOK)
		return
	}

	// Para cada status entry, registra audit. Meta agrupa em entry[]/
	// changes[]/value.statuses[] — denormaliza para uma entry por status.
	statusCount := 0
	for _, entry := range payload.Entry {
		for _, change := range entry.Changes {
			if change.Field != "messages" {
				continue
			}
			for _, st := range change.Value.Statuses {
				statusCount++
				audit.Record(c.Request.Context(), audit.Entry{
					Action:     "whatsapp.delivery_status",
					EntityType: "WhatsAppMessage",
					EntityID:   st.ID, // wamid
					Metadata: map[string]any{
						"status":       st.Status,
						"timestamp":    st.Timestamp,
						"recipient_id": st.RecipientID,
						"conversation": st.Conversation,
						"errors":       st.Errors,
					},
				})
				// Próximo PR: lookup Charge.RemindersSent por wamid →
				// atualizar DeliveryStatus + DeliveredAt. Requer
				// persistir wamid no envio (refactor de ChargeService.
				// NotifyWithOptions). Por agora audit serve como trail.
			}
		}
	}

	logger.InfoContext(c.Request.Context(), "whatsapp webhook processed",
		"status_count", statusCount,
		"entry_count", len(payload.Entry),
	)
	c.Status(http.StatusOK)
}

// verifyMetaSignature compara o HMAC-SHA256(body, app_secret) com o
// header X-Hub-Signature-256 do request. Meta usa o prefixo "sha256=".
// Constant-time compare para evitar timing attacks.
func verifyMetaSignature(appSecret string, body []byte, header string) bool {
	if !strings.HasPrefix(header, "sha256=") {
		return false
	}
	expectedHex := strings.TrimPrefix(header, "sha256=")
	mac := hmac.New(sha256.New, []byte(appSecret))
	mac.Write(body)
	computed := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expectedHex), []byte(computed))
}

// metaWebhookPayload — shape do POST que Meta Cloud API envia para
// status updates. Documentação:
// https://developers.facebook.com/docs/whatsapp/cloud-api/webhooks/components#statuses-object
type metaWebhookPayload struct {
	Object string      `json:"object"`
	Entry  []metaEntry `json:"entry"`
}

type metaEntry struct {
	ID      string       `json:"id"`
	Changes []metaChange `json:"changes"`
}

type metaChange struct {
	Field string          `json:"field"`
	Value metaChangeValue `json:"value"`
}

type metaChangeValue struct {
	MessagingProduct string         `json:"messaging_product"`
	Metadata         map[string]any `json:"metadata"`
	Statuses         []metaStatus   `json:"statuses"`
}

type metaStatus struct {
	ID           string           `json:"id"`     // wamid
	Status       string           `json:"status"` // sent | delivered | read | failed
	Timestamp    string           `json:"timestamp"`
	RecipientID  string           `json:"recipient_id"`
	Conversation map[string]any   `json:"conversation,omitempty"`
	Errors       []map[string]any `json:"errors,omitempty"`
}
