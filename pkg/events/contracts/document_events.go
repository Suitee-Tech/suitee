package contracts

import "github.com/google/uuid"

// DocumentUploadedEventPayload — publicado pelo toca-deal quando um documento de
// dados (identidade/comprovante, PDF/imagem) é enviado no fluxo de upload.
// Consumido pelo próprio toca-deal (subscriber dedicado, fila própria) para
// disparar a extração OCR+IA assíncrona — mantém o upload rápido e isola a
// latência do OCR/LLM. Idempotente por DocumentID no consumidor.
type DocumentUploadedEventPayload struct {
	DocumentID string `json:"document_id"`
	TenantID   string `json:"tenant_id"`
	EntityID   string `json:"entity_id,omitempty"`
	EntityType string `json:"entity_type,omitempty"`
	Category   string `json:"category,omitempty"`
	FileType   string `json:"file_type,omitempty"`
	UploadedBy string `json:"uploaded_by,omitempty"`
	Timestamp  string `json:"timestamp"`
}

type DocumentUploadedEvent struct {
	EventIDVal string                       `json:"event_id"`
	Data       DocumentUploadedEventPayload `json:"payload"`
}

func (e DocumentUploadedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e DocumentUploadedEvent) Name() string         { return "document.uploaded" }
func (e DocumentUploadedEvent) Payload() interface{} { return e.Data }
