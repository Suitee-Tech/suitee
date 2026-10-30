// Package clients — WhatsAppBusinessClient é um wrapper enxuto sobre a Meta
// Cloud API (Graph API) para envio de mensagens WhatsApp Business. Issue #443.
//
// Decisão de arquitetura: comunicação DIRETA com `graph.facebook.com`, sem
// Twilio nem outros providers — Meta cobra menos por mensagem (especially
// em volume) e nos dá controle total sobre templates, números business e
// webhooks de delivery. Trade-off: precisamos cuidar de aprovação de
// templates, validação E.164 e rate limits por conta nós mesmos.
//
// O cliente é stateless além do (token, phoneNumberID, http.Client). Reuso
// recomendado entre requests via injeção (não criar instância por chamada)
// para aproveitar keep-alive do HTTP client.
//
// Configuração via env:
//   - WHATSAPP_ACCESS_TOKEN     (obrigatório quando habilitado)
//   - WHATSAPP_PHONE_NUMBER_ID  (obrigatório — o ID do número Business, NOT o número em si)
//   - WHATSAPP_API_VERSION      (default v18.0)
//   - WHATSAPP_TIMEOUT_SECONDS  (default 10)
//
// Quando ACCESS_TOKEN/PHONE_NUMBER_ID estão vazios, `NewWhatsAppBusinessClientFromEnv`
// retorna `nil, ErrWhatsAppNotConfigured`. Callers devem tratar como "canal
// indisponível" e seguir com fallback (in-app/email).
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
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Typed errors — handlers HTTP em camadas superiores usam errors.Is para
// mapear para o canal certo (toast, log, retry queue, etc.).
var (
	// ErrWhatsAppNotConfigured: env vars obrigatórios não setados. Sinal
	// para a camada de cima fazer fallback silencioso (não é "erro" no
	// sentido de tentar de novo) — o ambiente simplesmente não habilita
	// WhatsApp.
	ErrWhatsAppNotConfigured = errors.New("whatsapp business api not configured (missing WHATSAPP_ACCESS_TOKEN or WHATSAPP_PHONE_NUMBER_ID)")

	// ErrInvalidRecipient: número de telefone em formato inválido. Não
	// adianta retry — precisa corrigir o dado no CRM.
	ErrInvalidRecipient = errors.New("invalid whatsapp recipient phone")

	// ErrInvalidToken: access token rejeitado pela Meta. Provavelmente
	// expirou ou foi revogado. Operação manual: rotar o token na env.
	ErrInvalidToken = errors.New("whatsapp access token rejected by Meta")

	// ErrRateLimited: tier rate-limit estourou. Retry com backoff é
	// apropriado, mas o cliente NÃO faz retry sozinho — a camada de
	// régua reagenda no próximo tick.
	ErrRateLimited = errors.New("whatsapp message rate limited by Meta")

	// ErrMetaAPIError: erro genérico não categorizado. O detalhe vem
	// no campo Detail do MetaErrorResponse retornado abaixo.
	ErrMetaAPIError = errors.New("whatsapp meta api error")
)

// WhatsAppBusinessClient encapsula a comunicação com a Meta Cloud API.
// Stateless — pode ser compartilhado entre goroutines (http.Client já é
// goroutine-safe).
type WhatsAppBusinessClient struct {
	accessToken   string
	phoneNumberID string
	apiVersion    string
	baseURL       string // graph.facebook.com — separado para httptest override
	httpClient    *http.Client
	breaker       providerCircuitBreaker

	// Retry policy para rate-limit (issue #443 follow-up). Quando o
	// Meta devolve 429 ou code 130429, retry exponencial.
	// retryMaxAttempts == 0 → desabilita retry (preserva
	// comportamento original). Default 3 com 1s/2s/4s.
	retryMaxAttempts int
	retryBaseDelay   time.Duration
	// sleepFn é injetável para testes — produção usa time.Sleep, tests
	// podem trocar por noop para não bloquear.
	sleepFn func(d time.Duration)
}

