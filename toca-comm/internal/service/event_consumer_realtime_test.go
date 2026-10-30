package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/oginoino/toca/pkg/events"
)

type realtimeTestEventBus struct {
	handlers map[string]events.EventHandler
}

func newRealtimeTestEventBus() *realtimeTestEventBus {
	return &realtimeTestEventBus{handlers: make(map[string]events.EventHandler)}
}

func (b *realtimeTestEventBus) Publish(context.Context, events.Event) error {
	return nil
}

func (b *realtimeTestEventBus) PublishReliable(context.Context, events.Event) error {
	return nil
}

func (b *realtimeTestEventBus) Subscribe(eventName string, handler events.EventHandler) error {
	b.handlers[eventName] = handler
	return nil
}

func (b *realtimeTestEventBus) SubscribeWithQueue(_ string, eventName string, handler events.EventHandler) error {
	b.handlers[eventName] = handler
	return nil
}

type realtimeTestEvent struct {
	id      string
	name    string
	payload interface{}
}

func (e realtimeTestEvent) EventID() string      { return e.id }
func (e realtimeTestEvent) Name() string         { return e.name }
func (e realtimeTestEvent) Payload() interface{} { return e.payload }

func registerRealtimeTestClient(hub *Hub, tenantID, userID, role string) *Client {
	client := &Client{
		Hub:      hub,
		Send:     make(chan []byte, 4),
		TenantID: tenantID,
		UserID:   userID,
		Role:     role,
	}

	hub.mu.Lock()
	if hub.clients[tenantID] == nil {
		hub.clients[tenantID] = make(map[string]map[*Client]bool)
	}
	if hub.clients[tenantID][userID] == nil {
		hub.clients[tenantID][userID] = make(map[*Client]bool)
	}
	hub.clients[tenantID][userID][client] = true
	hub.mu.Unlock()

	return client
}

func readRealtimeTestEvent(t *testing.T, client *Client) WSEvent {
	t.Helper()

	select {
	case raw := <-client.Send:
		var got WSEvent
		if err := json.Unmarshal(raw, &got); err != nil {
			t.Fatalf("decode websocket event: %v", err)
		}
		return got
	default:
		t.Fatal("expected websocket event, got none")
		return WSEvent{}
	}
}

func assertNoRealtimeTestEvent(t *testing.T, client *Client) {
	t.Helper()

	select {
	case raw := <-client.Send:
		t.Fatalf("unexpected websocket event: %s", raw)
	default:
	}
}

func hasRealtimeTestEventType(t *testing.T, client *Client, eventType string) bool {
	t.Helper()

	for {
		select {
		case raw := <-client.Send:
			var got WSEvent
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatalf("decode websocket event: %v", err)
			}
			if got.Type == eventType {
				return true
			}
		default:
			return false
		}
	}
}

func TestEventConsumer_InternalRealtimeBroadcastDoesNotRequirePortalRecipient(t *testing.T) {
	tests := []struct {
		name          string
		domainEvent   string
		websocketType string
		payload       map[string]interface{}
	}{
		{
			name:          "deal status",
			domainEvent:   "deal.status_changed",
			websocketType: "deal_status_changed",
			payload: map[string]interface{}{
				"deal_id":              "deal-1",
				"tenant_id":            "tenant-1",
				"old_status":           "PROPOSAL",
				"new_status":           "CONTRACT",
				"customer_account_ids": []string{},
				"actor_user_id":        "operator-2",
			},
		},
		{
			name:          "lead status",
			domainEvent:   "lead.status_changed",
			websocketType: "lead_status_changed",
			payload: map[string]interface{}{
				"lead_id":             "lead-1",
				"tenant_id":           "tenant-1",
				"customer_account_id": "",
				"old_status":          "NEW",
				"new_status":          "QUALIFIED",
			},
		},
		{
			name:          "checklist solicitation",
			domainEvent:   "checklist.solicited",
			websocketType: "checklist_solicited",
			payload: map[string]interface{}{
				"deal_id":             "deal-1",
				"tenant_id":           "tenant-1",
				"item_id":             "item-1",
				"item_name":           "Comprovante de renda",
				"customer_account_id": "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub()
			operator := registerRealtimeTestClient(hub, "tenant-1", "operator-1", "corretor")
			portalClient := registerRealtimeTestClient(hub, "tenant-1", "portal-user-1", "cliente")

			bus := newRealtimeTestEventBus()
			consumer := NewEventConsumer(bus, NewMessageService(nil, hub, nil, nil), nil)
			if err := consumer.Start(context.Background()); err != nil {
				t.Fatalf("start event consumer: %v", err)
			}

			handler := bus.handlers[tt.domainEvent]
			if handler == nil {
				t.Fatalf("handler for %q was not registered", tt.domainEvent)
			}
			if err := handler(context.Background(), realtimeTestEvent{
				id:      "event-1",
				name:    tt.domainEvent,
				payload: tt.payload,
			}); err != nil {
				t.Fatalf("handle %q: %v", tt.domainEvent, err)
			}

			got := readRealtimeTestEvent(t, operator)
			if got.Type != tt.websocketType {
				t.Fatalf("websocket event type = %q, want %q", got.Type, tt.websocketType)
			}
			assertNoRealtimeTestEvent(t, portalClient)

			if tt.domainEvent == "deal.status_changed" {
				payload, ok := got.Payload.(map[string]interface{})
				if !ok {
					t.Fatalf("deal payload type = %T, want map", got.Payload)
				}
				if payload["actor_user_id"] != "operator-2" {
					t.Fatalf("actor_user_id = %v, want operator-2", payload["actor_user_id"])
				}
			}
		})
	}
}

