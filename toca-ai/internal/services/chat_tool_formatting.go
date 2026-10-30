package services

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/oginoino/toca/pkg/money"
)

func checklistStatusColor(status string) string {
	s := strings.ToUpper(strings.TrimSpace(status))
	switch s {
	case "APPROVED":
		return "green"
	case "REJECTED":
		return "red"
	case "UPLOADED":
		return "yellow"
	case "PENDING":
		return "gray"
	default:
		return statusColorFromStatus(status)
	}
}

func formatCurrencyBRL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "r$") {
		return s
	}
	clean := make([]rune, 0, len(s))
	for _, r := range s {
		if (r >= '0' && r <= '9') || r == '.' || r == ',' {
			clean = append(clean, r)
		}
	}
	x := strings.ReplaceAll(string(clean), ".", "")
	x = strings.ReplaceAll(x, ",", ".")
	f, err := strconv.ParseFloat(x, 64)
	if err != nil || f <= 0 {
		return s
	}
	return formatCurrencyBRLFromFloat(f)
}

// formatCurrencyBRLFromFloat delega ao formatador compartilhado (`pkg/money`):
// o chat, os e-mails e o recibo precisam escrever o mesmo "R$ 60.000".
func formatCurrencyBRLFromFloat(f float64) string {
	if f <= 0 {
		return ""
	}
	return money.BRLWhole(f)
}

func humanizePropertyType(t string) string {
	s := strings.ToLower(strings.TrimSpace(t))
	switch s {
	case "apartment", "apartamento", "apto":
		return "Apartamento"
	case "house", "casa":
		return "Casa"
	case "commercial", "comercial":
		return "Comercial"
	case "land", "terreno":
		return "Terreno"
	default:
		return strings.TrimSpace(t)
	}
}

func humanizePropertyStatus(st string) string {
	s := strings.ToLower(strings.TrimSpace(st))
	switch s {
	case "prospecting", "captacao", "captação":
		return "Em captação"
	case "in_portfolio", "portfolio", "portfólio":
		return "Em portfólio"
	case "published", "anunciado":
		return "Anunciado"
	case "negotiation", "em_negociacao", "em_negociação":
		return "Em negociação"
	case "closed", "fechado":
		return "Fechado"
	case "sold", "vendido":
		return "Vendido"
	case "rented", "alugado":
		return "Alugado"
	default:
		return strings.TrimSpace(st)
	}
}

func humanizeDealStage(st string) string {
	s := strings.ToUpper(strings.TrimSpace(st))
	switch s {
	case "NEGOTIATION":
		return "Negociação"
	case "PROPOSAL":
		return "Proposta"
	case "ANALYSIS":
		return "Análise"
	case "CONTRACT":
		return "Contrato"
	case "CONVERTED":
		return "Convertido"
	case "CANCELLED":
		return "Cancelado"
	default:
		return strings.TrimSpace(st)
	}
}

func humanizeDealType(t string) string {
	s := strings.ToUpper(strings.TrimSpace(t))
	switch s {
	case "SALE":
		return "Venda"
	case "RENT":
		return "Locação"
	default:
		return strings.TrimSpace(t)
	}
}

func humanizeLeadStatus(st string) string {
	s := strings.ToLower(strings.TrimSpace(st))
	switch s {
	case "novo", "new":
		return "Novo"
	case "contato", "contact":
		return "Contato"
	case "agendado", "scheduled":
		return "Agendado"
	default:
		x := strings.TrimSpace(st)
		if x == "" {
			return ""
		}
		r := []rune(x)
		if len(r) == 0 {
			return ""
		}
		r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		return string(r)
	}
}

func humanizeChecklistCategory(c string) string {
	s := strings.ToLower(strings.TrimSpace(c))
	switch s {
	case "identity":
		return "Identidade"
	case "income":
		return "Renda"
	case "property":
		return "Imóvel"
	case "contract":
		return "Contrato"
	case "other":
		return "Outros"
	default:
		return strings.TrimSpace(c)
	}
}

func humanizeChecklistStatus(st string) string {
	s := strings.ToUpper(strings.TrimSpace(st))
	switch s {
	case "PENDING":
		return "Pendente"
	case "SOLICITED":
		return "Solicitado"
	case "UPLOADED":
		return "Enviado"
	case "APPROVED":
		return "Aprovado"
	case "REJECTED":
		return "Reprovado"
	default:
		return strings.TrimSpace(st)
	}
}

func boolFromAny(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return strings.EqualFold(strings.TrimSpace(t), "true") || strings.TrimSpace(t) == "1" || strings.EqualFold(strings.TrimSpace(t), "yes")
	case float64:
		return t != 0
	case int:
		return t != 0
	case int64:
		return t != 0
	default:
		return false
	}
}

func firstString(m map[string]any, keys ...string) string {
	if m == nil {
		return ""
	}
	for _, k := range keys {
		v, ok := m[k]
		if !ok || v == nil {
			continue
		}
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		case float64:
			if t != 0 {
				return strings.TrimSpace(fmt.Sprintf("%.0f", t))
			}
		case int:
			if t != 0 {
				return strings.TrimSpace(fmt.Sprintf("%d", t))
			}
		case int64:
			if t != 0 {
				return strings.TrimSpace(fmt.Sprintf("%d", t))
			}
		}
	}
	return ""
}
