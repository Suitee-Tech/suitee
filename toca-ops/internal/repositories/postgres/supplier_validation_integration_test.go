package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-ops/internal/models/supplier"
)

func TestSupplierDuplicateNormalizedContacts(t *testing.T) {
	uri := os.Getenv("SUPPLIER_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("requires disposable toca_supplier_review_qa_* database")
	}
	db, err := sqlx.Connect("pgx", uri)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err := db.Get(&name, "SELECT current_database()"); err != nil || !strings.HasPrefix(name, "toca_supplier_review_qa_") {
		t.Fatal("refusing non-QA database")
	}
	if _, err := db.Exec(`CREATE SCHEMA ops;
CREATE TABLE ops.suppliers(id uuid PRIMARY KEY, tenant_id uuid NOT NULL, metadata jsonb NOT NULL, created_at timestamptz DEFAULT now(), deleted_at timestamptz);`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA ops CASCADE")
	repo := NewSupplierRepository(&database.PostgresDB{DB: db})
	tenant := uuid.NewString()
	if found, err := repo.GetByID(context.Background(), tenant, uuid.NewString()); found != nil || !errors.Is(err, supplier.ErrNotFound) {
		t.Fatalf("missing supplier should return not found: %+v, %v", found, err)
	}
	for _, phone := range []string{"(11) 98888-7777", "+55 (11) 98888-7777", "11988887777", "+5511988887777"} {
		t.Run(phone, func(t *testing.T) {
			v := supplier.Supplier{ID: uuid.NewString(), TenantID: tenant, Name: "Ana", Phone: phone, Email: " Ana@Example.com ", Document: "529.982.247-25"}
			metadata, err := json.Marshal(v)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO ops.suppliers(id,tenant_id,metadata) VALUES($1,$2,$3)", v.ID, tenant, string(metadata)); err != nil {
				t.Fatal(err)
			}
			defer db.Exec("DELETE FROM ops.suppliers WHERE id=$1", v.ID)
			for _, contact := range [][3]string{{"52998224725", "", ""}, {"", "+5511988887777", ""}, {"", "", "ana@example.com"}, {"52998224725", "+5511988887777", "ana@example.com"}} {
				found, err := repo.FindDuplicate(context.Background(), tenant, contact[0], contact[1], contact[2])
				if err != nil || found == nil || found.ID != v.ID {
					t.Fatalf("missing duplicate for %v: %+v, %v", contact, found, err)
				}
				foreign, err := repo.FindDuplicate(context.Background(), uuid.NewString(), contact[0], contact[1], contact[2])
				if err != nil || foreign != nil {
					t.Fatalf("tenant leak: %+v, %v", foreign, err)
				}
			}
		})
	}
}
