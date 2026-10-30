package services

import "time"

// PortalLeadReceivedEvent is published when a lead is received from a portal webhook
type PortalLeadReceivedEvent struct {
	PortalLeadID string    `json:"portal_lead_id"`
	CRMLeadID    string    `json:"crm_lead_id"`
	TenantID     string    `json:"tenant_id"`
	Portal       string    `json:"portal"`
	LeadName     string    `json:"lead_name"`
	LeadEmail    string    `json:"lead_email"`
	LeadPhone    string    `json:"lead_phone"`
	PropertyID   string    `json:"property_id,omitempty"`
	Timestamp    time.Time `json:"timestamp"`
}

func (e PortalLeadReceivedEvent) Name() string         { return "portal.lead_received" }
func (e PortalLeadReceivedEvent) Payload() interface{} { return e }

// PortalSyncCompletedEvent is published when a sync completes successfully
type PortalSyncCompletedEvent struct {
	SyncID           string    `json:"sync_id"`
	IntegrationID    string    `json:"integration_id"`
	TenantID         string    `json:"tenant_id"`
	Portal           string    `json:"portal"`
	PropertiesSynced int       `json:"properties_synced"`
	Timestamp        time.Time `json:"timestamp"`
}

func (e PortalSyncCompletedEvent) Name() string         { return "portal.sync_completed" }
func (e PortalSyncCompletedEvent) Payload() interface{} { return e }

// PortalSyncFailedEvent is published when a sync fails
type PortalSyncFailedEvent struct {
	SyncID        string    `json:"sync_id"`
	IntegrationID string    `json:"integration_id"`
	TenantID      string    `json:"tenant_id"`
	Portal        string    `json:"portal"`
	Error         string    `json:"error"`
	Timestamp     time.Time `json:"timestamp"`
}

func (e PortalSyncFailedEvent) Name() string         { return "portal.sync_failed" }
func (e PortalSyncFailedEvent) Payload() interface{} { return e }

func (e PortalLeadReceivedEvent) EventID() string {
	return ""
}

func (e PortalSyncCompletedEvent) EventID() string {
	return ""
}

func (e PortalSyncFailedEvent) EventID() string {
	return ""
}
