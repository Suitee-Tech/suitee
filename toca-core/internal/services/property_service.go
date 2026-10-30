package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/oginoino/toca-core/internal/addressverification"
	"github.com/oginoino/toca-core/internal/domain/core/address"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	repository "github.com/oginoino/toca-core/internal/repositories"
	appCtx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/pkg/validation"
)

// ErrExclusiveCaptureNotShareable is returned when a save attempts to mark a
// property with an EXCLUSIVE capture as shared in the partnership portfolio.
// The two states are mutually exclusive (see plan Phase 1).
var ErrExclusiveCaptureNotShareable = errors.New("property with exclusive capture cannot be shared in the partnership portfolio")

// AmenitiesValidationError marca lista de comodidades fora dos tetos. É erro
// de entrada (400), não de servidor, e a mensagem já vem pronta para o
// operador — o transporte não deve embrulhá-la com prefixo técnico.
type AmenitiesValidationError struct{ Message string }

func (e *AmenitiesValidationError) Error() string { return e.Message }

// validateSharingConsistency rejects a property that is both shared in the
// partnership portfolio AND has an exclusive capture. Called on create/update.
func validateSharingConsistency(p *property.Property) error {
	if p.PartnershipPortfolio && !p.CanSharePartnershipPortfolio() {
		return ErrExclusiveCaptureNotShareable
	}
	return nil
}

// validateAmenities normaliza a lista e recusa o que passa dos tetos
// compartilhados com o formulário (`pkg/validation/tags.go`). Sem teto, um
// cliente grava milhares de itens no JSONB do imóvel e encarece a leitura de
// todo mundo. A lista normalizada substitui a recebida (sem espaços sobrando,
// sem duplicata e sem caractere de controle).
func validateAmenities(p *property.Property) error {
	if p == nil || len(p.Amenities) == 0 {
		if p != nil {
			p.Amenities = nil
		}
		return nil
	}
	if err := validation.ValidateTags(p.Amenities, "Comodidades"); err != nil {
		return &AmenitiesValidationError{Message: err.Error()}
	}
	normalized := validation.NormalizeTags(p.Amenities)
	if len(normalized) == 0 {
		p.Amenities = nil // lista só com vazios não vira item fantasma
		return nil
	}
	p.Amenities = normalized
	return nil
}

type PropertyService interface {
	Create(ctx context.Context, p *property.Property) error
	GetByID(ctx context.Context, id string) (*property.Property, error)
	// GetByIDForTenant — leitura serviço-a-serviço com tenant explícito,
	// resolvido pela credencial interna (ex.: a DIMOB do financeiro precisa do
	// endereço/CEP do imóvel). Não confia no tenant do contexto.
	GetByIDForTenant(ctx context.Context, tenantID, id string) (*property.Property, error)
	GetAll(ctx context.Context, filter property.PropertyFilter) ([]*property.Property, int64, error)
	Update(ctx context.Context, p *property.Property) error
	// AppendImage persiste uma imagem sem perder uploads concorrentes.
	AppendImage(ctx context.Context, id string, image property.Image) (*property.Property, error)
	Delete(ctx context.Context, id string) error
	UpdateStatus(ctx context.Context, id string, status property.PropertyStatus) error

	// Cross-entity linking — single source of truth
	AddInterestedLead(ctx context.Context, propertyID, leadID, source string) error
	RemoveInterestedLead(ctx context.Context, propertyID, leadID string) error
	AddCapturingBroker(ctx context.Context, propertyID string, broker property.CapturingBrokerRef) error
	AddOwner(ctx context.Context, propertyID string, owner property.OwnerRef) error
	SetActiveDeal(ctx context.Context, propertyID, dealID string) error

	// SetEventBus opts the service into emitting property.updated events
	// for downstream projections (toca-deal contract titles, toca-portal
	// listings, etc.). Nil bus = no-op, preserving the previous behaviour.
	SetEventBus(bus events.EventBus)

	// SetAddressProcessor wires normalization and provider verification before
	// Create/Update persistence. Nil falls back to deterministic normalization.
	SetAddressProcessor(processor AddressProcessor)
}

