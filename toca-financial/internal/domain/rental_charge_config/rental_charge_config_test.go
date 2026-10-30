package rental_charge_config

import (
	"encoding/json"
	"testing"
	"time"
)

func TestRentalChargeConfigJSONUsesSnakeCase(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	cfg := RentalChargeConfig{
		ID:                "config-1",
		TenantID:          "tenant-1",
		PropertyID:        "property-1",
		ContractID:        "contract-1",
		TenantPartyID:     "party-1",
		OwnerID:           "owner-1",
		StartDate:         now,
		EndDate:           now.AddDate(1, 0, 0),
		FirstChargeDate:   now,
		FirstChargeType:   SameMonthProRata,
		ProRataPolicy:     DefaultProRataPolicy,
		DefaultDueDay:     10,
		MonthlyAmount:     250000,
		BaseMonthlyAmount: 225000,
		Adjustments: []RentAdjustment{{
			EventID: "adjustment-1", EffectiveFrom: now, MonthlyAmount: 250000, OccurredAt: now,
		}},
		FirstChargePaid:        true,
		MonthlyProductRevision: 2,
		BundledItems: []BundledItem{{
			Key: "condominio", Description: "Condomínio", AmountCents: 50000, Enabled: true,
		}},
		CreatedAt: now,
		UpdatedAt: now,
	}

	payload, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal config JSON: %v", err)
	}

	want := map[string]bool{
		"id": true, "tenant_id": true, "property_id": true, "contract_id": true,
		"tenant_party_id": true, "owner_id": true, "start_date": true, "end_date": true,
		"first_charge_date": true, "first_charge_type": true, "pro_rata_policy": true,
		"default_due_day": true, "monthly_amount": true, "base_monthly_amount": true,
		"adjustments": true, "first_charge_paid": true, "monthly_product_revision": true,
		"bundled_items": true, "created_at": true, "updated_at": true,
	}
	if len(got) != len(want) {
		t.Fatalf("config JSON key count: got=%d want=%d keys=%v payload=%s", len(got), len(want), got, payload)
	}
	for key := range got {
		if !want[key] {
			t.Fatalf("unexpected config JSON key %q in %s", key, payload)
		}
	}
}

