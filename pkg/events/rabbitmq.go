package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/metrics"
	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "toca.events"
	ExchangeType = "topic"

	DLXExchangeName   = "toca.events.dlx"
	RetryExchangeName = "toca.events.retry.return"

	reconnectDelay = 3 * time.Second
	maxRetries     = 10

	handlerMaxRetries = 5
	handlerRetryBase  = 2 * time.Second
	handlerRetryMax   = 60 * time.Second

	deliveryHandlerTimeout = 60 * time.Second
	publishConfirmTimeout  = 5 * time.Second
)

// subscription holds info needed to re-subscribe after reconnect
type subscription struct {
	queueName string
	eventName string
	handler   EventHandler
}

// RabbitMQEventBus implements EventBus interface using RabbitMQ
type RabbitMQEventBus struct {
	conn          *amqp.Connection
	channel       *amqp.Channel
	uri           string
	pgDB          *database.PostgresDB // NOVO: Suporte a Postgres
	mu            sync.Mutex
	subscriptions []subscription
	dlqStarted    map[string]uint64
	generation    uint64
	closed        bool
	closeCh       chan struct{}
}

// NewRabbitMQEventBusPG creates a new RabbitMQEventBus using Postgres
func NewRabbitMQEventBusPG(uri string, pgDB *database.PostgresDB) (*RabbitMQEventBus, error) {
	bus := &RabbitMQEventBus{
		uri:        uri,
		pgDB:       pgDB,
		closeCh:    make(chan struct{}),
		dlqStarted: make(map[string]uint64),
	}

	bus.mu.Lock()
	err := bus.connectLocked()
	bus.mu.Unlock()
	if err != nil {
		log.Printf("Warning: failed to connect to RabbitMQ initially: %v. Will retry in background.", err)
	}

	go bus.monitorConnection()

	if pgDB != nil {
		bus.startOutboxPublisher()
	}

	return bus, nil
}

func (b *RabbitMQEventBus) Channel() *amqp.Channel {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.channel
}

// connectLocked establishes a fresh transport and publishes it as the current
// generation. The caller must hold b.mu. Keeping the generation change and the
// DLQ-consumer reset under the same lock prevents an old channel from claiming
// a consumer slot after reconnect.
func (b *RabbitMQEventBus) connectLocked() error {
	conn, err := amqp.Dial(b.uri)
	if err != nil {
		return fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to open a channel: %w", err)
	}

	// Declare main exchange
	err = ch.ExchangeDeclare(
		ExchangeName, // name
		ExchangeType, // type
		true,         // durable
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare exchange: %w", err)
	}

	// Declare dead-letter exchange
	err = ch.ExchangeDeclare(
		DLXExchangeName, // name
		"topic",         // type
		true,            // durable
		false,           // auto-deleted
		false,           // internal
		false,           // no-wait
		nil,             // arguments
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare DLX exchange: %w", err)
	}

	err = ch.ExchangeDeclare(
		RetryExchangeName,
		"direct",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return fmt.Errorf("failed to declare retry return exchange: %w", err)
	}

	b.conn = conn
	b.channel = ch
	b.generation++
	b.dlqStarted = make(map[string]uint64)
	return nil
}

func (b *RabbitMQEventBus) monitorConnection() {
	for {
		select {
		case <-b.closeCh:
			return
		default:
		}

		b.mu.Lock()
		conn := b.conn
		ch := b.channel
		closed := b.closed
		b.mu.Unlock()
		if closed {
			return
		}

		if conn == nil || conn.IsClosed() || ch == nil || ch.IsClosed() {
			b.reconnect()
			continue
		}

		// A protocol exception can close only the AMQP channel while the TCP
		// connection stays open. Observe both or every consumer/publisher on the
		// shared channel can remain dead forever without triggering reconnect.
		connClose := conn.NotifyClose(make(chan *amqp.Error, 1))
		channelClose := ch.NotifyClose(make(chan *amqp.Error, 1))
		kind, closeErr, stopped := waitForTransportClose(b.closeCh, connClose, channelClose)
		if stopped {
			return
		}
		log.Printf("RabbitMQ %s lost: %v. Reconnecting...", kind, closeErr)
		b.reconnect()
	}
}

