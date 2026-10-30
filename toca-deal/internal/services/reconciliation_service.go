package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/oginoino/toca/pkg/auth"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/metrics"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

// ReconciliationService runs proactive sweeps over toca-deal aggregates that
// could have drifted from their canonical source (typically signature
// envelopes) but were never re-touched by an event or status transition.
//
// Today the only sweep is signer-projection reconciliation — see
// docs/architecture/data-consistency-audit.md §8.10. The structure is meant
// to grow: as more projections are identified (broker labels, property
// snapshots), new methods land here and the runner in cmd/server/main.go
// schedules them on the same goroutine.
type ReconciliationService struct {
	db              *database.PostgresDB
	contractService *ContractService
}

// ReconciliationReport summarises one sweep.
type ReconciliationReport struct {
	StartedAt        time.Time
	FinishedAt       time.Time
	Scanned          int
	Reconciled       int
	Errors           int
	LastError        string
	MinAgeHours      int
	MaxAgeDays       int
	UnderlyingErrors []error
}

func NewReconciliationService(db *database.PostgresDB, cs *ContractService) *ReconciliationService {
	return &ReconciliationService{db: db, contractService: cs}
}

// RegisterDealReconcileAdminRoutes mounts POST /admin/deals/:id/reconcile-signers
// for operator-driven recovery of a single deal whose signer projection
// drifted faster than the scheduled sweep can catch. Requires
// X-Internal-Key (same gate as the DLQ replay endpoint). Expects the
// tenant id either as ?tenant_id=... query param or X-Tenant-Id header.
func RegisterDealReconcileAdminRoutes(r *gin.Engine, svc *ReconciliationService) {
	if r == nil || svc == nil {
		return
	}
	admin := r.Group("/admin")
	admin.Use(auth.RequireInternalKey())
	admin.POST("/deals/:id/reconcile-signers", func(c *gin.Context) {
		dealID := c.Param("id")
		if dealID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "deal id is required"})
			return
		}
		// A3 (reauditoria de 2026-09-20): o tenant vinha do `?tenant_id=` (ou do
		// header) sem nenhuma amarra à credencial — uma chave nomeada com
		// allowlist do tenant A reconciliava o tenant B. O resolvedor aplica a
		// allowlist da credencial; em chamada de usuário, o tenant vem do token.
		requested := c.Query("tenant_id")
		if requested == "" {
			requested = c.GetHeader("X-Tenant-Id")
		}
		tenantID, err := auth.ResolveTenantForInternalRequest(c, requested)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		if err := svc.ReconcileSingleDeal(c.Request.Context(), tenantID, dealID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "deal_id": dealID, "tenant_id": tenantID})
	})
}

// ReconcileSingleDeal forces an immediate signer-projection sync for one
// deal regardless of the age guards used by the scheduled sweep. Returned
// for operator recovery via the admin endpoint when the periodic loop
// would take too long to react — e.g. a freshly drifted deal that the
// user is actively trying to close. Idempotent.
func (s *ReconciliationService) ReconcileSingleDeal(ctx context.Context, tenantID, dealID string) error {
	if s == nil || s.contractService == nil {
		return fmt.Errorf("reconciliation service not fully wired")
	}
	if tenantID == "" || dealID == "" {
		return fmt.Errorf("tenant_id and deal_id are required")
	}
	tenantCtx := pkgctx.WithTenantID(ctx, tenantID)
	if err := s.contractService.SyncDealSignersFromActiveContract(tenantCtx, tenantID, dealID); err != nil {
		return fmt.Errorf("sync deal %s: %w", dealID, err)
	}
	metrics.AddReconciliationFixed(1)
	log.Printf("Reconciliation: manual sync completed for deal=%s tenant=%s", dealID, tenantID)
	return nil
}

