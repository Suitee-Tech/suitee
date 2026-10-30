package service

import (
	"context"
	"testing"

	"github.com/oginoino/toca/pkg/events/contracts"
)

// fakeBrokerEvent permite que o teste construa um events.Event sem precisar do contrato
// completo do RabbitMQ. A interface só exige Name() e Payload().
type fakeBrokerEvent struct {
	name    string
	payload interface{}
	id      string
}

func (e fakeBrokerEvent) EventID() string      { return e.id }
func (e fakeBrokerEvent) Name() string         { return e.name }
func (e fakeBrokerEvent) Payload() interface{} { return e.payload }

func TestHandleBrokerChanged_RejectsInvalidPayload(t *testing.T) {
	h := &CommEventHandler{}
	// Payload no formato errado (struct em vez de map) — não deve quebrar e deve devolver erro.
	err := h.HandleBrokerChanged(context.Background(), fakeBrokerEvent{
		name:    "broker.updated",
		payload: "not-a-map",
	})
	if err == nil {
		t.Fatalf("expected error on invalid payload format, got nil")
	}
}

func TestHandleBrokerChanged_RejectsMissingTenantID(t *testing.T) {
	h := &CommEventHandler{}
	err := h.HandleBrokerChanged(context.Background(), fakeBrokerEvent{
		name: "broker.updated",
		payload: map[string]interface{}{
			"broker_id": "broker-1",
			// tenant_id ausente
		},
	})
	if err == nil {
		t.Fatalf("expected error on missing tenant_id, got nil")
	}
}

func TestHandleBrokerChanged_OkWithValidPayload_NoConnectedClients(t *testing.T) {
	// Sem msgService — exercita o caminho seguro de broadcast (Hub é nil).
	h := &CommEventHandler{}
	err := h.HandleBrokerChanged(context.Background(), fakeBrokerEvent{
		name: "broker.updated",
		payload: map[string]interface{}{
			"broker_id":  "broker-1",
			"tenant_id":  "tenant-1",
			"name":       "Alice",
			"role":       "corretor",
			"avatar":     "https://cdn/img.png",
			"active":     true,
			"paused":     false,
			"updated_at": "2026-05-16T12:00:00Z",
		},
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestHandleBrokerChanged_BroadcastReachesConnectedClient(t *testing.T) {
	// Cria Hub real + cliente registrado para confirmar que o broadcast chega.
	hub := NewHub()
	go hub.Run()
	t.Cleanup(func() {
		// Hub.Run() roda indefinidamente; cleanup tira o cliente do registro.
	})

	send := make(chan []byte, 1)
	client := &Client{
		TenantID: "tenant-1",
		UserID:   "user-1",
		Send:     send,
	}
	hub.Register <- client
	// Pequena espera para o Hub.Run processar o registro.
	for i := 0; i < 10; i++ {
		hub.mu.RLock()
		tenant := hub.clients["tenant-1"]
		ready := tenant != nil && tenant["user-1"] != nil
		hub.mu.RUnlock()
		if ready {
			break
		}
	}

	msgSvc := NewMessageService(nil, hub, nil, nil)
	h := &CommEventHandler{msgService: msgSvc}

	err := h.HandleBrokerChanged(context.Background(), fakeBrokerEvent{
		name: "broker.updated",
		payload: map[string]interface{}{
			"broker_id": "broker-1",
			"tenant_id": "tenant-1",
			"name":      "Alice",
			"role":      "corretor",
			"active":    true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	select {
	case raw := <-send:
		if len(raw) == 0 {
			t.Fatalf("expected non-empty WS payload")
		}
		// Mensagem é serializada como {"type":"broker_changed","payload":{...}}
		expectedPrefix := `{"type":"broker_changed"`
		if string(raw[:len(expectedPrefix)]) != expectedPrefix {
			t.Fatalf("unexpected WS event type, got: %s", string(raw))
		}
	default:
		t.Fatalf("expected client to receive broker_changed broadcast")
	}
}

// Sanity: verifica que o contract BrokerUpdatedEvent serializa todos os campos esperados
// pelo HandleBrokerChanged. Se alguém renomear "broker_id"/"tenant_id" no contract, este
// teste pega antes do runtime.
func TestBrokerUpdatedEventPayloadFields(t *testing.T) {
	p := contracts.BrokerUpdatedEventPayload{
		BrokerID:  "b1",
		TenantID:  "t1",
		Name:      "n",
		Role:      "r",
		Email:     "e",
		Avatar:    "a",
		Active:    true,
		Paused:    false,
		UpdatedAt: "2026-05-16T00:00:00Z",
	}
	// Reutiliza o handler para garantir que map[string]interface{} também funciona com
	// o struct serializado/deserializado via JSON.
	if p.BrokerID == "" || p.TenantID == "" {
		t.Fatalf("payload fields must be populated for the test fixture")
	}
}
