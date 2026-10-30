package services

import (
	"os"
	"strings"
	"sync"
)

// Checklist gate (Phase 4/W3) — when enabled for a tenant, the deal
// cannot advance to CONTRACT (or CONVERTED) until every required
// checklist item is APPROVED. Off by default to preserve the historical
// "documents are advisory, not blocking" behaviour from the original
// roadmap.
//
// Configuration:
//
//   • TOCA_REQUIRE_CHECKLIST_GATE_ALL=true       → enable for every tenant.
//   • TOCA_REQUIRE_CHECKLIST_GATE_TENANTS=<csv>  → enable only for the
//     listed tenant UUIDs (case-insensitive, whitespace-trimmed). Wins
//     over the global flag when set — operators can keep the gate
//     global-off while piloting one tenant.
//
// Read once at process start so the per-request hot path is a plain
// map lookup. Restart the service to change the config (the tenant
// set rarely changes; cron-based hot reload is overkill).
//
// Decoupled from the rest of deal_service.go so the policy can move to
// a database-backed `tenant_settings` table later without touching
// every call site.

var (
	checklistGateConfigOnce sync.Once
	checklistGateAll        bool
	checklistGateTenants    map[string]bool
)

func loadChecklistGateConfig() {
	checklistGateConfigOnce.Do(func() {
		checklistGateAll = strings.EqualFold(strings.TrimSpace(os.Getenv("TOCA_REQUIRE_CHECKLIST_GATE_ALL")), "true")
		checklistGateTenants = parseTenantCSV(os.Getenv("TOCA_REQUIRE_CHECKLIST_GATE_TENANTS"))
	})
}

func parseTenantCSV(raw string) map[string]bool {
	if raw == "" {
		return nil
	}
	out := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		t := strings.ToLower(strings.TrimSpace(part))
		if t != "" {
			out[t] = true
		}
	}
	return out
}

// IsChecklistGateEnabled reports whether the deal-progress gate is
// active for the given tenant. Empty tenant is treated as "no" — we
// don't want a misconfigured caller to accidentally unlock the gate.
func IsChecklistGateEnabled(tenantID string) bool {
	if strings.TrimSpace(tenantID) == "" {
		return false
	}
	loadChecklistGateConfig()
	if len(checklistGateTenants) > 0 {
		return checklistGateTenants[strings.ToLower(tenantID)]
	}
	return checklistGateAll
}

// ResetChecklistGateConfigForTest exposes the read-once gate config to
// the test suite so each test can override the env vars without
// process restart. Calling code outside `_test.go` should never invoke
// this — flipping the gate at runtime in production would yield
// hard-to-trace status transitions.
func ResetChecklistGateConfigForTest() {
	checklistGateConfigOnce = sync.Once{}
	checklistGateAll = false
	checklistGateTenants = nil
}
