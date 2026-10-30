package repositories

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-deal/internal/domain/contract"
)

// Run only against a disposable empty database named toca_contract_regen_qa_*.
// This exercises real JSONB persistence and the concurrent-write guard.
func TestContractRegenerationPostgres(t *testing.T) {
	uri := os.Getenv("CONTRACT_REGEN_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("set CONTRACT_REGEN_TEST_DATABASE_URL to an isolated QA database")
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
	if !strings.HasPrefix(name, "toca_contract_regen_qa_") {
		t.Fatal("refusing non-QA database")
	}
	if _, err := db.Exec(`CREATE SCHEMA deal;
		CREATE TABLE deal.contracts (id text PRIMARY KEY, tenant_id text, deal_id text,
		metadata jsonb NOT NULL, created_at timestamptz DEFAULT now(), deleted_at timestamptz)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.Exec("DROP SCHEMA deal CASCADE"); err != nil {
			t.Errorf("cleanup QA schema: %v", err)
		}
	})
	repo := NewContractPostgresRepository(&database.PostgresDB{DB: db})
	ctx := context.Background()
	// Nine fractional digits catch accidental nanosecond/microsecond truncation.
	ts, _ := time.Parse(time.RFC3339Nano, "2026-09-09T20:00:00.123456789Z")
	c := &contract.Contract{ID: "qa-contract", TenantID: "qa-tenant", DealID: "qa-deal", UpdatedAt: ts, Content: "old", Status: contract.StatusDraft}
	if err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.Content = "new"
	c.DraftRevisions = []contract.DraftRevision{{Content: "old", ReplacedAt: ts, ReplacedBy: "qa"}}
	if err := repo.UpdateIfUnchanged(ctx, c, ts); err != nil {
		t.Fatalf("valid revision rejected: %v", err)
	}
	loaded, err := repo.GetByID(ctx, c.TenantID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Content != "new" || len(loaded.DraftRevisions) != 1 || loaded.DraftRevisions[0].Content != "old" {
		t.Fatal("revision history not persisted")
	}
	c.Content = "stale-overwrite"
	if err := repo.UpdateIfUnchanged(ctx, c, ts); !errors.Is(err, contract.ErrConcurrentModification) {
		t.Fatalf("expected stale conflict, got %v", err)
	}
	c.TenantID = "different-tenant"
	if err := repo.UpdateIfUnchanged(ctx, c, loaded.UpdatedAt); !errors.Is(err, contract.ErrConcurrentModification) {
		t.Fatalf("tenant filter failed: %v", err)
	}
	c.TenantID = loaded.TenantID

	guarded := repo.(interface {
		UpdateIfUnchangedInTx(context.Context, *sqlx.Tx, *contract.Contract, time.Time) error
	})
	pg := &database.PostgresDB{DB: db}
	if _, err := db.Exec("CREATE TABLE deal.signature_outbox_probe (contract_id text)"); err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback after event write")
	c.Status = contract.StatusSigned
	err = pg.RunInTx(ctx, func(tx *sqlx.Tx) error {
		if err := guarded.UpdateIfUnchangedInTx(ctx, tx, c, loaded.UpdatedAt); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO deal.signature_outbox_probe VALUES ($1)", c.ID); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	unchanged, err := repo.GetByID(ctx, c.TenantID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM deal.signature_outbox_probe"); err != nil {
		t.Fatal(err)
	}
	if unchanged.Status == contract.StatusSigned || count != 0 {
		t.Fatal("contract/event did not roll back together")
	}
	if err := pg.RunInTx(ctx, func(tx *sqlx.Tx) error { return guarded.UpdateIfUnchangedInTx(ctx, tx, c, loaded.UpdatedAt) }); err != nil {
		t.Fatal(err)
	}
	if err := pg.RunInTx(ctx, func(tx *sqlx.Tx) error { return guarded.UpdateIfUnchangedInTx(ctx, tx, c, loaded.UpdatedAt) }); !errors.Is(err, contract.ErrConcurrentModification) {
		t.Fatalf("transactional stale revision accepted: %v", err)
	}
	loaded, err = repo.GetByID(ctx, c.TenantID, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("UPDATE deal.contracts SET deleted_at=now() WHERE id=$1", c.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateIfUnchanged(ctx, c, loaded.UpdatedAt); !errors.Is(err, contract.ErrConcurrentModification) {
		t.Fatalf("deleted filter failed: %v", err)
	}
}
