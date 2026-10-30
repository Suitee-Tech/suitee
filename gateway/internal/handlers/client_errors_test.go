package handlers

import (
	"bytes"
	"log"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestClientErrorDiagnostics(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterClientErrors(r)
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(old) })
	valid := `{"kind":"http","code":"11111111-1111-4111-8111-111111111111","request_id":"11111111-1111-4111-8111-111111111111","operation":"PUT appointments","status":500,"release":"1.0.0"}`
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"correlation", valid, 202},
		{"família brokers", strings.Replace(valid, "PUT appointments", "POST brokers", 1), 202},
		{"form contents", strings.Replace(valid, `"kind":"http"`, `"email":"client@example.invalid","kind":"http"`, 1), 400},
		{"raw URL", strings.Replace(valid, "PUT appointments", "PUT /leads/private-client", 1), 400},
		{"invalid code", strings.ReplaceAll(valid, "11111111-1111-4111-8111-111111111111", "private-email@example.invalid"), 400},
		{"oversized", strings.Repeat(" ", 2050) + valid, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/api/v1/client-errors", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status %d: %s", w.Code, w.Body.String())
			}
			if tc.status == 202 && !strings.Contains(logs.String(), `"request_id":"11111111-1111-4111-8111-111111111111"`) {
				t.Fatalf("missing correlation: %s", logs.String())
			}
			if tc.status != 202 && logs.Len() != 0 {
				t.Fatalf("invalid diagnostic was logged: %s", logs.String())
			}
		})
	}
}
