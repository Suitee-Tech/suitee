// Package partnershipaccess is the shared policy for partner reads and operations.
package partnershipaccess

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

type Permission struct {
	Scope       string `json:"scope"`
	AccessLevel string `json:"access_level"`
}
type Sharing struct {
	ShareOwnerContact   bool     `json:"share_owner_contact"`
	ShareLeadContact    bool     `json:"share_lead_contact"`
	AllowCoBrokering    bool     `json:"allow_co_brokering"`
	SharedPropertyTypes []string `json:"shared_property_types"`
	SharedRegions       []string `json:"shared_regions"`
}
type Grant struct {
	Sharing     Sharing      `json:"sharing_config"`
	Permissions []Permission `json:"permissions"`
}

func (g Grant) Allows(scope string) bool {
	for _, p := range g.Permissions {
		if p.Scope == scope && p.AccessLevel == "view" {
			return true
		}
	}
	return false
}
func (g Grant) Validate() error {
	for _, p := range g.Permissions {
		if (p.Scope != "shared_portfolio" && p.Scope != "leads") || p.AccessLevel != "view" {
			return fmt.Errorf("permissão de compartilhamento inválida")
		}
	}
	for _, v := range g.Sharing.SharedPropertyTypes {
		if v != "house" && v != "apartment" && v != "land" && v != "commercial" {
			return fmt.Errorf("tipo de imóvel inválido")
		}
	}
	for _, v := range g.Sharing.SharedRegions {
		if strings.TrimSpace(v) == "" || strings.ContainsAny(v, "%_") {
			return fmt.Errorf("informe uma região sem curingas")
		}
	}
	return nil
}

// Legacy agreements remain readable using the existing accepted settings. The
// first mutation freezes both directions before changing only the actor's grant.
// Legacy owner-contact consent never implies consent to share lead contacts.
func Resolve(grants map[string]Grant, owner string, sharing Sharing, permissions []Permission) Grant {
	if grants != nil {
		return grants[owner]
	}
	sharing.ShareLeadContact = false
	return Grant{Sharing: sharing, Permissions: permissions}
}

type Property struct {
	Shared      bool   `json:"partnership_portfolio"`
	Exclusivity string `json:"capture_exclusivity"`
	Type        string `json:"property_type"`
	Address     struct {
		City         string `json:"city"`
		Neighborhood string `json:"neighborhood"`
		State        string `json:"state"`
	} `json:"address"`
}

func (g Grant) IncludesProperty(p Property) bool {
	if !g.Allows("shared_portfolio") || !p.Shared || p.Exclusivity == "exclusive" {
		return false
	}
	if len(g.Sharing.SharedPropertyTypes) > 0 {
		found := false
		for _, v := range g.Sharing.SharedPropertyTypes {
			if v == p.Type {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if len(g.Sharing.SharedRegions) > 0 {
		found := false
		for _, v := range g.Sharing.SharedRegions {
			v = strings.ToLower(strings.TrimSpace(v))
			for _, s := range []string{p.Address.City, p.Address.Neighborhood, p.Address.State} {
				if strings.Contains(strings.ToLower(s), v) {
					found = true
				}
			}
		}
		if !found {
			return false
		}
	}
	return true
}

type Snapshot struct {
	ID              string          `json:"id"`
	Version         int             `json:"version"`
	RequesterID     string          `json:"requester_id"`
	TargetID        string          `json:"target_id"`
	Type            string          `json:"type"`
	FinancialConfig json.RawMessage `json:"financial_config"`
}

// FeePayer names an explicit, bilateral rule for fixed fees charged once per
// deal at signing, separately from the commission split.
const FeeClosingPaysSource = "closing_pays_source"
const FeeSourcePaysClosing = "source_pays_closing"

func ValidateFees(adminFee, transactionFee float64, payer string) error {
	if math.IsNaN(adminFee) || math.IsInf(adminFee, 0) || math.IsNaN(transactionFee) || math.IsInf(transactionFee, 0) || adminFee < 0 || transactionFee < 0 || adminFee > 1e12 || transactionFee > 1e12 {
		return fmt.Errorf("taxas de parceria inválidas")
	}
	if (adminFee > 0 && math.Round(adminFee*100) == 0) || (transactionFee > 0 && math.Round(transactionFee*100) == 0) {
		return fmt.Errorf("taxas devem ser de pelo menos um centavo")
	}
	if payer != "" && payer != FeeClosingPaysSource && payer != FeeSourcePaysClosing {
		return fmt.Errorf("responsável pelas taxas inválido")
	}
	if (adminFee > 0 || transactionFee > 0) && payer == "" {
		return fmt.Errorf("defina quem paga as taxas fixas de parceria antes de aceitar o acordo")
	}
	return nil
}
