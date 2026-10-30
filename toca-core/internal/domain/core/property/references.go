package property

import "time"

// OwnerRef references a property owner (lead/client).
// Supports multiple owners with ownership percentage.
type OwnerRef struct {
	ID         string    `json:"id" bson:"id"`
	Type       string    `json:"type" bson:"type"`                                 // "person", "company"
	Percentage float64   `json:"percentage,omitempty" bson:"percentage,omitempty"` // ownership share (0–100)
	Since      time.Time `json:"since,omitempty" bson:"since,omitempty"`
}

// BrokerRef is kept for backward compatibility (single broker reference).
type BrokerRef struct {
	ID    string `json:"id" bson:"id"`
	Creci string `json:"creci,omitempty" bson:"creci,omitempty"`
}

// CapturingBrokerRef references a broker that captured/registered the property.
type CapturingBrokerRef struct {
	ID            string     `json:"id" bson:"id"`
	Creci         string     `json:"creci,omitempty" bson:"creci,omitempty"`
	Role          BrokerRole `json:"role" bson:"role"` // captador, co_captador, etc.
	CommissionPct float64    `json:"commission_pct,omitempty" bson:"commission_pct,omitempty"`
	CapturedAt    time.Time  `json:"captured_at" bson:"captured_at"`
}

// InterestedLeadRef links a lead to a property as a prospective buyer/tenant.
type InterestedLeadRef struct {
	LeadID       string    `json:"lead_id" bson:"lead_id"`
	InterestedAt time.Time `json:"interested_at" bson:"interested_at"`
	Source       string    `json:"source,omitempty" bson:"source,omitempty"` // "manual", "website", etc.
}

type CompanyRef struct {
	ID             string `json:"id" bson:"id"`
	BusinessUnitID string `json:"business_unit_id,omitempty" bson:"business_unit_id,omitempty"`
	DivisionID     string `json:"division_id,omitempty" bson:"division_id,omitempty"`
}

func (c *CompanyRef) Validate() error {
	return validate.Struct(c)
}
