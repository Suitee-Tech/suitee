package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker"
)

// newTestClient devolve um WhatsAppBusinessClient apontando para o
// httptest.Server passado, com token + phoneNumberID fixos. Mantém o setup
// idêntico entre testes para destacar só o que muda.
//
// Issue #443 follow-up: retry desabilitado por default em testes (configura
// RetryMaxAttempts=1) — tests verificam comportamento sem rate-limit delay
// arrastando o run. Tests específicos de retry usam newTestClientWithRetry.
func newTestClient(t *testing.T, server *httptest.Server) *WhatsAppBusinessClient {
	t.Helper()
	c, err := NewWhatsAppBusinessClient(WhatsAppConfig{
		AccessToken:      "TEST_TOKEN",
		PhoneNumberID:    "999999",
		APIVersion:       "v18.0",
		BaseURL:          server.URL,
		RetryMaxAttempts: 1, // sem retry — propaga erro imediato
	})
	if err != nil {
		t.Fatalf("NewWhatsAppBusinessClient: %v", err)
	}
	return c
}

// newTestClientWithRetry permite tests verificarem o backoff
// exponencial sem bloquear de verdade — SleepFn é noop (mas o select
// time.After no postMessage ainda usa relógio real; tests usam delay
// micro pra manter o run rápido).
func newTestClientWithRetry(t *testing.T, server *httptest.Server, attempts int) *WhatsAppBusinessClient {
	t.Helper()
	c, err := NewWhatsAppBusinessClient(WhatsAppConfig{
		AccessToken:      "TEST_TOKEN",
		PhoneNumberID:    "999999",
		APIVersion:       "v18.0",
		BaseURL:          server.URL,
		RetryMaxAttempts: attempts,
		RetryBaseDelay:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewWhatsAppBusinessClient: %v", err)
	}
	return c
}

func TestNewWhatsAppBusinessClient_RequiresCredentials(t *testing.T) {
	cases := []WhatsAppConfig{
		{},
		{AccessToken: "token"},                   // missing phoneNumberID
		{PhoneNumberID: "1234"},                  // missing accessToken
		{AccessToken: "  ", PhoneNumberID: "  "}, // whitespace counts as empty
	}
	for i, cfg := range cases {
		_, err := NewWhatsAppBusinessClient(cfg)
		if !errors.Is(err, ErrWhatsAppNotConfigured) {
			t.Errorf("case %d: expected ErrWhatsAppNotConfigured, got %v", i, err)
		}
	}
}

func TestSendText_Success(t *testing.T) {
	var receivedBody []byte
	var receivedAuth string
	var receivedPath string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("Authorization")
		// Read body for assertions
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		receivedBody = buf
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, `{
			"messaging_product": "whatsapp",
			"contacts": [{"input": "5511999999999", "wa_id": "5511999999999"}],
			"messages": [{"id": "wamid.HBgNNTUx..."}]
		}`)
	}))
	defer server.Close()

	c := newTestClient(t, server)
	res, err := c.SendText(context.Background(), "+55 11 99999-9999", "Olá!")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if res.WAMID != "wamid.HBgNNTUx..." {
		t.Errorf("expected WAMID, got %q", res.WAMID)
	}
	if res.RecipientWA != "5511999999999" {
		t.Errorf("expected RecipientWA 5511999999999, got %q", res.RecipientWA)
	}

	// Assertions sobre a request que saiu — corre risco de quebra se a
	// API da Meta mudar, mas é o tipo de coisa que queremos saber.
	if receivedPath != "/v18.0/999999/messages" {
		t.Errorf("unexpected request path: %s", receivedPath)
	}
	if receivedAuth != "Bearer TEST_TOKEN" {
		t.Errorf("unexpected auth header: %s", receivedAuth)
	}
	var payload map[string]any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["messaging_product"] != "whatsapp" {
		t.Errorf("missing messaging_product")
	}
	if payload["to"] != "5511999999999" {
		t.Errorf("phone wasn't normalized: %v", payload["to"])
	}
	if payload["type"] != "text" {
		t.Errorf("expected type=text, got %v", payload["type"])
	}
}