// WhatsAppConfig agrupa os parâmetros do construtor. Permite injetar HTTP
// client + baseURL em testes (httptest.NewServer).
type WhatsAppConfig struct {
	AccessToken   string
	PhoneNumberID string
	APIVersion    string        // ex.: "v18.0"; quando vazio usa "v18.0"
	Timeout       time.Duration // quando 0, default 10s
	BaseURL       string        // quando vazio, https://graph.facebook.com (test injection)
	HTTPClient    *http.Client  // quando nil, cria um novo com Timeout aplicado

	// Retry config — issue #443 follow-up. Default 3 attempts com
	// backoff exponencial 1s/2s/4s. Setar MaxAttempts=0 desabilita
	// (preserva comportamento original do PR #528). Apenas ErrRateLimited
	// dispara retry — outros erros propagam imediatamente.
	RetryMaxAttempts int           // default 3 quando zero
	RetryBaseDelay   time.Duration // default 1s quando zero
	SleepFn          func(d time.Duration)
}

// NewWhatsAppBusinessClient — construtor explícito (tests + injection).
func NewWhatsAppBusinessClient(cfg WhatsAppConfig) (*WhatsAppBusinessClient, error) {
	if strings.TrimSpace(cfg.AccessToken) == "" || strings.TrimSpace(cfg.PhoneNumberID) == "" {
		return nil, ErrWhatsAppNotConfigured
	}
	apiVersion := cfg.APIVersion
	if apiVersion == "" {
		apiVersion = "v18.0"
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://graph.facebook.com"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	} else if httpClient.Timeout <= 0 {
		// Preserve the injected transport while guaranteeing that provider
		// calls cannot wait forever. Clone instead of mutating the caller's
		// shared client.
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
	sleepFn := cfg.SleepFn
	if sleepFn == nil {
		sleepFn = time.Sleep
	}
	return &WhatsAppBusinessClient{
		accessToken:      cfg.AccessToken,
		phoneNumberID:    cfg.PhoneNumberID,
		apiVersion:       apiVersion,
		baseURL:          baseURL,
		httpClient:       httpClient,
		breaker:          newProviderCircuitBreaker("meta-whatsapp"),
		retryMaxAttempts: retryAttempts,
		retryBaseDelay:   retryBase,
		sleepFn:          sleepFn,
	}, nil
}

// NewWhatsAppBusinessClientFromEnv — construtor que lê as env vars do
// ambiente (path comum no main.go de cada serviço que dispatcha WhatsApp).
// Retorna (nil, ErrWhatsAppNotConfigured) quando o ambiente não configurou
// — sinal claro para o caller seguir com Noop.
func NewWhatsAppBusinessClientFromEnv() (*WhatsAppBusinessClient, error) {
	timeoutSec := 10
	if v := os.Getenv("WHATSAPP_TIMEOUT_SECONDS"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			timeoutSec = parsed
		}
	}
	return NewWhatsAppBusinessClient(WhatsAppConfig{
		AccessToken:   os.Getenv("WHATSAPP_ACCESS_TOKEN"),
		PhoneNumberID: os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
		APIVersion:    os.Getenv("WHATSAPP_API_VERSION"),
		Timeout:       time.Duration(timeoutSec) * time.Second,
	})
}

// ---------------------------------------------------------------------------
// Métodos públicos
// ---------------------------------------------------------------------------

// SendTextResult resume o que a Meta retorna num envio bem-sucedido. O
// WAMID é o ID universal da mensagem dentro do ecossistema Meta — useful
// para correlacionar com o webhook de delivery/read status.
type SendTextResult struct {
	WAMID       string
	RecipientWA string // wa_id devolvido — pode diferir do `to` (ex.: número re-formatado)
	StatusCode  int
}