func waitForTransportClose(stop <-chan struct{}, connClose, channelClose <-chan *amqp.Error) (kind string, err *amqp.Error, stopped bool) {
	select {
	case <-stop:
		return "", nil, true
	case err = <-connClose:
		return "connection", err, false
	case err = <-channelClose:
		return "channel", err, false
	}
}

func (b *RabbitMQEventBus) reconnect() {
	// Invalidate and close the broken generation first. Channel-only failures do
	// not close the TCP connection themselves, so leaving it behind would leak a
	// connection on every recovery.
	b.mu.Lock()
	oldChannel := b.channel
	oldConn := b.conn
	b.channel = nil
	b.conn = nil
	b.dlqStarted = make(map[string]uint64)
	b.mu.Unlock()
	if oldChannel != nil && !oldChannel.IsClosed() {
		_ = oldChannel.Close()
	}
	if oldConn != nil && !oldConn.IsClosed() {
		_ = oldConn.Close()
	}

	for i := 0; i < maxRetries; i++ {
		select {
		case <-b.closeCh:
			return
		default:
		}

		log.Printf("RabbitMQ reconnect attempt %d/%d", i+1, maxRetries)

		b.mu.Lock()
		err := b.connectLocked()
		subs := append([]subscription(nil), b.subscriptions...)
		b.mu.Unlock()
		if err != nil {
			log.Printf("RabbitMQ reconnect failed: %v", err)
			time.Sleep(reconnectDelay * time.Duration(i+1))
			continue
		}

		// Do not report recovery until every durable consumer (including its DLQ
		// drain) is live. A queue precondition failure closes only the channel; in
		// that case close this candidate transport and retry the whole topology.
		if err := setupAllSubscriptions(subs, func(sub subscription) error {
			return b.setupSubscription(sub.queueName, sub.eventName, sub.handler)
		}); err != nil {
			log.Printf("RabbitMQ reconnect topology failed: %v", err)
			b.mu.Lock()
			failedChannel := b.channel
			failedConn := b.conn
			b.channel = nil
			b.conn = nil
			b.dlqStarted = make(map[string]uint64)
			b.mu.Unlock()
			if failedChannel != nil && !failedChannel.IsClosed() {
				_ = failedChannel.Close()
			}
			if failedConn != nil && !failedConn.IsClosed() {
				_ = failedConn.Close()
			}
			time.Sleep(reconnectDelay * time.Duration(i+1))
			continue
		}

		log.Println("RabbitMQ reconnected successfully")
		return
	}

	log.Printf("RabbitMQ reconnect failed after %d attempts", maxRetries)
}

func setupAllSubscriptions(subs []subscription, setup func(subscription) error) error {
	for _, sub := range subs {
		if err := setup(sub); err != nil {
			return fmt.Errorf("re-subscribe %s on %s: %w", sub.eventName, sub.queueName, err)
		}
	}
	return nil
}

// Close closes the connection and channel
func (b *RabbitMQEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}
	b.closed = true
	close(b.closeCh)

	if b.channel != nil {
		b.channel.Close()
	}
	if b.conn != nil {
		b.conn.Close()
	}
}

// Publish publishes an event to the exchange
func (b *RabbitMQEventBus) Publish(ctx context.Context, event Event) error {
	body, err := json.Marshal(event.Payload())
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	// Extract tenant_id from payload for AMQP header propagation
	headers := amqp.Table{}
	if tenantID, err := pkgctx.GetTenantID(ctx); err == nil && tenantID != "" {
		headers["tenant_id"] = tenantID
	} else {
		// Fallback: try to extract from payload
		if payloadMap, ok := event.Payload().(map[string]interface{}); ok {
			if tid, ok := payloadMap["tenant_id"].(string); ok {
				headers["tenant_id"] = tid
			}
		}
	}

	msgID := ""
	if event.EventID() != "" {
		msgID = event.EventID()
	} else {
		msgID = uuid.NewString()
	}

	err = b.publishConfirmed(ctx,
		ExchangeName, // exchange
		event.Name(), // routing key
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
			Type:         event.Name(),
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			MessageId:    msgID,
		})

	if err != nil {
		return fmt.Errorf("failed to publish a message: %w", err)
	}

	return nil
}

