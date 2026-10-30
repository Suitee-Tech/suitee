package notification

import (
	"time"

	"github.com/google/uuid"
)

type InAppType string

const (
	InAppTypeChatMessage  InAppType = "chat_message"
	InAppTypeLeadAssigned InAppType = "lead_assigned"
	InAppTypeSystem       InAppType = "system"
)

type Priority string

const (
	PriorityUrgent    Priority = "urgent"
	PriorityImportant Priority = "important"
	PriorityNormal    Priority = "normal"
)

type InAppNotification struct {
	ID        string                 `json:"id" bson:"id"`
	TenantID  string                 `json:"tenant_id" bson:"tenantid"`
	UserID    string                 `json:"user_id" bson:"userid"`
	Type      InAppType              `json:"type" bson:"type"`
	Priority  Priority               `json:"priority" bson:"priority"`
	Title     string                 `json:"title" bson:"title"`
	Content   string                 `json:"content" bson:"content"`
	Link      string                 `json:"link,omitempty" bson:"link,omitempty"`
	Data      map[string]interface{} `json:"data,omitempty" bson:"data,omitempty"`
	IsRead    bool                   `json:"is_read" bson:"isread"`
	ReadAt    *time.Time             `json:"read_at,omitempty" bson:"readat,omitempty"`
	CreatedAt time.Time              `json:"created_at" bson:"createdat"`
}

func NewInApp(tenantID, userID string, notifType InAppType, priority Priority, title, content, link string, data map[string]interface{}) *InAppNotification {
	return &InAppNotification{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		UserID:    userID,
		Type:      notifType,
		Priority:  priority,
		Title:     title,
		Content:   content,
		Link:      link,
		Data:      data,
		IsRead:    false,
		CreatedAt: time.Now(),
	}
}
