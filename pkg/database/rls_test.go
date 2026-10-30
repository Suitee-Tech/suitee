//go:build integration

package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	appCtx "github.com/oginoino/toca/pkg/context"
)

// TestRLS_TenantIsolation validates that sqlx queries go through the pgxpool
// BeforeAcquire hook that sets request.jwt.claims, so Supabase RLS policies
// filter rows by tenant.
//
// Requires a Postgres reachable via DATABASE_URI_TEST (defaults to a local
// database) with the consolidated migration applied (i.e. RLS enabled on
// core.properties with tenant_isolation_* policies reading auth.jwt()). Run
// with `go test -tags=integration ./pkg/database/...`.
func TestRLS_TenantIsolation(t *testing.T) {
	uri := os.Getenv("DATABASE_URI_TEST")
	if uri == "" {
		uri = "postgres://postgres:postgres@localhost:5432/toca_test?sslmode=disable"
	}
	serviceURI := os.Getenv("DATABASE_URI_SERVICE_TEST")

	cfg := config.DatabaseConfig{URI: uri, ServiceURI: serviceURI}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := ConnectPostgres(ctx, cfg)
	if err != nil {
		t.Skipf("test database unavailable (%s): %v", uri, err)
	}
	defer db.Close()

	// Skip if RLS helper is absent (e.g. vanilla Postgres without Supabase auth schema)
	var hasJWT bool
	if err := db.QueryRowContext(ctx, "SELECT to_regprocedure('auth.jwt()') IS NOT NULL").Scan(&hasJWT); err != nil {
		t.Skipf("could not probe auth.jwt(): %v", err)
	}
	if !hasJWT {
		t.Skip("auth.jwt() not available; skipping RLS test (run against Supabase)")
	}

	var currentRole string
	var bypassRLS bool
	var ownsProperties bool
	var authenticatedMember bool
	if err := db.QueryRowContext(ctx, `
		SELECT
			current_user,
			role.rolbypassrls,
			pg_get_userbyid(table_class.relowner) = current_user,
			pg_has_role(current_user, 'authenticated', 'MEMBER')
		FROM pg_roles AS role
		JOIN pg_class AS table_class
		  ON table_class.oid = 'core.properties'::regclass
		WHERE role.rolname = current_user
	`).Scan(&currentRole, &bypassRLS, &ownsProperties, &authenticatedMember); err != nil {
		t.Fatalf("probe tenant database role: %v", err)
	}
	if bypassRLS || ownsProperties {
		t.Fatalf(
			"DATABASE_URI_TEST role %q bypasses RLS (bypass=%t owner=%t); use a dedicated non-owner, non-BYPASSRLS tenant role",
			currentRole,
			bypassRLS,
			ownsProperties,
		)
	}
	if !authenticatedMember {
		t.Fatalf(
			"DATABASE_URI_TEST role %q is not a member of authenticated; policies scoped TO authenticated will deny every row",
			currentRole,
		)
	}
	if serviceURI == "" {
		t.Fatal("DATABASE_URI_SERVICE_TEST is required to seed and clean up RLS integration fixtures")
	}

	tenantA := uuid.NewString()
	tenantB := uuid.NewString()

	// Seed two properties: one per tenant. Use the service pool so inserts
	// aren't themselves gated by RLS — the test is about *reads*.
	seed := func(tenantID, propID string) {
		t.Helper()
		meta := `{"title":"RLS test","status":"active"}`
		_, err := db.ServicePool.Exec(ctx,
			`INSERT INTO core.properties (id, tenant_id, metadata)
			 VALUES ($1, $2, $3::jsonb)
			 ON CONFLICT (id) DO NOTHING`,
			propID, tenantID, meta)
		if err != nil {
			t.Fatalf("seed failed for tenant %s: %v", tenantID, err)
		}
	}

	propA := uuid.NewString()
	propB := uuid.NewString()
	seed(tenantA, propA)
	seed(tenantB, propB)

	t.Cleanup(func() {
		_, _ = db.ServicePool.Exec(context.Background(),
			`DELETE FROM core.properties WHERE id = ANY($1)`,
			[]string{propA, propB})
	})

	countAs := func(t *testing.T, ctx context.Context, ids []string) int {
		t.Helper()
		var n int
		err := db.QueryRowContext(ctx,
			`SELECT COUNT(1) FROM core.properties WHERE id = ANY($1)`,
			ids).Scan(&n)
		if err != nil {
			t.Fatalf("count query failed: %v", err)
		}
		return n
	}

	ids := []string{propA, propB}

	// Tenant A should see only its own row.
	ctxA := appCtx.WithTenantID(ctx, tenantA)
	if got := countAs(t, ctxA, ids); got != 1 {
		t.Errorf("tenant A: expected 1 visible row, got %d", got)
	}

	// Tenant B should see only its own row.
	ctxB := appCtx.WithTenantID(ctx, tenantB)
	if got := countAs(t, ctxB, ids); got != 1 {
		t.Errorf("tenant B: expected 1 visible row, got %d", got)
	}

	// No tenant in ctx: RLS must return zero rows (claims blanked out).
	if got := countAs(t, ctx, ids); got != 0 {
		t.Errorf("no tenant: expected 0 visible rows, got %d", got)
	}
}
