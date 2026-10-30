package services

import (
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
)

// BackfillContract fills empty CRM snapshot fields on c from its linked Deal d.
// Returns true when at least one field changed. Idempotent — never overwrites a
// populated value, so re-running over an already-backfilled contract is a no-op.
// Used by cmd/migrate/backfill_contract_crm to repair contracts created before
// CreateContract started snapshotting from the Deal.
func BackfillContract(c *contract.Contract, d *deal.Deal) bool {
	if c == nil || d == nil {
		return false
	}
	changed := false
	if c.PropertyID == "" && d.PropertyID != "" {
		c.PropertyID = d.PropertyID
		changed = true
	}
	if c.CustomerID == "" && d.CustomerID != "" {
		c.CustomerID = d.CustomerID
		changed = true
	}
	if c.Value == 0 && d.Value != 0 {
		c.Value = d.Value
		changed = true
	}
	if c.CommissionPct == 0 && d.Commission != 0 {
		c.CommissionPct = d.Commission
		changed = true
	}
	if c.Client == "" {
		customerRole := deal.PartyRoleTenant
		if c.Type == contract.TypeVenda {
			customerRole = deal.PartyRoleBuyer
		}
		if p := d.PrimaryByRole(customerRole); p != nil && p.Name != "" {
			c.Client = p.Name
			changed = true
		}
	}
	return changed
}
