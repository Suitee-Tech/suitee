package services

import (
	"context"
	"encoding/json"
	"strings"
)

type toolRouterDecision struct {
	Action   string         `json:"action"`
	ToolName string         `json:"tool_name"`
	Args     map[string]any `json:"args"`
	Question string         `json:"question"`
}

func (s *ChatService) llmRouteTool(ctx context.Context, userMessage string, screenContext any, tools []DeepseekTool, recentToolsContext string) (toolRouterDecision, bool) {
	label, path := screenFieldsFromContext(screenContext)
	var ctxJSON string
	if screenContext != nil {
		if b, err := json.Marshal(screenContext); err == nil {
			ctxJSON = string(b)
		}
	}

	var b strings.Builder
	b.WriteString("Você é um roteador de ferramentas para a Suitee Intelligence.\n")
	b.WriteString("Tarefa: decidir se deve chamar uma ferramenta read-only do MCP ou perguntar algo ao usuário.\n")
	b.WriteString("Responda SOMENTE com JSON válido (sem Markdown, sem texto extra).\n")
	b.WriteString("Schema:\n")
	b.WriteString("{\"action\":\"call_tool|ask_user|no_tool\",\"tool_name\":\"\",\"args\":{},\"question\":\"\"}\n")
	b.WriteString("Regras:\n")
	b.WriteString("- Se precisar de dados atuais para responder, use action=call_tool.\n")
	b.WriteString("- Só escolha ferramentas read-only (get_*, list_*, search_*, find_*, fetch_*).\n")
	b.WriteString("- Se faltar contexto crítico, use action=ask_user com 1-2 perguntas objetivas.\n")
	b.WriteString("- Se der para responder sem dados atuais, use action=no_tool.\n\n")
	b.WriteString("Contexto de tela:\n")
	b.WriteString("- label: " + strings.TrimSpace(label) + "\n")
	b.WriteString("- path: " + strings.TrimSpace(path) + "\n")
	if ctxJSON != "" {
		b.WriteString("- screen_context_json: " + ctxJSON + "\n")
	}
	if strings.TrimSpace(recentToolsContext) != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(recentToolsContext))
		b.WriteString("\n")
	}
	b.WriteString("\nFerramentas disponíveis:\n")
	for _, t := range tools {
		name := strings.TrimSpace(t.Function.Name)
		desc := strings.TrimSpace(t.Function.Description)
		if name == "" || desc == "" {
			continue
		}
		b.WriteString("- " + name + ": " + desc + "\n")
	}

	msgs := []DeepseekMessage{
		{Role: "system", Content: b.String()},
		{Role: "user", Content: strings.TrimSpace(userMessage)},
	}
	assistant, _, err := s.deepseek.ChatNoTools(ctx, msgs, 512)
	if err != nil {
		return toolRouterDecision{}, false
	}

	raw := strings.TrimSpace(assistant.Content)
	var out toolRouterDecision
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return toolRouterDecision{}, false
	}
	out.Action = strings.TrimSpace(out.Action)
	out.ToolName = strings.TrimSpace(out.ToolName)
	out.Question = strings.TrimSpace(out.Question)
	if out.Action == "" {
		return toolRouterDecision{}, false
	}
	return out, true
}

func (s *ChatService) llmInterpretToolResult(ctx context.Context, userMessage string, toolName string, toolOutput string) (string, bool) {
	listWanted := userExplicitlyRequestsList(userMessage)

	sys := "Você é a Suitee Intelligence.\n" +
		"Você recebeu dados do sistema (MCP) em JSON.\n" +
		"Use somente esses dados para responder. Não invente números.\n" +
		"Responda em pt-BR e use Markdown estruturado.\n" +
		"Faça análise/síntese (resumo, evidências, insights, próximos passos).\n" +
		"Não escreva menu de opções genérico.\n" +
		"Se usar links ou open_path, use rotas canônicas: /leads/{id}, /imoveis/{id}, /esteira/deals/{id}, /contratos/{id}, /financeiro/repasses, /financeiro/cobrancas. Nunca use /properties, /deals, /transfers ou /finance.\n"
	if listWanted {
		sys += "O usuário pediu explicitamente uma listagem. Você pode usar no máximo 1 bloco ```suitee_cards``` para listar entidades navegáveis com open_path. Use rotas canônicas: /leads/{id}, /imoveis/{id}, /esteira/deals/{id}, /contratos/{id}, /financeiro/repasses, /financeiro/cobrancas. Nunca use /properties, /deals, /transfers ou /finance.\n"
	} else {
		sys += "O usuário NÃO pediu listagem explícita. Não use ```suitee_cards```. Prefira resumo e agregações em Markdown.\n"
	}

	msgs := []DeepseekMessage{
		{Role: "system", Content: sys},
		{Role: "user", Content: strings.TrimSpace(userMessage)},
		{Role: "system", Content: "Tool result (" + strings.TrimSpace(toolName) + "):\n" + toolOutput},
	}
	assistant, _, err := s.deepseek.ChatNoTools(ctx, msgs, s.maxTokens)
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(assistant.Content), true
}