// AddressProcessor keeps PropertyService independent from a concrete postal
// provider. addressverification.Processor is the production implementation.
type AddressProcessor interface {
	Process(ctx context.Context, value *address.Address) error
}

type propertyService struct {
	repo             repository.PropertyRepository
	crossRepo        repository.CrossTenantRepository
	eventBus         events.EventBus
	addressProcessor AddressProcessor
}

func NewPropertyService(repo repository.PropertyRepository) PropertyService {
	return &propertyService{
		repo: repo,
	}
}

// NewPropertyServiceWithCrossRepo wires an additional repo that lets the
// service validate broker/lead IDs against `crm.*` schemas. When the cross
// repo is nil (e.g. in unit tests), the validation is silently skipped — same
// pattern as toca-deal's DealService for nil repos.
func NewPropertyServiceWithCrossRepo(repo repository.PropertyRepository, crossRepo repository.CrossTenantRepository) PropertyService {
	return &propertyService{
		repo:      repo,
		crossRepo: crossRepo,
	}
}

// SetEventBus wires the event bus used to emit property.updated after a
// successful Update. Optional — when nil, the publish call is skipped and
// updates work as before. Kept as a setter (rather than another constructor
// variant) so existing call sites continue to compile.
func (s *propertyService) SetEventBus(bus events.EventBus) {
	s.eventBus = bus
}

func (s *propertyService) SetAddressProcessor(processor AddressProcessor) {
	s.addressProcessor = processor
}

func (s *propertyService) processAddress(ctx context.Context, value *address.Address) error {
	if s.addressProcessor != nil {
		return s.addressProcessor.Process(ctx, value)
	}
	// Constructor compatibility for tests/secondary entrypoints: even without
	// provider wiring, incoming values are never persisted unnormalized or with
	// a client-forged verification flag.
	if err := addressverification.Normalize(value); err != nil {
		return err
	}
	value.IsVerified = false
	if err := value.Validate(); err != nil {
		return fmt.Errorf("%w: %v", addressverification.ErrInvalidAddress, err)
	}
	return nil
}

// validateBrokerInTenant returns nil when `crossRepo` says the broker exists
// in the given tenant, an error otherwise. Skips when crossRepo is nil.
func (s *propertyService) validateBrokerInTenant(ctx context.Context, tenantID, brokerID string) error {
	if s.crossRepo == nil || brokerID == "" {
		return nil
	}
	ok, err := s.crossRepo.BrokerExists(ctx, tenantID, brokerID)
	if err != nil {
		return fmt.Errorf("validating capturing broker: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid capturing broker for tenant: broker %s not found", brokerID)
	}
	return nil
}

// validateLeadInTenant — same idea as above but for owner-lead ids.
func (s *propertyService) validateLeadInTenant(ctx context.Context, tenantID, leadID string) error {
	if s.crossRepo == nil || leadID == "" {
		return nil
	}
	ok, err := s.crossRepo.LeadExists(ctx, tenantID, leadID)
	if err != nil {
		return fmt.Errorf("validating owner lead: %w", err)
	}
	if !ok {
		return fmt.Errorf("invalid owner for tenant: lead %s not found", leadID)
	}
	return nil
}

