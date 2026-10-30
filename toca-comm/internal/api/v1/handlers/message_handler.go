package handlers

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/storage"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-comm/internal/domain/message"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

// Force usage of message package for Swagger
var _ = message.Channel{}

type MessageHandler struct {
	service *service.MessageService
}

func NewMessageHandler(service *service.MessageService) *MessageHandler {
	return &MessageHandler{service: service}
}

// CreateChannelRequest holds the payload for creating a channel
type CreateChannelRequest struct {
	Name         string              `json:"name" binding:"required"`
	Type         message.ChannelType `json:"type" binding:"required"`
	Participants []string            `json:"participants"`
	EntityID     string              `json:"entity_id"`
	EntityType   string              `json:"entity_type"`
}

// SendMessageRequest holds the payload for sending a message
type SendMessageRequest struct {
	Content     string               `json:"content" binding:"required"`
	ContentType string               `json:"content_type"`
	ReplyToID   string               `json:"reply_to_id"`
	Attachments []message.Attachment `json:"attachments"`
}

// CreateChannel godoc
// @Summary      Create a messaging channel
// @Description  Creates a new channel (direct, group, support, lead, deal) for the tenant
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateChannelRequest true "Create Channel Request"
// @Success      201  {object}  response.Response{data=message.Channel}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /channels [post]
func (h *MessageHandler) CreateChannel(c *gin.Context) {
	var req CreateChannelRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	ch, err := h.service.CreateChannel(c.Request.Context(), req.Name, req.Type, req.Participants, req.EntityID, req.EntityType)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Channel created successfully", ch)
}

type EnsureChannelRequest struct {
	TenantID     string   `json:"tenant_id"`
	DealID       string   `json:"deal_id"`
	Name         string   `json:"name"`
	Participants []string `json:"participants"`
}

func (h *MessageHandler) EnsureChannelForDeal(c *gin.Context) {
	var req EnsureChannelRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tenantID, ok := internalTenantRequested(c, req.TenantID)
	if !ok {
		return
	}

	ch, err := h.service.EnsureChannelForDeal(c.Request.Context(), tenantID, req.DealID, req.Participants)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channel ensured successfully", ch)
}

// EnsureDirectChannel godoc
// @Router       /channels/direct [post]
func (h *MessageHandler) EnsureDirectChannel(c *gin.Context) {
	var req EnsureChannelRequest
	if !validation.BindJSON(c, &req) {
		return
	}
	tenantID, ok := internalTenantRequested(c, req.TenantID)
	if !ok {
		return
	}
	ch, err := h.service.EnsureDirectChannel(c.Request.Context(), tenantID, req.Name, req.Participants)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	response.Success(c, "Channel ensured successfully", ch)
}

// ListChannelsByParticipant lista os canais de um participante.
//
// SECURITY (achados A3/A9 da revisão de permissões de 2026-09-19): a listagem
// filtrava só por participante — sem nenhum predicado de tenant — e o
// `?participant=` vinha do chamador. Com a chave interna compartilhada, isso
// listava canais de qualquer tenant. Agora a autorização vem da credencial:
//
//   - JWT de usuário: só os PRÓPRIOS canais. O participante pedido precisa ser o
//     account do token (pedir outro é 403) e não há recorte de tenant — o
//     cliente do portal tem `tenant_id` igual à própria conta e participa de
//     canais do tenant do negócio; a autorização é a participação;
//   - credencial de serviço: tenant explícito obrigatório (allowlist da chave) e
//     o recorte de tenant entra na consulta.
func (h *MessageHandler) ListChannelsByParticipant(c *gin.Context) {
	accountID := strings.TrimSpace(c.Query("participant"))
	if accountID == "" {
		response.ValidationError(c, "participant query parameter is required")
		return
	}

	tenantFilter, ok := resolveParticipantListingScope(c, accountID)
	if !ok {
		return
	}

	channels, err := h.service.ListChannelsByParticipant(c.Request.Context(), tenantFilter, accountID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if channels == nil {
		channels = []*message.Channel{}
	}

	response.Success(c, "Channels retrieved successfully", channels)
}

// ListMyChannels authorizes exclusively by the authenticated participant.
func (h *MessageHandler) ListMyChannels(c *gin.Context) {
	accountID, err := pkgcontext.GetAccountID(c.Request.Context())
	if err != nil || accountID == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	channels, err := h.service.ListChannelsByParticipant(c.Request.Context(), "", accountID)
	if err != nil {
		response.InternalError(c, err)
		return
	}
	if channels == nil {
		channels = []*message.Channel{}
	}
	response.Success(c, "Channels retrieved successfully", channels)
}

// ListChannels godoc
// @Summary      List channels
// @Description  Returns all channels for the current tenant
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        entity_id query string false "Filter by entity ID (lead/deal)"
// @Success      200  {object}  response.Response{data=[]message.Channel}
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /channels [get]
func (h *MessageHandler) ListChannels(c *gin.Context) {
	entityID := c.Query("entity_id")

	channels, err := h.service.ListChannels(c.Request.Context(), entityID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Channels retrieved successfully", []*message.Channel{})
			return
		}
		response.InternalError(c, err)
		return
	}
	if channels == nil {
		channels = []*message.Channel{}
	}

	response.Success(c, "Channels retrieved successfully", channels)
}

