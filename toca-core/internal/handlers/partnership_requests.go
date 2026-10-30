package handlers

import (
	"github.com/oginoino/toca-core/internal/domain/core/partnership"
)

type CreatePartnershipRequest struct {
	TargetID        string                      `json:"target_id" binding:"required"`
	Type            partnership.PartnershipType `json:"type" binding:"required"`
	FinancialConfig partnership.FinancialConfig `json:"financial_config"`
	SharingConfig   partnership.SharingConfig   `json:"sharing_config"`
	Permissions     []partnership.Permission    `json:"permissions"`
}

type UpdateFinancialConfigRequest struct {
	FinancialConfig partnership.FinancialConfig `json:"financial_config" binding:"required"`
}

type UpdatePermissionsRequest struct {
	Permissions []partnership.Permission `json:"permissions" binding:"required"`
}

// UpdateSharingConfigRequest carries the partnership-level shared-portfolio
// rules (owner-contact PII visibility, co-brokering, type/region whitelists).
type UpdateSharingConfigRequest struct {
	SharingConfig partnership.SharingConfig `json:"sharing_config" binding:"required"`
}

func (r *CreatePartnershipRequest) ToDomain(requesterID string) *partnership.Partnership {
	return &partnership.Partnership{
		RequesterID:     requesterID,
		TargetID:        r.TargetID,
		Type:            r.Type,
		Status:          partnership.PartnershipStatusPending,
		FinancialConfig: r.FinancialConfig,
		SharingConfig:   r.SharingConfig,
		Permissions:     r.Permissions,
	}
}
