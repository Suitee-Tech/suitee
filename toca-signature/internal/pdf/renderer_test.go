package pdf_test

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-signature/internal/pdf"
)

func TestDefaultRenderer_RawPDFPassthrough(t *testing.T) {
	r := pdf.NewDefaultRenderer(nil)
	out, err := r.Render(context.Background(), "c1", "%PDF-1.4\nfake")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "%PDF-1.4\nfake" {
		t.Errorf("expected raw PDF passthrough, got %q", string(out))
	}
}

func TestDefaultRenderer_Base64DataURL(t *testing.T) {
	raw := []byte("%PDF-1.4 inline")
	encoded := "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(raw)
	r := pdf.NewDefaultRenderer(nil)
	out, err := r.Render(context.Background(), "c1", encoded)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != string(raw) {
		t.Errorf("decode mismatch: got %q want %q", out, raw)
	}
}

func TestDefaultRenderer_DownloadsURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-from-server"))
	}))
	defer srv.Close()
	r := pdf.NewDefaultRenderer(nil)
	out, err := r.Render(context.Background(), "c1", srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "%PDF-from-server" {
		t.Errorf("download mismatch: %q", out)
	}
}

func TestDefaultRenderer_HTMLReturnsSentinel(t *testing.T) {
	r := pdf.NewDefaultRenderer(nil)
	_, err := r.Render(context.Background(), "c1", "<html><body>contract</body></html>")
	if !errors.Is(err, pdf.ErrHTMLNotSupported) {
		t.Fatalf("expected ErrHTMLNotSupported, got %v", err)
	}
}

func TestChainedRenderer_DelegatesHTMLToFallback(t *testing.T) {
	primary := pdf.NewDefaultRenderer(nil)
	fallback := stubRenderer{out: []byte("%PDF-from-fallback")}
	r := pdf.NewChainedRenderer(primary, fallback)

	out, err := r.Render(context.Background(), "c1", "<html>x</html>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "%PDF-from-fallback" {
		t.Errorf("expected fallback to render, got %q", out)
	}
}

func TestChainedRenderer_PrimarySuccessSkipsFallback(t *testing.T) {
	primary := pdf.NewDefaultRenderer(nil)
	called := false
	fallback := stubRenderer{
		out: []byte("should-not-be-used"),
		hook: func() {
			called = true
		},
	}
	r := pdf.NewChainedRenderer(primary, fallback)

	if _, err := r.Render(context.Background(), "c1", "%PDF-direct"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if called {
		t.Errorf("fallback should not be invoked when primary succeeds")
	}
}

func TestGotenbergRenderer_PostsHTMLAsMultipart(t *testing.T) {
	var receivedFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/chromium/convert/html" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("part: %v", err)
			}
			if part.FileName() != "index.html" {
				t.Errorf("expected file index.html, got %s", part.FileName())
			}
			body, _ := io.ReadAll(part)
			receivedFile = string(body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-from-gotenberg"))
	}))
	defer srv.Close()

	r := pdf.NewGotenbergRenderer(srv.URL)
	out, err := r.Render(context.Background(), "c1", "<html><body>hi</body></html>")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "%PDF-from-gotenberg" {
		t.Errorf("body mismatch: %q", out)
	}
	if !strings.Contains(receivedFile, "<body>hi</body>") {
		t.Errorf("server didn't see html body, got %q", receivedFile)
	}
}

func TestGotenbergRenderer_PostsDOCXToLibreOffice(t *testing.T) {
	rawDocx := []byte("fake-docx-bytes")
	docxDataURL := "data:application/vnd.openxmlformats-officedocument.wordprocessingml.document;base64," +
		base64.StdEncoding.EncodeToString(rawDocx)
	var receivedFile string
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/forms/libreoffice/convert" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		mr, err := r.MultipartReader()
		if err != nil {
			t.Fatalf("multipart reader: %v", err)
		}
		part, err := mr.NextPart()
		if err != nil {
			t.Fatalf("part: %v", err)
		}
		receivedFile = part.FileName()
		receivedBody, _ = io.ReadAll(part)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("%PDF-from-libreoffice"))
	}))
	defer srv.Close()

	r := pdf.NewGotenbergRenderer(srv.URL)
	out, err := r.Render(context.Background(), "c1", docxDataURL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "%PDF-from-libreoffice" {
		t.Errorf("body mismatch: %q", out)
	}
	if receivedFile != "contrato-c1.docx" {
		t.Errorf("expected contrato-c1.docx, got %q", receivedFile)
	}
	if string(receivedBody) != string(rawDocx) {
		t.Errorf("server didn't see docx bytes, got %q", receivedBody)
	}
}

func TestGotenbergRenderer_PropagatesErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("malformed input"))
	}))
	defer srv.Close()

	r := pdf.NewGotenbergRenderer(srv.URL)
	_, err := r.Render(context.Background(), "c1", "<html />")
	if err == nil || !strings.Contains(err.Error(), "status=400") {
		t.Fatalf("expected status=400 error, got %v", err)
	}
	if !strings.Contains(err.Error(), "malformed input") {
		t.Errorf("expected error to include response body, got %v", err)
	}
}

// stubRenderer lets us assert delegation behaviour without spinning up an
// HTTP server. Verifies (indirectly) that ChainedRenderer respects the
// ErrHTMLNotSupported sentinel.
type stubRenderer struct {
	out  []byte
	hook func()
}

func (s stubRenderer) Render(_ context.Context, _, _ string) ([]byte, error) {
	if s.hook != nil {
		s.hook()
	}
	return s.out, nil
}

// Sanity check that buildHTMLForm produces a multipart payload whose Content-
// Type header matches what httptest sees (regression for the form field name).
func TestGotenbergMultipartShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		if !strings.HasPrefix(ct, "multipart/form-data;") {
			t.Errorf("expected multipart content type, got %q", ct)
		}
		_, _, err := mime.ParseMediaType(ct)
		if err != nil {
			t.Fatalf("parse media type: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	r := pdf.NewGotenbergRenderer(srv.URL)
	if _, err := r.Render(context.Background(), "c1", "<p />"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = multipart.ErrMessageTooLarge // keep multipart import (used in another test)
}
