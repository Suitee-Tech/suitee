package handlers

import (
	"github.com/oginoino/toca-core/internal/domain/core/address"
	"github.com/oginoino/toca-core/internal/domain/core/property"
)

// CreatePropertyRequest define a estrutura para criação de uma nova propriedade.
// O campo ID foi removido para garantir que seja gerado exclusivamente pelo backend.
type CreatePropertyRequest struct {
	ExternalID        string                     `json:"external_id,omitempty"`
	Title             string                     `json:"title" binding:"required"`
	Description       string                     `json:"description,omitempty"`
	Status            property.PropertyStatus    `json:"status" binding:"required,oneof=prospecting in_portfolio published attending negotiating proposal analysis contract rented sold administered rescission available inactive"`
	PublicationStatus property.PublicationStatus `json:"publication_status" binding:"required,oneof=draft published"`

	PropertyType property.PropertyType      `json:"property_type" binding:"required,oneof=apartment house land commercial"`
	ListingType  property.ListingType       `json:"listing_type" binding:"required,oneof=sale rent exchange"`
	Condition    property.PropertyCondition `json:"condition" binding:"required,oneof=new used under_construction"`

	Address address.Address `json:"address"`

	Pricing property.Pricing `json:"pricing"`

	Areas property.Areas `json:"areas"`

	Features property.Features `json:"features"`

	Amenities []string `json:"amenities,omitempty"`

	Media property.Media `json:"media,omitempty"`

	Legal property.Legal `json:"legal,omitempty"`

	Owner   property.OwnerRef   `json:"owner"`
	Broker  property.BrokerRef  `json:"broker,omitempty"`
	Company property.CompanyRef `json:"company,omitempty"`

	Owners           []property.OwnerRef           `json:"owners,omitempty"`
	CapturingBrokers []property.CapturingBrokerRef `json:"capturing_brokers,omitempty"`
	AvailableFor     []property.ListingType        `json:"available_for,omitempty"`

	// PartnershipPortfolio opts this property into the shared partnership
	// portfolio. Rejected by the service when CaptureExclusivity=exclusive.
	PartnershipPortfolio bool `json:"partnership_portfolio,omitempty"`
	// CaptureExclusivity — "open" (default) or "exclusive". An exclusive
	// capture can never be shared in the partnership portfolio.
	CaptureExclusivity property.CaptureExclusivity `json:"capture_exclusivity,omitempty" binding:"omitempty,oneof=open exclusive"`

	Tags []string `json:"tags,omitempty"`
}

// ToDomain converte o request DTO para a entidade de domínio Property.
// O ID é deixado vazio intencionalmente para ser gerado pela camada de serviço/repositório.
func (r *CreatePropertyRequest) ToDomain() *property.Property {
	return &property.Property{
		// ID será gerado no repositório/serviço
		ExternalID:           r.ExternalID,
		Title:                r.Title,
		Description:          r.Description,
		Status:               r.Status,
		PublicationStatus:    r.PublicationStatus,
		PropertyType:         r.PropertyType,
		ListingType:          r.ListingType,
		Condition:            r.Condition,
		Address:              r.Address,
		Pricing:              r.Pricing,
		Areas:                r.Areas,
		Features:             r.Features,
		Amenities:            r.Amenities,
		Media:                r.Media,
		Legal:                r.Legal,
		Owner:                r.Owner,
		Broker:               r.Broker,
		Company:              r.Company,
		Owners:               r.Owners,
		CapturingBrokers:     r.CapturingBrokers,
		AvailableFor:         r.AvailableFor,
		PartnershipPortfolio: r.PartnershipPortfolio,
		CaptureExclusivity:   r.CaptureExclusivity,
		Tags:                 r.Tags,
	}
}

// UpdatePropertyRequest define a estrutura para atualização.
// Reutiliza a estrutura de criação pois os campos permitidos são os mesmos.
// O ID não está presente no corpo, devendo ser obtido da URL.
type UpdatePropertyRequest CreatePropertyRequest

// ToDomain converte o request de update para a entidade de domínio, injetando o ID da URL.
func (r *UpdatePropertyRequest) ToDomain(id string) *property.Property {
	// Reutiliza a conversão do CreatePropertyRequest
	p := (*CreatePropertyRequest)(r).ToDomain()
	p.ID = id
	return p
}

// AddInterestedLeadRequest is used when adding a lead to a property's interest list.
type AddInterestedLeadRequest struct {
	LeadID string `json:"lead_id" binding:"required"`
	Source string `json:"source,omitempty"`
}

// AddCapturingBrokerRequest is used when adding a capturing broker to a property.
type AddCapturingBrokerRequest struct {
	BrokerID      string  `json:"broker_id" binding:"required"`
	Creci         string  `json:"creci,omitempty"`
	Role          string  `json:"role" binding:"required,oneof=captador co_captador visitador atendente fechamento imobiliaria"`
	CommissionPct float64 `json:"commission_pct,omitempty" binding:"gte=0,lte=100"`
}

// AddOwnerRequest is used when adding an owner to a property.
type AddOwnerRequest struct {
	OwnerID    string  `json:"owner_id" binding:"required"`
	OwnerType  string  `json:"owner_type" binding:"required,oneof=person company"`
	Percentage float64 `json:"percentage,omitempty"`
}

// SetActiveDealRequest is used when linking a deal to a property.
type SetActiveDealRequest struct {
	DealID string `json:"deal_id"` // empty string clears the active deal
}
