package events

import "context"

// Event represents a domain event
type Event interface {
	EventID() string
	Name() string
	Payload() interface{}
}

// EventHandler handles an event
type EventHandler func(ctx context.Context, event Event) error

// EventBus defines the interface for publishing and subscribing to events.
//
// Publish is fire-and-forget against the broker — if RabbitMQ is unreachable
// the event is lost. Use PublishReliable for events whose delivery must
// survive broker downtime: it persists to events.outbox_events (Postgres) and
// a background drainer publishes asynchronously with retry. PublishReliable
// has an at-least-once guarantee — consumers must remain idempotent.
type EventBus interface {
	Publish(ctx context.Context, event Event) error
	PublishReliable(ctx context.Context, event Event) error
	Subscribe(eventName string, handler EventHandler) error
	SubscribeWithQueue(queueName string, eventName string, handler EventHandler) error
}