func TestSendText_DetectsLinkPreview(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.X"}]}`)
	}))
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.SendText(context.Background(), "5511999999999", "Pague em https://pay.example.com/x")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	text, ok := received["text"].(map[string]any)
	if !ok {
		t.Fatalf("text block missing: %+v", received)
	}
	if text["preview_url"] != true {
		t.Errorf("expected preview_url=true when body contains URL, got %v", text["preview_url"])
	}

	// Caso sem link: preview_url deve ser false.
	received = nil
	_, err = c.SendText(context.Background(), "5511999999999", "Sem URL")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	text, _ = received["text"].(map[string]any)
	if text["preview_url"] != false {
		t.Errorf("expected preview_url=false for plain body, got %v", text["preview_url"])
	}
}

func TestSendTemplate_BuildsPayload(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.T"}]}`)
	}))
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.SendTemplate(context.Background(), "5511999999999", "cobranca_aluguel", "pt_BR", []string{"João", "R$ 1.500,00", "10/06/2026"})
	if err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}

	if received["type"] != "template" {
		t.Errorf("expected type=template")
	}
	template, _ := received["template"].(map[string]any)
	if template["name"] != "cobranca_aluguel" {
		t.Errorf("expected name=cobranca_aluguel, got %v", template["name"])
	}
	lang, _ := template["language"].(map[string]any)
	if lang["code"] != "pt_BR" {
		t.Errorf("expected language pt_BR, got %v", lang["code"])
	}
	components, _ := template["components"].([]any)
	if len(components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(components))
	}
	body := components[0].(map[string]any)
	if body["type"] != "body" {
		t.Errorf("expected component type=body, got %v", body["type"])
	}
	params, _ := body["parameters"].([]any)
	if len(params) != 3 {
		t.Fatalf("expected 3 params, got %d", len(params))
	}
	first := params[0].(map[string]any)
	if first["text"] != "João" {
		t.Errorf("first param text mismatch: %v", first["text"])
	}
}

func TestSendTemplate_NoParametersOmitsComponents(t *testing.T) {
	// Template sem variáveis: Meta valida estritamente e recusa array
	// vazio de components. Garantia de que o cliente omite o campo.
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&received)
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.N"}]}`)
	}))
	defer server.Close()
	c := newTestClient(t, server)

	_, err := c.SendTemplate(context.Background(), "5511999999999", "hello_world", "en_US", nil)
	if err != nil {
		t.Fatalf("SendTemplate: %v", err)
	}
	template, _ := received["template"].(map[string]any)
	if _, has := template["components"]; has {
		t.Errorf("expected components to be omitted when no params, got: %v", template["components"])
	}
}

func TestSendText_MapsMetaErrorCodes(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		expectedAs error
	}{
		{
			name:   "invalid token (190)",
			status: http.StatusUnauthorized,
			body: `{"error":{"message":"Invalid OAuth 2.0 Access Token","type":"OAuthException",
                  "code":190,"error_subcode":463,"fbtrace_id":"AaBbCc"}}`,
			expectedAs: ErrInvalidToken,
		},
		{
			name:   "rate limited via code 130429",
			status: http.StatusBadRequest,
			body: `{"error":{"message":"Rate limit hit","type":"RateLimitException",
                  "code":130429,"fbtrace_id":"R1"}}`,
			expectedAs: ErrRateLimited,
		},
		{
			name:       "rate limited via HTTP 429 + no recognized code",
			status:     http.StatusTooManyRequests,
			body:       `{"error":{"message":"slow down","code":99999}}`,
			expectedAs: ErrRateLimited,
		},
		{
			name:   "template params mismatch (131008)",
			status: http.StatusBadRequest,
			body: `{"error":{"message":"Required parameter missing","type":"OAuthException",
                  "code":131008,"fbtrace_id":"P1"}}`,
			expectedAs: ErrInvalidRecipient,
		},
		{
			name:   "recipient not in allowed list (131056)",
			status: http.StatusBadRequest,
			body: `{"error":{"message":"recipient not allowed","type":"OAuthException",
                  "code":131056,"fbtrace_id":"S1"}}`,
			expectedAs: ErrInvalidRecipient,
		},
		{
			name:       "generic unknown 5xx",
			status:     http.StatusInternalServerError,
			body:       `{"error":{"message":"internal","code":1,"fbtrace_id":"G1"}}`,
			expectedAs: ErrMetaAPIError,
		},
		{
			name:       "non-JSON body still surfaces ErrMetaAPIError",
			status:     http.StatusBadGateway,
			body:       "Bad Gateway",
			expectedAs: ErrMetaAPIError,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintln(w, tc.body)
			}))
			defer server.Close()
			c := newTestClient(t, server)
			_, err := c.SendText(context.Background(), "5511999999999", "msg")
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, tc.expectedAs) {
				t.Errorf("expected errors.Is(%v); got %v", tc.expectedAs, err)
			}
		})
	}
}

