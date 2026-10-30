package services

import (
	"context"

	"github.com/oginoino/toca/pkg/events"
)

// ReliablePublisher é o recorte do EventBus necessário para publicar com
// garantia (outbox). Reduz a dependência dos testes e deixa explícito que
// eventos do toca-sites precisam sobreviver a broker fora do ar.
type ReliablePublisher interface {
	PublishReliable(ctx context.Context, event events.Event) error
}

// nopPublisher é o default quando o serviço roda sem barramento (testes/dev).
type nopPublisher struct{}

func (nopPublisher) PublishReliable(context.Context, events.Event) error { return nil }
