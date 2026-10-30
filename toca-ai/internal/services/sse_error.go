package services

type SSEError struct {
	Message        string
	Debug          string
	ConversationID string
}

func (e *SSEError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return "chat error"
}
