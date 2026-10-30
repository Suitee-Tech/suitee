package document

import (
	"strings"

	"github.com/google/uuid"
)

// checklistNamespace é o namespace fixo dos IDs derivados de itens do checklist.
// Fixo para que o mesmo item sempre produza o mesmo documento — reenvio do
// cliente e replay de evento atualizam a linha em vez de duplicar.
var checklistNamespace = uuid.MustParse("6f7c2f7e-3a8b-5d2c-9f1e-2b6a4c8d0e11")

// ChecklistDocumentID devolve o ID determinístico do documento espelho de um
// item do checklist da esteira digital.
func ChecklistDocumentID(itemID string) string {
	return uuid.NewSHA1(checklistNamespace, []byte(strings.TrimSpace(itemID))).String()
}

// ChecklistClassification é o resultado da classificação de um item do
// checklist: a categoria do painel de documentos e as tags de organização.
type ChecklistClassification struct {
	Category Category
	Tags     []string
}

// ClassifyChecklistItem classifica um documento enviado pelo cliente na esteira
// (item do checklist) para o painel de documentos da imobiliária.
//
// A ordem das regras importa: o NOME do item e os rótulos dos campos pedidos
// dizem mais do que a categoria genérica do checklist ("pessoal", "financeiro"),
// que entra como fallback. Tudo é normalizado (minúsculas, sem acento) para que
// "Certidão de Ônus" e "certidao de onus" caiam na mesma regra.
func ClassifyChecklistItem(itemName, itemCategory string, requestedFieldLabels ...string) ChecklistClassification {
	haystack := normalizeChecklistText(itemName)
	for _, label := range requestedFieldLabels {
		haystack += " " + normalizeChecklistText(label)
	}

	category := CategoryOutros
	switch {
	// Identidade — documentos pessoais do cliente/parte.
	case containsAny(haystack, "cpf", "cnh", "rg", "identidade", "passaporte", "titulo de eleitor", "documento com foto", "selfie"):
		category = CategoryIdentidade
	// Comprovação de renda/residência e extratos.
	case containsAny(haystack, "comprovante de renda", "comprovante de residencia", "comprovante", "holerite", "contracheque", "extrato", "irpf", "imposto de renda", "declaracao de renda", "fatura"):
		category = CategoryComprovante
	// Documentos do imóvel com peso jurídico.
	case containsAny(haystack, "matricula", "escritura", "certidao de onus", "onus e gravames", "registro do imovel", "contrato social", "procuracao", "certidao"):
		category = CategoryJuridico
	case containsAny(haystack, "iptu", "itbi", "tributo", "tributario", "fiscal"):
		category = CategoryFiscal
	case containsAny(haystack, "laudo"):
		category = CategoryLaudo
	case containsAny(haystack, "vistoria"):
		category = CategoryVistoria
	case containsAny(haystack, "nota fiscal", "nf ", "nf-", "nf.", "recibo", "pagamento", "financeiro", "cobranca", "repasse"):
		category = CategoryFinanceiro
	case containsAny(haystack, "foto", "imagem", "print", "captura"):
		category = CategoryFoto
	// Fallback: a categoria declarada pelo operador no checklist.
	default:
		switch normalizeChecklistText(itemCategory) {
		case "pessoal":
			category = CategoryIdentidade
		case "financeiro":
			category = CategoryFinanceiro
		case "imovel", "juridico":
			category = CategoryJuridico
		case "outros", "":
			category = CategoryOutros
		}
	}

	return ChecklistClassification{
		Category: category,
		Tags:     []string{"esteira", "cliente", string(category)},
	}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle != "" && strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}

// accentFold mapeia os acentuados do português para a letra base. Sem isso
// "certidão" viraria "certid o" (o acento não é marca combinante em Go, é um
// rune próprio) e nenhuma regra casaria.
var accentFold = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
)

// normalizeChecklistText deixa o texto comparável: minúsculas, sem acento e com
// espaços colapsados (pontuação vira espaço, para "NF-1234.pdf" casar com "nf").
func normalizeChecklistText(raw string) string {
	var b strings.Builder
	lastSpace := true
	for _, r := range accentFold.Replace(strings.ToLower(strings.TrimSpace(raw))) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastSpace = false
		default:
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}