func (s *propertyService) Create(ctx context.Context, p *property.Property) error {
	tenantID, err := appCtx.GetTenantID(ctx)
	if err != nil {
		// Fallback to AccountID if TenantID is not present (e.g. single user mode)
		tenantID, err = appCtx.GetAccountID(ctx)
		if err != nil {
			return fmt.Errorf("tenant identification failed: %w", err)
		}
	}
	p.TenantID = tenantID
	if err := s.processAddress(ctx, &p.Address); err != nil {
		return fmt.Errorf("processing property address: %w", err)
	}

	// An exclusive capture is never shareable in the partnership portfolio —
	// reject the inconsistent payload with a clear error.
	if err := validateSharingConsistency(p); err != nil {
		return err
	}
	if err := validateAmenities(p); err != nil {
		return err
	}

	// Phase 1 captação gate: when the create payload already includes
	// capturing_brokers/owners, every referenced ID must belong to the same
	// tenant. Without this, a tenant could attach a foreign broker/lead by
	// forging an id at creation time.
	for _, cb := range p.CapturingBrokers {
		if err := s.validateBrokerInTenant(ctx, tenantID, cb.ID); err != nil {
			return err
		}
	}
	for _, ow := range p.Owners {
		if err := s.validateLeadInTenant(ctx, tenantID, ow.ID); err != nil {
			return err
		}
	}

	return s.repo.Create(ctx, p)
}

func (s *propertyService) GetByID(ctx context.Context, id string) (*property.Property, error) {
	tenantID, _ := appCtx.GetTenantID(ctx)
	if tenantID == "" {
		tenantID, _ = appCtx.GetAccountID(ctx)
	}
	return s.repo.GetByID(ctx, id, tenantID)
}

func (s *propertyService) GetByIDForTenant(ctx context.Context, tenantID, id string) (*property.Property, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id required")
	}
	return s.repo.GetByID(ctx, id, tenantID)
}

func (s *propertyService) GetAll(ctx context.Context, filter property.PropertyFilter) ([]*property.Property, int64, error) {
	tenantID, _ := appCtx.GetTenantID(ctx)
	if tenantID == "" {
		tenantID, _ = appCtx.GetAccountID(ctx)
	}
	filter.TenantID = tenantID
	return s.repo.GetAll(ctx, filter)
}

func (s *propertyService) Update(ctx context.Context, p *property.Property) error {
	tenantID, err := appCtx.GetTenantID(ctx)
	if err != nil {
		tenantID, err = appCtx.GetAccountID(ctx)
		if err != nil {
			return fmt.Errorf("tenant identification failed: %w", err)
		}
	}

	// Ensure we are updating a property that belongs to this tenant
	// Also ensure p.TenantID matches context to prevent moving property to another tenant
	p.TenantID = tenantID
	if err := s.processAddress(ctx, &p.Address); err != nil {
		return fmt.Errorf("processing property address: %w", err)
	}

	// An exclusive capture is never shareable in the partnership portfolio.
	if err := validateSharingConsistency(p); err != nil {
		return err
	}
	if err := validateAmenities(p); err != nil {
		return err
	}

	if err := s.repo.Update(ctx, p); err != nil {
		return err
	}

	// Emit property.updated so downstream services that snapshot property
	// labels (toca-deal Contract.PropertyTitle, toca-portal listings,
	// toca-financial fiscal records) can refresh their projections. Failure
	// to enqueue is logged but does not block — the canonical state in
	// toca-core is already persisted and consumers can also reconcile via
	// scheduled jobs.
	s.emitPropertyUpdated(ctx, p)
	return nil
}

