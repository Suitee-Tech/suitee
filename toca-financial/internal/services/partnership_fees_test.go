package services

import (
	"context"
	"io"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/pkg/events/contracts"
	access "github.com/oginoino/toca/pkg/partnershipaccess"
	"github.com/oginoino/toca/toca-financial/internal/domain/partner_royalty"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/stretchr/testify/require"
)

// The property belongs to the closer; only the persisted deal origin identifies
// the partner who supplied the lead. Falling back to property ownership fails.
type leadOriginLookup struct {
	terms *repositories.PartnershipFinancials
	err   error
}

func (l *leadOriginLookup) PropertyOwnerTenant(context.Context, string) (string, error) {
	panic("must resolve persisted deal origin")
}
func (l *leadOriginLookup) ActivePartnershipBetween(context.Context, string, string) (*repositories.PartnershipFinancials, error) {
	panic("must not replace accepted terms")
}
func (l *leadOriginLookup) FinancialPartnershipForDeal(context.Context, string, string) (*repositories.PartnershipFinancials, string, error) {
	return l.terms, "source", l.err
}

type feeObligationsRepo struct {
	PartnerRoyaltyRepository
	rows []*partner_royalty.PartnerRoyalty
}

func (r *feeObligationsRepo) Create(_ context.Context, p *partner_royalty.PartnerRoyalty) error {
	for _, old := range r.rows {
		if old.SourceEventID == p.SourceEventID && old.TenantID == p.TenantID {
			return partner_royalty.ErrPartnerRoyaltyAlreadyExists
		}
	}
	r.rows = append(r.rows, p)
	return nil
}
func (r *feeObligationsRepo) GetBySourceEventID(_ context.Context, tenant, id string) (*partner_royalty.PartnerRoyalty, error) {
	for _, p := range r.rows {
		if p.TenantID == tenant && p.SourceEventID == id {
			return p, nil
		}
	}
	return nil, nil
}
func (r *feeObligationsRepo) ListPendingByPayerAndContract(_ context.Context, payer, deal, contract string) ([]map[string]any, error) {
	var rows []map[string]any
	for _, p := range r.rows {
		if p.PayerTenant == payer && p.DealID == deal && p.ContractID == contract && p.Status == partner_royalty.StatusPending {
			rows = append(rows, map[string]any{"id": p.ID, "payee_tenant": p.PayeeTenant, "amount": p.Amount, "kind": p.Kind, "percentage": p.Percentage})
		}
	}
	return rows, nil
}

func TestLeadOriginPreviewAndSigningPreserveCommissionAndFixedFees(t *testing.T) {
	for _, rule := range []string{access.FeeClosingPaysSource, access.FeeSourcePaysClosing} {
		t.Run(rule, func(t *testing.T) {
			ctx := context.Background()
			lookup := &leadOriginLookup{terms: &repositories.PartnershipFinancials{ID: "agreement", Type: "partner", OwnerCompanyPct: 30, ClosingCompanyPct: 70, AdminFee: 12.34, TransactionFee: 56.78, FeePayer: rule}}
			previewService := NewFinancialPreviewService(nil, NewRentalChargeConfigService(nil, nil, nil), NewSaleBillingPlanService(nil, nil), nil, nil)
			previewService.SetCrossCompanyLookup(lookup)
			d := &clients.DealDTO{ID: "deal", TenantID: "closing", PropertyID: "local-property", Type: "SALE", Value: 2000, CommissionPct: 10}
			preview, err := previewService.previewFromDeal(ctx, "closing", d, "")
			require.NoError(t, err)
			require.Len(t, preview.PartnershipFees, 2)
			require.Equal(t, int64(20000), preview.TotalCommissionCents)
			require.Equal(t, int64(180000), preview.Composition.OwnerPayableCents)
			obligations := &feeObligationsRepo{}
			txs := &fullStackTxRepo{}
			handler := NewCrossCompanyHandler(lookup, NewTransactionService(txs), nil, NewPartnerRoyaltyService(obligations))
			var input ContractFinancialsInput
			for _, event := range []string{"event-1", "event-2"} {
				input = ContractFinancialsInput{EventID: event, Payload: &contracts.ContractSignedEventPayload{DealID: d.ID, ContractID: "contract", TenantID: "closing", PropertyID: d.PropertyID, Type: d.Type, Value: d.Value, CommissionPct: d.CommissionPct}, CrossCompany: &CrossCompanySettlement{}}
				require.NoError(t, handler.Handle(ctx, input))
				require.Equal(t, int64(6000), input.CrossCompany.OwnerAmountCents)
				require.Equal(t, 7.0, input.Payload.CommissionPct)
			}
			require.Len(t, obligations.rows, 2, "new deliveries cannot charge fixed fees again")
			payouts := &fullStackPayoutRepo{}
			orchestration := NewPayoutOrchestrationService(payouts, nil, nil)
			orchestration.SetRoyaltyLookup(obligations)
			require.NoError(t, orchestration.provisionRoyaltiesAtSigning(ctx, input))
			require.Len(t, payouts.created, 2)
			for i, p := range payouts.created {
				fee := preview.PartnershipFees[i]
				require.Equal(t, fee.AmountCents, p.AmountCents)
				require.Equal(t, fee.PayerTenant, p.TenantID)
				require.Equal(t, fee.PayeeTenant, p.RecipientID)
				require.Equal(t, fee.Kind, p.SourceRef["obligation_kind"])
				require.Equal(t, payout_order.StatusPendingReview, p.Status)
			}
			lookup.terms.AdminFee = 99
			require.ErrorContains(t, handler.Handle(ctx, ContractFinancialsInput{EventID: "conflict", Payload: &contracts.ContractSignedEventPayload{DealID: d.ID, ContractID: "contract", TenantID: "closing", Value: d.Value, CommissionPct: d.CommissionPct}, CrossCompany: &CrossCompanySettlement{}}), "conflicts")
		})
	}
}

