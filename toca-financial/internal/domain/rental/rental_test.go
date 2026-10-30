package rental

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNew_Valid(t *testing.T) {
	start := time.Now()
	end := start.AddDate(1, 0, 0)
	r, err := New("t1", "d1", "c1", "p1", "tenant-l", "owner-l", "broker-1", start, end, 250000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Status != StatusActive {
		t.Errorf("expected StatusActive, got %v", r.Status)
	}
	if r.ID == "" {
		t.Error("expected generated ID")
	}
}

func TestRentalJSONUsesSnakeCase(t *testing.T) {
	start := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	r := &Rental{
		ID:                "rental-1",
		TenantID:          "tenant-1",
		DealID:            "deal-1",
		ContractID:        "contract-1",
		PropertyID:        "property-1",
		TenantLeadID:      "tenant-lead-1",
		OwnerLeadID:       "owner-lead-1",
		BrokerID:          "broker-1",
		StartDate:         start,
		EndDate:           start.AddDate(1, 0, 0),
		MonthlyValueCents: 250000,
		Status:            StatusActive,
		CreatedAt:         start,
		UpdatedAt:         start,
	}

	payload, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal rental: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal rental JSON: %v", err)
	}

	want := map[string]bool{
		"id": true, "tenant_id": true, "deal_id": true, "contract_id": true,
		"property_id": true, "tenant_lead_id": true, "owner_lead_id": true,
		"broker_id": true, "start_date": true, "end_date": true,
		"monthly_value_cents": true, "status": true, "created_at": true, "updated_at": true,
	}
	if len(got) != len(want) {
		t.Fatalf("rental JSON key count: got=%d want=%d keys=%v payload=%s", len(got), len(want), got, payload)
	}
	for key := range got {
		if !want[key] {
			t.Fatalf("unexpected rental JSON key %q in %s", key, payload)
		}
	}
}

func TestNew_RejectsMissingFields(t *testing.T) {
	tests := []struct {
		name      string
		tenantID  string
		dealID    string
		contract  string
		property  string
		tenantL   string
		ownerL    string
		monthly   int64
		startDiff time.Duration
		endDiff   time.Duration
	}{
		{name: "missing tenant", dealID: "d", contract: "c", property: "p", tenantL: "t", ownerL: "o", monthly: 1000, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "missing deal", tenantID: "t1", contract: "c", property: "p", tenantL: "t", ownerL: "o", monthly: 1000, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "missing contract", tenantID: "t1", dealID: "d", property: "p", tenantL: "t", ownerL: "o", monthly: 1000, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "missing property", tenantID: "t1", dealID: "d", contract: "c", tenantL: "t", ownerL: "o", monthly: 1000, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "missing tenant lead", tenantID: "t1", dealID: "d", contract: "c", property: "p", ownerL: "o", monthly: 1000, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "missing owner lead", tenantID: "t1", dealID: "d", contract: "c", property: "p", tenantL: "t", monthly: 1000, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "non-positive monthly", tenantID: "t1", dealID: "d", contract: "c", property: "p", tenantL: "t", ownerL: "o", monthly: 0, startDiff: 0, endDiff: time.Hour * 24 * 365},
		{name: "end before start", tenantID: "t1", dealID: "d", contract: "c", property: "p", tenantL: "t", ownerL: "o", monthly: 1000, startDiff: time.Hour * 48, endDiff: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			start := now.Add(tt.startDiff)
			end := now.Add(tt.endDiff)
			_, err := New(tt.tenantID, tt.dealID, tt.contract, tt.property, tt.tenantL, tt.ownerL, "", start, end, tt.monthly)
			if err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestRescindAndMarkEnded(t *testing.T) {
	start := time.Now()
	end := start.AddDate(1, 0, 0)
	r, err := New("t1", "d1", "c1", "p1", "tenant-l", "owner-l", "", start, end, 250000)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}

	r.Rescind()
	if r.Status != StatusRescinded {
		t.Errorf("expected RESCINDED, got %v", r.Status)
	}

	r.MarkEnded()
	if r.Status != StatusEnded {
		t.Errorf("expected ENDED, got %v", r.Status)
	}
}
