package services

import (
	"fmt"
	"strings"
)

func userExplicitlyRequestsList(userMessage string) bool {
	u := strings.ToLower(strings.TrimSpace(userMessage))
	if u == "" {
		return false
	}
	if strings.HasPrefix(u, "/leads") || strings.HasPrefix(u, "/imoveis") || strings.HasPrefix(u, "/imóveis") || strings.HasPrefix(u, "/pipeline") {
		return true
	}
	return strings.Contains(u, "liste") ||
		strings.Contains(u, "listar") ||
		strings.Contains(u, "mostre") ||
		strings.Contains(u, "mostra") ||
		strings.Contains(u, "exiba") ||
		strings.Contains(u, "exibir") ||
		strings.Contains(u, "me traga") ||
		strings.Contains(u, "me mostre") ||
		strings.Contains(u, "lista ") ||
		strings.Contains(u, "quais ")
}

func formatLeadsMarkdown(items []any, total int) string {
	statusCounts := map[string]int{}
	sample := make([]string, 0, 5)
	for i, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		status := humanizeLeadStatus(firstString(m, "status", "stage"))
		if status != "" {
			statusCounts[status]++
		}
		if i < 5 {
			name := strings.TrimSpace(firstString(m, "name", "full_name", "title"))
			assignee := strings.TrimSpace(firstString(m, "assignee_name", "broker_name", "owner_name"))
			line := "- " + name
			if status != "" {
				line += " · " + status
			}
			if assignee != "" {
				line += " · " + assignee
			}
			sample = append(sample, line)
		}
	}
	var b strings.Builder
	b.WriteString("### Leads\n\n")
	if total > 0 {
		b.WriteString("- Total encontrado: " + fmt.Sprintf("%d", total) + "\n")
	}
	if len(statusCounts) > 0 {
		b.WriteString("- Amostra por status: ")
		i := 0
		for st, c := range statusCounts {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(st + " (" + fmt.Sprintf("%d", c) + ")")
			i++
			if i >= 4 {
				break
			}
		}
		b.WriteString("\n")
	}
	if len(sample) > 0 {
		b.WriteString("\n**Amostra (até 5):**\n")
		b.WriteString(strings.Join(sample, "\n"))
	}
	b.WriteString("\n\nSe quiser, eu posso **listar** os leads em cards com ações (Abrir/Copiar/Próximos passos).")
	return strings.TrimSpace(b.String())
}

func formatPropertiesMarkdown(items []any, total int) string {
	typeCounts := map[string]int{}
	statusCounts := map[string]int{}
	sample := make([]string, 0, 5)
	for i, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		pt := humanizePropertyType(firstString(m, "type", "property_type"))
		st := humanizePropertyStatus(firstString(m, "status"))
		if pt != "" {
			typeCounts[pt]++
		}
		if st != "" {
			statusCounts[st]++
		}
		if i < 5 {
			title := strings.TrimSpace(firstString(m, "title", "name", "reference", "code"))
			price := formatCurrencyBRL(firstString(m, "price", "value"))
			line := "- " + title
			if price != "" {
				line += " · " + price
			}
			if st != "" {
				line += " · " + st
			}
			sample = append(sample, line)
		}
	}
	var b strings.Builder
	b.WriteString("### Imóveis\n\n")
	if total > 0 {
		b.WriteString("- Total encontrado: " + fmt.Sprintf("%d", total) + "\n")
	}
	if len(typeCounts) > 0 {
		b.WriteString("- Tipos (amostra): ")
		i := 0
		for t, c := range typeCounts {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(t + " (" + fmt.Sprintf("%d", c) + ")")
			i++
			if i >= 4 {
				break
			}
		}
		b.WriteString("\n")
	}
	if len(statusCounts) > 0 {
		b.WriteString("- Status (amostra): ")
		i := 0
		for st, c := range statusCounts {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(st + " (" + fmt.Sprintf("%d", c) + ")")
			i++
			if i >= 4 {
				break
			}
		}
		b.WriteString("\n")
	}
	if len(sample) > 0 {
		b.WriteString("\n**Amostra (até 5):**\n")
		b.WriteString(strings.Join(sample, "\n"))
	}
	b.WriteString("\n\nSe quiser, eu posso **listar** os imóveis em cards com ações (Abrir/Copiar/Próximos passos).")
	return strings.TrimSpace(b.String())
}

func formatDealMarkdown(deal map[string]any) string {
	dealID := strings.TrimSpace(firstString(deal, "display_id", "displayId"))
	if dealID == "" {
		dealID = strings.TrimSpace(firstString(deal, "id"))
	}
	status := humanizeDealStage(firstString(deal, "status", "stage"))
	dealType := humanizeDealType(firstString(deal, "type"))
	value := nestedNumber(deal, "value")
	customer := strings.TrimSpace(firstString(deal, "customer_name", "customerName"))
	updatedAt := strings.TrimSpace(firstString(deal, "updated_at"))

	var b strings.Builder
	b.WriteString("### Deal\n\n")
	if dealID != "" {
		b.WriteString("- Identificador: " + dealID + "\n")
	}
	if dealType != "" {
		b.WriteString("- Tipo: " + dealType + "\n")
	}
	if status != "" {
		b.WriteString("- Etapa/status: " + status + "\n")
	}
	if value > 0 {
		b.WriteString("- Valor: " + formatCurrencyBRLFromFloat(value) + "\n")
	}
	if customer != "" {
		b.WriteString("- Cliente: " + customer + "\n")
	}
	if updatedAt != "" {
		b.WriteString("- Atualizado em: " + updatedAt + "\n")
	}
	b.WriteString("\nQuer que eu resuma as pendências do checklist e sugira o próximo passo para avançar este deal?")
	return strings.TrimSpace(b.String())
}

