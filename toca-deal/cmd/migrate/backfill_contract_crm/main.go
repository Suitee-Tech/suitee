// Command backfill_contract_crm repairs contracts whose denormalized CRM
// snapshot fields (client, property_id, customer_id, value, commission_pct)
// were never persisted because CreateContract used to drop them.
//
// Idempotent: pulls every contract with at least one missing field, loads its
// Deal in the right tenant scope, fills only the blanks via BackfillContract,
// and writes back only when something changed. Re-running reports "0 updated".
//
// Run: `go run ./toca-deal/cmd/migrate/backfill_contract_crm [--tenant=<id>] [--dry-run]`
package main

import (
	"context"
	"flag"
	"log"

	"github.com/oginoino/toca/pkg/config"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	repository "github.com/oginoino/toca/toca-deal/internal/repositories"
	service "github.com/oginoino/toca/toca-deal/internal/services"
)

func main() {
	tenantFilter := flag.String("tenant", "", "limit the sweep to a single tenant_id (empty = all tenants)")
	dryRun := flag.Bool("dry-run", false, "log what would change without persisting")
	flag.Parse()

	ctx := context.Background()
	cfg := config.LoadBaseConfig()

	pgDB, err := database.ConnectPostgres(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer pgDB.Close()

	contractRepo := repository.NewContractPostgresRepository(pgDB)
	dealRepo := repository.NewDealPostgresRepository(pgDB)

	contracts, err := contractRepo.ListMissingCRMSnapshot(ctx, *tenantFilter)
	if err != nil {
		log.Fatalf("list contracts missing CRM snapshot: %v", err)
	}
	log.Printf("backfill scan: %d contracts with missing CRM fields (tenant filter=%q, dry_run=%v)", len(contracts), *tenantFilter, *dryRun)

	var updated, skippedNoDeal, skippedNoChange, failed int
	for _, c := range contracts {
		if c == nil {
			continue
		}
		if c.DealID == "" {
			log.Printf("contract %s tenant=%s has no deal_id; skipping", c.ID, c.TenantID)
			skippedNoDeal++
			continue
		}
		tenantCtx := pkgctx.WithTenantID(ctx, c.TenantID)
		d, err := dealRepo.GetByID(tenantCtx, c.TenantID, c.DealID)
		if err != nil || d == nil {
			log.Printf("contract %s tenant=%s: deal %s not found (err=%v); skipping", c.ID, c.TenantID, c.DealID, err)
			skippedNoDeal++
			continue
		}
		changed := service.BackfillContract(c, d)
		if !changed {
			skippedNoChange++
			continue
		}
		if *dryRun {
			log.Printf("[dry-run] would update contract %s tenant=%s client=%q property_id=%q customer_id=%q value=%.2f", c.ID, c.TenantID, c.Client, c.PropertyID, c.CustomerID, c.Value)
			updated++
			continue
		}
		if err := contractRepo.Update(tenantCtx, c); err != nil {
			log.Printf("update contract %s tenant=%s: %v", c.ID, c.TenantID, err)
			failed++
			continue
		}
		log.Printf("updated contract %s tenant=%s client=%q property_id=%q customer_id=%q value=%.2f", c.ID, c.TenantID, c.Client, c.PropertyID, c.CustomerID, c.Value)
		updated++
	}

	log.Printf("backfill summary: updated=%d skipped_no_deal=%d skipped_no_change=%d failed=%d total_scanned=%d", updated, skippedNoDeal, skippedNoChange, failed, len(contracts))
}