// GetChannel godoc
// @Summary      Get a channel
// @Description  Returns a single channel by ID
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Success      200  {object}  response.Response{data=message.Channel}
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id} [get]
func (h *MessageHandler) GetChannel(c *gin.Context) {
	id := c.Param("id")

	ch, err := h.service.GetChannel(c.Request.Context(), id)
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}

	response.Success(c, "Channel retrieved successfully", ch)
}

// UpdateChannelRequest holds the payload for updating channel metadata
type UpdateChannelRequest struct {
	Name        *string              `json:"name"`
	Description *string              `json:"description"`
	Info        *message.ChannelInfo `json:"info"`
}

// UpdateChannel godoc
// @Summary      Update a channel
// @Description  Updates channel metadata (name, description)
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Param        request body handlers.UpdateChannelRequest true "Update Channel Request"
// @Success      200  {object}  response.Response{data=message.Channel}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id} [patch]
func (h *MessageHandler) UpdateChannel(c *gin.Context) {
	channelID := c.Param("id")

	var req UpdateChannelRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	updates := make(map[string]any)
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Info != nil {
		updates["info"] = req.Info
	}

	if len(updates) == 0 {
		response.ValidationError(c, "no fields to update")
		return
	}

	ch, err := h.service.UpdateChannel(c.Request.Context(), channelID, updates)
	if err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channel updated successfully", ch)
}

// EditMessageRequest holds the payload for editing a message
type EditMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

