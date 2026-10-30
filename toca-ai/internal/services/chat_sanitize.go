package services

import (
	"strings"
	"unicode/utf8"
)

func sanitizeAssistantOutput(s string) string {
	orig := strings.TrimSpace(s)
	if orig == "" {
		return ""
	}
	stripped := strings.TrimSpace(stripAssistantBoilerplate(orig))
	if stripped == "" {
		if isLikelyBoilerplate(orig) {
			return "Para eu avançar com isso, preciso alinhar 2 pontos:\n\n" +
				"- Qual período devo considerar? (ex.: últimos 30 dias)\n" +
				"- O que conta como “fechado” aqui: **Convertido**, **Contrato**, ou outro status?\n\n" +
				"Se você preferir, eu posso assumir **últimos 30 dias** e começar agora."
		}
		stripped = orig
	}
	s = stripped

	var b strings.Builder
	b.Grow(len(s))
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		if r == utf8.RuneError && size == 1 {
			continue
		}
		if isEmojiRune(r) {
			continue
		}
		if r == '\uFE0F' {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return strings.TrimSpace(orig)
	}
	return out
}

func isLikelyBoilerplate(s string) bool {
	low := strings.ToLower(strings.TrimSpace(s))
	return strings.Contains(low, "como posso ajudar") ||
		strings.Contains(low, "o que você gostaria de fazer") ||
		strings.Contains(low, "o que você gostaria de consultar") ||
		strings.Contains(low, "o que você gostaria de analisar") ||
		strings.Contains(low, "o que você gostaria de consultar ou analisar") ||
		strings.Contains(low, "o que você precisa") ||
		strings.Contains(low, "aqui estão algumas") ||
		strings.Contains(low, "ações possíveis") ||
		strings.Contains(low, "você está na tela") ||
		strings.Contains(low, "posso ajudar com")
}

func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF:
		return true
	case r >= 0x2600 && r <= 0x27BF:
		return true
	default:
		return false
	}
}

func stripAssistantBoilerplate(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	inCapabilities := false

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			if inCapabilities {
				continue
			}
			out = append(out, "")
			continue
		}

		low := strings.ToLower(line)
		if len(out) == 0 {
			if strings.HasPrefix(low, "olá") || strings.HasPrefix(low, "oi") || strings.HasPrefix(low, "bom dia") || strings.HasPrefix(low, "boa tarde") || strings.HasPrefix(low, "boa noite") {
				continue
			}
			if strings.Contains(low, "bem-vindo") || strings.Contains(low, "bem vindo") {
				continue
			}
			if strings.Contains(low, "sou a suitee") || strings.Contains(low, "sou seu assistente") || strings.Contains(low, "seu assistente operacional") {
				continue
			}
		}

		if strings.Contains(low, "como posso ajudar") ||
			strings.Contains(low, "o que você gostaria de fazer") ||
			strings.Contains(low, "o que você gostaria de consultar") ||
			strings.Contains(low, "o que você gostaria de analisar") ||
			strings.Contains(low, "o que você gostaria de consultar ou analisar") ||
			strings.Contains(low, "o que você precisa") ||
			strings.Contains(low, "aqui estão algumas coisas que posso fazer") {
			inCapabilities = true
			continue
		}

		if inCapabilities {
			if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "•") || strings.HasPrefix(line, "—") {
				continue
			}
			if strings.Contains(low, "consultas") || strings.Contains(low, "relat") || strings.Contains(low, "ações") || strings.Contains(low, "operacion") {
				continue
			}
			if strings.Contains(low, "pode me perguntar") {
				continue
			}
			inCapabilities = false
		}

		out = append(out, raw)
	}

	return strings.TrimSpace(strings.Join(out, "\n"))
}
