package events

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	pkgctx "github.com/oginoino/toca/pkg/context"
	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeConfirmChannel struct {
	confirmErr error
	publishErr error
	outcome    string
	mandatory  bool
	immediate  bool

	confirmations chan amqp.Confirmation
	returns       chan amqp.Return
	closes        chan *amqp.Error
}

func (f *fakeConfirmChannel) Confirm(bool) error { return f.confirmErr }

func (f *fakeConfirmChannel) NotifyPublish(receiver chan amqp.Confirmation) chan amqp.Confirmation {
	f.confirmations = receiver
	return receiver
}

func (f *fakeConfirmChannel) NotifyReturn(receiver chan amqp.Return) chan amqp.Return {
	f.returns = receiver
	return receiver
}

func (f *fakeConfirmChannel) NotifyClose(receiver chan *amqp.Error) chan *amqp.Error {
	f.closes = receiver
	return receiver
}

func (f *fakeConfirmChannel) PublishWithContext(_ context.Context, exchange, key string, mandatory, immediate bool, _ amqp.Publishing) error {
	f.mandatory = mandatory
	f.immediate = immediate
	if f.publishErr != nil {
		return f.publishErr
	}
	switch f.outcome {
	case "returned":
		f.returns <- amqp.Return{ReplyCode: 312, ReplyText: "NO_ROUTE", Exchange: exchange, RoutingKey: key}
		f.confirmations <- amqp.Confirmation{Ack: true}
	case "nack":
		f.confirmations <- amqp.Confirmation{Ack: false}
	case "closed":
		f.closes <- &amqp.Error{Code: 504, Reason: "channel closed"}
	default:
		f.confirmations <- amqp.Confirmation{Ack: true}
	}
	return nil
}

func TestPublishConfirmedRequiresRouteAndBrokerAck(t *testing.T) {
	ch := &fakeConfirmChannel{}
	err := publishConfirmedOnChannel(context.Background(), ch, ExchangeName, "invoice.paid", amqp.Publishing{Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("publishConfirmedOnChannel() error = %v", err)
	}
	if !ch.mandatory || ch.immediate {
		t.Fatalf("publish flags = mandatory:%v immediate:%v, want true/false", ch.mandatory, ch.immediate)
	}
}

func TestPublishConfirmedRejectsMandatoryReturnEvenWhenBrokerAcks(t *testing.T) {
	ch := &fakeConfirmChannel{outcome: "returned"}
	err := publishConfirmedOnChannel(context.Background(), ch, ExchangeName, "missing.route", amqp.Publishing{Body: []byte(`{}`)})
	if err == nil || !strings.Contains(err.Error(), "unroutable") {
		t.Fatalf("publishConfirmedOnChannel() error = %v, want unroutable", err)
	}
}

func TestPublishConfirmedRejectsBrokerNackAndChannelClose(t *testing.T) {
	for _, tc := range []struct {
		name    string
		outcome string
		want    string
	}{
		{name: "nack", outcome: "nack", want: "rejected"},
		{name: "channel close", outcome: "closed", want: "closed before acknowledgement"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := publishConfirmedOnChannel(context.Background(), &fakeConfirmChannel{outcome: tc.outcome}, ExchangeName, "event", amqp.Publishing{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("publishConfirmedOnChannel() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestWaitForTransportCloseObservesChannelFailure(t *testing.T) {
	stop := make(chan struct{})
	connClose := make(chan *amqp.Error, 1)
	channelClose := make(chan *amqp.Error, 1)
	want := &amqp.Error{Code: 406, Reason: "PRECONDITION_FAILED"}
	channelClose <- want

	kind, got, stopped := waitForTransportClose(stop, connClose, channelClose)
	if stopped || kind != "channel" || got != want {
		t.Fatalf("waitForTransportClose() = (%q, %v, %v), want channel failure", kind, got, stopped)
	}
}

func TestSetupAllSubscriptionsPropagatesFirstFailure(t *testing.T) {
	subs := []subscription{{queueName: "q1", eventName: "e1"}, {queueName: "q2", eventName: "e2"}}
	want := errors.New("queue precondition failed")
	calls := 0
	err := setupAllSubscriptions(subs, func(subscription) error {
		calls++
		if calls == 1 {
			return want
		}
		return nil
	})
	if !errors.Is(err, want) || calls != 1 {
		t.Fatalf("setupAllSubscriptions() = (calls:%d, err:%v), want first failure", calls, err)
	}
}

func TestNewDeliveryContextCarriesTenantAndDeadline(t *testing.T) {
	started := time.Now()
	ctx, cancel := newDeliveryContext(amqp.Table{"tenant_id": "tenant-1"})
	defer cancel()

	tenantID, err := pkgctx.GetTenantID(ctx)
	if err != nil || tenantID != "tenant-1" {
		t.Fatalf("tenant context = (%q, %v), want tenant-1", tenantID, err)
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("delivery context has no deadline")
	}
	remaining := deadline.Sub(started)
	if remaining > deliveryHandlerTimeout+time.Second || remaining < deliveryHandlerTimeout-time.Second {
		t.Fatalf("delivery deadline remaining = %s, want about %s", remaining, deliveryHandlerTimeout)
	}
}

func TestDLQLegacyMessageIDsAreUnique(t *testing.T) {
	first := dlqMessageID("")
	second := dlqMessageID("")
	if first == second || !strings.HasPrefix(first, "legacy-") || !strings.HasPrefix(second, "legacy-") {
		t.Fatalf("legacy IDs = %q and %q, want distinct legacy UUIDs", first, second)
	}
	if got := dlqMessageID("producer-id"); got != "producer-id" {
		t.Fatalf("producer message ID changed to %q", got)
	}
}

func TestDLQConsumerClaimIsOncePerGeneration(t *testing.T) {
	ch := &amqp.Channel{}
	bus := &RabbitMQEventBus{
		channel:    ch,
		generation: 7,
		dlqStarted: make(map[string]uint64),
	}

	generation, claimed, err := bus.claimDLQConsumer(ch, "queue.dlq")
	if err != nil || !claimed || generation != 7 {
		t.Fatalf("first claim = (%d, %v, %v), want generation 7 claimed", generation, claimed, err)
	}
	_, claimed, err = bus.claimDLQConsumer(ch, "queue.dlq")
	if err != nil || claimed {
		t.Fatalf("duplicate claim = (%v, %v), want no-op", claimed, err)
	}

	bus.mu.Lock()
	bus.generation = 8
	bus.dlqStarted = make(map[string]uint64)
	bus.mu.Unlock()
	generation, claimed, err = bus.claimDLQConsumer(ch, "queue.dlq")
	if err != nil || !claimed || generation != 8 {
		t.Fatalf("new generation claim = (%d, %v, %v), want generation 8 claimed", generation, claimed, err)
	}
}
