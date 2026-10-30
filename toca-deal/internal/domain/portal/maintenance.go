package portal

import "time"

type MaintenanceTicket struct {
	ID            string     `json:"id"`
	Type          string     `json:"type"`
	Category      string     `json:"category"`
	Status        string     `json:"status"`
	Priority      string     `json:"priority"`
	Description   string     `json:"description"`
	Attachments   []string   `json:"attachments,omitempty"`
	PropertyTitle string     `json:"property_title,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
}

type CreateMaintenanceTicketRequest struct {
	Type        string   `json:"type"`
	Category    string   `json:"category"`
	Priority    string   `json:"priority"`
	Description string   `json:"description"`
	Attachments []string `json:"attachments,omitempty"`
}
