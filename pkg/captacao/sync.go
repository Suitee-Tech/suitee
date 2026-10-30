// Package captacao centralizes the bi-directional Lead↔Property linking that
// happens during the captação flow. Without this helper, both sides
// (toca-crm.lead_handler when setting Lead.CaptacaoPropertyID, and
// toca-core.property_handler when adding Property.Owners[]) would need to
// duplicate the "make the other side consistent" logic — and inconsistencies
// already happen today (audit gap 7.1).
//
// The sync is best-effort and idempotent: if the counterpart is already
// linked, the call is a no-op; if the counterpart write fails we log and
// continue, since the primary write already persisted. Strong consistency
// would require a transaction across two services and is overkill for the
// pilot — we'll revisit when Outbox supports cross-service sagas.
package captacao

import (
	"context"
	"errors"
	"fmt"
)

// PropertyOwnerLinker is the minimal interface needed to register an owner-
// lead on a property. Implemented by toca-core's property service via an
// HTTP client, or directly by the in-process service when same-binary.
type PropertyOwnerLinker interface {
	// AddOwner registers `leadID` as an owner of the property with the given
	// percentage (0-100). Must be idempotent — repeat calls with the same
	// (propertyID, leadID) shouldn't duplicate.
	AddOwner(ctx context.Context, tenantID, propertyID, leadID string, percentage float64) error
}

// LeadCaptacaoLinker is the inverse: registers `propertyID` on a lead's
// `captacao_property_id` so the lead is reachable from the property side.
// Implemented by toca-crm's lead service.
type LeadCaptacaoLinker interface {
	// SetCaptacaoProperty sets lead.captacao_property_id when empty. Should
	// also set role='owner' if it wasn't already. Must be idempotent.
	SetCaptacaoProperty(ctx context.Context, tenantID, leadID, propertyID string) error
}

// Syncer drives both directions. Construct once at server boot with the
// concrete linkers, then call FromLead / FromProperty from handlers.
type Syncer struct {
	property PropertyOwnerLinker
	lead     LeadCaptacaoLinker
	// Logger receives best-effort failures (the sync never returns them as
	// errors to keep handlers simple). Pass anything matching `Printf`.
	Logger interface {
		Printf(format string, args ...interface{})
	}
}

func NewSyncer(p PropertyOwnerLinker, l LeadCaptacaoLinker) *Syncer {
	return &Syncer{property: p, lead: l}
}

// FromLead is invoked by toca-crm after persisting a Lead with a non-empty
// CaptacaoPropertyID. It tries to make sure the Property side mirrors the
// link via AddOwner. Returns nil even on partial failure — log only.
func (s *Syncer) FromLead(ctx context.Context, tenantID, leadID, propertyID string, percentage float64) {
	if s == nil || s.property == nil || tenantID == "" || leadID == "" || propertyID == "" {
		return
	}
	if percentage <= 0 {
		percentage = 100
	}
	if err := s.property.AddOwner(ctx, tenantID, propertyID, leadID, percentage); err != nil {
		s.logf("captacao.Syncer.FromLead: AddOwner(prop=%s, lead=%s) failed: %v", propertyID, leadID, err)
	}
}

// FromProperty is invoked by toca-core after appending a new owner to a
// Property's Owners[]. It mirrors the link by setting Lead.CaptacaoPropertyID
// (and ensuring role='owner' on the lead).
func (s *Syncer) FromProperty(ctx context.Context, tenantID, propertyID, leadID string) {
	if s == nil || s.lead == nil || tenantID == "" || leadID == "" || propertyID == "" {
		return
	}
	if err := s.lead.SetCaptacaoProperty(ctx, tenantID, leadID, propertyID); err != nil {
		s.logf("captacao.Syncer.FromProperty: SetCaptacaoProperty(lead=%s, prop=%s) failed: %v", leadID, propertyID, err)
	}
}

func (s *Syncer) logf(format string, args ...interface{}) {
	if s.Logger != nil {
		s.Logger.Printf(format, args...)
	}
}

// Validate is a sanity check exposed for tests — ensures both sides are wired.
// Returns a non-nil error only if the syncer would silently no-op on every
// invocation; useful as a startup smoke test.
func (s *Syncer) Validate() error {
	if s == nil {
		return errors.New("nil syncer")
	}
	if s.property == nil && s.lead == nil {
		return fmt.Errorf("captacao.Syncer has no linkers wired")
	}
	return nil
}
