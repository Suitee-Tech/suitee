package contracts

import "github.com/google/uuid"

// PropertyCapturedEventPayload — fires from toca-core when a Property is
// created with status=`prospecting` and at least one capturing broker.
// Marks the start of the captação lifecycle. Downstream:
// - toca-comm: notify the owner-lead that the captação was registered.
// - analytics: count captações per broker.
// - audit log: who captured what and when.
type PropertyCapturedEventPayload struct {
	PropertyID    string  `json:"property_id"`
	TenantID      string  `json:"tenant_id"`
	OwnerLeadID   string  `json:"owner_lead_id,omitempty"`
	CapturingBrokerIDs []string `json:"capturing_broker_ids"`
	PropertyTitle string  `json:"property_title,omitempty"`
	Status        string  `json:"status"` // typically "prospecting"
	ListingType   string  `json:"listing_type,omitempty"` // "sale" / "rent"
	ExpectedValue float64 `json:"expected_value,omitempty"`
	Timestamp     string  `json:"timestamp"`
}

type PropertyCapturedEvent struct {
	EventIDVal string                       `json:"event_id"`
	Data       PropertyCapturedEventPayload `json:"payload"`
}

func (e PropertyCapturedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e PropertyCapturedEvent) Name() string         { return "property.captured" }
func (e PropertyCapturedEvent) Payload() interface{} { return e.Data }

// CapturingBrokerAddedEventPayload — fires when an additional broker joins
// an already-captured property (co_captador, visitador). Used by analytics
// + commission-rule engine to recompute splits.
type CapturingBrokerAddedEventPayload struct {
	PropertyID string `json:"property_id"`
	TenantID   string `json:"tenant_id"`
	BrokerID   string `json:"broker_id"`
	Role       string `json:"role"` // captador/co_captador/visitador/fechamento
	Creci      string `json:"creci,omitempty"`
	Timestamp  string `json:"timestamp"`
}

type CapturingBrokerAddedEvent struct {
	EventIDVal string                           `json:"event_id"`
	Data       CapturingBrokerAddedEventPayload `json:"payload"`
}

func (e CapturingBrokerAddedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e CapturingBrokerAddedEvent) Name() string         { return "property.capturing_broker_added" }
func (e CapturingBrokerAddedEvent) Payload() interface{} { return e.Data }

// PropertyUpdatedEvent fires whenever a Property's user-visible data
// changes (title, address, listing type, pricing, status). Downstream
// services that cache or snapshot property labels (toca-deal contract
// titles, toca-portal listings, toca-financial fiscal records) can
// subscribe to refresh their projections.
//
// Status transitions also have their dedicated property.captured /
// property.capturing_broker_added events for the captação flow — this
// catch-all covers everything else.
type PropertyUpdatedEventPayload struct {
	PropertyID    string  `json:"property_id"`
	TenantID      string  `json:"tenant_id"`
	PropertyTitle string  `json:"property_title,omitempty"`
	Status        string  `json:"status,omitempty"`
	ListingType   string  `json:"listing_type,omitempty"`
	Address       string  `json:"address,omitempty"`
	ExpectedValue float64 `json:"expected_value,omitempty"`
	UpdatedAt     string  `json:"updated_at"`
}

type PropertyUpdatedEvent struct {
	EventIDVal string                      `json:"event_id"`
	Data       PropertyUpdatedEventPayload `json:"payload"`
}

func (e PropertyUpdatedEvent) EventID() string {
	if e.EventIDVal == "" {
		return uuid.New().String()
	}
	return e.EventIDVal
}

func (e PropertyUpdatedEvent) Name() string         { return "property.updated" }
func (e PropertyUpdatedEvent) Payload() interface{} { return e.Data }
