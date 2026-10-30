package sync

import (
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
)

type SyncStatus string

const (
	SyncPending    SyncStatus = "pending"
	SyncInProgress SyncStatus = "in_progress"
	SyncCompleted  SyncStatus = "completed"
	SyncFailed     SyncStatus = "failed"
)

type PortalSync struct {
	ID               string                 `json:"id" bson:"_id"`
	IntegrationID    string                 `json:"integration_id" bson:"integration_id"`
	TenantID         string                 `json:"tenant_id" bson:"tenant_id"`
	Portal           integration.PortalType `json:"portal" bson:"portal"`
	Status           SyncStatus             `json:"status" bson:"status"`
	StartedAt        time.Time              `json:"started_at" bson:"started_at"`
	CompletedAt      *time.Time             `json:"completed_at,omitempty" bson:"completed_at,omitempty"`
	PropertiesSynced int                    `json:"properties_synced" bson:"properties_synced"`
	PropertiesFailed int                    `json:"properties_failed" bson:"properties_failed"`
	ErrorDetails     []string               `json:"error_details,omitempty" bson:"error_details,omitempty"`
	FeedSizeBytes    int64                  `json:"feed_size_bytes" bson:"feed_size_bytes"`
}

func NewPortalSync(integrationID, tenantID string, portal integration.PortalType) *PortalSync {
	return &PortalSync{
		ID:            uuid.New().String(),
		IntegrationID: integrationID,
		TenantID:      tenantID,
		Portal:        portal,
		Status:        SyncPending,
		StartedAt:     time.Now(),
	}
}

func (s *PortalSync) MarkInProgress() {
	s.Status = SyncInProgress
}

func (s *PortalSync) MarkCompleted(propertiesSynced, propertiesFailed int, feedSizeBytes int64) {
	now := time.Now()
	s.Status = SyncCompleted
	s.CompletedAt = &now
	s.PropertiesSynced = propertiesSynced
	s.PropertiesFailed = propertiesFailed
	s.FeedSizeBytes = feedSizeBytes
}

func (s *PortalSync) MarkFailed(errorDetails []string) {
	now := time.Now()
	s.Status = SyncFailed
	s.CompletedAt = &now
	s.ErrorDetails = errorDetails
}