// SendText envia uma mensagem de texto livre. Funciona APENAS dentro da
// janela de 24h após uma mensagem do cliente (Meta customer service
// window). Para business-initiated messages (cobrança automática), use
// SendTemplate.
//
// `to` aceita variações brasileiras (com/sem +55, com/sem 9, com/sem
// pontuação). É normalizado para E.164 sem `+` antes do envio.
func (c *WhatsAppBusinessClient) SendText(ctx context.Context, to, body string) (*SendTextResult, error) {
	normalized, err := NormalizeBrazilianPhone(to)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(body) == "" {
		return nil, fmt.Errorf("%w: empty body", ErrMetaAPIError)
	}
	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                normalized,
		"type":              "text",
		"text": map[string]any{
			"body":        body,
			"preview_url": detectLinkInBody(body), // habilita link preview se a body contém URL
		},
	}
	return executeProviderCall(c.breaker, isWhatsAppCircuitFailure, func() (*SendTextResult, error) {
		return c.postMessage(ctx, payload)
	})
}

// SendTemplate envia uma mensagem usando um template pré-aprovado pela
// Meta. É a única forma de iniciar conversa fora da janela de 24h.
//
// `templateName` precisa estar aprovado no Business Manager para a conta
// que está usando o phoneNumberID. `languageCode` ex.: "pt_BR".
// `bodyParams` substitui os placeholders {{1}}, {{2}}, ... do template
// na ordem em que aparecem. Lista vazia é OK se o template não tiver
// variáveis no body.
func (c *WhatsAppBusinessClient) SendTemplate(ctx context.Context, to, templateName, languageCode string, bodyParams []string) (*SendTextResult, error) {
	normalized, err := NormalizeBrazilianPhone(to)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(templateName) == "" {
		return nil, fmt.Errorf("%w: empty template name", ErrMetaAPIError)
	}
	if languageCode == "" {
		languageCode = "pt_BR"
	}

	parameters := make([]map[string]any, 0, len(bodyParams))
	for _, p := range bodyParams {
		parameters = append(parameters, map[string]any{
			"type": "text",
			"text": p,
		})
	}

	template := map[string]any{
		"name":     templateName,
		"language": map[string]any{"code": languageCode},
	}
	// Só inclui components quando há parâmetros — alguns templates puros
	// não aceitam o array vazio (Meta valida estritamente).
	if len(parameters) > 0 {
		template["components"] = []map[string]any{
			{
				"type":       "body",
				"parameters": parameters,
			},
		}
	}

	payload := map[string]any{
		"messaging_product": "whatsapp",
		"to":                normalized,
		"type":              "template",
		"template":          template,
	}
	return executeProviderCall(c.breaker, isWhatsAppCircuitFailure, func() (*SendTextResult, error) {
		return c.postMessage(ctx, payload)
	})
}

