package services

import (
	"context"
	"errors"
	"fmt"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"log"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	"github.com/oginoino/toca-core/internal/domain/core/property"
	repository "github.com/oginoino/toca-core/internal/repositories"
	"github.com/oginoino/toca/pkg/events"
)

// ErrSharedPortfolioUnavailable is returned by SharedPortfolioForPartnership
// when the partnership exists but is not active or does not grant the
// shared_portfolio view permission.
var ErrSharedPortfolioUnavailable = errors.New("shared portfolio not available for this partnership")

type PartnershipService struct {
	repo         repository.PartnershipRepository
	propertyRepo repository.PropertyRepository
	eventBus     events.EventBus
}

func NewPartnershipService(repo repository.PartnershipRepository) *PartnershipService {
	return &PartnershipService{
		repo: repo,
	}
}

// SetPropertyRepository wires the property repository the shared-portfolio
// endpoint needs to list the counterpart company's properties. Optional —
// mirroring SetEventBus so existing call sites keep compiling; when nil,
// SharedPortfolioForPartnership returns an error instead of panicking.
func (s *PartnershipService) SetPropertyRepository(repo repository.PropertyRepository) {
	s.propertyRepo = repo
}

// SetEventBus preserves the application wiring contract. Partnership mutations
// are persisted through the repository outbox in the same transaction.
func (s *PartnershipService) SetEventBus(bus events.EventBus) {
	s.eventBus = bus
}

func (s *PartnershipService) CreatePartnership(ctx context.Context, p *partnership.Partnership, userID ...string) error {
	if p == nil {
		return fmt.Errorf("%w: parceria obrigatória", ErrAgreementValidation)
	}
	p.Status = partnership.PartnershipStatusPending
	if err := p.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrAgreementValidation, err)
	}
	grant := access.Grant{Sharing: p.SharingConfig, Permissions: p.Permissions}
	if err := grant.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrAgreementValidation, err)
	}
	p.ID = uuid.NewString()
	p.Version = 1
	p.FinancialVersion = 1
	p.CreatedAt = time.Now()
	p.UpdatedAt = p.CreatedAt
	p.Grants = map[string]access.Grant{p.RequesterID: grant, p.TargetID: {}}
	actor := ""
	if len(userID) > 0 {
		actor = userID[0]
	}
	p.AuditLogs = []partnership.AuditLog{{Action: "created", UserID: actor, TenantID: p.RequesterID, Version: 1, Timestamp: p.CreatedAt, After: p.FinancialConfig}}
	p.EventAction = "invited"
	return s.repo.Create(ctx, p)
}

func (s *PartnershipService) GetPartnership(ctx context.Context, id string) (*partnership.Partnership, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *PartnershipService) ListPartnerships(ctx context.Context, filter map[string]interface{}) ([]*partnership.Partnership, error) {
	return s.repo.List(ctx, filter)
}

var ErrAgreementConflict = partnership.ErrConflict
var ErrAgreementAccess = errors.New("access denied")
var ErrAgreementValidation = errors.New("acordo inválido")
var ErrAgreementVersion = errors.New("recarregue o acordo e envie a versão revisada")

type AgreementChange struct {
	ExpectedVersion *int                         `json:"expected_version"`
	Action          string                       `json:"action"`
	Grant           *access.Grant                `json:"grant,omitempty"`
	FinancialConfig *partnership.FinancialConfig `json:"financial_config,omitempty"`
	Reason          string                       `json:"reason,omitempty"`
}

