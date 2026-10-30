package clients

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// jsonResponse devolve uma resposta HTTP mínima com corpo JSON.
func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{},
	}
}

// parseQuery interpreta a query string do request capturado.
func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("query inválida %q: %v", raw, err)
	}
	return values
}

// Regressão do incidente "Financeiro no portal do cliente" (2026-09-19):
// a tela respondia 403 porque a chamada interna de cobranças saía SEM
// `tenant_id`; o toca-financial resolve o tenant pela credencial
// (`auth.ResolveTenantForInternalRequest`, achado A3) e devolvia 400 — que o
// handler do portal traduzia como "acesso negado".
//
// Este teste trava o contrato do lado do cliente: o tenant vai na query, os
// demais parâmetros não se perdem e a chamada nem sai quando o tenant falta.
func TestFinancialListChargesByCustomerCarriesTenant(t *testing.T) {
	base := NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"

	var gotPath, gotQuery string
	base.HTTPClient.Transport = visitTransport(func(r *http.Request) (*http.Response, error) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		if r.Header.Get("X-Internal-Key") != "test-key" {
			t.Fatal("internal key ausente")
		}
		return jsonResponse(200, `{"data":[{"id":"charge-1","status":"PENDING"}]}`), nil
	})

	charges, err := (&FinancialClient{BaseClient: base}).ListChargesByCustomer(
		context.Background(), "tenant-1", "deal-1", "account-1", "locatario")
	if err != nil {
		t.Fatalf("list charges: %v", err)
	}
	if len(charges) != 1 || charges[0].ID != "charge-1" {
		t.Fatalf("charges = %+v", charges)
	}
	if gotPath != "/api/v1/financial/internal/deals/deal-1/charges" {
		t.Fatalf("path = %q", gotPath)
	}
	query := parseQuery(t, gotQuery)
	for key, want := range map[string]string{
		"tenant_id":  "tenant-1",
		"account_id": "account-1",
		"role":       "locatario",
	} {
		if query.Get(key) != want {
			t.Fatalf("%s = %q, want %q (query=%s)", key, query.Get(key), want, gotQuery)
		}
	}
}

func TestFinancialListChargesByDealCarriesTenant(t *testing.T) {
	base := NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	var gotQuery string
	base.HTTPClient.Transport = visitTransport(func(r *http.Request) (*http.Response, error) {
		gotQuery = r.URL.RawQuery
		return jsonResponse(200, `{"data":[]}`), nil
	})

	if _, err := (&FinancialClient{BaseClient: base}).ListChargesByDeal(context.Background(), "tenant-1", "deal-1"); err != nil {
		t.Fatalf("list charges: %v", err)
	}
	if parseQuery(t, gotQuery).Get("tenant_id") != "tenant-1" {
		t.Fatalf("tenant_id ausente: %s", gotQuery)
	}
}

func TestFinancialInternalChargesRefuseMissingTenantBeforeNetwork(t *testing.T) {
	base := NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	called := false
	base.HTTPClient.Transport = visitTransport(func(*http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(200, `{"data":[]}`), nil
	})
	client := &FinancialClient{BaseClient: base}

	if _, err := client.ListChargesByCustomer(context.Background(), "  ", "deal-1", "account-1", "locatario"); !errors.Is(err, ErrFinancialTenantRequired) {
		t.Fatalf("err = %v, want ErrFinancialTenantRequired", err)
	}
	if _, err := client.ListChargesByDeal(context.Background(), "", "deal-1"); !errors.Is(err, ErrFinancialTenantRequired) {
		t.Fatalf("err = %v, want ErrFinancialTenantRequired", err)
	}
	if called {
		t.Fatal("chamada saiu sem tenant: o serviço responderia 400 e o portal viraria 403")
	}
}

// TestFinancialSalePlanReadiness trava o contrato do endpoint de readiness
// (T-01): path, tenant na query, header interno e decode do payload.
func TestFinancialSalePlanReadiness(t *testing.T) {
	base := NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"

	var gotPath, gotQuery string
	base.HTTPClient.Transport = visitTransport(func(r *http.Request) (*http.Response, error) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		if r.Header.Get("X-Internal-Key") != "test-key" {
			t.Fatal("internal key ausente")
		}
		return jsonResponse(200, `{"message":"ok","data":{"active":true,"total_amount_cents":100000,"mode":"INSTALLMENTS"}}`), nil
	})

	readiness, err := (&FinancialClient{BaseClient: base}).GetSalePlanReadiness(
		context.Background(), "tenant-1", "deal-1")
	if err != nil {
		t.Fatalf("get sale plan readiness: %v", err)
	}
	if gotPath != "/api/v1/financial/internal/deals/deal-1/sale-plan-readiness" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotQuery != "tenant_id=tenant-1" {
		t.Fatalf("query = %q", gotQuery)
	}
	if !readiness.Active || readiness.TotalAmountCents != 100000 || readiness.Mode != "INSTALLMENTS" {
		t.Fatalf("readiness = %+v", readiness)
	}
}

// TestFinancialSalePlanReadinessRequiresTenant: sem tenant a chamada nem sai.
func TestFinancialSalePlanReadinessRequiresTenant(t *testing.T) {
	base := NewBaseClient("http://financial.test")
	base.InternalAPIKey = "test-key"
	called := false
	base.HTTPClient.Transport = visitTransport(func(*http.Request) (*http.Response, error) {
		called = true
		return jsonResponse(200, `{"data":{"active":false}}`), nil
	})
	client := &FinancialClient{BaseClient: base}

	if _, err := client.GetSalePlanReadiness(context.Background(), " ", "deal-1"); !errors.Is(err, ErrFinancialTenantRequired) {
		t.Fatalf("err = %v, want ErrFinancialTenantRequired", err)
	}
	if called {
		t.Fatal("chamada saiu sem tenant")
	}
}
