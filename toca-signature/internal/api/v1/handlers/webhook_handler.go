package handlers

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
	"github.com/oginoino/toca/toca-signature/internal/service"
)

// WebhookHandler is the public endpoint Clicksign POSTs to. It performs three
// gates before invoking the WebhookService:
//  1. Source IP allowlist (configurable, empty = disabled — useful for ngrok in dev).
//  2. HMAC SHA256 of the raw body against the webhook secret.
//  3. JSON parse — malformed bodies get 400 (Clicksign won't retry on 4xx).
//
// Everything from there returns 200, even if internal processing failed: a
// 5xx would push Clicksign to retry, and we already persist the raw body for
// later replay.
type WebhookHandler struct {
	svc            *service.WebhookService
	creds          credential.CredentialProvider
	allowedEntries []allowedEntry
	rawAllowed     []string
}

// allowedEntry holds a parsed allowlist item — either an exact IP or a CIDR
// network. Parsed once at construction so the per-request check is cheap.
type allowedEntry struct {
	ip      net.IP
	network *net.IPNet
	raw     string
}

func NewWebhookHandler(svc *service.WebhookService, creds credential.CredentialProvider, allowedIPs []string) *WebhookHandler {
	trimmed := trimAll(allowedIPs)
	return &WebhookHandler{
		svc:            svc,
		creds:          creds,
		allowedEntries: parseAllowedEntries(trimmed),
		rawAllowed:     trimmed,
	}
}

// Receive godoc
// @Summary      Receive Clicksign webhook
// @Description  Public webhook receiver for Clicksign envelope events. Authenticated via HMAC signature in Content-Hmac header and optional IP allowlist (CLICKSIGN_WEBHOOK_ALLOWED_IPS).
// @Tags         webhooks
// @Accept       json
// @Produce      plain
// @Param        Content-Hmac  header  string  true  "HMAC-SHA256 of the body, formatted as sha256=<hex>"
// @Param        X-Delivery-Id header  string  false "Clicksign delivery id (for idempotency)"
// @Success      200  {string}  string  "OK"
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Router       /webhooks/clicksign [post]
func (h *WebhookHandler) Receive(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		response.ValidationError(c, "failed to read body")
		return
	}

	// Log toda chegada antes de qualquer gate. Sem isso, quando Clicksign não
	// atinge o handler (URL errada no painel, IP allowlist, gateway), o
	// silêncio nos logs é indistinguível de webhook nunca enviado.
	log.Printf("clicksign webhook received ip=%s bytes=%d hmac_present=%t delivery_id=%q xff=%q",
		c.ClientIP(), len(body), c.GetHeader("Content-Hmac") != "",
		c.GetHeader("X-Delivery-Id"), c.GetHeader("X-Forwarded-For"))

	if !h.ipAllowed(c.ClientIP()) {
		// Log RemoteAddr separately from ClientIP — when they differ, the
		// real culprit is usually a proxy hop in front of the gateway that
		// rewrote X-Forwarded-For. ClientIP comes from Gin's XFF parsing;
		// RemoteAddr is the immediate TCP peer (typically the gateway).
		log.Printf("clicksign webhook rejected: ip %s not in allowlist (allowed=%v remote_addr=%s xff=%q)",
			c.ClientIP(), h.rawAllowed, c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For"))
		response.Forbidden(c, "source ip not allowed")
		return
	}

	cred, err := h.creds.Get(c.Request.Context(), "")
	if err != nil {
		// Misconfiguration — fail loudly.
		log.Printf("clicksign webhook: credential lookup failed: %v", err)
		response.InternalServerError(c, "webhook secret unavailable")
		return
	}
	if err := clicksign.VerifyHMAC(body, c.GetHeader("Content-Hmac"), cred.WebhookSecret); err != nil {
		log.Printf("clicksign webhook hmac rejected: %v (ip=%s)", err, c.ClientIP())
		response.Unauthorized(c, "invalid signature")
		return
	}

	var payload clicksign.WebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("clicksign webhook body parse failed: %v", err)
		response.ValidationError(c, "invalid payload")
		return
	}

	deliveryID := c.GetHeader("X-Delivery-Id")
	sigHeader := c.GetHeader("Content-Hmac")
	if err := h.svc.Process(c.Request.Context(), payload, body, deliveryID, c.ClientIP(), sigHeader); err != nil {
		// Log but ack with 200 so Clicksign doesn't retry — the row is in
		// signature.webhook_events with process_error populated for replay.
		log.Printf("clicksign webhook process error (envelope=%s event=%s): %v",
			payloadEnvelopeID(payload), payload.Event.Name, err)
	}
	c.Status(http.StatusOK)
}

func (h *WebhookHandler) ipAllowed(ip string) bool {
	if len(h.allowedEntries) == 0 {
		return true
	}
	parsed := net.ParseIP(ip)
	for _, entry := range h.allowedEntries {
		if entry.network != nil {
			if parsed != nil && entry.network.Contains(parsed) {
				return true
			}
			continue
		}
		if entry.ip != nil && parsed != nil && entry.ip.Equal(parsed) {
			return true
		}
		// Fallback string match for cases where the IP didn't parse (rare —
		// keeps behavior compatible with the previous exact-string check).
		if entry.raw == ip {
			return true
		}
	}
	return false
}

// parseAllowedEntries pre-parses the raw allowlist into either CIDR networks
// or exact IPs. Invalid entries are kept as raw-string fallbacks so a typo in
// config never silently disables a previously-working entry.
func parseAllowedEntries(items []string) []allowedEntry {
	out := make([]allowedEntry, 0, len(items))
	for _, raw := range items {
		entry := allowedEntry{raw: raw}
		if strings.Contains(raw, "/") {
			if _, ipNet, err := net.ParseCIDR(raw); err == nil {
				entry.network = ipNet
			}
		} else {
			entry.ip = net.ParseIP(raw)
		}
		out = append(out, entry)
	}
	return out
}

func payloadEnvelopeID(p clicksign.WebhookPayload) string {
	if p.Envelope != nil {
		return p.Envelope.ID
	}
	return "unknown"
}

func trimAll(ss []string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}
