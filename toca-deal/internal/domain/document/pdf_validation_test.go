package document

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestValidateSignedPDF_RejectsEmpty(t *testing.T) {
	if err := ValidateSignedPDF(nil); !errors.Is(err, ErrInvalidPDF) {
		t.Errorf("expected ErrInvalidPDF for nil, got %v", err)
	}
	if err := ValidateSignedPDF([]byte{}); !errors.Is(err, ErrInvalidPDF) {
		t.Errorf("expected ErrInvalidPDF for empty slice, got %v", err)
	}
}

func TestValidateSignedPDF_RejectsTooSmall(t *testing.T) {
	// 50 bytes of legitimate-looking PDF — passes magic + EOF but
	// fails the minimum-size floor.
	tiny := []byte("%PDF-1.4\n%%EOF")
	if err := ValidateSignedPDF(tiny); !errors.Is(err, ErrInvalidPDF) {
		t.Errorf("expected ErrInvalidPDF for tiny payload, got %v", err)
	}
}

func TestValidateSignedPDF_RejectsMissingMagic(t *testing.T) {
	// HTML error page returned with application/pdf content-type — the
	// most common real-world failure we want to catch.
	html := []byte(strings.Repeat("<html><body>Bad Gateway</body></html>", 50))
	err := ValidateSignedPDF(html)
	if !errors.Is(err, ErrInvalidPDF) {
		t.Fatalf("expected ErrInvalidPDF for HTML payload, got %v", err)
	}
	if !strings.Contains(err.Error(), "%PDF-") {
		t.Errorf("expected error to mention missing %%PDF- header, got %q", err.Error())
	}
}

func TestValidateSignedPDF_RejectsMissingEOF(t *testing.T) {
	// Starts with %PDF-, is large enough, but the trailer was lost in
	// a truncated download.
	payload := []byte("%PDF-1.4\n")
	payload = append(payload, bytes.Repeat([]byte("x"), 4096)...)
	err := ValidateSignedPDF(payload)
	if !errors.Is(err, ErrInvalidPDF) {
		t.Fatalf("expected ErrInvalidPDF for truncated PDF, got %v", err)
	}
	if !strings.Contains(err.Error(), "EOF") {
		t.Errorf("expected error to mention missing EOF, got %q", err.Error())
	}
}

func TestValidateSignedPDF_AcceptsRealisticPayload(t *testing.T) {
	// %PDF- header, large enough, %%EOF in the trailing window.
	payload := []byte("%PDF-1.4\n")
	payload = append(payload, bytes.Repeat([]byte("body content "), 200)...)
	payload = append(payload, []byte("\n%%EOF\n")...)
	if err := ValidateSignedPDF(payload); err != nil {
		t.Errorf("expected nil for realistic PDF, got %v", err)
	}
}

func TestValidateSignedPDF_AcceptsEOFInSignatureTrailer(t *testing.T) {
	// Some signed PDFs have a small post-EOF signature trailer. As long
	// as %%EOF is within the last 2KB, the file is valid.
	payload := []byte("%PDF-1.4\n")
	payload = append(payload, bytes.Repeat([]byte("body "), 500)...)
	payload = append(payload, []byte("\n%%EOF\n")...)
	payload = append(payload, bytes.Repeat([]byte("sig-trailer "), 50)...) // ~600 bytes after EOF
	if err := ValidateSignedPDF(payload); err != nil {
		t.Errorf("expected nil when %%EOF is within trailing window, got %v", err)
	}
}
