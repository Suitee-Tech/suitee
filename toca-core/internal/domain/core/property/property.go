package property

import (
	"github.com/oginoino/toca-core/internal/domain/core/address"
	"github.com/oginoino/toca/pkg/validation"
)

var validate = validation.New()

type Property struct {
	ID                string            `json:"id" validate:"required"`
	DisplayID         string            `json:"display_id,omitempty" bson:"display_id,omitempty"`
	ExternalID        string            `json:"external_id,omitempty"`
	Title             string            `json:"title" validate:"required"`
	Description       string            `json:"description,omitempty"`
	Status            PropertyStatus    `json:"status" validate:"required,oneof=prospecting in_portfolio published attending negotiating proposal analysis contract rented sold administered rescission available inactive"`
	PublicationStatus PublicationStatus `json:"publication_status" validate:"required,oneof=draft published"`

	PropertyType PropertyType      `json:"property_type" validate:"required,oneof=apartment house land commercial"`
	ListingType  ListingType       `json:"listing_type" validate:"required,oneof=sale rent"`
	Condition    PropertyCondition `json:"condition" validate:"required,oneof=new used under_construction"`

	Address address.Address `json:"address"`

	Pricing Pricing `json:"pricing"`

	Areas Areas `json:"areas"`

	Features Features `json:"features"`

	Amenities []string `json:"amenities,omitempty"`

	Media Media `json:"media,omitempty"`

	Legal Legal `json:"legal,omitempty"`

	// Legacy single owner/broker (kept for backward compatibility)
	Owner   OwnerRef   `json:"owner"`
	Broker  BrokerRef  `json:"broker,omitempty"`
	Company CompanyRef `json:"company,omitempty"`

	// Multi-owner support: multiple owners with ownership percentage
	Owners []OwnerRef `json:"owners,omitempty" bson:"owners,omitempty"`

	// Capturing brokers: multiple brokers with roles (captador, co_captador, etc.)
	CapturingBrokers []CapturingBrokerRef `json:"capturing_brokers,omitempty" bson:"capturing_brokers,omitempty"`

	// Available business types: a property can be available for sale, rent and/or exchange simultaneously
	AvailableFor []ListingType `json:"available_for,omitempty" bson:"available_for,omitempty"`

	// IntermediationContractID references the intermediation/administration/brokerage contract
	IntermediationContractID string `json:"intermediation_contract_id,omitempty" bson:"intermediation_contract_id,omitempty"`

	// InterestedLeads lists leads that expressed interest in this property
	InterestedLeads []InterestedLeadRef `json:"interested_leads,omitempty" bson:"interested_leads,omitempty"`

	// ActiveDealID references the current active deal (negotiation) for this property
	ActiveDealID string `json:"active_deal_id,omitempty" bson:"active_deal_id,omitempty"`

	// PartnershipPortfolio indicates this property is shared in the partnership portfolio
	PartnershipPortfolio bool `json:"partnership_portfolio,omitempty" bson:"partnership_portfolio,omitempty"`

	// CaptureExclusivity is the brokerage exclusivity of the capture. Empty
	// is treated as "open". An "exclusive" capture can never be shared in
	// the partnership portfolio (see CanSharePartnershipPortfolio).
	CaptureExclusivity CaptureExclusivity `json:"capture_exclusivity,omitempty" bson:"capture_exclusivity,omitempty" validate:"omitempty,oneof=open exclusive"`

	Tags []string `json:"tags,omitempty"`

	// TenantID is the identifier for multi-tenancy context (AccountID)
	TenantID string `json:"tenant_id" bson:"tenant_id" validate:"required"`
}

type PropertyFilter struct {
	TenantID             string
	Query                string
	Address              string
	Neighborhood         string
	City                 string
	State                string
	ZipCode              string
	Status               PropertyStatus
	PropertyType         PropertyType
	ListingType          ListingType
	AvailableFor         ListingType // filter by available business type
	OwnerID              string      // filter by owner lead/client ID
	CapturingBrokerID    string      // filter by capturing broker ID
	InterestedLeadID     string      // filter by interested lead ID
	MinPrice             *float64
	MaxPrice             *float64
	MinBedrooms          *int
	MinBathrooms         *int
	MinArea              *float64
	MaxArea              *float64
	PartnershipPortfolio *bool
	Limit                int64
	Offset               int64
}

func (p *Property) Validate() error {
	return validate.Struct(p)
}

// EffectiveCaptureExclusivity returns the capture exclusivity, defaulting an
// empty value to "open".
func (p *Property) EffectiveCaptureExclusivity() CaptureExclusivity {
	if p.CaptureExclusivity == "" {
		return CaptureExclusivityOpen
	}
	return p.CaptureExclusivity
}

// CanSharePartnershipPortfolio reports whether the property is eligible to be
// shared in the partnership portfolio. A property with an EXCLUSIVE capture is
// never eligible.
func (p *Property) CanSharePartnershipPortfolio() bool {
	return p.EffectiveCaptureExclusivity() != CaptureExclusivityExclusive
}
