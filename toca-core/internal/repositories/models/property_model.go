package models

import (
	"time"

	"github.com/oginoino/toca-core/internal/domain/core/property"
)

// PropertyModel is a DTO for MongoDB persistence
type PropertyModel struct {
	ID                string                     `bson:"_id,omitempty" json:"id"`
	DisplayID         string                     `bson:"display_id,omitempty" json:"display_id,omitempty"`
	ExternalID        string                     `bson:"external_id,omitempty" json:"external_id,omitempty"`
	Title             string                     `bson:"title" json:"title"`
	Description       string                     `bson:"description,omitempty" json:"description,omitempty"`
	Status            property.PropertyStatus    `bson:"status" json:"status"`
	PublicationStatus property.PublicationStatus `bson:"publication_status" json:"publication_status"`
	PropertyType      property.PropertyType      `bson:"property_type" json:"property_type"`
	ListingType       property.ListingType       `bson:"listing_type" json:"listing_type"`
	Condition         property.PropertyCondition `bson:"condition" json:"condition"`
	Address           AddressModel               `bson:"address" json:"address"`
	Pricing           PricingModel               `bson:"pricing" json:"pricing"`
	Areas             AreasModel                 `bson:"areas" json:"areas"`
	Features          FeaturesModel              `bson:"features" json:"features"`
	Amenities         []string                   `bson:"amenities,omitempty" json:"amenities,omitempty"`
	Media             MediaModel                 `bson:"media,omitempty" json:"media,omitempty"`
	Legal             LegalModel                 `bson:"legal,omitempty" json:"legal,omitempty"`
	// Legacy single owner/broker fields (kept for backward compatibility)
	Owner   OwnerModel   `bson:"owner" json:"owner"`
	Broker  BrokerModel  `bson:"broker,omitempty" json:"broker,omitempty"`
	Company CompanyModel `bson:"company,omitempty" json:"company,omitempty"`
	// Multi-owner/broker support
	Owners           []OwnerModel           `bson:"owners,omitempty" json:"owners,omitempty"`
	CapturingBrokers []CapturingBrokerModel `bson:"capturing_brokers,omitempty" json:"capturing_brokers,omitempty"`
	// Available business types (sale, rent, exchange)
	AvailableFor []property.ListingType `bson:"available_for,omitempty" json:"available_for,omitempty"`
	// Contract and deal references
	IntermediationContractID string `bson:"intermediation_contract_id,omitempty" json:"intermediation_contract_id,omitempty"`
	ActiveDealID             string `bson:"active_deal_id,omitempty" json:"active_deal_id,omitempty"`
	// Interested leads
	InterestedLeads []InterestedLeadModel `bson:"interested_leads,omitempty" json:"interested_leads,omitempty"`
	// Partnership portfolio
	PartnershipPortfolio bool      `bson:"partnership_portfolio,omitempty" json:"partnership_portfolio,omitempty"`
	Tags                 []string  `bson:"tags,omitempty" json:"tags,omitempty"`
	TenantID             string    `bson:"tenant_id" json:"tenant_id"`
	CreatedAt            time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt            time.Time `bson:"updated_at" json:"updated_at"`
}

type AddressModel struct {
	Street           string `bson:"street" json:"street"`
	Number           string `bson:"number" json:"number"`
	Complement       string `bson:"complement,omitempty" json:"complement,omitempty"`
	Neighborhood     string `bson:"neighborhood" json:"neighborhood"`
	District         string `bson:"district,omitempty" json:"district,omitempty"`
	City             string `bson:"city" json:"city"`
	State            string `bson:"state" json:"state"`
	StateCode        string `bson:"state_code" json:"stateCode"`
	PostalCode       string `bson:"postal_code" json:"postalCode"`
	Country          string `bson:"country" json:"country"`
	CountryCode      string `bson:"country_code" json:"countryCode"`
	IsVerified       bool   `bson:"is_verified" json:"is_verified"`
	Longitude        string `bson:"longitude,omitempty" json:"longitude,omitempty"`
	Latitude         string `bson:"latitude,omitempty" json:"latitude,omitempty"`
	FormattedAddress string `bson:"formatted_address,omitempty" json:"formatted_address,omitempty"`
	ReferencePoint   string `bson:"reference_point,omitempty" json:"reference_point,omitempty"`
}

