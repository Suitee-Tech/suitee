package services

import (
	"fmt"
	"strings"

	"github.com/oginoino/toca/toca-ai/internal/domain"
)

// ExtractionPromptBuilder monta as mensagens da extração estruturada a partir do
// catálogo de campos (recebido do toca-deal) e do markdown do OCR. Segue o estilo
// da casa (pt-BR, direto, sem saudações/emojis — ver prompt_builder.go).
type ExtractionPromptBuilder struct{}

func NewExtractionPromptBuilder() *ExtractionPromptBuilder { return &ExtractionPromptBuilder{} }

// limite defensivo de tamanho do texto enviado ao LLM (caracteres).
const extractionMaxDocChars = 24000

// Build devolve [system, user]. O system contém a palavra "json" (requisito do
// JSON mode do DeepSeek) e fecha o espaço de saída no schema esperado.
func (b *ExtractionPromptBuilder) Build(docType string, ctx domain.DocumentContext, fields []domain.FieldSpec, markdown string) []DeepseekMessage {
	var sb strings.Builder
	sb.WriteString("Você é um extrator de dados de documentos brasileiros para uma plataforma imobiliária. ")
	sb.WriteString("Recebe o texto (markdown) de UM documento já processado por OCR e deve identificar APENAS os campos listados.\n\n")
	sb.WriteString("Regras:\n")
	sb.WriteString("- Responda SOMENTE com um objeto json válido, sem nenhum texto fora dele e sem cercas de código.\n")
	sb.WriteString("- Formato exato: {\"fields\":[{\"name\":\"<chave>\",\"value\":\"<valor>\",\"confidence\":<0..1>,\"evidence\":\"<trecho>\"}]}\n")
	sb.WriteString("- Use exatamente as chaves (name) da lista abaixo; não crie campos novos.\n")
	sb.WriteString("- Se um campo não aparecer no documento, retorne value \"\" e confidence 0.\n")
	sb.WriteString("- Formatos brasileiros: CPF 000.000.000-00, CNPJ 00.000.000/0000-00, datas DD/MM/AAAA, CEP 00000-000.\n")
	sb.WriteString("- confidence reflete sua certeza pela legibilidade e pelo contexto (não superestime).\n")
	sb.WriteString("- evidence é um trecho curto do texto onde o valor foi encontrado.\n")
	sb.WriteString("- Não inclua comentários nem campos extras no json.\n\n")

	if docType != "" {
		sb.WriteString(fmt.Sprintf("Tipo do documento: %s\n", docType))
	}
	if ctx.Category != "" {
		sb.WriteString(fmt.Sprintf("Categoria: %s\n", ctx.Category))
	}
	if ctx.PartyKind != "" {
		sb.WriteString(fmt.Sprintf("Natureza da parte: %s\n", ctx.PartyKind))
	}

	sb.WriteString("\nCampos a identificar:\n")
	for _, f := range fields {
		label := f.Label
		if label == "" {
			label = f.Name
		}
		sb.WriteString(fmt.Sprintf("- %s (tipo %s): %s\n", f.Name, f.Type, label))
	}

	user := strings.TrimSpace(markdown)
	if len(user) > extractionMaxDocChars {
		user = user[:extractionMaxDocChars]
	}

	return []DeepseekMessage{
		{Role: "system", Content: sb.String()},
		{Role: "user", Content: "Documento (markdown do OCR):\n\n" + user},
	}
}
