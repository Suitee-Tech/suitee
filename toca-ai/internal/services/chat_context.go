package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-ai/internal/domain"
)

// maxToolResultChars limita o tamanho de resultados de tools persistidos e
// enviados ao modelo. Ferramentas de abstração (listas completas de leads,
// documentos, transações) podem devolver centenas de KB; guardar o payload
// integral no histórico faz a conversa estourar o context window do modelo
// (DeepSeek 400 "maximum context length") e ficar permanentemente inutilizável
// — cada retry reenvia o mesmo histórico gigante.
const maxToolResultChars = 40_000

// truncateToolResult encurta qualquer conteúdo acima do teto, preservando o
// início (que carrega a estrutura da resposta) e sinalizando o corte.
func truncateToolResult(s string) string {
	if len(s) <= maxToolResultChars {
		return s
	}
	return s[:maxToolResultChars] +
		fmt.Sprintf("\n… (resultado truncado: %d caracteres omitidos)", len(s)-maxToolResultChars)
}

// maxHistoryChars é o teto do volume total de histórico enviado ao modelo.
// O context window do DeepSeek é de ~1M tokens (≈4M chars); ficamos bem
// abaixo para reservar espaço para system + tools + completions.
const maxHistoryChars = 2_000_000

// budgetHistoryForDeepseek aplica dois limites antes de chamar o modelo:
//  1. cada mensagem (tool results inclusive) respeita maxToolResultChars;
//  2. o total do histórico respeita maxHistoryChars — quando estoura, os
//     registros mais antigos (nunca system) são descartados e a lista é
//     re-sanitizada para não cortar blocos tool no meio (senão 400).
func budgetHistoryForDeepseek(messages []DeepseekMessage) []DeepseekMessage {
	if len(messages) == 0 {
		return messages
	}
	for i := range messages {
		messages[i].Content = truncateToolResult(messages[i].Content)
	}
	total := 0
	for _, m := range messages {
		total += len(m.Content)
	}
	if total <= maxHistoryChars {
		return messages
	}
	start := 0
	for start < len(messages) && total > maxHistoryChars {
		if messages[start].Role != "system" {
			total -= len(messages[start].Content)
		}
		start++
	}
	if start >= len(messages) {
		// Caso extremo (mensagens individuais enormes): fica com a última
		// mensagem — mesmo sem contexto, não há resposta possível além dela.
		start = len(messages) - 1
		if start < 0 {
			start = 0
		}
	}
	out := append([]DeepseekMessage{}, messages[start:]...)
	// O corte pode ter quebrado um bloco tool (tool_message sem o assistant
	// com tool_calls). Recua até o assistant correspondente para não produzir
	// orphan tool e causar 400.
	if len(out) > 0 && out[0].Role == "tool" {
		start2 := start
		for start2 > 0 && messages[start2-1].Role == "tool" {
			start2--
		}
		if start2 > 0 && messages[start2-1].Role == "assistant" {
			start2--
			out = append([]DeepseekMessage{messages[start2]}, out...)
		}
	}
	return sanitizeHistoryForDeepseek(out)
}

func toDeepseekMessage(m domain.Message) DeepseekMessage {
	msg := DeepseekMessage{
		Role:    string(m.Role),
		Content: m.Content,
	}
	if m.Role == domain.RoleTool {
		msg.Role = "tool"
		msg.ToolCallID = m.ToolCallID
		msg.Name = m.ToolName
	}
	if m.Role == domain.RoleAssistant && m.ToolCalls != nil {
		var tcWrapper struct {
			ToolCalls []DeepseekToolCall `json:"tool_calls"`
		}
		b, _ := json.Marshal(m.ToolCalls)
		_ = json.Unmarshal(b, &tcWrapper)
		if len(tcWrapper.ToolCalls) > 0 {
			msg.ToolCalls = tcWrapper.ToolCalls
		}
	}
	return msg
}