func TestSendText_NetworkErrorWrapped(t *testing.T) {
	// Servidor que cai imediatamente — força erro de rede no Do().
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close() // mata antes de qualquer call
	c := newTestClient(t, server)
	_, err := c.SendText(context.Background(), "5511999999999", "msg")
	if !errors.Is(err, ErrMetaAPIError) {
		t.Errorf("expected ErrMetaAPIError on network failure, got %v", err)
	}
}

func TestSendText_EmptyBodyRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for empty body")
	}))
	defer server.Close()
	c := newTestClient(t, server)
	_, err := c.SendText(context.Background(), "5511999999999", "   ")
	if !errors.Is(err, ErrMetaAPIError) {
		t.Errorf("expected ErrMetaAPIError for empty body, got %v", err)
	}
}

func TestNormalizeBrazilianPhone(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"E.164 com +", "+5511999999999", "5511999999999", false},
		{"E.164 sem +", "5511999999999", "5511999999999", false},
		{"sem DDI (11 dígitos)", "11999999999", "5511999999999", false},
		{"sem DDI com 9 (10 dígitos = fixo, adiciona 55)", "1199999999", "551199999999", false},
		{"com pontuação", "(11) 99999-9999", "5511999999999", false},
		{"com hifens e espaços", "+55 11 99999 9999", "5511999999999", false},
		{"vazio", "", "", true},
		{"só whitespace", "   ", "", true},
		{"13 dígitos sem prefixo 55", "9911999999999", "", true},
		{"comprimento esquisito", "55119", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeBrazilianPhone(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %q", got)
				}
				if !errors.Is(err, ErrInvalidRecipient) && err != nil {
					t.Errorf("expected ErrInvalidRecipient wrapping, got %v", err)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("normalize(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSendText_InvalidPhoneShortcircuit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for invalid phone")
	}))
	defer server.Close()
	c := newTestClient(t, server)
	_, err := c.SendText(context.Background(), "", "msg")
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Errorf("expected ErrInvalidRecipient for empty phone, got %v", err)
	}
}

func TestTruncateHelper(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Errorf("short input unchanged, got %q", got)
	}
	got := truncate(strings.Repeat("x", 50), 10)
	if !strings.HasSuffix(got, "...(truncated)") || len(got) > 30 {
		t.Errorf("truncate not applied: %q", got)
	}
}

// ──────────────────────── Rate-limit retry (#443) ────────────────────────

// TestSendText_RetryOnRateLimit_Succeeds: primeira chamada devolve
// rate-limit, retry chama de novo e o servidor responde 200. Issue #443
// follow-up.
func TestSendText_RetryOnRateLimit_Succeeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprintln(w, `{"error":{"message":"slow","code":99999}}`)
			return
		}
		// segunda tentativa: 200 OK.
		_, _ = fmt.Fprintln(w, `{"messages":[{"id":"wamid.RETRY"}]}`)
	}))
	defer server.Close()

	c := newTestClientWithRetry(t, server, 3)
	res, err := c.SendText(context.Background(), "5511999999999", "msg")
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if res.WAMID != "wamid.RETRY" {
		t.Errorf("expected wamid from second attempt, got %q", res.WAMID)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 server calls (1 fail + 1 retry), got %d", calls.Load())
	}
}

// TestSendText_RetryExhaustion: todas as N tentativas falham com
// rate-limit; cliente devolve erro embrulhando ErrRateLimited com a
// contagem de tentativas.
func TestSendText_RetryExhaustion(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprintln(w, `{"error":{"code":130429}}`)
	}))
	defer server.Close()

	c := newTestClientWithRetry(t, server, 3)
	_, err := c.SendText(context.Background(), "5511999999999", "msg")
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected wrapping ErrRateLimited, got %v", err)
	}
	if !strings.Contains(err.Error(), "retry exhausted") {
		t.Errorf("expected 'retry exhausted' in error, got %q", err.Error())
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", calls.Load())
	}
}

