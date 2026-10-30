package repository_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-crm/internal/domain/lead"
	"github.com/oginoino/toca/toca-crm/internal/repository"
)

// This test owns its schema; refuse any application/shared database.
func TestLeadPostgresVisitCAS(t *testing.T) {
	uri := os.Getenv("CRM_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set CRM_REVIEW_TEST_DATABASE_URL to a disposable toca_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "toca_review_qa_") {
		t.Fatal("refusing non-QA database")
	}
	if _, err := db.Exec(`CREATE SCHEMA crm;
		CREATE TABLE crm.leads (id text PRIMARY KEY, tenant_id text NOT NULL, metadata jsonb NOT NULL,
		created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
		updated_seq bigint NOT NULL DEFAULT 0, deleted_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA crm CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})
	ctx := context.Background()
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "sequence", true: "legacy timestamp"}[legacy], func(t *testing.T) {
			if legacy {
				if _, err := db.Exec("ALTER TABLE crm.leads DROP COLUMN updated_seq"); err != nil {
					t.Fatal(err)
				}
			}
			repo := repository.NewLeadPostgresRepository(&database.PostgresDB{DB: db})
			id := "with-sequence"
			if legacy {
				id = "without-sequence"
			}
			l := &lead.Lead{ID: id, TenantID: "tenant", Phone: "original", UpdatedSeq: 999, UpdatedAt: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}
			if err := repo.Create(ctx, l); err != nil {
				t.Fatal(err)
			}
			loaded, err := repo.GetByID(ctx, "tenant", id)
			if err != nil {
				t.Fatal(err)
			}
			if loaded.UpdatedSeq != 0 || !loaded.UpdatedAt.After(l.UpdatedAt) {
				t.Fatal("read used stale metadata version")
			}
			expected := &lead.LeadExpectedVersion{UpdatedSeq: &loaded.UpdatedSeq, UpdatedAt: &loaded.UpdatedAt}
			phoneEdited, ok, err := repo.Patch(ctx, id, "tenant", expected, map[string]interface{}{"Phone": "concurrent-edit"})
			if err != nil || !ok {
				t.Fatalf("valid version failed: %v", err)
			}
			if _, ok, err := repo.Patch(ctx, id, "tenant", expected, map[string]interface{}{"visit_request": &lead.VisitRequest{Status: "Aguardando"}}); err != nil || ok {
				t.Fatalf("stale version was accepted or failed unexpectedly: saved=%v err=%v", ok, err)
			}
			fresh := &lead.LeadExpectedVersion{UpdatedSeq: &phoneEdited.UpdatedSeq, UpdatedAt: &phoneEdited.UpdatedAt}
			updated, ok, err := repo.Patch(ctx, id, "tenant", fresh, map[string]interface{}{"visit_request": &lead.VisitRequest{AppointmentID: "appointment", Status: "Aguardando"}})
			if err != nil || !ok {
				t.Fatalf("returned SQL version cannot be reused: %v", err)
			}
			if updated.Phone != "concurrent-edit" || updated.VisitRequest == nil || updated.VisitRequest.AppointmentID != "appointment" {
				t.Fatal("partial visit update lost lead data")
			}
			op := &lead.VisitOperation{ID: "operation", Token: "lease", Desired: &lead.VisitRequest{AppointmentID: "durable-appointment"}}
			withIntent, ok, err := repo.Patch(ctx, id, "tenant", &lead.LeadExpectedVersion{UpdatedSeq: &updated.UpdatedSeq, UpdatedAt: &updated.UpdatedAt}, map[string]interface{}{"visit_operation": op})
			if err != nil || !ok {
				t.Fatalf("intent reservation: %v", err)
			}
			// A whole-lead update from another workflow must retain the intent and visit.
			stale := *loaded
			stale.Name = "Unrelated edit"
			if err := repo.Update(ctx, &stale); err != nil {
				t.Fatal(err)
			}
			durable, err := repo.GetByID(ctx, "tenant", id)
			if err != nil || durable.VisitOperation == nil || durable.VisitOperation.ID != op.ID || durable.VisitRequest == nil || durable.VisitRequest.AppointmentID != "appointment" {
				t.Fatalf("whole update erased durable state: %v", err)
			}
			workerDB, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri, MaxPoolSize: 2})
			if err != nil {
				t.Fatal(err)
			}
			pending, scanErr := repository.NewLeadPostgresRepository(workerDB).ListPendingVisitOperations(ctx, 100)
			workerDB.Close()
			if scanErr != nil || len(pending) != 1 || pending[0].ID != id || pending[0].TenantID != "tenant" {
				t.Fatalf("recovery scan: %v, count=%d", scanErr, len(pending))
			}
			if _, ok, err := repo.Patch(ctx, id, "tenant", &lead.LeadExpectedVersion{UpdatedSeq: &withIntent.UpdatedSeq, UpdatedAt: &withIntent.UpdatedAt}, map[string]interface{}{"visit_operation": nil}); err != nil || ok {
				t.Fatal("stale lease cleared newer state")
			}
			if _, ok, err := repo.Patch(ctx, id, "tenant", &lead.LeadExpectedVersion{UpdatedSeq: &durable.UpdatedSeq, UpdatedAt: &durable.UpdatedAt}, map[string]interface{}{"visit_operation": nil}); err != nil || !ok {
				t.Fatalf("intent completion: %v", err)
			}
			if _, ok, err := repo.Patch(ctx, id, "other-tenant", nil, map[string]interface{}{"Phone": "forged"}); err != nil || ok {
				t.Fatal("tenant scope not enforced")
			}
			if _, err := db.Exec("UPDATE crm.leads SET deleted_at=now() WHERE id=$1", id); err != nil {
				t.Fatal(err)
			}
			if _, ok, err := repo.Patch(ctx, id, "tenant", nil, map[string]interface{}{"Phone": "deleted"}); err != nil || ok {
				t.Fatal("deleted lead changed")
			}
		})
	}
}
