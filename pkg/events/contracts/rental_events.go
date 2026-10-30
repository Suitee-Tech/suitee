package contracts

import (
	"time"

	"github.com/google/uuid"
)

// RentalCreatedEventPayload is the public contract for the rental.created
// event published by toca-financial after the consolidated Rental aggregate
// is persisted (see rental_service.go CreateFromDeal). Other modules
// subscribe to this — toca-comm fires a portal notification, analytics
// updates dashboards, etc.
type RentalCreatedEventPayload struct {
	RentalID     string `json:"rental_id"`
	TenantID     string `json:"tenant_id"`
	DealID       string `json:"deal_id"`
	ContractID   string `json:"contract_id"`
	PropertyID   string `json:"property_id"`
	TenantLeadID string `json:"tenant_lead_id"`
	OwnerLeadID  string `json:"owner_lead_id"`
	BrokerID     string `json:"broker_id,omitempty"`
	Timestamp    string `json:"timestamp"`
}

type RentalCreatedEvent struct {
	EventIDVal string                    `json:"event_id"`
	Data       RentalCreatedEventPayload `json:"payload"`
}

func (e RentalCreatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e RentalCreatedEvent) Name() string         { return "rental.created" }
func (e RentalCreatedEvent) Payload() interface{} { return e.Data }

// RentalChargeOverdueEventPayload — published by toca-financial when a
// recurring rental charge crosses the due date threshold without payment.
// Lets the comm module nudge the tenant lead before delinquency hardens.
type RentalChargeOverdueEventPayload struct {
	ChargeID        string  `json:"charge_id"`
	RentalID        string  `json:"rental_id"`
	TenantID        string  `json:"tenant_id"`
	TenantLeadID    string  `json:"tenant_lead_id"`
	OwnerLeadID     string  `json:"owner_lead_id"`
	Amount          float64 `json:"amount"`
	DueDate         string  `json:"due_date"`
	DaysOverdue     int     `json:"days_overdue"`
	PropertyID      string  `json:"property_id,omitempty"`
	PropertyTitle   string  `json:"property_title,omitempty"`
	ContractID      string  `json:"contract_id,omitempty"`
	ReferenceMonth  string  `json:"reference_month,omitempty"`
	Timestamp       string  `json:"timestamp"`
}

type RentalChargeOverdueEvent struct {
	EventIDVal string                          `json:"event_id"`
	Data       RentalChargeOverdueEventPayload `json:"payload"`
}

func (e RentalChargeOverdueEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e RentalChargeOverdueEvent) Name() string         { return "rental.charge_overdue" }
func (e RentalChargeOverdueEvent) Payload() interface{} { return e.Data }

// NowRFC3339 is a convenience for handlers that need to stamp Timestamp.
// Lives here (not in an internal package) because every event payload here
// formats time the same way.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}