// ChangeAgreement applies one reviewed command. Repository Update uses version
// compare-and-set and persists the change, audit and outbox in one transaction.
func (s *PartnershipService) ChangeAgreement(ctx context.Context, id, tenant, user string, c AgreementChange) (*partnership.Partnership, error) {
	p, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, partnership.ErrNotFound
	}
	if !p.IsParticipant(tenant) {
		return nil, ErrAgreementAccess
	}
	if c.ExpectedVersion == nil {
		return nil, ErrAgreementVersion
	}
	if *c.ExpectedVersion != p.Version {
		return nil, ErrAgreementConflict
	}
	if c.Grant != nil {
		if err := c.Grant.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrAgreementValidation, err)
		}
	}
	if c.FinancialConfig != nil {
		if err := c.FinancialConfig.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrAgreementValidation, err)
		}
	}
	before := map[string]any{"status": p.Status, "financial_config": p.FinancialConfig, "grant": p.GrantFrom(tenant), "pending_financial": p.PendingFinancial}
	p.FreezeGrants()
	switch c.Action {
	case "save":
		if p.Status != partnership.PartnershipStatusActive {
			return nil, fmt.Errorf("%w: somente parcerias ativas podem ser alteradas", ErrAgreementValidation)
		}
		if c.Grant != nil {
			p.Grants[tenant] = *c.Grant
		}
		if c.FinancialConfig != nil && !c.FinancialConfig.Equivalent(p.FinancialConfig) {
			if p.PendingFinancial != nil {
				return nil, fmt.Errorf("%w: resolva a proposta financeira pendente antes de enviar outra", ErrAgreementValidation)
			}
			p.PendingFinancial = &partnership.FinancialProposal{ProposedBy: tenant, Version: p.Version + 1, Config: *c.FinancialConfig}
		}
	case "accept":
		if err := p.FinancialConfig.Validate(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrAgreementValidation, err)
		}
		if p.Status != partnership.PartnershipStatusPending || tenant != p.TargetID {
			return nil, ErrAgreementAccess
		}
		if c.Grant == nil {
			return nil, fmt.Errorf("%w: revise o que sua empresa compartilhará", ErrAgreementValidation)
		}
		p.Grants[tenant] = *c.Grant
		p.Status = partnership.PartnershipStatusActive
	case "accept_financial", "reject_financial":
		if p.Status != partnership.PartnershipStatusActive || p.PendingFinancial == nil || p.PendingFinancial.ProposedBy == tenant {
			return nil, ErrAgreementAccess
		}
		if c.Action == "accept_financial" {
			if err := p.PendingFinancial.Config.Validate(); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrAgreementValidation, err)
			}
			p.FinancialConfig = p.PendingFinancial.Config
			p.FinancialVersion = p.Version + 1
		}
		p.PendingFinancial = nil
	case "cancel", "reject":
		if p.Status != partnership.PartnershipStatusPending {
			return nil, ErrAgreementConflict
		}
		if (c.Action == "cancel" && tenant != p.RequesterID) || (c.Action == "reject" && tenant != p.TargetID) {
			return nil, ErrAgreementAccess
		}
		p.Status = partnership.PartnershipStatusInactive
	case "suspend", "terminate":
		if p.Status != partnership.PartnershipStatusActive && string(p.Status) != "suspended" {
			return nil, ErrAgreementConflict
		}
		if strings.TrimSpace(c.Reason) == "" {
			return nil, fmt.Errorf("%w: informe o motivo", ErrAgreementValidation)
		}
		p.Status = partnership.PartnershipStatusInactive
		if c.Action == "suspend" {
			p.Status = "suspended"
		}
		p.PendingFinancial = nil
	default:
		return nil, fmt.Errorf("%w: ação de parceria inválida", ErrAgreementValidation)
	}
	p.Version++
	p.UpdatedAt = time.Now()
	p.EventAction = "changed"
	if c.Action == "accept" {
		p.EventAction = "accepted"
	}
	after := map[string]any{"status": p.Status, "financial_config": p.FinancialConfig, "grant": p.GrantFrom(tenant), "pending_financial": p.PendingFinancial}
	p.AuditLogs = append(p.AuditLogs, partnership.AuditLog{Action: c.Action, UserID: user, TenantID: tenant, Version: p.Version, Timestamp: p.UpdatedAt, Details: strings.TrimSpace(c.Reason), Before: before, After: after})
	if err := s.repo.Update(ctx, tenant, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Legacy mutation paths fail closed: clients must send one versioned command.
func (s *PartnershipService) AcceptPartnership(context.Context, string, string, string) (*partnership.Partnership, error) {
	return nil, ErrAgreementVersion
}
func (s *PartnershipService) RejectPartnership(context.Context, string, string, string) (*partnership.Partnership, error) {
	return nil, ErrAgreementVersion
}
func (s *PartnershipService) UpdateFinancialConfig(context.Context, string, partnership.FinancialConfig, string, string) error {
	return ErrAgreementVersion
}
func (s *PartnershipService) UpdateSharingConfig(context.Context, string, partnership.SharingConfig, string, string) error {
	return ErrAgreementVersion
}
func (s *PartnershipService) UpdatePermissions(context.Context, string, []partnership.Permission, string, string) error {
	return ErrAgreementVersion
}

// SharedPortfolioForPartnership returns the COUNTERPART company's shared
// properties for a partnership the caller belongs to, with each property
// privacy-projected for partner consumption.
//
// The method enforces, in order:
//  1. the partnership exists (else "partnership not found");
//  2. the caller is a participant (else "access denied" → 403);
//  3. the partnership is active (else ErrSharedPortfolioUnavailable);
//  4. the partnership grants shared_portfolio view (else ErrSharedPortfolioUnavailable).
//
// Only after all checks pass does it cross tenant isolation to list the
// counterpart's properties. A shared-portfolio access audit log is appended
// to the partnership (best-effort — a logging failure does not fail the read).
func (s *PartnershipService) SharedPortfolioForPartnership(ctx context.Context, partnershipID, callerTenantID string) ([]*property.Property, error) {
	p, err := s.repo.GetByID(ctx, partnershipID)
	if err != nil {
		return nil, errors.New("partnership not found")
	}
	if p == nil {
		return nil, errors.New("partnership not found")
	}

	if !p.IsParticipant(callerTenantID) {
		return nil, errors.New("access denied")
	}

	if p.Status != partnership.PartnershipStatusActive {
		return nil, ErrSharedPortfolioUnavailable
	}

	if !p.GrantFrom(p.Counterpart(callerTenantID)).Allows("shared_portfolio") {
		return nil, ErrSharedPortfolioUnavailable
	}

	if s.propertyRepo == nil {
		return nil, errors.New("property repository not configured")
	}

	ownerTenantID := p.Counterpart(callerTenantID)
	if ownerTenantID == "" {
		// Defensive: IsParticipant already guarantees this, but guard so a
		// bug never leaks an empty-tenant cross-schema query.
		return nil, errors.New("access denied")
	}

	// The owner company's SharingConfig narrows what it exposes — its
	// autonomy over the shared portfolio (property types and regions).
	props, err := s.propertyRepo.ListSharedForPartner(
		ctx,
		ownerTenantID,
		p.GrantFrom(ownerTenantID).Sharing.SharedPropertyTypes,
		p.GrantFrom(ownerTenantID).Sharing.SharedRegions,
	)
	if err != nil {
		return nil, err
	}

	projected := make([]*property.Property, 0, len(props))
	for _, prop := range props {
		projected = append(projected, property.ProjectForPartner(prop, p.GrantFrom(ownerTenantID).Sharing.ShareOwnerContact))
	}

	// Audit the cross-tenant access. Best-effort — the read already
	// succeeded and the partnership state is canonical.
	auditLog := partnership.AuditLog{
		Action:    "shared_portfolio_accessed",
		UserID:    callerTenantID,
		Timestamp: time.Now(),
		Details:   "Shared portfolio of partner company accessed",
	}
	if err := s.repo.AddAuditLog(ctx, callerTenantID, partnershipID, auditLog); err != nil {
		log.Printf("WARN: failed to append shared_portfolio_accessed audit log for partnership=%s: %v", partnershipID, err)
	}

	return projected, nil
}

// SharedProperty wraps a privacy-projected property exposed through the
// shared portfolio with the partnership it came from. SourceCompanyID is the
// counterpart (owner) tenant; SourceCompanyName is best-effort — toca-core has
// no cross-company name lookup, so it is left empty (the frontend resolves the
// label from the company directory it already loads).
type SharedProperty struct {
	Property          *property.Property `json:"property"`
	PartnershipID     string             `json:"partnership_id"`
	SourceCompanyID   string             `json:"source_company_id"`
	SourceCompanyName string             `json:"source_company_name,omitempty"`
}

// AllSharedPropertiesForCompany aggregates the shared portfolio across EVERY
// active partnership the caller participates in. For each active partnership
// that grants shared_portfolio view, it lists the counterpart company's
// shared properties (narrowed by that partnership's SharingConfig) and
// projects each for partner consumption. Every cross-tenant access is audited
// on the originating partnership (best-effort).
//
// Unlike SharedPortfolioForPartnership this never fails the whole request for
// a single inactive/unpermitted partnership — it simply skips it. A repository
// error on one partnership's listing aborts the call (the caller gets a 500).
func (s *PartnershipService) AllSharedPropertiesForCompany(ctx context.Context, callerTenantID string) ([]SharedProperty, error) {
	if callerTenantID == "" {
		return nil, errors.New("access denied")
	}
	if s.propertyRepo == nil {
		return nil, errors.New("property repository not configured")
	}

	// Every partnership the caller belongs to, in either direction.
	asRequester, err := s.repo.List(ctx, map[string]interface{}{
		"requester_id": callerTenantID,
		"status":       partnership.PartnershipStatusActive,
	})
	if err != nil {
		return nil, err
	}
	asTarget, err := s.repo.List(ctx, map[string]interface{}{
		"target_id": callerTenantID,
		"status":    partnership.PartnershipStatusActive,
	})
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	out := make([]SharedProperty, 0)

	collect := func(partnerships []*partnership.Partnership) error {
		for _, p := range partnerships {
			if p == nil {
				continue
			}
			if _, dup := seen[p.ID]; dup {
				continue
			}
			seen[p.ID] = struct{}{}

			if p.Status != partnership.PartnershipStatusActive {
				continue
			}
			if !p.IsParticipant(callerTenantID) {
				continue
			}
			if !p.GrantFrom(p.Counterpart(callerTenantID)).Allows("shared_portfolio") {
				continue
			}
			ownerTenantID := p.Counterpart(callerTenantID)
			if ownerTenantID == "" {
				continue
			}

			props, err := s.propertyRepo.ListSharedForPartner(
				ctx,
				ownerTenantID,
				p.GrantFrom(ownerTenantID).Sharing.SharedPropertyTypes,
				p.GrantFrom(ownerTenantID).Sharing.SharedRegions,
			)
			if err != nil {
				return err
			}
			for _, prop := range props {
				out = append(out, SharedProperty{
					Property:          property.ProjectForPartner(prop, p.GrantFrom(ownerTenantID).Sharing.ShareOwnerContact),
					PartnershipID:     p.ID,
					SourceCompanyID:   ownerTenantID,
					SourceCompanyName: p.Companies[ownerTenantID].Name,
				})
			}

			// Audit the cross-tenant access. Best-effort.
			auditLog := partnership.AuditLog{
				Action:    "shared_portfolio_accessed",
				UserID:    callerTenantID,
				Timestamp: time.Now(),
				Details:   "Shared portfolio of partner company accessed (aggregate)",
			}
			if err := s.repo.AddAuditLog(ctx, callerTenantID, p.ID, auditLog); err != nil {
				log.Printf("WARN: failed to append shared_portfolio_accessed audit log for partnership=%s: %v", p.ID, err)
			}
		}
		return nil
	}

	if err := collect(asRequester); err != nil {
		return nil, err
	}
	if err := collect(asTarget); err != nil {
		return nil, err
	}

	return out, nil
}

// CheckPermission verifies if a partnership allows a specific action
func (s *PartnershipService) CheckPermission(ctx context.Context, requesterID, targetID string, scope partnership.PermissionScope, access partnership.AccessLevel) (bool, error) {
	// requesterID consumes data owned by targetID, regardless of who invited whom.
	partnerships, err := s.repo.List(ctx, map[string]interface{}{"status": partnership.PartnershipStatusActive})
	if err != nil {
		return false, err
	}
	for _, p := range partnerships {
		if requesterID != targetID && p.IsParticipant(requesterID) && p.IsParticipant(targetID) && access == partnership.AccessLevelView && p.GrantFrom(targetID).Allows(scope) {
			return true, nil
		}
	}
	return false, nil
}