func indexToolsByName(tools []DeepseekTool) map[string]DeepseekTool {
	m := map[string]DeepseekTool{}
	for _, t := range tools {
		name := strings.TrimSpace(t.Function.Name)
		if name != "" {
			m[name] = t
		}
	}
	return m
}

func toolIsReadOnly(toolName string) bool {
	n := strings.ToLower(strings.TrimSpace(toolName))
	return strings.HasPrefix(n, "get_") ||
		strings.HasPrefix(n, "list_") ||
		strings.HasPrefix(n, "search_") ||
		strings.HasPrefix(n, "find_") ||
		strings.HasPrefix(n, "fetch_")
}

func requiresConfirmation(calls []DeepseekToolCall, toolIndex map[string]DeepseekTool) bool {
	for _, tc := range calls {
		if t, ok := toolIndex[tc.Function.Name]; ok {
			if !t.ReadOnly {
				return true
			}
			continue
		}
		if !toolIsReadOnly(tc.Function.Name) {
			return true
		}
	}
	return false
}

func shouldForceReadToolFallback(assistantContent string, userMessage string) bool {
	u := strings.ToLower(strings.TrimSpace(userMessage))
	if u == "" {
		return false
	}
	wantsData := strings.HasPrefix(u, "/") ||
		strings.Contains(u, "listar") ||
		strings.Contains(u, "liste") ||
		strings.Contains(u, "mostre") ||
		strings.Contains(u, "exiba") ||
		strings.Contains(u, "quais ") ||
		strings.Contains(u, "me traga") ||
		strings.Contains(u, "recent") ||
		strings.Contains(u, "últim") ||
		strings.Contains(u, "ultim") ||
		strings.Contains(u, "taxa") ||
		strings.Contains(u, "convers") ||
		strings.Contains(u, "funil") ||
		strings.Contains(u, "propor") ||
		strings.Contains(u, "percent") ||
		strings.Contains(u, "porcent") ||
		strings.Contains(u, "distrib") ||
		strings.Contains(u, "quebra") ||
		strings.Contains(u, "kpi") ||
		strings.Contains(u, "métrica") ||
		strings.Contains(u, "metrica") ||
		strings.Contains(u, "performance") ||
		strings.Contains(u, "fechad") ||
		strings.Contains(u, "ganh")
	if !wantsData {
		return false
	}
	a := strings.ToLower(strings.TrimSpace(assistantContent))
	if a == "" {
		return true
	}
	if strings.Contains(a, "o que você gostaria de fazer") ||
		strings.Contains(a, "qual dessas opções") ||
		strings.Contains(a, "como posso ajudar") ||
		strings.Contains(a, "posso:") ||
		strings.Contains(a, "posso ") {
		return true
	}
	if strings.HasSuffix(a, "?") && len([]rune(a)) < 80 {
		return true
	}
	return false
}

