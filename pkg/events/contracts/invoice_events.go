package contracts

import (
	"time"

	"github.com/google/uuid"
)

type InvoicePaidEventPayload struct {
	// SchemaVersion 2 identifies the semantic invoice aggregate contract.
	// Version 0 is reserved for legacy messages emitted from ChargeService.
	SchemaVersion int    `json:"schema_version,omitempty"`
	InvoiceID     string `json:"invoice_id"`
	// ChargeID is populated by current publishers. It remains omitempty only
	// so historical invoice.paid messages can still be decoded during rollout.
	ChargeID    string  `json:"charge_id,omitempty"`
	TenantID    string  `json:"tenant_id"`
	ReferenceID string  `json:"reference_id"`
	Type        string  `json:"type"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	PaidAt      string  `json:"paid_at"`
}

type InvoicePaidEvent struct {
	EventIDVal string                  `json:"event_id"`
	Data       InvoicePaidEventPayload `json:"payload"`
}

func (e InvoicePaidEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e InvoicePaidEvent) Name() string {
	return "invoice.paid"
}

func (e InvoicePaidEvent) Payload() interface{} {
	if e.Data.PaidAt == "" {
		e.Data.PaidAt = time.Now().UTC().Format(time.RFC3339)
	}
	return e.Data
}
