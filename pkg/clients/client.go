package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// BaseClient handles common HTTP operations for internal service-to-service calls
type BaseClient struct {
	BaseURL        string
	HTTPClient     *http.Client
	InternalAPIKey string
	breaker        *gobreaker.CircuitBreaker
}

// internalCallerKey resolve a chave que ESTE serviço apresenta nas chamadas
// internas.
//
// `INTERNAL_CALLER_KEY` tem precedência sobre `INTERNAL_API_KEY` para permitir
// a migração sem big-bang do modelo de credencial compartilhada
// (ver pkg/auth/service_identity.go):
//
//  1. sem INTERNAL_CALLER_KEY nada muda — o serviço segue apresentando a chave
//     compartilhada, que os receptores aceitam como identidade "legacy";
//  2. o operador publica a chave nomeada de cada chamador em
//     INTERNAL_SERVICE_KEYS nos receptores (a chave compartilhada continua
//     valendo);
//  3. só então preenche INTERNAL_CALLER_KEY de cada serviço — a partir daí a
//     chamada interna chega identificada (`service=<nome>` no log
//     admin.access) e pode ser restringida por tenant.
func internalCallerKey() string {
	if scoped := strings.TrimSpace(os.Getenv("INTERNAL_CALLER_KEY")); scoped != "" {
		return scoped
	}
	return os.Getenv("INTERNAL_API_KEY")
}

// NewBaseClient creates a new base client for internal calls
func NewBaseClient(baseURL string) *BaseClient {
	internalAPIKey := internalCallerKey()
	cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        baseURL,
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(_ string, _ gobreaker.State, to gobreaker.State) {
			metrics.SetCircuitBreakerState(baseURL, int64(to))
		},
	})
	metrics.SetCircuitBreakerState(baseURL, int64(cb.State()))
	return &BaseClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 5 * time.Second, // Timeout of 5s
		},
		InternalAPIKey: internalAPIKey,
		breaker:        cb,
	}
}

func (c *BaseClient) Do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	return c.do(ctx, method, path, body, "")
}

// DoWithTenant se comporta como Do, mas também envia o header X-Tenant-Id.
// É obrigatório para serviços cujo middleware GLOBAL de rotas internas
// (auth.InternalOrMiddleware) prioriza o caminho da chave interna quando
// X-Internal-Key está presente e exige X-Tenant-Id para resolver o tenant
// (o toca-core é assim: sem o header, a chamada responde 401
// "missing X-Tenant-Id"). O JWT do usuário, quando existe, continua sendo
// encaminhado junto — inerte nos destinos que não o usam.
func (c *BaseClient) DoWithTenant(ctx context.Context, method, path string, body interface{}, tenantID string) (*http.Response, error) {
	return c.do(ctx, method, path, body, tenantID)
}

func (c *BaseClient) do(ctx context.Context, method, path string, body interface{}, tenantID string) (*http.Response, error) {
	internalKey := c.InternalAPIKey
	if internalKey == "" {
		return nil, fmt.Errorf("INTERNAL_API_KEY is not set")
	}

	if c.BaseURL == "" {
		return nil, fmt.Errorf("BaseURL is not configured for internal call to %s", path)
	}

	url := c.BaseURL + path
	var bodyBytes []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body: %w", err)
		}
		bodyBytes = b
	}

	out, err := c.breaker.Execute(func() (interface{}, error) {
		var resp *http.Response
		var doErr error
		maxRetries := 3
		for attempt := 0; attempt < maxRetries; attempt++ {
			var reqBody io.Reader
			if body != nil {
				reqBody = bytes.NewReader(bodyBytes)
			}

			req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			req.Header.Set("X-Internal-Key", internalKey)
			if tenantID != "" {
				req.Header.Set("X-Tenant-Id", tenantID)
			}
			// P1/A3: quando a chamada interna acontece DENTRO de uma requisição
			// de usuário, encaminhamos também o JWT dele. O serviço de destino
			// passa a poder derivar o tenant do TOKEN em vez de confiar no
			// `?tenant_id=` que o chamador escolheu (achado A3: rotas internas
			// aceitavam o tenant do query string).
			//
			// É inerte para rotas que só exigem X-Internal-Key — nenhum destino
			// muda de comportamento sem adotar `auth.InternalOrUserMiddleware`.
			// Só trafega para serviços irmãos (*_SERVICE_URL): nenhum cliente de
			// provedor externo usa BaseClient.
			if token, terr := appCtx.GetAuthToken(ctx); terr == nil && token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			if body != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

			resp, doErr = c.HTTPClient.Do(req)
			if doErr == nil && resp.StatusCode < 500 {
				return resp, nil
			}

			if attempt == maxRetries-1 {
				break
			}

			if resp != nil {
				drainAndClose(resp.Body)
			}

			// Stop once the caller's deadline is blown — another attempt (plus
			// its backoff sleep) only piles load on a slow upstream and can't
			// beat the request deadline anyway. Keeps a slow internal hop (e.g.
			// financial-preview -> deal) from eating the whole route budget.
			if ctx.Err() != nil {
				break
			}

			backoff := 100 * time.Millisecond
			for i := 0; i < attempt; i++ {
				backoff *= 4
			}
			time.Sleep(backoff)
		}

		if doErr != nil {
			return nil, fmt.Errorf("request failed after retries: %w", doErr)
		}

		if resp == nil {
			return nil, fmt.Errorf("request failed with unknown error")
		}
		return nil, formatUpstreamStatusError(resp)
	})
	if err != nil {
		return nil, err
	}
	return out.(*http.Response), nil
}
