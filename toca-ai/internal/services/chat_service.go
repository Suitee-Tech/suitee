package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	pkgAuth "github.com/oginoino/toca/pkg/auth"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/toca-ai/internal/domain"
	"github.com/oginoino/toca/toca-ai/internal/repositories"
)

type SSEEmitter func(eventType string, payload any) error

type ChatService struct {
	deepseek      *DeepseekClient
	mcp           *MCPClient
	prompts       *PromptBuilder
	repo          repositories.ConversationRepository
	toolInvRepo   repositories.ToolInvocationRepository
	maxIterations int
	maxTokens     int
	build         string
	// permissionProvider habilita a autorização por TOOL (P1/A8): tool de escrita
	// exige a permissão do domínio na matriz do tenant.
	permissionProvider pkgAuth.PermissionProvider
}

func NewChatService(deepseek *DeepseekClient, mcp *MCPClient, prompts *PromptBuilder, repo repositories.ConversationRepository, toolInvRepo repositories.ToolInvocationRepository, maxIterations int, maxTokens int) *ChatService {
	if maxIterations <= 0 {
		maxIterations = 5
	}
	if maxTokens <= 0 {
		maxTokens = 2048
	}
	return &ChatService{
		deepseek:      deepseek,
		mcp:           mcp,
		prompts:       prompts,
		repo:          repo,
		toolInvRepo:   toolInvRepo,
		maxIterations: maxIterations,
		maxTokens:     maxTokens,
		build:         "toca-ai-2026-04-26",
	}
}

func (s *ChatService) persistConversation(ctx context.Context, emit SSEEmitter, conv *domain.Conversation) error {
	if err := s.repo.CreateConversation(ctx, conv); err != nil {
		tenantID, _ := appCtx.GetTenantID(ctx)
		accountID, _ := appCtx.GetAccountID(ctx)
		logger.Error("Failed to persist conversation", map[string]interface{}{
			"error":           err.Error(),
			"conversation_id": conv.ID,
			"tenant_id":       tenantID,
			"account_id":      accountID,
		})
		return &SSEError{
			Message:        "Não consegui salvar o histórico desta conversa. Tente novamente.",
			Debug:          err.Error(),
			ConversationID: conv.ID,
		}
	}
	return nil
}

func (s *ChatService) persistMessage(ctx context.Context, emit SSEEmitter, conversationID string, msg *domain.Message) error {
	// context.WithoutCancel: a persistência deve SEMPRE completar, mesmo se o
	// cliente fechar a conexão SSE no meio (navegação, aborto do fetch, queda
	// do proxy). Cliente desconectado não pode deixar o histórico inconsistente
	// nem transformar a conversa em erro permanente.
	if err := s.repo.CreateMessage(context.WithoutCancel(ctx), msg); err != nil {
		tenantID, _ := appCtx.GetTenantID(ctx)
		accountID, _ := appCtx.GetAccountID(ctx)
		logger.Error("Failed to persist message", map[string]interface{}{
			"error":           err.Error(),
			"conversation_id": msg.ConversationID,
			"tenant_id":       tenantID,
			"account_id":      accountID,
		})
		return &SSEError{
			Message:        "Não consegui salvar o histórico desta conversa. Tente novamente.",
			Debug:          err.Error(),
			ConversationID: conversationID,
		}
	}
	return nil
}

