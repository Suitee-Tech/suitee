package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/metrics"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// retryDelay computes the delay before the next attempt using exponential
// backoff with full jitter (https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/).
// attempt: 0-indexed retry number (0=first retry).
// Returns a duration in [base/2 ... 1.25*expo, cap] approximately.
//
// The base is 100ms, the cap 5s — long enough to absorb a brief upstream
// hiccup, short enough to leave headroom for the surrounding 30s request
// timeout (3 retries fit comfortably under that ceiling).
func retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	if attempt > 6 {
		attempt = 6 // 100ms<<6 = 6.4s — already above cap
	}
	base := 100 * time.Millisecond
	expo := base << uint(attempt)
	if expo > 5*time.Second {
		expo = 5 * time.Second
	}
	// Full jitter: pick uniformly in [0, expo] then bias by 25% so we don't
	// have a thundering-herd on attempt=0. Falls back to expo if rand
	// somehow returns zero across all retries.
	jitter := time.Duration(rand.Int63n(int64(expo / 2)))
	return expo/2 + jitter
}

// parseRetryAfter honors the Retry-After header in either of its two
// RFC 7231 forms (seconds-int OR HTTP-date). Returns 0 when the header
// is missing/invalid so callers fall back to retryDelay.
func parseRetryAfter(h http.Header) time.Duration {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return time.Duration(n) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d > 0 {
			return d
		}
	}
	return 0
}

// shouldRetryStatus tells the retry loop which HTTP statuses are worth
// another attempt. 5xx is the default retryable bucket; among 4xx only
// 408 (Request Timeout) and 429 (Too Many Requests) get a retry — every
// other 4xx is a client-side issue that retrying won't fix.
func shouldRetryStatus(code int) bool {
	if code >= 500 {
		return true
	}
	return code == http.StatusRequestTimeout || code == http.StatusTooManyRequests
}

type HeaderSetter func(h http.Header)

type HTTPClient struct {
	BaseURL     string
	HTTPClient  *http.Client
	breaker     *gobreaker.CircuitBreaker
	headers     HeaderSetter
	maxAttempts int
}

type HTTPClientOption func(*HTTPClient)

func WithHeaders(setter HeaderSetter) HTTPClientOption {
	return func(c *HTTPClient) {
		c.headers = setter
	}
}

func WithTimeout(timeout time.Duration) HTTPClientOption {
	return func(c *HTTPClient) {
		if c.HTTPClient == nil {
			c.HTTPClient = &http.Client{}
		}
		c.HTTPClient.Timeout = timeout
	}
}

// WithMaxAttempts configures the total number of HTTP attempts for one
// logical call. Production clients normally keep the default of three; a
// value of one is useful for non-idempotent provider operations and for
// deterministic contract tests.
func WithMaxAttempts(attempts int) HTTPClientOption {
	return func(c *HTTPClient) {
		if attempts > 0 {
			c.maxAttempts = attempts
		}
	}
}

func NewHTTPClient(baseURL string, opts ...HTTPClientOption) *HTTPClient {
	c := &HTTPClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		maxAttempts: 3,
	}
	for _, opt := range opts {
		opt(c)
	}

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
	c.breaker = cb
	metrics.SetCircuitBreakerState(baseURL, int64(cb.State()))

	return c
}

func (c *HTTPClient) Do(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	if c.BaseURL == "" {
		return nil, fmt.Errorf("BaseURL is not configured for call to %s", path)
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
		maxRetries := c.maxAttempts
		if maxRetries <= 0 {
			maxRetries = 1
		}
		for attempt := 0; attempt < maxRetries; attempt++ {
			var reqBody io.Reader
			if body != nil {
				reqBody = bytes.NewReader(bodyBytes)
			}

			req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
			if err != nil {
				return nil, fmt.Errorf("failed to create request: %w", err)
			}

			if body != nil {
				req.Header.Set("Content-Type", "application/json")
			}

			if c.headers != nil {
				c.headers(req.Header)
			}

			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(req.Header))

			resp, doErr = c.HTTPClient.Do(req)
			// Success path: any response code not in the retryable bucket
			// returns immediately. 4xx (except 408/429) is "client bug,
			// don't retry"; 2xx/3xx is success.
			if doErr == nil && !shouldRetryStatus(resp.StatusCode) {
				return resp, nil
			}

			if attempt == maxRetries-1 {
				break
			}

			// Compute the next delay. Retry-After (when the upstream
			// supplied one) takes precedence over our default backoff
			// because the server knows when it's actually ready again —
			// blindly retrying earlier would amplify the load it asked
			// us to back off from.
			var delay time.Duration
			if resp != nil {
				delay = parseRetryAfter(resp.Header)
				resp.Body.Close()
			}
			if delay == 0 {
				delay = retryDelay(attempt)
			}

			// Honor ctx cancellation while sleeping so a caller timeout
			// (RequestTimeout middleware) doesn't have to wait out the
			// backoff before unwinding the call stack.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
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
