package validation

import (
	"fmt"
	"strings"
	"unicode"
)

// Listas de etiquetas (comodidades, tags, palavras-chave) têm o mesmo contrato
// dos dois lados: normalizar, tirar duplicata sem diferenciar maiúsculas e
// respeitar teto de quantidade e de tamanho. Sem teto, um cliente pode gravar
// milhares de itens (o campo vai para o JSONB do registro) e a leitura de todo
// mundo fica carregada.
const (
	// MaxTagItems é o teto de itens de uma lista.
	MaxTagItems = 40
	// MaxTagRunes é o teto de caracteres de um item.
	MaxTagRunes = 60
)

// NormalizeTag limpa um item: sem espaços nas pontas, sem caracteres de
// controle (o valor vem de formulário e vai para HTML/JSON) e com espaços
// internos colapsados. Item vazio devolve "".
func NormalizeTag(tag string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, tag)
	return strings.Join(strings.Fields(cleaned), " ")
}

// NormalizeTags aplica NormalizeTag, descarta vazios e remove duplicatas sem
// diferenciar maiúsculas/minúsculas, preservando a ordem e a grafia do primeiro.
func NormalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		tag := NormalizeTag(raw)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	return out
}

// ValidateTags recusa a lista que passaria dos tetos, com mensagem voltada ao
// operador. `label` é o nome do campo (ex.: "Comodidades").
func ValidateTags(tags []string, label string) error {
	normalized := NormalizeTags(tags)
	if len(normalized) > MaxTagItems {
		return fmt.Errorf("%s: máximo de %d itens (recebido: %d)", label, MaxTagItems, len(normalized))
	}
	for _, tag := range normalized {
		if len([]rune(tag)) > MaxTagRunes {
			return fmt.Errorf("%s: cada item pode ter até %d caracteres (recebido: %q)", label, MaxTagRunes, truncateRunes(tag, 30))
		}
	}
	return nil
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}
