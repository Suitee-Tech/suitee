package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

// TestRegisterCorePartnershipsDispatch guards against the gateway regression
// where the whole /partnerships subtree was proxied to toca-core, so
// /partnerships/shared/leads (owned by toca-crm) returned 404 — breaking the
// frontend's getAllSharedLeads() call. shared/leads must reach CRM while the
// rest of the subtree (shared/properties, lifecycle, /:id) stays on core.
func TestRegisterCorePartnershipsDispatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("core"))
	}))
	defer core.Close()
	crm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("crm"))
	}))
	defer crm.Close()

	r := gin.New()
	RegisterCore(r, gwconfig.Config{
		CoreServiceURL: core.URL,
		CRMServiceURL:  crm.URL,
	}, proxy.New())

	// Real server + client: the stdlib ReverseProxy needs an http.CloseNotifier,
	// which httptest.NewRecorder does not provide.
	gw := httptest.NewServer(r)
	defer gw.Close()

	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/partnerships/shared/leads", "crm"},
		{"/api/v1/partnerships/shared/properties", "core"},
		{"/api/v1/partnerships", "core"},
		{"/api/v1/partnerships/abc-123", "core"},
		{"/api/v1/partnerships/abc-123/shared-portfolio", "core"},
	}
	for _, tc := range cases {
		resp, err := http.Get(gw.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if got := string(body); got != tc.want {
			t.Errorf("%s routed to %q, want %q (status %d)", tc.path, got, tc.want, resp.StatusCode)
		}
	}
}
