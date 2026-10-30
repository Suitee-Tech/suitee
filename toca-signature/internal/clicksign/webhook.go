package clicksign

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// VerifyHMAC validates the Clicksign Content-Hmac header. The header arrives as
// "sha256=<hex>"; the digest is HMAC-SHA256(rawBody, webhookSecret).
//
// Returns nil on a valid signature, and a descriptive error otherwise. We
// intentionally accept a missing secret as an error rather than silently
// allowing the request through — the deployment is misconfigured if a webhook
// reaches this code path with no secret.
func VerifyHMAC(body []byte, headerVal, secret string) error {
	if secret == "" {
		return errors.New("webhook secret not configured")
	}
	if headerVal == "" {
		return errors.New("missing Content-Hmac header")
	}
	expected := strings.TrimPrefix(headerVal, "sha256=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	actual := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(expected), []byte(actual)) {
		return errors.New("invalid hmac signature")
	}
	return nil
}

// SignBody computes the Content-Hmac header value for a body. Used by tests
// (and could be reused by an outbound replay tool).
func SignBody(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
