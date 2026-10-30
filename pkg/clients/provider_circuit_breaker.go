package clients

import (
	"fmt"
	"time"

	"github.com/oginoino/toca/pkg/metrics"
	"github.com/sony/gobreaker"
)

// newProviderCircuitBreaker standardizes resilience for provider-specific
// clients that need to inspect the upstream response body themselves (Meta and
// Zenvia). HTTPClient already applies the same policy to clients such as
// Clicksign, DeepSeek and Resend.
func newProviderCircuitBreaker(name string) *gobreaker.CircuitBreaker {
	breaker := gobreaker.NewCircuitBreaker(gobreaker.Settings{
		Name:        name,
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
		OnStateChange: func(_ string, _ gobreaker.State, to gobreaker.State) {
			metrics.SetCircuitBreakerState(name, int64(to))
		},
	})
	metrics.SetCircuitBreakerState(name, int64(breaker.State()))
	return breaker
}

type providerCircuitBreaker interface {
	Execute(func() (interface{}, error)) (interface{}, error)
}

type providerCallResult[T any] struct {
	value T
	err   error
}

// executeProviderCall lets provider adapters distinguish availability failures
// from request-level rejections. A malformed recipient must not open a global
// circuit and block valid messages for every tenant.
func executeProviderCall[T any](breaker providerCircuitBreaker, shouldTrip func(error) bool, call func() (T, error)) (T, error) {
	var zero T
	out, breakerErr := breaker.Execute(func() (interface{}, error) {
		value, callErr := call()
		result := providerCallResult[T]{value: value, err: callErr}
		if callErr != nil && (shouldTrip == nil || shouldTrip(callErr)) {
			return result, callErr
		}
		// Report request-level errors as successful circuit probes; the caller
		// still receives the original typed error below.
		return result, nil
	})
	result, ok := out.(providerCallResult[T])
	if ok && result.err != nil {
		return zero, result.err
	}
	if breakerErr != nil {
		return zero, breakerErr
	}
	if !ok {
		return zero, fmt.Errorf("provider circuit breaker returned an unexpected result")
	}
	return result.value, nil
}
