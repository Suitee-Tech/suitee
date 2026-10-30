package portal

type CreateTicketRequest struct {
	Type        string `json:"type"`
	Category    string `json:"category"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
}

type AddTicketCommentRequest struct {
	Content     string   `json:"content"`
	Attachments []string `json:"attachments,omitempty"`
	// ClientRequestID: idempotency token opcional, gerado pelo
	// frontend (UUID v4 por intenção do clique "Enviar"). Reenvios
	// com o mesmo valor não duplicam o comment no ticket. Issue #453.
	ClientRequestID string `json:"client_request_id,omitempty"`
}
