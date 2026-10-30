package property

type PropertyStatus string

const (
	PropertyStatusProspecting  PropertyStatus = "prospecting"  // Em Captação
	PropertyStatusInPortfolio  PropertyStatus = "in_portfolio" // Em Portfólio
	PropertyStatusPublished    PropertyStatus = "published"    // Anunciado
	PropertyStatusAttending    PropertyStatus = "attending"    // Em Atendimento
	PropertyStatusNegotiating  PropertyStatus = "negotiating"  // Em Negociação
	PropertyStatusProposal     PropertyStatus = "proposal"     // Em Proposta
	PropertyStatusAnalysis     PropertyStatus = "analysis"     // Em Análise
	PropertyStatusContract     PropertyStatus = "contract"     // Em Contrato
	PropertyStatusRented       PropertyStatus = "rented"       // Alugado
	PropertyStatusSold         PropertyStatus = "sold"         // Vendido
	PropertyStatusAdministered PropertyStatus = "administered" // Em Administração
	PropertyStatusRescission   PropertyStatus = "rescission"   // Em Rescisão
	PropertyStatusAvailable    PropertyStatus = "available"    // Disponível
	PropertyStatusInactive     PropertyStatus = "inactive"     // Inativo
)

type PublicationStatus string

const (
	PublicationStatusDraft     PublicationStatus = "draft"
	PublicationStatusPublished PublicationStatus = "published"
)

type PropertyType string

const (
	PropertyTypeApartment  PropertyType = "apartment"
	PropertyTypeHouse      PropertyType = "house"
	PropertyTypeLand       PropertyType = "land"
	PropertyTypeCommercial PropertyType = "commercial"
)

type ListingType string

const (
	ListingTypeSale     ListingType = "sale"
	ListingTypeRent     ListingType = "rent"
	ListingTypeExchange ListingType = "exchange"
)

// BrokerRole represents the participation role of a broker in a property or deal
type BrokerRole string

const (
	BrokerRoleCaptador    BrokerRole = "captador"    // Capturing broker
	BrokerRoleVisitador   BrokerRole = "visitador"   // Visiting broker
	BrokerRoleAtendente   BrokerRole = "atendente"   // Lead service broker
	BrokerRoleFechamento  BrokerRole = "fechamento"  // Closing broker
	BrokerRoleCoCaptador  BrokerRole = "co_captador" // Co-capturing broker
	BrokerRoleImobiliaria BrokerRole = "imobiliaria" // Agency retained share
)

// ClientRole represents the role of a lead/client in a transaction
type ClientRole string

const (
	ClientRoleOwner     ClientRole = "owner"     // Proprietário
	ClientRoleBuyer     ClientRole = "buyer"     // Comprador
	ClientRoleTenant    ClientRole = "tenant"    // Inquilino
	ClientRoleGuarantor ClientRole = "guarantor" // Fiador
)

type PropertyCondition string

const (
	PropertyConditionNew               PropertyCondition = "new"
	PropertyConditionUsed              PropertyCondition = "used"
	PropertyConditionUnderConstruction PropertyCondition = "under_construction"
)

// CaptureExclusivity describes the brokerage exclusivity of a property's
// capture (captação). An "exclusive" capture means only the capturing company
// may broker the property — it can never be shared in the partnership
// portfolio. An empty value is treated as "open".
type CaptureExclusivity string

const (
	CaptureExclusivityOpen      CaptureExclusivity = "open"
	CaptureExclusivityExclusive CaptureExclusivity = "exclusive"
)