// trimHistory keeps the tail of the conversation up to `max` messages while
// preserving tool-call atomicity. A naive `in[len(in)-max:]` can place the
// cut in the middle of an `assistant(tool_calls) → tool → tool` block, leaving
// either an orphan tool response at the start or an assistant with unmatched
// tool_calls — both produce DeepSeek 400 ("insufficient tool messages...").
//
// We never cut inside such a block: the start index is moved BACK to include
// the assistant message that preceded any tool messages at the boundary. If a
// pending assistant(tool_calls) is included but its tool responses were already
// trimmed away, it gets dropped by sanitizeHistoryForDeepseek before send.
func trimHistory(in []domain.Message, max int) []domain.Message {
	if max <= 0 {
		return in
	}
	if len(in) <= max {
		return in
	}
	start := len(in) - max
	// If the tentative start is a `tool` message, walk backward to include the
	// preceding assistant(tool_calls). Then keep walking back while the new
	// start is itself a tool message tail of an earlier assistant.
	for start > 0 && in[start].Role == domain.RoleTool {
		start--
	}
	// At this point start may sit on an assistant. If that assistant has
	// tool_calls and the tail of the slice still contains its tool responses,
	// we keep it; otherwise sanitizeHistoryForDeepseek will drop it later.
	return in[start:]
}

// sanitizeHistoryForDeepseek removes any structural inconsistency from the
// message list before it goes to the DeepSeek API. It enforces the OpenAI tool
// calling contract:
//
//   - every `tool` message must follow an assistant message in the same slice
//     whose tool_calls include the matching tool_call_id;
//   - every assistant message with `tool_calls` must be followed by a `tool`
//     message for EACH tool_call_id before any further assistant/user content.
//
// We drop messages that violate these rules instead of failing — better to send
// a slightly shorter context than to refuse the call. This protects against:
//  1. trimHistory boundaries that lost some tool responses;
//  2. abandoned conversations where the user closed the chat before confirming
//     a write tool, leaving an orphan assistant(tool_calls);
//  3. partial executions in tryExecutePendingToolCall that resolved only one
//     of multiple tool_calls.
func sanitizeHistoryForDeepseek(messages []DeepseekMessage) []DeepseekMessage {
	if len(messages) == 0 {
		return messages
	}

	out := make([]DeepseekMessage, 0, len(messages))

	// First pass: index assistant messages with tool_calls so we can decide
	// up-front whether each one has all of its tool responses present.
	for i := 0; i < len(messages); i++ {
		m := messages[i]

		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			// Collect required IDs.
			required := make(map[string]bool, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				required[tc.ID] = true
			}
			// Walk forward until we hit another assistant/user/system message,
			// collecting tool responses by id.
			seen := make(map[string]bool, len(m.ToolCalls))
			j := i + 1
			for j < len(messages) {
				nxt := messages[j]
				if nxt.Role != "tool" {
					break
				}
				if nxt.ToolCallID != "" && required[nxt.ToolCallID] {
					seen[nxt.ToolCallID] = true
				}
				j++
			}
			complete := len(seen) == len(required)
			if !complete {
				// Drop this assistant entry AND any partial tool responses it
				// produced; jump i past them.
				missing := make([]string, 0, len(required)-len(seen))
				for id := range required {
					if !seen[id] {
						missing = append(missing, id)
					}
				}
				logger.Warn(
					"sanitizeHistoryForDeepseek: dropping orphan assistant tool_calls",
					"required_count", len(required),
					"seen_count", len(seen),
					"missing_ids", missing,
				)
				i = j - 1
				continue
			}
			// Keep the assistant + all matching tool responses; emit them now
			// and advance.
			out = append(out, m)
			for k := i + 1; k < j; k++ {
				if messages[k].Role == "tool" && required[messages[k].ToolCallID] {
					out = append(out, messages[k])
				}
			}
			i = j - 1
			continue
		}

		if m.Role == "tool" {
			// Orphan tool message (no preceding assistant with matching id, or
			// preceded one that we already emitted/dropped above). The matching
			// case is already handled inside the assistant branch via the inner
			// j-loop, so reaching here means orphan — skip it.
			continue
		}

		out = append(out, m)
	}
	return out
}

