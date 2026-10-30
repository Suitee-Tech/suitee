package message

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrChannelNotFound é o sentinel de "canal não existe / não é visível para
// quem chamou".
//
// SECURITY (achado A9 da revisão de permissões de 2026-09-19): o serviço fazia
// fallback para uma busca de canal **sem tenant** em QUALQUER erro de
// `GetChannel` — inclusive falha de banco — e a partir daí a única checagem era
// a lista de participantes. Sem separar "não encontrado" de "erro de
// infraestrutura" não há como decidir se o fallback é legítimo.
var ErrChannelNotFound = errors.New("channel not found")

// ChannelType represents the type of a message channel
type ChannelType string

const (
	ChannelTypeDirect  ChannelType = "direct"  // 1-to-1 between users
	ChannelTypeGroup   ChannelType = "group"   // Group conversation
	ChannelTypeSupport ChannelType = "support" // Client support thread
	ChannelTypeLead    ChannelType = "lead"    // Linked to a lead
	ChannelTypeDeal    ChannelType = "deal"    // Linked to a deal
)

// ChannelStatus represents the status of a message channel
type ChannelStatus string

const (
	ChannelStatusOpen     ChannelStatus = "open"
	ChannelStatusClosed   ChannelStatus = "closed"
	ChannelStatusArchived ChannelStatus = "archived"
)

// ChannelInfoSection groups related links and reference data displayed in the
// channel details panel (property, parties, owner, documents, etc.).
type ChannelInfoSection struct {
	Label string            `json:"label"`
	Items []ChannelInfoItem `json:"items"`
}

// ChannelInfoItem is a single row inside a ChannelInfoSection.
type ChannelInfoItem struct {
	Label string `json:"label"`
	Value string `json:"value,omitempty"`
	Link  string `json:"link,omitempty"`
}

// ChannelInfo holds structured reference data pinned to a channel (property
// links, party contacts, owner docs, etc.). Stored inside the channel JSONB
// metadata so it never needs a separate table or migration.
type ChannelInfo struct {
	Sections []ChannelInfoSection `json:"sections,omitempty"`
}

type ParticipantProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Channel represents a messaging channel (group/room)
type Channel struct {
	ParticipantProfiles []ParticipantProfile `json:"participant_profiles,omitempty"`
	ID                  string               `json:"id" bson:"id"`
	TenantID            string               `json:"tenant_id" bson:"tenantid"`
	Name                string               `json:"name" bson:"name"`
	Type                ChannelType          `json:"type" bson:"type"`
	Status              ChannelStatus        `json:"status" bson:"status"`
	Participants        []string             `json:"participants" bson:"participants"`                  // Account IDs
	EntityID            string               `json:"entity_id,omitempty" bson:"entityid,omitempty"`     // Optional: linked lead/deal ID
	EntityType          string               `json:"entity_type,omitempty" bson:"entitytype,omitempty"` // "lead", "deal", "contract"
	CreatedBy           string               `json:"created_by" bson:"createdby"`
	PinnedBy            []string             `json:"pinned_by,omitempty" bson:"pinnedby,omitempty"` // User IDs who pinned this channel
	MutedBy             []string             `json:"muted_by,omitempty" bson:"mutedby,omitempty"`   // User IDs who muted this channel
	Info                *ChannelInfo         `json:"info,omitempty" bson:"info,omitempty"`          // Structured info panel data
	CreatedAt           time.Time            `json:"created_at" bson:"createdat"`
	UpdatedAt           time.Time            `json:"updated_at" bson:"updatedat"`
	LastMessage         *Message             `json:"last_message,omitempty" bson:"lastmessage,omitempty"`
}

