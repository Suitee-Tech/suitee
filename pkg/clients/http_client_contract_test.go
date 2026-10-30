package clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sony/gobreaker"
)

func TestHTTPClientContract_EnforcesConfiguredTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(500 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, WithTimeout(10*time.Millisecond), WithMaxAttempts(1))
	started := time.Now()
	_, err := client.Do(context.Background(), http.MethodGet, "/slow", nil)
	if err == nil {
		t.Fatal("expected provider timeout")
	}
	if elapsed := time.Since(started); elapsed >= 300*time.Millisecond {
		t.Fatalf("configured timeout was not enforced: elapsed=%s", elapsed)
	}
}

func TestHTTPClientContract_OpensCircuitAfterConsecutiveFailures(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewHTTPClient(server.URL, WithMaxAttempts(1))
	for i := 0; i < 5; i++ {
		if _, err := client.Do(context.Background(), http.MethodGet, "/unavailable", nil); err == nil {
			t.Fatalf("attempt %d: expected upstream failure", i+1)
		}
	}

	_, err := client.Do(context.Background(), http.MethodGet, "/unavailable", nil)
	if !errors.Is(err, gobreaker.ErrOpenState) {
		t.Fatalf("expected open circuit, got %v", err)
	}
	if got := calls.Load(); got != 5 {
		t.Fatalf("open circuit should reject without network I/O: calls=%d", got)
	}
}
