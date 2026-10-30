package clicksign_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/oginoino/toca/toca-signature/internal/clicksign"
	"github.com/oginoino/toca/toca-signature/internal/domain/credential"
)

// stubCreds returns fixed credentials pointing at the given base URL. AuthScheme
// is controllable per-test so we can exercise both "bearer" and "raw" paths.
type stubCreds struct {
	baseURL string
	token   string
	scheme  string
}

func (s stubCreds) Get(_ context.Context, _ string) (*credential.Credentials, error) {
	return &credential.Credentials{
		BaseURL:     s.baseURL,
		AccessToken: s.token,
		Environment: "sandbox",
		AuthScheme:  s.scheme,
	}, nil
}

func newTestClient(t *testing.T, server *httptest.Server, scheme string) *clicksign.Client {
	t.Helper()
	return clicksign.NewClient(stubCreds{
		baseURL: server.URL,
		token:   "TOKEN-123",
		scheme:  scheme,
	})
}

// captureHeader runs an arbitrary client call and records the Authorization
// header observed by the test server. Used by both auth-scheme tests so the
// scaffolding is shared.
func captureAuthHeader(t *testing.T, scheme string) string {
	t.Helper()
	var observed atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{"data":{"id":"abc","type":"envelopes","attributes":{"status":"running"}}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, scheme)
	if _, err := c.GetEnvelope(context.Background(), "tenant-1", "abc"); err != nil {
		t.Fatalf("GetEnvelope: %v", err)
	}
	val, _ := observed.Load().(string)
	return val
}

func TestClient_AuthorizationHeader_BearerScheme(t *testing.T) {
	got := captureAuthHeader(t, credential.AuthSchemeBearer)
	if got != "Bearer TOKEN-123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer TOKEN-123")
	}
}

func TestClient_AuthorizationHeader_RawScheme(t *testing.T) {
	got := captureAuthHeader(t, credential.AuthSchemeRaw)
	if got != "TOKEN-123" {
		t.Errorf("Authorization = %q, want %q (raw, no prefix)", got, "TOKEN-123")
	}
}

func TestClient_ParseError_DecodesJSONAPIErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"errors": [
				{"code":"invalid_signer","title":"signatário inválido","detail":"email duplicado","source":{"pointer":"/data/attributes/email"}}
			]
		}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	_, err := c.GetEnvelope(context.Background(), "tenant-1", "any")
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var apiErr *clicksign.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.Status != http.StatusUnprocessableEntity {
		t.Errorf("Status = %d, want 422", apiErr.Status)
	}
	if len(apiErr.Errors) != 1 {
		t.Fatalf("expected 1 error entry, got %d", len(apiErr.Errors))
	}
	first := apiErr.Errors[0]
	if first.Code != "invalid_signer" || first.Title != "signatário inválido" || first.Detail != "email duplicado" {
		t.Errorf("error fields not extracted: %+v", first)
	}
	if first.Source["pointer"] != "/data/attributes/email" {
		t.Errorf("source pointer not extracted: %+v", first.Source)
	}
}

func TestClient_ParseError_FallsBackToRawBody(t *testing.T) {
	// 4xx is preserved by the underlying retry-aware HTTP client; 5xx is
	// retried up to 3× and the body discarded, so we use 400 here.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<html>not jsonapi</html>`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	_, err := c.GetEnvelope(context.Background(), "tenant-1", "x")
	if err == nil {
		t.Fatal("expected error")
	}
	var apiErr *clicksign.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if !strings.Contains(apiErr.RawBody, "not jsonapi") {
		t.Errorf("RawBody = %q, want it to include the original body", apiErr.RawBody)
	}
	if len(apiErr.Errors) != 0 {
		t.Errorf("Errors should be empty when the body isn't JSON:API")
	}
}