func (s *propertyService) emitPropertyUpdated(ctx context.Context, p *property.Property) {
	if s.eventBus == nil || p == nil {
		return
	}
	evt := contracts.PropertyUpdatedEvent{
		Data: contracts.PropertyUpdatedEventPayload{
			PropertyID:    p.ID,
			TenantID:      p.TenantID,
			PropertyTitle: propertyTitle(p),
			Status:        string(p.Status),
			ListingType:   propertyListingType(p),
			Address:       propertyAddress(p),
			ExpectedValue: propertyExpectedValue(p),
			UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := s.eventBus.PublishReliable(ctx, evt); err != nil {
		log.Printf("WARN: failed to enqueue property.updated event for property=%s: %v", p.ID, err)
	}
}

// propertyTitle / propertyListingType / propertyAddress / propertyExpectedValue
// are deliberate thin getters so the Update handler doesn't need to know the
// internal property struct layout. They fall back to empty values when the
// underlying field is missing — downstream consumers must treat each field as
// optional.
func propertyTitle(p *property.Property) string {
	if p == nil {
		return ""
	}
	return p.Title
}

func propertyListingType(p *property.Property) string {
	if p == nil {
		return ""
	}
	return string(p.ListingType)
}

func propertyAddress(p *property.Property) string {
	if p == nil {
		return ""
	}
	if p.Address.FormattedAddress != "" {
		return p.Address.FormattedAddress
	}
	// Cheap fallback if the formatted_address column was never populated.
	parts := make([]string, 0, 5)
	if p.Address.Street != "" {
		parts = append(parts, p.Address.Street)
	}
	if p.Address.Number != "" {
		parts = append(parts, p.Address.Number)
	}
	if p.Address.Neighborhood != "" {
		parts = append(parts, p.Address.Neighborhood)
	}
	if p.Address.City != "" {
		parts = append(parts, p.Address.City)
	}
	if p.Address.StateCode != "" {
		parts = append(parts, p.Address.StateCode)
	}
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += ", "
		}
		out += part
	}
	return out
}

func propertyExpectedValue(p *property.Property) float64 {
	if p == nil {
		return 0
	}
	if p.Pricing.SalePrice != nil {
		return p.Pricing.SalePrice.CurrentAmount
	}
	if p.Pricing.RentPrice != nil {
		return p.Pricing.RentPrice.CurrentAmount
	}
	return 0
}

func (s *propertyService) AppendImage(ctx context.Context, id string, image property.Image) (*property.Property, error) {
	tenantID, err := appCtx.GetTenantID(ctx)
	if err != nil {
		tenantID, err = appCtx.GetAccountID(ctx)
		if err != nil {
			return nil, fmt.Errorf("tenant identification failed: %w", err)
		}
	}
	p, err := s.repo.AppendImage(ctx, id, tenantID, image)
	if err != nil {
		return nil, err
	}
	s.emitPropertyUpdated(ctx, p)
	return p, nil
}

func (s *propertyService) Delete(ctx context.Context, id string) error {
	tenantID, _ := appCtx.GetTenantID(ctx)
	if tenantID == "" {
		tenantID, _ = appCtx.GetAccountID(ctx)
	}
	return s.repo.Delete(ctx, id, tenantID)
}

func (s *propertyService) UpdateStatus(ctx context.Context, id string, status property.PropertyStatus) error {
	p, err := s.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if !isValidTransition(p.Status, status) {
		return fmt.Errorf("invalid status transition from %s to %s", p.Status, status)
	}

	p.Status = status
	return s.Update(ctx, p)
}

// AddInterestedLead adds a lead to the property's interested leads list (idempotent).
func (s *propertyService) AddInterestedLead(ctx context.Context, propertyID, leadID, source string) error {
	p, err := s.GetByID(ctx, propertyID)
	if err != nil {
		return err
	}
	// Check for duplicates
	for _, il := range p.InterestedLeads {
		if il.LeadID == leadID {
			return nil // already registered, idempotent
		}
	}
	p.InterestedLeads = append(p.InterestedLeads, property.InterestedLeadRef{
		LeadID:       leadID,
		InterestedAt: time.Now(),
		Source:       source,
	})
	return s.Update(ctx, p)
}

// RemoveInterestedLead removes a lead from the property's interested leads list.
func (s *propertyService) RemoveInterestedLead(ctx context.Context, propertyID, leadID string) error {
	p, err := s.GetByID(ctx, propertyID)
	if err != nil {
		return err
	}
	filtered := p.InterestedLeads[:0]
	for _, il := range p.InterestedLeads {
		if il.LeadID != leadID {
			filtered = append(filtered, il)
		}
	}
	p.InterestedLeads = filtered
	return s.Update(ctx, p)
}

