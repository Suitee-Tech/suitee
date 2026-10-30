package contracts

import "github.com/google/uuid"

type BrokerParticipation struct {
	BrokerID   string  `json:"broker_id"`
	Role       string  `json:"role"`
	Percentage float64 `json:"percentage"`
}

type PartyPayload struct {
	PartyID   string  `json:"party_id,omitempty"`
	LeadID    string  `json:"lead_id,omitempty"`
	AccountID string  `json:"account_id,omitempty"`
	Email     string  `json:"email,omitempty"`
	Role      string  `json:"role,omitempty"`
	IsPrimary bool    `json:"is_primary,omitempty"`
	SharePct  float64 `json:"share_pct,omitempty"`
}

type ContractSignedEventPayload struct {
	DealID               string                `json:"deal_id"`
	ContractID           string                `json:"contract_id"`
	TenantID             string                `json:"tenant_id"`
	SignedAt             string                `json:"signed_at"`
	StartDate            string                `json:"start_date,omitempty"`
	EndDate              string                `json:"end_date,omitempty"`
	Value                float64               `json:"value"`
	CommissionPct        float64               `json:"commission_pct"`
	Type                 string                `json:"type"`
	CustomerID           string                `json:"customer_id"`
	OwnerID              string                `json:"owner_id"`
	PropertyID           string                `json:"property_id"`
	BrokerParticipations []BrokerParticipation `json:"broker_participations"`
	Parties              []PartyPayload        `json:"parties,omitempty"`
}

type ContractSignedEvent struct {
	EventIDVal string                     `json:"event_id"`
	Data       ContractSignedEventPayload `json:"payload"`
}

func (e ContractSignedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e ContractSignedEvent) Name() string {
	return "deal.contract.signed"
}

func (e ContractSignedEvent) Payload() interface{} {
	return e.Data
}

type DealPartyDueDiligenceUpdatedEventPayload struct {
	DealID    string `json:"deal_id"`
	PartyID   string `json:"party_id"`
	TenantID  string `json:"tenant_id,omitempty"`
	AccountID string `json:"account_id,omitempty"`
	OldStatus string `json:"old_status,omitempty"`
	NewStatus string `json:"new_status,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

type DealPartyDueDiligenceUpdatedEvent struct {
	EventIDVal string                                   `json:"event_id"`
	Data       DealPartyDueDiligenceUpdatedEventPayload `json:"payload"`
}

func (e DealPartyDueDiligenceUpdatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e DealPartyDueDiligenceUpdatedEvent) Name() string {
	return "deal.party.due_diligence.updated"
}

func (e DealPartyDueDiligenceUpdatedEvent) Payload() interface{} {
	return e.Data
}
