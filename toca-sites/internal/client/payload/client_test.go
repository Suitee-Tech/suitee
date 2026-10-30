package payload

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

const testTenant = "11111111-1111-4111-8111-111111111111"

func testSite() *site.Site {
	return &site.Site{
		ID: "site-1", TenantID: testTenant, Slug: "exemplo", Name: "Imobiliária Exemplo", Enabled: true,
		Branding: site.Branding{PrimaryColor: "#111111"},
	}
}

func TestEnsureTenantCreatesWhenMissing(t *testing.T) {
	var created map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/tenants":
			if r.URL.Query().Get("where[suiteeTenantId][equals]") != testTenant {
				t.Errorf("where sem tenant: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"docs":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/tenants":
			if r.Header.Get("X-Internal-Key") != "secret" || r.Header.Get("X-Tenant-Id") != testTenant {
				t.Errorf("headers internos ausentes")
			}
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"doc":{"id":"tenant-doc-1"}}`))
		default:
			t.Errorf("requisição inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	id, err := New(srv.URL, "secret", "imoveis.suitee.com.br").EnsureTenant(context.Background(), testSite())
	if err != nil {
		t.Fatalf("EnsureTenant: %v", err)
	}
	if id != "tenant-doc-1" {
		t.Fatalf("id=%q", id)
	}
	if created["slug"] != "exemplo" || created["name"] != "Imobiliária Exemplo" {
		t.Fatalf("body: %+v", created)
	}
	domains := created["domains"].([]interface{})
	if domains[0].(map[string]interface{})["domain"] != "exemplo.imoveis.suitee.com.br" {
		t.Fatalf("domains: %+v", domains)
	}
}

func TestEnsureTenantPatchesExisting(t *testing.T) {
	var patched bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"docs":[{"id":42}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/tenants/42":
			patched = true
			_, _ = w.Write([]byte(`{"doc":{"id":42}}`))
		default:
			t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	id, err := New(srv.URL, "secret", "imoveis.suitee.com.br").EnsureTenant(context.Background(), testSite())
	if err != nil || id != "42" || !patched {
		t.Fatalf("id=%q patched=%v err=%v", id, patched, err)
	}
}

func TestUpsertListingCreatesWithProjectionContract(t *testing.T) {
	var posted map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/listings":
			_, _ = w.Write([]byte(`{"docs":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/listings":
			_ = json.NewDecoder(r.Body).Decode(&posted)
			_, _ = w.Write([]byte(`{"doc":{"id":"listing-1"}}`))
		default:
			t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	published := time.Now().UTC()
	s := testSite()
	s.PayloadTenantID = "tenant-doc-1"
	l := &site.Listing{
		TenantID: testTenant, SiteID: "site-1", PropertyID: "prop-1",
		SeoSlug: "apartamento-centro", Status: site.ListingPublished,
		Title: "Apartamento", ListingType: "sale", PropertyType: "apartment",
		SalePrice: 350000, Features: site.Features{Bedrooms: ptr(3)},
		Images:      []site.Image{{URL: "https://cdn/1.jpg", Description: "Sala"}},
		Address:     site.Address{City: "Juazeiro", State: "CE", PostalCode: "63000-000"},
		PublishedAt: &published,
	}
	id, err := New(srv.URL, "secret", "imoveis.suitee.com.br").UpsertListing(context.Background(), s, l)
	if err != nil {
		t.Fatalf("UpsertListing: %v", err)
	}
	if id != "listing-1" {
		t.Fatalf("id=%q", id)
	}
	if posted["tenant"] != "tenant-doc-1" || posted["seoSlug"] != "apartamento-centro" {
		t.Fatalf("body: %+v", posted)
	}
	if posted["status"] != "published" {
		t.Fatalf("status=%v", posted["status"])
	}
	images := posted["images"].([]interface{})
	img := images[0].(map[string]interface{})
	if img["url"] != "https://cdn/1.jpg" || img["alt"] != "Sala" {
		t.Fatalf("images: %+v", images)
	}
	address := posted["address"].(map[string]interface{})
	if address["postalCode"] != "63000-000" {
		t.Fatalf("address: %+v", address)
	}
}

func TestUpsertListingPatchesKnownDocID(t *testing.T) {
	var patched bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch && r.URL.Path == "/api/listings/doc-9" {
			patched = true
			_, _ = w.Write([]byte(`{"doc":{"id":"doc-9"}}`))
			return
		}
		t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	s := testSite()
	s.PayloadTenantID = "tenant-doc-1"
	l := &site.Listing{TenantID: testTenant, PropertyID: "prop-1", PayloadDocID: "doc-9", SeoSlug: "x", Status: site.ListingPublished}
	id, err := New(srv.URL, "secret", "imoveis.suitee.com.br").UpsertListing(context.Background(), s, l)
	if err != nil || id != "doc-9" || !patched {
		t.Fatalf("id=%q patched=%v err=%v", id, patched, err)
	}
}

func TestUnpublishListingSendsStatusOnly(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write([]byte(`{"doc":{"id":"doc-9"}}`))
	}))
	defer srv.Close()

	s := testSite()
	s.PayloadTenantID = "tenant-doc-1"
	err := New(srv.URL, "secret", "imoveis.suitee.com.br").UnpublishListing(context.Background(), s,
		&site.Listing{PayloadDocID: "doc-9", TenantID: testTenant})
	if err != nil {
		t.Fatal(err)
	}
	if body["status"] != "unpublished" || len(body) != 1 {
		t.Fatalf("body: %+v", body)
	}
}

func TestRevalidateSendsInternalKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/revalidate" || r.Header.Get("X-Internal-Key") != "secret" {
			t.Errorf("revalidate: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"revalidated":true}`))
	}))
	defer srv.Close()

	err := New(srv.URL, "secret", "imoveis.suitee.com.br").Revalidate(context.Background(), "exemplo", []string{"/imoveis"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpsertPageUsesPlainTextBody(t *testing.T) {
	var body map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/pages":
			_, _ = w.Write([]byte(`{"docs":[{"id":"page-1"}]}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/pages/page-1":
			_ = json.NewDecoder(r.Body).Decode(&body)
			_, _ = w.Write([]byte(`{"doc":{"id":"page-1"}}`))
		default:
			t.Errorf("inesperado: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	s := testSite()
	s.PayloadTenantID = "tenant-doc-1"
	err := New(srv.URL, "secret", "imoveis.suitee.com.br").UpsertPage(context.Background(), s, site.Page{
		Key: site.PageAbout, Title: "Sobre", Body: json.RawMessage(`"texto simples"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["pageKey"] != "about" || body["body"] != "texto simples" {
		t.Fatalf("body: %+v", body)
	}
	seo, ok := body["seo"].(map[string]interface{})
	if !ok || seo["metaTitle"] != "" || seo["metaDescription"] != "" {
		t.Fatalf("SEO vazio precisa ser enviado para limpar o Payload: %+v", body["seo"])
	}
}

func TestNoopDoesNothing(t *testing.T) {
	var p site.Projection = Noop{}
	if id, err := p.EnsureTenant(context.Background(), testSite()); id != "" || err != nil {
		t.Fatal("noop")
	}
}

func ptr[T any](v T) *T { return &v }
