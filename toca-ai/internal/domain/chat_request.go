package domain

type ChatRequest struct {
	ConversationID string                `json:"conversation_id,omitempty"`
	Message        string                `json:"message,omitempty"`
	ScreenContext  any                   `json:"screen_context,omitempty"`
	Confirm        *ActionConfirmRequest `json:"confirm,omitempty"`
}

type ActionConfirmRequest struct {
	ConversationID string         `json:"conversation_id,omitempty"`
	ActionID       string         `json:"action_id"`
	Inputs         map[string]any `json:"inputs,omitempty"`
}

type ToolEvent struct {
	ToolName   string `json:"tool_name"`
	ToolCallID string `json:"tool_call_id"`
}

type ToolEndEvent struct {
	ToolCallID    string `json:"tool_call_id"`
	ResultPreview string `json:"result_preview"`
	HasError      bool   `json:"has_error,omitempty"`
}

type MessageStartEvent struct {
	ConversationID string `json:"conversation_id"`
	MCPConnected   bool   `json:"mcp_connected"`
	ToolsCount     int    `json:"tools_count"`
	Build          string `json:"build,omitempty"`
}

type MessageDeltaEvent struct {
	ConversationID string `json:"conversation_id"`
	Content        string `json:"content"`
}

type ErrorEvent struct {
	Message string `json:"message"`
	Debug   string `json:"debug,omitempty"`
}
