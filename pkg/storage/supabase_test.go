package storage

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSupabaseStorageUsesPublicURLForBrowserFacingObjects(t *testing.T) {
	t.Parallel()

	svc, err := NewSupabaseStorage(SupabaseConfig{
		URL:            "http://supabase.internal:54321/",
		PublicURL:      "https://files.example.test/",
		ServiceRoleKey: "service-key",
	})
	if err != nil {
		t.Fatalf("NewSupabaseStorage() error = %v", err)
	}

	publicURL := svc.GetURL("toca-property-images", "tenant one/photo 1.png")
	wantPublic := "https://files.example.test/storage/v1/object/public/toca-property-images/tenant%20one/photo%201.png"
	if publicURL != wantPublic {
		t.Errorf("GetURL(public) = %q, want %q", publicURL, wantPublic)
	}

	privateURL := svc.GetURL("toca-documents", "tenant one/contract.pdf")
	wantPrivate := "https://files.example.test/storage/v1/object/authenticated/toca-documents/tenant%20one/contract.pdf"
	if privateURL != wantPrivate {
		t.Errorf("GetURL(private) = %q, want %q", privateURL, wantPrivate)
	}
	if !svc.IsPrivate(FinancialProofsBucket) {
		t.Errorf("%s must be private", FinancialProofsBucket)
	}
}

func TestSupabaseStorageInfersLocalhostForDockerInternalURL(t *testing.T) {
	t.Parallel()

	svc, err := NewSupabaseStorage(SupabaseConfig{
		URL:            "http://host.docker.internal:54321/",
		ServiceRoleKey: "service-key",
	})
	if err != nil {
		t.Fatalf("NewSupabaseStorage() error = %v", err)
	}

	got := svc.GetURL("toca-property-images", "photo.png")
	want := "http://localhost:54321/storage/v1/object/public/toca-property-images/photo.png"
	if got != want {
		t.Errorf("GetURL() = %q, want %q", got, want)
	}
}

func TestSupabaseStoragePresignedURLUsesInternalAPIAndPublicOrigin(t *testing.T) {
	t.Parallel()

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/storage/v1/object/sign/toca-documents/tenant/contract%201.pdf" {
			t.Errorf("request path = %q", r.URL.EscapedPath())
		}
		if got := r.Header.Get("Authorization"); got != "Bearer service-key" {
			t.Errorf("Authorization = %q", got)
		}
		var payload struct {
			ExpiresIn int `json:"expiresIn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		if payload.ExpiresIn != 900 {
			t.Errorf("expiresIn = %d, want 900", payload.ExpiresIn)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"signedURL": "http://127.0.0.1:54321/storage/v1/object/sign/toca-documents/tenant/contract%201.pdf?token=abc",
		})
	}))
	defer server.Close()

	svc, err := NewSupabaseStorage(SupabaseConfig{
		URL:            server.URL + "/",
		PublicURL:      "https://files.example.test/proxy/",
		ServiceRoleKey: "service-key",
	})
	if err != nil {
		t.Fatalf("NewSupabaseStorage() error = %v", err)
	}

	got, err := svc.GetPresignedURL(
		context.Background(),
		"toca-documents",
		"tenant/contract 1.pdf",
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("GetPresignedURL() error = %v", err)
	}
	want := "https://files.example.test/proxy/storage/v1/object/sign/toca-documents/tenant/contract%201.pdf?token=abc"
	if got != want {
		t.Errorf("GetPresignedURL() = %q, want %q", got, want)
	}

	resolver, ok := svc.(interface {
		ResolveInternalURL(string) (string, error)
	})
	if !ok {
		t.Fatal("Supabase storage should expose internal URL resolution")
	}
	internalURL, err := resolver.ResolveInternalURL(got)
	if err != nil {
		t.Fatalf("ResolveInternalURL() error = %v", err)
	}
	wantInternal := server.URL + "/storage/v1/object/sign/toca-documents/tenant/contract%201.pdf?token=abc"
	if internalURL != wantInternal {
		t.Errorf("ResolveInternalURL() = %q, want %q", internalURL, wantInternal)
	}
}

func TestSupabaseStoragePresignedPutURLUsesPublicOrigin(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/storage/v1/object/upload/sign/") {
			t.Errorf("request path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{
			"url": "/object/upload/sign/toca-documents/tenant/contract.pdf?token=upload",
		})
	}))
	defer server.Close()

	svc, err := NewSupabaseStorage(SupabaseConfig{
		URL:            server.URL,
		PublicURL:      "https://files.example.test/proxy",
		ServiceRoleKey: "service-key",
	})
	if err != nil {
		t.Fatalf("NewSupabaseStorage() error = %v", err)
	}

	got, err := svc.GetPresignedPutURL(
		context.Background(),
		"toca-documents",
		"tenant/contract.pdf",
		5*time.Minute,
	)
	if err != nil {
		t.Fatalf("GetPresignedPutURL() error = %v", err)
	}
	want := "https://files.example.test/proxy/storage/v1/object/upload/sign/toca-documents/tenant/contract.pdf?token=upload"
	if got != want {
		t.Errorf("GetPresignedPutURL() = %q, want %q", got, want)
	}
}

func TestSupabaseStorageRejectsInvalidAndEmptySignedURLs(t *testing.T) {
	t.Parallel()

	if _, err := NewSupabaseStorage(SupabaseConfig{
		URL:            "supabase.invalid",
		ServiceRoleKey: "service-key",
	}); err == nil {
		t.Fatal("NewSupabaseStorage() should reject a URL without scheme")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"signedURL":""}`))
	}))
	defer server.Close()

	svc, err := NewSupabaseStorage(SupabaseConfig{
		URL:            server.URL,
		ServiceRoleKey: "service-key",
	})
	if err != nil {
		t.Fatalf("NewSupabaseStorage() error = %v", err)
	}
	if _, err := svc.GetPresignedURL(
		context.Background(),
		"toca-documents",
		"contract.pdf",
		time.Minute,
	); err == nil {
		t.Fatal("GetPresignedURL() should reject an empty response URL")
	}
}