func TestClient_GetEnvelope_DecodesEnvelopeResource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v3/envelopes/cs-env-42" {
			t.Errorf("path = %s, want /api/v3/envelopes/cs-env-42", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{"data":{"id":"cs-env-42","type":"envelopes","attributes":{"name":"Contrato CONT-001","status":"running"}}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	env, err := c.GetEnvelope(context.Background(), "tenant-1", "cs-env-42")
	if err != nil {
		t.Fatalf("GetEnvelope: %v", err)
	}
	if env.ID != "cs-env-42" || env.Attributes.Status != "running" {
		t.Errorf("envelope not decoded: %+v", env)
	}
}

func TestClient_CreateEnvelope_JSONAPIContract(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v3/envelopes" {
			t.Errorf("request = %s %s, want POST /api/v3/envelopes", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.api+json" {
			t.Errorf("Accept = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/vnd.api+json" {
			t.Errorf("Content-Type = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		data, _ := body["data"].(map[string]any)
		attributes, _ := data["attributes"].(map[string]any)
		if data["type"] != "envelopes" || attributes["name"] != "Contrato sandbox" {
			t.Errorf("unexpected JSON:API payload: %#v", body)
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{"data":{"id":"env-sandbox","type":"envelopes","attributes":{"name":"Contrato sandbox","status":"draft"}}}`))
	}))
	defer server.Close()

	client := newTestClient(t, server, credential.AuthSchemeBearer)
	envelope, err := client.CreateEnvelope(context.Background(), "tenant-1", "Contrato sandbox")
	if err != nil {
		t.Fatalf("CreateEnvelope: %v", err)
	}
	if envelope.ID != "env-sandbox" || envelope.Attributes.Status != "draft" {
		t.Fatalf("unexpected envelope: %#v", envelope)
	}
}

func TestClient_ResendNotification_EnvelopeWide(t *testing.T) {
	var observed atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Store(r.URL.Path)
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		data, _ := body["data"].(map[string]any)
		if data["type"] != "notifications" {
			t.Errorf("body type = %v, want notifications", data["type"])
		}
		if _, ok := data["attributes"].(map[string]any); !ok {
			t.Errorf("body attributes = %T, want object", data["attributes"])
		}
		w.Header().Set("Content-Type", "application/vnd.api+json")
		_, _ = w.Write([]byte(`{"data":{"id":"notif-1","type":"notifications"}}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	if err := c.ResendNotification(context.Background(), "tenant-1", "cs-env-42", ""); err != nil {
		t.Fatalf("ResendNotification: %v", err)
	}
	if got, _ := observed.Load().(string); got != "/api/v3/envelopes/cs-env-42/notifications" {
		t.Errorf("path = %s, want /api/v3/envelopes/cs-env-42/notifications", got)
	}
}

func TestClient_ResendNotification_PerSigner(t *testing.T) {
	var observed atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed.Store(r.URL.Path)
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	if err := c.ResendNotification(context.Background(), "tenant-1", "cs-env-42", "cs-sig-7"); err != nil {
		t.Fatalf("ResendNotification: %v", err)
	}
	if got, _ := observed.Load().(string); got != "/api/v3/envelopes/cs-env-42/signers/cs-sig-7/notifications" {
		t.Errorf("path = %s, want per-signer notifications path", got)
	}
}

// O cancelamento na v3 é por DOCUMENTO. O PATCH no envelope com
// status=canceled devolve 400 ("status deve estar em: draft, running") — foi o
// que o operador viu no beta em 2026-10-05, com o link de assinatura seguindo
// válido. Este teste fixa o caminho certo para a regressão não voltar.
func TestClient_CancelEnvelope_CancelsDocuments(t *testing.T) {
	var (
		envelopeGets int32
		docListGets  int32
		patches      []string
		patchBodies  []map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/envelopes/cs-env-42":
			atomic.AddInt32(&envelopeGets, 1)
			_, _ = w.Write([]byte(`{"data":{"id":"cs-env-42","type":"envelopes","attributes":{"status":"running"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/envelopes/cs-env-42/documents":
			atomic.AddInt32(&docListGets, 1)
			_, _ = w.Write([]byte(`{"data":[
				{"id":"doc-1","type":"documents","attributes":{"filename":"contrato.pdf","status":"running"}},
				{"id":"doc-2","type":"documents","attributes":{"filename":"procuracao.pdf","status":"running"}}
			]}`))
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/v3/envelopes/cs-env-42/documents/"):
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			patches = append(patches, r.URL.Path)
			patchBodies = append(patchBodies, body)
			_, _ = w.Write([]byte(`{"data":{"id":"doc-x","type":"documents","attributes":{"status":"canceled"}}}`))
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	if err := c.CancelEnvelope(context.Background(), "tenant-1", "cs-env-42"); err != nil {
		t.Fatalf("CancelEnvelope: %v", err)
	}
	if atomic.LoadInt32(&envelopeGets) != 1 {
		t.Errorf("GET envelope = %d, want 1 (para não repetir um cancelamento já feito)", envelopeGets)
	}
	if atomic.LoadInt32(&docListGets) != 1 {
		t.Errorf("GET documents = %d, want 1", docListGets)
	}
	want := []string{
		"/api/v3/envelopes/cs-env-42/documents/doc-1",
		"/api/v3/envelopes/cs-env-42/documents/doc-2",
	}
	if strings.Join(patches, ",") != strings.Join(want, ",") {
		t.Fatalf("PATCHes = %v, want %v (o cancelamento é por documento)", patches, want)
	}
	for i, body := range patchBodies {
		data, _ := body["data"].(map[string]any)
		if data["type"] != "documents" {
			t.Errorf("patch %d: type = %v, want documents", i, data["type"])
		}
		if data["id"] == "" || data["id"] == nil {
			t.Errorf("patch %d: id ausente (%v) — o PATCH exige o id no envelope JSON:API", i, data["id"])
		}
		attrs, _ := data["attributes"].(map[string]any)
		if attrs["status"] != "canceled" {
			t.Errorf("patch %d: status = %v, want canceled", i, attrs["status"])
		}
	}
}

// Envelope já cancelado no provedor não gera PATCH nenhum: o botão pode ser
// clicado de novo depois de um cancelamento que já valeu.
func TestClient_CancelEnvelope_SkipsTerminalEnvelope(t *testing.T) {
	var patches int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		if r.Method == http.MethodPatch {
			atomic.AddInt32(&patches, 1)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/envelopes/cs-env-42":
			_, _ = w.Write([]byte(`{"data":{"id":"cs-env-42","type":"envelopes","attributes":{"status":"canceled"}}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v3/envelopes/cs-env-42/documents":
			_, _ = w.Write([]byte(`{"data":[{"id":"doc-1","type":"documents","attributes":{"status":"canceled"}}]}`))
		default:
			t.Errorf("chamada inesperada: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	c := newTestClient(t, srv, credential.AuthSchemeBearer)
	if err := c.CancelEnvelope(context.Background(), "tenant-1", "cs-env-42"); err != nil {
		t.Fatalf("CancelEnvelope em envelope já cancelado devolveu erro: %v", err)
	}
	if got := atomic.LoadInt32(&patches); got != 0 {
		t.Errorf("PATCHes = %d, want 0", got)
	}
}
