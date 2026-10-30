// Package clients — SMSZenviaClient é o adapter direto para a API da
// Zenvia (https://zenvia.github.io/zenvia-openapi-spec/v2/). Issue #444.
//
// Por que Zenvia: provider brasileiro maduro, HTTP REST com auth por
// token (sem SDK pesado), supports Brazil SMS short codes nativamente.
// Mantém a mesma filosofia da escolha de WhatsApp Meta (#443/#528):
// adapter HTTP direto, sem dependência de SDK terceiro, retry interno
// para rate-limit.
//
// Configuração via env:
//   - SMS_ZENVIA_API_TOKEN   (obrigatório)
//   - SMS_ZENVIA_FROM        (obrigatório — sender ID/short-code
//                             aprovado pela Anatel pro tenant)
//   - SMS_ZENVIA_TIMEOUT_SECONDS (default 10)
//   - SMS_ZENVIA_RETRY_MAX_ATTEMPTS (default 3, igual ao WhatsApp)
//   - SMS_ZENVIA_RETRY_BASE_DELAY_MS (default 1000)
//
// Templates SMS são apenas string body — sem o conceito de "template
// aprovado" que existe no WhatsApp Business. Limite operacional de 160
// caracteres é validado no envio para evitar truncamento + cobrança
// dupla (mensagens > 160 chars viram 2 SMS no billing).

package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Typed errors. Mesmo padrão do WhatsApp client para handlers em
// camadas superiores usarem errors.Is.
var (
	// ErrSMSNotConfigured: env vars obrigatórios não setados. Sinal
	// para o caller fazer fallback silencioso.
	ErrSMSNotConfigured = errors.New("sms zenvia not configured (missing SMS_ZENVIA_API_TOKEN or SMS_ZENVIA_FROM)")

	// ErrSMSInvalidRecipient: número em formato inválido (não passou
	// na normalização E.164).
	ErrSMSInvalidRecipient = errors.New("invalid sms recipient phone")

	// ErrSMSBodyTooLong: body > 160 chars. SMS de 160+ vira 2 cobranças
	// ou é truncado pelo provider; fail-fast deixa o caller corrigir o
	// template antes de subir.
	ErrSMSBodyTooLong = errors.New("sms body exceeds 160 characters")

	// ErrSMSInvalidToken: Zenvia rejeitou o token. Rotar nas env vars.
	ErrSMSInvalidToken = errors.New("sms zenvia api token rejected")

	// ErrSMSRateLimited: Zenvia rate-limit. Retry interno tenta de
	// novo com backoff; se mesmo assim falhar, propaga.
	ErrSMSRateLimited = errors.New("sms zenvia rate limited")

	// ErrSMSAPIError: genérico não-categorizado.
	ErrSMSAPIError = errors.New("sms zenvia api error")
)

// SMSZenviaClient encapsula a comunicação com a API REST da Zenvia.
// Stateless — pode ser compartilhado entre goroutines.
type SMSZenviaClient struct {
	apiToken   string
	from       string
	baseURL    string
	httpClient *http.Client
	breaker    providerCircuitBreaker

	retryMaxAttempts int
	retryBaseDelay   time.Duration
}

// SMSZenviaConfig — params do construtor. Permite injection p/ tests.
type SMSZenviaConfig struct {
	APIToken         string
	From             string
	Timeout          time.Duration
	BaseURL          string // quando vazio, https://api.zenvia.com
	HTTPClient       *http.Client
	RetryMaxAttempts int           // default 3
	RetryBaseDelay   time.Duration // default 1s
}

// SMSSendResult resume o que o provider devolveu num envio bem-sucedido.
type SMSSendResult struct {
	MessageID  string // id Zenvia (correlação com webhook de delivery)
	Recipient  string // número normalizado E.164
	StatusCode int
}

// NewSMSZenviaClient — construtor explícito.
func NewSMSZenviaClient(cfg SMSZenviaConfig) (*SMSZenviaClient, error) {
	if strings.TrimSpace(cfg.APIToken) == "" || strings.TrimSpace(cfg.From) == "" {
		return nil, ErrSMSNotConfigured
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.zenvia.com"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else if httpClient.Timeout <= 0 {
		// Preserve the injected transport but never allow a provider call to
		// block indefinitely.
		cloned := *httpClient
		cloned.Timeout = timeout
		httpClient = &cloned
	}
	retryAttempts := cfg.RetryMaxAttempts
	if retryAttempts == 0 {
		retryAttempts = 3
	}
	retryBase := cfg.RetryBaseDelay
	if retryBase <= 0 {
		retryBase = time.Second
	}
	return &SMSZenviaClient{
		apiToken:         cfg.APIToken,
		from:             cfg.From,
		baseURL:          baseURL,
		httpClient:       httpClient,
		breaker:          newProviderCircuitBreaker("zenvia-sms"),
		retryMaxAttempts: retryAttempts,
		retryBaseDelay:   retryBase,
	}, nil
}

