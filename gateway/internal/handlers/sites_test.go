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

// O toca-sites é o dono de /api/v1/sites*. Um registro faltando não quebra o
// build — quebra a tela /site com 404 —, então o roteamento é testado.
func TestRegisterSitesProxiesSitesSubtree(t *testing.T) {
	gin.SetMode(gin.TestMode)

	sites := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("sites:" + r.URL.Path))
	}))
	defer sites.Close()

	r := gin.New()
	RegisterSites(r, gwconfig.Config{SitesServiceURL: sites.URL}, proxy.New())
	gw := httptest.NewServer(r)
	defer gw.Close()

	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/sites/me", "sites:/api/v1/sites/me"},
		{"/api/v1/sites/listings/abc/stats", "sites:/api/v1/sites/listings/abc/stats"},
		{"/api/v1/sites/public/leads", "sites:/api/v1/sites/public/leads"},
	}
	for _, tc := range cases {
		resp, err := http.Get(gw.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if got := string(body); got != tc.want {
			t.Fatalf("%s → %q, want %q", tc.path, got, tc.want)
		}
	}
}