// ReconcileStaleSignerProjections scans every Contract that is in SIGNED
// status, was last updated between minAgeHours and maxAgeDays ago, and
// re-applies the same auto-heal sync that runs reactively on
// MoveDealStatus. The sync is idempotent — if the projection is already
// fresh, UpdateSignerStatusByIDOrEmail is a no-op.
//
// minAgeHours exists so we don't trample on contracts whose webhooks are
// still settling (default 24h gives Clicksign plenty of retry headroom).
// maxAgeDays bounds the work — older contracts are unlikely to gain new
// signers and the sweep would otherwise grow unboundedly with the table.
//
// Uses the service pool so it spans all tenants in one pass, restoring
// tenant_id into the context for each row so the underlying repo calls
// honour RLS-aware invariants.
func (s *ReconciliationService) ReconcileStaleSignerProjections(ctx context.Context, minAgeHours, maxAgeDays int) (ReconciliationReport, error) {
	report := ReconciliationReport{
		StartedAt:   time.Now(),
		MinAgeHours: minAgeHours,
		MaxAgeDays:  maxAgeDays,
	}

	if s == nil || s.db == nil || s.contractService == nil {
		report.FinishedAt = time.Now()
		return report, fmt.Errorf("reconciliation service not fully wired")
	}

	query := `
		SELECT id, tenant_id, deal_id, metadata
		FROM deal.contracts
		WHERE deleted_at IS NULL
		  AND (metadata->>'status') = $1
		  AND (metadata->>'updated_at')::timestamptz < NOW() - ($2 * INTERVAL '1 hour')
		  AND (metadata->>'updated_at')::timestamptz > NOW() - ($3 * INTERVAL '1 day')
		ORDER BY (metadata->>'updated_at')::timestamptz ASC
		LIMIT 500
	`

	rows, err := s.db.QueryServiceRows(ctx, query, string(contract.StatusSigned), minAgeHours, maxAgeDays)
	if err != nil {
		report.FinishedAt = time.Now()
		return report, fmt.Errorf("scan stale contracts: %w", err)
	}
	defer rows.Close()

	type job struct {
		id       string
		tenantID string
		dealID   string
		metadata []byte
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.tenantID, &j.dealID, &j.metadata); err != nil {
			report.Errors++
			report.UnderlyingErrors = append(report.UnderlyingErrors, fmt.Errorf("scan row: %w", err))
			continue
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		report.UnderlyingErrors = append(report.UnderlyingErrors, fmt.Errorf("rows iter: %w", err))
	}
	report.Scanned = len(jobs)

	for _, j := range jobs {
		var c contract.Contract
		if err := json.Unmarshal(j.metadata, &c); err != nil {
			report.Errors++
			report.UnderlyingErrors = append(report.UnderlyingErrors, fmt.Errorf("decode contract %s: %w", j.id, err))
			continue
		}

		// Restore the tenant on the context so syncAllDealSignersFromContract's
		// downstream repo calls remain tenant-scoped. The DB rows just told us
		// the tenant — use it verbatim.
		tenantCtx := pkgctx.WithTenantID(ctx, j.tenantID)

		if err := s.contractService.SyncDealSignersFromActiveContract(tenantCtx, j.tenantID, c.DealID); err != nil {
			report.Errors++
			report.UnderlyingErrors = append(report.UnderlyingErrors, fmt.Errorf("sync deal %s: %w", c.DealID, err))
			report.LastError = err.Error()
			continue
		}
		report.Reconciled++
	}

	report.FinishedAt = time.Now()
	metrics.IncReconciliationRun()
	metrics.AddReconciliationFixed(uint64(report.Reconciled))
	metrics.AddReconciliationErrors(uint64(report.Errors))
	return report, nil
}

// DanglingLeadRef is a single deal whose owner_id or customer_id no longer
// resolves to an active row in crm.leads.
type DanglingLeadRef struct {
	DealID          string
	TenantID        string
	OwnerID         string
	CustomerID      string
	OwnerMissing    bool
	CustomerMissing bool
}

