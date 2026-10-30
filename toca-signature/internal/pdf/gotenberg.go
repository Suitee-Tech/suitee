package pdf

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

const docxDataURLPrefix = "data:application/vnd.openxmlformats-officedocument.wordprocessingml.document;base64,"

// GotenbergRenderer turns HTML and DOCX data URLs into PDF bytes using a
// Gotenberg v8 instance (https://gotenberg.dev). Configure GOTENBERG_URL in the
// env to enable; otherwise the service falls back to DefaultRenderer-only.
//
// We use the Chromium HTML route because it accepts a single index.html and
// renders it the way a browser would. The form field name MUST be "files"
// (per the Gotenberg API) and the file MUST be named index.html.
type GotenbergRenderer struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewGotenbergRenderer(baseURL string) *GotenbergRenderer {
	return &GotenbergRenderer{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
	}
}

func (r *GotenbergRenderer) Render(ctx context.Context, contractID, content string) ([]byte, error) {
	if r.BaseURL == "" {
		return nil, fmt.Errorf("gotenberg base url not configured (contract=%s)", contractID)
	}
	// DOCX pode chegar como data URL ou como base64 puro.
	//
	// Nem todo caminho que grava o contrato mantém o prefixo `data:` (a tela de
	// modelos e o painel de aditivo salvam base64 puro; a esteira salva data
	// URL). Sem reconhecer os dois formatos, o base64 puro caía no ramo de HTML
	// e o contrato enviado para assinatura saía como uma página de texto base64 —
	// o signatário via `UEsDBBQAAAAI...` em vez do contrato.
	if strings.HasPrefix(content, docxDataURLPrefix) {
		return r.renderOffice(ctx, contractID, content)
	}
	if isBareBase64Docx(content) {
		return r.renderOffice(ctx, contractID, docxDataURLPrefix+content)
	}

	body, contentType, err := buildHTMLForm(content)
	if err != nil {
		return nil, fmt.Errorf("build gotenberg form: %w", err)
	}

	url := r.BaseURL + "/forms/chromium/convert/html"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	// Gotenberg honors a custom trace header for log correlation.
	req.Header.Set("Gotenberg-Trace", contractID)

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg request: %w", err)
	}
	defer resp.Body.Close()

	pdf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gotenberg read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gotenberg status=%d body=%s", resp.StatusCode, truncate(string(pdf), 200))
	}
	return pdf, nil
}

func (r *GotenbergRenderer) renderOffice(ctx context.Context, contractID, content string) ([]byte, error) {
	raw := strings.TrimPrefix(content, docxDataURLPrefix)
	docx, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode docx data URL: %w", err)
	}

	body, contentType, err := buildOfficeForm(fmt.Sprintf("contrato-%s.docx", contractID), docx)
	if err != nil {
		return nil, fmt.Errorf("build gotenberg office form: %w", err)
	}

	url := r.BaseURL + "/forms/libreoffice/convert"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Gotenberg-Trace", contractID)

	resp, err := r.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gotenberg office request: %w", err)
	}
	defer resp.Body.Close()

	pdf, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gotenberg office read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gotenberg office status=%d body=%s", resp.StatusCode, truncate(string(pdf), 200))
	}
	return pdf, nil
}

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

func buildOfficeForm(filename string, doc []byte) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("files", filename)
	if err != nil {
		return nil, "", err
	}
	if _, err := fw.Write(doc); err != nil {
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

// isBareBase64Docx reconhece um DOCX em base64 sem o prefixo `data:`.
//
// Confere a assinatura ZIP ("PK\x03\x04") no conteúdo DECODIFICADO, e não só o
// prefixo em base64: um falso positivo aqui renderizaria lixo como documento, e
// um falso negativo manda base64 cru para o signatário. Ambos são ruins, então a
// checagem é sobre o que o arquivo de fato é.
func isBareBase64Docx(content string) bool {
	const zipMagic = "PK\x03\x04"
	// "UEsDB" é o começo de qualquer ZIP em base64; filtra antes de decodificar.
	if !strings.HasPrefix(content, "UEsDB") || len(content) < 64 {
		return false
	}
	// Só o começo interessa: decodificar o contrato inteiro aqui seria custoso e
	// desnecessário para identificar o formato.
	head := content
	if len(head) > 512 {
		head = head[:512]
	}
	// Corta até um múltiplo de 4 para decodificar sem erro de padding.
	if cut := len(head) - len(head)%4; cut >= 8 {
		head = head[:cut]
	}
	raw, err := base64.StdEncoding.DecodeString(head)
	if err != nil {
		return false
	}
	return strings.HasPrefix(string(raw), zipMagic)
}
