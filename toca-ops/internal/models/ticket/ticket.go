package ticket

import (
	"time"

	"github.com/google/uuid"
)

type TicketType string

const (
	TicketTypeSupport       TicketType = "support"
	TicketTypeMaintenance   TicketType = "maintenance"
	TicketTypeBilling       TicketType = "billing"
	TicketTypeDocumentation TicketType = "documentation"
	TicketTypeGeneral       TicketType = "general"
)

type TicketStatus string

const (
	TicketStatusOpen           TicketStatus = "open"
	TicketStatusAssigned       TicketStatus = "assigned"
	TicketStatusInProgress     TicketStatus = "in_progress"
	TicketStatusAwaitingClient TicketStatus = "awaiting_client"
	TicketStatusResolved       TicketStatus = "resolved"
	TicketStatusClosed         TicketStatus = "closed"
)

type TicketPriority string

const (
	TicketPriorityLow    TicketPriority = "low"
	TicketPriorityMedium TicketPriority = "medium"
	TicketPriorityHigh   TicketPriority = "high"
	TicketPriorityUrgent TicketPriority = "urgent"
)

type Ticket struct {
	ID         string `json:"id" bson:"_id"`
	TenantID   string `json:"tenant_id" bson:"tenant_id"`
	DealID     string `json:"deal_id,omitempty" bson:"deal_id,omitempty"`
	PropertyID string `json:"property_id,omitempty" bson:"property_id,omitempty"`

	Type        TicketType `json:"type" bson:"type"`
	Category    string     `json:"category" bson:"category"`
	Subject     string     `json:"subject" bson:"subject"`
	Description string     `json:"description,omitempty" bson:"description,omitempty"`

	Status   TicketStatus   `json:"status" bson:"status"`
	Priority TicketPriority `json:"priority" bson:"priority"`

	RequesterAccountID string `json:"requester_account_id" bson:"requester_account_id"`
	RequesterName      string `json:"requester_name,omitempty" bson:"requester_name,omitempty"`
	RequesterEmail     string `json:"requester_email,omitempty" bson:"requester_email,omitempty"`

	AssignedToID   string `json:"assigned_to_id,omitempty" bson:"assigned_to_id,omitempty"`
	AssignedToName string `json:"assigned_to_name,omitempty" bson:"assigned_to_name,omitempty"`

	Comments []Comment `json:"comments,omitempty" bson:"comments,omitempty"`

	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" bson:"updated_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty" bson:"resolved_at,omitempty"`
	ClosedAt   *time.Time `json:"closed_at,omitempty" bson:"closed_at,omitempty"`
}

type CommentVisibility string

const (
	CommentVisibilityPublic   CommentVisibility = "public"
	CommentVisibilityInternal CommentVisibility = "internal"
)

func (v CommentVisibility) IsValid() bool {
	return v == CommentVisibilityPublic || v == CommentVisibilityInternal
}

type Comment struct {
	ID         string            `json:"id" bson:"id"`
	AuthorID   string            `json:"author_id" bson:"author_id"`
	AuthorName string            `json:"author_name,omitempty" bson:"author_name,omitempty"`
	AuthorType string            `json:"author_type" bson:"author_type"`
	Visibility CommentVisibility `json:"visibility,omitempty" bson:"visibility,omitempty"`
	Content    string            `json:"content" bson:"content"`

	Attachments []string  `json:"attachments,omitempty" bson:"attachments,omitempty"`
	CreatedAt   time.Time `json:"created_at" bson:"created_at"`

	ClientRequestID string `json:"client_request_id,omitempty" bson:"client_request_id,omitempty"`
}

func (c Comment) EffectiveVisibility() CommentVisibility {
	if c.Visibility == "" {
		return CommentVisibilityPublic
	}
	return c.Visibility
}

func (c Comment) IsPublic() bool {
	return c.EffectiveVisibility() == CommentVisibilityPublic
}

func New(tenantID, dealID, propertyID string, t TicketType, category, subject, description string, priority TicketPriority, requesterAccountID, requesterName, requesterEmail string) *Ticket {
	now := time.Now()
	return &Ticket{
		ID:                 uuid.New().String(),
		TenantID:           tenantID,
		DealID:             dealID,
		PropertyID:         propertyID,
		Type:               t,
		Category:           category,
		Subject:            subject,
		Description:        description,
		Status:             TicketStatusOpen,
		Priority:           priority,
		RequesterAccountID: requesterAccountID,
		RequesterName:      requesterName,
		RequesterEmail:     requesterEmail,
		Comments:           []Comment{},
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func NewComment(authorID, authorName, authorType, content string, attachments []string) Comment {
	return NewCommentWithVisibility(authorID, authorName, authorType, content, attachments, CommentVisibilityPublic)
}

func NewCommentWithVisibility(authorID, authorName, authorType, content string, attachments []string, visibility CommentVisibility) Comment {
	return NewCommentFull(authorID, authorName, authorType, content, attachments, visibility, "")
}

func NewCommentFull(authorID, authorName, authorType, content string, attachments []string, visibility CommentVisibility, clientRequestID string) Comment {
	if !visibility.IsValid() {
		visibility = CommentVisibilityPublic
	}
	return Comment{
		ID:              uuid.New().String(),
		AuthorID:        authorID,
		AuthorName:      authorName,
		AuthorType:      authorType,
		Visibility:      visibility,
		Content:         content,
		Attachments:     attachments,
		CreatedAt:       time.Now(),
		ClientRequestID: clientRequestID,
	}
}

func FilterPublicComments(comments []Comment) []Comment {
	out := make([]Comment, 0, len(comments))
	for _, c := range comments {
		if c.IsPublic() {
			out = append(out, c)
		}
	}
	return out
}
