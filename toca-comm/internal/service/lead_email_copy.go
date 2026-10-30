package service

import (
	"fmt"
	"html"
	"strings"
)

// Copy e rótulos do e-mail de lead atribuído.
//
// Os payloads de domínio carregam enums crus (NEW, sequential, COMPRA_VENDA,
// HIGH_PRICE…). O e-mail é lido pelo corretor, então traduzimos para a mesma
// linguagem da tela (toka/src/shared/lib/leads/mappers.ts + Leads.tsx) e
// agrupamos as tags por significado — fonte, papel, interesse, tipo, faixa de
// preço e região — em vez de despejar a lista técnica.

// leadStatusLabels espelha os rótulos da tela de leads do painel.
var leadStatusLabels = map[string]string{
	"NEW":                "Novo",
	"PENDING_ACCEPTANCE": "Em Atendimento",
	"CONTACTED":          "Em Atendimento",
	"QUALIFIED":          "Qualificado",
	"VISIT_SCHEDULED":    "Visita Agendada",
	"PROPOSAL":           "Proposta Enviada",
	"NEGOTIATION":        "Em Negociação",
	"CONVERTED":          "Convertido",
	"LOST":               "Perdido",
	"DISQUALIFIED":       "Desqualificado",
}

// leadSourceLabels — de onde o lead veio.
var leadSourceLabels = map[string]string{
	"WEBSITE":        "Site",
	"REFERRAL":       "Indicação",
	"SOCIAL":         "Redes sociais",
	"MANUAL":         "Cadastro manual",
	"CAMPAIGN":       "Campanha",
	"ZAP_IMOVEIS":    "ZAP Imóveis",
	"OLX_GRUPO_ZAP":  "OLX / Grupo ZAP",
	"VIVAREAL":       "Viva Real",
	"PORTAL_CLIENTE": "Portal do cliente",
}

// leadRoleLabels — papel do cliente no atendimento.
var leadRoleLabels = map[string]string{
	"BUYER":     "Comprador",
	"TENANT":    "Locatário",
	"OWNER":     "Proprietário",
	"GUARANTOR": "Fiador",
}

// leadInterestLabels — finalidade do atendimento (inclui os aliases legados
// com e sem acento que o DeriveTags emite em duplicidade).
var leadInterestLabels = map[string]string{
	"COMPRA_VENDA":   "Compra e venda",
	"COMPRA E VENDA": "Compra e venda",
	"LOCACAO":        "Locação",
	"LOCAÇÃO":        "Locação",
	"CAPTACAO":       "Captação de imóvel",
	"CAPTAÇÃO":       "Captação de imóvel",
}

// leadPropertyTypeLabels — tipo de imóvel de interesse.
var leadPropertyTypeLabels = map[string]string{
	"APARTMENT":  "Apartamento",
	"HOUSE":      "Casa",
	"LAND":       "Terreno",
	"COMMERCIAL": "Comercial",
}

// leadPriceBandLabels — faixa de preço derivada da preferência do lead.
var leadPriceBandLabels = map[string]string{
	"LOW_PRICE":     "Até R$ 300 mil",
	"MID_PRICE":     "R$ 300 mil a R$ 800 mil",
	"HIGH_PRICE":    "R$ 800 mil a R$ 2 milhões",
	"PREMIUM_PRICE": "Acima de R$ 2 milhões",
}

// leadNeutralTagLabels — valores ambíguos no domínio (OTHER é usado tanto
// como origem quanto como tipo de imóvel). Rotulamos de forma neutra em
// "Outros" em vez de afirmar uma categoria que pode estar errada.
var leadNeutralTagLabels = map[string]string{
	"OTHER": "Outro",
}

// leadMethodLabels — como o lead chegou ao corretor. Vazio = atribuição
// manual (só a roleta preenche Method no evento).
var leadMethodLabels = map[string]string{
	"":             "Manual (pelo painel)",
	"manual":       "Manual (pelo painel)",
	"sequential":   "Roleta sequencial",
	"load_balance": "Roleta com carga equilibrada",
	"weighted":     "Roleta ponderada",
	"priority":     "Roleta por prioridade",
}

// leadTagGroup é um bloco do perfil do lead (ex.: "Origem" → "Cadastro manual").
type leadTagGroup struct {
	Label  string
	Values []string
}

// leadStatusLabel traduz o status/etapa; valor desconhecido é humanizado.
func leadStatusLabel(status string) string {
	if label, ok := leadStatusLabels[strings.ToUpper(strings.TrimSpace(status))]; ok {
		return label
	}
	return humanizeEnum(status)
}

