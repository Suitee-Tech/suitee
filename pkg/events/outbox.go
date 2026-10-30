package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	pkgctx "github.com/oginoino/toca/pkg/context"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/metrics"
)

// MaxOutboxAttempts caps the number of publish retries before the drainer
// moves an outbox row to status='FAILED'. With exponential backoff (see
// outboxBackoff) attempts 0..9 cover ~1s+2s+...+30min before giving up,
// well past any transient broker outage. Once FAILED, the row must be
// inspected and replayed manually (clear next_attempt_at + reset to
// PENDING) — a known-bad payload should not be retried forever.
const MaxOutboxAttempts = 10

// outboxBackoff returns the delay before the next attempt for a given
// attempt count. Capped at 30 minutes so very high attempt counts still
// retry within a reasonable window after the cap is lifted.
func outboxBackoff(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	if attempts > 20 {
		attempts = 20
	}
	delay := time.Duration(1<<attempts) * time.Second
	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}
	return delay
}

type outboxEventDoc struct {
	ID          string     `json:"id"`
	Event       string     `json:"event"`
	TenantID    string     `json:"tenant_id,omitempty"`
	Payload     []byte     `json:"payload"`
	CreatedAt   time.Time  `json:"created_at"`
	PublishedAt *time.Time `json:"published_at,omitempty"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
}

// PublishReliable persists the event to events.outbox_events and lets the
// background drainer (startOutboxPublisher) publish it asynchronously. If the
// outbox is not configured (pgDB == nil), falls back to direct Publish so
// callers in lightweight setups still get delivery.
//
// PublishReliable uses the default Postgres pool — it is NOT joined to any
// transaction that the caller might have open. If the business write
// commits but PublishReliable fails right after (or vice-versa), the
// system briefly diverges. For atomicity, use EnqueueOutboxInTx inside a
// pgDB.RunInTx callback.
func (b *RabbitMQEventBus) PublishReliable(ctx context.Context, event Event) error {
	if b == nil {
		return fmt.Errorf("event bus is nil")
	}
	if b.pgDB == nil {
		return b.Publish(ctx, event)
	}
	_, err := b.EnqueueOutbox(ctx, event)
	return err
}

// EnqueueOutboxInTx inserts the outbox row using the caller-supplied
// transaction so the publish becomes atomic with the business write that
// shares the same tx. Returns the generated outbox row id.
//
// Use this from inside a pgDB.RunInTx callback:
//
//	err := pgDB.RunInTx(ctx, func(tx *sqlx.Tx) error {
//	    if _, err := tx.ExecContext(ctx, "UPDATE ..."); err != nil { return err }
//	    if _, err := bus.EnqueueOutboxInTx(ctx, tx, evt); err != nil { return err }
//	    return nil
//	})
//
// On commit, both the business row and the outbox row land together. If
// either fails, the whole tx rolls back — no ghost events, no missed
// events. The background drainer then picks up the outbox row and
// publishes to RabbitMQ asynchronously.
//
// See docs/architecture/data-consistency-audit.md §8.1 for context.
func (b *RabbitMQEventBus) EnqueueOutboxInTx(ctx context.Context, tx *sqlx.Tx, event Event) (string, error) {
	if b == nil {
		return "", fmt.Errorf("event bus is nil")
	}
	return EnqueueOutboxInTxService(ctx, tx, b.pgDB, event)
}

// outboxInsertQuery é o INSERT canônico em events.outbox_events — usado tanto
// pelo caminho in-tx (pool de serviço) quanto pelo caminho adiado pós-commit.
const outboxInsertQuery = `
	INSERT INTO events.outbox_events (tenant_id, event_name, payload, status)
	VALUES ($1, $2, $3, 'PENDING')
	RETURNING id
`

// EnqueueOutboxInTx persists an event even when a broker connection is offline.
// Only a database transaction is required; the drainer delivers it later.
//
// ATENÇÃO RLS: events.outbox_events é deny-by-default para roles sem
// BYPASSRLS (ver .no-rls-allowlist.txt). Esta função escreve NA transação
// recebida e por isso só é segura quando o caller já está numa transação do
// pool de serviço (RunInServiceContext). Para transações do pool de aplicação,
// use EnqueueOutboxInTxService, que adia a escrita para depois do commit via
// pool de serviço.
func EnqueueOutboxInTx(ctx context.Context, tx *sqlx.Tx, event Event) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("transaction is nil; use EnqueueOutbox for non-tx callers")
	}
	if event == nil {
		return "", fmt.Errorf("event is nil")
	}

	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return "", fmt.Errorf("failed to marshal event payload: %w", err)
	}

	tenantID := tenantIDForOutbox(ctx, event)

	var msgID string
	var tid interface{} = nil
	if tenantID != "" {
		tid = tenantID
	}
	if err := tx.QueryRowContext(ctx, outboxInsertQuery, tid, event.Name(), string(payloadBytes)).Scan(&msgID); err != nil {
		return "", fmt.Errorf("failed to insert outbox event in tx: %w", err)
	}
	return msgID, nil
}

// EnqueueOutboxInTxService enfileira o evento de forma atômica com a escrita
// de negócio que compartilha a transação `tx`, escrevendo a linha do outbox no
// pool CORRETO sob o modelo de RLS da plataforma:
//
//   - transação do pool de serviço (RunInServiceContext): o INSERT entra na
//     própria transação — atômico e sem RLS (BYPASSRLS);
//   - transação do pool de aplicação (RunInTx): events.outbox_events é
//     deny-by-default, então o INSERT é ADIADO para imediatamente depois do
//     commit e executado pelo pool de serviço. Rollback descarta o callback —
//     a semântica "sem ghost events, sem missed events" é preservada, com a
//     única janela de risco sendo uma queda do processo entre commit e insert
//     (a mesma degradação documentada de PublishReliable).
//   - transação não gerenciada (sem registro de commit): mantém o write in-tx
//     legado (testes sem RLS).
func EnqueueOutboxInTxService(ctx context.Context, tx *sqlx.Tx, db *database.PostgresDB, event Event) (string, error) {
	if tx == nil {
		return "", fmt.Errorf("transaction is nil; use EnqueueOutbox for non-tx callers")
	}
	if event == nil {
		return "", fmt.Errorf("event is nil")
	}
	if db == nil || db.ServicePool == nil {
		return EnqueueOutboxInTx(ctx, tx, event)
	}

	// Transação do pool de serviço: write in-tx direto.
	if database.ServiceTxFromCtx(ctx) {
		return EnqueueOutboxInTx(ctx, tx, event)
	}

	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return "", fmt.Errorf("failed to marshal event payload: %w", err)
	}
	tenantID := tenantIDForOutbox(ctx, event)

	if database.DeferTxUntilCommit(tx, func() {
		actx, cancel := context.WithTimeout(context.Background(), postCommitOutboxTimeout)
		defer cancel()
		var tid interface{} = nil
		if tenantID != "" {
			tid = tenantID
		}
		var msgID string
		if err := db.ServicePool.QueryRow(actx, outboxInsertQuery, tid, event.Name(), string(payloadBytes)).Scan(&msgID); err != nil {
			log.Printf("outbox deferred insert failed after commit (event=%s): %v", event.Name(), err)
		}
	}) {
		// Enfileirado: a linha entra no pool de serviço logo após o commit.
		return "", nil
	}

	// Transação fora do registro gerenciado (ex.: testes): comportamento legado.
	return EnqueueOutboxInTx(ctx, tx, event)
}

// postCommitOutboxTimeout limita o insert adiado do outbox: o commit já
// aconteceu, então uma indisponibilidade do pool de serviço não pode pendurar
// a resposta do caller indefinidamente.
const postCommitOutboxTimeout = 5 * time.Second

// tenantIDForOutbox extracts the tenant id used for the outbox row,
// preferring the context value over the payload's tenant_id field. Kept
// in a helper so EnqueueOutbox and EnqueueOutboxInTx share the same
// resolution rules.
func tenantIDForOutbox(ctx context.Context, event Event) string {
	if tid, err := pkgctx.GetTenantID(ctx); err == nil && tid != "" {
		return tid
	}
	if payloadMap, ok := event.Payload().(map[string]interface{}); ok {
		if tid, ok := payloadMap["tenant_id"].(string); ok {
			return tid
		}
	}
	return ""
}

func (b *RabbitMQEventBus) EnqueueOutbox(ctx context.Context, event Event) (string, error) {
	if b == nil || b.pgDB == nil {
		return "", fmt.Errorf("outbox not configured or pgDB is nil")
	}
	if event == nil {
		return "", fmt.Errorf("event is nil")
	}
	if b.pgDB.ServicePool == nil {
		return "", fmt.Errorf("outbox service pool is not configured")
	}

	payloadBytes, err := json.Marshal(event.Payload())
	if err != nil {
		return "", fmt.Errorf("failed to marshal event payload: %w", err)
	}

	tenantID := tenantIDForOutbox(ctx, event)

	var msgID string
	var tid interface{} = nil
	if tenantID != "" {
		tid = tenantID
	}

	// RLS: events.outbox_events é deny-by-default para roles sem BYPASSRLS —
	// a escrita do outbox SEMPRE sai pelo pool de serviço (ver
	// .no-rls-allowlist.txt), nunca pelo pool de aplicação.
	err = b.pgDB.ServicePool.QueryRow(ctx, outboxInsertQuery, tid, event.Name(), string(payloadBytes)).Scan(&msgID)
	if err != nil {
		return "", fmt.Errorf("failed to insert outbox event: %w", err)
	}

	return msgID, nil
}

func (b *RabbitMQEventBus) startOutboxPublisher() {
	if b == nil || b.pgDB == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-b.closeCh:
				return
			case <-ticker.C:
				b.drainOutboxOncePG()
			}
		}
	}()
}

func (b *RabbitMQEventBus) drainOutboxOncePG() {
	b.mu.Lock()
	ch := b.channel
	b.mu.Unlock()
	if ch == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pulls only rows whose backoff window has elapsed and whose attempt
	// budget hasn't been exhausted. Legacy rows (next_attempt_at NULL) match
	// the NULL branch and behave like before.
	query := `
		SELECT id, tenant_id, event_name, payload, attempts
		FROM events.outbox_events
		WHERE status = 'PENDING'
		  AND attempts < $1
		  AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
		ORDER BY created_at ASC
		LIMIT 50
	`

	rows, err := b.pgDB.QueryServiceRows(ctx, query, MaxOutboxAttempts)
	if err != nil {
		log.Printf("Outbox: find failed: %v", err)
		return
	}
	defer rows.Close()

	var pending []outboxEventDoc
	for rows.Next() {
		var doc outboxEventDoc
		var tid *string
		if err := rows.Scan(&doc.ID, &tid, &doc.Event, &doc.Payload, &doc.Attempts); err != nil {
			log.Printf("Outbox: scan failed: %v", err)
			continue
		}
		if tid != nil {
			doc.TenantID = *tid
		}
		pending = append(pending, doc)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		log.Printf("Outbox: row iteration failed: %v", err)
		return
	}
	// Release the query connection before executing updates or gauge queries.
	// With DATABASE_MAX_POOL_SIZE=1, leaving rows open across the loop below
	// makes the drainer wait on its own connection.
	rows.Close()

	for _, doc := range pending {
		// A confirm wait is bounded per message. Reusing the 10s query context for
		// the entire batch would make one slow confirmation expire the context and
		// incorrectly burn an attempt on every remaining row.
		attemptCtx, attemptCancel := context.WithTimeout(context.Background(), publishConfirmTimeout+5*time.Second)
		err := b.publishRaw(attemptCtx, doc.Event, doc.Payload, doc.TenantID, doc.ID)
		if err == nil {
			_ = b.pgDB.ExecService(attemptCtx, `
				UPDATE events.outbox_events
				SET status = 'PUBLISHED', published_at = NOW(), last_error = NULL, next_attempt_at = NULL
				WHERE id = $1
			`, doc.ID)
			attemptCancel()
			continue
		}

		metrics.IncOutboxPublishError(doc.Event)

		nextAttempts := doc.Attempts + 1
		if nextAttempts >= MaxOutboxAttempts {
			// Final attempt — mark FAILED so the drainer stops picking it
			// up. Operators (or a future replay tool) must re-enable it
			// manually after fixing the root cause.
			_ = b.pgDB.ExecService(attemptCtx, `
				UPDATE events.outbox_events
				SET status = 'FAILED', attempts = $1, last_error = $2, next_attempt_at = NULL
				WHERE id = $3
			`, nextAttempts, err.Error(), doc.ID)
			attemptCancel()
			metrics.IncOutboxFailed(doc.Event)
			log.Printf("Outbox: event %s/%s exhausted retries after %d attempts, marked FAILED: %v",
				doc.Event, doc.ID, nextAttempts, err)
			continue
		}

		// Schedule the next attempt with exponential backoff.
		backoffSec := int(outboxBackoff(nextAttempts).Seconds())
		_ = b.pgDB.ExecService(attemptCtx, `
			UPDATE events.outbox_events
			SET attempts = $1,
			    last_error = $2,
			    next_attempt_at = NOW() + ($3 * INTERVAL '1 second')
			WHERE id = $4
		`, nextAttempts, err.Error(), backoffSec, doc.ID)
		attemptCancel()
	}

	refreshCtx, refreshCancel := context.WithTimeout(context.Background(), 10*time.Second)
	b.refreshOutboxAndDLQGauges(refreshCtx)
	refreshCancel()
}

// refreshOutboxAndDLQGauges keeps the Prometheus gauges in sync with the
// current state of events.outbox_events and events.dlq_events. Cheap aggregate
// queries — runs once per drainer cycle (default 2s) which is well under
// scrape interval. Best-effort: failures are logged and the previous gauge
// value is preserved.
func (b *RabbitMQEventBus) refreshOutboxAndDLQGauges(ctx context.Context) {
	if b == nil || b.pgDB == nil {
		return
	}
	var pending, failed, dlqPending int64
	if err := b.pgDB.QueryServiceRow(ctx,
		`SELECT
		    COUNT(*) FILTER (WHERE status = 'PENDING'),
		    COUNT(*) FILTER (WHERE status = 'FAILED')
		FROM events.outbox_events`,
	).Scan(&pending, &failed); err != nil {
		log.Printf("Outbox: failed to refresh outbox gauges: %v", err)
	} else {
		metrics.SetOutboxPending(pending)
		metrics.SetOutboxFailedGauge(failed)
	}

	if err := b.pgDB.QueryServiceRow(ctx,
		`SELECT COUNT(*) FROM events.dlq_events WHERE replayed_at IS NULL`,
	).Scan(&dlqPending); err != nil {
		log.Printf("Outbox: failed to refresh DLQ gauge: %v", err)
	} else {
		metrics.SetDLQPending(dlqPending)
	}
}
