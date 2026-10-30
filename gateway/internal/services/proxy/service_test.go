package proxy

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestReverseProxyErrorHandler_ClassifiesFailures(t *testing.T) {
	target, err := url.Parse("http://toca-financial:8083")
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}
	handler := reverseProxyErrorHandler(target)

	tests := []struct {
		name       string
		requestCtx context.Context
		err        error
		wantStatus int
	}{
		{
			name:       "deadline exceeded",
			requestCtx: context.Background(),
			err:        context.DeadlineExceeded,
			wantStatus: http.StatusGatewayTimeout,
		},
		{
			name:       "transport timeout",
			requestCtx: context.Background(),
			err:        proxyTimeoutError{},
			wantStatus: http.StatusGatewayTimeout,
		},
		{
			name:       "connection failure",
			requestCtx: context.Background(),
			err:        fmt.Errorf("dial tcp: connection refused"),
			wantStatus: http.StatusBadGateway,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/transactions", nil).WithContext(tt.requestCtx)
			writer := newResponseWriteTracker()

			handler(writer, req, tt.err)

			if writer.status != tt.wantStatus {
				t.Fatalf("status: want %d, got %d", tt.wantStatus, writer.status)
			}
			if got := writer.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
				t.Fatalf("content type: %q", got)
			}
			if body := writer.body.String(); !strings.Contains(body, fmt.Sprintf(`"statusCode":%d`, tt.wantStatus)) {
				t.Fatalf("body does not contain status %d: %q", tt.wantStatus, body)
			}
		})
	}
}

func TestReverseProxyErrorHandler_RequestDeadlineReturnsGatewayTimeout(t *testing.T) {
	target, _ := url.Parse("http://toca-financial:8083")
	handler := reverseProxyErrorHandler(target)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/charges", nil).WithContext(ctx)
	writer := newResponseWriteTracker()
	handler(writer, req, fmt.Errorf("upstream stopped"))

	if writer.status != http.StatusGatewayTimeout {
		t.Fatalf("status: want %d, got %d", http.StatusGatewayTimeout, writer.status)
	}
}

func TestReverseProxyErrorHandler_ClientCancellationWritesNothing(t *testing.T) {
	target, _ := url.Parse("http://toca-financial:8083")
	handler := reverseProxyErrorHandler(target)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/commissions", nil).WithContext(ctx)
	writer := newResponseWriteTracker()
	handler(writer, req, context.Canceled)

	if writer.status != 0 {
		t.Fatalf("canceled request wrote status %d", writer.status)
	}
	if writer.body.Len() != 0 {
		t.Fatalf("canceled request wrote body %q", writer.body.String())
	}
}

func TestStreamingReverseProxy_TruncatedUpstreamDoesNotPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("upstream response writer does not support hijacking")
		}

		conn, rw, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack upstream connection: %v", err)
		}
		defer conn.Close()

		// Declare a body larger than the bytes actually sent. The abrupt close
		// makes ReverseProxy's response copier return unexpected EOF after the
		// SSE response has already started.
		_, _ = fmt.Fprint(rw, "HTTP/1.1 200 OK\r\n")
		_, _ = fmt.Fprint(rw, "Content-Type: text/event-stream\r\n")
		_, _ = fmt.Fprint(rw, "Content-Length: 128\r\n")
		_, _ = fmt.Fprint(rw, "\r\n")
		_, _ = fmt.Fprint(rw, "data: partial\n\n")
		_ = rw.Flush()
	}))
	defer upstream.Close()

	router := gin.New()
	router.GET("/api/v1/ai/chat", New().StreamingReverseProxy(upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ai/chat", nil)
	// ReverseProxy emits http.ErrAbortHandler for body-copy failures only
	// when it is running under an HTTP server. Reproduce that production
	// context while invoking Gin directly so the test can observe the panic.
	ctx := context.WithValue(req.Context(), http.ServerContextKey, &http.Server{})
	req = req.WithContext(ctx)
	recorder := &closeNotifyRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		closed:           make(chan bool),
	}

	recovered := serveHTTPAndCapturePanic(router, recorder, req)
	if recovered != nil {
		t.Fatalf("streaming proxy leaked a panic: %v", recovered)
	}
	if got := recorder.Code; got != http.StatusOK {
		t.Fatalf("status: want %d, got %d", http.StatusOK, got)
	}
	if got := recorder.Body.String(); !strings.Contains(got, "data: partial") {
		t.Fatalf("partial SSE payload was not forwarded: %q", got)
	}
}

func serveHTTPAndCapturePanic(handler http.Handler, recorder http.ResponseWriter, req *http.Request) (recovered any) {
	defer func() {
		recovered = recover()
	}()
	handler.ServeHTTP(recorder, req)
	return nil
}

type closeNotifyRecorder struct {
	*httptest.ResponseRecorder
	closed chan bool
}

