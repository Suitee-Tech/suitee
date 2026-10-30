package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/oginoino/toca/pkg/config"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
	"github.com/oginoino/toca/toca-crm/internal/service"
)

func main() {
	ctx := context.Background()

	dbURI := os.Getenv("DATABASE_URI")
	if dbURI == "" {
		dbURI = "postgresql://app_user:app_pass@localhost:5432/toca?sslmode=disable"
	}
	dbServiceURI := os.Getenv("DATABASE_URI_SERVICE")
	if dbServiceURI == "" {
		dbServiceURI = "postgresql://postgres:postgres@localhost:5432/toca?sslmode=disable"
	}

	pg, err := database.ConnectPostgres(ctx, config.DatabaseConfig{
		URI:         dbURI,
		ServiceURI:  dbServiceURI,
		Name:        "toca",
		MaxPoolSize: 5,
		MinPoolSize: 0,
	})
	if err != nil {
		panic(err)
	}
	defer pg.Close()

	rouletteRepo := repository.NewRoulettePostgresRepository(pg)
	leadRepo := repository.NewLeadPostgresRepository(pg)

	tenantIDs, err := rouletteRepo.ListEnabledTenantIDs(ctx)
	if err != nil {
		panic(err)
	}
	for _, tid := range tenantIDs {
		tenantCtx := pkgctx.WithTenantID(ctx, tid)
		var claims string
		_ = pg.QueryRowContext(tenantCtx, "SELECT current_setting('request.jwt.claims', true)").Scan(&claims)
		fmt.Printf("tenant=%s claims=%s\n", tid, claims)

		cfg, err := rouletteRepo.GetConfig(tenantCtx, tid)
		if err != nil {
			panic(err)
		}
		if cfg == nil {
			fmt.Printf("tenant=%s cfg=nil\n", tid)
		} else {
			fmt.Printf("tenant=%s cfg_enabled=%v sla_tiers=%d\n", tid, cfg.Enabled, len(cfg.SLATiers))
		}

		cutoff := time.Now().Add(-1 * time.Minute)
		leads, err := leadRepo.ListForSLARedirect(ctx, tid, []string{string(lead.LeadStatusPendingAcceptance)}, 2, cutoff, 50)
		if err != nil {
			panic(err)
		}
		fmt.Printf("tenant=%s leads_for_sla=%d\n", tid, len(leads))
	}

	brokerRepo := repository.NewBrokerPostgresRepository(pg)
	slaSvc := service.NewLeadSLAService(leadRepo, rouletteRepo, brokerRepo, nil, nil)
	if err := slaSvc.CheckSLAViolations(ctx); err != nil {
		panic(err)
	}

	type leadRow struct {
		ID       string
		TenantID string
		Status   string
		Retries  int
	}

	rows, err := pg.QueryServiceRows(ctx, `
		SELECT
			id::text,
			tenant_id::text,
			metadata->>'Status' AS status,
			COALESCE((metadata->>'RouletteRetries')::int, 0) AS retries
		FROM crm.leads
		WHERE deleted_at IS NULL
		ORDER BY tenant_id, id
	`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var r leadRow
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Status, &r.Retries); err != nil {
			panic(err)
		}
		fmt.Printf("lead id=%s tenant=%s status=%s retries=%d\n", r.ID, r.TenantID, r.Status, r.Retries)
	}
	if err := rows.Err(); err != nil {
		panic(err)
	}
}
