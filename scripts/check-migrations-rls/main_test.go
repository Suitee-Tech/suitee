package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeMigration(t *testing.T, dir, name, sql string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(sql), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}
	return path
}

func tenantTable(t *testing.T, tables []table, fullName string) table {
	t.Helper()
	for _, candidate := range tables {
		if candidate.FullName() == fullName {
			return candidate
		}
	}
	t.Fatalf("table %s not found", fullName)
	return table{}
}

func dynamicRLSBlock(guard string) string {
	return `DO $$
BEGIN
  IF ` + guard + ` THEN
    RETURN;
  END IF;
  PERFORM 1 FROM pg_namespace WHERE nspname IN ('core') AND 'tenant_id' = 'tenant_id';
  EXECUTE 'ALTER TABLE core.example ENABLE ROW LEVEL SECURITY';
END $$;`
}

func TestDynamicCoverageDoesNotFlowForward(t *testing.T) {
	dir := t.TempDir()
	early := writeMigration(t, dir, "20260101000000_dynamic.sql",
		dynamicRLSBlock("to_regprocedure('auth.jwt()') IS NULL"))
	later := writeMigration(t, dir, "20260201000000_table.sql",
		`CREATE TABLE core.future (tenant_id uuid NOT NULL);`)
	files := []string{early, later}

	tables, err := extractTenantTables(files)
	if err != nil {
		t.Fatal(err)
	}
	dynamic, err := dynamicallyCoveredSchemas(files)
	if err != nil {
		t.Fatal(err)
	}
	if coveredAfter(tenantTable(t, tables, "core.future"), dynamic["core"]) {
		t.Fatal("an earlier dynamic block must not cover a future table")
	}
}

func TestLaterDynamicCoverageProtectsExistingTable(t *testing.T) {
	dir := t.TempDir()
	early := writeMigration(t, dir, "20260101000000_table.sql",
		`CREATE TABLE core.existing (tenant_id uuid NOT NULL);`)
	later := writeMigration(t, dir, "20260201000000_dynamic.sql",
		dynamicRLSBlock("to_regprocedure('auth.jwt()') IS NULL"))
	files := []string{early, later}

	tables, err := extractTenantTables(files)
	if err != nil {
		t.Fatal(err)
	}
	dynamic, err := dynamicallyCoveredSchemas(files)
	if err != nil {
		t.Fatal(err)
	}
	if !coveredAfter(tenantTable(t, tables, "core.existing"), dynamic["core"]) {
		t.Fatal("a later valid dynamic block should cover an existing table")
	}
}

func TestBadAuthJWTGuardIsNotCoverage(t *testing.T) {
	dir := t.TempDir()
	file := writeMigration(t, dir, "20260101000000_bad_guard.sql", `
CREATE TABLE core.example (tenant_id uuid NOT NULL);
`+dynamicRLSBlock("to_regproc('auth.jwt()') IS NULL"))

	tables, err := extractTenantTables([]string{file})
	if err != nil {
		t.Fatal(err)
	}
	dynamic, err := dynamicallyCoveredSchemas([]string{file})
	if err != nil {
		t.Fatal(err)
	}
	if coveredAfter(tenantTable(t, tables, "core.example"), dynamic["core"]) {
		t.Fatal("the known-bad to_regproc signature guard must not count as coverage")
	}
}

func TestDroppedTableDoesNotRequireFinalRLS(t *testing.T) {
	dir := t.TempDir()
	created := writeMigration(t, dir, "20260101000000_create.sql",
		`CREATE TABLE financial.retired (tenant_id uuid NOT NULL);`)
	dropped := writeMigration(t, dir, "20260201000000_drop.sql",
		`DROP TABLE IF EXISTS financial.retired;`)
	files := []string{created, dropped}

	tables, err := extractTenantTables(files)
	if err != nil {
		t.Fatal(err)
	}
	drops, err := indexMatches(files, dropTableRe)
	if err != nil {
		t.Fatal(err)
	}
	active, retired := resolveTenantTableLifecycles(tables, drops)
	if len(active) != 0 || len(retired) != 1 || retired[0].FullName() != "financial.retired" {
		t.Fatalf("expected one dropped table and no active table, got active=%v dropped=%v", active, retired)
	}
}