// Gin's response writer still supports the deprecated CloseNotifier contract,
// so the direct ServeHTTP fixture needs to provide it just like net/http does.
func (r *closeNotifyRecorder) CloseNotify() <-chan bool {
	return r.closed
}

type proxyTimeoutError struct{}

func (proxyTimeoutError) Error() string   { return "upstream timeout" }
func (proxyTimeoutError) Timeout() bool   { return true }
func (proxyTimeoutError) Temporary() bool { return true }

type responseWriteTracker struct {
	header http.Header
	status int
	body   strings.Builder
}

func newResponseWriteTracker() *responseWriteTracker {
	return &responseWriteTracker{header: make(http.Header)}
}

func (w *responseWriteTracker) Header() http.Header {
	return w.header
}

func (w *responseWriteTracker) WriteHeader(statusCode int) {
	w.status = statusCode
}

func (w *responseWriteTracker) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(p)
}

// A16 — a cadeia de encaminhamento entregue ao serviço interno não pode conter
// valores escolhidos pelo cliente.
func TestReverseProxySanitizesClientSuppliedForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotForwardedFor, gotRealIP, gotForwarded string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		gotRealIP = r.Header.Get("X-Real-IP")
		gotForwarded = r.Header.Get("Forwarded")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	engine := gin.New()
	engine.SetTrustedProxies(nil) // nenhum proxy confiável: cabeçalhos do cliente são descartados
	engine.Any("/api/v1/deals", New().ReverseProxy(upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil)
	req.RemoteAddr = "198.51.100.9:4321"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	req.Header.Set("X-Real-IP", "9.9.9.9")
	req.Header.Set("Forwarded", "for=1.2.3.4")
	w := proxyRecorder()
	if recovered := serveHTTPAndCapturePanic(engine, w, req); recovered != nil {
		t.Fatalf("proxy leak de panic: %v", recovered)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(gotForwardedFor, "1.2.3.4") || strings.Contains(gotForwardedFor, "5.6.7.8") {
		t.Fatalf("X-Forwarded-For repassou valor do cliente: %q", gotForwardedFor)
	}
	if !strings.HasPrefix(gotForwardedFor, "198.51.100.9") {
		t.Fatalf("X-Forwarded-For = %q, want o cliente real 198.51.100.9 na primeira posição", gotForwardedFor)
	}
	if gotRealIP != "" || gotForwarded != "" {
		t.Fatalf("cabeçalhos forjados repassados: X-Real-IP=%q Forwarded=%q", gotRealIP, gotForwarded)
	}
}

// Com o proxy de entrada declarado, o cliente real (o que o proxy viu) é o
// valor repassado.
func TestReverseProxyForwardsTrustedProxyClientIP(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var gotForwardedFor string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotForwardedFor = r.Header.Get("X-Forwarded-For")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	engine := gin.New()
	if err := engine.SetTrustedProxies([]string{"192.0.2.0/24"}); err != nil {
		t.Fatalf("set trusted proxies: %v", err)
	}
	engine.Any("/api/v1/deals", New().ReverseProxy(upstream.URL))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deals", nil)
	req.RemoteAddr = "192.0.2.10:4321" // par TCP = proxy confiável
	// O proxy confiável ACRESCENTA o cliente real na última posição; entradas à
	// esquerda são lixo do cliente e não podem ser repassadas.
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")
	w := proxyRecorder()
	if recovered := serveHTTPAndCapturePanic(engine, w, req); recovered != nil {
		t.Fatalf("proxy leak de panic: %v", recovered)
	}

	if !strings.HasPrefix(gotForwardedFor, "203.0.113.7") {
		t.Fatalf("X-Forwarded-For = %q, want 203.0.113.7 (cliente real visto pelo proxy)", gotForwardedFor)
	}
	if strings.Contains(gotForwardedFor, "1.2.3.4") {
		t.Fatalf("X-Forwarded-For manteve valor forjado à esquerda: %q", gotForwardedFor)
	}
}

// proxyRecorder dá ao ReverseProxy um ResponseWriter com CloseNotify, como o
// net/http faz em produção (o httptest.ResponseRecorder puro não implementa o
// contrato depreciado e o proxy entra em panic).
func proxyRecorder() *closeNotifyRecorder {
	return &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder(), closed: make(chan bool)}
}

func TestReverseProxyStripsUpstreamCORSHeaders(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "https://app.example.com")
		w.Header().Add("Access-Control-Allow-Origin", "https://app.example.com")
		w.Header().Set("Access-Control-Expose-Headers", "X-Request-Id")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	engine := gin.New()
	engine.Any("/api/v1/sites/me", New().ReverseProxy(upstream.URL))
	w := proxyRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/sites/me", nil))
	if got := w.Header().Values("Access-Control-Allow-Origin"); len(got) != 0 {
		t.Fatalf("gateway deve ser a única fonte de CORS, veio %#v", got)
	}
	if got := w.Header().Get("Access-Control-Expose-Headers"); got != "" {
		t.Fatalf("CORS do upstream não deve vazar, veio %q", got)
	}
}