// AddCapturingBroker adds a capturing broker to the property (idempotent by broker ID).
func (s *propertyService) AddCapturingBroker(ctx context.Context, propertyID string, broker property.CapturingBrokerRef) error {
	p, err := s.GetByID(ctx, propertyID)
	if err != nil {
		return err
	}
	// Validate the broker belongs to the same tenant as the property — the
	// property fetch above already verified the property is in this tenant
	// (s.GetByID is tenant-scoped).
	if err := s.validateBrokerInTenant(ctx, p.TenantID, broker.ID); err != nil {
		return err
	}
	for _, cb := range p.CapturingBrokers {
		if cb.ID == broker.ID {
			return nil // already registered, idempotent
		}
	}
	if broker.CapturedAt.IsZero() {
		broker.CapturedAt = time.Now()
	}
	p.CapturingBrokers = append(p.CapturingBrokers, broker)
	return s.Update(ctx, p)
}

// AddOwner adds an owner to the property's owners list (idempotent by owner ID).
func (s *propertyService) AddOwner(ctx context.Context, propertyID string, owner property.OwnerRef) error {
	p, err := s.GetByID(ctx, propertyID)
	if err != nil {
		return err
	}
	if err := s.validateLeadInTenant(ctx, p.TenantID, owner.ID); err != nil {
		return err
	}
	for _, o := range p.Owners {
		if o.ID == owner.ID {
			return nil // already registered, idempotent
		}
	}
	if owner.Since.IsZero() {
		owner.Since = time.Now()
	}
	p.Owners = append(p.Owners, owner)
	return s.Update(ctx, p)
}

// SetActiveDeal sets or clears the active deal reference on a property.
func (s *propertyService) SetActiveDeal(ctx context.Context, propertyID, dealID string) error {
	p, err := s.GetByID(ctx, propertyID)
	if err != nil {
		return err
	}
	p.ActiveDealID = dealID
	return s.Update(ctx, p)
}

func isValidTransition(current, next property.PropertyStatus) bool {
	// If it's the same status, it's valid (idempotency)
	if current == next {
		return true
	}

	// Admin can force inactive from anywhere
	if next == property.PropertyStatusInactive {
		return true
	}

	switch current {
	case property.PropertyStatusProspecting:
		return next == property.PropertyStatusInPortfolio

	case property.PropertyStatusInPortfolio:
		return next == property.PropertyStatusPublished || next == property.PropertyStatusProspecting

	case property.PropertyStatusPublished:
		return next == property.PropertyStatusAttending || next == property.PropertyStatusInPortfolio

	case property.PropertyStatusAttending:
		return next == property.PropertyStatusNegotiating || next == property.PropertyStatusPublished

	case property.PropertyStatusNegotiating:
		return next == property.PropertyStatusProposal || next == property.PropertyStatusAttending

	case property.PropertyStatusProposal:
		return next == property.PropertyStatusAnalysis || next == property.PropertyStatusNegotiating

	case property.PropertyStatusAnalysis:
		return next == property.PropertyStatusContract || next == property.PropertyStatusProposal || next == property.PropertyStatusPublished // Rejected

	case property.PropertyStatusContract:
		return next == property.PropertyStatusRented || next == property.PropertyStatusSold || next == property.PropertyStatusAnalysis

	case property.PropertyStatusRented:
		return next == property.PropertyStatusAdministered

	case property.PropertyStatusAdministered:
		return next == property.PropertyStatusRescission

	case property.PropertyStatusRescission:
		return next == property.PropertyStatusAvailable

	case property.PropertyStatusAvailable:
		return next == property.PropertyStatusInPortfolio || next == property.PropertyStatusPublished

	case property.PropertyStatusSold:
		return false // End state

	case property.PropertyStatusInactive:
		return next == property.PropertyStatusInPortfolio // Reactivation

	default:
		return false
	}
}