func isPipelineAggregate(items []any) bool {
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok || m == nil {
			continue
		}
		if strings.TrimSpace(firstString(m, "id")) != "" {
			return false
		}
		if strings.TrimSpace(firstString(m, "stage", "status")) != "" {
			return true
		}
	}
	return false
}

func formatPipelineMarkdown(items []any) string {
	type row struct {
		stage    string
		count    int
		value    string
		isClosed bool
	}
	rows := make([]row, 0, len(items))
	totalCount := 0
	closedCount := 0
	var totalValue float64
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok || m == nil {
			continue
		}
		stage := strings.TrimSpace(firstString(m, "stage", "status"))
		if stage == "" {
			continue
		}
		stageHuman := humanizeDealStage(stage)
		stageLow := strings.ToLower(stageHuman)
		isClosed := strings.Contains(stageLow, "convert") || strings.Contains(stageLow, "fech") || strings.Contains(stageLow, "ganh") || strings.Contains(stageLow, "contrat")
		count := intFromAny(m["count"])
		if count <= 0 {
			count = intFromAny(m["total"])
		}
		if count <= 0 {
			count = intFromAny(m["qty"])
		}
		if count <= 0 {
			if v := nestedNumber(m, "count"); v > 0 {
				count = int(v)
			}
		}
		valStr := strings.TrimSpace(firstString(m, "value", "amount", "total_value"))
		valNum := nestedNumber(m, "value")
		if valNum <= 0 && valStr != "" {
			valStrFmt := formatCurrencyBRL(valStr)
			valStr = valStrFmt
		}
		if valNum > 0 {
			totalValue += valNum
			valStr = formatCurrencyBRLFromFloat(valNum)
		}
		if count > 0 {
			totalCount += count
			if isClosed {
				closedCount += count
			}
		}
		rows = append(rows, row{
			stage:    stageHuman,
			count:    count,
			value:    valStr,
			isClosed: isClosed,
		})
	}
	if len(rows) == 0 {
		return "Não consegui resumir o pipeline a partir da resposta da ferramenta."
	}
	var b strings.Builder
	b.WriteString("### Pipeline\n")
	if totalCount > 0 || totalValue > 0 {
		b.WriteString("\n")
		b.WriteString("- Total: ")
		if totalCount > 0 {
			b.WriteString(fmt.Sprintf("%d deals", totalCount))
		} else {
			b.WriteString("—")
		}
		if totalValue > 0 {
			b.WriteString(" · ")
			b.WriteString(formatCurrencyBRLFromFloat(totalValue))
		}
		if totalCount > 0 && closedCount > 0 {
			b.WriteString(" · ")
			b.WriteString("Fechados: ")
			b.WriteString(fmt.Sprintf("%d", closedCount))
			b.WriteString(" (")
			b.WriteString(fmt.Sprintf("%.1f", (float64(closedCount)/float64(totalCount))*100))
			b.WriteString("%)")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n| Etapa | Qtde | Valor |\n|---|---:|---:|\n")
	for _, r := range rows {
		b.WriteString("| ")
		b.WriteString(r.stage)
		b.WriteString(" | ")
		if r.count > 0 {
			b.WriteString(fmt.Sprintf("%d", r.count))
		} else {
			b.WriteString("—")
		}
		b.WriteString(" | ")
		if r.value != "" {
			b.WriteString(r.value)
		} else {
			b.WriteString("—")
		}
		b.WriteString(" |\n")
	}
	b.WriteString("\n### Insights\n")
	if totalCount > 0 && closedCount == 0 {
		b.WriteString("\n- Não identifiquei nenhuma etapa claramente “fechada/convertida” nos dados retornados. Me diga qual etapa conta como fechamento para eu calcular corretamente.\n")
	} else if totalCount > 0 {
		b.WriteString("\n- Conversão aproximada para etapas finais (fechado/convertido/contrato): ")
		b.WriteString(fmt.Sprintf("%.1f", (float64(closedCount)/float64(totalCount))*100))
		b.WriteString("%.\n")
	}
	b.WriteString("\n### Próximos passos\n")
	b.WriteString("\n- Se você me disser o período (ex.: últimos 30 dias) e qual etapa é “fechamento”, eu recalculo e detalho gargalos por etapa.\n")
	return strings.TrimSpace(b.String())
}

func formatAgendaMarkdown(items []any) string {
	var b strings.Builder
	b.WriteString("### Agenda\n\n")
	limit := minInt(len(items), 10)
	for i := 0; i < limit; i++ {
		m, _ := items[i].(map[string]any)
		title := strings.TrimSpace(firstString(m, "title", "name", "summary"))
		start := strings.TrimSpace(firstString(m, "start_at", "start", "starts_at", "date"))
		where := strings.TrimSpace(firstString(m, "location", "place"))
		line := ""
		if start != "" && title != "" {
			line = "- **" + start + "** — " + title
		} else if title != "" {
			line = "- " + title
		} else if start != "" {
			line = "- **" + start + "**"
		}
		if line == "" {
			continue
		}
		if where != "" {
			line += " · " + where
		}
		b.WriteString(line + "\n")
	}
	out := strings.TrimSpace(b.String())
	if out == "### Agenda" {
		return "Encontrei compromissos na agenda, mas não consegui montar uma lista legível. Quer que eu tente novamente com um limite maior?"
	}
	return out
}
