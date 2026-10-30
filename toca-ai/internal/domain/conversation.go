package domain

import "time"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Conversation struct {
	ID        string         `json:"id"`
	TenantID  string         `json:"tenant_id"`
	AccountID string         `json:"account_id"`
	Title     string         `json:"title"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt *time.Time     `json:"deleted_at,omitempty"`
}

type Message struct {
	ID             string         `json:"id"`
	TenantID       string         `json:"tenant_id"`
	ConversationID string         `json:"conversation_id"`
	Role           Role           `json:"role"`
	Content        string         `json:"content"`
	ToolCalls      map[string]any `json:"tool_calls,omitempty"`
	ToolCallID     string         `json:"tool_call_id,omitempty"`
	ToolName       string         `json:"tool_name,omitempty"`
	TokensIn       int            `json:"tokens_in,omitempty"`
	TokensOut      int            `json:"tokens_out,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}