func fallbackReadToolForUserMessage(userMessage string, screenContext any, toolIndex map[string]DeepseekTool) (string, map[string]any) {
	msg := strings.ToLower(strings.TrimSpace(userMessage))
	if msg == "" {
		return "", nil
	}
	label, path := screenFieldsFromContext(screenContext)
	label = strings.ToLower(label)

	entity, entityID := screenEntityFromContext(screenContext)
	if entityID == "" {
		entity, entityID = entityFromPath(path)
	}

	limit := 20
	for i := 0; i < len(msg); i++ {
		if msg[i] < '0' || msg[i] > '9' {
			continue
		}
		n := 0
		for i < len(msg) && msg[i] >= '0' && msg[i] <= '9' {
			n = n*10 + int(msg[i]-'0')
			i++
		}
		if n > 0 && n <= 200 {
			limit = n
		}
		break
	}

	wantsDetail := strings.Contains(msg, "detalh") || strings.Contains(msg, "resum") || strings.Contains(msg, "este ") || strings.Contains(msg, "essa ")
	wantsAnalytics := strings.Contains(msg, "taxa") ||
		strings.Contains(msg, "convers") ||
		strings.Contains(msg, "funil") ||
		strings.Contains(msg, "propor") ||
		strings.Contains(msg, "percent") ||
		strings.Contains(msg, "porcent") ||
		strings.Contains(msg, "distrib") ||
		strings.Contains(msg, "quebra") ||
		strings.Contains(msg, "kpi") ||
		strings.Contains(msg, "métrica") ||
		strings.Contains(msg, "metrica") ||
		strings.Contains(msg, "performance") ||
		strings.Contains(msg, "fechad") ||
		strings.Contains(msg, "ganh")

	switch entity {
	case "lead":
		if wantsDetail {
			if _, ok := toolIndex["get_lead_by_id"]; ok && entityID != "" {
				return "get_lead_by_id", map[string]any{"lead_id": entityID}
			}
		}
	case "property":
		if wantsDetail {
			if _, ok := toolIndex["get_property_by_id"]; ok && entityID != "" {
				return "get_property_by_id", map[string]any{"property_id": entityID}
			}
		}
	case "deal":
		if wantsDetail || strings.Contains(msg, "checklist") || strings.Contains(msg, "document") {
			if _, ok := toolIndex["get_deal_by_id"]; ok && entityID != "" {
				return "get_deal_by_id", map[string]any{"deal_id": entityID}
			}
		}
	}

	switch {
	case wantsAnalytics:
		if (strings.Contains(msg, "lead") || strings.Contains(label, "lead")) && !strings.Contains(msg, "deal") && !strings.Contains(msg, "negóci") && !strings.Contains(msg, "negocia") {
			if _, ok := toolIndex["search_leads"]; ok {
				if limit < 50 {
					limit = 200
				}
				return "search_leads", map[string]any{"limit": limit}
			}
		}
		if _, ok := toolIndex["get_deals_pipeline"]; ok {
			return "get_deals_pipeline", map[string]any{}
		}
	case strings.Contains(msg, "lead") || strings.Contains(label, "lead"):
		if _, ok := toolIndex["search_leads"]; ok {
			return "search_leads", map[string]any{"limit": limit}
		}
	case strings.Contains(msg, "imóve") || strings.Contains(msg, "imove") || strings.Contains(msg, "propriedad") || strings.Contains(label, "imóve") || strings.Contains(label, "imove"):
		if _, ok := toolIndex["search_properties"]; ok {
			return "search_properties", map[string]any{"limit": limit}
		}
	case strings.Contains(msg, "agenda") || strings.Contains(msg, "compromiss") || strings.Contains(msg, "calend"):
		if _, ok := toolIndex["get_calendar_events"]; ok {
			return "get_calendar_events", map[string]any{"limit": limit}
		}
	case strings.Contains(msg, "pipeline") || strings.Contains(msg, "deal") || strings.Contains(msg, "negocia") || strings.Contains(msg, "negóci") || strings.Contains(msg, "esteira"):
		if _, ok := toolIndex["get_deals_pipeline"]; ok {
			return "get_deals_pipeline", map[string]any{}
		}
	}
	return "", nil
}

type suiteeAction struct {
	ActionID       string `json:"action_id"`
	ToolName       string `json:"tool_name"`
	Title          string `json:"title"`
	Description    string `json:"description,omitempty"`
	ConfirmMessage string `json:"confirm_message,omitempty"`
	Prefill        string `json:"prefill,omitempty"`
	PrimaryLabel   string `json:"primary_label,omitempty"`
	SecondaryLabel string `json:"secondary_label,omitempty"`
	InputsSchema   any    `json:"inputs_schema,omitempty"`
	InputsDefault  any    `json:"inputs_default,omitempty"`
}

