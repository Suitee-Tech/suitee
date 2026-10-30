package handlers

import (
	"strconv"

	"github.com/gin-gonic/gin"
	pkgcontext "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/response"
	"github.com/oginoino/toca/pkg/validation"
	"github.com/oginoino/toca/toca-comm/internal/domain/notification"
	"github.com/oginoino/toca/toca-comm/internal/service"
)

type InAppNotificationHandler struct {
	service *service.InAppNotificationService
}

func NewInAppNotificationHandler(service *service.InAppNotificationService) *InAppNotificationHandler {
	return &InAppNotificationHandler{service: service}
}

type PushNotificationRequest struct {
	Title   string      `json:"title"`
	Message string      `json:"message"`
	Type    string      `json:"type"`
	Link    string      `json:"link,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func (h *InAppNotificationHandler) PushNotification(c *gin.Context) {
	accountID := c.Param("accountId")
	var req PushNotificationRequest
	if !validation.BindJSON(c, &req) {
		return
	}

	// Usually we need tenant ID for notification, we might need to extract it from data
	// or assume the comm service can handle it.
	// For now, we'll try to find a tenant ID or let the service handle it.
	tenantID := ""
	dataMap := make(map[string]interface{})
	if req.Data != nil {
		if dMap, ok := req.Data.(map[string]interface{}); ok {
			dataMap = dMap
			if tID, ok := dMap["tenant_id"].(string); ok {
				tenantID = tID
			}
		}
	}

	// P1/A3: o tenant pedido no corpo passa pelo resolver — credencial de serviço
	// só atende os tenants da allowlist e chamada com JWT usa o tenant do token.
	tenantID, ok := internalTenantRequested(c, tenantID)
	if !ok {
		return
	}

	err := h.service.Create(c.Request.Context(), tenantID, accountID, notification.InAppTypeSystem, notification.PriorityNormal, req.Title, req.Message, req.Link, dataMap)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Notification pushed successfully", nil)
}

// @Summary      List user notifications
// @Description  Get a paginated list of user notifications
// @Tags         notifications
// @Accept       json
// @Produce      json
// @Param        limit    query     int  false  "Limit"
// @Param        offset   query     int  false  "Offset"
// @Success      200      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /notifications/inapp [get]
func (h *InAppNotificationHandler) List(c *gin.Context) {
	tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
	userID, _ := pkgcontext.GetAccountID(c.Request.Context())

	limitStr := c.DefaultQuery("limit", "20")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, _ := strconv.Atoi(limitStr)
	offset, _ := strconv.Atoi(offsetStr)

	if limit <= 0 || limit > 100 {
		limit = 20
	}

	notifications, total, err := h.service.List(c.Request.Context(), tenantID, userID, limit, offset)
	if err != nil {
		if isNotImplemented(err) {
			page := (offset / limit) + 1
			response.Paginated(c, "Notifications retrieved", []*notification.InAppNotification{}, page, limit, 0)
			return
		}
		response.InternalError(c, err)
		return
	}

	page := (offset / limit) + 1
	response.Paginated(c, "Notifications retrieved", notifications, page, limit, int64(total))
}

// @Summary      Get unread count
// @Description  Get the number of unread notifications
// @Tags         notifications
// @Produce      json
// @Success      200      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /notifications/inapp/unread-count [get]
func (h *InAppNotificationHandler) GetUnreadCount(c *gin.Context) {
	tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
	userID, _ := pkgcontext.GetAccountID(c.Request.Context())

	count, err := h.service.GetUnreadCount(c.Request.Context(), tenantID, userID)
	if err != nil {
		if isNotImplemented(err) {
			response.Success(c, "Unread count retrieved", map[string]int{"count": 0})
			return
		}
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Unread count retrieved", map[string]int{"count": count})
}

// @Summary      Mark as read
// @Description  Mark a specific notification as read
// @Tags         notifications
// @Produce      json
// @Param        id       path      string  true  "Notification ID"
// @Success      200      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /notifications/inapp/{id}/read [put]
func (h *InAppNotificationHandler) MarkAsRead(c *gin.Context) {
	tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
	userID, _ := pkgcontext.GetAccountID(c.Request.Context())
	id := c.Param("id")

	err := h.service.MarkAsRead(c.Request.Context(), id, tenantID, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "Notification marked as read", nil)
}

// @Summary      Mark all as read
// @Description  Mark all notifications as read
// @Tags         notifications
// @Produce      json
// @Success      200      {object}  response.Response
// @Failure      500      {object}  response.Response
// @Router       /notifications/inapp/read-all [put]
func (h *InAppNotificationHandler) MarkAllAsRead(c *gin.Context) {
	tenantID, _ := pkgcontext.GetTenantID(c.Request.Context())
	userID, _ := pkgcontext.GetAccountID(c.Request.Context())

	err := h.service.MarkAllAsRead(c.Request.Context(), tenantID, userID)
	if err != nil {
		response.InternalError(c, err)
		return
	}

	response.Success(c, "All notifications marked as read", nil)
}
