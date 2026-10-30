// Package rental introduces a consolidated Rental aggregate that links a
// signed RENT deal+contract to its owner-lead, tenant-lead, property and
// charge configuration. Until now toca-financial only had
// `RentalChargeConfig` (the recurring billing config); answers like "all
// rentals where lead X is the tenant" or "the rental for deal Y" needed to
// traverse contract→deal→lead, which is fragile and slow.
//
// Rental is an orchestration aggregate — it doesn't replace
// RentalChargeConfig (which keeps owning the billing cadence); it sits one
// layer up and groups together the participants + contract + financial
// configuration into a single navigable entity.
package rental

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusEnded     Status = "ENDED"
	StatusRescinded Status = "RESCINDED"
)

// Rental is the consolidated view of a locação. All FK columns point at IDs
// owned by other modules (toca-deal, toca-crm, toca-core); we don't enforce
// referential integrity at the DB level because the schemas are siblings —
// validation happens at the service layer (RentalService.CreateFromDeal).
type Rental struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	DealID     string `json:"deal_id"`
	ContractID string `json:"contract_id"`
	PropertyID string `json:"property_id"`
	// TenantLeadID is the lead.id of the inquilino (lead with role='tenant').
	TenantLeadID string `json:"tenant_lead_id"`
	// OwnerLeadID is the lead.id of the proprietário (lead with role='owner').
	OwnerLeadID string `json:"owner_lead_id"`
	// BrokerID is the primary broker that closed the deal — kept for
	// reporting/cross-link in the admin UI; commission split with captador
	// etc. is recorded in the original deal's BrokerParticipations.
	BrokerID  string    `json:"broker_id,omitempty"`
	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`
	// MonthlyValueCents is the agreed rent in cents — same denomination as
	// charges, avoids float drift over years of recurring billing.
	MonthlyValueCents int64     `json:"monthly_value_cents"`
	Status            Status    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func New(tenantID, dealID, contractID, propertyID, tenantLeadID, ownerLeadID, brokerID string, startDate, endDate time.Time, monthlyValueCents int64) (*Rental, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required")
	}
	if strings.TrimSpace(dealID) == "" {
		return nil, errors.New("deal_id is required")
	}
	if strings.TrimSpace(contractID) == "" {
		return nil, errors.New("contract_id is required")
	}
	if strings.TrimSpace(propertyID) == "" {
		return nil, errors.New("property_id is required")
	}
	if strings.TrimSpace(tenantLeadID) == "" {
		return nil, errors.New("tenant_lead_id is required")
	}
	if strings.TrimSpace(ownerLeadID) == "" {
		return nil, errors.New("owner_lead_id is required")
	}
	if monthlyValueCents <= 0 {
		return nil, errors.New("monthly_value_cents must be positive")
	}
	if endDate.Before(startDate) {
		return nil, errors.New("end_date must be on/after start_date")
	}

	now := time.Now()
	return &Rental{
		ID:                uuid.New().String(),
		TenantID:          tenantID,
		DealID:            dealID,
		ContractID:        contractID,
		PropertyID:        propertyID,
		TenantLeadID:      tenantLeadID,
		OwnerLeadID:       ownerLeadID,
		BrokerID:          brokerID,
		StartDate:         startDate,
		EndDate:           endDate,
		MonthlyValueCents: monthlyValueCents,
		Status:            StatusActive,
		CreatedAt:         now,
		UpdatedAt:         now,
	}, nil
}

// MarkEnded transitions the rental to ENDED at the natural end-date arrival.
// Use Rescind for early termination.
func (r *Rental) MarkEnded() {
	r.Status = StatusEnded
	r.UpdatedAt = time.Now()
}

// Rescind transitions the rental to RESCINDED — early termination. Idempotent.
func (r *Rental) Rescind() {
	r.Status = StatusRescinded
	r.UpdatedAt = time.Now()
}