type confirmPublishingChannel interface {
	Confirm(noWait bool) error
	NotifyPublish(confirm chan amqp.Confirmation) chan amqp.Confirmation
	NotifyReturn(receiver chan amqp.Return) chan amqp.Return
	NotifyClose(receiver chan *amqp.Error) chan *amqp.Error
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// publishConfirmed uses a dedicated confirm channel for one publication. A
// successful method return alone only means the frame reached the client
// socket; waiting for broker confirmation and mandatory returns is what lets
// the outbox/replay code safely advance its database state.
func (b *RabbitMQEventBus) publishConfirmed(ctx context.Context, exchange, routingKey string, publishing amqp.Publishing) error {
	b.mu.Lock()
	conn := b.conn
	b.mu.Unlock()
	if conn == nil || conn.IsClosed() {
		return fmt.Errorf("rabbitmq connection not initialized")
	}

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq confirm channel: %w", err)
	}
	defer ch.Close()

	return publishConfirmedOnChannel(ctx, ch, exchange, routingKey, publishing)
}

func publishConfirmedOnChannel(ctx context.Context, ch confirmPublishingChannel, exchange, routingKey string, publishing amqp.Publishing) error {
	if ch == nil {
		return fmt.Errorf("rabbitmq confirm channel is nil")
	}
	if err := ch.Confirm(false); err != nil {
		return fmt.Errorf("enable rabbitmq publisher confirms: %w", err)
	}
	confirmations := ch.NotifyPublish(make(chan amqp.Confirmation, 1))
	returns := ch.NotifyReturn(make(chan amqp.Return, 1))
	closes := ch.NotifyClose(make(chan *amqp.Error, 1))

	// mandatory=true makes a healthy exchange with no matching queue an error
	// instead of a silent drop. immediate is intentionally false (unsupported
	// by modern RabbitMQ and unnecessary for durable queues).
	if err := ch.PublishWithContext(ctx, exchange, routingKey, true, false, publishing); err != nil {
		return fmt.Errorf("publish rabbitmq message: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, publishConfirmTimeout)
	defer cancel()
	return awaitPublishOutcome(waitCtx, confirmations, returns, closes)
}

func awaitPublishOutcome(ctx context.Context, confirmations <-chan amqp.Confirmation, returns <-chan amqp.Return, closes <-chan *amqp.Error) error {
	for {
		select {
		case returned, ok := <-returns:
			if !ok {
				returns = nil
				continue
			}
			return fmt.Errorf("rabbitmq returned unroutable message: code=%d text=%s exchange=%s routing_key=%s",
				returned.ReplyCode, returned.ReplyText, returned.Exchange, returned.RoutingKey)
		case confirmation, ok := <-confirmations:
			if !ok {
				return fmt.Errorf("rabbitmq publisher confirmation channel closed")
			}
			if !confirmation.Ack {
				return fmt.Errorf("rabbitmq rejected published message")
			}
			// AMQP orders basic.return before basic.ack for an unroutable
			// mandatory publish. The client dispatcher writes to buffered listener
			// channels in that order, so inspect the return buffer once more before
			// accepting the confirmation.
			select {
			case returned, ok := <-returns:
				if ok {
					return fmt.Errorf("rabbitmq returned unroutable message: code=%d text=%s exchange=%s routing_key=%s",
						returned.ReplyCode, returned.ReplyText, returned.Exchange, returned.RoutingKey)
				}
			default:
			}
			return nil
		case closeErr, ok := <-closes:
			if !ok || closeErr == nil {
				return fmt.Errorf("rabbitmq confirm channel closed before acknowledgement")
			}
			return fmt.Errorf("rabbitmq confirm channel closed before acknowledgement: %w", closeErr)
		case <-ctx.Done():
			return fmt.Errorf("wait for rabbitmq publisher confirmation: %w", ctx.Err())
		}
	}
}

func (b *RabbitMQEventBus) Subscribe(eventName string, handler EventHandler) error {
	// Create a temporary, auto-deleted queue for basic subscribe
	q, err := b.channel.QueueDeclare(
		"",    // random name
		false, // not durable
		true,  // delete when unused
		true,  // exclusive
		false, // no-wait
		nil,   // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare queue: %w", err)
	}

	if err := b.channel.QueueBind(
		q.Name,
		eventName,
		ExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to bind queue: %w", err)
	}

	msgs, err := b.channel.Consume(
		q.Name,
		"",    // consumer tag
		false, // auto-ack
		true,  // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register consumer: %w", err)
	}

	go func() {
		for msg := range msgs {
			func() {
				ctx, cancel := newDeliveryContext(msg.Headers)
				defer cancel()

				var payload interface{}
				if err := json.Unmarshal(msg.Body, &payload); err != nil {
					log.Printf("Error unmarshalling temporary event body: %v", err)
					_ = msg.Nack(false, true)
					return
				}

				evt := &SimpleEvent{
					name:    eventName,
					payload: payload,
					id:      msg.MessageId,
				}

				if err := handler(ctx, evt); err != nil {
					log.Printf("Failed to handle temporary event: %v", err)
					_ = msg.Nack(false, true) // Requeue
				} else {
					_ = msg.Ack(false)
				}
			}()
		}
	}()

	return nil
}

// SubscribeWithQueue subscribes using a specific queue name for durability
func (b *RabbitMQEventBus) SubscribeWithQueue(queueName string, eventName string, handler EventHandler) error {
	if queueName == "" {
		return fmt.Errorf("durable subscription queue name is required")
	}
	b.mu.Lock()
	b.subscriptions = append(b.subscriptions, subscription{
		queueName: queueName,
		eventName: eventName,
		handler:   handler,
	})
	connected := b.channel != nil
	b.mu.Unlock()

	if !connected {
		log.Printf("RabbitMQ not connected yet, subscription to %s saved for later", eventName)
		return nil
	}

	return b.setupSubscription(queueName, eventName, handler)
}

func (b *RabbitMQEventBus) setupSubscription(queueName string, eventName string, handler EventHandler) error {
	b.mu.Lock()
	ch := b.channel
	b.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("rabbitmq channel not initialized")
	}

	dlqName := queueName + ".dlq"
	retryQueueName := queueName + ".retry"
	if queueName == "" {
		dlqName = eventName + ".dlq"
		retryQueueName = eventName + ".retry"
	}

	// Declare dead-letter queue
	_, err := ch.QueueDeclare(
		dlqName, // name
		true,    // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return fmt.Errorf("failed to declare DLQ: %w", err)
	}

	// Bind DLQ to DLX exchange
	err = ch.QueueBind(
		dlqName,         // queue name
		eventName,       // routing key
		DLXExchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind DLQ: %w", err)
	}
	// Retried deliveries return to the consumer queue with queueName as their
	// routing key. Bind that key too so exhaustion still reaches the same DLQ.
	if err := ch.QueueBind(dlqName, queueName, DLXExchangeName, false, nil); err != nil {
		return fmt.Errorf("failed to bind retried deliveries to DLQ: %w", err)
	}

	// Failed handlers are republished here with a per-message expiration. The
	// queue has no consumer; after the backoff RabbitMQ dead-letters the message
	// to the original consumer queue. This provides bounded retries without a hot
	// Nack(requeue=true) loop. Only after handlerMaxRetries does the main queue's
	// DLX route the message to the operational DLQ.
	_, err = ch.QueueDeclare(
		retryQueueName,
		true,
		false,
		false,
		false,
		amqp.Table{
			// A dedicated direct exchange routes back only to this consumer.
			// Sending through the topic exchange would redeliver to every service.
			"x-dead-letter-exchange":    RetryExchangeName,
			"x-dead-letter-routing-key": queueName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare retry queue: %w", err)
	}

	// Declare main queue with DLX configuration
	q, err := ch.QueueDeclare(
		queueName, // name (empty means auto-generated, but named is better for durability)
		true,      // durable
		false,     // delete when unused
		false,     // exclusive (false to allow reconnect)
		false,     // no-wait
		amqp.Table{
			"x-dead-letter-exchange": DLXExchangeName,
		},
	)
	if err != nil {
		return fmt.Errorf("failed to declare a queue: %w", err)
	}
	err = ch.QueueBind(
		q.Name,       // queue name
		eventName,    // routing key
		ExchangeName, // exchange
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to bind a queue: %w", err)
	}
	if err := ch.QueueBind(q.Name, queueName, RetryExchangeName, false, nil); err != nil {
		return fmt.Errorf("failed to bind retry return queue: %w", err)
	}

	err = ch.Qos(1, 0, false)
	if err != nil {
		return fmt.Errorf("failed to set QoS: %w", err)
	}
	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack (manual ack for reliability)
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return fmt.Errorf("failed to register a consumer: %w", err)
	}

	go func() {
		for d := range msgs {
			b.handleDurableDelivery(d, queueName, retryQueueName, eventName, handler)
		}
	}()

	if err := b.startDLQConsumerOnce(ch, queueName, eventName); err != nil {
		return fmt.Errorf("start DLQ consumer for %s: %w", eventName, err)
	}

	return nil
}

func (b *RabbitMQEventBus) handleDurableDelivery(d amqp.Delivery, queueName, retryQueueName, eventName string, handler EventHandler) {
	ctx, cancel := newDeliveryContext(d.Headers)
	defer cancel()

	msgID := d.MessageId
	if msgID != "" && b.pgDB != nil {
		var exists int
		err := b.pgDB.QueryServiceRow(
			ctx,
			"SELECT 1 FROM events.processed_events WHERE consumer = $1 AND message_id = $2",
			queueName,
			msgID,
		).Scan(&exists)
		if err == nil {
			log.Printf("Event %s (consumer=%s, ID: %s) already processed, skipping", eventName, queueName, msgID)
			_ = d.Ack(false)
			return
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, pgx.ErrNoRows) {
			log.Printf("Error checking processed event %s (consumer=%s, ID: %s): %v", eventName, queueName, msgID, err)
			b.retryOrDeadLetter(d, retryQueueName, eventName, err)
			return
		}
	}

	var payload interface{}
	if err := json.Unmarshal(d.Body, &payload); err != nil {
		log.Printf("Error unmarshalling event body: %v", err)
		_ = d.Nack(false, false)
		return
	}

	evt := &SimpleEvent{name: d.Type, payload: payload, id: msgID}
	start := time.Now()
	err := handler(ctx, evt)
	metrics.ObserveEventHandlerDuration(eventName, time.Since(start))
	if err != nil {
		log.Printf("Error handling event %s: %v", eventName, err)
		b.retryOrDeadLetter(d, retryQueueName, eventName, err)
		return
	}

	if msgID != "" && b.pgDB != nil {
		var inserted int
		err := b.pgDB.QueryServiceRow(
			ctx,
			"INSERT INTO events.processed_events (consumer, message_id) VALUES ($1, $2) ON CONFLICT (consumer, message_id) DO NOTHING RETURNING 1",
			queueName,
			msgID,
		).Scan(&inserted)
		if errors.Is(err, pgx.ErrNoRows) || errors.Is(err, sql.ErrNoRows) {
			_ = d.Ack(false)
			return
		}
		if err != nil {
			log.Printf("Error recording processed event %s (consumer=%s, ID: %s): %v", eventName, queueName, msgID, err)
			b.retryOrDeadLetter(d, retryQueueName, eventName, err)
			return
		}
		_ = inserted
	}

	_ = d.Ack(false)
}

func newDeliveryContext(headers amqp.Table) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(context.Background(), deliveryHandlerTimeout)
	if tid, ok := headers["tenant_id"].(string); ok && tid != "" {
		ctx = pkgctx.WithTenantID(ctx, tid)
	}
	return ctx, cancel
}

