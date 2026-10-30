package clients

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// O fuso da imobiliária é lido da rota INTERNA do core com a credencial de
// serviço (sem JWT de usuário) e fica em cache — é configuração, não muda por
// requisição.
func TestGetTenantTimezoneUsesInternalRouteAndCaches(t *testing.T) {
	t.Setenv("INTERNAL_API_KEY", "chave-interna-de-teste")
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/core/internal/tenant-settings" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("tenant_id"); got != "tenant-1" {
			t.Errorf("tenant_id = %q, want tenant-1", got)
		}
		if got := r.Header.Get("X-Internal-Key"); got != "chave-interna-de-teste" {
			t.Errorf("X-Internal-Key = %q", got)
		}
		if got := r.Header.Get("X-Tenant-Id"); got != "tenant-1" {
			t.Errorf("X-Tenant-Id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tenant_id":"tenant-1","timezone":"America/Manaus"}`))
	}))
	defer server.Close()

	c := &CoreClient{BaseURL: server.URL, HTTPClient: server.Client()}
	InvalidateTenantTimezoneCache()
	t.Cleanup(func() { InvalidateTenantTimezoneCache() })

	got, err := c.GetTenantTimezone(context.Background(), "tenant-1")
	if err != nil {
		t.Fatalf("GetTenantTimezone: %v", err)
	}
	if got != "America/Manaus" {
		t.Errorf("timezone = %q, want America/Manaus", got)
	}
	if _, err := c.GetTenantTimezone(context.Background(), "tenant-1"); err != nil {
		t.Fatalf("segunda leitura: %v", err)
	}
	if calls != 1 {
		t.Errorf("chamadas HTTP = %d, want 1 (cache por tenant)", calls)
	}
}

// O adaptador do timeutil devolve a localização do tenant e, quando o core
// falha, diz "não configurado" para o fluxo cair no fuso do ambiente em vez de
// quebrar.
func TestTenantLocationResolverFallsBackOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	c := &CoreClient{BaseURL: server.URL, HTTPClient: server.Client()}
	InvalidateTenantTimezoneCache()
	t.Cleanup(func() { InvalidateTenantTimezoneCache() })

	if loc, ok := c.TenantLocationResolver()(context.Background(), "tenant-erro"); ok || loc != nil {
		t.Fatalf("loc=%v ok=%v, want fallback (nil,false)", loc, ok)
	}

	// E com o fuso configurado, devolve a localização certa.
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"timezone":"America/Belem"}`))
	}))
	defer ok.Close()
	cOk := &CoreClient{BaseURL: ok.URL, HTTPClient: ok.Client()}
	loc, found := cOk.TenantLocationResolver()(context.Background(), "tenant-ok")
	if !found || loc == nil {
		t.Fatal("esperava a localização configurada")
	}
	_, offset := time.Now().In(loc).Zone()
	if offset != -3*60*60 {
		t.Errorf("offset = %d, want -10800", offset)
	}
}
