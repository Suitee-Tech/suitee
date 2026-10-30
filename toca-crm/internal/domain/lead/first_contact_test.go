package lead

import (
	"testing"
	"time"
)

func TestMarkFirstContact_SetsTimestampAndTransitionsStatus(t *testing.T) {
	cases := []struct {
		name        string
		startStatus LeadStatus
		wantStatus  LeadStatus
	}{
		{"NEW → CONTACTED", LeadStatusNew, LeadStatusContacted},
		{"PENDING_ACCEPTANCE → CONTACTED", LeadStatusPendingAcceptance, LeadStatusContacted},
		{"QUALIFIED stays QUALIFIED", LeadStatusQualified, LeadStatusQualified},
		{"VISIT_SCHEDULED stays VISIT_SCHEDULED", LeadStatusVisitScheduled, LeadStatusVisitScheduled},
		{"CONTACTED stays CONTACTED (idempotent path)", LeadStatusContacted, LeadStatusContacted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
			l := &Lead{Status: tc.startStatus}
			changed := l.MarkFirstContact(now)
			if !changed {
				t.Errorf("expected changed=true on first call")
			}
			if l.FirstContactAt == nil || !l.FirstContactAt.Equal(now) {
				t.Errorf("FirstContactAt not set correctly")
			}
			if l.Status != tc.wantStatus {
				t.Errorf("Status = %q, want %q", l.Status, tc.wantStatus)
			}
		})
	}
}

func TestMarkFirstContact_IdempotentOnSecondCall(t *testing.T) {
	first := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	later := time.Date(2026, 5, 19, 11, 30, 0, 0, time.UTC)
	l := &Lead{Status: LeadStatusNew}
	l.MarkFirstContact(first)
	changed := l.MarkFirstContact(later)
	if changed {
		t.Error("expected changed=false on second call (idempotent)")
	}
	if !l.FirstContactAt.Equal(first) {
		t.Errorf("FirstContactAt overwritten: %v want %v", l.FirstContactAt, first)
	}
}

func TestFirstContactSLABreached_Cases(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	cutoff := now.Add(-60 * time.Minute) // 1h ago

	cases := []struct {
		name string
		lead Lead
		want bool
	}{
		{
			"breach: assigned 2h ago, no first contact",
			Lead{Status: LeadStatusNew, AssignedAt: now.Add(-2 * time.Hour)},
			true,
		},
		{
			"no breach: assigned 30min ago",
			Lead{Status: LeadStatusNew, AssignedAt: now.Add(-30 * time.Minute)},
			false,
		},
		{
			"no breach: first contact already registered",
			Lead{
				Status:         LeadStatusContacted,
				AssignedAt:     now.Add(-2 * time.Hour),
				FirstContactAt: timePtr(now.Add(-90 * time.Minute)),
			},
			false,
		},
		{
			"no breach: never assigned (AssignedAt zero)",
			Lead{Status: LeadStatusNew},
			false,
		},
		{
			"no breach: terminal status CONVERTED",
			Lead{Status: LeadStatusConverted, AssignedAt: now.Add(-2 * time.Hour)},
			false,
		},
		{
			"no breach: terminal status LOST",
			Lead{Status: LeadStatusLost, AssignedAt: now.Add(-2 * time.Hour)},
			false,
		},
		{
			"edge: assigned exactly at cutoff",
			// AssignedAt == cutoff → AssignedAt.Before(cutoff) is false → não breach
			Lead{Status: LeadStatusNew, AssignedAt: cutoff},
			false,
		},
		{
			"edge: 1 second before cutoff",
			Lead{Status: LeadStatusNew, AssignedAt: cutoff.Add(-1 * time.Second)},
			true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.lead.FirstContactSLABreached(cutoff)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func timePtr(t time.Time) *time.Time {
	return &t
}
