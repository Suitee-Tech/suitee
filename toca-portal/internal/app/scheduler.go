package app

import (
	"context"
	"log"
	"time"

	"github.com/oginoino/toca/toca-portal/internal/domain/integration"
	service "github.com/oginoino/toca/toca-portal/internal/services"
)

// Scheduler handles periodic sync operations for active integrations
type Scheduler struct {
	integRepo   integration.Repository
	syncService *service.SyncService
	stopCh      chan struct{}
	interval    time.Duration
}

func NewScheduler(integRepo integration.Repository, syncService *service.SyncService) *Scheduler {
	return &Scheduler{
		integRepo:   integRepo,
		syncService: syncService,
		stopCh:      make(chan struct{}),
		interval:    5 * time.Minute,
	}
}

// Start begins the scheduler loop
func (s *Scheduler) Start() {
	go s.run()
	log.Printf("[Scheduler] Started with interval %v", s.interval)
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop() {
	close(s.stopCh)
	log.Println("[Scheduler] Stopped")
}

func (s *Scheduler) run() {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.checkAndSync()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) checkAndSync() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// Get all active integrations
	integrations, err := s.integRepo.ListActive(ctx)
	if err != nil {
		log.Printf("[Scheduler] Failed to list active integrations: %v", err)
		return
	}

	for _, integ := range integrations {
		if s.isDueForSync(integ) {
			log.Printf("[Scheduler] Triggering sync for integration %s (portal: %s, tenant: %s)",
				integ.ID, integ.Portal, integ.TenantID)

			// Execute sync in a goroutine to not block other syncs.
			// O tenant vem do próprio registro carregado por ListActive
			// (não há chamador HTTP neste caminho).
			go func(tenantID, id string) {
				syncCtx, syncCancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer syncCancel()

				_, err := s.syncService.TriggerSync(syncCtx, tenantID, id)
				if err != nil {
					log.Printf("[Scheduler] Sync failed for integration %s: %v", id, err)
				}
			}(integ.TenantID, integ.ID)
		}
	}
}

func (s *Scheduler) isDueForSync(integ *integration.PortalIntegration) bool {
	if integ.Status != integration.IntegrationActive {
		return false
	}

	// Never synced before
	if integ.LastSyncAt == nil {
		return true
	}

	// Parse sync frequency
	frequency, err := time.ParseDuration(integ.SyncFrequency)
	if err != nil {
		log.Printf("[Scheduler] Invalid sync frequency '%s' for integration %s", integ.SyncFrequency, integ.ID)
		return false
	}

	return time.Since(*integ.LastSyncAt) >= frequency
}
