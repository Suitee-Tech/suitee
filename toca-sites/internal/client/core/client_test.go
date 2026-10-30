package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

const (
	testTenant   = "11111111-1111-4111-8111-111111111111"
	testProperty = "22222222-2222-4222-8222-222222222222"
)

func TestGetPropertyMapsCorePayload(t *testing.T) {
	var sawKey, sawTenant, sawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawKey = r.Header.Get("X-Internal-Key")
		sawTenant = r.Header.Get("X-Tenant-Id")
		sawQuery = r.URL.Query().Get("tenant_id")
		if r.URL.Path != "/api/v1/core/internal/properties/"+testProperty {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": map[string]interface{}{
				"id":                 testProperty,
				"title":              "Apartamento Centro",
				"description":        "Sol da manhã",
				"status":             "published",
				"publication_status": "published",
				"property_type":      "apartment",
				"listing_type":       "sale",
				"address": map[string]string{
					"street": "Rua A", "number": "10", "neighborhood": "Centro",
					"city": "Juazeiro", "state": "Ceará", "stateCode": "CE", "postalCode": "63000-000",
				},
				"pricing": map[string]interface{}{
					"sale_price":      map[string]interface{}{"current_amount": 350000.5, "currency": "BRL"},
					"condominium_fee": map[string]interface{}{"current_amount": 450, "currency": "BRL"},
				},
				"areas":     map[string]interface{}{"total_area_m2": 95, "built_area_m2": 80},
				"features":  map[string]interface{}{"bedrooms": 3, "bathrooms": 2, "parking_spaces": 1},
				"amenities": []string{"Piscina"},
				"media": map[string]interface{}{
					"images": []map[string]interface{}{{"url": "https://cdn/1.jpg", "description": "Sala", "order": 1}},
				},
			},
		})
	}))
	defer srv.Close()

	p, err := New(srv.URL, "secret").GetProperty(context.Background(), testTenant, testProperty)
	if err != nil {
		t.Fatalf("GetProperty: %v", err)
	}
	if sawKey != "secret" || sawTenant != testTenant || sawQuery != testTenant {
		t.Fatalf("credenciais/tenant: key=%q tenant=%q query=%q", sawKey, sawTenant, sawQuery)
	}
	if p.Title != "Apartamento Centro" || p.SalePrice != 350000.5 || p.CondoFee != 450 {
		t.Fatalf("snapshot: %+v", p)
	}
	if p.Address.State != "CE" {
		t.Fatalf("UF deve preferir stateCode: %q", p.Address.State)
	}
	if p.Features.Bedrooms == nil || *p.Features.Bedrooms != 3 {
		t.Fatalf("features: %+v", p.Features)
	}
	if len(p.Images) != 1 || p.Images[0].URL != "https://cdn/1.jpg" {
		t.Fatalf("imagens: %+v", p.Images)
	}
	if p.AreaTotal != 95 || p.AreaBuilt != 80 {
		t.Fatalf("áreas: total=%v built=%v", p.AreaTotal, p.AreaBuilt)
	}
	if !p.IsPublishableOnSite() {
		t.Fatal("imóvel publicado/publicado deveria ser publicável")
	}
}

func TestGetPropertyNotFoundIsSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "secret").GetProperty(context.Background(), testTenant, testProperty)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("esperava ErrNotFound, veio %v", err)
	}
}

func TestGetPropertyRequiresTenantAndBaseURL(t *testing.T) {
	if _, err := New("", "k").GetProperty(context.Background(), testTenant, testProperty); err == nil {
		t.Fatal("sem CORE_SERVICE_URL deve falhar")
	}
	if _, err := New("http://localhost", "k").GetProperty(context.Background(), "", testProperty); err == nil {
		t.Fatal("sem tenant deve falhar")
	}
}

func TestListPropertiesPaginatesAndMapsSnapshots(t *testing.T) {
	var gotLimit, gotOffset, gotTenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/core/internal/properties" {
			t.Errorf("path=%s", r.URL.Path)
		}
		gotLimit = r.URL.Query().Get("limit")
		gotOffset = r.URL.Query().Get("offset")
		gotTenant = r.URL.Query().Get("tenant_id")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{
					"id": testProperty, "title": "Casa", "status": "published",
					"publication_status": "published", "property_type": "house", "listing_type": "sale",
					"pricing": map[string]interface{}{"sale_price": map[string]interface{}{"current_amount": 500000}},
				},
			},
			"meta": map[string]interface{}{"total": 3},
		})
	}))
	defer srv.Close()

	list, total, err := New(srv.URL, "secret").ListProperties(context.Background(), testTenant, 50, 100)
	if err != nil {
		t.Fatalf("ListProperties: %v", err)
	}
	if total != 3 || len(list) != 1 || list[0].ID != testProperty || list[0].SalePrice != 500000 {
		t.Fatalf("resposta: total=%d list=%+v", total, list)
	}
	if gotLimit != "50" || gotOffset != "100" || gotTenant != testTenant {
		t.Fatalf("paginação/tenant: limit=%s offset=%s tenant=%s", gotLimit, gotOffset, gotTenant)
	}
}

func TestListPropertiesClampsPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "100" || r.URL.Query().Get("offset") != "0" {
			t.Errorf("clamp: %s", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(`{"data":[],"meta":{"total":0}}`))
	}))
	defer srv.Close()

	if _, _, err := New(srv.URL, "secret").ListProperties(context.Background(), testTenant, 9999, -5); err != nil {
		t.Fatal(err)
	}
}
