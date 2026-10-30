package health

import (
	"context"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type mutableRabbitMQProvider struct {
	channel *amqp.Channel
}

func (p *mutableRabbitMQProvider) Channel() *amqp.Channel { return p.channel }

func TestDynamicRabbitMQCheckerReadsCurrentChannelOnEveryCheck(t *testing.T) {
	provider := &mutableRabbitMQProvider{}
	checker := NewDynamicRabbitMQChecker(provider)
	if err := checker.Check(context.Background()); err == nil {
		t.Fatal("Check() with missing initial channel succeeded")
	}

	provider.channel = &amqp.Channel{}
	if err := checker.Check(context.Background()); err != nil {
		t.Fatalf("Check() after channel replacement = %v, want success", err)
	}
}

func TestDynamicRabbitMQCheckerRejectsMissingProvider(t *testing.T) {
	checker := NewDynamicRabbitMQChecker(nil)
	if err := checker.Check(context.Background()); err == nil {
		t.Fatal("Check() with missing provider succeeded")
	}
}
