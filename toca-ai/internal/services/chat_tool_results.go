package services

import (
	"encoding/json"
	"fmt"
	"strings"
)

func summarizeToolResult(toolName, output, userMessage string) (string, bool) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(output), &obj); err != nil {
		return "Não consegui interpretar a resposta da ferramenta. Tente novamente.", true
	}

	data := obj["data"]
	meta, _ := obj["meta"].(map[string]any)
	listWanted := userExplicitlyRequestsList(userMessage)

	switch toolName {
	case "create_deal_from_lead_and_property":
		dealID := firstString(obj, "deal_id")
		if dealID == "" {
			return "A ação foi executada, mas não consegui identificar o deal criado.", true
		}
		if !listWanted {
			out := "### Deal criado\n\n" +
				"- ID: " + dealID + "\n" +
				"- Próximo passo sugerido: revisar checklist e avançar para proposta.\n\n" +
				"Quer que eu abra o deal ou inicie o checklist?"
			return out, true
		}
		opsCount := 0
		if ops, ok := obj["ops"].([]any); ok {
			opsCount = len(ops)
		}
		block := suiteeCardsBlock{
			Title: "Deal",
			Items: []suiteeCardItem{
				{
					Key:          dealID,
					Title:        "Deal criado",
					Subtitle:     "ID " + dealID,
					Badges:       []string{fmt.Sprintf("%d operações", opsCount)},
					StatusColor:  "green",
					OpenPath:     "/esteira/deals/" + dealID,
					Copy:         dealID,
					ActionPrompt: "Sugira os próximos passos imediatos para este deal (checklist, proposta e pendências).",
				},
			},
		}
		return mustSuiteeCards(block), true

	case "search_leads":
		total := intFromAny(meta["total"])
		if total == 0 {
			return "Não encontrei leads para esse filtro. Se quiser, posso listar por outro status (ex.: novo, contato, agendado) ou criar um lead agora.", true
		}
		items, ok := data.([]any)
		if !ok || len(items) == 0 {
			return "Encontrei leads, mas não consegui listar os itens retornados. Quer que eu tente novamente com outro limite?", true
		}
		if !listWanted {
			return formatLeadsMarkdown(items, total), true
		}
		block := suiteeCardsBlock{
			Title: "Leads",
			Items: make([]suiteeCardItem, 0, minInt(len(items), 10)),
		}
		for i, it := range items {
			if i >= 10 {
				break
			}
			m, _ := it.(map[string]any)
			id := firstString(m, "id")
			name := firstString(m, "name", "full_name", "title")
			status := firstString(m, "status", "stage")
			assignee := firstString(m, "assignee_name", "broker_name", "owner_name")
			phone := firstString(m, "phone", "whatsapp")
			email := firstString(m, "email")
			createdAt := firstString(m, "created_at", "createdAt")

			sub := strings.TrimSpace(assignee)
			if sub == "" && phone != "" {
				sub = phone
			}
			badges := make([]string, 0, 3)
			if status != "" {
				badges = append(badges, humanizeLeadStatus(status))
			}
			if assignee != "" {
				badges = append(badges, assignee)
			}

			openPath := ""
			if id != "" {
				openPath = "/leads/" + id
			}
			copyText := strings.TrimSpace(name)
			if phone != "" {
				copyText += " — " + phone
			}
			actionPrompt := "Sugira os próximos 3 passos para o lead \"" + strings.TrimSpace(name) + "\" com base no status atual."

			metadata := make([]suiteeCardMetadata, 0, 2)
			if phone != "" {
				metadata = append(metadata, suiteeCardMetadata{Kind: "phone", Value: phone})
			}
			if createdAt != "" {
				metadata = append(metadata, suiteeCardMetadata{Kind: "date", Value: createdAt})
			}
			details := make([]suiteeCardDetail, 0, 4)
			if email != "" {
				details = append(details, suiteeCardDetail{Label: "Email", Value: email})
			}
			if assignee != "" {
				details = append(details, suiteeCardDetail{Label: "Responsável", Value: assignee})
			}

			block.Items = append(block.Items, suiteeCardItem{
				Key:          id,
				Title:        strings.TrimSpace(name),
				Subtitle:     sub,
				Badges:       badges,
				StatusColor:  statusColorFromStatus(status),
				OpenPath:     openPath,
				Copy:         copyText,
				ActionPrompt: actionPrompt,
				Metadata:     metadata,
				Details:      details,
			})
		}
		if len(block.Items) == 0 {
			return "Encontrei leads, mas não consegui montar a lista. Quer que eu tente novamente com outro limite?", true
		}
		return mustSuiteeCards(block), true

	case "search_properties":
		total := intFromAny(meta["total"])
		if total == 0 {
			return "Não encontrei imóveis para esse filtro. Se quiser, posso buscar por tipo/status diferentes ou cadastrar um imóvel.", true
		}
		items, ok := data.([]any)
		if !ok || len(items) == 0 {
			return "Encontrei imóveis, mas não consegui listar os itens retornados. Quer que eu tente novamente com outro filtro?", true
		}
		if !listWanted {
			return formatPropertiesMarkdown(items, total), true
		}
		block := suiteeCardsBlock{
			Title: "Imóveis",
			Items: make([]suiteeCardItem, 0, minInt(len(items), 10)),
		}
		for i, it := range items {
			if i >= 10 {
				break
			}
			m, _ := it.(map[string]any)
			id := firstString(m, "id")
			title := firstString(m, "title", "name", "reference", "code")
			status := firstString(m, "status")
			pType := firstString(m, "type", "property_type")
			price := firstString(m, "price", "value")
			if price == "" {
				if v := nestedNumber(m, "pricing", "sale_price", "current_amount"); v > 0 {
					price = fmt.Sprintf("%.0f", v)
				} else if v := nestedNumber(m, "pricing", "rent_price", "current_amount"); v > 0 {
					price = fmt.Sprintf("%.0f", v)
				}
			}
			priceFmt := formatCurrencyBRL(price)
			neighborhood := nestedString(m, "address", "neighborhood")
			city := nestedString(m, "address", "city")
			bedrooms := nestedNumber(m, "features", "bedrooms")
			bathrooms := nestedNumber(m, "features", "bathrooms")
			area := nestedNumber(m, "areas", "total")

			badges := make([]string, 0, 3)
			if pType != "" {
				badges = append(badges, humanizePropertyType(pType))
			}
			if status != "" {
				badges = append(badges, humanizePropertyStatus(status))
			}
			sub := strings.TrimSpace(priceFmt)

			openPath := ""
			if id != "" {
				openPath = "/imoveis/" + id
			}
			copyText := strings.TrimSpace(title)
			actionPrompt := "Resuma os pontos fortes do imóvel \"" + strings.TrimSpace(title) + "\" e sugira o melhor próximo passo comercial."

			metadata := make([]suiteeCardMetadata, 0, 2)
			if priceFmt != "" {
				metadata = append(metadata, suiteeCardMetadata{Kind: "currency", Value: priceFmt})
			}
			details := make([]suiteeCardDetail, 0, 6)
			loc := strings.TrimSpace(strings.Trim(strings.Join([]string{neighborhood, city}, " · "), " ·"))
			if loc != "" {
				details = append(details, suiteeCardDetail{Label: "Local", Value: loc})
			}
			if bedrooms > 0 {
				details = append(details, suiteeCardDetail{Label: "Quartos", Value: fmt.Sprintf("%d", int(bedrooms))})
			}
			if bathrooms > 0 {
				details = append(details, suiteeCardDetail{Label: "Banheiros", Value: fmt.Sprintf("%d", int(bathrooms))})
			}
			if area > 0 {
				details = append(details, suiteeCardDetail{Label: "Área", Value: fmt.Sprintf("%.0f m²", area)})
			}

			block.Items = append(block.Items, suiteeCardItem{
				Key:          id,
				Title:        strings.TrimSpace(title),
				Subtitle:     sub,
				Badges:       badges,
				StatusColor:  statusColorFromStatus(status),
				OpenPath:     openPath,
				Copy:         copyText,
				ActionPrompt: actionPrompt,
				Metadata:     metadata,
				Details:      details,
			})
		}
		if len(block.Items) == 0 {
			return "Encontrei imóveis, mas não consegui montar a lista. Quer que eu refine por tipo/status?", true
		}
		return mustSuiteeCards(block), true

	case "get_calendar_events":
		items, ok := data.([]any)
		if !ok || len(items) == 0 {
			return "Não encontrei compromissos na agenda para esse período/limite. Quer que eu busque com um limite maior?", true
		}
		return formatAgendaMarkdown(items), true

	case "get_deals_pipeline":
		items, ok := data.([]any)
		if !ok || len(items) == 0 {
			return "Não encontrei deals no pipeline com esse filtro. Quer que eu busque sem filtro de status?", true
		}
		if !listWanted {
			return formatPipelineMarkdown(items), true
		}
		if isPipelineAggregate(items) {
			return formatPipelineMarkdown(items), true
		}
		block := suiteeCardsBlock{
			Title: "Pipeline",
			Items: make([]suiteeCardItem, 0, minInt(len(items), 10)),
		}
		for i, it := range items {
			if i >= 10 {
				break
			}
			m, _ := it.(map[string]any)
			id := firstString(m, "id")
			title := firstString(m, "title", "name", "customer_name")
			stage := firstString(m, "stage", "status")
			value := firstString(m, "value", "amount", "total_value")
			valueFmt := formatCurrencyBRL(value)

			badges := make([]string, 0, 2)
			if stage != "" {
				badges = append(badges, humanizeDealStage(stage))
			}
			sub := strings.TrimSpace(valueFmt)

			openPath := "/esteira"
			if id != "" {
				openPath = "/esteira/deals/" + id
			}
			copyText := strings.TrimSpace(title)
			if stage != "" {
				copyText += " — " + humanizeDealStage(stage)
			}
			if valueFmt != "" {
				copyText += " — " + valueFmt
			}
			actionPrompt := "Sugira a próxima ação mais provável para avançar este deal: \"" + strings.TrimSpace(title) + "\"."

			block.Items = append(block.Items, suiteeCardItem{
				Key:          id,
				Title:        strings.TrimSpace(title),
				Subtitle:     sub,
				Badges:       badges,
				StatusColor:  statusColorFromStatus(stage),
				OpenPath:     openPath,
				Copy:         copyText,
				ActionPrompt: actionPrompt,
				Metadata: func() []suiteeCardMetadata {
					if valueFmt == "" {
						return nil
					}
					return []suiteeCardMetadata{{Kind: "currency", Value: valueFmt}}
				}(),
			})
		}
		if len(block.Items) == 0 {
			return "Encontrei deals no pipeline, mas não consegui montar a lista. Quer que eu tente novamente com outro filtro?", true
		}
		return mustSuiteeCards(block), true

	case "get_deal_by_id", "update_deal_status", "init_deal_checklist", "add_deal_checklist_item", "update_deal_checklist_item_status", "solicit_deal_checklist_item", "remove_deal_checklist_item":
		dm, ok := data.(map[string]any)
		if !ok || dm == nil {
			return "Encontrei dados do deal, mas não consegui montar a visualização. Tente novamente.", true
		}
		if !listWanted {
			return formatDealMarkdown(dm), true
		}
		return mustSuiteeCards(buildDealCards(dm)), true
	}

	_ = userMessage
	return "A ferramenta retornou dados. Quer que eu resuma os principais itens?", false
}
