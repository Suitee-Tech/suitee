package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan"
	"github.com/oginoino/toca/toca-financial/internal/domain/sale_billing_plan_draft"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/repositories"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticSaleDraftRepo struct {
	draft *sale_billing_plan_draft.Draft
}

type staticPreviewCrossCompany struct {
	ownerTenant string
	partnership *repositories.PartnershipFinancials
}

func (s *staticPreviewCrossCompany) PropertyOwnerTenant(context.Context, string) (string, error) {
	return s.ownerTenant, nil
}

func (s *staticPreviewCrossCompany) ActivePartnershipBetween(context.Context, string, string) (*repositories.PartnershipFinancials, error) {
	return s.partnership, nil
}

func (r *staticSaleDraftRepo) Upsert(_ context.Context, d *sale_billing_plan_draft.Draft) error {
	r.draft = d
	return nil
}

func (r *staticSaleDraftRepo) GetActiveByDealForService(_ context.Context, _, _ string) (*sale_billing_plan_draft.Draft, error) {
	return r.draft, nil
}
func (r *staticSaleDraftRepo) GetActiveByDeal(_ context.Context, _, _ string) (*sale_billing_plan_draft.Draft, error) {
	return r.draft, nil
}

func (r *staticSaleDraftRepo) MarkConsumed(_ context.Context, _, _ string) error { return nil }

type staticPreviewSettingsRepo struct {
	settings *tenant_financial_settings.Settings
}

func (r *staticPreviewSettingsRepo) Get(_ context.Context, _ string) (*tenant_financial_settings.Settings, error) {
	return r.settings, nil
}

func (r *staticPreviewSettingsRepo) Upsert(_ context.Context, settings *tenant_financial_settings.Settings) error {
	r.settings = settings
	return nil
}

func TestFinancialPreviewUsesTenantDefaultCommissionPercentage(t *testing.T) {
	tests := []struct {
		name          string
		dealType      string
		expectedPct   float64
		expectedCents int64
	}{
		{name: "sale", dealType: "SALE", expectedPct: 7.5, expectedCents: 7500},
		{name: "rent", dealType: "RENT", expectedPct: 80, expectedCents: 80000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := tenant_financial_settings.New("tenant-1")
			settings.DefaultCommissionPctSale = 7.5
			settings.DefaultCommissionPctRent = 80
			settingsService := NewTenantFinancialSettingsService(&staticPreviewSettingsRepo{settings: settings})
			previewService := NewFinancialPreviewService(
				nil,
				NewRentalChargeConfigService(nil, nil, nil),
				NewSaleBillingPlanService(nil, nil),
				nil,
				settingsService,
			)

			start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
			end := start.AddDate(0, 1, 0)
			deal := &clients.DealDTO{
				ID:            "deal-1",
				TenantID:      "tenant-1",
				Type:          tt.dealType,
				Value:         1000,
				CommissionPct: 0,
				CustomerID:    "customer-1",
				OwnerID:       "owner-1",
				PropertyID:    "property-1",
				StartDate:     &start,
				EndDate:       &end,
			}

			preview, err := previewService.previewFromDeal(context.Background(), "tenant-1", deal, "")

			require.NoError(t, err)
			require.Len(t, preview.Commissions, 1)
			// Negócio sem participantes: a comissão inteira fica retida pela
			// imobiliária (a distribuição sempre fecha 100%). O que este teste
			// protege continua sendo o percentual padrão do tenant — ele define o
			// VALOR da comissão, e é isso que as asserções de valor verificam.
			assert.True(t, preview.Commissions[0].Retained)
			assert.Equal(t, "imobiliaria", preview.Commissions[0].Role)
			assert.InDelta(t, 100, preview.Commissions[0].Percentage, 0.0001)
			assert.Equal(t, tt.expectedCents, preview.Commissions[0].AmountCents)
			assert.Equal(t, tt.expectedCents, preview.TotalCommissionCents)
		})
	}
}

