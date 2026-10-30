package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CalendarLink assina o download de um compromisso para quem recebe o e-mail e
// não tem sessão. O token carrega só o tenant e a validade: o compromisso está
// no caminho e é revalidado (tenant, status e data) a cada download, sem
// confiar em nada que o cliente envie.
type CalendarLink struct {
	secret  []byte
	baseURL string
}

// NewCalendarLink monta o assinador. Sem segredo ou sem URL pública o
// assinador fica inerte: os e-mails saem sem o CTA em vez de sair com um link
// que não funciona.
func NewCalendarLink(secret, baseURL string) *CalendarLink {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.TrimSpace(secret) == "" || baseURL == "" {
		return &CalendarLink{}
	}
	return &CalendarLink{secret: []byte(secret), baseURL: baseURL}
}

func (l *CalendarLink) Enabled() bool { return l != nil && len(l.secret) > 0 && l.baseURL != "" }

// URL devolve o link assinado para o download do arquivo.
func (l *CalendarLink) URL(appointmentID, tenantID string, expires time.Time) string {
	if !l.Enabled() || appointmentID == "" || tenantID == "" {
		return ""
	}
	payload := tenantID + "|" + strconv.FormatInt(expires.UTC().Unix(), 10)
	token := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." +
		base64.RawURLEncoding.EncodeToString(l.sign(payload))
	return fmt.Sprintf("%s/api/v1/appointments/public/%s/calendar.ics?token=%s",
		l.baseURL, url.PathEscape(appointmentID), url.QueryEscape(token))
}

// Verify confere assinatura e validade, devolvendo o tenant do link.
func (l *CalendarLink) Verify(token string, now time.Time) (string, bool) {
	if !l.Enabled() {
		return "", false
	}
	payloadPart, signaturePart, found := strings.Cut(token, ".")
	if !found {
		return "", false
	}
	rawPayload, err := base64.RawURLEncoding.DecodeString(payloadPart)
	if err != nil {
		return "", false
	}
	signature, err := base64.RawURLEncoding.DecodeString(signaturePart)
	if err != nil {
		return "", false
	}
	if !hmac.Equal(signature, l.sign(string(rawPayload))) {
		return "", false
	}
	tenantID, expiry, found := strings.Cut(string(rawPayload), "|")
	if !found || tenantID == "" {
		return "", false
	}
	seconds, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		return "", false
	}
	if !now.UTC().Before(time.Unix(seconds, 0).UTC()) {
		return "", false
	}
	return tenantID, true
}

func (l *CalendarLink) sign(payload string) []byte {
	mac := hmac.New(sha256.New, l.secret)
	mac.Write([]byte(payload))
	return mac.Sum(nil)
}

// CalendarLinkTTL é o prazo do link do e-mail: o download continua possível por
// 30 dias depois do fim do compromisso, para quem só abrir o e-mail depois.
const CalendarLinkTTL = 30 * 24 * time.Hour
