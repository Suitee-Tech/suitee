package postgres

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-ops/internal/models/inspection"
	"os"
	"strings"
	"testing"
	"time"
)

func TestInspectionCalendarAtomicOutbox(t *testing.T) {
	uri := os.Getenv("INSPECTION_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("requires disposable toca_inspection_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil || !strings.HasPrefix(name, "toca_inspection_review_qa_") {
		t.Fatal("refusing non-QA database")
	}
	_, err = db.Exec(`CREATE SCHEMA ops; CREATE SCHEMA events;
 CREATE TABLE ops.inspections(id uuid PRIMARY KEY, tenant_id uuid NOT NULL, metadata jsonb NOT NULL, created_at timestamptz DEFAULT now(), updated_at timestamptz DEFAULT now(), deleted_at timestamptz);
 CREATE TABLE events.outbox_events(id uuid DEFAULT gen_random_uuid() PRIMARY KEY, tenant_id uuid, event_name text NOT NULL, payload jsonb NOT NULL, status text NOT NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA ops CASCADE; DROP SCHEMA events CASCADE")
	repo := NewInspectionRepository(&database.PostgresDB{DB: db})
	ctx := context.Background()
	i := inspection.New(uuid.NewString(), uuid.NewString(), "ENTRY", uuid.NewString(), "Ana", uuid.NewString(), time.Now().Add(time.Hour))
	i.TimeZone = "America/Sao_Paulo"
	if err := repo.Create(ctx, i); err != nil {
		t.Fatal(err)
	}
	i.Status = inspection.StatusConcluida
	if err := repo.Update(ctx, i); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, uuid.NewString(), i.ID); err != nil {
		t.Fatal(err)
	}
	if existing, err := repo.GetByID(ctx, i.TenantID, i.ID); err != nil || existing == nil {
		t.Fatal("foreign tenant deleted inspection")
	}
	if err := repo.Delete(ctx, i.TenantID, i.ID); err != nil {
		t.Fatal(err)
	}
	var payloads [][]byte
	if err := db.Select(&payloads, "SELECT payload FROM events.outbox_events ORDER BY (payload->>'updated_at')::timestamptz"); err != nil {
		t.Fatal(err)
	}
	if len(payloads) != 3 {
		t.Fatalf("want 3 snapshots, got %d", len(payloads))
	}
	for index, status := range []string{inspection.StatusAgendada, inspection.StatusConcluida, inspection.StatusCancelada} {
		var p contracts.InspectionCalendarPayload
		if err := json.Unmarshal(payloads[index], &p); err != nil {
			t.Fatal(err)
		}
		if p.Status != status || p.InspectorID != i.InspectorID || p.ContractID != i.ContractID {
			t.Fatalf("bad snapshot %+v", p)
		}
	}
	// Force an outbox failure; creating the inspection must roll back too.
	if _, err := db.Exec("ALTER TABLE events.outbox_events ADD CONSTRAINT reject_write CHECK (status <> 'PENDING') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	next := inspection.New(i.PropertyID, i.ContractID, "ENTRY", i.InspectorID, i.InspectorName, i.TenantID, i.ScheduledDate)
	if err := repo.Create(ctx, next); err == nil {
		t.Fatal("accepted failed enqueue")
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM ops.inspections WHERE id=$1", next.ID); err != nil || count != 0 {
		t.Fatal("inspection committed without outbox", err)
	}
}
