package repositories

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readCommissionDeductionMigration(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "supabase", "migrations", "20260810125000_commission_deduction_ids.sql")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read commission deduction migration: %v", err)
	}
	return string(payload)
}

func TestCommissionDeductionMigrationUsesDeterministicRuntimeIdentity(t *testing.T) {
	t.Parallel()
	migration := readCommissionDeductionMigration(t)

	for _, fragment := range []string{
		"d7b8e6e67e165d3b9c5b1f87a5e8cd6e",
		"'commission_deduction'",
		"NULLIF(BTRIM(tenant_id::text), '')",
		"NULLIF(BTRIM(id::text), '')",
		"NULLIF(kind, '')",
		"NULLIF(description, '')",
		"NULLIF(amount_text, '')",
		"digest(",
		"'sha1'",
		"(get_byte(sha1_bytes, 6) & 15) | 80",
		"(get_byte(sha1_bytes, 8) & 63) | 128",
	} {
		if !strings.Contains(migration, fragment) {
			t.Fatalf("migration is missing deterministic identity fragment %q", fragment)
		}
	}

	if strings.Contains(strings.ToLower(migration), "gen_random_uuid") {
		t.Fatal("legacy deductions must not receive random IDs: retries use the runtime deterministic identity")
	}
}