func TestRentalChargeConfigUnmarshalLegacyPascalCasePreservesFinancialData(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"ID":"config-legacy",
		"TenantID":"tenant-legacy",
		"PropertyID":"property-legacy",
		"ContractID":"contract-legacy",
		"TenantPartyID":"party-legacy",
		"OwnerID":"owner-legacy",
		"StartDate":"2026-01-10T00:00:00Z",
		"EndDate":"2027-01-10T00:00:00Z",
		"FirstChargeDate":"2026-02-10T00:00:00Z",
		"FirstChargeType":"next_month_accumulated",
		"DefaultDueDay":10,
		"MonthlyAmount":250000,
		"BaseMonthlyAmount":225000,
		"Adjustments":[{
			"event_id":"adjustment-1",
			"effective_from":"2026-07-10T00:00:00Z",
			"monthly_amount":250000,
			"occurred_at":"2026-06-01T12:00:00Z"
		}],
		"FirstChargePaid":true,
		"MonthlyProductRevision":3,
		"BundledItems":[{
			"key":"condominio",
			"description":"Condominio",
			"amount_cents":50000,
			"enabled":true
		}],
		"CreatedAt":"2026-01-01T09:00:00Z",
		"UpdatedAt":"2026-06-01T12:00:00Z"
	}`)

	var got RentalChargeConfig
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal legacy rental charge config: %v", err)
	}

	if got.ID != "config-legacy" || got.TenantID != "tenant-legacy" || got.ContractID != "contract-legacy" {
		t.Fatalf("legacy identity was not preserved: %#v", got)
	}
	if got.PropertyID != "property-legacy" || got.TenantPartyID != "party-legacy" || got.OwnerID != "owner-legacy" {
		t.Fatalf("legacy parties were not preserved: %#v", got)
	}
	if got.MonthlyAmount != 250000 || got.BaseMonthlyAmount != 225000 || got.DefaultDueDay != 10 {
		t.Fatalf("legacy financial values were not preserved: %#v", got)
	}
	if !got.FirstChargePaid || got.MonthlyProductRevision != 3 || got.FirstChargeType != NextMonthAccumulated {
		t.Fatalf("legacy billing state was not preserved: %#v", got)
	}
	if got.StartDate.IsZero() || got.EndDate.IsZero() || got.FirstChargeDate.IsZero() || got.CreatedAt.IsZero() || got.UpdatedAt.IsZero() {
		t.Fatalf("legacy dates were not preserved: %#v", got)
	}
	if len(got.Adjustments) != 1 || got.Adjustments[0].EventID != "adjustment-1" {
		t.Fatalf("legacy adjustment was not preserved: %#v", got.Adjustments)
	}
	if len(got.BundledItems) != 1 || got.BundledItems[0].AmountCents != 50000 {
		t.Fatalf("legacy bundled items were not preserved: %#v", got.BundledItems)
	}
}

func TestRentalChargeConfigUnmarshalPrefersSnakeCaseOverLegacyKeys(t *testing.T) {
	t.Parallel()
	payload := []byte(`{
		"tenant_id":"tenant-current",
		"TenantID":"tenant-stale",
		"contract_id":"contract-current",
		"ContractID":"contract-stale",
		"monthly_amount":0,
		"MonthlyAmount":999999,
		"first_charge_paid":false,
		"FirstChargePaid":true
	}`)

	var got RentalChargeConfig
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal mixed rental charge config: %v", err)
	}
	if got.TenantID != "tenant-current" || got.ContractID != "contract-current" {
		t.Fatalf("legacy keys overrode current identity: %#v", got)
	}
	if got.MonthlyAmount != 0 || got.FirstChargePaid {
		t.Fatalf("legacy keys overrode explicit current zero values: %#v", got)
	}
}

func TestCalculateProRataAmount_ActualActual(t *testing.T) {
	t.Parallel()
	monthly := int64(100000)
	cases := []struct {
		name  string
		start time.Time
		want  int64
	}{
		{"30-day month mid-start", time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), 70000},
		{"28-day month day one", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 100000},
		{"28-day month mid-start", time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), 50000},
		{"29-day leap month mid-start", time.Date(2028, 2, 15, 0, 0, 0, 0, time.UTC), 51724},
		{"31-day month day one", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 100000},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateProRataAmount(tc.start, monthly, ProRataPolicyActualActual)
			if got != tc.want {
				t.Fatalf("CalculateProRataAmount(%s) = %d, want %d", tc.start.Format("2006-01-02"), got, tc.want)
			}
		})
	}
}

func TestCalculateProRataAmount_Legacy30_360(t *testing.T) {
	t.Parallel()
	monthly := int64(100000)
	cases := []struct {
		name  string
		start time.Time
		want  int64
	}{
		{"30-day month mid-start", time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC), 70000},
		{"28-day month day one overcharges", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 93333},
		{"31-day month day one overcharges", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC), 103333},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateProRataAmount(tc.start, monthly, ProRataPolicyLegacy30_360)
			if got != tc.want {
				t.Fatalf("CalculateProRataAmount(%s) = %d, want %d", tc.start.Format("2006-01-02"), got, tc.want)
			}
		})
	}
}

func TestCalculateProRataAmount_EmptyPolicyResolvesToLegacy(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if got := CalculateProRataAmount(start, 100000, ""); got != 103333 {
		t.Fatalf("empty policy = %d, want legacy 103333", got)
	}
}

func TestCalculateLastMonthAmount(t *testing.T) {
	t.Parallel()
	monthly := int64(100000)
	cases := []struct {
		name   string
		end    time.Time
		policy ProRataPolicy
		want   int64
	}{
		{"31-day month partial end", time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC), ProRataPolicyActualActual, 32258},
		{"28-day month partial end", time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), ProRataPolicyActualActual, 53571},
		{"28-day month last day is full", time.Date(2026, 2, 28, 0, 0, 0, 0, time.UTC), ProRataPolicyActualActual, 100000},
		{"29-day leap month last day is full", time.Date(2028, 2, 29, 0, 0, 0, 0, time.UTC), ProRataPolicyActualActual, 100000},
		{"legacy partial end uses 30 denominator", time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC), ProRataPolicyLegacy30_360, 50000},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := CalculateLastMonthAmount(tc.end, monthly, tc.policy)
			if got != tc.want {
				t.Fatalf("CalculateLastMonthAmount(%s) = %d, want %d", tc.end.Format("2006-01-02"), got, tc.want)
			}
		})
	}
}

func TestCalculateFirstChargeAmount_SameMonth(t *testing.T) {
	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	monthly := int64(100000)
	got, err := CalculateFirstChargeAmount(start, end, monthly, SameMonthProRata, ProRataPolicyActualActual)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(70000)
	if got != want {
		t.Fatalf("expected %d, got %d", want, got)
	}
}

func TestCalculateFirstChargeAmount_Accumulated(t *testing.T) {
	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	monthly := int64(100000)
	got, err := CalculateFirstChargeAmount(start, end, monthly, NextMonthAccumulated, ProRataPolicyActualActual)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := int64(170000)
	if got != want {
		t.Fatalf("expected %d, got %d", want, got)
	}
}

func TestCalculateFirstChargeAmount_AccumulatedLastMonthIsProrated(t *testing.T) {
	start := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 10, 0, 0, 0, 0, time.UTC)
	monthly := int64(100000)
	got, err := CalculateFirstChargeAmount(start, end, monthly, NextMonthAccumulated, ProRataPolicyActualActual)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// April pro-rata (21/30 = 70000) + May pro-rata (10/31 = 32258).
	want := int64(102258)
	if got != want {
		t.Fatalf("expected %d, got %d", want, got)
	}
}

func TestCalculateProRataAmount_UsesLocalCalendarDay(t *testing.T) {
	t.Parallel()
	loc := time.FixedZone("UTC-3", -3*60*60)
	// 2026-04-09 23:00 in UTC-3 is already 2026-04-10 02:00 UTC; the local
	// calendar day must drive the proration, not the UTC instant.
	start := time.Date(2026, 4, 9, 23, 0, 0, 0, loc)
	monthly := int64(100000)
	// Local day is April 9 → 22 days (9..30 inclusive) of 30.
	want := int64(73333) // 100000 * 22 / 30
	got := CalculateProRataAmount(start, monthly, ProRataPolicyActualActual)
	if got != want {
		t.Fatalf("local calendar day = %d, want %d", got, want)
	}
}

func TestCalculateProRataAmount_FloorRounding(t *testing.T) {
	t.Parallel()
	// Feb 15 → 14 days in effect of 28. 100003 * 14 / 28 = 50001.5 → floor
	// 50001, proving deterministic integer (floor) rounding instead of
	// floating-point.
	start := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	got := CalculateProRataAmount(start, 100003, ProRataPolicyActualActual)
	if got != 50001 {
		t.Fatalf("floor rounding = %d, want 50001", got)
	}
}

func TestNew_RejectsTermLessThanOneMonth(t *testing.T) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, 1)
	end := start.AddDate(0, 0, 20)

	_, err := New("tenant-1", "prop-1", "contract-1", "party-1", "owner-1", start, end, start, SameMonthProRata, 7, 100000)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestNew_RejectsPastEndDate(t *testing.T) {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
	start := end.AddDate(0, -1, 0)

	_, err := New("tenant-1", "prop-1", "contract-1", "party-1", "owner-1", start, end, start, SameMonthProRata, 7, 100000)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestGetTermInMonths(t *testing.T) {
	start := time.Date(2026, 4, 16, 0, 0, 0, 0, time.UTC)
	end := time.Date(2028, 10, 16, 0, 0, 0, 0, time.UTC)

	cfg, err := New("tenant-1", "prop-1", "contract-1", "party-1", "owner-1", start, end, start, SameMonthProRata, 7, 100000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.GetTermInMonths() != 30 {
		t.Fatalf("expected 30, got %d", cfg.GetTermInMonths())
	}
}
