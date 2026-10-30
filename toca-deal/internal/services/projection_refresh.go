package services

import (
	"context"
	"fmt"
	"log"
	"strings"

	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// NewPropertyUpdatedHandler returns the bus handler that consumes
// property.updated and refreshes Contract.PropertyTitle on every contract
// that references the updated property.
//
// Closes the loop on audit P1.8: toca-core now emits property.updated when
// a property is renamed/edited; this consumer keeps the toca-deal snapshot
// in sync without forcing the UI to do a fresh lookup. Idempotent — a
// no-op title diff results in no DB write.
func NewPropertyUpdatedHandler(contractRepo contract.Repository) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("property.updated: unexpected payload type %T", evt.Payload())
			return nil
		}
		propertyID := stringField(payload, "property_id")
		tenantID := stringField(payload, "tenant_id")
		newTitle := stringField(payload, "property_title")
		if propertyID == "" {
			return nil
		}

		// ListByPropertyID is intentionally cross-tenant (service pool /
		// BYPASSRLS) because the event carries the tenant_id verbatim and
		// the row metadata already pins the tenant — we restore the right
		// tenant on the context before each Update so the repo's
		// per-tenant invariants stay honoured.
		contracts, err := contractRepo.ListByPropertyID(ctx, propertyID)
		if err != nil {
			return fmt.Errorf("list contracts by property %s: %w", propertyID, err)
		}
		for _, c := range contracts {
			if c == nil {
				continue
			}
			if c.PropertyTitle == newTitle {
				continue
			}
			// Restore the tenant from the row so the underlying repo
			// honours the right scope when persisting.
			updateCtx := pkgctx.WithTenantID(ctx, c.TenantID)
			if tenantID != "" && c.TenantID != tenantID {
				// Defensive: skip if the row claims a different tenant
				// than the event. Should never happen in production but
				// avoids silently mixing tenants on a payload drift.
				log.Printf("property.updated: contract %s tenant=%s diverges from event tenant=%s; skipping", c.ID, c.TenantID, tenantID)
				continue
			}
			c.PropertyTitle = newTitle
			if err := contractRepo.Update(updateCtx, c); err != nil {
				return fmt.Errorf("update contract %s with refreshed title: %w", c.ID, err)
			}
		}
		return nil
	}
}

// NewLeadUpdatedHandler returns the bus handler that consumes lead.updated
// and refreshes Party.Name/Email/Phone snapshots in every Deal whose Party
// list references the updated Lead.
//
// Party labels are denormalized from Lead at deal creation; without this
// consumer, renaming a Lead in toca-crm leaves the deal showing stale
// data until the user manually re-saves. dealRepo.FindByPartyLeadID
// already exists for this exact purpose — we just wire it into the event.
func NewLeadUpdatedHandler(dealRepo deal.Repository) events.EventHandler {
	return func(ctx context.Context, evt events.Event) error {
		payload, ok := evt.Payload().(map[string]interface{})
		if !ok {
			log.Printf("lead.updated: unexpected payload type %T", evt.Payload())
			return nil
		}
		leadID := stringField(payload, "lead_id")
		tenantID := stringField(payload, "tenant_id")
		name := strings.TrimSpace(stringField(payload, "name"))
		email := strings.TrimSpace(stringField(payload, "email"))
		phone := strings.TrimSpace(stringField(payload, "phone"))
		if leadID == "" {
			return nil
		}
		// At least one identifying field is required — if all three are
		// empty the event is meaningless for label refresh, skip cleanly.
		if name == "" && email == "" && phone == "" {
			return nil
		}

		deals, err := dealRepo.FindByPartyLeadID(ctx, leadID)
		if err != nil {
			return fmt.Errorf("find deals by party lead %s: %w", leadID, err)
		}
		for _, d := range deals {
			if d == nil {
				continue
			}
			if tenantID != "" && d.TenantID != tenantID {
				log.Printf("lead.updated: deal %s tenant=%s diverges from event tenant=%s; skipping", d.ID, d.TenantID, tenantID)
				continue
			}
			changed := false
			for i := range d.Parties {
				if d.Parties[i].LeadID != leadID {
					continue
				}
				if name != "" && d.Parties[i].Name != name {
					d.Parties[i].Name = name
					changed = true
				}
				if email != "" && !strings.EqualFold(d.Parties[i].Email, email) {
					d.Parties[i].Email = email
					changed = true
				}
				if phone != "" && d.Parties[i].Phone != phone {
					d.Parties[i].Phone = phone
					changed = true
				}
			}
			if !changed {
				continue
			}
			updateCtx := pkgctx.WithTenantID(ctx, d.TenantID)
			if err := dealRepo.Update(updateCtx, d); err != nil {
				return fmt.Errorf("update deal %s with refreshed party labels: %w", d.ID, err)
			}
		}
		return nil
	}
}
