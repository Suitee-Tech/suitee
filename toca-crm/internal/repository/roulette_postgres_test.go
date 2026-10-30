//go:build integration

package repository

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
)

func TestSetIndexCAS_Concurrent(t *testing.T) {
	uri := os.Getenv("DATABASE_URI_TEST")
	if uri == "" {
		uri = "postgres://postgres:postgres@localhost:5432/toca_test?sslmode=disable"
	}

	cfg := config.DatabaseConfig{
		URI: uri,
	}

	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, cfg)
	if err != nil {
		t.Skipf("skipping integration test: failed to connect to postgres: %v", err)
	}
	defer db.Close()

	repo := NewRoulettePostgresRepository(db)

	tenantID := uuid.NewString()

	config := &lead.RouletteConfig{
		TenantID:      tenantID,
		Enabled:       true,
		ActiveBrokers: []string{"broker1", "broker2", "broker3"},
		LastIndex:     -1,
	}

	err = repo.UpdateConfig(ctx, config)
	if err != nil {
		t.Fatalf("Failed to init config: %v", err)
	}

	var wg sync.WaitGroup
	workers := 50
	successCount := 0
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each worker tries to advance the index from -1 to 0
			ok, err := repo.SetIndexCAS(ctx, tenantID, "", -1, 0)
			if err != nil {
				t.Errorf("SetIndexCAS error: %v", err)
				return
			}
			if ok {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	if successCount != 1 {
		t.Errorf("Expected exactly 1 successful CAS, got %d", successCount)
	}
}