func TestDropThenRecreateStartsFreshRLSLifecycle(t *testing.T) {
	dir := t.TempDir()
	initial := writeMigration(t, dir, "20260101000000_create.sql", `
CREATE TABLE core.recreated (tenant_id uuid NOT NULL);
ALTER TABLE core.recreated ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_select ON core.recreated USING (true);`)
	drop := writeMigration(t, dir, "20260201000000_drop.sql",
		`DROP TABLE core.recreated;`)
	recreate := writeMigration(t, dir, "20260301000000_recreate.sql",
		`CREATE TABLE core.recreated (tenant_id uuid NOT NULL);`)
	files := []string{initial, drop, recreate}

	declarations, err := extractTenantTables(files)
	if err != nil {
		t.Fatal(err)
	}
	drops, err := indexMatches(files, dropTableRe)
	if err != nil {
		t.Fatal(err)
	}
	active, retired := resolveTenantTableLifecycles(declarations, drops)
	if len(retired) != 0 || len(active) != 1 {
		t.Fatalf("expected one active recreated table, got active=%v dropped=%v", active, retired)
	}
	if active[0].File != recreate {
		t.Fatalf("expected lifecycle to start at %s, got %s", recreate, active[0].File)
	}

	rls, err := indexMatches(files, enableRLSRe)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := indexMatches(files, createPolicyRe)
	if err != nil {
		t.Fatal(err)
	}
	if coveredAfter(active[0], rls["core.recreated"]) ||
		coveredAfter(active[0], policies["core.recreated"]) {
		t.Fatal("RLS and policies from before DROP must not cover a recreated table")
	}
}

func TestRepeatedCreateWithoutDropKeepsOriginalLifecycle(t *testing.T) {
	dir := t.TempDir()
	initial := writeMigration(t, dir, "20260101000000_create.sql", `
CREATE TABLE core.stable (tenant_id uuid NOT NULL);
ALTER TABLE core.stable ENABLE ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation_select ON core.stable USING (true);`)
	repeated := writeMigration(t, dir, "20260201000000_repeat.sql",
		`CREATE TABLE IF NOT EXISTS core.stable (tenant_id uuid NOT NULL);`)
	files := []string{initial, repeated}

	declarations, err := extractTenantTables(files)
	if err != nil {
		t.Fatal(err)
	}
	active, retired := resolveTenantTableLifecycles(declarations, nil)
	if len(retired) != 0 || len(active) != 1 || active[0].File != initial {
		t.Fatalf("repeated CREATE IF NOT EXISTS must keep the original lifecycle: active=%v dropped=%v", active, retired)
	}

	rls, err := indexMatches(files, enableRLSRe)
	if err != nil {
		t.Fatal(err)
	}
	policies, err := indexMatches(files, createPolicyRe)
	if err != nil {
		t.Fatal(err)
	}
	if !coveredAfter(active[0], rls["core.stable"]) ||
		!coveredAfter(active[0], policies["core.stable"]) {
		t.Fatal("existing RLS coverage must survive a repeated CREATE IF NOT EXISTS")
	}
}

func TestDropInCommentsAndStringsIsIgnored(t *testing.T) {
	dir := t.TempDir()
	file := writeMigration(t, dir, "20260101000000_noise.sql", `
-- DROP TABLE core.line_comment;
/* DROP TABLE core.block_comment;
   /* DROP TABLE core.nested_comment; */
*/
SELECT 'DROP TABLE core.string_literal;';
SELECT E'DROP TABLE core.escape\'d_literal;';
SELECT $$DROP TABLE core.dollar_literal;$$;
SELECT $body$DROP TABLE core.tagged_dollar_literal;$body$;
`)

	drops, err := indexMatches([]string{file}, dropTableRe)
	if err != nil {
		t.Fatal(err)
	}
	if len(drops) != 0 {
		t.Fatalf("comments and strings must not create DROP lifecycle events: %v", drops)
	}
}
