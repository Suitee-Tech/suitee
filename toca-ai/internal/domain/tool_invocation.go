package domain

import "time"

type ToolInvocation struct {
	ID             string         `json:"id"`
	ConversationID string         `json:"conversation_id"`
	TenantID       string         `json:"tenant_id"`
	AccountID      string         `json:"account_id"`
	ToolName       string         `json:"tool_name"`
	Args           map[string]any `json:"args,omitempty"`
	ResultPreview  string         `json:"result_preview,omitempty"`
	Success        bool           `json:"success"`
	HasError       bool           `json:"has_error"`
	DurationMS     int            `json:"duration_ms,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
}
