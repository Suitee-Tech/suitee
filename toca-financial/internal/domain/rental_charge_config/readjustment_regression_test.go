package rental_charge_config

import (
	"testing"
	"time"
)

func TestScheduleAdjustmentResolvesEffectiveAmountWhenEventsArriveOutOfOrder(t *testing.T) {
	cfg := &RentalChargeConfig{MonthlyAmount: 100000}
	laterEffective := time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	earlierEffective := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)

	changed, err := cfg.ScheduleAdjustment(
		"readjustment-2027",
		laterEffective,
		150000,
		time.Date(2026, time.December, 10, 12, 0, 0, 0, time.UTC),
	)
	if err != nil || !changed {
		t.Fatalf("schedule later-effective event first: changed=%v err=%v", changed, err)
	}
	changed, err = cfg.ScheduleAdjustment(
		"readjustment-2026",
		earlierEffective,
		120000,
		time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC),
	)
	if err != nil || !changed {
		t.Fatalf("schedule earlier-effective event second: changed=%v err=%v", changed, err)
	}

	if cfg.BaseMonthlyAmount != 100000 {
		t.Fatalf("expected immutable base of 100000 cents, got %d", cfg.BaseMonthlyAmount)
	}
	if len(cfg.Adjustments) != 2 {
		t.Fatalf("expected two scheduled adjustments, got %d", len(cfg.Adjustments))
	}
	if cfg.Adjustments[0].EventID != "readjustment-2026" || cfg.Adjustments[1].EventID != "readjustment-2027" {
		t.Fatalf("expected effective-date ordering, got %+v", cfg.Adjustments)
	}
	if cfg.MonthlyAmount != 150000 {
		t.Fatalf("backward-compatible current amount must remain latest-effective value, got %d", cfg.MonthlyAmount)
	}

	tests := []struct {
		name string
		asOf time.Time
		want int64
	}{
		{name: "before any adjustment", asOf: earlierEffective.Add(-time.Nanosecond), want: 100000},
		{name: "first effective instant", asOf: earlierEffective, want: 120000},
		{name: "between revisions", asOf: time.Date(2026, time.December, 31, 23, 59, 0, 0, time.UTC), want: 120000},
		{name: "latest effective instant", asOf: laterEffective, want: 150000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cfg.MonthlyAmountFor(tt.asOf); got != tt.want {
				t.Fatalf("MonthlyAmountFor(%s): want %d, got %d", tt.asOf, tt.want, got)
			}
		})
	}
}

func TestScheduleAdjustmentKeepsNewestCorrectionForSameEffectiveDay(t *testing.T) {
	cfg := &RentalChargeConfig{MonthlyAmount: 100000}
	effective := time.Date(2026, time.July, 1, 0, 0, 0, 0, time.UTC)
	firstOccurred := time.Date(2026, time.June, 10, 12, 0, 0, 0, time.UTC)

	if changed, err := cfg.ScheduleAdjustment("original", effective, 120000, firstOccurred); err != nil || !changed {
		t.Fatalf("schedule original: changed=%v err=%v", changed, err)
	}
	if changed, err := cfg.ScheduleAdjustment("correction", effective.Add(8*time.Hour), 125000, firstOccurred.Add(2*time.Hour)); err != nil || !changed {
		t.Fatalf("schedule newer correction: changed=%v err=%v", changed, err)
	}
	revisionAfterCorrection := cfg.MonthlyProductRevision

	if changed, err := cfg.ScheduleAdjustment("late-old-event", effective, 115000, firstOccurred.Add(time.Hour)); err != nil || changed {
		t.Fatalf("older late delivery must be ignored: changed=%v err=%v", changed, err)
	}
	if changed, err := cfg.ScheduleAdjustment("correction", effective.Add(8*time.Hour), 125000, firstOccurred.Add(2*time.Hour)); err != nil || changed {
		t.Fatalf("exact correction retry must be idempotent: changed=%v err=%v", changed, err)
	}
	if _, err := cfg.ScheduleAdjustment("correction", effective, 130000, firstOccurred.Add(3*time.Hour)); err == nil {
		t.Fatal("reusing an active event id with different data must fail")
	}

	if len(cfg.Adjustments) != 1 || cfg.Adjustments[0].EventID != "correction" {
		t.Fatalf("expected only newest same-day correction, got %+v", cfg.Adjustments)
	}
	if cfg.MonthlyAmountFor(effective.Add(12*time.Hour)) != 125000 {
		t.Fatalf("expected corrected amount of 125000 cents, got %d", cfg.MonthlyAmountFor(effective.Add(12*time.Hour)))
	}
	if cfg.MonthlyProductRevision != revisionAfterCorrection {
		t.Fatalf("ignored/retried events must not bump revision: before=%d after=%d", revisionAfterCorrection, cfg.MonthlyProductRevision)
	}
}
