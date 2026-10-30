// Package pdf provides a shared HTML→PDF renderer backed by Gotenberg
// (https://gotenberg.dev). Antes desse pacote, só toca-signature tinha
// um renderer (internal/pdf), preso ao fluxo Clicksign. Issue #435/#436
// precisam gerar PDFs de domínio (laudo de vistoria, distrato) em
// toca-ops e toca-deal — daí a promoção para pkg/.
//
// Escopo: só a rota HTML do Gotenberg (Chromium). A rota LibreOffice
// (DOCX) fica no toca-signature porque é específica do fluxo de
// contrato assinável. Documentos de domínio gerados por nós são
// sempre HTML templated.
//
// Wiring típico no service:
//
//	renderer := pdf.NewRendererFromEnv() // lê GOTENBERG_URL
//	bytes, err := renderer.RenderHTML(ctx, "laudo-"+inspectionID, html)
//	// → upload via pkg/storage, set ReportURL, etc.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
)

// Renderer converte HTML em bytes de PDF. Interface para que services
// injetem um fake em testes e para o NoopRenderer cobrir ambientes
// sem Gotenberg configurado.
type Renderer interface {
	// RenderHTML transforma o HTML completo (com <html>, <head>, CSS
	// inline) em PDF. traceID vai no header Gotenberg-Trace para
	// correlação de logs — passe o id do domínio (inspectionID,
	// contractID).
	RenderHTML(ctx context.Context, traceID, html string) ([]byte, error)
	// Available reporta se o renderer consegue produzir PDF de fato.
	// NoopRenderer retorna false; o caller decide se isso é fatal ou
	// degrada gracioso (ex.: grava o documento sem PDF e reprocessa
	// depois).
	Available() bool
}

// ErrNotConfigured — Gotenberg não está configurado. Caller trata
// como "feature de PDF desligada" e segue sem o documento, ou loga
// erro de deploy, conforme a criticidade.
var ErrNotConfigured = errors.New("pdf renderer not configured (GOTENBERG_URL unset)")

// GotenbergRenderer fala com uma instância Gotenberg v8 pela rota
// Chromium HTML. Stateless — pode ser compartilhado entre goroutines.
type GotenbergRenderer struct {
	baseURL    string
	httpClient *http.Client
}

// NewGotenbergRenderer — construtor explícito. timeout default 60s
// (renderização de Chromium pode levar alguns segundos em páginas
// pesadas com muitas imagens, ex.: laudo de vistoria com 30 fotos).
func NewGotenbergRenderer(baseURL string) *GotenbergRenderer {
	return &GotenbergRenderer{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

// NewRendererFromEnv devolve um GotenbergRenderer quando GOTENBERG_URL
// está setado, senão um NoopRenderer. Nunca retorna erro — o caller
// checa Available() se quiser falhar no boot.
func NewRendererFromEnv() Renderer {
	url := strings.TrimSpace(os.Getenv("GOTENBERG_URL"))
	if url == "" {
		return NoopRenderer{}
	}
	return NewGotenbergRenderer(url)
}

func (r *GotenbergRenderer) Available() bool {
	return r.baseURL != ""
}

func (r *GotenbergRenderer) RenderHTML(ctx context.Context, traceID, html string) ([]byte, error) {
	if r.baseURL == "" {
		return nil, ErrNotConfigured
	}

	body, contentType, err := buildHTMLForm(html)
	if err != nil {
		return nil, fmt.Errorf("build gotenberg form: %w", err)
	}

	endpoint := r.baseURL + "/forms/chromium/convert/html"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if traceID != "" {
		// Gotenberg propaga esse header pros logs dele — facilita
		// correlacionar "PDF X falhou" com a request original.
		req.Header.Set("Gotenberg-Trace", traceID)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg request: %w", err)
	}
	defer resp.Body.Close()

	pdfBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gotenberg read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gotenberg status=%d body=%s", resp.StatusCode, truncate(string(pdfBytes), 240))
	}
	return pdfBytes, nil
}

// NoopRenderer — usado quando GOTENBERG_URL não está setado. Toda
// chamada falha com ErrNotConfigured. Existe para que o wiring nos
// services não precise de nil-check em todo lugar.
type NoopRenderer struct{}

func (NoopRenderer) Available() bool { return false }
func (NoopRenderer) RenderHTML(_ context.Context, _, _ string) ([]byte, error) {
	return nil, ErrNotConfigured
}

// buildHTMLForm monta o multipart form que o Gotenberg espera: o
// campo DEVE se chamar "files" e o arquivo DEVE se chamar index.html
// (convenção da API Chromium do Gotenberg).
func buildHTMLForm(html string) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("files", "index.html")
	if err != nil {
		return nil, "", err
	}
	if _, err := fw.Write([]byte(html)); err != nil {
		return nil, "", err
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
