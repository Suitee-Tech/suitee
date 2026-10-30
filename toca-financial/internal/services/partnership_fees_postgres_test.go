package services

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/config"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events/contracts"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-financial/internal/domain/partner_royalty"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/stretchr/testify/require"
)

// Run against a dedicated disposable database, separately from repository tests
// that own the same schemas. This uses production repositories and transactions.
func TestPartnershipFeesPostgresLifecycle(t *testing.T) {
	uri := os.Getenv("PARTNERSHIP_REVIEW_TEST_DATABASE_URL")
	if uri == "" {
		t.Skip("requires disposable toca_partnership_review_qa_* database")
	}
	t.Setenv("REQUIRE_RLS_BACKSTOP", "false")
	ctx := context.Background()
	db, err := database.ConnectPostgres(ctx, config.DatabaseConfig{URI: uri, ServiceURI: uri})
	require.NoError(t, err)
	defer db.Close()
	var name string
	require.NoError(t, db.Get(&name, "SELECT current_database()"))
	require.True(t, strings.HasPrefix(name, "toca_partnership_review_qa_"), "refusing non-QA database")
	_, err = db.Exec(ctx, `CREATE SCHEMA financial; CREATE SCHEMA deal; CREATE TABLE deal.deals(id uuid,tenant_id uuid,metadata jsonb); CREATE TABLE deal.contracts(id uuid,tenant_id uuid,metadata jsonb); CREATE TABLE financial.transactions(id uuid PRIMARY KEY,tenant_id text,metadata jsonb,created_at timestamptz DEFAULT now(),updated_at timestamptz DEFAULT now(),deleted_at timestamptz); CREATE UNIQUE INDEX transaction_source ON financial.transactions(tenant_id,((metadata->>'source_event_id'))) WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL`)
	require.NoError(t, err)
	defer db.Exec(ctx, "DROP SCHEMA financial CASCADE; DROP SCHEMA deal CASCADE")
	for _, filename := range []string{"20260522120001_partner_royalties.sql", "20260526120100_create_payout_orders.sql"} {
		sql, err := os.ReadFile("../../../supabase/migrations/" + filename)
		require.NoError(t, err)
		_, err = db.Exec(ctx, string(sql))
		require.NoError(t, err)
	}
	_, err = db.Exec(ctx, `CREATE UNIQUE INDEX fee_source ON financial.partner_royalties(tenant_id,((metadata->>'source_event_id'))) WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL; CREATE UNIQUE INDEX payout_source ON financial.payout_orders(tenant_id,((metadata->>'source_event_id'))) WHERE deleted_at IS NULL AND (metadata->>'source_event_id') IS NOT NULL AND COALESCE(metadata->>'status','') <> 'CANCELLED'`)
	require.NoError(t, err)
	obligations := repositories.NewPartnerRoyaltyPostgresRepository(db)
	orders := repositories.NewPayoutOrderPostgresRepository(db)
	royalties := NewPartnerRoyaltyService(obligations)
	handler := NewCrossCompanyHandler(repositories.NewCrossPostgresRepository(db), NewTransactionService(repositories.NewTransactionPostgresRepository(db)), nil, royalties)
	orchestration := NewPayoutOrchestrationService(orders, nil, nil)
	orchestration.SetRoyaltyLookup(obligations)
	transitions := NewPayoutOrderService(orders)
	transitions.SetPartnerRoyaltyService(royalties)
	for _, rule := range []string{access.FeeClosingPaysSource, access.FeeSourcePaysClosing} {
		t.Run(rule, func(t *testing.T) {
			closing, source, deal, contract, agreement := uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString()
			meta, err := json.Marshal(map[string]any{"source_company_id": source, "partnership_snapshot": map[string]any{"id": agreement, "type": "partner", "requester_id": source, "target_id": closing, "financial_config": map[string]any{"commission_model": map[string]any{"owner_company_pct": 30, "closing_company_pct": 70}, "admin_fee": 12.34, "transaction_fee": 56.78, "fee_payer": rule}}})
			require.NoError(t, err)
			_, err = db.Exec(ctx, `INSERT INTO deal.deals VALUES($1,$2,$3)`, deal, closing, string(meta))
			require.NoError(t, err)
			_, err = db.Exec(ctx, `INSERT INTO deal.contracts VALUES($1,$2,jsonb_build_object('deal_id',$3::text))`, contract, closing, deal)
			require.NoError(t, err)
			dealValue := 2000.0
			run := func(event string) error {
				return db.RunInServiceContext(ctx, func(txctx context.Context) error {
					in := ContractFinancialsInput{EventID: event, Payload: &contracts.ContractSignedEventPayload{DealID: deal, ContractID: contract, TenantID: closing, Type: "SALE", Value: dealValue, CommissionPct: 10}, CrossCompany: &CrossCompanySettlement{}}
					if err := handler.Handle(txctx, in); err != nil {
						return err
					}
					return orchestration.OnContractSigned(txctx, in)
				})
			}
			orchestration.SetRoyaltyLookup(nil)
			require.Error(t, run("injected-failure"))
			var before int
			require.NoError(t, db.Get(&before, `SELECT count(*) FROM financial.partner_royalties WHERE deal_id=$1`, deal))
			require.Zero(t, before, "failed payout provisioning must roll back fees")
			require.NoError(t, db.Get(&before, `SELECT count(*) FROM financial.transactions WHERE tenant_id=$1`, source))
			require.Zero(t, before, "failed payout provisioning must roll back commission")
			orchestration.SetRoyaltyLookup(obligations)
			require.NoError(t, run("first"))
			require.NoError(t, run("redelivery"))
			payer, payee := closing, source
			if rule == access.FeeSourcePaysClosing {
				payer, payee = source, closing
			}
			received, total, err := obligations.List(ctx, payee, 100, 0)
			require.NoError(t, err)
			require.Equal(t, int64(2), total)
			payable, count, err := orders.List(ctx, payer, payout_order.ListFilter{ContractID: contract, Limit: 100})
			require.NoError(t, err)
			expected := int64(2)
			if payer == closing {
				expected = 3
			}
			require.Equal(t, expected, count)
			wrong, count, err := orders.List(ctx, uuid.NewString(), payout_order.ListFilter{Limit: 100})
			require.NoError(t, err)
			require.Zero(t, count)
			require.Empty(t, wrong)
			byID := map[string]*partner_royalty.PartnerRoyalty{}
			for _, r := range received {
				byID[r.ID] = r
			}
			for _, order := range payable {
				if order.RecipientType == payout_order.RecipientPartnerTenant {
					require.Equal(t, int64(6000), order.AmountCents)
					continue
				}
				r := byID[order.SourceID]
				require.NotNil(t, r)
				require.Equal(t, r.Kind, order.SourceRef["obligation_kind"])
				_, err = transitions.Approve(ctx, payer, order.ID, "approver")
				require.NoError(t, err)
				if r.Kind == "admin_fee" {
					_, err = transitions.MarkPaid(ctx, payer, order.ID, "settler", MarkPaidInput{Method: string(payout_order.SettlementManualPIX), ProofURL: "https://proof.example/fee"})
					require.NoError(t, err)
				} else {
					_, err = transitions.Cancel(ctx, payer, order.ID, "approver", "revisão do negócio")
					require.NoError(t, err)
				}
				updated, err := obligations.GetByID(ctx, payee, r.ID)
				require.NoError(t, err)
				require.Equal(t, order.ID, updated.PayoutOrderID)
				if r.Kind == "admin_fee" {
					require.Equal(t, partner_royalty.StatusPaid, updated.Status)
				} else {
					require.Equal(t, partner_royalty.StatusCancelled, updated.Status)
				}
			}
			require.NoError(t, run("after-terminal-states"))
			_, count, err = orders.List(ctx, payer, payout_order.ListFilter{ContractID: contract, Limit: 100})
			require.NoError(t, err)
			require.Equal(t, expected, count, "terminal states cannot resurrect fee obligations")
			// An amendment and a new event reuse the economic obligations. Also
			// exercise mirrors written before deal_id was stored on transactions.
			_, err = db.Exec(ctx, `UPDATE financial.transactions SET metadata=metadata-'deal_id' WHERE tenant_id=$1`, source)
			require.NoError(t, err)
			contract = uuid.NewString()
			_, err = db.Exec(ctx, `INSERT INTO deal.contracts VALUES($1,$2,jsonb_build_object('deal_id',$3::text))`, contract, closing, deal)
			require.NoError(t, err)
			require.NoError(t, run("amendment"))
			_, count, err = orders.List(ctx, payer, payout_order.ListFilter{Limit: 100})
			require.NoError(t, err)
			require.Equal(t, expected, count)
			var mirrors int
			require.NoError(t, db.Get(&mirrors, `SELECT count(*) FROM financial.transactions WHERE tenant_id=$1`, source))
			require.Equal(t, 1, mirrors)
			dealValue = 3000
			require.ErrorContains(t, run("changed-amendment"), "divergente")
			require.NoError(t, db.Get(&mirrors, `SELECT count(*) FROM financial.transactions WHERE tenant_id=$1`, source))
			require.Equal(t, 1, mirrors)

		})
	}
}