func retryCount(headers amqp.Table) int {
	if headers == nil {
		return 0
	}
	switch value := headers["x-retry-count"].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		if value > uint64(^uint(0)>>1) {
			return handlerMaxRetries
		}
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(value)
		return parsed
	default:
		return 0
	}
}

func handlerRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := handlerRetryBase
	for i := 1; i < attempt && delay < handlerRetryMax; i++ {
		delay *= 2
		if delay > handlerRetryMax {
			return handlerRetryMax
		}
	}
	return delay
}

func copyAMQPHeaders(headers amqp.Table) amqp.Table {
	cloned := make(amqp.Table, len(headers)+2)
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func (b *RabbitMQEventBus) retryOrDeadLetter(d amqp.Delivery, retryQueueName, eventName string, cause error) {
	currentAttempt := retryCount(d.Headers)
	if currentAttempt >= handlerMaxRetries {
		log.Printf("Event %s exhausted %d handler retries; routing to DLQ: %v", eventName, currentAttempt, cause)
		_ = d.Nack(false, false)
		return
	}

	nextAttempt := currentAttempt + 1
	headers := copyAMQPHeaders(d.Headers)
	headers["x-retry-count"] = int32(nextAttempt)
	if cause != nil {
		message := cause.Error()
		if len(message) > 512 {
			message = message[:512]
		}
		headers["x-last-error"] = message
	}
	delay := handlerRetryDelay(nextAttempt)

	publishCtx, cancel := context.WithTimeout(context.Background(), publishConfirmTimeout)
	err := b.publishConfirmed(publishCtx,
		"",
		retryQueueName,
		amqp.Publishing{
			ContentType:   d.ContentType,
			Body:          d.Body,
			Timestamp:     time.Now(),
			Type:          eventName,
			DeliveryMode:  amqp.Persistent,
			Headers:       headers,
			MessageId:     d.MessageId,
			CorrelationId: d.CorrelationId,
			Expiration:    strconv.FormatInt(delay.Milliseconds(), 10),
		})
	cancel()
	if err != nil {
		log.Printf("Failed to schedule retry %d/%d for event %s: %v", nextAttempt, handlerMaxRetries, eventName, err)
		_ = d.Nack(false, true)
		return
	}
	log.Printf("Scheduled retry %d/%d for event %s after %s", nextAttempt, handlerMaxRetries, eventName, delay)
	_ = d.Ack(false)
}

func (b *RabbitMQEventBus) StartDLQConsumers() {
	b.mu.Lock()
	subs := make([]subscription, len(b.subscriptions))
	copy(subs, b.subscriptions)
	b.mu.Unlock()

	for _, sub := range subs {
		b.mu.Lock()
		connected := b.channel != nil
		b.mu.Unlock()
		if !connected {
			return
		}
		if err := b.startDLQConsumer(sub.queueName, sub.eventName); err != nil {
			log.Printf("Failed to start DLQ consumer for %s: %v", sub.eventName, err)
		}
	}
}

// SimpleEvent is a basic implementation of Event interface
type SimpleEvent struct {
	name    string
	payload interface{}
	id      string
}

func (e *SimpleEvent) Name() string {
	return e.name
}

func (e *SimpleEvent) Payload() interface{} {
	return e.payload
}

func (e *SimpleEvent) EventID() string {
	return e.id
}
