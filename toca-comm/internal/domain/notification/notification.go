package notification

import (
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypeEmail    Type = "EMAIL"
	TypeSMS      Type = "SMS"
	TypePush     Type = "PUSH"
	TypeWhatsApp Type = "WHATSAPP"
)

type Status string

const (
	StatusPending Status = "PENDING"
	StatusSent    Status = "SENT"
	StatusFailed  Status = "FAILED"
)

type Notification struct {
	ID        string     `json:"id"`
	Type      Type       `json:"type"`
	Recipient string     `json:"recipient"` // Email, Phone, DeviceID
	Subject   string     `json:"subject,omitempty"`
	Content   string     `json:"content"`
	Status    Status     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	SentAt    *time.Time `json:"sent_at,omitempty"`
	Error     string     `json:"error,omitempty"`
}

func New(notifType Type, recipient, subject, content string) *Notification {
	return &Notification{
		ID:        uuid.New().String(),
		Type:      notifType,
		Recipient: recipient,
		Subject:   subject,
		Content:   content,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
}
