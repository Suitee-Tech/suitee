package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/events"
)

// TestLeadEventContracts pins the routing-key (Name) and JSON payload shape
// for every event the Leads module publishes. Downstream consumers
// (toca-deal, toca-comm, integration partners) subscribe to these strings —
// renaming one silently breaks them.
//
// When you add a new field to a payload, update the test fixture; when you
// rename or remove a field, ship a versioned event instead so existing
// consumers keep working.
func TestLeadEventContracts(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name           string
		event          events.Event
		wantName       string
		mustHaveFields []string
	}{
		{
			name:           "LeadCreatedEvent",
			event:          LeadCreatedEvent{LeadID: "lead-1", TenantID: "t-1", LeadName: "Alice", Email: "a@b.com", Phone: "+5511999999999", Source: "WEBSITE", Status: "NEW", Timestamp: now},
			wantName:       "lead.created",
			mustHaveFields: []string{"lead_id", "tenant_id", "name", "email", "phone", "source", "assigned_broker_id", "status", "timestamp"},
		},
		{
			name:           "LeadStatusChangedEvent",
			event:          LeadStatusChangedEvent{LeadID: "lead-1", TenantID: "t-1", OldStatus: "NEW", NewStatus: "CONTACTED", Timestamp: now},
			wantName:       "lead.status_changed",
			mustHaveFields: []string{"lead_id", "tenant_id", "old_status", "new_status", "timestamp"},
		},
		{
			name:           "LeadEscalatedEvent",
			event:          LeadEscalatedEvent{LeadID: "lead-1", TenantID: "t-1", Action: "notify_manager", Level: 1, PreviousLevel: 0, StepLabel: "Step 1", StepID: "s1"},
			wantName:       "lead.escalated",
			mustHaveFields: []string{"lead_id", "tenant_id", "action", "level", "previous_level", "step_label"},
		},
		{
			name:           "LeadAssignedEvent",
			event:          LeadAssignedEvent{LeadID: "lead-1", LeadName: "Alice", TenantID: "t-1", BrokerID: "broker-1"},
			wantName:       "lead.assigned",
			mustHaveFields: []string{"lead_id", "lead_name", "tenant_id", "broker_id"},
		},
		{
			name:           "RouletteConfigUpdatedEvent",
			event:          RouletteConfigUpdatedEvent{TenantID: "t-1", ActorID: "u-1", ChangedFields: []string{"enabled", "sla_tiers"}, UpdatedAt: now},
			wantName:       "roulette.config_updated",
			mustHaveFields: []string{"tenant_id", "changed_fields", "updated_at"},
		},
		{
			name:           "ClientInvitedEvent",
			event:          ClientInvitedEvent{TenantID: "t-1", Email: "a@b.com", ClientName: "Alice", Phone: "+5511999999999", AccountID: "acc-1"},
			wantName:       "auth.client.invited",
			mustHaveFields: []string{"email", "name", "phone", "account_id", "tenant_id"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.event.Name(); got != tc.wantName {
				t.Fatalf("Name(): got %q want %q (renaming breaks consumers)", got, tc.wantName)
			}
			if id := tc.event.EventID(); id == "" {
				t.Fatalf("EventID must be non-empty")
			}
			payload := tc.event.Payload()
			if payload == nil {
				t.Fatalf("Payload must be non-nil")
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("payload not JSON-serializable: %v", err)
			}
			s := string(raw)
			for _, field := range tc.mustHaveFields {
				needle := `"` + field + `"`
				if !strings.Contains(s, needle) {
					t.Errorf("payload missing required field %q\nfull: %s", field, s)
				}
			}
		})
	}
}

// TestLeadEventID_UniquePerCall guards against accidental switch to a static
// EventID. The current implementation uses time.Now().UnixNano() so each
// emission is unique — useful for idempotency keys in consumers.
func TestLeadEventID_UniquePerCall(t *testing.T) {
	e := LeadCreatedEvent{LeadID: "lead-1"}
	a := e.EventID()
	time.Sleep(1 * time.Millisecond)
	b := e.EventID()
	if a == b {
		t.Fatalf("EventID should differ between calls; got %q both times", a)
	}
}
