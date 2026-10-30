package contracts

import "github.com/google/uuid"

// PartnershipInvitedEvent fires when a company invites another company into a
// partnership or franchise (toca-core PartnershipService.CreatePartnership).
// The partnership row is created in `pending` status — this event lets
// toca-comm notify the target company's administrator so they can accept or
// reject the invite.
type PartnershipInvitedEventPayload struct {
	PartnershipID string `json:"partnership_id"`
	RequesterID   string `json:"requester_id"` // inviting company (franchisor/initiator)
	TargetID      string `json:"target_id"`    // invited company (franchisee/partner)
	Type          string `json:"type"`         // "partner" | "franchise"
	CreatedAt     string `json:"created_at"`
}

type PartnershipInvitedEvent struct {
	EventIDVal string                         `json:"event_id"`
	Data       PartnershipInvitedEventPayload `json:"payload"`
}

func (e PartnershipInvitedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e PartnershipInvitedEvent) Name() string         { return "partnership.invited" }
func (e PartnershipInvitedEvent) Payload() interface{} { return e.Data }

// PartnershipAcceptedEvent fires when the target company accepts the invite
// (status pending -> active). toca-comm notifies the requester company that
// the partnership/franchise is now active.
type PartnershipAcceptedEventPayload struct {
	PartnershipID string `json:"partnership_id"`
	RequesterID   string `json:"requester_id"`
	TargetID      string `json:"target_id"`
	Type          string `json:"type"`
	AcceptedAt    string `json:"accepted_at"`
}

type PartnershipAcceptedEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       PartnershipAcceptedEventPayload `json:"payload"`
}

func (e PartnershipAcceptedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e PartnershipAcceptedEvent) Name() string         { return "partnership.accepted" }
func (e PartnershipAcceptedEvent) Payload() interface{} { return e.Data }

// PartnershipChangedEvent invalidates agreement and shared-content views for both companies.
type PartnershipChangedEvent struct {
	ID   string
	Data map[string]interface{}
}

func (e PartnershipChangedEvent) EventID() string      { return e.ID }
func (e PartnershipChangedEvent) Name() string         { return "partnership.changed" }
func (e PartnershipChangedEvent) Payload() interface{} { return e.Data }