func TestEventConsumer_PortalRealtimeAndNotificationStillRequireRecipient(t *testing.T) {
	tests := []struct {
		name          string
		domainEvent   string
		websocketType string
		payload       map[string]interface{}
	}{
		{
			name:          "deal status",
			domainEvent:   "deal.status_changed",
			websocketType: "deal_status_changed",
			payload: map[string]interface{}{
				"deal_id":              "deal-1",
				"tenant_id":            "tenant-1",
				"old_status":           "PROPOSAL",
				"new_status":           "CONTRACT",
				"customer_account_ids": []string{"portal-user-1"},
				"actor_user_id":        "operator-2",
			},
		},
		{
			name:          "lead status",
			domainEvent:   "lead.status_changed",
			websocketType: "lead_status_changed",
			payload: map[string]interface{}{
				"lead_id":             "lead-1",
				"tenant_id":           "tenant-1",
				"customer_account_id": "portal-user-1",
				"old_status":          "NEW",
				"new_status":          "QUALIFIED",
			},
		},
		{
			name:          "checklist solicitation",
			domainEvent:   "checklist.solicited",
			websocketType: "checklist_solicited",
			payload: map[string]interface{}{
				"deal_id":             "deal-1",
				"tenant_id":           "tenant-1",
				"item_id":             "item-1",
				"item_name":           "Comprovante de renda",
				"customer_account_id": "portal-user-1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hub := NewHub()
			operator := registerRealtimeTestClient(hub, "tenant-1", "operator-1", "corretor")
			portalClient := registerRealtimeTestClient(hub, "tenant-1", "portal-user-1", "cliente")
			inAppRepo := &mockInAppRepo{}
			inAppService := NewInAppNotificationService(inAppRepo, hub)

			bus := newRealtimeTestEventBus()
			consumer := NewEventConsumer(bus, NewMessageService(nil, hub, nil, inAppService), inAppService)
			if err := consumer.Start(context.Background()); err != nil {
				t.Fatalf("start event consumer: %v", err)
			}

			handler := bus.handlers[tt.domainEvent]
			if handler == nil {
				t.Fatalf("handler for %q was not registered", tt.domainEvent)
			}
			if err := handler(context.Background(), realtimeTestEvent{
				id:      "event-1",
				name:    tt.domainEvent,
				payload: tt.payload,
			}); err != nil {
				t.Fatalf("handle %q: %v", tt.domainEvent, err)
			}

			if !hasRealtimeTestEventType(t, operator, tt.websocketType) {
				t.Fatalf("operator did not receive %q", tt.websocketType)
			}
			if !hasRealtimeTestEventType(t, portalClient, tt.websocketType) {
				t.Fatalf("portal client did not receive %q", tt.websocketType)
			}
			if len(inAppRepo.notifications) != 1 {
				t.Fatalf("portal notifications = %d, want 1", len(inAppRepo.notifications))
			}
			if inAppRepo.notifications[0].UserID != "portal-user-1" {
				t.Fatalf("notification user = %q, want portal-user-1", inAppRepo.notifications[0].UserID)
			}
		})
	}
}
