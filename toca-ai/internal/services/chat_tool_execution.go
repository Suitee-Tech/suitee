package services

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-ai/internal/domain"
)

// callAuthorizedTool executa a tool depois de checar a permissão do domínio
// (P1/A8). Negado → devolve erro de ferramenta em vez de executar: o modelo vê a
// recusa e o usuário não recebe um efeito que o papel dele não permite.
func (s *ChatService) callAuthorizedTool(ctx context.Context, session *MCPSession, name string, args any) (string, bool, error) {
	if err := s.authorizeToolCall(ctx, name); err != nil {
		return "permission denied for tool " + name, true, nil
	}
	return s.mcp.CallTool(ctx, session, name, args)
}

func (s *ChatService) tryExecutePendingToolCall(
	ctx context.Context,
	session *MCPSession,
	tenantID string,
	accountID string,
	conversationID string,
	history []domain.Message,
	messages []DeepseekMessage,
	toolIndex map[string]DeepseekTool,
	confirm domain.ActionConfirmRequest,
	screenContext any,
	emit SSEEmitter,
) bool {
	pending := findPendingToolCalls(history, toolIndex)
	if len(pending) == 0 {
		return false
	}

	var tc *DeepseekToolCall
	for i := range pending {
		if pending[i].ID == confirm.ActionID {
			tc = &pending[i]
			break
		}
	}
	if tc == nil {
		return false
	}

	args := parseToolArgs(tc.Function.Arguments)
	if confirm.Inputs != nil {
		if m, ok := args.(map[string]any); ok {
			for k, v := range confirm.Inputs {
				m[k] = v
			}
			args = m
		} else {
			args = confirm.Inputs
		}
	}
	args = enrichToolArgsForScreen(tc.Function.Name, args, screenContext)

	if emit != nil {
		_ = emit("tool_start", domain.ToolEvent{ToolName: tc.Function.Name, ToolCallID: tc.ID})
	}
	start := time.Now()
	out, hasError, _ := s.callAuthorizedTool(ctx, session, tc.Function.Name, args)
	duration := time.Since(start)
	preview := strings.TrimSpace(out)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	if emit != nil {
		_ = emit("tool_end", domain.ToolEndEvent{ToolCallID: tc.ID, ResultPreview: preview, HasError: hasError})
	}
	s.recordToolInvocation(ctx, tenantID, accountID, conversationID, tc.Function.Name, args, preview, !hasError, hasError, int(duration.Milliseconds()))

	if err := s.persistMessage(ctx, emit, conversationID, &domain.Message{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		ConversationID: conversationID,
		Role:           domain.RoleTool,
		Content:        out,
		ToolCallID:     tc.ID,
		ToolName:       tc.Function.Name,
	}); err != nil {
		return true
	}

	messages = append(messages, DeepseekMessage{
		Role:       "tool",
		ToolCallID: tc.ID,
		Name:       tc.Function.Name,
		Content:    out,
	})

	messages = append(messages, DeepseekMessage{
		Role: "system",
		Content: "Instruções: o usuário confirmou a execução da ação proposta anteriormente. " +
			"Responda diretamente com o resultado e próximos passos objetivos. Não cumprimente e não descreva capacidades.",
	})

	messages = sanitizeHistoryForDeepseek(messages)
	assistant, usage, err := s.deepseek.ChatNoTools(ctx, messages, s.maxTokens)
	if err != nil {
		if emit != nil {
			_ = emit("error", domain.ErrorEvent{Message: err.Error()})
			_ = emit("done", map[string]any{"conversation_id": conversationID})
		}
		return true
	}

	content := sanitizeAssistantOutput(strings.TrimSpace(assistant.Content))
	if err := s.persistMessage(ctx, emit, conversationID, &domain.Message{
		ID:             uuid.NewString(),
		TenantID:       tenantID,
		ConversationID: conversationID,
		Role:           domain.RoleAssistant,
		Content:        content,
		TokensIn:       usage.PromptTokens,
		TokensOut:      usage.CompletionTokens,
	}); err != nil {
		// Persistence failed, but still emit the response so the UI updates.
		// The error toast was already shown by persistMessage via SSEError path.
		if emit != nil {
			_ = emit("message", map[string]any{"conversation_id": conversationID, "content": content})
			_ = emit("done", map[string]any{"conversation_id": conversationID})
		}
		return true
	}

	if emit != nil {
		_ = emit("message", map[string]any{"conversation_id": conversationID, "content": content})
		_ = emit("done", map[string]any{"conversation_id": conversationID})
	}
	return true
}

func findPendingToolCalls(history []domain.Message, toolIndex map[string]DeepseekTool) []DeepseekToolCall {
	type wrapper struct {
		ToolCalls []DeepseekToolCall `json:"tool_calls"`
	}

	for i := len(history) - 1; i >= 0; i-- {
		m := history[i]
		if m.Role != domain.RoleAssistant || m.ToolCalls == nil {
			continue
		}

		var w wrapper
		b, _ := json.Marshal(m.ToolCalls)
		_ = json.Unmarshal(b, &w)
		if len(w.ToolCalls) == 0 {
			continue
		}

		if !requiresConfirmation(w.ToolCalls, toolIndex) {
			continue
		}

		executed := false
		for j := i + 1; j < len(history); j++ {
			if history[j].Role != domain.RoleTool {
				continue
			}
			for _, tc := range w.ToolCalls {
				if history[j].ToolCallID == tc.ID {
					executed = true
					break
				}
			}
			if executed {
				break
			}
		}
		if executed {
			continue
		}

		return w.ToolCalls
	}
	return nil
}

func parseToolArgs(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var obj any
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return map[string]any{}
	}
	return obj
}

func (s *ChatService) recordToolInvocation(ctx context.Context, tenantID, accountID, conversationID, toolName string, args any, preview string, success bool, hasError bool, durationMS int) {
	if s == nil || s.toolInvRepo == nil {
		return
	}
	argsMap := map[string]any{}
	if m, ok := args.(map[string]any); ok && m != nil {
		argsMap = m
	}
	if err := s.toolInvRepo.Create(context.WithoutCancel(ctx), &domain.ToolInvocation{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		TenantID:       tenantID,
		AccountID:      accountID,
		ToolName:       toolName,
		Args:           argsMap,
		ResultPreview:  preview,
		Success:        success,
		HasError:       hasError,
		DurationMS:     durationMS,
	}); err != nil {
		logger.Error(
			"Failed to persist tool invocation",
			"error", err.Error(),
			"tenant_id", tenantID,
			"account_id", accountID,
			"conversation_id", conversationID,
			"tool_name", toolName,
		)
	}
}
