package handlers

import (
	"testing"
	"time"
)

func TestReportRangeIncludesEntireFinalCalendarDay(t *testing.T) {
	from, to, err := parseReportRange("2026-09-01", "2026-09-09")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"2026-09-09T00:00:00Z", "2026-09-09T15:00:00Z", "2026-09-09T23:59:59.999999Z"} {
		paidAt, _ := time.Parse(time.RFC3339Nano, value)
		if paidAt.Before(from) || paidAt.After(to) {
			t.Fatalf("payment %s excluded from %s - %s", value, from, to)
		}
	}
	nextDay, _ := time.Parse(time.RFC3339, "2026-09-10T00:00:00Z")
	if !nextDay.After(to) {
		t.Fatal("next day must be excluded")
	}
}

func TestReportRangePreservesExplicitInstantAndRejectsInvertedRange(t *testing.T) {
	_, to, err := parseReportRange("2026-09-09", "2026-09-09T12:00:00-03:00")
	if err != nil {
		t.Fatal(err)
	}
	if to.UTC().Format(time.RFC3339) != "2026-09-09T15:00:00Z" {
		t.Fatalf("changed explicit time: %s", to)
	}
	if _, _, err := parseReportRange("2026-09-10", "2026-09-09"); err == nil {
		t.Fatal("expected invalid interval")
	}
	if _, _, err := parseReportRange("2026-09-09", "2026-09-09"); err != nil {
		t.Fatal(err)
	}
}