// leadMethodLabel traduz o método de distribuição.
func leadMethodLabel(method string) string {
	key := strings.ToLower(strings.TrimSpace(method))
	if label, ok := leadMethodLabels[key]; ok {
		return label
	}
	return humanizeEnum(method)
}

// leadTagGroups classifica as tags cruas em blocos legíveis, colapsando
// aliases (COMPRA_VENDA + "COMPRA E VENDA") e descartando repetições. Tags
// não reconhecidas caem em "Região" quando parecem localização e em "Outros"
// caso contrário — o corretor nunca vê um enum cru sem tradução.
func leadTagGroups(tags []string) []leadTagGroup {
	order := []string{"Origem", "Perfil", "Interesse", "Tipo de imóvel", "Faixa de preço", "Região", "Outros"}
	grouped := map[string][]string{}
	seen := map[string]bool{}

	add := func(group, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := group + "\x1f" + strings.ToLower(value)
		if seen[key] {
			return
		}
		seen[key] = true
		grouped[group] = append(grouped[group], value)
	}

	for _, raw := range tags {
		tag := strings.ToUpper(strings.TrimSpace(raw))
		if tag == "" {
			continue
		}
		switch {
		case leadNeutralTagLabels[tag] != "":
			// Valor ambíguo/no-op (OTHER): não agrega nada ao corretor —
			// omitir é melhor que afirmar uma categoria errada.
			continue
		case leadSourceLabels[tag] != "":
			add("Origem", leadSourceLabels[tag])
		case leadRoleLabels[tag] != "":
			add("Perfil", leadRoleLabels[tag])
		case leadInterestLabels[tag] != "":
			add("Interesse", leadInterestLabels[tag])
		case leadPropertyTypeLabels[tag] != "":
			add("Tipo de imóvel", leadPropertyTypeLabels[tag])
		case leadPriceBandLabels[tag] != "":
			add("Faixa de preço", leadPriceBandLabels[tag])
		case strings.Contains(raw, "_"):
			// Enum novo ainda sem tradução: humaniza e agrupa como "Outros".
			add("Outros", humanizeEnum(raw))
		default:
			// Texto livre do operador (localização preferida): "Região".
			add("Região", humanizeEnum(raw))
		}
	}

	out := make([]leadTagGroup, 0, len(grouped))
	for _, label := range order {
		if values := grouped[label]; len(values) > 0 {
			out = append(out, leadTagGroup{Label: label, Values: values})
		}
	}
	return out
}

// humanizeEnum converte um enum cru em texto apresentável:
// "HIGH PRICE" → "High Price"; preserva valores já legíveis.
func humanizeEnum(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return r == '_' || r == '-' || r == ' '
	})
	for i, w := range words {
		if w == "" {
			continue
		}
		runes := []rune(w)
		words[i] = strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	if len(words) == 0 {
		return ""
	}
	return strings.Join(words, " ")
}

// leadAssignedSubject monta o assunto do e-mail de lead atribuído. Sem nome
// no payload (eventos antigos), cai para o formato genérico anterior.
func leadAssignedSubject(leadName string) string {
	if leadName == "" {
		return "Novo lead atribuído"
	}
	return fmt.Sprintf("Novo lead atribuído: %s", leadName)
}

// leadAssignedBody monta o corpo do e-mail com os campos traduzidos e o
// perfil do lead agrupado. Valores dinâmicos são escapados (região vem de
// texto livre digitado pelo operador).
func leadAssignedBody(payload leadAssignedPayload) string {
	displayName := payload.LeadName
	if displayName == "" {
		displayName = "Um novo lead"
	}
	body := fmt.Sprintf("<p style='margin: 0 0 24px 0; font-size: 16px; color: #4b5563; line-height: 1.6;'><strong>%s</strong> foi atribuído a você.</p>", html.EscapeString(displayName))

	line := func(label, value string) string {
		if strings.TrimSpace(value) == "" {
			return ""
		}
		return fmt.Sprintf("<p style='margin: 0 0 6px 0; font-size: 14px; color: #6b7280;'><strong style='color: #374151;'>%s:</strong> %s</p>", html.EscapeString(label), html.EscapeString(value))
	}

	body += line("Etapa", leadStatusLabel(payload.Stage))
	body += line("Distribuição", leadMethodLabel(payload.Method))
	for _, group := range leadTagGroups(payload.Tags) {
		body += line(group.Label, strings.Join(group.Values, " · "))
	}
	return body
}
