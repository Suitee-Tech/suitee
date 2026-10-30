package events

import (
	"context"
	"sync"
)

// InMemoryEventBus is a simple in-memory implementation of EventBus
type InMemoryEventBus struct {
	handlers map[string][]EventHandler
	mu       sync.RWMutex
}

func NewInMemoryEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		handlers: make(map[string][]EventHandler),
	}
}

// PublishReliable on the in-memory bus has no broker to fail against, so it
// simply delegates to Publish. Tests that need to assert reliable-publish
// semantics can rely on the same handler invocation as Publish.
func (b *InMemoryEventBus) PublishReliable(ctx context.Context, event Event) error {
	return b.Publish(ctx, event)
}

func (b *InMemoryEventBus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers, ok := b.handlers[event.Name()]
	b.mu.RUnlock()

	if !ok {
		return nil
	}

	// Execute handlers concurrently or sequentially?
	// For simplicity, sequentially for now, but in a separate goroutine to be non-blocking?
	// Or blocking? Usually Publish is fire-and-forget or strictly consistent.
	// Let's make it blocking for simplicity and consistency in this monolith phase.

	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			// Log error?
			return err
		}
	}

	return nil
}

func (b *InMemoryEventBus) Subscribe(eventName string, handler EventHandler) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[eventName] = append(b.handlers[eventName], handler)
	return nil
}

func (b *InMemoryEventBus) SubscribeWithQueue(_ string, eventName string, handler EventHandler) error {
	return b.Subscribe(eventName, handler)
}
