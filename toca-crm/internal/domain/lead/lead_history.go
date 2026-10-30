package lead

import "time"

type LeadAction string

const (
	LeadActionCreated          LeadAction = "CREATED"
	LeadActionAssigned         LeadAction = "ASSIGNED"
	LeadActionStatusChanged    LeadAction = "STATUS_CHANGED"
	LeadActionContactAttempt   LeadAction = "CONTACT_ATTEMPT"
	LeadActionContacted        LeadAction = "CONTACTED"
	LeadActionProposalSent     LeadAction = "PROPOSAL_SENT"
	LeadActionProposalAccepted LeadAction = "PROPOSAL_ACCEPTED"
	LeadActionProposalRejected LeadAction = "PROPOSAL_REJECTED"
	LeadActionLost             LeadAction = "LOST"
	LeadActionConverted        LeadAction = "CONVERTED"
	LeadActionNote             LeadAction = "NOTE"
	LeadActionDisqualified     LeadAction = "DISQUALIFIED"
)

type LeadHistory struct {
	ID        string     `bson:"_id" validate:"required,uuid"`
	LeadID    string     `bson:"lead_id" validate:"required,uuid"`
	Action    LeadAction `bson:"action" validate:"required"`
	Timestamp time.Time  `bson:"timestamp" validate:"required"`

	PreviousStatus *LeadStatus `bson:"previous_status,omitempty"`
	NewStatus      *LeadStatus `bson:"new_status,omitempty"`

	Details string `bson:"details,omitempty"`
	UserID  string `bson:"user_id,omitempty" validate:"omitempty,uuid"`
}
