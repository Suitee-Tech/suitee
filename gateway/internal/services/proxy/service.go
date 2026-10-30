package proxy

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Service struct{}

func New() *Service {
	return &Service{}
}

func (s *Service) ReverseProxy(target string) gin.HandlerFunc {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Error parsing target URL %s: %v", target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
	}
	proxy.ErrorHandler = reverseProxyErrorHandler(targetURL)
	proxy.ModifyResponse = stripUpstreamCORS

	return func(c *gin.Context) {
		c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))
		sanitizeForwardedChain(c)

		if c.GetHeader("Upgrade") == "websocket" {
			c.Request.Header.Set("Connection", "Upgrade")
			c.Request.Header.Set("Upgrade", "websocket")
		}

		serveHTTP(proxy, c)
	}
}

func (s *Service) StreamingReverseProxy(target string) gin.HandlerFunc {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Error parsing target URL %s: %v", target, err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.Transport = &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ResponseHeaderTimeout: 2 * time.Minute,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
	}
	proxy.FlushInterval = -1
	proxy.ErrorHandler = reverseProxyErrorHandler(targetURL)
	proxy.ModifyResponse = stripUpstreamCORS

	return func(c *gin.Context) {
		c.Request.Header.Set("X-Forwarded-Host", c.Request.Header.Get("Host"))
		sanitizeForwardedChain(c)
		serveHTTP(proxy, c)
	}
}

// sanitizeForwardedChain (achado A16 da revisão de permissões de 2026-09-19)
// reconstrói a cadeia de encaminhamento antes de entregar a requisição ao
// serviço interno.
//
// O `httputil.ReverseProxy` do net/http ACRESCENTA o par TCP ao
// `X-Forwarded-For` recebido — ele não substitui um valor forjado pelo cliente.
// Como o gin, por padrão, confia em todos os proxies, o serviço interno acabava
// resolvendo `c.ClientIP()` a partir do primeiro item da lista, que é
// exatamente o que o cliente enviou. Aqui apagamos `X-Forwarded-For`,
// `X-Real-IP` e `Forwarded` do cliente e escrevemos a identidade resolvida pela
// política de proxies confiáveis (pkg/middleware.ApplyTrustedProxies); o
// ReverseProxy então acrescenta o próprio par TCP, formando
// "<cliente real>, <gateway>" — que qualquer serviço atrás do gateway resolve
// corretamente, com ou sem TRUSTED_PROXIES configurado.
func sanitizeForwardedChain(c *gin.Context) {
	// A resolução PRECISA acontecer antes da limpeza: `c.ClientIP()` é quem lê
	// o X-Forwarded-For que o proxy confiável acrescentou.
	clientIP := c.ClientIP()

	c.Request.Header.Del("Forwarded")
	c.Request.Header.Del("X-Real-IP")
	c.Request.Header.Del("X-Forwarded-For")

	if clientIP != "" {
		c.Request.Header.Set("X-Forwarded-For", clientIP)
	}
}

// stripUpstreamCORS remove os headers CORS que o serviço interno também
// adiciona. O gateway é a única borda CORS: repassar os dois conjuntos faz o
// browser rejeitar `Access-Control-Allow-Origin` com valores duplicados.
// O middleware CORS do gateway, executado antes do proxy, recoloca os headers
// corretos na resposta final.
func stripUpstreamCORS(resp *http.Response) error {
	for key := range resp.Header {
		if strings.HasPrefix(http.CanonicalHeaderKey(key), "Access-Control-") {
			resp.Header.Del(key)
		}
	}
	return nil
}

func reverseProxyErrorHandler(targetURL *url.URL) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		// When the browser disconnected or deliberately cancelled the request,
		// there is no client left to receive an error response. Writing a
		// synthetic 502 only pollutes access logs and may race a closed socket.
		if errors.Is(r.Context().Err(), context.Canceled) {
			log.Printf("Reverse proxy request canceled target=%s method=%s path=%s request_id=%s err=%v", targetURL.String(), r.Method, r.URL.Path, r.Header.Get("X-Request-ID"), err)
			return
		}

		status := http.StatusBadGateway
		message := "Bad Gateway"
		if isProxyTimeout(r, err) {
			status = http.StatusGatewayTimeout
			message = "Gateway Timeout"
		}

		log.Printf("Reverse proxy error target=%s method=%s path=%s request_id=%s status=%d err=%v", targetURL.String(), r.Method, r.URL.Path, r.Header.Get("X-Request-ID"), status, err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"success":false,"message":"` + message + `","statusCode":` + strconv.Itoa(status) + `,"data":null}`))
	}
}

func isProxyTimeout(r *http.Request, err error) bool {
	if errors.Is(r.Context().Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// serveHTTP contains the sentinel panic that net/http's ReverseProxy uses when
// a response body is interrupted after the headers were sent. Client
// disconnects and truncated SSE/download responses are request-scoped
// failures; they must not terminate the gateway process. Any unrelated panic
// is re-thrown so the normal recovery middleware can report it.
func serveHTTP(proxy *httputil.ReverseProxy, c *gin.Context) {
	defer func() {
		if recovered := recover(); recovered != nil && recovered != http.ErrAbortHandler {
			panic(recovered)
		}
	}()

	proxy.ServeHTTP(c.Writer, c.Request)
}
