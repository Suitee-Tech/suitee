package repositories

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-deal/internal/domain/deal"
	"os"
	"strings"
	"testing"
)

func TestDealRevalidatesSharingWhenPersisted(t *testing.T) {
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
	if _, err := db.Exec(`CREATE SCHEMA deal;CREATE SCHEMA core;CREATE SCHEMA crm;
 CREATE TABLE deal.deals(id uuid,tenant_id uuid,metadata jsonb,created_at timestamptz DEFAULT now());
 CREATE TABLE core.partnerships(id uuid,status text,metadata jsonb,deleted_at timestamptz);
 CREATE TABLE core.properties(id uuid,tenant_id uuid,metadata jsonb,deleted_at timestamptz);
 CREATE TABLE crm.leads(id uuid,tenant_id uuid,metadata jsonb,deleted_at timestamptz);`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA deal,core,crm CASCADE")
	owner, closing, id, propertyID, leadID := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
	grant := access.Grant{Permissions: []access.Permission{{Scope: "shared_portfolio", AccessLevel: "view"}, {Scope: "leads", AccessLevel: "view"}}, Sharing: access.Sharing{AllowCoBrokering: true, SharedPropertyTypes: []string{"house"}, SharedRegions: []string{"Recife"}}}
	p := CrossPartnership{ID: id, RequesterID: owner, TargetID: closing, Status: "active", FinancialVersion: 1, Grants: map[string]access.Grant{owner: grant}, FinancialConfig: json.RawMessage(`{"commission_model":{"owner_company_pct":60,"closing_company_pct":40}}`)}
	data, _ := json.Marshal(p)
	if _, err := db.Exec(`INSERT INTO core.partnerships VALUES($1,'active',$2,NULL)`, id, string(data)); err != nil {
		t.Fatal(err)
	}
	property := `{"partnership_portfolio":true,"capture_exclusivity":"non_exclusive","property_type":"house","address":{"city":"Recife"}}`
	if _, err := db.Exec(`INSERT INTO core.properties VALUES($1,$2,$3,NULL)`, propertyID, owner, property); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO crm.leads VALUES($1,$2,'{"partnership_shared":true}',NULL)`, leadID, owner); err != nil {
		t.Fatal(err)
	}
	repo := NewDealPostgresRepository(&database.PostgresDB{DB: db})
	ctx := context.Background()
	create := func() error {
		return repo.Create(ctx, &deal.Deal{TenantID: closing, PropertyID: propertyID, LeadID: leadID, SourceCompanyID: owner, CoBrokering: true, PartnershipSnapshot: p.Snapshot()})
	}
	if err := create(); err != nil {
		t.Fatal("valid sharing denied", err)
	}
	cases := []struct{ name, query, restore string }{
		{"revoked agreement", `UPDATE core.partnerships SET status='inactive'`, `UPDATE core.partnerships SET status='active'`},
		{"new financial version", `UPDATE core.partnerships SET metadata=jsonb_set(metadata,'{financial_version}','2')`, `UPDATE core.partnerships SET metadata=jsonb_set(metadata,'{financial_version}','1')`},
		{"exclusive property", `UPDATE core.properties SET metadata=jsonb_set(metadata,'{capture_exclusivity}','"exclusive"')`, `UPDATE core.properties SET metadata=jsonb_set(metadata,'{capture_exclusivity}','"non_exclusive"')`},
		{"lead opt-out", `UPDATE crm.leads SET metadata='{"partnership_shared":false}'`, `UPDATE crm.leads SET metadata='{"partnership_shared":true}'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := db.Exec(tc.query); err != nil {
				t.Fatal(err)
			}
			if err := create(); err == nil {
				t.Fatal("stale authorization accepted")
			}
			if _, err := db.Exec(tc.restore); err != nil {
				t.Fatal(err)
			}
		})
	}
	var count int
	if err := db.Get(&count, "SELECT count(*) FROM deal.deals"); err != nil || count != 1 {
		t.Fatal("denied deal persisted", count, err)
	}
}