func screenFieldsFromContext(screenContext any) (string, string) {
	m, ok := screenContext.(map[string]any)
	if !ok || m == nil {
		return "", ""
	}
	label, _ := m["label"].(string)
	path, _ := m["path"].(string)
	return strings.TrimSpace(label), strings.TrimSpace(path)
}

func entityFromPath(path string) (string, string) {
	p := strings.TrimSpace(path)
	p = strings.TrimSuffix(p, "/")
	if p == "" {
		return "", ""
	}
	if m := regexp.MustCompile(`^/leads/([^/]+)$`).FindStringSubmatch(p); len(m) == 2 {
		return "lead", m[1]
	}
	if m := regexp.MustCompile(`^/(imoveis|properties)/([^/]+)$`).FindStringSubmatch(p); len(m) == 3 {
		return "property", m[2]
	}
	if m := regexp.MustCompile(`^/esteira/deals/([^/]+)$`).FindStringSubmatch(p); len(m) == 2 {
		return "deal", m[1]
	}
	return "", ""
}

func screenEntityFromContext(screenContext any) (string, string) {
	m, ok := screenContext.(map[string]any)
	if !ok || m == nil {
		return "", ""
	}
	entity, _ := m["entity"].(string)
	entityID, _ := m["entity_id"].(string)
	entity = strings.TrimSpace(entity)
	entityID = strings.TrimSpace(entityID)
	if entity != "" && entityID != "" {
		return entity, entityID
	}
	return entity, entityID
}

func findFirstStringByKeys(v any, keys map[string]struct{}) string {
	switch t := v.(type) {
	case map[string]any:
		for k := range keys {
			if val, ok := t[k]; ok {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
		for _, val := range t {
			if s := findFirstStringByKeys(val, keys); s != "" {
				return s
			}
		}
	case []any:
		for _, it := range t {
			if s := findFirstStringByKeys(it, keys); s != "" {
				return s
			}
		}
	}
	return ""
}

func screenLeadIDFromContext(screenContext any) string {
	entity, entityID := screenEntityFromContext(screenContext)
	if entity == "lead" && entityID != "" {
		return entityID
	}
	_, path := screenFieldsFromContext(screenContext)
	if entity, entityID = entityFromPath(path); entity == "lead" && entityID != "" {
		return entityID
	}
	return findFirstStringByKeys(screenContext, map[string]struct{}{
		"lead_id":        {},
		"leadId":         {},
		"selectedLeadId": {},
		"currentLeadId":  {},
	})
}

func screenPropertyIDFromContext(screenContext any) string {
	entity, entityID := screenEntityFromContext(screenContext)
	if entity == "property" && entityID != "" {
		return entityID
	}
	_, path := screenFieldsFromContext(screenContext)
	if entity, entityID = entityFromPath(path); entity == "property" && entityID != "" {
		return entityID
	}
	return findFirstStringByKeys(screenContext, map[string]struct{}{
		"property_id":        {},
		"propertyId":         {},
		"selectedPropertyId": {},
		"currentPropertyId":  {},
	})
}

func screenDealIDFromContext(screenContext any) string {
	entity, entityID := screenEntityFromContext(screenContext)
	if entity == "deal" && entityID != "" {
		return entityID
	}
	_, path := screenFieldsFromContext(screenContext)
	if entity, entityID = entityFromPath(path); entity == "deal" && entityID != "" {
		return entityID
	}
	return findFirstStringByKeys(screenContext, map[string]struct{}{
		"deal_id":        {},
		"dealId":         {},
		"selectedDealId": {},
		"currentDealId":  {},
	})
}

func enrichToolArgsForScreen(toolName string, args any, screenContext any) any {
	m, ok := args.(map[string]any)
	if !ok || m == nil {
		return args
	}
	switch strings.TrimSpace(toolName) {
	case "add_lead_note_from_screen", "add_lead_note", "update_lead", "get_lead_by_id":
		if v, _ := m["lead_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenLeadIDFromContext(screenContext); id != "" {
				m["lead_id"] = id
			}
		}
	case "get_property_by_id", "set_property_active_deal":
		if v, _ := m["property_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenPropertyIDFromContext(screenContext); id != "" {
				m["property_id"] = id
			}
		}
	case "link_lead_to_property", "unlink_lead_from_property":
		if v, _ := m["lead_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenLeadIDFromContext(screenContext); id != "" {
				m["lead_id"] = id
			}
		}
		if v, _ := m["property_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenPropertyIDFromContext(screenContext); id != "" {
				m["property_id"] = id
			}
		}
	case "create_deal_from_lead_and_property":
		if v, _ := m["lead_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenLeadIDFromContext(screenContext); id != "" {
				m["lead_id"] = id
			}
		}
		if v, _ := m["property_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenPropertyIDFromContext(screenContext); id != "" {
				m["property_id"] = id
			}
		}
		if _, ok := m["link_interested"]; !ok {
			m["link_interested"] = true
		}
		if _, ok := m["set_active_deal"]; !ok {
			m["set_active_deal"] = true
		}
	case "update_deal_status", "get_deal_by_id":
		if v, _ := m["deal_id"].(string); strings.TrimSpace(v) == "" {
			if id := screenDealIDFromContext(screenContext); id != "" {
				m["deal_id"] = id
			}
		}
	}
	return m
}

