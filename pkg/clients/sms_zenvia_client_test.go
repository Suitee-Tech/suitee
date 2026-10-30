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

// newTestSMSClient devolve um client com baseURL apontando para o
// httptest.Server, retry desabilitado por default (RetryMaxAttempts=1).
// Tests dedicados ao retry usam newTestSMSClientWithRetry.
func newTestSMSClient(t *testing.T, server *httptest.Server) *SMSZenviaClient {
	t.Helper()
	c, err := NewSMSZenviaClient(SMSZenviaConfig{
		APIToken:         "TEST_TOKEN",
		From:             "TocaImoveis",
		BaseURL:          server.URL,
		RetryMaxAttempts: 1,
	})
	if err != nil {
		t.Fatalf("NewSMSZenviaClient: %v", err)
	}
	return c
}

func newTestSMSClientWithRetry(t *testing.T, server *httptest.Server, attempts int) *SMSZenviaClient {
	t.Helper()
	c, err := NewSMSZenviaClient(SMSZenviaConfig{
		APIToken:         "T",
		From:             "F",
		BaseURL:          server.URL,
		RetryMaxAttempts: attempts,
		RetryBaseDelay:   1 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewSMSZenviaClient: %v", err)
	}
	return c
}

func TestNewSMSZenviaClient_RequiresCredentials(t *testing.T) {
	cases := []SMSZenviaConfig{
		{},
		{APIToken: "t"},            // sem From
		{From: "F"},                // sem APIToken
		{APIToken: " ", From: " "}, // whitespace
	}
	for i, cfg := range cases {
		_, err := NewSMSZenviaClient(cfg)
		if !errors.Is(err, ErrSMSNotConfigured) {
			t.Errorf("case %d: expected ErrSMSNotConfigured, got %v", i, err)
		}
	}
}

func TestSMSSendText_Success(t *testing.T) {
	var receivedAuth, receivedPath string
	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		receivedAuth = r.Header.Get("X-API-TOKEN")
		receivedBody, _ = readAll(r.Body)
		_, _ = fmt.Fprintln(w, `{
			"id":"msg-abc",
			"from":"TocaImoveis",
			"to":"5511999999999",
			"direction":"OUT"
		}`)
	}))
	defer server.Close()

	c := newTestSMSClient(t, server)
	res, err := c.SendText(context.Background(), "+55 11 99999-9999", "Olá, lembrete de pagamento.")
	if err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if res.MessageID != "msg-abc" {
		t.Errorf("expected msg-abc, got %q", res.MessageID)
	}
	if res.Recipient != "5511999999999" {
		t.Errorf("expected normalized recipient, got %q", res.Recipient)
	}
	if receivedPath != "/v2/channels/sms/messages" {
		t.Errorf("unexpected path: %s", receivedPath)
	}
	if receivedAuth != "TEST_TOKEN" {
		t.Errorf("expected X-API-TOKEN header, got %q", receivedAuth)
	}

	var payload map[string]any
	if err := json.Unmarshal(receivedBody, &payload); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if payload["from"] != "TocaImoveis" {
		t.Errorf("from mismatch: %v", payload["from"])
	}
	if payload["to"] != "5511999999999" {
		t.Errorf("to should be normalized: %v", payload["to"])
	}
	contents, _ := payload["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(contents))
	}
	first := contents[0].(map[string]any)
	if first["type"] != "text" {
		t.Errorf("content type should be text, got %v", first["type"])
	}
}

func TestSMSSendText_RejectsTooLongBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called when body > 160 chars")
	}))
	defer server.Close()

	c := newTestSMSClient(t, server)
	long := strings.Repeat("x", 161)
	_, err := c.SendText(context.Background(), "5511999999999", long)
	if !errors.Is(err, ErrSMSBodyTooLong) {
		t.Errorf("expected ErrSMSBodyTooLong, got %v", err)
	}
}

func TestSMSSendText_RejectsEmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for empty body")
	}))
	defer server.Close()
	c := newTestSMSClient(t, server)
	_, err := c.SendText(context.Background(), "5511999999999", "   ")
	if !errors.Is(err, ErrSMSAPIError) {
		t.Errorf("expected ErrSMSAPIError for empty body, got %v", err)
	}
}

func TestSMSSendText_RejectsBadPhone(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called for invalid phone")
	}))
	defer server.Close()
	c := newTestSMSClient(t, server)
	_, err := c.SendText(context.Background(), "", "msg")
	if !errors.Is(err, ErrSMSInvalidRecipient) {
		t.Errorf("expected ErrSMSInvalidRecipient, got %v", err)
	}
}