// ReconcileDanglingLeadReferences runs a single sweep that finds every
// non-deleted deal whose owner_id or customer_id no longer resolves to a
// non-deleted row in crm.leads. The method does NOT auto-fix the deals —
// the right action depends on context (resurrect the lead, point to a
// different lead, or archive the deal) and that's an operator decision.
// We log structured entries and increment a counter so the situation is
// visible in dashboards.
//
// See audit §8.9. Postgres can use the leads pk index to short-circuit
// the NOT EXISTS predicate even when crm.leads is large, so the sweep is
// cheap enough to run alongside the signer reconciliation cycle.
func (s *ReconciliationService) ReconcileDanglingLeadReferences(ctx context.Context) (int, []DanglingLeadRef, error) {
	if s == nil || s.db == nil {
		return 0, nil, fmt.Errorf("reconciliation service not fully wired")
	}

	const query = `
		SELECT
		    d.id,
		    d.tenant_id::text,
		    COALESCE(d.metadata->>'owner_id', '') AS owner_id,
		    COALESCE(d.metadata->>'customer_id', '') AS customer_id,
		    NOT EXISTS (
		        SELECT 1 FROM crm.leads l
		        WHERE l.id::text = d.metadata->>'owner_id' AND l.deleted_at IS NULL
		    ) AS owner_missing,
		    NOT EXISTS (
		        SELECT 1 FROM crm.leads l
		        WHERE l.id::text = d.metadata->>'customer_id' AND l.deleted_at IS NULL
		    ) AS customer_missing
		FROM deal.deals d
		WHERE d.deleted_at IS NULL
		  AND (
		    (d.metadata->>'owner_id' IS NOT NULL AND d.metadata->>'owner_id' <> '' AND NOT EXISTS (
		        SELECT 1 FROM crm.leads l
		        WHERE l.id::text = d.metadata->>'owner_id' AND l.deleted_at IS NULL
		    ))
		    OR
		    (d.metadata->>'customer_id' IS NOT NULL AND d.metadata->>'customer_id' <> '' AND NOT EXISTS (
		        SELECT 1 FROM crm.leads l
		        WHERE l.id::text = d.metadata->>'customer_id' AND l.deleted_at IS NULL
		    ))
		  )
		LIMIT 500
	`

	rows, err := s.db.QueryServiceRows(ctx, query)
	if err != nil {
		return 0, nil, fmt.Errorf("scan deals: %w", err)
	}
	defer rows.Close()

	out := make([]DanglingLeadRef, 0)
	for rows.Next() {
		var ref DanglingLeadRef
		if err := rows.Scan(&ref.DealID, &ref.TenantID, &ref.OwnerID, &ref.CustomerID, &ref.OwnerMissing, &ref.CustomerMissing); err != nil {
			log.Printf("Reconciliation/FK: scan row: %v", err)
			continue
		}
		out = append(out, ref)
		log.Printf("Reconciliation/FK: dangling lead reference detected — deal=%s tenant=%s owner_missing=%t customer_missing=%t owner_id=%s customer_id=%s",
			ref.DealID, ref.TenantID, ref.OwnerMissing, ref.CustomerMissing, ref.OwnerID, ref.CustomerID)
	}
	if err := rows.Err(); err != nil {
		return len(out), out, fmt.Errorf("rows iter: %w", err)
	}

	metrics.SetDanglingLeadRefs(int64(len(out)))
	return len(out), out, nil
}

// RunReconciliationLoop schedules the reconciliation sweeps on a goroutine.
// Returns a cancel function used by main on graceful shutdown.
//
// The default cadence is conservative (every 6h) — signer drift is rare
// after the Onda 1/2 fixes, so this is mostly belt-and-suspenders. Operators
// can override via env (RECONCILIATION_INTERVAL_MINUTES). minAgeHours and
// maxAgeDays are also configurable so the same primitive can be repurposed
// in incident response (e.g. minAgeHours=0 maxAgeDays=1 after a Clicksign
// outage).
func (s *ReconciliationService) RunReconciliationLoop(ctx context.Context, interval time.Duration, minAgeHours, maxAgeDays int) {
	if interval <= 0 {
		interval = 6 * time.Hour
	}
	go func() {
		// First tick after a short startup delay so the service is fully
		// healthy before doing background work.
		first := time.NewTimer(2 * time.Minute)
		defer first.Stop()
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		}

		report, err := s.ReconcileStaleSignerProjections(ctx, minAgeHours, maxAgeDays)
		s.logReport(report, err)
		if n, _, err := s.ReconcileDanglingLeadReferences(ctx); err != nil {
			log.Printf("Reconciliation/FK: sweep failed: %v", err)
		} else if n > 0 {
			log.Printf("Reconciliation/FK: %d dangling deal→lead references; check logs above for per-deal detail", n)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				report, err := s.ReconcileStaleSignerProjections(ctx, minAgeHours, maxAgeDays)
				s.logReport(report, err)
				if n, _, err := s.ReconcileDanglingLeadReferences(ctx); err != nil {
					log.Printf("Reconciliation/FK: sweep failed: %v", err)
				} else if n > 0 {
					log.Printf("Reconciliation/FK: %d dangling deal→lead references; check logs above for per-deal detail", n)
				}
			}
		}
	}()
}

func (s *ReconciliationService) logReport(report ReconciliationReport, err error) {
	if err != nil {
		log.Printf("Reconciliation: top-level error: %v", err)
		return
	}
	duration := report.FinishedAt.Sub(report.StartedAt)
	if report.Reconciled == 0 && report.Errors == 0 {
		log.Printf("Reconciliation: scanned=%d reconciled=0 errors=0 took=%s — projection healthy", report.Scanned, duration)
		return
	}
	log.Printf("Reconciliation: scanned=%d reconciled=%d errors=%d took=%s last_error=%q",
		report.Scanned, report.Reconciled, report.Errors, duration, report.LastError)
}
