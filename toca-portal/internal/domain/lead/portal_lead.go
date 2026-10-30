package lead

import (
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
)

type PortalLeadStatus string

const (
	PortalLeadReceived  PortalLeadStatus = "received"
	PortalLeadProcessed PortalLeadStatus = "processed"
	PortalLeadFailed    PortalLeadStatus = "failed"
	PortalLeadDuplicate PortalLeadStatus = "duplicate"
)

type PortalLead struct {
	ID              string                 `json:"id" bson:"_id"`
	TenantID        string                 `json:"tenant_id" bson:"tenant_id"`
	IntegrationID   string                 `json:"integration_id" bson:"integration_id"`
	Portal          integration.PortalType `json:"portal" bson:"portal"`
	RawPayload      string                 `json:"raw_payload" bson:"raw_payload"`
	Name            string                 `json:"name" bson:"name"`
	Email           string                 `json:"email" bson:"email"`
	Phone           string                 `json:"phone" bson:"phone"`
	Message         string                 `json:"message,omitempty" bson:"message,omitempty"`
	PropertyID      string                 `json:"property_id,omitempty" bson:"property_id,omitempty"`
	PortalListingID string                 `json:"portal_listing_id,omitempty" bson:"portal_listing_id,omitempty"`
	CRMLeadID       string                 `json:"crm_lead_id,omitempty" bson:"crm_lead_id,omitempty"`
	Status          PortalLeadStatus       `json:"status" bson:"status"`
	ReceivedAt      time.Time              `json:"received_at" bson:"received_at"`
	ProcessedAt     *time.Time             `json:"processed_at,omitempty" bson:"processed_at,omitempty"`
	ErrorMessage    string                 `json:"error_message,omitempty" bson:"error_message,omitempty"`
}

func NewPortalLead(tenantID, integrationID string, portal integration.PortalType, rawPayload, name, email, phone, message, portalListingID string) *PortalLead {
	return &PortalLead{
		ID:              uuid.New().String(),
		TenantID:        tenantID,
		IntegrationID:   integrationID,
		Portal:          portal,
		RawPayload:      rawPayload,
		Name:            name,
		Email:           email,
		Phone:           phone,
		Message:         message,
		PortalListingID: portalListingID,
		Status:          PortalLeadReceived,
		ReceivedAt:      time.Now(),
	}
}

func (pl *PortalLead) MarkProcessed(crmLeadID string) {
	now := time.Now()
	pl.Status = PortalLeadProcessed
	pl.CRMLeadID = crmLeadID
	pl.ProcessedAt = &now
}

func (pl *PortalLead) MarkFailed(errMsg string) {
	now := time.Now()
	pl.Status = PortalLeadFailed
	pl.ErrorMessage = errMsg
	pl.ProcessedAt = &now
}

func (pl *PortalLead) MarkDuplicate() {
	now := time.Now()
	pl.Status = PortalLeadDuplicate
	pl.ProcessedAt = &now
}
