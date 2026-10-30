package services

import "testing"

func TestIsChecklistGateEnabled_OffByDefault(t *testing.T) {
	ResetChecklistGateConfigForTest()
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_ALL", "")
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_TENANTS", "")
	if IsChecklistGateEnabled("tenant-1") {
		t.Errorf("expected gate off by default")
	}
}

func TestIsChecklistGateEnabled_GlobalOn(t *testing.T) {
	ResetChecklistGateConfigForTest()
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_ALL", "true")
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_TENANTS", "")
	if !IsChecklistGateEnabled("tenant-1") {
		t.Errorf("expected gate ON with TOCA_REQUIRE_CHECKLIST_GATE_ALL=true")
	}
}

func TestIsChecklistGateEnabled_TenantListWinsOverGlobal(t *testing.T) {
	ResetChecklistGateConfigForTest()
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_ALL", "true") // intentionally set
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_TENANTS", "tenant-2")
	if IsChecklistGateEnabled("tenant-1") {
		t.Errorf("tenant-1 must NOT be gated — the tenant list overrides the global flag")
	}
	if !IsChecklistGateEnabled("tenant-2") {
		t.Errorf("tenant-2 should be gated — it's in the explicit list")
	}
}

func TestIsChecklistGateEnabled_CaseInsensitiveAndTrimmed(t *testing.T) {
	ResetChecklistGateConfigForTest()
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_ALL", "")
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_TENANTS", "  TENANT-A , tenant-b ")
	if !IsChecklistGateEnabled("tenant-a") {
		t.Errorf("expected case-insensitive match for tenant-a")
	}
	if !IsChecklistGateEnabled("TENANT-B") {
		t.Errorf("expected case-insensitive match for TENANT-B")
	}
	if IsChecklistGateEnabled("tenant-c") {
		t.Errorf("tenant-c should not be gated")
	}
}

func TestIsChecklistGateEnabled_EmptyTenantNeverEnabled(t *testing.T) {
	ResetChecklistGateConfigForTest()
	t.Setenv("TOCA_REQUIRE_CHECKLIST_GATE_ALL", "true")
	if IsChecklistGateEnabled("") {
		t.Errorf("empty tenant must never be treated as gated")
	}
	if IsChecklistGateEnabled("   ") {
		t.Errorf("whitespace tenant must never be treated as gated")
	}
}