func TestFinancialPreviewCarriesFirstRentIntermediationIntoFollowingMonth(t *testing.T) {
	settings := tenant_financial_settings.New("tenant-rent")
	settings.AdminFeePct = 10
	settings.DefaultCommissionPctRent = 100
	previewService := NewFinancialPreviewService(
		nil,
		NewRentalChargeConfigService(nil, nil, nil),
		NewSaleBillingPlanService(nil, nil),
		nil,
		NewTenantFinancialSettingsService(&staticPreviewSettingsRepo{settings: settings}),
	)
	start := time.Date(2026, time.August, 20, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, time.October, 31, 0, 0, 0, 0, time.UTC)
	deal := &clients.DealDTO{
		ID: "deal-rent", TenantID: "tenant-rent", Type: "RENT", Value: 5000,
		CustomerID: "tenant-party", OwnerID: "owner-rent", PropertyID: "property-rent",
		StartDate: &start, EndDate: &end,
		BrokerParticipations: []clients.DealBrokerParticipationDTO{
			{BrokerID: "broker-rent", Role: "fechamento", Percentage: 60},
			{Role: "imobiliaria", Percentage: 40},
		},
	}

	preview, err := previewService.previewFromDeal(context.Background(), "tenant-rent", deal, "")
	require.NoError(t, err)
	require.Len(t, preview.Transfers, 3)
	first := preview.Transfers[0]
	assert.Equal(t, "2026-08", first.ReferenceMonth)
	assert.Equal(t, int64(193548), first.GrossCents)
	assert.Equal(t, int64(19355), first.AdminFeeCents)
	assert.Equal(t, int64(174193), first.CommissionFundedCents)
	assert.Equal(t, int64(0), first.NetCents)
	assert.Equal(t, int64(325807), first.IntermediationCarryCents)

	second := preview.Transfers[1]
	assert.Equal(t, "2026-09", second.ReferenceMonth)
	assert.Equal(t, int64(125807), second.CommissionFundedCents)
	assert.Equal(t, int64(200000), second.AgencyIntermediationCents)
	assert.Equal(t, int64(124193), second.NetCents)
	assert.Equal(t, int64(0), second.IntermediationCarryCents)

	require.NotNil(t, preview.Composition)
	assert.Equal(t, int64(1193548), preview.Composition.GrossCents)
	assert.Equal(t, int64(119355), preview.Composition.AdminFeeCents)
	assert.Equal(t, int64(500000), preview.Composition.IntermediationCents)
	assert.Equal(t, int64(300000), preview.Composition.CommissionFundedCents)
	assert.Equal(t, int64(319355), preview.Composition.AgencyRetainedCents)
	assert.Equal(t, int64(574193), preview.Composition.OwnerPayableCents)
	assert.Equal(t, int64(574193), preview.TotalTransfersCents)
	assert.Contains(t, preview.Notes, "O primeiro aluguel proporcional não cobre toda a intermediação; o saldo será apropriado automaticamente nas competências seguintes, após a taxa de administração.")
}

func TestFinancialPreviewUsesActiveSaleDraft(t *testing.T) {
	firstDue := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	draft, err := sale_billing_plan_draft.New(
		"tenant-1", "deal-1", sale_billing_plan.ModeInstallments,
		100000, 20000, 40000, 2, firstDue, 30,
		sale_billing_plan.CompositeInput{}, "operator-1",
	)
	require.NoError(t, err)
	saleService := NewSaleBillingPlanService(nil, nil).WithDraftRepo(&staticSaleDraftRepo{draft: draft})
	previewService := NewFinancialPreviewService(nil, nil, saleService, nil, nil)
	deal := &clients.DealDTO{
		ID: "deal-1", TenantID: "tenant-1", Type: "SALE", Value: 1000,
		CustomerID: "buyer-1", OwnerID: "owner-1", PropertyID: "property-1",
	}

	preview, err := previewService.previewFromDeal(context.Background(), "tenant-1", deal, "")
	require.NoError(t, err)
	require.Len(t, preview.Charges, 3)
	assert.Equal(t, int64(100000), preview.TotalChargesCents)
	assert.Contains(t, preview.Notes, "Prévia baseada no draft ativo (INSTALLMENTS).")
}

func TestFinancialPreviewRejectsSaleDraftWithStaleTotal(t *testing.T) {
	firstDue := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	draft, err := sale_billing_plan_draft.New(
		"tenant-1", "deal-1", sale_billing_plan.ModeSingle,
		90000, 0, 90000, 1, firstDue, 30,
		sale_billing_plan.CompositeInput{}, "operator-1",
	)
	require.NoError(t, err)
	saleService := NewSaleBillingPlanService(nil, nil).WithDraftRepo(&staticSaleDraftRepo{draft: draft})
	previewService := NewFinancialPreviewService(nil, nil, saleService, nil, nil)
	deal := &clients.DealDTO{ID: "deal-1", TenantID: "tenant-1", Type: "SALE", Value: 1000}

	_, err = previewService.previewFromDeal(context.Background(), "tenant-1", deal, "")
	require.ErrorContains(t, err, "does not match frozen deal total")
}

