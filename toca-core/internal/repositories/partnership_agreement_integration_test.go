package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca-core/internal/domain/core/partnership"
	"github.com/oginoino/toca/pkg/database"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestPartnershipAtomicPersistence(t *testing.T) {
	uri := os.Getenv("PARTNERSHIP_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("requires disposable toca_partnership_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil || !strings.HasPrefix(name, "toca_partnership_review_qa_") {
		t.Fatal("refusing non-QA database")
	}
	_, err = db.Exec(`CREATE SCHEMA core; CREATE SCHEMA toca_auth; CREATE SCHEMA events;
 CREATE TABLE toca_auth.accounts(id uuid PRIMARY KEY, metadata jsonb NOT NULL);
 CREATE TABLE core.partnerships(id uuid PRIMARY KEY,requester_id uuid,target_id uuid,status text,metadata jsonb,created_at timestamptz DEFAULT now(),updated_at timestamptz DEFAULT now(),deleted_at timestamptz);
 CREATE TABLE events.outbox_events(id uuid DEFAULT gen_random_uuid(),tenant_id uuid,event_name text,payload jsonb,status text);`)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA core,toca_auth,events CASCADE")
	a, b := uuid.NewString(), uuid.NewString()
	for _, id := range []string{a, b} {
		if _, err := db.Exec(`INSERT INTO toca_auth.accounts VALUES($1,'{"Type":"company","Company":{"Name":"Parceiro QA","TaxID":"123","IsActive":true}}')`, id); err != nil {
			t.Fatal(err)
		}
	}
	repo := NewPartnershipPostgresRepository(&database.PostgresDB{DB: db})
	ctx := context.Background()
	makeP := func(a, b string) *partnership.Partnership {
		return &partnership.Partnership{ID: uuid.NewString(), RequesterID: a, TargetID: b, Status: "pending", Type: "partner", Version: 1, EventAction: "invited", AuditLogs: []partnership.AuditLog{{Action: "created"}}}
	}
	p, q := makeP(a, b), makeP(b, a)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, p := range []*partnership.Partnership{p, q} {
		wg.Add(1)
		go func(p *partnership.Partnership) { defer wg.Done(); results <- repo.Create(ctx, p) }(p)
	}
	wg.Wait()
	close(results)
	success, duplicate := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, partnership.ErrDuplicate) {
			duplicate++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatalf("simultaneous opposite invitations: success=%d duplicate=%d", success, duplicate)
	}
	list, err := repo.List(ctx, nil)
	if err != nil || len(list) != 1 {
		t.Fatal(list, err)
	}
	p = list[0]
	if p.Companies[a].Name != "Parceiro QA" || p.Companies[b].Name != "Parceiro QA" {
		t.Fatal("public identities missing")
	}
	staleBytes, _ := json.Marshal(p)
	var stale partnership.Partnership
	json.Unmarshal(staleBytes, &stale)
	if err := repo.AddAuditLog(ctx, a, p.ID, partnership.AuditLog{Action: "shared_portfolio_viewed"}); err != nil {
		t.Fatal(err)
	}
	p.Version++
	p.Status = "active"
	p.EventAction = "accepted"
	p.AuditLogs = append(p.AuditLogs, partnership.AuditLog{Action: "accept", UserID: "reviewer", TenantID: b, Version: 2})
	if err := repo.Update(ctx, b, p); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.GetByID(ctx, p.ID)
	if err != nil || len(stored.AuditLogs) != 3 {
		t.Fatalf("concurrent access audit lost: %+v %v", stored, err)
	}
	stale.Version++
	stale.AuditLogs = append(stale.AuditLogs, partnership.AuditLog{Action: "save"})
	if err := repo.Update(ctx, a, &stale); !errors.Is(err, partnership.ErrConflict) {
		t.Fatal("stale update accepted", err)
	}
	var count int
	db.Get(&count, "SELECT count(*) FROM events.outbox_events")
	if count != 4 {
		t.Fatalf("unexpected outbox count %d", count)
	}
	if _, err := db.Exec(`ALTER TABLE events.outbox_events ADD CONSTRAINT reject_new_events CHECK(status='REJECTED') NOT VALID`); err != nil {
		t.Fatal(err)
	}
	stored.Version++
	stored.Status = "inactive"
	stored.EventAction = "changed"
	stored.AuditLogs = append(stored.AuditLogs, partnership.AuditLog{Action: "terminate"})
	if err := repo.Update(ctx, a, stored); err == nil {
		t.Fatal("expected outbox failure")
	}
	after, err := repo.GetByID(ctx, p.ID)
	if err != nil || after.Status != "active" || after.Version != 2 || len(after.AuditLogs) != 3 {
		t.Fatalf("outbox failure did not roll back agreement: %+v %v", after, err)
	}
}
