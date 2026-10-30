package contract

import "time"

// TemplateKind distingue para que serve o template: contrato de locação
// ou distrato (acordo de rescisão). Templates sem kind explícito — linhas
// criadas antes do #436 — são tratados como contract (ver EffectiveKind).
type TemplateKind string

const (
	TemplateKindContract TemplateKind = "contract"
	TemplateKindDistrato TemplateKind = "distrato"
)

// ValidTemplateKind reporta se k é um kind reconhecido (string vazia não
// conta — o caller deve aplicar o default contract antes de validar).
func ValidTemplateKind(k TemplateKind) bool {
	return k == TemplateKindContract || k == TemplateKindDistrato
}

type ContractTemplate struct {
	ID          string   `json:"id" bson:"_id"`
	Name        string   `json:"name" bson:"name" validate:"required"`
	Description string   `json:"description" bson:"description"`
	Content     string   `json:"content" bson:"content"`     // Base64 encoded DOCX file
	Variables   []string `json:"variables" bson:"variables"` // e.g. ["buyer_name", "property_address", "deal_value"]
	// Kind — contrato ou distrato (#436). Omitido em templates legados;
	// use EffectiveKind para ler com o default aplicado.
	Kind      TemplateKind `json:"kind,omitempty" bson:"kind,omitempty"`
	CreatedAt time.Time    `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time    `json:"updated_at" bson:"updated_at"`
	TenantID  string       `json:"tenant_id" bson:"tenant_id"`
}

// EffectiveKind devolve o kind do template, tratando o vazio (templates
// criados antes do #436) como contrato.
func (t *ContractTemplate) EffectiveKind() TemplateKind {
	if t.Kind == "" {
		return TemplateKindContract
	}
	return t.Kind
}
