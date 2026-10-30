package handlers

import (
	"testing"

	gwconfig "github.com/oginoino/toca/gateway/internal/config"
	"github.com/oginoino/toca/gateway/internal/services/proxy"

	"github.com/gin-gonic/gin"
)

// TestRegisterDealProxiesChecklistTemplates guards against the gateway
// whitelist missing /api/v1/checklist-templates — when the route is absent,
// the Esteira Documentation tab silently falls back to the bundled kit set
// and backend-side kit configuration (form-only CPF/telefone etc.) never
// reaches operators.
func TestRegisterDealProxiesChecklistTemplates(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterDeal(r, gwconfig.Config{DealServiceURL: "http://deal.test"}, proxy.New())

	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Path] = true
	}

	want := []string{
		"/api/v1/checklist-templates",
		"/api/v1/checklist-templates/*path",
	}
	for _, path := range want {
		if !registered[path] {
			t.Errorf("gateway does not proxy %q to the deal service; checklist-templates requests would 404", path)
		}
	}
}
