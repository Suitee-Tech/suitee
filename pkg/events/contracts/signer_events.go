package contracts

import "github.com/google/uuid"

// SignerInvitedEventPayload is published whenever an admin invites a signer to
// a deal — meaning the signer now has a portal account and can sign.
type SignerInvitedEventPayload struct {
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	SignerID          string `json:"signer_id"`
	SignerName        string `json:"signer_name"`
	SignerEmail       string `json:"signer_email"`
	CustomerAccountID string `json:"customer_account_id"`
}

type SignerInvitedEvent struct {
	EventIDVal string                    `json:"event_id"`
	Data       SignerInvitedEventPayload `json:"payload"`
}

func (e SignerInvitedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e SignerInvitedEvent) Name() string { return "signer.invited" }

func (e SignerInvitedEvent) Payload() interface{} { return e.Data }

// ChecklistItemSolicitedEventPayload is published when an operator requests a
// document from a specific party in a deal.
type ChecklistItemSolicitedEventPayload struct {
	DealID            string `json:"deal_id"`
	TenantID          string `json:"tenant_id"`
	ItemID            string `json:"item_id"`
	ItemName          string `json:"item_name"`
	PartyID           string `json:"party_id,omitempty"`
	CustomerAccountID string `json:"customer_account_id"`
}

type ChecklistItemSolicitedEvent struct {
	EventIDVal string                             `json:"event_id"`
	Data       ChecklistItemSolicitedEventPayload `json:"payload"`
}

func (e ChecklistItemSolicitedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ChecklistItemSolicitedEvent) Name() string { return "checklist.solicited" }

func (e ChecklistItemSolicitedEvent) Payload() interface{} { return e.Data }
