package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// AIFieldSpec descreve um campo que a extração deve identificar. O catálogo é
// montado pelo toca-deal (dono do modelo) — ver services.PartyFieldCatalog — e
// enviado ao toca-ai, que extrai exatamente o que lhe for pedido.
type AIFieldSpec struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Type     string `json:"type"` // cpf_cnpj|text|date|cep|phone|email|currency|uf
	Target   string `json:"target"`
	Required bool   `json:"required"`
}

// AIDocumentContext dá pistas de origem ao extrator.
type AIDocumentContext struct {
	Category   string `json:"category,omitempty"`
	EntityType string `json:"entity_type,omitempty"`
	EntityID   string `json:"entity_id,omitempty"`
	PartyKind  string `json:"party_kind,omitempty"`
}

// ExtractDocumentRequest é o corpo de POST /api/v1/ai/internal/documents/extract.
type ExtractDocumentRequest struct {
	DocumentID   string            `json:"document_id"`
	DocumentURL  string            `json:"document_url"`
	DocumentType string            `json:"document_type,omitempty"`
	TenantID     string            `json:"tenant_id,omitempty"`
	Context      AIDocumentContext `json:"document_context,omitempty"`
	TargetFields []AIFieldSpec     `json:"target_fields"`
}

// AIExtractedField espelha o campo identificado pela IA (toca-ai domain.ExtractedField).
type AIExtractedField struct {
	Name              string  `json:"name"`
	Label             string  `json:"label,omitempty"`
	Target            string  `json:"target,omitempty"`
	Value             string  `json:"value"`
	RawValue          string  `json:"raw_value,omitempty"`
	Evidence          string  `json:"evidence,omitempty"`
	Confidence        float64 `json:"confidence"`
	Tier              string  `json:"tier"`
	Source            string  `json:"source"`
	ValidationStatus  string  `json:"validation_status"`
	ValidationMessage string  `json:"validation_message,omitempty"`
}

// AIExtractedDocument é a resposta de extração do toca-ai.
type AIExtractedDocument struct {
	DocumentID        string             `json:"document_id"`
	DocumentType      string             `json:"document_type,omitempty"`
	Status            string             `json:"status"` // completed|failed|empty
	Error             string             `json:"error,omitempty"`
	Fields            []AIExtractedField `json:"fields"`
	OverallConfidence float64            `json:"overall_confidence"`
	Model             string             `json:"model,omitempty"`
	OCRProvider       string             `json:"ocr_provider,omitempty"`
	OCRConfidence     *float64           `json:"ocr_confidence,omitempty"`
	DurationMS        int64              `json:"duration_ms,omitempty"`
}

// AIClient fala com o toca-ai por rotas internas (X-Internal-Key).
type AIClient struct {
	BaseClient *BaseClient
}

func NewAIClient() *AIClient {
	bc := NewBaseClient(os.Getenv("AI_SERVICE_URL"))
	// OCR (docling) + LLM podem levar dezenas de segundos; o timeout interno
	// padrão (5s) é curto demais.
	bc.HTTPClient.Timeout = 90 * time.Second
	return &AIClient{BaseClient: bc}
}

// ExtractDocument dispara a extração OCR+IA de um documento e devolve os campos
// identificados com confiança. Erros de OCR/LLM voltam como status "failed"/"empty"
// no corpo (não como erro de transporte).
func (c *AIClient) ExtractDocument(ctx context.Context, req ExtractDocumentRequest) (*AIExtractedDocument, error) {
	resp, err := c.BaseClient.Do(ctx, http.MethodPost, "/api/v1/ai/internal/documents/extract", req)
	if err != nil {
		return nil, err
	}
	defer drainAndClose(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, aiServiceError(resp)
	}
	var out struct {
		Data AIExtractedDocument `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Data, nil
}

func aiServiceError(resp *http.Response) error {
	raw, _ := io.ReadAll(resp.Body)
	message := strings.TrimSpace(string(raw))
	var r struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &r); err == nil && strings.TrimSpace(r.Message) != "" {
		message = strings.TrimSpace(r.Message)
	}
	if message == "" {
		return fmt.Errorf("ai service returned status: %d", resp.StatusCode)
	}
	return fmt.Errorf("ai service returned status %d: %s", resp.StatusCode, message)
}