func shouldAttemptAutoToolRouting(assistantContent string) bool {
	c := strings.TrimSpace(assistantContent)
	if c == "" {
		return true
	}
	return isLikelyBoilerplate(c)
}

func shouldReuseLastToolResult(userMessage string) bool {
	u := strings.ToLower(strings.TrimSpace(userMessage))
	if u == "" {
		return false
	}
	if strings.Contains(u, "atualiz") ||
		strings.Contains(u, "recalcule") ||
		strings.Contains(u, "recalcular") ||
		strings.Contains(u, "de novo") ||
		strings.Contains(u, "novamente") ||
		strings.Contains(u, "recarreg") {
		return false
	}
	return true
}

func findLastToolInMessages(messages []DeepseekMessage) (string, string) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "tool" && strings.TrimSpace(messages[i].Content) != "" {
			return strings.TrimSpace(messages[i].Name), messages[i].Content
		}
	}
	return "", ""
}

func buildRecentToolContextFromMessages(messages []DeepseekMessage, limit int) string {
	if limit <= 0 {
		limit = 3
	}
	type it struct {
		name    string
		preview string
	}
	items := make([]it, 0, limit)
	seen := map[string]struct{}{}
	for i := len(messages) - 1; i >= 0 && len(items) < limit; i-- {
		if messages[i].Role != "tool" {
			continue
		}
		name := strings.TrimSpace(messages[i].Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		p := strings.TrimSpace(messages[i].Content)
		if len(p) > 240 {
			p = p[:240]
		}
		items = append(items, it{name: name, preview: p})
	}
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Ferramentas já executadas nesta conversa (para evitar repetir chamadas):\n")
	for _, x := range items {
		b.WriteString("- ")
		b.WriteString(x.name)
		if x.preview != "" {
			b.WriteString(" (preview: ")
			b.WriteString(strings.ReplaceAll(x.preview, "\n", " "))
			b.WriteString(")")
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func shouldAppendToolSummary(content string) bool {
	c := strings.TrimSpace(content)
	if c == "" {
		return true
	}
	low := strings.ToLower(c)
	if strings.Contains(low, "o que você gostaria de fazer") ||
		strings.Contains(low, "como posso ajudar") ||
		strings.Contains(low, "aqui estão algumas ações") ||
		strings.Contains(low, "posso ajudar com") {
		return true
	}
	if strings.HasSuffix(c, "?") && len([]rune(c)) < 90 {
		return true
	}
	return false
}
