package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime/debug"
	"strings"

	"github.com/gin-gonic/gin"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/logger"
	domain "github.com/oginoino/toca/toca-ai/internal/domain"
	"github.com/oginoino/toca/toca-ai/internal/services"
)

type ChatHandler struct {
	chat *services.ChatService
	mcp  *services.MCPClient
}

func NewChatHandler(chat *services.ChatService, mcp *services.MCPClient) *ChatHandler {
	return &ChatHandler{chat: chat, mcp: mcp}
}

func (h *ChatHandler) Chat(c *gin.Context) {
	ctx := c.Request.Context()

	// Vigia o cancelamento do request e registra QUEM cancelou.
	//
	// O sintoma e sempre o mesmo: o stream fecha em ~1,3s com "context
	// canceled", sem erro no servidor e sem abort no frontend (instrumentado).
	// Como `context canceled` e igual para "o cliente desconectou" e "alguem
	// cancelou aqui dentro", o log sozinho nao distingue — e a pilha de quem
	// chamou o cancel e a unica evidencia que separa os dois.
	go func() {
		<-ctx.Done()
		logger.Warn("contexto do request de chat terminou",
			"erro", ctx.Err().Error(),
			"stack", string(debug.Stack()),
		)
	}()

	var req domain.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if strings.TrimSpace(req.Message) == "" && req.Confirm == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	accountID, err := appCtx.RequireAccountID(ctx)
	if err != nil || strings.TrimSpace(accountID) == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	authToken, err := appCtx.GetAuthToken(ctx)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	emit := func(eventType string, payload any) error {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		_, _ = c.Writer.Write([]byte("event: " + eventType + "\n"))
		_, _ = c.Writer.Write([]byte("data: " + string(b) + "\n\n"))
		flusher.Flush()
		return nil
	}

	session, err := h.mcp.Connect(ctx, tenantID, authToken)
	if err != nil {
		logger.Error(
			"MCP connect failed — chat will continue without tools",
			"error", err.Error(),
			"tenant_id", tenantID,
			"account_id", accountID,
			"mcp_url", h.mcp.SanitizedServerURL(),
		)
		session = nil
	} else {
		defer session.Close()
	}

	if err := h.chat.HandleChat(ctx, session, req, emit); err != nil {
		conversationID := strings.TrimSpace(req.ConversationID)
		if conversationID == "" && req.Confirm != nil {
			conversationID = strings.TrimSpace(req.Confirm.ConversationID)
		}

		logger.Error(
			"Chat handler failed",
			"error", err.Error(),
			"tenant_id", tenantID,
			"account_id", accountID,
			"conversation_id", conversationID,
			"has_mcp_session", session != nil,
			"has_confirm", req.Confirm != nil,
		)

		isRelease := strings.ToLower(strings.TrimSpace(os.Getenv("APP_MODE"))) == "release"
		debugRequested := strings.TrimSpace(c.GetHeader("X-Debug")) == "1" || strings.TrimSpace(c.Query("debug")) == "1"
		exposeDebug := !isRelease
		if isRelease && debugRequested {
			userRole, _ := appCtx.GetUserRole(ctx)
			userPermission, _ := appCtx.GetUserPermission(ctx)
			functionalRole, _ := appCtx.GetFunctionalRole(ctx)
			isPrivileged := userRole == "admin" || userRole == "owner" || userRole == "service" ||
				userPermission == "admin" || userPermission == "owner" || userPermission == "service" ||
				functionalRole == "admin" || functionalRole == "owner" || functionalRole == "service"
			exposeDebug = isPrivileged
		}
		userMessage := "Não consegui salvar ou processar esta conversa. Tente novamente."
		debugMessage := ""
		if sseErr, ok := err.(*services.SSEError); ok && sseErr != nil {
			if strings.TrimSpace(sseErr.Message) != "" {
				userMessage = sseErr.Message
			}
			if strings.TrimSpace(sseErr.ConversationID) != "" {
				conversationID = strings.TrimSpace(sseErr.ConversationID)
			}
			debugMessage = sseErr.Debug
		}

		ev := domain.ErrorEvent{Message: userMessage}
		if exposeDebug {
			if strings.TrimSpace(debugMessage) != "" {
				ev.Debug = debugMessage
			} else {
				ev.Debug = err.Error()
			}
		}
		_ = emit("error", ev)
		_ = emit("done", map[string]any{"conversation_id": conversationID})
	}
}

func (h *ChatHandler) Tools(c *gin.Context) {
	ctx := c.Request.Context()

	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	authToken, err := appCtx.GetAuthToken(ctx)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	endpoint, _ := h.mcp.ResolveSSEEndpoint()
	session, err := h.mcp.Connect(ctx, tenantID, authToken)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"mcp_connected":       false,
			"tools":               []any{},
			"mcp_endpoint":        endpoint,
			"mcp_server_url_used": h.mcp.SanitizedServerURL(),
			"error":               err.Error(),
		})
		return
	}
	defer session.Close()

	tools, err := h.mcp.ListTools(ctx, session)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"mcp_connected":       false,
			"tools":               []any{},
			"mcp_endpoint":        endpoint,
			"mcp_server_url_used": h.mcp.SanitizedServerURL(),
			"error":               err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"mcp_connected":       true,
		"tools":               tools,
		"mcp_endpoint":        endpoint,
		"mcp_server_url_used": h.mcp.SanitizedServerURL(),
	})
}