func appendSuiteeActionBlocksIfMissing(content string, toolCalls []DeepseekToolCall, toolIndex map[string]DeepseekTool, screenContext any) string {
	if strings.Contains(content, "```suitee_action") {
		return content
	}

	blocks := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if t, ok := toolIndex[tc.Function.Name]; ok {
			if t.ReadOnly {
				continue
			}
		} else if toolIsReadOnly(tc.Function.Name) {
			continue
		}

		title := strings.ReplaceAll(strings.TrimSpace(tc.Function.Name), "_", " ")
		if title == "" {
			title = "Executar ação"
		} else {
			title = "Executar " + title
		}

		a := suiteeAction{
			ActionID:       tc.ID,
			ToolName:       tc.Function.Name,
			Title:          title,
			Description:    "Posso executar esta ação no sistema. Revise os campos e confirme para continuar.",
			ConfirmMessage: "CONFIRMAR " + tc.Function.Name,
			Prefill:        "CONFIRMAR " + tc.Function.Name,
			PrimaryLabel:   "Executar",
			SecondaryLabel: "Editar",
		}
		if t, ok := toolIndex[tc.Function.Name]; ok {
			a.InputsSchema = t.Function.Parameters
		}
		a.InputsDefault = enrichToolArgsForScreen(tc.Function.Name, parseToolArgs(tc.Function.Arguments), screenContext)

		b, _ := json.MarshalIndent(a, "", "  ")
		blocks = append(blocks, "```suitee_action\n"+string(b)+"\n```")
	}

	if len(blocks) == 0 {
		return content
	}
	if strings.TrimSpace(content) == "" {
		return strings.Join(blocks, "\n\n")
	}
	return strings.TrimSpace(content) + "\n\n" + strings.Join(blocks, "\n\n")
}

