// Package commission models the per-tenant rules that decide how the deal
// commission is split between brokers participating in the deal (captador,
// visitador, fechamento). Until now the split was hardcoded at 50% in the
// event_handler tests (audit gap 7.3); this package introduces a real
// configuration store with sensible defaults so admins can tune per tenant.
package commission

import (
	"errors"
	"strings"
	"time"
)

// Role mirrors deal.BrokerParticipationRole + the property capturing roles —
// any role that can hold a slice of the commission. Strings instead of an
// enum because the deal/property packages already use strings and we want to
// stay loose-coupled.
type Role string

const (
	RoleCaptador    Role = "captador"
	RoleCoCaptador  Role = "co_captador"
	RoleVisitador   Role = "visitador"
	RoleAtendente   Role = "atendente"
	RoleFechamento  Role = "fechamento"
	RoleImobiliaria Role = "imobiliaria"
)

// DefaultPercentages is what we seed for a new tenant. Numbers reflect the
// most common split observed in the existing test fixtures and current
// industry practice in the Brazilian residential market — captador shares
// the captação fee, fechamento gets the lion's share for closing the deal.
var DefaultPercentages = map[Role]float64{
	RoleCaptador:    30,
	RoleCoCaptador:  0, // co-captador only earns when explicitly configured
	RoleVisitador:   20,
	RoleAtendente:   0,
	RoleFechamento:  50,
	RoleImobiliaria: 0,
}

// Rule is a single (tenant, role) → percentage entry. The set of Rules for
// one tenant should sum to 100 across the **active** roles being used in a
// given deal — but the data model doesn't enforce that; it's the caller's
// responsibility to apply percentages relative to the participations.
type Rule struct {
	TenantID          string    `json:"tenant_id"`
	Role              Role      `json:"role"`
	DefaultPercentage float64   `json:"default_percentage"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Validate ensures the rule is internally consistent. Returns the first
// problem found, or nil. Callers should call this in handlers before
// persisting user-supplied rules.
func (r *Rule) Validate() error {
	if r == nil {
		return errors.New("rule is nil")
	}
	if strings.TrimSpace(r.TenantID) == "" {
		return errors.New("tenant_id is required")
	}
	if !validRole(r.Role) {
		return errors.New("invalid role")
	}
	if r.DefaultPercentage < 0 || r.DefaultPercentage > 100 {
		return errors.New("default_percentage must be between 0 and 100")
	}
	return nil
}

func validRole(r Role) bool {
	switch r {
	case RoleCaptador, RoleCoCaptador, RoleVisitador, RoleAtendente, RoleFechamento, RoleImobiliaria:
		return true
	}
	return false
}

// PercentageFor returns the configured percentage for `role` from the slice,
// falling back to DefaultPercentages when the tenant hasn't customised it.
// O(n) over `rules` — fine because n ≤ 4 in practice.
func PercentageFor(rules []Rule, role Role) float64 {
	for _, r := range rules {
		if r.Role == role {
			return r.DefaultPercentage
		}
	}
	if def, ok := DefaultPercentages[role]; ok {
		return def
	}
	return 0
}