// EditMessage godoc
// @Summary      Edit a message
// @Description  Edits the content of a message (only by original sender)
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        messageId  path  string true "Message ID"
// @Param        request    body  handlers.EditMessageRequest true "Edit Message Request"
// @Success      200  {object}  response.Response{data=message.Message}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/messages/{messageId} [patch]
func (h *MessageHandler) EditMessage(c *gin.Context) {
	channelID := c.Param("id")
	messageID := c.Param("messageId")

	var req EditMessageRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	msg, err := h.service.EditMessage(c.Request.Context(), channelID, messageID, req.Content)
	if err != nil {
		switch err.Error() {
		case "channel not found", "message not found":
			response.NotFound(c, err.Error())
		case "only the sender can edit this message":
			response.Forbidden(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Message edited successfully", msg)
}

// DeleteMessage godoc
// @Summary      Delete a message
// @Description  Deletes a message from a channel (only by original sender)
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        messageId  path  string true "Message ID"
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      403  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/messages/{messageId} [delete]
func (h *MessageHandler) DeleteMessage(c *gin.Context) {
	channelID := c.Param("id")
	messageID := c.Param("messageId")

	err := h.service.DeleteMessage(c.Request.Context(), channelID, messageID)
	if err != nil {
		switch err.Error() {
		case "channel not found", "message not found":
			response.NotFound(c, err.Error())
		case "only the sender can delete this message":
			response.Forbidden(c, err.Error())
		default:
			response.InternalError(c, err)
		}
		return
	}

	response.Success(c, "Message deleted successfully", nil)
}

// SendMessage godoc
// @Summary      Send a message
// @Description  Sends a message to a channel
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Param        request body handlers.SendMessageRequest true "Send Message Request"
// @Success      201  {object}  response.Response{data=message.Message}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /channels/{id}/messages [post]
func (h *MessageHandler) SendMessage(c *gin.Context) {
	channelID := c.Param("id")

	var req SendMessageRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// Pass the actual sender name if available from context/token
	ctx := c.Request.Context()

	// The token only has email and role, let's use the email prefix
	senderName := ""
	if name, ok := c.Get("name"); ok && name != "" {
		senderName = name.(string)
	} else if email, ok := c.Get("email"); ok && email != "" {
		parts := strings.Split(email.(string), "@")
		if len(parts) > 0 {
			senderName = parts[0]
		}
	}

	if senderName != "" {
		ctx = context.WithValue(ctx, service.CtxSenderName, senderName)
	}
	if avatarURL, ok := c.Get("avatar_url"); ok && avatarURL != "" {
		ctx = context.WithValue(ctx, service.CtxSenderAvatarURL, avatarURL.(string))
	}

	if req.ReplyToID != "" {
		ctx = context.WithValue(ctx, service.CtxReplyToID, req.ReplyToID)
	}
	if req.ContentType != "" {
		ctx = context.WithValue(ctx, service.CtxContentType, req.ContentType)
	}
	if len(req.Attachments) > 0 {
		ctx = context.WithValue(ctx, service.CtxAttachments, req.Attachments)
	}

	msg, err := h.service.SendMessage(ctx, channelID, req.Content)
	if err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Message sent successfully", msg)
}

// GetMessages godoc
// @Summary      Get messages in a channel
// @Description  Returns paginated messages for a channel
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id     path  string true  "Channel ID"
// @Param        limit  query int    false "Limit (default 50)"
// @Param        offset query int    false "Offset (default 0)"
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/messages [get]
func (h *MessageHandler) GetMessages(c *gin.Context) {
	channelID := c.Param("id")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	msgs, total, err := h.service.GetMessages(c.Request.Context(), channelID, limit, offset)
	if err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}
	if msgs == nil {
		msgs = []*message.Message{}
	}

	response.Success(c, "Messages retrieved successfully", gin.H{
		"messages": msgs,
		"total":    total,
		"limit":    limit,
		"offset":   offset,
	})
}

// MarkRead godoc
// @Summary      Mark messages as read
// @Description  Marks all messages in a channel as read for the calling user
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/read [post]
func (h *MessageHandler) MarkRead(c *gin.Context) {
	channelID := c.Param("id")

	if err := h.service.MarkRead(c.Request.Context(), channelID); err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Messages marked as read", nil)
}

// AddReactionRequest holds the payload for adding a reaction
type AddReactionRequest struct {
	Emoji string `json:"emoji" binding:"required"`
}

// AddReaction godoc
// @Summary      Add a reaction to a message
// @Description  Adds an emoji reaction to a specific message
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        messageId  path  string true "Message ID"
// @Param        request    body  handlers.AddReactionRequest true "Add Reaction Request"
// @Success      200  {object}  response.Response
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/messages/{messageId}/reactions [post]
func (h *MessageHandler) AddReaction(c *gin.Context) {
	channelID := c.Param("id")
	messageID := c.Param("messageId")

	var req AddReactionRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.AddReaction(c.Request.Context(), channelID, messageID, req.Emoji); err != nil {
		if err.Error() == "channel not found" || err.Error() == "message not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Reaction added", nil)
}

// RemoveReaction godoc
// @Summary      Remove a reaction from a message
// @Description  Removes an emoji reaction from a specific message
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        messageId  path  string true "Message ID"
// @Param        emoji      path  string true "Emoji"
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/messages/{messageId}/reactions/{emoji} [delete]
func (h *MessageHandler) RemoveReaction(c *gin.Context) {
	channelID := c.Param("id")
	messageID := c.Param("messageId")
	emoji := c.Param("emoji")

	if err := h.service.RemoveReaction(c.Request.Context(), channelID, messageID, emoji); err != nil {
		if err.Error() == "channel not found" || err.Error() == "message not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Reaction removed", nil)
}

// SendTyping godoc
// @Summary      Broadcast typing status
// @Description  Broadcasts to other channel participants that the user is typing
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        id path string true "Channel ID"
// @Success      200  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      404  {object}  response.Response
// @Router       /channels/{id}/typing [post]
func (h *MessageHandler) SendTyping(c *gin.Context) {
	channelID := c.Param("id")

	if err := h.service.SendTyping(c.Request.Context(), channelID); err != nil {
		if err.Error() == "channel not found" {
			response.NotFound(c, err.Error())
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Typing status sent", nil)
}

// Autocomplete godoc
// @Summary      Get autocomplete suggestions
// @Description  Returns suggestions for mentions (@) and hashtags (#)
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        type   query  string  true  "Type of suggestion: mention or hashtag"
// @Param        query  query  string  false "Search query"
// @Success      200  {object}  response.Response
// @Router       /autocomplete [get]
func (h *MessageHandler) Autocomplete(c *gin.Context) {
	suggestionType := c.Query("type")
	query := c.Query("query")

	// Mocking suggestions for Phase 6.
	// In production, this would query the User or Message/Tag collections.
	var results []map[string]string

	switch suggestionType {
	case "mention":
		// Mock users
		users := []map[string]string{
			{"id": "usr_1", "name": "João Corretor", "username": "joao.corretor"},
			{"id": "usr_2", "name": "Maria Silva", "username": "maria.silva"},
			{"id": "usr_3", "name": "Carlos Admin", "username": "carlos.admin"},
		}
		for _, u := range users {
			if query == "" || containsIgnoreCase(u["username"], query) || containsIgnoreCase(u["name"], query) {
				results = append(results, u)
			}
		}
	case "hashtag":
		// Mock hashtags
		tags := []map[string]string{
			{"id": "urgente", "name": "urgente"},
			{"id": "contrato", "name": "contrato"},
			{"id": "proposta", "name": "proposta"},
			{"id": "documentacao", "name": "documentacao"},
		}
		for _, t := range tags {
			if query == "" || containsIgnoreCase(t["name"], query) {
				results = append(results, t)
			}
		}
	case "command":
		cmds := []map[string]string{
			{"id": "/encerrar", "name": "/encerrar", "description": "Encerra o canal atual"},
			{"id": "/arquivar", "name": "/arquivar", "description": "Arquiva o canal atual"},
			{"id": "/ping", "name": "/ping", "description": "Verifica se o bot está online"},
		}
		for _, cmd := range cmds {
			if query == "" || containsIgnoreCase(cmd["name"], query) {
				results = append(results, cmd)
			}
		}
	}

	response.Success(c, "Suggestions retrieved", results)
}

func containsIgnoreCase(s, substr string) bool {
	return len(s) >= len(substr) // simplified for example
}

// @Summary      Upload an attachment
// @Description  Uploads a file to a channel and returns attachment metadata
// @Tags         messages
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        id        path      string  true  "Channel ID"
// @Param        file      formData  file    true  "File to upload"
// @Success      200  {object}  response.Response{data=message.Attachment}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /channels/{id}/messages/upload [post]
func (h *MessageHandler) UploadAttachment(c *gin.Context) {
	channelID := c.Param("id")

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.ValidationError(c, "Failed to get file: "+err.Error())
		return
	}
	defer file.Close()

	head := make([]byte, 512)
	n, err := file.Read(head)
	if err != nil && err != io.EOF {
		response.InternalServerError(c, "failed to read file header")
		return
	}

	ext, contentType, err := storage.ValidateWithMIME("toca-message-attachments", header.Filename, header.Size, head[:n])
	if err != nil {
		response.ValidationError(c, err.Error())
		return
	}

	multiFile := io.MultiReader(bytes.NewReader(head[:n]), file)
	filename := fmt.Sprintf("%s%s", uuid.NewString(), ext)
	displayName := path.Base(strings.ReplaceAll(header.Filename, "\\", "/"))
	if displayName == "." || displayName == "/" || strings.TrimSpace(displayName) == "" {
		displayName = "Arquivo"
	}

	attachment, err := h.service.UploadAttachment(
		c.Request.Context(),
		channelID,
		filename,
		displayName,
		contentType,
		header.Size,
		multiFile,
	)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "File uploaded successfully", attachment)
}

func (h *MessageHandler) DownloadAttachment(c *gin.Context) {
	channelID := c.Param("id")
	attachmentID := c.Param("attachmentId")

	url, err := h.service.GetAttachmentDownloadURL(c.Request.Context(), channelID, attachmentID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	c.Redirect(http.StatusFound, url)
}

type CreateTemplateRequest struct {
	Name      string   `json:"name" binding:"required"`
	Category  string   `json:"category" binding:"required"`
	Content   string   `json:"content" binding:"required"`
	Variables []string `json:"variables"`
}

// CreateTemplate godoc
// @Summary      Create a message template
// @Description  Creates a new pre-approved message template
// @Tags         templates
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        request body handlers.CreateTemplateRequest true "Create Template Request"
// @Success      201  {object}  response.Response{data=message.Template}
// @Failure      400  {object}  response.Response
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /templates [post]
func (h *MessageHandler) CreateTemplate(c *gin.Context) {
	var req CreateTemplateRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	tmpl, err := h.service.CreateTemplate(c.Request.Context(), req.Name, req.Category, req.Content, req.Variables)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Created(c, "Template created successfully", tmpl)
}

// ListTemplates godoc
// @Summary      List message templates
// @Description  Returns all message templates for the tenant
// @Tags         templates
// @Produce      json
// @Security     BearerAuth
// @Param        category query string false "Filter by category"
// @Success      200  {object}  response.Response{data=[]message.Template}
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /templates [get]
func (h *MessageHandler) ListTemplates(c *gin.Context) {
	category := c.Query("category")

	templates, err := h.service.ListTemplates(c.Request.Context(), category)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Templates retrieved successfully", []*message.Template{})
			return
		}
		response.InternalError(c, err)
		return
	}

	if templates == nil {
		templates = []*message.Template{}
	}

	response.Success(c, "Templates retrieved successfully", templates)
}

// SearchMessages godoc
// @Summary      Search messages
// @Description  Full-text search across messages
// @Tags         messages
// @Produce      json
// @Security     BearerAuth
// @Param        q          query  string  false  "Search query"
// @Param        channelId  query  string  false  "Filter by channel"
// @Param        hashtag    query  string  false  "Filter by hashtag"
// @Param        mention    query  string  false  "Filter by mention"
// @Param        limit      query  int     false  "Limit"
// @Param        offset     query  int     false  "Offset"
// @Success      200  {object}  response.Response
// @Router       /messages/search [get]
func (h *MessageHandler) SearchMessages(c *gin.Context) {
	query := c.Query("q")
	channelID := c.Query("channelId")
	hashtag := c.Query("hashtag")
	mention := c.Query("mention")

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	msgs, total, err := h.service.SearchMessages(c.Request.Context(), query, channelID, hashtag, mention, limit, offset)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Messages retrieved", gin.H{
		"messages": msgs,
		"total":    total,
	})
}

