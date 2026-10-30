//go:build integration

package events

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
)

// Regressão do incidente "500 ao criar lead": events.outbox_events ficou
// deny-by-default sob RLS e as escritas do outbox continuavam saindo pelo pool
// de aplicação. Este teste prova o roteamento corrigido: com um role de
// aplicação SEM BYPASSRLS e um pool de serviço COM BYPASSRLS, a linha do outbox
// só pode entrar via pool de serviço — inclusive quando enfileirada dentro de
// uma transação do pool de aplicação (adiada para depois do commit).
//
// Rode com banco descartável:
//
//	DATABASE_URI_TEST=postgres://toca_rls_app:toca@localhost:54322/toca_review_qa_rls?sslmode=disable \
//	DATABASE_URI_SERVICE_TEST=postgres://postgres:postgres@localhost:54322/toca_review_qa_rls?sslmode=disable \
//	go test -tags=integration ./pkg/events/ -run TestOutboxWriteRoutesThroughServicePool
func TestOutboxWriteRoutesThroughServicePool(t *testing.T) {
	uri := strings.TrimSpace(os.Getenv("DATABASE_URI_TEST"))
	serviceURI := strings.TrimSpace(os.Getenv("DATABASE_URI_SERVICE_TEST"))
	if uri == "" || serviceURI == "" {
		t.Skip("set DATABASE_URI_TEST (sem BYPASSRLS) e DATABASE_URI_SERVICE_TEST (BYPASSRLS)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri, ServiceURI: serviceURI})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	if backstop := db.RLSBackstop(); backstop.UserRoleBypassesRLS {
		t.Fatalf("DATABASE_URI_TEST role bypassa RLS; o teste exige role de aplicação sem BYPASSRLS (%s)", uri)
	}

	// Schema mínimo: tabela de negócio (deny-by-default no pool de aplicação
	// também, para provar que a escrita de negócio usa o tx) + outbox.
	if _, err := db.ServicePool.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS events`); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := db.ServicePool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS events.outbox_events (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
			tenant_id uuid NULL,
			event_name text NOT NULL,
			payload jsonb NOT NULL,
			status text NOT NULL DEFAULT 'PENDING',
			attempts int NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT now(),
			next_attempt_at timestamptz NULL,
			published_at timestamptz NULL,
			last_error text NULL
		)`); err != nil {
		t.Fatalf("create outbox table: %v", err)
	}
	if _, err := db.ServicePool.Exec(ctx, `ALTER TABLE events.outbox_events ENABLE ROW LEVEL SECURITY`); err != nil {
		t.Fatalf("enable RLS on outbox: %v", err)
	}
	if _, err := db.ServicePool.Exec(ctx, `DELETE FROM events.outbox_events`); err != nil {
		t.Fatalf("clean outbox: %v", err)
	}

	evt := &testOutboxEvent{name: "lead.created", payload: map[string]interface{}{"tenant_id": "00000000-0000-0000-0000-000000000001"}}

	// Caminho não-transacional: EnqueueOutbox direto no pool de serviço.
	bus := &RabbitMQEventBus{pgDB: db}
	if _, err := bus.EnqueueOutbox(ctx, evt); err != nil {
		t.Fatalf("EnqueueOutbox via service pool: %v", err)
	}

	// Caminho transacional no pool de aplicação: o INSERT é adiado para depois
	// do commit e sai pelo pool de serviço.
	if err := db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT 1`); err != nil {
			return err
		}
		_, err := EnqueueOutboxInTxService(ctx, tx, db, &testOutboxEvent{name: "deal.created", payload: map[string]interface{}{}})
		return err
	}); err != nil {
		t.Fatalf("RunInTx with deferred outbox: %v", err)
	}

	var count int
	if err := db.ServicePool.QueryRow(ctx, `SELECT count(*) FROM events.outbox_events WHERE event_name = ANY($1::text[])`, []string{"lead.created", "deal.created"}).Scan(&count); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("outbox rows = %d, want 2 (lead.created via service pool + deal.created deferred)", count)
	}

	// Rollback descarta a linha adiada: nada de ghost events.
	if err := db.RunInTx(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.ExecContext(ctx, `SELECT 1`); err != nil {
			return err
		}
		_, err := EnqueueOutboxInTxService(ctx, tx, db, &testOutboxEvent{name: "rollback.me", payload: map[string]interface{}{}})
		if err != nil {
			return err
		}
		return errRollbackForTest
	}); err == nil {
		t.Fatal("RunInTx deveria ter falhado com rollback")
	}

	var rollbackCount int
	if err := db.ServicePool.QueryRow(ctx, `SELECT count(*) FROM events.outbox_events WHERE event_name = 'rollback.me'`).Scan(&rollbackCount); err != nil {
		t.Fatalf("count rollback rows: %v", err)
	}
	if rollbackCount != 0 {
		t.Fatalf("ghost outbox row after rollback: %d", rollbackCount)
	}
}

var errRollbackForTest = &testRollbackError{}

type testRollbackError struct{}

func (e *testRollbackError) Error() string { return "rollback on purpose" }

type testOutboxEvent struct {
	name    string
	payload interface{}
}

func (e *testOutboxEvent) Name() string         { return e.name }
func (e *testOutboxEvent) Payload() interface{} { return e.payload }
func (e *testOutboxEvent) EventID() string      { return e.name }
