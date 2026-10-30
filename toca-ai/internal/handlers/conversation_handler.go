package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/toca-ai/internal/repositories"
)

type ConversationHandler struct {
	repo repositories.ConversationRepository
}

func NewConversationHandler(repo repositories.ConversationRepository) *ConversationHandler {
	return &ConversationHandler{repo: repo}
}

func (h *ConversationHandler) List(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accountID, err := appCtx.RequireAccountID(ctx)
	if err != nil || strings.TrimSpace(accountID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	convs, err := h.repo.ListConversations(ctx, tenantID, accountID, limit)
	if err != nil {
		response.InternalServerError(c, "failed to list conversations")
		return
	}
	response.Success(c, "ok", convs)
}

func (h *ConversationHandler) Delete(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accountID, err := appCtx.RequireAccountID(ctx)
	if err != nil || strings.TrimSpace(accountID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	id := c.Param("id")
	if id == "" {
		response.ValidationError(c, "id required")
		return
	}

	ok, err := h.repo.SoftDeleteConversation(ctx, id, tenantID, accountID)
	if err != nil {
		response.InternalServerError(c, "failed to delete conversation")
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	response.Success(c, "ok", gin.H{"deleted": true})
}

func (h *ConversationHandler) Clear(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accountID, err := appCtx.RequireAccountID(ctx)
	if err != nil || strings.TrimSpace(accountID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	deleted, err := h.repo.SoftDeleteAllConversations(ctx, tenantID, accountID)
	if err != nil {
		response.InternalServerError(c, "failed to clear conversations")
		return
	}
	response.Success(c, "ok", gin.H{"deleted": deleted})
}

func (h *ConversationHandler) ListMessages(c *gin.Context) {
	ctx := c.Request.Context()
	tenantID, err := appCtx.RequireTenantID(ctx)
	if err != nil || strings.TrimSpace(tenantID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}
	accountID, err := appCtx.RequireAccountID(ctx)
	if err != nil || strings.TrimSpace(accountID) == "" {
		response.Unauthorized(c, "unauthorized")
		return
	}

	conversationID := c.Param("id")
	if conversationID == "" {
		response.ValidationError(c, "conversation id required")
		return
	}

	limit := 200
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	msgs, err := h.repo.ListMessages(ctx, conversationID, tenantID, accountID, limit)
	if err != nil {
		response.InternalServerError(c, "failed to list messages")
		return
	}
	response.Success(c, "ok", msgs)
}
