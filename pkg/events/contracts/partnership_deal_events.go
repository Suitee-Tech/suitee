package contracts

import "github.com/google/uuid"

// PartnerDealInitiatedEvent fires when a company starts a deal against a
// resource (property or lead) SHARED by a partner company through an active
// partnership that allows co-brokering (toca-deal DealService.Create, the
// cross-tenant branch). It lets toca-comm notify the resource OWNER company's
// administrators that a partner is now operating their shared content.
//
// Mirrors the shape of PartnershipInvitedEvent / PartnershipAcceptedEvent in
// partnership_events.go.
type PartnerDealInitiatedEventPayload struct {
	DealID            string `json:"deal_id"`
	ResourceType      string `json:"resource_type"`       // "property" | "lead"
	ResourceID        string `json:"resource_id"`         // shared property/lead id
	InitiatorTenantID string `json:"initiator_tenant_id"` // company that started the deal
	OwnerTenantID     string `json:"owner_tenant_id"`     // company that owns the resource
	CreatedAt         string `json:"created_at"`
}

type PartnerDealInitiatedEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       PartnerDealInitiatedEventPayload `json:"payload"`
}

func (e PartnerDealInitiatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e PartnerDealInitiatedEvent) Name() string         { return "partner.deal_initiated" }
func (e PartnerDealInitiatedEvent) Payload() interface{} { return e.Data }
