package events

import (
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestRetryCountAcceptsAMQPIntegerRepresentations(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  int
	}{
		{value: int32(3), want: 3},
		{value: int64(4), want: 4},
		{value: "5", want: 5},
		{value: nil, want: 0},
	} {
		if got := retryCount(amqp.Table{"x-retry-count": tc.value}); got != tc.want {
			t.Fatalf("retryCount(%T(%v)) = %d, want %d", tc.value, tc.value, got, tc.want)
		}
	}
}

func TestHandlerRetryDelayUsesBoundedExponentialBackoff(t *testing.T) {
	wants := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second, 32 * time.Second, 60 * time.Second}
	for i, want := range wants {
		if got := handlerRetryDelay(i + 1); got != want {
			t.Fatalf("handlerRetryDelay(%d) = %s, want %s", i+1, got, want)
		}
	}
	if got := handlerRetryDelay(20); got != handlerRetryMax {
		t.Fatalf("handlerRetryDelay(20) = %s, want cap %s", got, handlerRetryMax)
	}
}

func TestCopyAMQPHeadersDoesNotMutateOriginal(t *testing.T) {
	original := amqp.Table{"tenant_id": "tenant-1"}
	cloned := copyAMQPHeaders(original)
	cloned["x-retry-count"] = int32(1)
	if _, exists := original["x-retry-count"]; exists {
		t.Fatal("copyAMQPHeaders mutated original delivery headers")
	}
}
