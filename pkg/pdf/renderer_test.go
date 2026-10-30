package pdf

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoopRenderer(t *testing.T) {
	r := NoopRenderer{}
	if r.Available() {
		t.Error("NoopRenderer should report Available()=false")
	}
	_, err := r.RenderHTML(context.Background(), "trace", "<html></html>")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestNewRendererFromEnv_NoURL(t *testing.T) {
	t.Setenv("GOTENBERG_URL", "")
	r := NewRendererFromEnv()
	if _, ok := r.(NoopRenderer); !ok {
		t.Errorf("expected NoopRenderer when GOTENBERG_URL unset, got %T", r)
	}
}

func TestNewRendererFromEnv_WithURL(t *testing.T) {
	t.Setenv("GOTENBERG_URL", "http://gotenberg:3000")
	r := NewRendererFromEnv()
	gr, ok := r.(*GotenbergRenderer)
	if !ok {
		t.Fatalf("expected *GotenbergRenderer, got %T", r)
	}
	if !gr.Available() {
		t.Error("GotenbergRenderer with URL should be Available()")
	}
}

func TestGotenbergRenderer_EmptyURL_NotConfigured(t *testing.T) {
	r := NewGotenbergRenderer("")
	if r.Available() {
		t.Error("empty URL should not be Available()")
	}
	_, err := r.RenderHTML(context.Background(), "t", "<html></html>")
	if !errors.Is(err, ErrNotConfigured) {
		t.Errorf("expected ErrNotConfigured, got %v", err)
	}
}

func TestGotenbergRenderer_Success(t *testing.T) {
	var receivedPath, receivedTrace, receivedFilename string
	var receivedHTML string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		receivedPath = req.URL.Path
		receivedTrace = req.Header.Get("Gotenberg-Trace")

		// Parse multipart, confirma que o arquivo se chama index.html
		// e o campo é "files" (contrato da API Chromium do Gotenberg).
		_, params, _ := mime.ParseMediaType(req.Header.Get("Content-Type"))
		mr := multipart.NewReader(req.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			if part.FormName() == "files" {
				receivedFilename = part.FileName()
				b, _ := io.ReadAll(part)
				receivedHTML = string(b)
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-1.7 fake pdf bytes"))
	}))
	defer server.Close()

	r := NewGotenbergRenderer(server.URL)
	pdf, err := r.RenderHTML(context.Background(), "laudo-abc", "<html><body>Laudo</body></html>")
	if err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	if !strings.HasPrefix(string(pdf), "%PDF") {
		t.Errorf("expected PDF bytes, got %q", string(pdf))
	}
	if receivedPath != "/forms/chromium/convert/html" {
		t.Errorf("path = %q, want /forms/chromium/convert/html", receivedPath)
	}
	if receivedTrace != "laudo-abc" {
		t.Errorf("trace header = %q, want laudo-abc", receivedTrace)
	}
	if receivedFilename != "index.html" {
		t.Errorf("filename = %q, want index.html", receivedFilename)
	}
	if !strings.Contains(receivedHTML, "Laudo") {
		t.Errorf("HTML não chegou no server: %q", receivedHTML)
	}
}

func TestGotenbergRenderer_Non200_ReturnsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid html"))
	}))
	defer server.Close()

	r := NewGotenbergRenderer(server.URL)
	_, err := r.RenderHTML(context.Background(), "t", "<html></html>")
	if err == nil {
		t.Fatal("expected error on non-200")
	}
	if !strings.Contains(err.Error(), "status=400") {
		t.Errorf("error should mention status, got %v", err)
	}
}

func TestGotenbergRenderer_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	server.Close() // fecha já — conexão vai falhar

	r := NewGotenbergRenderer(server.URL)
	_, err := r.RenderHTML(context.Background(), "t", "<html></html>")
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "gotenberg request") {
		t.Errorf("error should wrap request failure, got %v", err)
	}
}

func TestGotenbergRenderer_TrimsTrailingSlash(t *testing.T) {
	r := NewGotenbergRenderer("http://gotenberg:3000/")
	if r.baseURL != "http://gotenberg:3000" {
		t.Errorf("baseURL = %q, want sem barra final", r.baseURL)
	}
}

func TestBuildHTMLForm(t *testing.T) {
	body, contentType, err := buildHTMLForm("<html><body>x</body></html>")
	if err != nil {
		t.Fatalf("buildHTMLForm: %v", err)
	}
	if !strings.HasPrefix(contentType, "multipart/form-data") {
		t.Errorf("content-type = %q", contentType)
	}
	if len(body) == 0 {
		t.Error("body vazio")
	}
	// Confirma que o boundary + filename estão no corpo.
	if !strings.Contains(string(body), "index.html") {
		t.Error("form não contém index.html")
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("short string: %q", got)
	}
	if got := truncate("abcdefghij", 5); got != "abcde..." {
		t.Errorf("long string: %q", got)
	}
}
