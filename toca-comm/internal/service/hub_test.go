package service

import (
	"encoding/json"
	"testing"
)

func TestHub_BroadcastEvent_NoClients(t *testing.T) {
	hub := NewHub()
	// No client registered — must not panic and must be a no-op.
	hub.BroadcastEvent("tenant-1", []string{"user-1"}, "deal_status_changed", map[string]any{"deal_id": "abc"})
}

func TestHub_BroadcastEvent_DeliversToRegisteredClient(t *testing.T) {
	hub := NewHub()

	// Register a fake client without running Hub.Run() — register directly into the
	// internal map. This isolates the broadcast logic from the goroutine pump.
	client := &Client{
		Hub:      hub,
		Conn:     nil,
		Send:     make(chan []byte, 1),
		TenantID: "tenant-1",
		UserID:   "user-1",
	}
	hub.mu.Lock()
	hub.clients["tenant-1"] = map[string]map[*Client]bool{
		"user-1": {client: true},
	}
	hub.mu.Unlock()

	payload := map[string]any{"deal_id": "abc", "new_status": "CONVERTED"}
	hub.BroadcastEvent("tenant-1", []string{"user-1"}, "deal_status_changed", payload)

	select {
	case data := <-client.Send:
		var got WSEvent
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("malformed WSEvent: %v", err)
		}
		if got.Type != "deal_status_changed" {
			t.Errorf("event type = %q, want %q", got.Type, "deal_status_changed")
		}
	default:
		t.Fatal("expected event on client.Send, got none")
	}
}

func TestHub_BroadcastEvent_DeliversExplicitParticipantAcrossTenantBoundary(t *testing.T) {
	hub := NewHub()

	client := &Client{
		Send:     make(chan []byte, 1),
		TenantID: "tenant-1",
		UserID:   "user-1",
	}
	hub.mu.Lock()
	hub.clients["tenant-1"] = map[string]map[*Client]bool{
		"user-1": {client: true},
	}
	hub.mu.Unlock()

	hub.BroadcastEvent("tenant-OTHER", []string{"user-1"}, "x", "y")

	select {
	case data := <-client.Send:
		var got WSEvent
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("malformed WSEvent: %v", err)
		}
		if got.Type != "x" {
			t.Fatalf("event type = %q, want x", got.Type)
		}
	default:
		t.Fatal("explicit participant should receive the event even when the portal token uses another tenant")
	}
}

func TestHub_BroadcastEvent_DoesNotDeliverCrossTenantToUnlistedParticipant(t *testing.T) {
	hub := NewHub()
	client := &Client{Send: make(chan []byte, 1), TenantID: "tenant-1", UserID: "user-1"}
	hub.clients["tenant-1"] = map[string]map[*Client]bool{
		"user-1": {client: true},
	}

	hub.BroadcastEvent("tenant-OTHER", []string{"another-user"}, "x", "y")

	select {
	case <-client.Send:
		t.Fatal("unlisted participant received a cross-tenant event")
	default:
	}
}

func TestHub_BroadcastEvent_DropsWhenSendFull(t *testing.T) {
	hub := NewHub()

	// Send channel with capacity 1, already filled — broadcast should NOT block.
	client := &Client{
		Send:     make(chan []byte, 1),
		TenantID: "tenant-1",
		UserID:   "user-1",
	}
	client.Send <- []byte("pre-existing")

	hub.mu.Lock()
	hub.clients["tenant-1"] = map[string]map[*Client]bool{
		"user-1": {client: true},
	}
	hub.mu.Unlock()

	done := make(chan struct{})
	go func() {
		hub.BroadcastEvent("tenant-1", []string{"user-1"}, "evt", "x")
		close(done)
	}()
	<-done
	// Successful return means the select default path fired (no deadlock).
}
