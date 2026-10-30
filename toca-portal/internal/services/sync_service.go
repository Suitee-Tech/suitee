package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	psync "github.com/oginoino/toca/toca-portal/internal/domain/sync"
)

type SyncService struct {
	syncRepo    psync.Repository
	integRepo   integration.Repository
	feedService *FeedService
	eventBus    events.EventBus
}

func NewSyncService(syncRepo psync.Repository, integRepo integration.Repository, feedService *FeedService, eventBus events.EventBus) *SyncService {
	return &SyncService{
		syncRepo:    syncRepo,
		integRepo:   integRepo,
		feedService: feedService,
		eventBus:    eventBus,
	}
}

// TriggerSync manually triggers a sync for an integration of the caller's
// tenant. Cross-tenant ids are not found (404), never synced.
func (s *SyncService) TriggerSync(ctx context.Context, tenantID, integrationID string) (*psync.PortalSync, error) {
	integ, err := s.integRepo.GetByID(ctx, tenantID, integrationID)
	if err != nil {
		return nil, err
	}

	if integ.Status != integration.IntegrationActive {
		return nil, fmt.Errorf("integration is not active")
	}

	return s.executeSync(ctx, integ)
}

// executeSync performs the actual sync operation
func (s *SyncService) executeSync(ctx context.Context, integ *integration.PortalIntegration) (*psync.PortalSync, error) {
	// Create sync record
	syncRecord := psync.NewPortalSync(integ.ID, integ.TenantID, integ.Portal)
	if err := s.syncRepo.Create(ctx, syncRecord); err != nil {
		return nil, err
	}

	syncRecord.MarkInProgress()
	_ = s.syncRepo.Update(ctx, syncRecord)

	// Generate and cache the feed
	successCount, failCount, sizeBytes, err := s.feedService.GenerateAndCacheFeed(ctx, integ)
	if err != nil {
		syncRecord.MarkFailed([]string{err.Error()})
		_ = s.syncRepo.Update(ctx, syncRecord)

		// Update integration status
		integ.ConsecutiveFails++
		if integ.ConsecutiveFails >= 3 {
			integ.Status = integration.IntegrationError
			integ.ErrorMessage = fmt.Sprintf("3 consecutive sync failures: %s", err.Error())
		}
		integ.LastSyncStatus = "failed"
		_ = s.integRepo.Update(ctx, integ.TenantID, integ)

		// Publish sync failed event
		if s.eventBus != nil {
			evt := PortalSyncFailedEvent{
				SyncID:        syncRecord.ID,
				IntegrationID: integ.ID,
				TenantID:      integ.TenantID,
				Portal:        string(integ.Portal),
				Error:         err.Error(),
				Timestamp:     time.Now(),
			}
			if perr := s.eventBus.PublishReliable(ctx, evt); perr != nil {
				log.Printf("WARN: failed to enqueue portal.sync_failed event for sync=%s: %v", syncRecord.ID, perr)
			}
		}

		return syncRecord, err
	}

	// Mark sync as completed
	syncRecord.MarkCompleted(successCount, failCount, sizeBytes)
	_ = s.syncRepo.Update(ctx, syncRecord)

	// Update integration
	now := time.Now()
	integ.LastSyncAt = &now
	integ.LastSyncStatus = "completed"
	integ.ConsecutiveFails = 0
	integ.ErrorMessage = ""
	_ = s.integRepo.Update(ctx, integ.TenantID, integ)

	// Publish sync completed event
	if s.eventBus != nil {
		evt := PortalSyncCompletedEvent{
			SyncID:           syncRecord.ID,
			IntegrationID:    integ.ID,
			TenantID:         integ.TenantID,
			Portal:           string(integ.Portal),
			PropertiesSynced: successCount,
			Timestamp:        time.Now(),
		}
		if perr := s.eventBus.PublishReliable(ctx, evt); perr != nil {
			log.Printf("WARN: failed to enqueue portal.sync_completed event for sync=%s: %v", syncRecord.ID, perr)
		}
	}

	log.Printf("[SyncService] Sync completed for %s (tenant: %s): %d properties synced, %d failed, %d bytes",
		integ.Portal, integ.TenantID, successCount, failCount, sizeBytes)

	return syncRecord, nil
}

// ListSyncs returns sync history for an integration, but only if the
// integration belongs to the caller's tenant (C4) — otherwise ErrNotFound.
func (s *SyncService) ListSyncs(ctx context.Context, tenantID, integrationID string, limit, offset int64) ([]*psync.PortalSync, int64, error) {
	if _, err := s.integRepo.GetByID(ctx, tenantID, integrationID); err != nil {
		return nil, 0, err
	}
	return s.syncRepo.ListByIntegrationID(ctx, tenantID, integrationID, limit, offset)
}
