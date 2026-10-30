package clicksign

import (
	"os"
	"strings"
)

// WebhookPath is the canonical path the Clicksign webhook handler is mounted
// at on both toca-signature and the gateway. Keep them in sync if one moves.
const WebhookPath = "/webhooks/clicksign"

// PublicWebhookURL returns the URL the operator should register in the
// Clicksign dashboard, derived from env config in this order:
//
//  1. WEBHOOK_PUBLIC_URL — explicit override, used when the public URL
//     differs from the gateway (rare; e.g. CDN in front of the gateway).
//  2. PUBLIC_GATEWAY_URL — the canonical "where the gateway is reachable
//     from outside" var the rest of the platform already uses.
//  3. Empty string — caller decides whether to fall back to a default or
//     surface a "not configured" warning.
//
// In dev with ngrok, set PUBLIC_GATEWAY_URL=https://abc.ngrok-free.app and
// this function returns https://abc.ngrok-free.app/webhooks/clicksign with
// no other config needed.
func PublicWebhookURL() string {
	override := strings.TrimSpace(os.Getenv("WEBHOOK_PUBLIC_URL"))
	if override != "" {
		return joinPath(override, WebhookPath)
	}
	base := strings.TrimSpace(os.Getenv("PUBLIC_GATEWAY_URL"))
	if base == "" {
		return ""
	}
	return joinPath(base, WebhookPath)
}

func joinPath(base, path string) string {
	return strings.TrimRight(base, "/") + path
}
