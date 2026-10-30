package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/clients"
)

// OCRPageConfidence espelha a confiança por página retornada pelo toca-ocr.
type OCRPageConfidence struct {
	Page         any      `json:"page"`
	OCRScore     *float64 `json:"ocr_score"`
	LayoutScore  *float64 `json:"layout_score"`
	ParseScore   *float64 `json:"parse_score"`
	TableScore   *float64 `json:"table_score"`
	OverallScore *float64 `json:"overall_score"`
}

// OCRResult é a resposta normalizada de POST /ocr/parse no toca-ocr.
type OCRResult struct {
	Markdown      string              `json:"markdown"`
	Text          string              `json:"text"`
	Pages         []OCRPageConfidence `json:"pages"`
	OCRConfidence *float64            `json:"ocr_confidence"`
	Provider      string              `json:"provider"`
}

// OCRClient fala com o serviço toca-ocr (docling), autenticado por X-Internal-Key.
type OCRClient struct {
	base *clients.BaseClient
}

// NewOCRClient cria o cliente do toca-ocr. baseURL vem de OCR_SERVICE_URL.
func NewOCRClient(baseURL string) *OCRClient {
	bc := clients.NewBaseClient(baseURL)
	// O toca-ocr valida X-Internal-Key contra INTERNAL_API_KEY (chave
	// compartilhada), não contra INTERNAL_SERVICE_KEYS. O BaseClient prioriza
	// INTERNAL_CALLER_KEY, que oscila entre recriações do serviço e fazia o OCR
	// responder 401 (X-Internal-Key inválida ou ausente). Aqui usamos a chave
	// compartilhada estável, que é a mesma configurada no toca-ocr.
	if shared := strings.TrimSpace(os.Getenv("INTERNAL_API_KEY")); shared != "" {
		bc.InternalAPIKey = shared
	}
	// OCR (docling) pode levar dezenas de segundos; o timeout interno padrão (5s)
	// é agressivo demais — espelha o bump do signature_client.
	bc.HTTPClient.Timeout = 90 * time.Second
	return &OCRClient{base: bc}
}

// Parse pede ao toca-ocr para baixar e converter um documento pela URL pré-assinada.
func (c *OCRClient) Parse(ctx context.Context, sourceURL string) (*OCRResult, error) {
	return c.do(ctx, map[string]string{"source_url": sourceURL})
}

func (c *OCRClient) do(ctx context.Context, body any) (*OCRResult, error) {
	resp, err := c.base.Do(ctx, http.MethodPost, "/ocr/parse", body)
	if err != nil {
		return nil, fmt.Errorf("ocr request failed: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2000))
		return nil, fmt.Errorf("ocr service status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var out OCRResult
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("failed to decode ocr response: %w", err)
	}
	return &out, nil
}
