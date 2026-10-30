package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/auth"
	amqp "github.com/rabbitmq/amqp091-go"
)

func (b *RabbitMQEventBus) startDLQConsumer(queueName, eventName string) error {
	b.mu.Lock()
	ch := b.channel
	b.mu.Unlock()
	if ch == nil {
		return fmt.Errorf("rabbitmq channel not initialized")
	}
	return b.startDLQConsumerOnce(ch, queueName, eventName)
}

func (b *RabbitMQEventBus) startDLQConsumerOnce(ch *amqp.Channel, queueName, eventName string) error {
	dlqName := queueName + ".dlq"
	generation, claimed, err := b.claimDLQConsumer(ch, dlqName)
	if err != nil || !claimed {
		return err
	}

	if err := b.startDLQConsumerOnChannel(ch, queueName, eventName); err != nil {
		b.releaseDLQConsumerClaim(dlqName, generation)
		return err
	}
	return nil
}

func (b *RabbitMQEventBus) claimDLQConsumer(ch *amqp.Channel, dlqName string) (generation uint64, claimed bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if ch == nil || b.channel != ch || ch.IsClosed() {
		return 0, false, fmt.Errorf("rabbitmq channel generation changed before DLQ consumer start")
	}
	generation = b.generation
	if b.dlqStarted[dlqName] == generation {
		return generation, false, nil
	}
	b.dlqStarted[dlqName] = generation
	return generation, true, nil
}

func (b *RabbitMQEventBus) releaseDLQConsumerClaim(dlqName string, generation uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.dlqStarted[dlqName] == generation {
		delete(b.dlqStarted, dlqName)
	}
}

func (b *RabbitMQEventBus) startDLQConsumerOnChannel(ch *amqp.Channel, queueName, eventName string) error {
	dlqName := queueName + ".dlq"
	msgs, err := ch.Consume(
		dlqName,
		"",    // consumer tag
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		return fmt.Errorf("failed to register dlq consumer: %w", err)
	}

	go func() {
		for d := range msgs {
			func() {
				ctx, cancel := newDeliveryContext(d.Headers)
				defer cancel()

				var payload interface{}
				if err := json.Unmarshal(d.Body, &payload); err != nil {
					log.Printf("DLQ: Error unmarshalling event body from %s: %v", dlqName, err)
					_ = d.Ack(false) // Ack invalid message to discard it
					return
				}

				if b.pgDB != nil {
					tenantID, _ := d.Headers["tenant_id"].(string)
					reason, deathCount := parseXDeath(d.Headers["x-death"])
					if lastError, ok := d.Headers["x-last-error"].(string); ok && lastError != "" {
						reason = lastError
					}
					if attempts := int64(retryCount(d.Headers)); attempts > deathCount {
						deathCount = attempts
					}
					msgID := dlqMessageID(d.MessageId)

					var tid interface{} = nil
					if tenantID != "" {
						tid = tenantID
					}

					query := `
						INSERT INTO events.dlq_events
						(message_id, dlq_name, queue_name, event_name, tenant_id, payload, reason, death_count)
						VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
						ON CONFLICT (message_id, dlq_name)
						DO UPDATE SET death_count = events.dlq_events.death_count + EXCLUDED.death_count, reason = EXCLUDED.reason
					`

					err := b.pgDB.ExecService(ctx, query,
						msgID, dlqName, queueName, eventName, tid, d.Body, reason, deathCount)

					if err != nil {
						log.Printf("DLQ: Failed to save message %s to Postgres: %v", msgID, err)
						_ = d.Nack(false, true)
						return
					}
				}

				log.Printf("DLQ: Logged message %s from %s", d.MessageId, dlqName)
				_ = d.Ack(false)
			}()
		}
	}()

	return nil
}

func dlqMessageID(messageID string) string {
	if messageID != "" {
		return messageID
	}
	// DeliveryTag is scoped to one AMQP channel and resets after reconnect. It
	// cannot be used as a durable identity: two unrelated legacy messages would
	// collide and the second payload would be ACKed without being stored. A new
	// UUID may create a duplicate row after an uncertain ACK, which is the safe
	// side of the at-least-once contract and never destroys another payload.
	return "legacy-" + uuid.NewString()
}

func parseXDeath(xDeath interface{}) (reason string, count int64) {
	if deaths, ok := xDeath.([]interface{}); ok && len(deaths) > 0 {
		if death, ok := deaths[0].(amqp.Table); ok {
			if r, ok := death["reason"].(string); ok {
				reason = r
			}
			if c, ok := death["count"].(int64); ok {
				count = c
			} else if c2, ok := death["count"].(int32); ok {
				count = int64(c2)
			}
		}
	}
	return
}

func (b *RabbitMQEventBus) publishRaw(ctx context.Context, routingKey string, body []byte, tenantID string, msgID string) error {
	headers := amqp.Table{}
	if tenantID != "" {
		headers["tenant_id"] = tenantID
	}

	return b.publishConfirmed(ctx,
		ExchangeName,
		routingKey,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
			Type:         routingKey,
			DeliveryMode: amqp.Persistent,
			Headers:      headers,
			MessageId:    msgID,
		},
	)
}

// dlqReplayLookupQuery busca o payload guardado para replay.
//
// A coluna é `created_at`: a versão anterior filtrava por `received_at`, que não
// existe em events.dlq_events. O erro de SQL era devolvido como 404 "dlq event
// not found", então o endpoint de replay parecia não achar a mensagem mesmo com
// ela na tabela — descoberto ao tentar reprocessar os eventos de assinatura do
// contrato 277b965e (30/09/2026).
const dlqReplayLookupQuery = `
	SELECT event_name, tenant_id::text, payload
	FROM events.dlq_events
	WHERE message_id = $1
	ORDER BY created_at DESC
	LIMIT 1
`

// RegisterDLQAdminRoutes mounts /admin/events/dlq/* operational endpoints.
// The group requires the INTERNAL_API_KEY header (auth.RequireInternalKey()).
// Public path bypass via isPublicEndpoint("/admin/*") was removed — any
// service registering admin routes must apply its own auth at the group
// level. See pkg/auth/middleware.go for context.
func RegisterDLQAdminRoutes(r *gin.Engine, bus *RabbitMQEventBus) {
	if r == nil || bus == nil {
		return
	}

	admin := r.Group("/admin")
	admin.Use(auth.RequireInternalKey())
	admin.POST("/events/dlq/replay/:message_id", func(c *gin.Context) {
		if bus.pgDB == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "dlq storage disabled"})
			return
		}

		messageID := c.Param("message_id")
		if messageID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "message_id required"})
			return
		}

		var eventName string
		var tenantID *string
		var payload []byte

		err := bus.pgDB.QueryServiceRow(c.Request.Context(), dlqReplayLookupQuery, messageID).Scan(&eventName, &tenantID, &payload)
		if err != nil {
			// Erro de SQL NÃO é "não encontrado": foi exatamente essa confusão
			// (received_at inexistente → 404) que fez o replay parecer que a
			// mensagem não estava na tabela.
			if errors.Is(err, sql.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": "dlq event not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		tid := ""
		if tenantID != nil {
			tid = *tenantID
		}

		if err := bus.publishRaw(c.Request.Context(), eventName, payload, tid, messageID); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}

		if err := bus.pgDB.ExecService(c.Request.Context(), `
			UPDATE events.dlq_events
			SET replayed_at = NOW()
			WHERE message_id = $1
		`, messageID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "event published but replay status update failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
}