func TestPartnershipFeesRejectUnreviewedOrInvalidTerms(t *testing.T) {
	for _, terms := range []*repositories.PartnershipFinancials{{AdminFee: 1}, {AdminFee: math.NaN()}, {TransactionFee: -1}, {AdminFee: .001, FeePayer: access.FeeClosingPaysSource}, {FeePayer: "unknown"}} {
		_, err := partnershipFees(terms, "source", "closing")
		require.ErrorIs(t, err, ErrPartnershipFeesReview)
	}
	fees, err := partnershipFees(&repositories.PartnershipFinancials{}, "source", "closing")
	require.NoError(t, err)
	require.Empty(t, fees)
	lookup := &leadOriginLookup{err: repositories.ErrHistoricalPartnershipRequired}
	h := NewCrossCompanyHandler(lookup, nil, nil, nil)
	require.ErrorIs(t, h.Handle(context.Background(), ContractFinancialsInput{Payload: &contracts.ContractSignedEventPayload{DealID: "legacy", TenantID: "closing"}}), repositories.ErrHistoricalPartnershipRequired)
	p := NewPayoutOrchestrationService(nil, nil, nil)
	require.Error(t, p.provisionRoyaltiesAtSigning(context.Background(), ContractFinancialsInput{CrossCompany: &CrossCompanySettlement{FeePayerTenant: "source"}}))
}

func TestFixedPartnershipFeePayoutAndReceivableLifecycle(t *testing.T) {
	for _, kind := range []string{"admin_fee", "transaction_fee"} {
		for _, action := range []string{"paid", "cancelled"} {
			t.Run(kind+"/"+action, func(t *testing.T) {
				royalty, order := newRoyaltyPayoutTest(t)
				royalty.Kind = kind
				royaltyRepo := &royaltyLifecycleRepo{royalty: royalty}
				payoutRepo := &payoutCASTestRepo{order: order, rollbackOnError: true, externalSnapshot: royaltyRepo.snapshot}
				service := NewPayoutOrderService(payoutRepo)
				service.SetPartnerRoyaltyService(NewPartnerRoyaltyService(royaltyRepo))
				require.NoError(t, func() error {
					_, err := service.Approve(context.Background(), order.TenantID, order.ID, "operator")
					return err
				}())
				if action == "paid" {
					_, err := service.MarkPaid(context.Background(), order.TenantID, order.ID, "settler", MarkPaidInput{Method: string(payout_order.SettlementManualPIX), ProofURL: "https://proof.example/fee"})
					require.NoError(t, err)
					require.Equal(t, partner_royalty.StatusPaid, royaltyRepo.royalty.Status)
					require.NotNil(t, royaltyRepo.royalty.PaidAt)
				} else {
					_, err := service.Cancel(context.Background(), order.TenantID, order.ID, "operator", "cancelled by agreement")
					require.NoError(t, err)
					require.Equal(t, partner_royalty.StatusCancelled, royaltyRepo.royalty.Status)
				}
				require.Equal(t, kind, royaltyRepo.royalty.Kind)
				require.True(t, royaltyRepo.royalty.UpdatedAt.Before(time.Now().Add(time.Second)))
			})
		}
	}
}

func TestPartnerLeadWizardWaitsForSigningAndCannotUseLocalOnlyReceiptPlan(t *testing.T) {
	base := clients.NewBaseClient("http://deal.test")
	base.InternalAPIKey = "test-internal-key"
	base.HTTPClient = &http.Client{Transport: previewDealRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":{"id":"deal","tenant_id":"closing","type":"SALE","property_id":"local","value":2000,"commission":10}}`)), Request: r}, nil
	})}
	client := &clients.DealClient{BaseClient: base}
	lookup := &leadOriginLookup{terms: &repositories.PartnershipFinancials{ID: "agreement", OwnerCompanyPct: 30, ClosingCompanyPct: 70}}
	// Empty chain would panic if wizard attempted local-only materialization.
	wizard := NewSalePlanMaterializationService(&ChainedContractHandler{}, client, lookup, nil)
	require.NoError(t, wizard.EnsureMaterialized(context.Background(), "closing", "contract", "deal"))
	preview := NewFinancialPreviewService(client, nil, nil, nil, nil)
	preview.SetCrossCompanyLookup(lookup)
	_, err := preview.ResolvePlanForDeal(context.Background(), "closing", "deal")
	require.ErrorContains(t, err, "assine o contrato")
	lookup.err = repositories.ErrHistoricalPartnershipRequired
	require.ErrorIs(t, wizard.EnsureMaterialized(context.Background(), "closing", "contract", "deal"), repositories.ErrHistoricalPartnershipRequired)
}
