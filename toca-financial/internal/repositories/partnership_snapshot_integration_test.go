package repositories

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"os"
	"strings"
	"testing"
)

func TestAcceptedPartnershipSnapshotSurvivesRevocation(t *testing.T) {
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
	if _, err := db.Exec(`CREATE SCHEMA deal;CREATE TABLE deal.deals(id uuid,tenant_id uuid,metadata jsonb);`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA deal CASCADE")
	owner, closing, id := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO deal.deals VALUES($1,$2::uuid,jsonb_build_object('partnership_snapshot',jsonb_build_object('id','accepted-agreement','type','partner','requester_id',$3::text,'target_id',($2::uuid)::text,'financial_config','{"commission_model":{"owner_company_pct":60,"closing_company_pct":40}}'::jsonb)))`, id, closing, owner); err != nil {
		t.Fatal(err)
	}
	// There is deliberately no current core.partnerships table: an accepted
	// snapshot must resolve independently of current agreement existence/status.
	r := &crossPostgresRepository{db: &database.PostgresDB{DB: db}}
	p, err := r.PartnershipForDeal(context.Background(), id, owner, closing)
	if err != nil || p == nil || p.OwnerCompanyPct != 60 || p.ClosingCompanyPct != 40 {
		t.Fatalf("accepted terms lost: %+v %v", p, err)
	}
	if _, err := r.PartnershipForDeal(context.Background(), id, uuid.NewString(), closing); err == nil {
		t.Fatal("snapshot accepted for wrong owner")
	}
	if _, err := r.PartnershipForDeal(context.Background(), id, owner, uuid.NewString()); err == nil {
		t.Fatal("snapshot leaked to wrong tenant")
	}
}

func TestFinancialPartnershipOriginAndLegacyQuarantine(t *testing.T) {
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
	if _, err := db.Exec(`CREATE SCHEMA deal; CREATE TABLE deal.deals(id uuid,tenant_id uuid,metadata jsonb); CREATE SCHEMA core; CREATE TABLE core.properties(id uuid, tenant_id uuid, deleted_at timestamptz);`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP SCHEMA deal CASCADE; DROP SCHEMA core CASCADE")
	closing, source, property := uuid.NewString(), uuid.NewString(), uuid.NewString()
	if _, err := db.Exec(`INSERT INTO core.properties(id,tenant_id) VALUES($1,$2)`, property, closing); err != nil {
		t.Fatal(err)
	}
	r := &crossPostgresRepository{db: &database.PostgresDB{DB: db}}
	cases := []struct {
		name, meta              string
		wantPartner, wantReview bool
	}{
		{"local property with shared lead", `{"property_id":"` + property + `","source_company_id":"` + source + `","partnership_snapshot":{"id":"agreement","requester_id":"` + source + `","target_id":"` + closing + `","financial_config":{"commission_model":{"owner_company_pct":30,"closing_company_pct":70},"admin_fee":12.34,"fee_payer":"closing_pays_source"}}}`, true, false},
		{"legacy shared lead", `{"property_id":"` + property + `","source_company_id":"` + source + `"}`, false, true},
		{"legacy co-brokering", `{"co_brokering":true}`, false, true},
		{"ordinary local deal", `{"property_id":"` + property + `"}`, false, false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.NewString()
			if _, err := db.Exec(`INSERT INTO deal.deals VALUES($1,$2,$3)`, id, closing, tt.meta); err != nil {
				t.Fatal(err)
			}
			p, origin, err := r.FinancialPartnershipForDeal(context.Background(), id, closing)
			if tt.wantReview {
				if !errors.Is(err, ErrHistoricalPartnershipRequired) {
					t.Fatalf("wanted historical review, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPartner {
				if p == nil || origin != source || p.AdminFee != 12.34 || p.FeePayer != "closing_pays_source" {
					t.Fatalf("wrong origin or terms: %+v %s", p, origin)
				}
			} else if p != nil {
				t.Fatal("ordinary deal became partnership")
			}
			if _, _, err := r.FinancialPartnershipForDeal(context.Background(), id, uuid.NewString()); err == nil {
				t.Fatal("cross-tenant access")
			}
		})
	}
}
