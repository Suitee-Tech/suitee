// Package pdf turns Contract.Content (which can be a URL, inline base64, raw
// PDF bytes, DOCX data URL, or HTML) into PDF bytes ready to upload to
// Clicksign.
//
// The Renderer interface is what EnvelopeService consumes. NewRenderer wires
// the right strategy based on env: if GOTENBERG_URL is configured, an HTML
// fallback is enabled; otherwise an HTML payload yields an explicit error.
package pdf

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Renderer turns the Contract.Content string into PDF bytes.
type Renderer interface {
	Render(ctx context.Context, contractID, content string) ([]byte, error)
}

// DefaultRenderer handles the no-network paths and HTTPS download. It does not
// know how to render HTML/DOCX — wrap it with a ChainedRenderer that delegates
// those inputs to a real engine (Gotenberg in production).
type DefaultRenderer struct {
	httpClient *http.Client
}

func NewDefaultRenderer(hc *http.Client) *DefaultRenderer {
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	return &DefaultRenderer{httpClient: hc}
}

func (r *DefaultRenderer) Render(ctx context.Context, contractID, content string) ([]byte, error) {
	if content == "" {
		return nil, errors.New("contract has no content")
	}
	switch {
	case strings.HasPrefix(content, "data:application/pdf;base64,"):
		raw := strings.TrimPrefix(content, "data:application/pdf;base64,")
		return base64.StdEncoding.DecodeString(raw)
	case strings.HasPrefix(content, "%PDF"):
		return []byte(content), nil
	case strings.HasPrefix(content, "https://"), strings.HasPrefix(content, "http://"):
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, content, nil)
		if err != nil {
			return nil, err
		}
		resp, err := r.httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("download pdf: status %d", resp.StatusCode)
		}
		return io.ReadAll(resp.Body)
	}
	return nil, ErrHTMLNotSupported
}

// ErrHTMLNotSupported is returned by DefaultRenderer when content looks like
// HTML/DOCX (or anything that's not URL/base64/raw PDF). ChainedRenderer detects
// this sentinel and falls back to its conversion engine.
var ErrHTMLNotSupported = errors.New("pdf rendering not configured: content is not a PDF URL, base64, raw PDF, or convertible DOCX/HTML")

// ChainedRenderer tries Default first; on ErrHTMLNotSupported it falls back to
// the conversion renderer. Use this when GOTENBERG_URL is configured in
// production.
type ChainedRenderer struct {
	primary  Renderer
	fallback Renderer
}

func NewChainedRenderer(primary, fallback Renderer) *ChainedRenderer {
	return &ChainedRenderer{primary: primary, fallback: fallback}
}

func (r *ChainedRenderer) Render(ctx context.Context, contractID, content string) ([]byte, error) {
	out, err := r.primary.Render(ctx, contractID, content)
	if err == nil {
		return out, nil
	}
	if errors.Is(err, ErrHTMLNotSupported) && r.fallback != nil {
		return r.fallback.Render(ctx, contractID, content)
	}
	return nil, err
}
