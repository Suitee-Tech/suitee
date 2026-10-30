package domain

import "time"

// FieldSpec descreve um campo que a extração deve tentar identificar. O catálogo
// é montado pelo serviço dono do modelo de dados (toca-deal) e enviado ao toca-ai,
// de modo que o toca-ai permanece genérico: extrai exatamente o que lhe for pedido
// ("tudo que for campo mapeado na plataforma" para o alvo do documento).
type FieldSpec struct {
	Name     string `json:"name"`   // chave canônica: "document","name","birth_date","cep"...
	Label    string `json:"label"`  // rótulo pt-BR para a UI
	Type     string `json:"type"`   // cpf_cnpj|text|date|cep|phone|email|currency|uf
	Target   string `json:"target"` // destino no modelo: "party.name", "party.custom_fields.cep"
	Required bool   `json:"required"`
}

// DocumentContext dá ao extrator pistas sobre a origem do documento.
type DocumentContext struct {
	Category   string `json:"category,omitempty"`    // identidade, comprovante, ...
	EntityType string `json:"entity_type,omitempty"` // deal, contract, ...
	EntityID   string `json:"entity_id,omitempty"`
	PartyKind  string `json:"party_kind,omitempty"` // person|company
}

// ExtractDocumentRequest é o corpo de POST /api/v1/ai/internal/documents/extract.
type ExtractDocumentRequest struct {
	DocumentID   string          `json:"document_id"`
	DocumentURL  string          `json:"document_url"` // presigned, TTL curto
	DocumentType string          `json:"document_type,omitempty"`
	TenantID     string          `json:"tenant_id,omitempty"`
	Context      DocumentContext `json:"document_context,omitempty"`
	TargetFields []FieldSpec     `json:"target_fields"`
}

// Status de uma extração.
const (
	ExtractionStatusCompleted = "completed"
	ExtractionStatusFailed    = "failed"
	ExtractionStatusEmpty     = "empty"
)

// Faixas de confiança (ver document_extraction_service.go).
const (
	TierHigh   = "high"   // >= 0.85 — auto-aplicável
	TierMedium = "medium" // 0.60..0.85 — revisar
	TierLow    = "low"    // < 0.60 — manual
)

// Origem do dado de um campo extraído (sempre OCR neste serviço; a party guarda
// a origem final — ocr/ai/manual/validated — no toca-deal).
const SourceOCR = "ocr"

// Resultado da validação determinística de um campo.
const (
	ValidationValid     = "valid"
	ValidationInvalid   = "invalid"
	ValidationUnchecked = "unchecked"
	ValidationEmpty     = "empty"
)

// ExtractedField é um campo identificado, com confiança composta e status de validação.
type ExtractedField struct {
	Name              string  `json:"name"`
	Label             string  `json:"label,omitempty"`
	Target            string  `json:"target,omitempty"`
	Value             string  `json:"value"`               // normalizado (dígitos p/ CPF, YYYY-MM-DD p/ datas)
	RawValue          string  `json:"raw_value,omitempty"` // como o modelo/OCR viu
	Evidence          string  `json:"evidence,omitempty"`  // trecho de onde o valor veio
	Confidence        float64 `json:"confidence"`          // 0..1 composta
	Tier              string  `json:"tier"`
	Source            string  `json:"source"`
	ValidationStatus  string  `json:"validation_status"`
	ValidationMessage string  `json:"validation_message,omitempty"`
}

// ExtractedDocument é o resultado completo de uma extração.
type ExtractedDocument struct {
	DocumentID        string           `json:"document_id"`
	DocumentType      string           `json:"document_type,omitempty"`
	Status            string           `json:"status"` // completed|failed|empty
	Error             string           `json:"error,omitempty"`
	Fields            []ExtractedField `json:"fields"`
	OverallConfidence float64          `json:"overall_confidence"`
	Model             string           `json:"model,omitempty"`
	OCRProvider       string           `json:"ocr_provider,omitempty"`
	OCRConfidence     *float64         `json:"ocr_confidence,omitempty"`
	DurationMS        int64            `json:"duration_ms,omitempty"`
	CreatedAt         time.Time        `json:"created_at"`
}