type PricingModel struct {
	SalePrice      *MoneyModel `bson:"sale_price,omitempty" json:"sale_price,omitempty"`
	RentPrice      *MoneyModel `bson:"rent_price,omitempty" json:"rent_price,omitempty"`
	CondominiumFee *MoneyModel `bson:"condominium_fee,omitempty" json:"condominium_fee,omitempty"`
	PropertyTax    *MoneyModel `bson:"property_tax,omitempty" json:"property_tax,omitempty"`
}

type MoneyModel struct {
	OriginalAmount float64 `bson:"original_amount" json:"original_amount"`
	CurrentAmount  float64 `bson:"current_amount" json:"current_amount"`
	Currency       string  `bson:"currency" json:"currency"`
}

type AreasModel struct {
	TotalAreaM2  float64  `bson:"total_area_m2,omitempty" json:"total_area_m2,omitempty"`
	UsableAreaM2 *float64 `bson:"usable_area_m2,omitempty" json:"usable_area_m2,omitempty"`
	LandAreaM2   *float64 `bson:"land_area_m2,omitempty" json:"land_area_m2,omitempty"`
	BuiltAreaM2  *float64 `bson:"built_area_m2,omitempty" json:"built_area_m2,omitempty"`
}

type FeaturesModel struct {
	Bedrooms      *int  `bson:"bedrooms,omitempty" json:"bedrooms,omitempty"`
	Bathrooms     *int  `bson:"bathrooms,omitempty" json:"bathrooms,omitempty"`
	Suites        *int  `bson:"suites,omitempty" json:"suites,omitempty"`
	ParkingSpaces *int  `bson:"parking_spaces,omitempty" json:"parking_spaces,omitempty"`
	Floors        *int  `bson:"floors,omitempty" json:"floors,omitempty"`
	FloorNumber   *int  `bson:"floor_number,omitempty" json:"floor_number,omitempty"`
	HasElevator   *bool `bson:"has_elevator,omitempty" json:"has_elevator,omitempty"`
	Furnished     *bool `bson:"furnished,omitempty" json:"furnished,omitempty"`
	PetsAllowed   *bool `bson:"pets_allowed,omitempty" json:"pets_allowed,omitempty"`
}

type MediaModel struct {
	Images []ImageModel `bson:"images,omitempty" json:"images,omitempty"`
	Videos []VideoModel `bson:"videos,omitempty" json:"videos,omitempty"`
	Tours  []TourModel  `bson:"tours,omitempty" json:"tours,omitempty"`
}

type ImageModel struct {
	URL         string `bson:"url" json:"url"`
	Description string `bson:"description,omitempty" json:"description,omitempty"`
	Order       int    `bson:"order,omitempty" json:"order,omitempty"`
}

type VideoModel struct {
	URL      string `bson:"url" json:"url"`
	Platform string `bson:"platform,omitempty" json:"platform,omitempty"`
}

type TourModel struct {
	URL string `bson:"url" json:"url"`
}

type LegalModel struct {
	RegistrationNumber string `bson:"registration_number,omitempty" json:"registrationNumber,omitempty"`
	NotaryOffice       string `bson:"notary_office,omitempty" json:"notaryOffice,omitempty"`
	DeedStatus         string `bson:"deed_status,omitempty" json:"deedStatus,omitempty"`
}

type OwnerModel struct {
	ID         string    `bson:"id" json:"id"`
	Type       string    `bson:"type" json:"type"`
	Percentage float64   `bson:"percentage,omitempty" json:"percentage,omitempty"`
	Since      time.Time `bson:"since,omitempty" json:"since,omitempty"`
}

type BrokerModel struct {
	ID    string `bson:"id" json:"id"`
	Creci string `bson:"creci,omitempty" json:"creci,omitempty"`
}

type CapturingBrokerModel struct {
	ID         string    `bson:"id" json:"id"`
	Creci      string    `bson:"creci,omitempty" json:"creci,omitempty"`
	Role       string    `bson:"role" json:"role"`
	CapturedAt time.Time `bson:"captured_at" json:"captured_at"`
}

type InterestedLeadModel struct {
	LeadID       string    `bson:"lead_id" json:"lead_id"`
	InterestedAt time.Time `bson:"interested_at" json:"interested_at"`
	Source       string    `bson:"source,omitempty" json:"source,omitempty"`
}

type CompanyModel struct {
	ID             string `bson:"id" json:"id"`
	BusinessUnitID string `bson:"business_unit_id,omitempty" json:"business_unit_id,omitempty"`
	DivisionID     string `bson:"division_id,omitempty" json:"division_id,omitempty"`
}