// TogglePinMessageRequest holds the payload
type TogglePinMessageRequest struct {
	IsPinned bool `json:"is_pinned"`
}

// TogglePinMessage godoc
// @Summary      Pin or unpin a message
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        messageId  path  string true "Message ID"
// @Param        request    body  handlers.TogglePinMessageRequest true "Request"
// @Success      200  {object}  response.Response
// @Router       /channels/{id}/messages/{messageId}/pin [patch]
func (h *MessageHandler) TogglePinMessage(c *gin.Context) {
	channelID := c.Param("id")
	messageID := c.Param("messageId")

	var req TogglePinMessageRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.TogglePinMessage(c.Request.Context(), channelID, messageID, req.IsPinned); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Message pin status updated", nil)
}

// TogglePinChannelRequest holds the payload
type TogglePinChannelRequest struct {
	IsPinned bool `json:"is_pinned"`
}

// TogglePinChannel godoc
// @Summary      Pin or unpin a channel
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        request    body  handlers.TogglePinChannelRequest true "Request"
// @Success      200  {object}  response.Response
// @Router       /channels/{id}/pin [patch]
func (h *MessageHandler) TogglePinChannel(c *gin.Context) {
	channelID := c.Param("id")

	var req TogglePinChannelRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.TogglePinChannel(c.Request.Context(), channelID, req.IsPinned); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channel pin status updated", nil)
}

