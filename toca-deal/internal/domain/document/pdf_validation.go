package document

import (
	"bytes"
	"errors"
	"fmt"
)

// ValidateSignedPDF performs cheap structural sanity checks on bytes we
// expect to be a finished, signed PDF. The goal is not full
// signature-dictionary validation (that needs a real parser like
// pdfcpu / unipdf — tracked in roadmap §3 S4 follow-up) but to reject
// the obvious failure modes that have shown up in incident postmortems:
// HTML error pages returned with `application/pdf` content-type,
// truncated downloads, and empty/zero-byte bodies.
//
// Checks performed:
//
//   • size >= MinSignedPDFBytes (rejects empties + truncated downloads)
//   • leading bytes start with "%PDF-" (rejects HTML/JSON/text payloads)
//   • trailing window contains "%%EOF"  (rejects truncated downloads)
//
// Returns nil for valid bytes. Any failure wraps a sentinel
// (`ErrInvalidPDF`) so callers can decide whether to retry, drop, or
// flag for manual review without inspecting error strings.
//
// Callsite: `DocumentService.PersistSignedContractPDF` runs this on the
// bytes downloaded from Clicksign *before* uploading to Supabase, so
// we never store garbage that an operator would later confuse with a
// real signed contract.

const (
	// MinSignedPDFBytes is a deliberately conservative floor — even an
	// almost-empty PDF (`%PDF-1.4\n%%EOF`) is dozens of bytes. We've
	// never seen a real signed contract under 1KB, and Clicksign's
	// error pages are typically 200-800 bytes of HTML.
	MinSignedPDFBytes = 1024
	// eofWindowBytes is how far back from the end of the file we look
	// for the `%%EOF` marker. PDF spec allows trailing whitespace and
	// some signature trailers append a small amount of data after EOF,
	// so a fixed window is safer than requiring the last 5 bytes.
	eofWindowBytes = 2048
)

// ErrInvalidPDF is the sentinel wrapped by every validation failure.
// Callers can do `errors.Is(err, document.ErrInvalidPDF)`.
var ErrInvalidPDF = errors.New("invalid PDF")

var (
	pdfMagicBytes = []byte("%PDF-")
	pdfEOFMarker  = []byte("%%EOF")
)

func ValidateSignedPDF(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("%w: empty payload", ErrInvalidPDF)
	}
	if len(data) < MinSignedPDFBytes {
		return fmt.Errorf("%w: payload too small (%d bytes, need >= %d)",
			ErrInvalidPDF, len(data), MinSignedPDFBytes)
	}
	if !bytes.HasPrefix(data, pdfMagicBytes) {
		preview := previewBytes(data, 16)
		return fmt.Errorf("%w: missing %%PDF- header (got %q)",
			ErrInvalidPDF, preview)
	}
	// Look for %%EOF in the trailing window so we tolerate the
	// signature trailer / linearized layouts that may append a few
	// bytes after the canonical EOF.
	window := data
	if len(data) > eofWindowBytes {
		window = data[len(data)-eofWindowBytes:]
	}
	if !bytes.Contains(window, pdfEOFMarker) {
		return fmt.Errorf("%w: missing %%%%EOF marker in last %d bytes",
			ErrInvalidPDF, eofWindowBytes)
	}
	return nil
}

func previewBytes(data []byte, n int) string {
	if len(data) > n {
		data = data[:n]
	}
	// Strip non-printable so log output stays readable when the payload
	// is binary garbage or HTML control chars.
	out := make([]byte, 0, len(data))
	for _, b := range data {
		if b >= 32 && b < 127 {
			out = append(out, b)
		} else {
			out = append(out, '.')
		}
	}
	return string(out)
}