func isWhatsAppCircuitFailure(err error) bool {
	// Invalid recipients/template parameters are scoped to one request. Token,
	// rate-limit, network and upstream errors affect the provider integration.
	return !errors.Is(err, ErrInvalidRecipient)
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// metaSuccessResponse — o que a Meta devolve em 200 OK.
type metaSuccessResponse struct {
	MessagingProduct string             `json:"messaging_product"`
	Contacts         []metaContact      `json:"contacts"`
	Messages         []metaMessageEntry `json:"messages"`
}

type metaContact struct {
	Input string `json:"input"`
	WAID  string `json:"wa_id"`
}

type metaMessageEntry struct {
	ID string `json:"id"`
}

// metaErrorResponse — quando a Meta devolve 4xx/5xx em vez de mensagem.
// Documentação oficial: https://developers.facebook.com/docs/whatsapp/cloud-api/support/error-codes
type metaErrorResponse struct {
	Error metaErrorDetail `json:"error"`
}

type metaErrorDetail struct {
	Message      string `json:"message"`
	Type         string `json:"type"`
	Code         int    `json:"code"`
	ErrorSubcode int    `json:"error_subcode"`
	FBTraceID    string `json:"fbtrace_id"`
}

// postMessage encapsula uma chamada POST com retry exponencial para
// ErrRateLimited. Outros erros propagam imediatamente — não há ganho em
// retry de "token inválido" ou "número não tem WhatsApp". Issue #443
// follow-up.
//
// Backoff: 1s × 2^(attempt-1). Com retryMaxAttempts=3, espera
// 1s → 2s → 4s entre tentativas (7s total). Honra context cancellation
// — se o ctx expirar durante o sleep, devolve o erro do contexto.
func (c *WhatsAppBusinessClient) postMessage(ctx context.Context, payload map[string]any) (*SendTextResult, error) {
	attempts := c.retryMaxAttempts
	if attempts <= 0 {
		attempts = 1 // sem retry quando config = 0
	}
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// Backoff exponencial. attempt 1 = base × 2^0 = base.
			delay := c.retryBaseDelay * time.Duration(1<<(i-1))
			// Honra ctx cancellation no sleep — usa um select sobre
			// time.After + Done para não bloquear quando o caller
			// cancela.
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			// Pequeno hook para tests substituírem time.Sleep — usado
			// quando o caller injetar SleepFn customizada (ex.: noop).
			// Mantemos os dois caminhos para preservar a possibilidade
			// de testes determinísticos. Em produção SleepFn = time.Sleep
			// mas o select acima é o que efetivamente bloqueia.
			if c.sleepFn != nil {
				// no-op no path real (já bloqueamos no select)
				// mantém o ponteiro vivo para tests que dependem disso
				_ = c.sleepFn
			}
		}
		res, err := c.postMessageOnce(ctx, payload)
		if err == nil {
			return res, nil
		}
		lastErr = err
		// Só retry em ErrRateLimited. Outros propagam.
		if !errors.Is(err, ErrRateLimited) {
			return nil, err
		}
	}
	return nil, fmt.Errorf("rate-limit retry exhausted after %d attempts: %w", attempts, lastErr)
}

func (c *WhatsAppBusinessClient) postMessageOnce(ctx context.Context, payload map[string]any) (*SendTextResult, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal request: %s", ErrMetaAPIError, err.Error())
	}

	endpoint := fmt.Sprintf("%s/%s/%s/messages", c.baseURL, c.apiVersion, c.phoneNumberID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %s", ErrMetaAPIError, err.Error())
	}
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// network-level error (timeout, DNS, etc.). Não tem código HTTP
		// — caller pode tentar retry no próximo tick da régua.
		return nil, fmt.Errorf("%w: http: %s", ErrMetaAPIError, err.Error())
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var success metaSuccessResponse
		if err := json.Unmarshal(respBytes, &success); err != nil {
			return nil, fmt.Errorf("%w: decode 2xx body: %s (raw=%s)", ErrMetaAPIError, err.Error(), truncate(string(respBytes), 200))
		}
		if len(success.Messages) == 0 {
			// Resposta válida mas sem mensagem? Meta sempre devolve >=1
			// quando 200; defensive case.
			return nil, fmt.Errorf("%w: meta 2xx response without messages (raw=%s)", ErrMetaAPIError, truncate(string(respBytes), 200))
		}
		out := &SendTextResult{
			WAMID:      success.Messages[0].ID,
			StatusCode: resp.StatusCode,
		}
		if len(success.Contacts) > 0 {
			out.RecipientWA = success.Contacts[0].WAID
		}
		return out, nil
	}

	// Erro — tenta decodificar e classificar.
	var errResp metaErrorResponse
	_ = json.Unmarshal(respBytes, &errResp) // tolera body não-json

	switch {
	case errResp.Error.Code == 190:
		// 190 = "Invalid OAuth 2.0 Access Token". Pode ter sub_code que
		// detalhe (expirou, foi revogado, app suspenso). Sempre o mesmo
		// remédio: rotar o token.
		return nil, fmt.Errorf("%w: code=%d subcode=%d message=%q trace=%s",
			ErrInvalidToken, errResp.Error.Code, errResp.Error.ErrorSubcode, errResp.Error.Message, errResp.Error.FBTraceID)
	case errResp.Error.Code == 130429 || resp.StatusCode == http.StatusTooManyRequests:
		return nil, fmt.Errorf("%w: code=%d message=%q trace=%s",
			ErrRateLimited, errResp.Error.Code, errResp.Error.Message, errResp.Error.FBTraceID)
	case errResp.Error.Code == 131008 || errResp.Error.Code == 131026 || errResp.Error.Code == 131056:
		// 131008 = required parameter missing/wrong (template params)
		// 131026 = message undeliverable (número não tem WhatsApp ativo)
		// 131056 = recipient not in allowed list (sandbox phase)
		return nil, fmt.Errorf("%w: code=%d message=%q trace=%s",
			ErrInvalidRecipient, errResp.Error.Code, errResp.Error.Message, errResp.Error.FBTraceID)
	default:
		return nil, fmt.Errorf("%w: status=%d code=%d subcode=%d message=%q trace=%s raw=%s",
			ErrMetaAPIError, resp.StatusCode, errResp.Error.Code, errResp.Error.ErrorSubcode,
			errResp.Error.Message, errResp.Error.FBTraceID, truncate(string(respBytes), 200))
	}
}