// TestSendText_RetryDisabledByConfig: RetryMaxAttempts=1 = comportamento
// original (sem retry); um rate-limit devolve imediato.
func TestSendText_RetryDisabledByConfig(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprintln(w, `{"error":{"code":130429}}`)
	}))
	defer server.Close()

	c := newTestClientWithRetry(t, server, 1)
	_, err := c.SendText(context.Background(), "5511999999999", "msg")
	if !errors.Is(err, ErrRateLimited) {
		t.Errorf("expected ErrRateLimited, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("expected 1 attempt (no retry), got %d", calls.Load())
	}
}

// TestSendText_RetryDoesNotApplyToOtherErrors: token inválido (190) é
// erro permanente — não deve disparar retry.
func TestSendText_RetryDoesNotApplyToOtherErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, `{"error":{"code":190}}`)
	}))
	defer server.Close()

	c := newTestClientWithRetry(t, server, 3)
	_, err := c.SendText(context.Background(), "5511999999999", "msg")
	if !errors.Is(err, ErrInvalidToken) {
		t.Errorf("expected ErrInvalidToken, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("token errors should NOT retry; expected 1 attempt, got %d", calls.Load())
	}
}

// TestSendText_RetryHonorsContextCancellation: ctx canceled durante o
// backoff deve devolver o erro do contexto e não esperar o sleep todo.
func TestSendText_RetryHonorsContextCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprintln(w, `{"error":{"code":130429}}`)
	}))
	defer server.Close()

	// Base delay grande pra forçar o cancel a chegar antes do sleep
	// terminar.
	c, err := NewWhatsAppBusinessClient(WhatsAppConfig{
		AccessToken:      "T",
		PhoneNumberID:    "P",
		BaseURL:          server.URL,
		RetryMaxAttempts: 3,
		RetryBaseDelay:   5 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewWhatsAppBusinessClient: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		// Cancela enquanto o cliente está no primeiro backoff
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = c.SendText(ctx, "5511999999999", "msg")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from ctx cancellation")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if elapsed > 1*time.Second {
		t.Errorf("ctx cancel didn't short-circuit backoff; elapsed=%v", elapsed)
	}
}

func TestWhatsAppContract_EnforcesTimeoutAndOpensCircuit(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
				_, _ = fmt.Fprintln(w, `{"messages":[{"id":"late"}]}`)
			}
		}))
		defer server.Close()

		client, err := NewWhatsAppBusinessClient(WhatsAppConfig{
			AccessToken:      "sandbox-token",
			PhoneNumberID:    "sandbox-phone",
			BaseURL:          server.URL,
			Timeout:          10 * time.Millisecond,
			RetryMaxAttempts: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		_, err = client.SendText(context.Background(), "5511999999999", "contrato")
		if !errors.Is(err, ErrMetaAPIError) {
			t.Fatalf("expected timeout wrapped as ErrMetaAPIError, got %v", err)
		}
		if elapsed := time.Since(started); elapsed >= 300*time.Millisecond {
			t.Fatalf("configured timeout was not enforced: elapsed=%s", elapsed)
		}
	})

	t.Run("circuit breaker", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = fmt.Fprintln(w, `{"error":{"code":1,"message":"sandbox unavailable"}}`)
		}))
		defer server.Close()
		client := newTestClient(t, server)

		for i := 0; i < 5; i++ {
			if _, err := client.SendText(context.Background(), "5511999999999", "contrato"); !errors.Is(err, ErrMetaAPIError) {
				t.Fatalf("attempt %d: expected provider error, got %v", i+1, err)
			}
		}
		_, err := client.SendText(context.Background(), "5511999999999", "contrato")
		if !errors.Is(err, gobreaker.ErrOpenState) {
			t.Fatalf("expected open circuit, got %v", err)
		}
		if got := calls.Load(); got != 5 {
			t.Fatalf("open circuit should reject locally: calls=%d", got)
		}
	})

	t.Run("recipient errors do not open global circuit", func(t *testing.T) {
		var calls atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls.Add(1)
			w.WriteHeader(http.StatusBadRequest)
			_, _ = fmt.Fprintln(w, `{"error":{"code":131026,"message":"recipient unavailable"}}`)
		}))
		defer server.Close()
		client := newTestClient(t, server)

		for i := 0; i < 6; i++ {
			if _, err := client.SendText(context.Background(), "5511999999999", "contrato"); !errors.Is(err, ErrInvalidRecipient) {
				t.Fatalf("attempt %d: expected recipient error, got %v", i+1, err)
			}
		}
		if got := calls.Load(); got != 6 {
			t.Fatalf("request-level errors must not open provider circuit: calls=%d", got)
		}
	})
}