// NewChannel creates a new Channel with generated ID and timestamps
func NewChannel(tenantID, name string, channelType ChannelType, createdBy string, participants []string) *Channel {
	now := time.Now()
	return &Channel{
		ID:           uuid.New().String(),
		TenantID:     tenantID,
		Name:         name,
		Type:         channelType,
		Status:       ChannelStatusOpen,
		Participants: participants,
		CreatedBy:    createdBy,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
}

// Reaction represents a user's reaction to a message
type Reaction struct {
	Emoji   string   `json:"emoji" bson:"emoji"`
	UserIDs []string `json:"user_ids" bson:"userids"`
}

// Attachment represents a file or media attached to a message
type Attachment struct {
	ID   string `json:"id" bson:"id"`
	Name string `json:"name" bson:"name"`
	URL  string `json:"url" bson:"url"`
	Type string `json:"type" bson:"type"` // mime type
	Size int64  `json:"size" bson:"size"`
}

// Message represents a message sent in a channel
type Message struct {
	ID              string       `json:"id" bson:"id"`
	ChannelID       string       `json:"channel_id" bson:"channelid"`
	TenantID        string       `json:"tenant_id" bson:"tenantid"`
	SenderID        string       `json:"sender_id" bson:"senderid"`
	SenderName      string       `json:"sender_name" bson:"sendername"`
	SenderAvatarURL string       `json:"sender_avatar_url,omitempty" bson:"sender_avatar_url,omitempty"`
	Content         string       `json:"content" bson:"content"`
	ContentType     string       `json:"content_type" bson:"contenttype"` // "text", "image", "file"
	Attachments     []Attachment `json:"attachments,omitempty" bson:"attachments,omitempty"`
	Reactions       []Reaction   `json:"reactions,omitempty" bson:"reactions,omitempty"`
	Mentions        []string     `json:"mentions,omitempty" bson:"mentions,omitempty"` // Account IDs
	Hashtags        []string     `json:"hashtags,omitempty" bson:"hashtags,omitempty"` // Strings
	ReplyToID       string       `json:"reply_to_id,omitempty" bson:"replytoid,omitempty"`
	IsPinned        bool         `json:"is_pinned,omitempty" bson:"ispinned,omitempty"`
	IsEdited        bool         `json:"is_edited,omitempty" bson:"isedited,omitempty"`
	EditedAt        *time.Time   `json:"edited_at,omitempty" bson:"editedat,omitempty"`
	SentAt          time.Time    `json:"sent_at" bson:"sentat"`
	ReadBy          []string     `json:"read_by" bson:"readby"`                           // Account IDs who read the message
	IsFlagged       bool         `json:"is_flagged,omitempty" bson:"isflagged,omitempty"` // For moderation
}

// NewMessage creates a new Message with generated ID and timestamp
func NewMessage(channelID, tenantID, senderID, senderName, content string) *Message {
	return &Message{
		ID:          uuid.New().String(),
		ChannelID:   channelID,
		TenantID:    tenantID,
		SenderID:    senderID,
		SenderName:  senderName,
		Content:     content,
		ContentType: "text",
		SentAt:      time.Now(),
		ReadBy:      []string{senderID},
	}
}

// Template represents a pre-approved message template
type Template struct {
	ID        string    `json:"id" bson:"id"`
	TenantID  string    `json:"tenant_id" bson:"tenantid"`
	Name      string    `json:"name" bson:"name"`
	Category  string    `json:"category" bson:"category"` // e.g., "visit", "proposal", "contract"
	Content   string    `json:"content" bson:"content"`
	Variables []string  `json:"variables" bson:"variables"` // e.g., ["client_name", "property_address", "date"]
	CreatedBy string    `json:"created_by" bson:"createdby"`
	CreatedAt time.Time `json:"created_at" bson:"createdat"`
}

// NewTemplate creates a new Template
func NewTemplate(tenantID, name, category, content string, variables []string, createdBy string) *Template {
	return &Template{
		ID:        uuid.New().String(),
		TenantID:  tenantID,
		Name:      name,
		Category:  category,
		Content:   content,
		Variables: variables,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}
}

// Metrics represents chat system metrics for the dashboard
type Metrics struct {
	TotalMessages      int64   `json:"total_messages"`
	TotalChannels      int64   `json:"total_channels"`
	ActiveUsers        int64   `json:"active_users"`
	FlaggedMessages    int64   `json:"flagged_messages"`
	AvgResponseTimeMin float64 `json:"avg_response_time_minutes"`
}