// buildNoToolSuggestions gera cards clicáveis com prompts alternativos
// baseados nas tools disponíveis e no contexto da tela. O usuário pode
// clicar em qualquer card para enviar o prompt diretamente.
func buildNoToolSuggestions(screenContext any, tools []DeepseekTool, userMessage string) string {
	label, _ := screenFieldsFromContext(screenContext)
	labelLow := strings.ToLower(strings.TrimSpace(label))

	available := map[string]bool{}
	for _, t := range tools {
		name := strings.TrimSpace(t.Function.Name)
		if name != "" {
			available[name] = t.ReadOnly
		}
	}

	var items []suiteeCardItem

	byDomain := func(domainKeyword string) bool {
		return strings.Contains(labelLow, domainKeyword)
	}

	switch {
	case byDomain("lead"):
		if available["search_leads"] {
			items = append(items, suiteeCardItem{
				Title:        "Listar leads recentes",
				Subtitle:     "Liste os 10 leads mais recentes com nome, telefone, status e corretor.",
				ActionPrompt: "Liste os 10 leads mais recentes com nome, telefone, status e corretor.",
			})
		}
		if available["get_calendar_events"] {
			items = append(items, suiteeCardItem{
				Title:        "Ver compromissos",
				Subtitle:     "Mostre meus compromissos de hoje e amanhã.",
				ActionPrompt: "Mostre meus compromissos de hoje e amanhã.",
			})
		}
		if available["get_deals_pipeline"] {
			items = append(items, suiteeCardItem{
				Title:        "Pipeline de negócios",
				Subtitle:     "Como está meu pipeline de negócios? Mostre quantidade e valor por etapa.",
				ActionPrompt: "Como está meu pipeline de negócios? Mostre quantidade e valor por etapa.",
			})
		}
		if available["search_properties"] {
			items = append(items, suiteeCardItem{
				Title:        "Buscar imóveis",
				Subtitle:     "Liste os imóveis disponíveis para venda com status e valor.",
				ActionPrompt: "Liste os imóveis disponíveis para venda com status e valor.",
			})
		}
	case byDomain("imóve") || byDomain("imove") || byDomain("propriedad"):
		if available["search_properties"] {
			items = append(items, suiteeCardItem{
				Title:        "Listar imóveis",
				Subtitle:     "Liste os imóveis com status, valor, tipo e bairro.",
				ActionPrompt: "Liste os imóveis com status, valor, tipo e bairro.",
			})
		}
		if available["get_calendar_events"] {
			items = append(items, suiteeCardItem{
				Title:        "Ver agenda de visitas",
				Subtitle:     "Quais visitas estão agendadas para esta semana?",
				ActionPrompt: "Quais visitas estão agendadas para esta semana?",
			})
		}
		if available["search_leads"] {
			items = append(items, suiteeCardItem{
				Title:        "Leads interessados",
				Subtitle:     "Liste os leads recentes que demonstraram interesse em imóveis.",
				ActionPrompt: "Liste os leads recentes que demonstraram interesse em imóveis.",
			})
		}
	case byDomain("pipeline") || byDomain("negóci") || byDomain("negocia") || byDomain("esteira") || byDomain("deal"):
		if available["get_deals_pipeline"] {
			items = append(items, suiteeCardItem{
				Title:        "Ver pipeline",
				Subtitle:     "Mostre o pipeline completo com quantidade e valor por etapa.",
				ActionPrompt: "Mostre o pipeline completo com quantidade e valor por etapa.",
			})
		}
		if available["search_leads"] {
			items = append(items, suiteeCardItem{
				Title:        "Leads em negociação",
				Subtitle:     "Liste os leads que estão em negociação ativa.",
				ActionPrompt: "Liste os leads que estão em negociação ativa.",
			})
		}
	case byDomain("agenda") || byDomain("calend") || byDomain("compromiss"):
		if available["get_calendar_events"] {
			items = append(items, suiteeCardItem{
				Title:        "Compromissos do dia",
				Subtitle:     "Mostre todos os meus compromissos de hoje com horário e local.",
				ActionPrompt: "Mostre todos os meus compromissos de hoje com horário e local.",
			})
		}
	case byDomain("financ"):
		if available["get_financial_dashboard"] {
			items = append(items, suiteeCardItem{
				Title:        "Resumo financeiro",
				Subtitle:     "Mostre o resumo financeiro do mês corrente.",
				ActionPrompt: "Mostre o resumo financeiro do mês corrente.",
			})
		}
		if available["search_charges"] {
			items = append(items, suiteeCardItem{
				Title:        "Cobranças em atraso",
				Subtitle:     "Mostre as cobranças em atraso com valor, cliente e dias de atraso.",
				ActionPrompt: "Mostre as cobranças em atraso com valor, cliente e dias de atraso.",
			})
		}
		if available["search_transfers"] {
			items = append(items, suiteeCardItem{
				Title:        "Repasses pendentes",
				Subtitle:     "Mostre os repasses pendentes com valor, proprietário e competência.",
				ActionPrompt: "Mostre os repasses pendentes com valor, proprietário e competência.",
			})
		}
	}

	// Fallback genérico: sugere as tools read-only mais comuns disponíveis
	if len(items) == 0 {
		if available["search_leads"] {
			items = append(items, suiteeCardItem{
				Title:        "Listar leads",
				Subtitle:     "Liste os leads mais recentes com status e corretor.",
				ActionPrompt: "Liste os leads mais recentes com status e corretor.",
			})
		}
		if available["get_calendar_events"] {
			items = append(items, suiteeCardItem{
				Title:        "Ver agenda",
				Subtitle:     "Mostre meus compromissos de hoje.",
				ActionPrompt: "Mostre meus compromissos de hoje.",
			})
		}
		if available["get_deals_pipeline"] {
			items = append(items, suiteeCardItem{
				Title:        "Pipeline",
				Subtitle:     "Como está meu pipeline de negócios?",
				ActionPrompt: "Como está meu pipeline de negócios?",
			})
		}
		if available["search_properties"] {
			items = append(items, suiteeCardItem{
				Title:        "Buscar imóveis",
				Subtitle:     "Liste os imóveis disponíveis.",
				ActionPrompt: "Liste os imóveis disponíveis.",
			})
		}
	}

	if len(items) == 0 {
		return "Não encontrei ferramentas disponíveis para processar sua solicitação. Tente descrever de forma mais específica o que você precisa."
	}

	// Limita a 4 cards
	if len(items) > 4 {
		items = items[:4]
	}

	intro := "Não encontrei uma ferramenta específica para **\"" + sanitizeForMarkdown(userMessage) + "\"**.\n\n" +
		"Que tal experimentar uma destas perguntas? **Clique no botão** do card para enviar:\n\n"

	return intro + mustSuiteeCards(suiteeCardsBlock{
		Title: "Sugestões — " + label,
		Items: items,
	})
}

func sanitizeForMarkdown(s string) string {
	s = strings.ReplaceAll(s, "*", "\\*")
	s = strings.ReplaceAll(s, "_", "\\_")
	s = strings.ReplaceAll(s, "`", "\\`")
	return s
}