func (s *ChatService) HandleChat(ctx context.Context, session *MCPSession, req domain.ChatRequest, emit SSEEmitter) error {
	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil {
		return err
	}
	accountID, err := appCtx.RequireAccountID(ctx)
	if err != nil {
		return err
	}
	userRole, _ := appCtx.GetUserRole(ctx)
	userPermission, _ := appCtx.GetUserPermission(ctx)

	convID := strings.TrimSpace(req.ConversationID)
	if req.Confirm != nil {
		if strings.TrimSpace(req.Confirm.ConversationID) != "" {
			convID = strings.TrimSpace(req.Confirm.ConversationID)
		}
		if convID == "" {
			if emit != nil {
				_ = emit("error", domain.ErrorEvent{Message: "conversation_id é obrigatório para confirmação"})
			}
			return nil
		}
	}

	isNewConversation := false
	if convID == "" {
		isNewConversation = true
		convID = uuid.NewString()
		title := strings.TrimSpace(req.Message)
		if len(title) > 80 {
			title = title[:80]
		}
		if err := s.persistConversation(ctx, emit, &domain.Conversation{
			ID:        convID,
			TenantID:  tenantID,
			AccountID: accountID,
			Title:     title,
			Metadata:  map[string]any{},
		}); err != nil {
			return err
		}
	}

	if req.Confirm == nil {
		userMsgID := uuid.NewString()
		if err := s.persistMessage(ctx, emit, convID, &domain.Message{
			ID:             userMsgID,
			TenantID:       tenantID,
			ConversationID: convID,
			Role:           domain.RoleUser,
			Content:        req.Message,
		}); err != nil {
			return err
		}
	}

	history, err := s.repo.ListMessages(ctx, convID, tenantID, accountID, 200)
	if err != nil {
		logger.Error(
			"Failed to list messages",
			"error", err.Error(),
			"tenant_id", tenantID,
			"account_id", accountID,
			"conversation_id", convID,
		)
		history = nil
	}
	history = trimHistory(history, 40)

	tools := []DeepseekTool{}
	hasMCP := false
	if session != nil {
		if t, err := s.mcp.ListTools(ctx, session); err == nil {
			tools = t
			hasMCP = true
		} else {
			logger.Error(
				"Failed to list MCP tools",
				"error", err.Error(),
				"tenant_id", tenantID,
				"account_id", accountID,
				"conversation_id", convID,
			)
			hasMCP = false
		}
	}
	toolIndex := indexToolsByName(tools)

	if emit != nil {
		_ = emit("message_start", domain.MessageStartEvent{
			ConversationID: convID,
			MCPConnected:   hasMCP,
			ToolsCount:     len(tools),
			Build:          s.build,
		})
	}

	screenLabel, screenPath := screenFieldsFromContext(req.ScreenContext)
	system := DeepseekMessage{Role: "system", Content: s.prompts.SystemPrompt(PromptContext{
		TenantID:       tenantID,
		AccountID:      accountID,
		UserRole:       userRole,
		UserPermission: userPermission,
		ScreenLabel:    screenLabel,
		ScreenPath:     screenPath,
		HasMCP:         hasMCP,
		AvailableTools: tools,
	})}
	messages := make([]DeepseekMessage, 0, len(history)+2)
	messages = append(messages, system)
	for _, m := range history {
		messages = append(messages, toDeepseekMessage(m))
	}
	if req.ScreenContext != nil {
		if b, err := json.Marshal(req.ScreenContext); err == nil && len(b) > 0 {
			messages = append(messages, DeepseekMessage{
				Role:    "system",
				Content: "Contexto da tela atual (use isso para sugerir ações e evitar suposições):\n" + string(b),
			})
		}
	}
	if req.ScreenContext != nil {
		entity, entityID := screenEntityFromContext(req.ScreenContext)
		if entityID == "" {
			_, p := screenFieldsFromContext(req.ScreenContext)
			entity, entityID = entityFromPath(p)
		}
		if entityID != "" {
			messages = append(messages, DeepseekMessage{
				Role: "system",
				Content: "Contexto de entidade:\n" +
					"- entity: " + entity + "\n" +
					"- entity_id: " + entityID + "\n" +
					"Instruções:\n" +
					"- Quando chamar ferramentas que aceitam lead_id/property_id/deal_id e o usuário não especificar, use entity_id como default.\n",
			})
		}
	}

	if isNewConversation {
		history, err = s.repo.ListMessages(ctx, convID, tenantID, accountID, 200)
		if err != nil {
			logger.Error(
				"Failed to list messages after conversation create",
				"error", err.Error(),
				"tenant_id", tenantID,
				"account_id", accountID,
				"conversation_id", convID,
			)
			history = nil
		}
	}

	if req.Confirm != nil {
		handled := s.tryExecutePendingToolCall(ctx, session, tenantID, accountID, convID, history, messages, toolIndex, *req.Confirm, req.ScreenContext, emit)
		if handled {
			return nil
		}
		if emit != nil {
			_ = emit("error", domain.ErrorEvent{Message: "ação para confirmação não encontrada"})
			_ = emit("done", map[string]any{"conversation_id": convID})
		}
		return nil
	}

	for iteration := 0; iteration < s.maxIterations; iteration++ {
		// Always sanitize before hitting DeepSeek — protects against orphan
		// tool_calls / tool messages from any source (trim boundaries, abandoned
		// confirmations, partial executions). Cheap O(n) scan.
		messages = sanitizeHistoryForDeepseek(messages)
		// Budget de histórico: resultados de tools truncados e total limitado
		// — sem isso payloads gigantes crescem até estourar o context window
		// do modelo (400) e a conversa fica permanentemente quebrada.
		messages = budgetHistoryForDeepseek(messages)

		var streamed strings.Builder
		assistant, usage, err := s.deepseek.ChatStream(ctx, messages, tools, s.maxTokens, func(text string) {
			if strings.TrimSpace(text) == "" {
				return
			}
			streamed.WriteString(text)
			if emit == nil {
				return
			}
			_ = emit("message_delta", domain.MessageDeltaEvent{ConversationID: convID, Content: text})
		})
		if err != nil {
			logger.Error(
				"DeepSeek stream failed",
				"error", err.Error(),
				"tenant_id", tenantID,
				"account_id", accountID,
				"conversation_id", convID,
				"iteration", iteration,
			)
			return &SSEError{
				Message:        err.Error(),
				Debug:          err.Error(),
				ConversationID: convID,
			}
		}
		fullContent := assistant.Content
		if strings.TrimSpace(fullContent) == "" {
			fullContent = streamed.String()
		}

		if len(assistant.ToolCalls) == 0 {
			content := strings.TrimSpace(fullContent)
			lastToolName, lastToolOut := findLastToolInMessages(messages)
			if lastToolName != "" && lastToolOut != "" && shouldReuseLastToolResult(req.Message) {
				if interpreted, ok := s.llmInterpretToolResult(ctx, req.Message, lastToolName, lastToolOut); ok && strings.TrimSpace(interpreted) != "" {
					content = interpreted
				}
			}
			if hasMCP && session != nil && len(tools) > 0 && shouldAttemptAutoToolRouting(content) {
				recentTools := buildRecentToolContextFromMessages(messages, 3)
				if decision, ok := s.llmRouteTool(ctx, req.Message, req.ScreenContext, tools, recentTools); ok {
					if decision.Action == "ask_user" && strings.TrimSpace(decision.Question) != "" {
						content = strings.TrimSpace(decision.Question)
					} else if decision.Action == "call_tool" && strings.TrimSpace(decision.ToolName) != "" {
						if t, ok := toolIndex[decision.ToolName]; ok && t.ReadOnly {
							args := decision.Args
							if args == nil {
								args = map[string]any{}
							}
							args = enrichToolArgsForScreen(decision.ToolName, args, req.ScreenContext).(map[string]any)
							if emit != nil {
								_ = emit("tool_start", domain.ToolEvent{ToolName: decision.ToolName, ToolCallID: "auto:" + decision.ToolName})
							}
							start := time.Now()
							out, hasError, _ := s.callAuthorizedTool(ctx, session, decision.ToolName, args)
							out = truncateToolResult(out)
							duration := time.Since(start)
							preview := strings.TrimSpace(out)
							if len(preview) > 200 {
								preview = preview[:200]
							}
							if emit != nil {
								_ = emit("tool_end", domain.ToolEndEvent{ToolCallID: "auto:" + decision.ToolName, ResultPreview: preview, HasError: hasError})
							}
							s.recordToolInvocation(ctx, tenantID, accountID, convID, decision.ToolName, args, preview, !hasError, hasError, int(duration.Milliseconds()))
							if !hasError {
								if interpreted, ok := s.llmInterpretToolResult(ctx, req.Message, decision.ToolName, out); ok && strings.TrimSpace(interpreted) != "" {
									content = interpreted
								}
							}
						}
					}
				}
			}
			if hasMCP && session != nil && len(tools) > 0 && shouldForceReadToolFallback(content, req.Message) {
				if toolName, args := fallbackReadToolForUserMessage(req.Message, req.ScreenContext, toolIndex); toolName != "" {
					if t, ok := toolIndex[toolName]; ok && t.ReadOnly {
						args = enrichToolArgsForScreen(toolName, args, req.ScreenContext).(map[string]any)
						if emit != nil {
							_ = emit("tool_start", domain.ToolEvent{ToolName: toolName, ToolCallID: "auto:" + toolName})
						}
						start := time.Now()
						out, hasError, _ := s.callAuthorizedTool(ctx, session, toolName, args)
						out = truncateToolResult(out)
						duration := time.Since(start)
						preview := strings.TrimSpace(out)
						if len(preview) > 200 {
							preview = preview[:200]
						}
						if emit != nil {
							_ = emit("tool_end", domain.ToolEndEvent{ToolCallID: "auto:" + toolName, ResultPreview: preview, HasError: hasError})
						}
						s.recordToolInvocation(ctx, tenantID, accountID, convID, toolName, args, preview, !hasError, hasError, int(duration.Milliseconds()))
						if !hasError {
							if interpreted, ok := s.llmInterpretToolResult(ctx, req.Message, toolName, out); ok && strings.TrimSpace(interpreted) != "" {
								content = interpreted
							} else if summary, ok := summarizeToolResult(toolName, out, req.Message); ok && strings.TrimSpace(summary) != "" {
								content = summary
							}
						}
					}
				}
			}
			if !strings.Contains(content, "```suitee_cards") && shouldAppendToolSummary(content) {
				lastToolName := ""
				lastToolOut := ""
				for i := len(messages) - 1; i >= 0; i-- {
					if messages[i].Role == "tool" && strings.TrimSpace(messages[i].Content) != "" {
						lastToolName = strings.TrimSpace(messages[i].Name)
						lastToolOut = messages[i].Content
						break
					}
				}
				if lastToolName != "" && lastToolOut != "" {
					if summary, ok := summarizeToolResult(lastToolName, lastToolOut, req.Message); ok && strings.TrimSpace(summary) != "" {
						content = strings.TrimSpace(content)
						if content == "" {
							content = summary
						} else {
							content = content + "\n\n" + summary
						}
					}
				}
			}
			content = sanitizeAssistantOutput(content)

			// Se nenhum fallback produziu conteúdo útil e há tools disponíveis,
			// sugere perguntas alternativas que usem as tools do contexto atual
			if strings.TrimSpace(content) == "" && hasMCP && session != nil && len(tools) > 0 {
				content = buildNoToolSuggestions(req.ScreenContext, tools, req.Message)
			}

			// Fallback quando MCP não está disponível e o modelo não produziu resposta
			if strings.TrimSpace(content) == "" {
				content = "No momento não consigo acessar dados do sistema. " +
					"Posso ajudar com perguntas gerais — tente descrever o que você precisa."
			}

			if err := s.persistMessage(ctx, emit, convID, &domain.Message{
				ID:             uuid.NewString(),
				TenantID:       tenantID,
				ConversationID: convID,
				Role:           domain.RoleAssistant,
				Content:        content,
				TokensIn:       usage.PromptTokens,
				TokensOut:      usage.CompletionTokens,
			}); err != nil {
				return err
			}
			if emit != nil {
				_ = emit("message", map[string]any{"conversation_id": convID, "content": content})
				_ = emit("done", map[string]any{"conversation_id": convID})
			}
			return nil
		}

		if requiresConfirmation(assistant.ToolCalls, toolIndex) {
			content := strings.TrimSpace(fullContent)
			content = appendSuiteeActionBlocksIfMissing(content, assistant.ToolCalls, toolIndex, req.ScreenContext)
			content = sanitizeAssistantOutput(content)

			toolCallsJSON := map[string]any{"tool_calls": assistant.ToolCalls}
			if err := s.persistMessage(ctx, emit, convID, &domain.Message{
				ID:             uuid.NewString(),
				TenantID:       tenantID,
				ConversationID: convID,
				Role:           domain.RoleAssistant,
				Content:        content,
				ToolCalls:      toolCallsJSON,
				TokensIn:       usage.PromptTokens,
				TokensOut:      usage.CompletionTokens,
			}); err != nil {
				return err
			}

			if emit != nil {
				_ = emit("message", map[string]any{"conversation_id": convID, "content": content})
				_ = emit("done", map[string]any{"conversation_id": convID})
			}
			return nil
		}

		toolCallsJSON := map[string]any{"tool_calls": assistant.ToolCalls}
		if err := s.persistMessage(ctx, emit, convID, &domain.Message{
			ID:             uuid.NewString(),
			TenantID:       tenantID,
			ConversationID: convID,
			Role:           domain.RoleAssistant,
			Content:        sanitizeAssistantOutput(strings.TrimSpace(fullContent)),
			ToolCalls:      toolCallsJSON,
			TokensIn:       usage.PromptTokens,
			TokensOut:      usage.CompletionTokens,
		}); err != nil {
			return err
		}

		messages = append(messages, DeepseekMessage{
			Role:      "assistant",
			Content:   fullContent,
			ToolCalls: assistant.ToolCalls,
		})

		type toolResult struct {
			callID  string
			name    string
			content string
		}

		results := make([]toolResult, len(assistant.ToolCalls))
		var wg sync.WaitGroup
		for i, tc := range assistant.ToolCalls {
			wg.Add(1)
			go func(i int, tc DeepseekToolCall) {
				defer wg.Done()
				if emit != nil {
					_ = emit("tool_start", domain.ToolEvent{ToolName: tc.Function.Name, ToolCallID: tc.ID})
				}

				args := enrichToolArgsForScreen(tc.Function.Name, parseToolArgs(tc.Function.Arguments), req.ScreenContext)

				// A chamada da ferramenta NÃO pode herdar o ciclo de vida do
				// request HTTP.
				//
				// `ctx` é `c.Request.Context()`: quando o request termina — ou o
				// cliente desconecta — o Go cancela esse contexto e leva junto
				// toda ferramenta em voo. Medido com instrumentação: a primeira
				// ferramenta passava de `ctx_err=""` para `context canceled` em
				// 1 ms, e as seguintes já entravam com o contexto cancelado
				// (`ms=0`). O resultado eram 46% das chamadas falhando com
				// "transport error: failed to send request: ... context canceled",
				// o stream fechando sem evento de conclusão e a resposta saindo
				// incompleta — porque as ferramentas que falharam nunca entravam
				// no contexto do modelo.
				//
				// `WithoutCancel` preserva os valores do contexto (tenant e token
				// de auth, que as ferramentas usam para chamar o gateway) e solta
				// só o cancelamento. O timeout individual continua valendo.
				baseCtx := context.WithoutCancel(ctx)

				// Per-tool timeout — when DeepSeek fans out N parallel tool calls
				// (e.g. one get_broker_performance per broker), a single hung
				// downstream call must NOT block wg.Wait() forever and freeze
				// the whole SSE stream. 60s is generous for any single MCP→
				// gateway round-trip.
				callCtx, cancel := context.WithTimeout(baseCtx, 60*time.Second)
				start := time.Now()
				out, hasError, _ := s.callAuthorizedTool(callCtx, session, tc.Function.Name, args)
				out = truncateToolResult(out)
				cancel()
				duration := time.Since(start)
				if errors.Is(callCtx.Err(), context.DeadlineExceeded) && (out == "" || !hasError) {
					out = fmt.Sprintf("tool %s timed out after 60s", tc.Function.Name)
					hasError = true
				}
				preview := strings.TrimSpace(out)
				if len(preview) > 200 {
					preview = preview[:200]
				}
				if emit != nil {
					_ = emit("tool_end", domain.ToolEndEvent{ToolCallID: tc.ID, ResultPreview: preview, HasError: hasError})
				}
				s.recordToolInvocation(ctx, tenantID, accountID, convID, tc.Function.Name, args, preview, !hasError, hasError, int(duration.Milliseconds()))

				results[i] = toolResult{
					callID:  tc.ID,
					name:    tc.Function.Name,
					content: out,
				}
			}(i, tc)
		}
		wg.Wait()

		for _, tr := range results {
			if err := s.persistMessage(ctx, emit, convID, &domain.Message{
				ID:             uuid.NewString(),
				TenantID:       tenantID,
				ConversationID: convID,
				Role:           domain.RoleTool,
				Content:        tr.content,
				ToolCallID:     tr.callID,
				ToolName:       tr.name,
			}); err != nil {
				return err
			}
			messages = append(messages, DeepseekMessage{
				Role:       "tool",
				ToolCallID: tr.callID,
				Name:       tr.name,
				Content:    tr.content,
			})
		}
	}

	if emit != nil {
		_ = emit("error", domain.ErrorEvent{Message: "max iterations exceeded"})
		_ = emit("done", map[string]any{"conversation_id": convID})
	}
	return nil
}
