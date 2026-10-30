package contracts

import "github.com/google/uuid"

type TicketCreatedEventPayload struct {
	TicketID          string `json:"ticket_id"`
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	CustomerAccountID string `json:"customer_account_id"`
	Type              string `json:"type"`
	Category          string `json:"category"`
	Subject           string `json:"subject"`
}

type TicketCreatedEvent struct {
	EventIDVal string                    `json:"event_id"`
	Data       TicketCreatedEventPayload `json:"payload"`
}

func (e TicketCreatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e TicketCreatedEvent) Name() string { return "ticket.created" }

func (e TicketCreatedEvent) Payload() interface{} { return e.Data }

type TicketStatusChangedEventPayload struct {
	TicketID          string `json:"ticket_id"`
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	OldStatus         string `json:"old_status"`
	NewStatus         string `json:"new_status"`
	CustomerAccountID string `json:"customer_account_id"`
	AssignedToID      string `json:"assigned_to_id"`
}

type TicketStatusChangedEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       TicketStatusChangedEventPayload `json:"payload"`
}

func (e TicketStatusChangedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e TicketStatusChangedEvent) Name() string { return "ticket.status_changed" }

func (e TicketStatusChangedEvent) Payload() interface{} { return e.Data }

type TicketCommentAddedEventPayload struct {
	TicketID          string `json:"ticket_id"`
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	CommentID         string `json:"comment_id"`
	AuthorID          string `json:"author_id"`
	AuthorType        string `json:"author_type"`
	CustomerAccountID string `json:"customer_account_id"`
	AssignedToID      string `json:"assigned_to_id"`
}

type TicketCommentAddedEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       TicketCommentAddedEventPayload `json:"payload"`
}

func (e TicketCommentAddedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e TicketCommentAddedEvent) Name() string { return "ticket.comment_added" }

func (e TicketCommentAddedEvent) Payload() interface{} { return e.Data }

// TicketAssignedEventPayload é emitido quando um ticket é (re)atribuído
// a um membro de operações. Trigger para toca-comm criar in-app
// notification + email + push WS no canal pessoal do assignee
// (broadcast em user:{assignee_account_id}).
//
// Reassignment é distinguido via OldAssignedToID != "" — frontend pode
// usar para mostrar "Reatribuído de X para você" em vez de "Atribuído a você".
type TicketAssignedEventPayload struct {
	TicketID           string `json:"ticket_id"`
	DealID             string `json:"deal_id"`
	TenantID           string `json:"tenant_id"`
	OldAssignedToID    string `json:"old_assigned_to_id,omitempty"`
	AssignedToID       string `json:"assigned_to_id"`
	AssignedToName     string `json:"assigned_to_name,omitempty"`
	AssignedByID       string `json:"assigned_by_id,omitempty"`
	CustomerAccountID  string `json:"customer_account_id,omitempty"`
	Subject            string `json:"subject,omitempty"`
	Priority           string `json:"priority,omitempty"`
}

type TicketAssignedEvent struct {
	EventIDVal string                     `json:"event_id"`
	Data       TicketAssignedEventPayload `json:"payload"`
}

func (e TicketAssignedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e TicketAssignedEvent) Name() string { return "ticket.assigned" }

func (e TicketAssignedEvent) Payload() interface{} { return e.Data }