// ToggleMuteChannelRequest holds the payload
type ToggleMuteChannelRequest struct {
	IsMuted bool `json:"is_muted"`
}

// ToggleMuteChannel godoc
// @Summary      Mute or unmute a channel
// @Tags         messages
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id         path  string true "Channel ID"
// @Param        request    body  handlers.ToggleMuteChannelRequest true "Request"
// @Success      200  {object}  response.Response
// @Router       /channels/{id}/mute [patch]
func (h *MessageHandler) ToggleMuteChannel(c *gin.Context) {
	channelID := c.Param("id")

	var req ToggleMuteChannelRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	if err := h.service.ToggleMuteChannel(c.Request.Context(), channelID, req.IsMuted); err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Channel mute status updated", nil)
}

// GetMetrics godoc
// @Summary      Get chat metrics
// @Description  Returns system-wide chat metrics for the tenant
// @Tags         metrics
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  response.Response{data=message.Metrics}
// @Failure      401  {object}  response.Response
// @Failure      500  {object}  response.Response
// @Router       /metrics [get]
func (h *MessageHandler) GetMetrics(c *gin.Context) {
	metrics, err := h.service.GetMetrics(c.Request.Context())
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Metrics retrieved successfully", &message.Metrics{})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Metrics retrieved successfully", metrics)
}