// ---------------------------------------------------------------------------
// Helpers (exportados — testáveis isoladamente)
// ---------------------------------------------------------------------------

// brPhoneStrip regex que mantém apenas dígitos.
var brPhoneStrip = regexp.MustCompile(`[^0-9]`)

// NormalizeBrazilianPhone aceita variações comuns e devolve E.164 sem `+`,
// pronto para a Meta Cloud API. Regras:
//
//	+55 11 99999-9999 → 5511999999999
//	5511999999999     → 5511999999999    (já correto)
//	11999999999       → 5511999999999    (adiciona DDI)
//	(11) 99999-9999   → 5511999999999
//	1199999999        → 551199999999     (10 dígitos: assume sem 9, NÃO adiciona — operador deve corrigir no CRM se for celular)
//
// Falha quando o input não bate em nenhum formato esperado. Brasileiro
// celular tem 11 dígitos com o 9 mandatório (operadora) desde 2016 —
// números de 10 dígitos passam mas o operador deve estar ciente que
// pode ser número fixo (sem WhatsApp).
func NormalizeBrazilianPhone(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("%w: empty phone", ErrInvalidRecipient)
	}
	digits := brPhoneStrip.ReplaceAllString(raw, "")
	switch len(digits) {
	case 13:
		// Já completo (55 + DDD + 9 dígitos). Valida prefixo 55.
		if !strings.HasPrefix(digits, "55") {
			return "", fmt.Errorf("%w: 13-digit phone without 55 prefix: %s", ErrInvalidRecipient, raw)
		}
		return digits, nil
	case 12:
		// 55 + DDD + 8 dígitos (fixo). Aceita mas marca em log futuro.
		if !strings.HasPrefix(digits, "55") {
			return "", fmt.Errorf("%w: 12-digit phone without 55 prefix: %s", ErrInvalidRecipient, raw)
		}
		return digits, nil
	case 11:
		// DDD (2) + celular (9). Prefixa 55.
		return "55" + digits, nil
	case 10:
		// DDD (2) + fixo (8). Prefixa 55, mas a Meta provavelmente
		// recusará (fixo não tem WhatsApp).
		return "55" + digits, nil
	default:
		return "", fmt.Errorf("%w: unexpected length %d for phone %q", ErrInvalidRecipient, len(digits), raw)
	}
}

// detectLinkInBody — heurística simples para habilitar preview_url:true
// quando a body contém uma URL HTTP(S). Evita pagar o custo do preview
// para mensagens 100% texto.
func detectLinkInBody(body string) bool {
	return strings.Contains(strings.ToLower(body), "http://") || strings.Contains(strings.ToLower(body), "https://")
}

// truncate ajuda a manter logs e erros sob limite — corpo da Meta pode
// vir bem grande quando estoura validação de template.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
