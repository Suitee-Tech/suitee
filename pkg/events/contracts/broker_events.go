package contracts

import "github.com/google/uuid"

// BrokerUpdatedEvent fires whenever a Broker's user-visible profile data
// changes (name, role, email, avatar). Downstream services that snapshot
// broker labels — most notably Deal.BrokerParticipations in toca-deal —
// can subscribe to refresh their projections instead of stamping the
// label at the moment the participation was recorded.
//
// Operational broker state (paused/active/weights/business_hours) is part
// of the same payload for completeness; subscribers that only care about
// label refreshes can ignore the extra fields.
type BrokerUpdatedEventPayload struct {
	BrokerID  string `json:"broker_id"`
	TenantID  string `json:"tenant_id"`
	Name      string `json:"name,omitempty"`
	Role      string `json:"role,omitempty"`
	Email     string `json:"email,omitempty"`
	Avatar    string `json:"avatar,omitempty"`
	Active    bool   `json:"active"`
	Paused    bool   `json:"paused"`
	UpdatedAt string `json:"updated_at"`
}

type BrokerUpdatedEvent struct {
	EventIDVal string                    `json:"event_id"`
	Data       BrokerUpdatedEventPayload `json:"payload"`
}

func (e BrokerUpdatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e BrokerUpdatedEvent) Name() string         { return "broker.updated" }
func (e BrokerUpdatedEvent) Payload() interface{} { return e.Data }
