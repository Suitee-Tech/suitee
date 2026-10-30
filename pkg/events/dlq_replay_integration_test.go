package events

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"

	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
)

// Integração (roda só com RLS_PROBE_URI_APP/RLS_PROBE_URI_SERVICE definidos):
// a query de replay precisa ser SQL válido contra o schema real.
//
// Ela filtrava por `received_at`, coluna que não existe em events.dlq_events. O
// erro do Postgres era traduzido para 404 "dlq event not found" no endpoint, e
// o operador concluía que a mensagem não estava na tabela — foi o que
// aconteceu ao tentar reprocessar os eventos de assinatura do contrato
// 277b965e em 30/09/2026. Com a coluna errada este teste falha com o erro de
// SQL em vez de devolver no-rows.
func TestDLQReplayLookupQueryMatchesSchema(t *testing.T) {
	svcURI := os.Getenv("RLS_PROBE_URI_SERVICE")
	appURI := os.Getenv("RLS_PROBE_URI_APP")
	if svcURI == "" || appURI == "" {
		t.Skip("defina RLS_PROBE_URI_APP e RLS_PROBE_URI_SERVICE")
	}
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: appURI, ServiceURI: svcURI})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const messageID = "00000000-0000-4000-8000-00000000d1a1"
	if err := db.ExecService(ctx, `DELETE FROM events.dlq_events WHERE message_id = $1`, messageID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = db.ExecService(ctx, `DELETE FROM events.dlq_events WHERE message_id = $1`, messageID)
	}()

	if err := db.ExecService(ctx, `
		INSERT INTO events.dlq_events (message_id, dlq_name, queue_name, event_name, tenant_id, payload, reason, death_count)
		VALUES ($1, 'test.dlq', 'test', 'signature.signer_signed', NULL, $2::bytea, 'erro de teste', 5)`,
		messageID, `{"contract_id":"277b965e-d0d6-43db-992b-d4c49f0bc455"}`); err != nil {
		t.Fatal(err)
	}

	var eventName string
	var tenantID *string
	var payload []byte
	if err := db.QueryServiceRow(ctx, dlqReplayLookupQuery, messageID).Scan(&eventName, &tenantID, &payload); err != nil {
		t.Fatalf("query de replay falhou (coluna inexistente?): %v", err)
	}
	if eventName != "signature.signer_signed" {
		t.Errorf("event_name = %q, want signature.signer_signed", eventName)
	}
	if len(payload) == 0 {
		t.Error("payload vazio: o replay republicaria um corpo vazio")
	}

	// id desconhecido continua sendo no-rows (404), não erro de SQL.
	err = db.QueryServiceRow(ctx, dlqReplayLookupQuery, "00000000-0000-4000-8000-000000000000").Scan(&eventName, &tenantID, &payload)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("id desconhecido devolveu %v, want no-rows", err)
	}
}