func TestFinancialPreviewExplainsSaleOwnerAndAgencyWaterfall(t *testing.T) {
	firstDue := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	draft, err := sale_billing_plan_draft.New(
		"tenant-1", "deal-sale-waterfall", sale_billing_plan.ModeSingle,
		98000000, 0, 98000000, 1, firstDue, 30,
		sale_billing_plan.CompositeInput{}, "operator-1",
	)
	require.NoError(t, err)
	saleService := NewSaleBillingPlanService(nil, nil).WithDraftRepo(&staticSaleDraftRepo{draft: draft})
	previewService := NewFinancialPreviewService(nil, nil, saleService, nil, nil)
	deal := &clients.DealDTO{
		ID: "deal-sale-waterfall", TenantID: "tenant-1", Type: "SALE", Value: 980000, CommissionPct: 6,
		CustomerID: "buyer-1", OwnerID: "seller-1", PropertyID: "property-1",
		BrokerParticipations: []clients.DealBrokerParticipationDTO{
			{BrokerID: "broker-captador", Role: "captador", Percentage: 20},
			{BrokerID: "broker-fechamento", Role: "fechamento", Percentage: 40},
			{BrokerID: "", Role: "imobiliaria", Percentage: 40},
		},
	}

	preview, err := previewService.previewFromDeal(context.Background(), "tenant-1", deal, "")
	require.NoError(t, err)
	require.NotNil(t, preview.Composition)
	assert.Equal(t, int64(98000000), preview.Composition.GrossCents)
	assert.Equal(t, int64(5880000), preview.Composition.CommissionPoolCents)
	assert.Equal(t, int64(3528000), preview.Composition.CommissionPayableCents)
	assert.Equal(t, int64(2352000), preview.Composition.AgencyRetainedCents)
	assert.Equal(t, int64(92120000), preview.Composition.OwnerPayableCents)
}

func TestSaleEconomicCompositionTreatsRoyaltyAsAgencyObligation(t *testing.T) {
	service := NewFinancialPreviewService(nil, nil, nil, nil, nil)
	preview := &FinancialPreviewDTO{
		DealType: "SALE",
		Commissions: []CommissionPreview{
			{Role: "empresa_captadora", AmountCents: 2352000},
			{BrokerID: "broker-1", Role: "fechamento", AmountCents: 1764000},
			{Role: "imobiliaria", AmountCents: 1764000, Retained: true},
			{Role: "royalty_franquia", AmountCents: 176400},
		},
	}
	service.computeTotals(preview)
	service.buildEconomicComposition(preview, 980000)

	require.NotNil(t, preview.Composition)
	assert.Equal(t, int64(5880000), preview.Composition.CommissionPoolCents)
	assert.Equal(t, int64(92120000), preview.Composition.OwnerPayableCents, "royalty must not reduce seller proceeds twice")
	assert.Equal(t, int64(2528400), preview.Composition.PartnerPayableCents)
	assert.Equal(t, int64(1587600), preview.Composition.AgencyRetainedCents)
}

func TestFinancialPreviewMirrorsCrossCompanyCommissionSplit(t *testing.T) {
	previewService := NewFinancialPreviewService(
		nil, NewRentalChargeConfigService(nil, nil, nil), nil, nil, nil,
	)
	previewService.SetCrossCompanyLookup(&staticPreviewCrossCompany{
		ownerTenant: "tenant-owner",
		partnership: &repositories.PartnershipFinancials{
			ID: "partnership-1", Type: "partner", OwnerCompanyPct: 40, ClosingCompanyPct: 60,
		},
	})
	start := time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	deal := &clients.DealDTO{
		ID: "deal-1", TenantID: "tenant-closing", Type: "RENT", Value: 1000, CommissionPct: 10,
		CustomerID: "customer-1", OwnerID: "owner-1", PropertyID: "property-1", StartDate: &start, EndDate: &end,
	}

	preview, err := previewService.previewFromDeal(context.Background(), "tenant-closing", deal, "")
	require.NoError(t, err)
	require.Len(t, preview.Commissions, 2)
	assert.Equal(t, "empresa_captadora", preview.Commissions[0].Role)
	assert.Equal(t, int64(4000), preview.Commissions[0].AmountCents)
	assert.Equal(t, int64(6000), preview.Commissions[1].AmountCents)
	assert.Equal(t, int64(10000), preview.TotalCommissionCents)
}