// NewSMSZenviaClientFromEnv lê env vars do ambiente. Padrão match com
// o WhatsApp client (#528): retorna (nil, ErrSMSNotConfigured) quando
// não configurado — sinal para caller seguir com Noop.
func NewSMSZenviaClientFromEnv() (*SMSZenviaClient, error) {
	timeoutSec := 10
	if v := os.Getenv("SMS_ZENVIA_TIMEOUT_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeoutSec = parsed
		}
	}
	retryAttempts := 3
	if v := os.Getenv("SMS_ZENVIA_RETRY_MAX_ATTEMPTS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			retryAttempts = parsed
		}
	}
	retryBaseMs := 1000
	if v := os.Getenv("SMS_ZENVIA_RETRY_BASE_DELAY_MS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			retryBaseMs = parsed
		}
	}
	return NewSMSZenviaClient(SMSZenviaConfig{
		APIToken:         os.Getenv("SMS_ZENVIA_API_TOKEN"),
		From:             os.Getenv("SMS_ZENVIA_FROM"),
		Timeout:          time.Duration(timeoutSec) * time.Second,
		RetryMaxAttempts: retryAttempts,
		RetryBaseDelay:   time.Duration(retryBaseMs) * time.Millisecond,
	})
}

// SendText envia um SMS de texto puro. Valida body length e normaliza
// o destinatário para E.164. Retry exponencial em rate-limit.
func (c *SMSZenviaClient) SendText(ctx context.Context, to, body string) (*SMSSendResult, error) {
	normalized, err := NormalizeBrazilianPhone(to)
	if err != nil {
		// Reusa o helper do WhatsApp client (mesmo pkg). Wrap em
		// ErrSMSInvalidRecipient pro caller diferenciar SMS de WhatsApp.
		return nil, fmt.Errorf("%w: %s", ErrSMSInvalidRecipient, err.Error())
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("%w: empty body", ErrSMSAPIError)
	}
	// Limite de 160 chars do SMS GSM-7 padrão. Acima disso o provider
	// fatura como múltiplos SMS — operacionalmente esse é o tipo de
	// bug que se nota no fim do mês. Fail-fast.
	if len([]rune(body)) > 160 {
		return nil, fmt.Errorf("%w: %d chars (max 160)", ErrSMSBodyTooLong, len([]rune(body)))
	}

	payload := map[string]any{
		"from": c.from,
		"to":   normalized,
		"contents": []map[string]any{
			{
				"type": "text",
				"text": body,
			},
		},
	}
	return executeProviderCall(c.breaker, isZenviaCircuitFailure, func() (*SMSSendResult, error) {
		return c.postMessage(ctx, payload, normalized)
	})
}

func isZenviaCircuitFailure(err error) bool {
	// Input problems are local to one message and must not block the provider
	// for all other recipients.
	return !errors.Is(err, ErrSMSInvalidRecipient) && !errors.Is(err, ErrSMSBodyTooLong)
}

// ─────────────────── internals ───────────────────

type zenviaSuccessResponse struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Direction string `json:"direction"`
}

type zenviaErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"details"`
}

func (c *SMSZenviaClient) postMessage(ctx context.Context, payload map[string]any, normalizedTo string) (*SMSSendResult, error) {
	attempts := c.retryMaxAttempts
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			delay := c.retryBaseDelay * time.Duration(1<<(i-1))
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		res, err := c.postMessageOnce(ctx, payload, normalizedTo)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if !errors.Is(err, ErrSMSRateLimited) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("rate-limit retry exhausted after %d attempts: %w", attempts, lastErr)
}

func (c *SMSZenviaClient) postMessageOnce(ctx context.Context, payload map[string]any, normalizedTo string) (*SMSSendResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %s", ErrSMSAPIError, err.Error())
	}
	endpoint := fmt.Sprintf("%s/v2/channels/sms/messages", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %s", ErrSMSAPIError, err.Error())
	}
	// Zenvia v2 usa X-API-TOKEN no header (não Bearer). Documentação:
	// https://zenvia.github.io/zenvia-openapi-spec/v2/
	req.Header.Set("X-API-TOKEN", c.apiToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: http: %s", ErrSMSAPIError, err.Error())
	}
	defer resp.Body.Close()
	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var success zenviaSuccessResponse
		if err := json.Unmarshal(respBytes, &success); err != nil {
			return nil, fmt.Errorf("%w: decode 2xx body: %s (raw=%s)", ErrSMSAPIError, err.Error(), truncate(string(respBytes), 200))
		}
		if success.ID == "" {
			return nil, fmt.Errorf("%w: zenvia 2xx response without id (raw=%s)", ErrSMSAPIError, truncate(string(respBytes), 200))
		}
		return &SMSSendResult{
			MessageID:  success.ID,
			Recipient:  normalizedTo,
			StatusCode: resp.StatusCode,
		}, nil
	}

	var errResp zenviaErrorResponse
	_ = json.Unmarshal(respBytes, &errResp)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("%w: status=%d code=%s message=%q", ErrSMSInvalidToken, resp.StatusCode, errResp.Code, errResp.Message)
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: code=%s message=%q", ErrSMSRateLimited, errResp.Code, errResp.Message)
	default:
		return nil, fmt.Errorf("%w: status=%d code=%s message=%q raw=%s",
			ErrSMSAPIError, resp.StatusCode, errResp.Code, errResp.Message, truncate(string(respBytes), 200))
	}
}
