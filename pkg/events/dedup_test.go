package events

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
)

// TestDeriveNotificationID_Deterministic locks in the contract that the same
// natural-key produces the same UUID across processes — handler-side dedup
// in toca-comm (and similar consumers) depends on this so that retries
// collapse via ON CONFLICT (id) DO NOTHING.
func TestDeriveNotificationID_Deterministic(t *testing.T) {
	a := DeriveNotificationID("evt-123", "user-a", "lead_assigned")
	b := DeriveNotificationID("evt-123", "user-a", "lead_assigned")
	if a != b {
		t.Fatalf("expected deterministic id, got %s and %s", a, b)
	}
	if _, err := uuid.Parse(a); err != nil {
		t.Fatalf("derived id must be a valid UUID, got %q: %v", a, err)
	}
}

func TestDeriveNotificationID_DifferentInputs(t *testing.T) {
	cases := []struct {
		name  string
		parts []string
	}{
		{"different event id", []string{"evt-A", "user-1", "action"}},
		{"different user id", []string{"evt-A", "user-2", "action"}},
		{"different action", []string{"evt-A", "user-1", "other_action"}},
		{"empty mixed", []string{"", "user-1", "action"}},
	}
	seen := make(map[string]string, len(cases))
	for _, tc := range cases {
		id := DeriveNotificationID(tc.parts...)
		if prior, dup := seen[id]; dup {
			t.Fatalf("collision between %q and %q both produced %s", prior, tc.name, id)
		}
		seen[id] = tc.name
	}
}

func TestDeriveNotificationID_EmptyAllParts(t *testing.T) {
	// All-empty input falls back to a random uuid so we don't accidentally
	// produce a stable id that every empty call collapses onto.
	a := DeriveNotificationID("", "", "")
	b := DeriveNotificationID("", "", "")
	if a == b {
		t.Fatalf("all-empty input must NOT be deterministic (got identical %s)", a)
	}
}

// TestDeriveNotificationID_SeparatorMatters guards against the trivial pitfall
// where "a|b" and "ab|" would collide if we ever switched to a separator that
// could appear naturally in event ids. The pipe is intentional — it doesn't
// appear in any of our event id formats today.
func TestDeriveNotificationID_SeparatorMatters(t *testing.T) {
	a := DeriveNotificationID("a", "b")
	b := DeriveNotificationID("ab", "")
	if a == b {
		t.Fatalf("DeriveNotificationID must be injective wrt joining, got %s twice", a)
	}
}

// TestInMemoryEventBus_PublishReliable_DeliversToSubscriber covers the new
// PublishReliable surface added in Onda 1: on the in-memory bus it must
// behave like Publish (so unit tests of consumers can rely on synchronous
// delivery without standing up RabbitMQ).
func TestInMemoryEventBus_PublishReliable_DeliversToSubscriber(t *testing.T) {
	bus := NewInMemoryEventBus()

	var (
		mu       sync.Mutex
		received []Event
	)
	if err := bus.Subscribe("toca.test.event", func(_ context.Context, e Event) error {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	evt := SimpleEvent{name: "toca.test.event", id: "evt-1", payload: map[string]any{"k": "v"}}
	if err := bus.PublishReliable(context.Background(), &evt); err != nil {
		t.Fatalf("PublishReliable: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(received))
	}
	if received[0].Name() != "toca.test.event" {
		t.Fatalf("unexpected event name %q", received[0].Name())
	}
}

// TestOutboxBackoff verifies the exponential curve doesn't overflow at high
// attempt counts and respects the 30-minute cap added in Onda 2 (P1.7).
func TestOutboxBackoff(t *testing.T) {
	cases := []struct {
		attempts int
		wantMin  int
		wantMax  int
	}{
		{0, 1, 1},      // 1s
		{1, 2, 2},      // 2s
		{5, 32, 32},    // 32s
		{10, 1024, 1024},
		{15, 1800, 1800}, // capped to 30 min = 1800s
		{50, 1800, 1800}, // still capped
		{-1, 1, 1},       // negative defensive: treated as 0
	}
	for _, tc := range cases {
		d := outboxBackoff(tc.attempts).Seconds()
		if int(d) < tc.wantMin || int(d) > tc.wantMax {
			t.Errorf("outboxBackoff(%d) = %ds; want [%d, %d]", tc.attempts, int(d), tc.wantMin, tc.wantMax)
		}
	}
}
