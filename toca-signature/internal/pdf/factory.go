package pdf

import (
	"os"
	"strings"
)

// NewFromEnv returns the production-ready Renderer based on env vars.
//
// If GOTENBERG_URL is set, returns a ChainedRenderer (DefaultRenderer for
// URL/base64/raw PDF + GotenbergRenderer for HTML/DOCX). Otherwise returns a
// bare DefaultRenderer that errors with ErrHTMLNotSupported on HTML/DOCX
// content — the EnvelopeService persists that as Status=ERROR and the operator
// can fix the content (or wire Gotenberg) and resend.
func NewFromEnv() Renderer {
	gotenbergURL := strings.TrimSpace(os.Getenv("GOTENBERG_URL"))
	def := NewDefaultRenderer(nil)
	if gotenbergURL == "" {
		return def
	}
	return NewChainedRenderer(def, NewGotenbergRenderer(gotenbergURL))
}
