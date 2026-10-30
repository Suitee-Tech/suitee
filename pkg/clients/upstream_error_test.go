package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBaseClientDoPreservesUpstreamErrorMessage(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "test-key")

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("X-Internal-Key"); got != "test-key" {
			t.Fatalf("X-Internal-Key = %q, want test-key", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"success":false,"message":"load clicksign credentials: CLICKSIGN_ACCESS_TOKEN is required","statusCode":500,"data":null}`))
	}))
	defer server.Close()

	client := NewBaseClient(server.URL)
	_, err := client.Do(context.Background(), http.MethodPost, "/signature/contracts/contract-1/envelope", nil)
	if err == nil {
		t.Fatal("expected error")
	}

	got := err.Error()
	if !strings.Contains(got, "upstream returned status 500: load clicksign credentials: CLICKSIGN_ACCESS_TOKEN is required") {
		t.Fatalf("error = %q", got)
	}
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 retries", calls)
	}
}
