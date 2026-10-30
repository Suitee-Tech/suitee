package crm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oginoino/toca/toca-sites/internal/domain/site"
)

const testTenant = "11111111-1111-4111-8111-111111111111"

func TestCreateLeadPostsInternalScopedPayload(t *testing.T) {
	var body map[string]interface{}
	var key, tenant string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get("X-Internal-Key")
		tenant = r.Header.Get("X-Tenant-Id")
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"crm-1"}}`))
	}))
	defer srv.Close()

	id, err := New(srv.URL, "secret").CreateLead(context.Background(), testTenant, site.Lead{
		Name: "Ana", Email: "ana@example.com", Phone: "88999990000",
		PropertyContext: &site.LeadPropertyContext{
			PropertyID: "prop-1", Title: "Casa", PropertyType: "house", ListingType: "sale",
			Neighborhood: "Centro", City: "Atibaia", State: "SP", SalePrice: 4200000,
		},
	})
	if err != nil {
		t.Fatalf("CreateLead: %v", err)
	}
	if id != "crm-1" {
		t.Fatalf("id=%q", id)
	}
	if key != "secret" || tenant != testTenant {
		t.Fatalf("headers: key=%q tenant=%q", key, tenant)
	}
	if body["source"] != "WEBSITE" {
		t.Fatalf("source=%v (o CRM só conhece o enum existente)", body["source"])
	}
	interest, ok := body["property_interest"].(map[string]interface{})
	if !ok || interest["type"] != "HOUSE" || interest["preferred_location"] != "Centro, Atibaia, SP" {
		t.Fatalf("property_interest=%v", body["property_interest"])
	}
	price, _ := interest["price_range"].(map[string]interface{})
	if price["max"] != float64(4200000) {
		t.Fatalf("price_range=%v", interest["price_range"])
	}
	if body["recommended_property_id"] != "prop-1" {
		t.Fatalf("recommended_property_id=%v", body["recommended_property_id"])
	}
	snapshot, _ := body["recommended_property_snapshot"].(map[string]interface{})
	if snapshot["title"] != "Casa" || snapshot["city"] != "Atibaia" {
		t.Fatalf("snapshot=%v", body["recommended_property_snapshot"])
	}
}

func TestCreateLeadClassifiesServerErrorAsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "secret").CreateLead(context.Background(), testTenant, site.Lead{Name: "Ana"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("5xx deveria ser ErrUnavailable, veio %v", err)
	}
}

func TestCreateLeadWithoutBaseURL(t *testing.T) {
	_, err := New("", "secret").CreateLead(context.Background(), testTenant, site.Lead{Name: "Ana"})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("sem CRM_URL deveria ser ErrUnavailable, veio %v", err)
	}
}