func TestSMSSendText_MapsErrorCodes(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		expectedAs error
	}{
		{
			name:       "invalid token (401)",
			status:     http.StatusUnauthorized,
			body:       `{"code":"AUTH_FAILED","message":"invalid token"}`,
			expectedAs: ErrSMSInvalidToken,
		},
		{
			name:       "forbidden (403)",
			status:     http.StatusForbidden,
			body:       `{"code":"FORBIDDEN","message":"sender not allowed"}`,
			expectedAs: ErrSMSInvalidToken,
		},
		{
			name:       "rate limited (429)",
			status:     http.StatusTooManyRequests,
			body:       `{"code":"RATE_LIMIT","message":"slow down"}`,
			expectedAs: ErrSMSRateLimited,
		},
		{
			name:       "generic 5xx",
			status:     http.StatusBadGateway,
			body:       `{"code":"INTERNAL","message":"upstream failure"}`,
			expectedAs: ErrSMSAPIError,
		},
		{
			name:       "non-JSON body",
			status:     http.StatusInternalServerError,
			body:       "<html>503</html>",
			expectedAs: ErrSMSAPIError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintln(w, tc.body)
			}))
			defer server.Close()
			c := newTestSMSClient(t, server)
			_, err := c.SendText(context.Background(), "5511999999999", "x")
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tc.expectedAs) {
				t.Errorf("expected errors.Is(%v), got %v", tc.expectedAs, err)
			}
		})
	}
}

func TestSMSSendText_RetryOnRateLimit_Succeeds(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = fmt.Fprintln(w, `{"code":"RATE","message":"slow"}`)
			return
		}
		_, _ = fmt.Fprintln(w, `{"id":"msg-retry","from":"F","to":"5511999999999","direction":"OUT"}`)
	}))
	defer server.Close()

	c := newTestSMSClientWithRetry(t, server, 3)
	res, err := c.SendText(context.Background(), "5511999999999", "msg")
	if err != nil {
		t.Fatalf("expected success after retry, got %v", err)
	}
	if res.MessageID != "msg-retry" {
		t.Errorf("expected wamid from second attempt, got %q", res.MessageID)
	}
	if calls.Load() != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 retry), got %d", calls.Load())
	}
}

func TestSMSSendText_RetryExhaustion(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprintln(w, `{"code":"RATE"}`)
	}))
	defer server.Close()

	c := newTestSMSClientWithRetry(t, server, 3)
	_, err := c.SendText(context.Background(), "5511999999999", "msg")
	if !errors.Is(err, ErrSMSRateLimited) {
		t.Errorf("expected ErrSMSRateLimited, got %v", err)
	}
	if !strings.Contains(err.Error(), "retry exhausted") {
		t.Errorf("expected 'retry exhausted' wrap, got %q", err.Error())
	}
	if calls.Load() != 3 {
		t.Errorf("expected 3 attempts, got %d", calls.Load())
	}
}

func TestSMSSendText_NetworkErrorWrapped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()
	c := newTestSMSClient(t, server)
	_, err := c.SendText(context.Background(), "5511999999999", "msg")
	if !errors.Is(err, ErrSMSAPIError) {
		t.Errorf("expected ErrSMSAPIError on network failure, got %v", err)
	}
}

func TestZenviaContract_EnforcesTimeoutAndOpensCircuit(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(500 * time.Millisecond):
				_, _ = fmt.Fprintln(w, `{"id":"late"}`)
			}
		}))
		defer server.Close()

		client, err := NewSMSZenviaClient(SMSZenviaConfig{
			APIToken:         "sandbox-token",
			From:             "sandbox-sender",
			BaseURL:          server.URL,
			Timeout:          10 * time.Millisecond,
			RetryMaxAttempts: 1,
		})
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		_, err = client.SendText(context.Background(), "5511999999999", "contrato")
		if !errors.Is(err, ErrSMSAPIError) {
			t.Fatalf("expected timeout wrapped as ErrSMSAPIError, got %v", err)
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
			_, _ = fmt.Fprintln(w, `{"code":"UPSTREAM","message":"sandbox unavailable"}`)
		}))
		defer server.Close()
		client := newTestSMSClient(t, server)

		for i := 0; i < 5; i++ {
			if _, err := client.SendText(context.Background(), "5511999999999", "contrato"); !errors.Is(err, ErrSMSAPIError) {
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
}

// readAll é um pequeno helper para ler ReadCloser sem importar mais
// pacotes nos tests.
func readAll(r interface{ Read(p []byte) (int, error) }) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			return buf, err
		}
	}
}
