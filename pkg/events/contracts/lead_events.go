package contracts

import "github.com/google/uuid"

type LeadConvertedEventPayload struct {
	LeadID   string `json:"lead_id"`
	TenantID string `json:"tenant_id"`
	Role     string `json:"role"`
}

type LeadConvertedEvent struct {
	EventIDVal string                    `json:"event_id"`
	Data       LeadConvertedEventPayload `json:"payload"`
}

func (e LeadConvertedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e LeadConvertedEvent) Name() string {
	return "lead.converted"
}

func (e LeadConvertedEvent) Payload() interface{} {
	return e.Data
}

// LeadUpdatedEvent fires whenever a Lead's mutable user-visible data
// changes (name, email, phone, document, etc.). Downstream services that
// cache or snapshot lead labels (toca-deal contract titles, toca-financial
// fiscal records, toca-portal client portal) can subscribe to refresh
// their projections so reports/UI don't drift from the canonical lead
// record in toca-crm.
//
// Status / assignment changes have their own dedicated events
// (lead.status_changed, lead.assigned) — this one is the catch-all for
// the rest of the payload.
type LeadUpdatedEventPayload struct {
	LeadID    string `json:"lead_id"`
	TenantID  string `json:"tenant_id"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
	Document  string `json:"document,omitempty"`
	Role      string `json:"role,omitempty"`
	UpdatedAt string `json:"updated_at"`
}

type LeadUpdatedEvent struct {
	EventIDVal string                  `json:"event_id"`
	Data       LeadUpdatedEventPayload `json:"payload"`
}

func (e LeadUpdatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e LeadUpdatedEvent) Name() string {
	return "lead.updated"
}

func (e LeadUpdatedEvent) Payload() interface{} {
	return e.Data
}
