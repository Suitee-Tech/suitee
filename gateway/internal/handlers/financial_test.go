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

// TestRegisterFinancialProxiesFinancialPreview guards against the gateway
// refactor regression where /api/v1/financial-preview/* was never wired to
// toca-financial, making the Esteira "Financeiro (Prévia)" tab return 404.
func TestRegisterFinancialProxiesFinancialPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterFinancial(r, gwconfig.Config{FinancialServiceURL: "http://financial.test"}, proxy.New())

	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Path] = true
	}

	want := []string{
		"/api/v1/financial-preview",
		"/api/v1/financial-preview/*path",
	}
	for _, path := range want {
		if !registered[path] {
			t.Errorf("gateway does not proxy %q to the financial service; financial-preview requests would 404", path)
		}
	}
}

// TestRegisterFinancialProxiesManagedBills guards against the gateway
// whitelist missing /api/v1/managed-bills, which made "Financeiro > Contas de
// consumo" fail with 404 (list, approve, pay, reconcile, reverse).
func TestRegisterFinancialProxiesManagedBills(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterFinancial(r, gwconfig.Config{FinancialServiceURL: "http://financial.test"}, proxy.New())

	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Path] = true
	}

	want := []string{
		"/api/v1/managed-bills",
		"/api/v1/managed-bills/*path",
	}
	for _, path := range want {
		if !registered[path] {
			t.Errorf("gateway does not proxy %q to the financial service; managed-bills requests would 404", path)
		}
	}
}

func TestRegisterFinancialProxiesAuthenticatedProofContent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterFinancial(r, gwconfig.Config{FinancialServiceURL: "http://financial.test"}, proxy.New())

	registered := make(map[string]bool)
	for _, route := range r.Routes() {
		registered[route.Path] = true
	}
	if !registered["/api/v1/proofs/*path"] {
		t.Fatal("gateway does not proxy authenticated proof content to the financial service")
	}
}

func TestRegisterDealDispatchesSecurityDepositBalanceToFinancial(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("deal"))
	}))
	defer deal.Close()
	financial := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("financial"))
	}))
	defer financial.Close()

	r := gin.New()
	RegisterDeal(r, gwconfig.Config{
		DealServiceURL:      deal.URL,
		FinancialServiceURL: financial.URL,
	}, proxy.New())

	gw := httptest.NewServer(r)
	defer gw.Close()

	cases := []struct {
		path string
		want string
	}{
		{"/api/v1/contracts/contract-1", "deal"},
		{"/api/v1/contracts/contract-1/security-deposit-balance", "financial"},
	}
	for _, tc := range cases {
		resp, err := http.Get(gw.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if got := string(body); got != tc.want {
			t.Errorf("%s routed to %q, want %q", tc.path, got, tc.want)
		}
	}
}
