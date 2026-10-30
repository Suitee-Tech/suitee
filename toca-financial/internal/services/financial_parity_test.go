package services

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParity_PreviewVsMaterialization_SALE verifies FIN-COM-03:
// given the same inputs (deal value, type, commission pct, broker
// participations, rules, and tenant settings), the preview and the
// contract pipeline resolve to the same ResolvedCommissionPlan with
// the same PlanHash, same amounts, and same share distribution.
func TestParity_PreviewVsMaterialization_SALE(t *testing.T) {
	tt := struct {
		dealType      string
		dealValue     float64
		commissionPct float64
		participants  []contracts.BrokerParticipation
		rules         []commission.Rule
		settings      *tenant_financial_settings.Settings
	}{
		dealType:      "SALE",
		dealValue:     100000.0,
		commissionPct: 6.0,
		participants: []contracts.BrokerParticipation{
			{BrokerID: "broker-cap", Role: "captador", Percentage: 30},
			{BrokerID: "broker-vis", Role: "visitador", Percentage: 20},
			{BrokerID: "broker-fec", Role: "fechamento", Percentage: 50},
		},
		rules: []commission.Rule{
			{Role: commission.RoleCaptador, DefaultPercentage: 30},
			{Role: commission.RoleVisitador, DefaultPercentage: 20},
			{Role: commission.RoleFechamento, DefaultPercentage: 50},
		},
		settings: &tenant_financial_settings.Settings{
			DefaultCommissionPctSale: 6.0,
			DefaultCommissionPctRent: 100.0,
		},
	}

	// Simulate preview path.
	previewPlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             tt.dealType,
		DealValue:            tt.dealValue,
		CommissionPct:        tt.commissionPct,
		BrokerParticipations: tt.participants,
		Rules:                tt.rules,
		TenantSettings:       tt.settings,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, previewPlan.PlanHash)

	// Simulate contract.signed path (same inputs).
	eventPlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             tt.dealType,
		DealValue:            tt.dealValue,
		CommissionPct:        tt.commissionPct,
		BrokerParticipations: tt.participants,
		Rules:                tt.rules,
		TenantSettings:       tt.settings,
	})
	assert.NoError(t, err)

	// FIN-COM-03 invariant: same inputs → same PlanHash.
	assert.True(t, previewPlan.SameHash(eventPlan),
		"preview PlanHash=%s, event PlanHash=%s", previewPlan.PlanHash, eventPlan.PlanHash)

	// Same total amount.
	assert.Equal(t, previewPlan.TotalAmountCents, eventPlan.TotalAmountCents,
		"preview total cents=%d, event total cents=%d", previewPlan.TotalAmountCents, eventPlan.TotalAmountCents)

	// Same commission percentage.
	assert.InDelta(t, previewPlan.CommissionPct, eventPlan.CommissionPct, 0.001)

	// Same number of shares.
	assert.Equal(t, len(previewPlan.Shares), len(eventPlan.Shares))

	// Same per-share amounts (sorted by broker).
	for i := range previewPlan.Shares {
		ps := &previewPlan.Shares[i]
		es := &eventPlan.Shares[i]
		assert.Equal(t, ps.BrokerID, es.BrokerID)
		assert.InDelta(t, ps.Percentage, es.Percentage, 0.001,
			"broker=%s role=%s preview_pct=%.2f event_pct=%.2f", ps.BrokerID, ps.Role, ps.Percentage, es.Percentage)
		assert.Equal(t, ps.AmountCents, es.AmountCents,
			"broker=%s role=%s preview_amount_cents=%d event_amount_cents=%d", ps.BrokerID, ps.Role, ps.AmountCents, es.AmountCents)
	}
}

// TestParity_PreviewVsMaterialization_RENT validates the rental path.
func TestParity_PreviewVsMaterialization_RENT(t *testing.T) {
	tt := struct {
		dealType      string
		dealValue     float64
		commissionPct float64
		participants  []contracts.BrokerParticipation
		rules         []commission.Rule
		settings      *tenant_financial_settings.Settings
	}{
		dealType:      "RENT",
		dealValue:     2500.0,
		commissionPct: 100.0,
		participants: []contracts.BrokerParticipation{
			{BrokerID: "broker-cap", Role: "captador", Percentage: 40},
			{BrokerID: "broker-fec", Role: "fechamento", Percentage: 60},
		},
		rules: []commission.Rule{
			{Role: commission.RoleCaptador, DefaultPercentage: 40},
			{Role: commission.RoleFechamento, DefaultPercentage: 60},
		},
		settings: &tenant_financial_settings.Settings{
			DefaultCommissionPctRent: 100.0,
			DefaultCommissionPctSale: 6.0,
		},
	}

	previewPlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             tt.dealType,
		DealValue:            tt.dealValue,
		CommissionPct:        tt.commissionPct,
		BrokerParticipations: tt.participants,
		Rules:                tt.rules,
		TenantSettings:       tt.settings,
	})
	assert.NoError(t, err)
	assert.NotEmpty(t, previewPlan.PlanHash)

	eventPlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             tt.dealType,
		DealValue:            tt.dealValue,
		CommissionPct:        tt.commissionPct,
		BrokerParticipations: tt.participants,
		Rules:                tt.rules,
		TenantSettings:       tt.settings,
	})
	assert.NoError(t, err)

	assert.True(t, previewPlan.SameHash(eventPlan))
	assert.Equal(t, previewPlan.TotalAmountCents, eventPlan.TotalAmountCents)
	assert.InDelta(t, previewPlan.CommissionPct, eventPlan.CommissionPct, 0.001)
	assert.Equal(t, len(previewPlan.Shares), len(eventPlan.Shares))
}

// TestParity_PlanHashChangesWhenTenantSettingsDiffer validates that
// PlanHash is sensitive to tenant settings changes. The same deal with
// different tenant defaults must produce different hashes.
func TestParity_PlanHashChangesWhenTenantSettingsDiffer(t *testing.T) {
	dealType := "SALE"
	dealValue := 100000.0
	commissionPct := 0.0                              // will fall back to tenant default
	participants := []contracts.BrokerParticipation{} // no explicit participants

	planDefault6, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             dealType,
		DealValue:            dealValue,
		CommissionPct:        commissionPct,
		BrokerParticipations: participants,
		TenantSettings: &tenant_financial_settings.Settings{
			DefaultCommissionPctSale: 6.0,
		},
	})
	assert.NoError(t, err)

	planCustom4, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             dealType,
		DealValue:            dealValue,
		CommissionPct:        commissionPct,
		BrokerParticipations: participants,
		TenantSettings: &tenant_financial_settings.Settings{
			DefaultCommissionPctSale: 4.0,
		},
	})
	assert.NoError(t, err)

	// Different tenant defaults → different hashes.
	assert.False(t, planDefault6.SameHash(planCustom4),
		"same deal with different tenant defaults must produce different PlanHash")

	// Different commission totals.
	assert.Equal(t, int64(600000), planDefault6.TotalAmountCents)
	assert.Equal(t, int64(400000), planCustom4.TotalAmountCents)
}

// TestParity_PlanHashSameWhenSettingsNil verifies that nil settings
// produce the same hash as an all-zero settings struct (consistency
// between handler paths that pass nil vs the preview path).
func TestParity_PlanHashSameWhenSettingsNil(t *testing.T) {
	dealType := "SALE"
	dealValue := 50000.0
	commissionPct := 5.0

	planNil := func() ResolvedCommissionPlan {
		p, err := ResolveCommissionPlan(CommissionPlanInput{
			DealType:      dealType,
			DealValue:     dealValue,
			CommissionPct: commissionPct,
		})
		assert.NoError(t, err)
		return p
	}()

	planZero := func() ResolvedCommissionPlan {
		p, err := ResolveCommissionPlan(CommissionPlanInput{
			DealType:       dealType,
			DealValue:      dealValue,
			CommissionPct:  commissionPct,
			TenantSettings: &tenant_financial_settings.Settings{},
		})
		assert.NoError(t, err)
		return p
	}()

	assert.True(t, planNil.SameHash(planZero),
		"nil settings and zero-value settings must produce same PlanHash")
}

// TestParity_DefaultVsExplicitPercentage produces the same result whether
// commission pct is resolved from tenant settings or explicitly provided.
func TestParity_DefaultVsExplicitPercentage(t *testing.T) {
	dealType := "SALE"
	dealValue := 100000.0
	participants := []contracts.BrokerParticipation{
		{BrokerID: "broker-cap", Role: "captador", Percentage: 30},
		{BrokerID: "broker-vis", Role: "visitador", Percentage: 20},
		{BrokerID: "broker-fec", Role: "fechamento", Percentage: 50},
	}

	// Path 1: commission_pct from event (explicit 6%).
	planExplicit, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             dealType,
		DealValue:            dealValue,
		CommissionPct:        6.0,
		BrokerParticipations: participants,
	})
	assert.NoError(t, err)

	// Path 2: commission_pct from tenant settings (0% → fallback to 6%).
	planFallback, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             dealType,
		DealValue:            dealValue,
		CommissionPct:        0.0,
		BrokerParticipations: participants,
		TenantSettings: &tenant_financial_settings.Settings{
			DefaultCommissionPctSale: 6.0,
		},
	})
	assert.NoError(t, err)

	assert.Equal(t, int64(600000), planExplicit.TotalAmountCents)
	assert.Equal(t, int64(600000), planFallback.TotalAmountCents)

	// Hashes differ because the inputs differ (explicit 6.0 vs 0.0
	// with tenant default), but the results are the same.
	assert.False(t, planExplicit.SameHash(planFallback))
}

// TestParity_DifferentDealTypesDifferentPlan verifies that SALE and RENT
// deals of the same value but different types produce different plans due
// to different default commission percentages.
func TestParity_DifferentDealTypesDifferentPlan(t *testing.T) {
	salePlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:  "SALE",
		DealValue: 100000.0,
		TenantSettings: &tenant_financial_settings.Settings{
			DefaultCommissionPctSale: 6.0,
			DefaultCommissionPctRent: 100.0,
		},
	})
	assert.NoError(t, err)

	rentPlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:  "RENT",
		DealValue: 100000.0,
		TenantSettings: &tenant_financial_settings.Settings{
			DefaultCommissionPctSale: 6.0,
			DefaultCommissionPctRent: 100.0,
		},
	})
	assert.NoError(t, err)

	assert.False(t, salePlan.SameHash(rentPlan))
	assert.Equal(t, int64(600000), salePlan.TotalAmountCents)   // 6% of 100k
	assert.Equal(t, int64(10000000), rentPlan.TotalAmountCents) // 100% of 100k
}

// mockEvent is a minimal event implementation for tests.
type mockEvent struct {
	name    string
	payload map[string]interface{}
}

func (m mockEvent) Name() string         { return m.name }
func (m mockEvent) EventID() string      { return "" }
func (m mockEvent) Payload() interface{} { return m.payload }

// ── Full-stack test mocks (FIN-COM-15) ─────────────────────────────

type fullStackTxRepo struct {
	created []*transaction.Transaction
}

func (m *fullStackTxRepo) Create(_ context.Context, tx *transaction.Transaction) error {
	m.created = append(m.created, tx)
	return nil
}
func (m *fullStackTxRepo) CreateBatch(_ context.Context, txs []*transaction.Transaction) error {
	m.created = append(m.created, txs...)
	return nil
}
func (m *fullStackTxRepo) GetByID(_ context.Context, _ string, id string) (*transaction.Transaction, error) {
	for _, tx := range m.created {
		if tx.ID == id {
			return tx, nil
		}
	}
	return nil, nil
}
func (m *fullStackTxRepo) GetBySourceEventID(_ context.Context, _, _ string) (*transaction.Transaction, error) {
	return nil, nil
}
func (m *fullStackTxRepo) List(_ context.Context, _ string, _ string, _, _ int64) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (m *fullStackTxRepo) Search(_ context.Context, _ string, _ transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}
func (m *fullStackTxRepo) Update(_ context.Context, _ *transaction.Transaction) error { return nil }
func (m *fullStackTxRepo) Delete(_ context.Context, _, _ string) error                { return nil }

type fullStackCommRepo struct {
	created []*commission.Commission
}

func (m *fullStackCommRepo) Create(_ context.Context, c *commission.Commission) error {
	m.created = append(m.created, c)
	return nil
}
func (m *fullStackCommRepo) CreateWithRoleFromEvent(_ context.Context, _ *commission.Commission) error {
	return nil
}
func (m *fullStackCommRepo) CreateFromRequest(_ context.Context, _ *commission.Commission) error {
	return nil
}
func (m *fullStackCommRepo) GetByID(_ context.Context, _ string, id string) (*commission.Commission, error) {
	for _, c := range m.created {
		if c.ID == id {
			return c, nil
		}
	}
	return nil, nil
}
func (m *fullStackCommRepo) GetBySourceEventID(_ context.Context, _, _ string) (*commission.Commission, error) {
	return nil, nil
}
func (m *fullStackCommRepo) List(_ context.Context, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	return nil, 0, nil
}
func (m *fullStackCommRepo) Update(_ context.Context, _ *commission.Commission) error { return nil }
func (m *fullStackCommRepo) UpdateStatus(_ context.Context, _, _ string, _ commission.Status) error {
	return nil
}
func (m *fullStackCommRepo) UpdateStatusByDealID(_ context.Context, _, _ string, _ commission.Status) error {
	return nil
}
func (m *fullStackCommRepo) ListByDealID(_ context.Context, _, _ string) ([]*commission.Commission, error) {
	return m.created, nil
}
func (m *fullStackCommRepo) ListByBrokerID(_ context.Context, _, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	return nil, 0, nil
}
func (m *fullStackCommRepo) Delete(_ context.Context, _, _ string) error { return nil }

type fullStackPayoutRepo struct {
	created []*payout_order.PayoutOrder
}

func (m *fullStackPayoutRepo) Create(_ context.Context, o *payout_order.PayoutOrder) error {
	m.created = append(m.created, o)
	return nil
}
func (m *fullStackPayoutRepo) GetByID(_ context.Context, _ string, id string) (*payout_order.PayoutOrder, error) {
	for _, o := range m.created {
		if o.ID == id {
			return o, nil
		}
	}
	return nil, nil
}
func (m *fullStackPayoutRepo) GetBySourceEventID(_ context.Context, _, _ string) (*payout_order.PayoutOrder, error) {
	return nil, nil
}
func (m *fullStackPayoutRepo) List(_ context.Context, _ string, _ payout_order.ListFilter) ([]*payout_order.PayoutOrder, int64, error) {
	return nil, 0, nil
}
func (m *fullStackPayoutRepo) Totals(_ context.Context, _ string, _ payout_order.ListFilter) (*payout_order.PayoutOrderTotals, error) {
	return nil, nil
}
func (m *fullStackPayoutRepo) ListByIDs(_ context.Context, _ string, _ []string) ([]*payout_order.PayoutOrder, error) {
	return nil, nil
}
func (m *fullStackPayoutRepo) Update(_ context.Context, o *payout_order.PayoutOrder) error {
	for i, existing := range m.created {
		if existing.ID == o.ID {
			m.created[i] = o
			return nil
		}
	}
	return nil
}
func (m *fullStackPayoutRepo) Delete(_ context.Context, _, _ string) error { return nil }

type fullStackRulesRepo struct {
	rules []commission.Rule
}

func (r *fullStackRulesRepo) List(_ context.Context, _ string) ([]commission.Rule, error) {
	return append([]commission.Rule(nil), r.rules...), nil
}
func (r *fullStackRulesRepo) Upsert(_ context.Context, _ string, rules []commission.Rule) error {
	r.rules = append([]commission.Rule(nil), rules...)
	return nil
}

type fullStackSettingsRepo struct {
	settings *tenant_financial_settings.Settings
}

func (r *fullStackSettingsRepo) Get(_ context.Context, _ string) (*tenant_financial_settings.Settings, error) {
	return r.settings, nil
}
func (r *fullStackSettingsRepo) Upsert(_ context.Context, s *tenant_financial_settings.Settings) error {
	r.settings = s
	return nil
}

// TestParity_FullStack_CommissionLifecycle executes the complete commission
// pipeline end-to-end: settings → preview → sign → commission → ledger →
// payout → baixa (FIN-COM-15). Every step asserts the expected outputs
// so regressions in any layer are caught.
func TestParity_FullStack_CommissionLifecycle(t *testing.T) {
	// ── 1. Settings: configure commission rules ─────────────────────
	rules := []commission.Rule{
		{TenantID: "tenant-1", Role: commission.RoleImobiliaria, DefaultPercentage: 40},
		{TenantID: "tenant-1", Role: commission.RoleCaptador, DefaultPercentage: 30},
		{TenantID: "tenant-1", Role: commission.RoleFechamento, DefaultPercentage: 30},
	}
	settings := tenant_financial_settings.New("tenant-1")
	settings.DefaultCommissionPctSale = 6.0
	previewRules, err := NewCommissionRulesService(&fullStackRulesRepo{rules: rules}).Get(context.Background(), "tenant-1")
	require.NoError(t, err)

	dealValue := 500000.0
	participants := []contracts.BrokerParticipation{
		{BrokerID: "broker-cap", Role: "captador", Percentage: 50},
		{BrokerID: "broker-fec", Role: "fechamento", Percentage: 50},
	}

	// ── 2. Preview: plan must be identical to sign path ─────────────
	previewPlan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:             "SALE",
		DealValue:            dealValue,
		CommissionPct:        6.0,
		BrokerParticipations: participants,
		Rules:                previewRules,
		TenantSettings:       settings,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, previewPlan.PlanHash)

	// Total commission: 6% of 500k = 30,000.00
	// Broker participations have explicit percentages:
	//   Captador: 50% → 15,000
	//   Fechamento: 50% → 15,000
	// Commission rules apply only when broker participation percentage
	// is 0 (fallback to rule default). Since both have explicit 50%,
	// the rules' default percentages (30/30) are not used.
	// Imobiliária rule (40%) applies only when a participation has
	// role=imobiliaria — not present here.
	assert.Equal(t, int64(3000000), previewPlan.TotalAmountCents)
	assert.Equal(t, 2, len(previewPlan.Shares))

	// ── 3. Sign: contract.signed event via handler ─────────────────
	txRepo := &fullStackTxRepo{}
	commRepo := &fullStackCommRepo{}
	payoutRepo := &fullStackPayoutRepo{}

	txService := NewTransactionService(txRepo)
	commService := NewCommissionService(commRepo).WithManualTransactionService(txService)
	rulesService := NewCommissionRulesService(&fullStackRulesRepo{rules: rules})
	settingsService := NewTenantFinancialSettingsService(&fullStackSettingsRepo{settings: settings})
	payoutService := NewPayoutOrchestrationService(payoutRepo, nil, nil)
	payoutService.SetCommissionService(commService)

	handler := NewContractSignedEventHandler(
		nil, // pgDB
		nil, // chargeService
		nil, // transferService
		txService,
		nil, // rentalConfigService
		nil, // rentalService
		commService,
		nil, // crossRepo
		nil, // partnerRoyaltyService
		nil, // saleBillingPlanService
		rulesService,
		settingsService,
		payoutService, // payoutOrchestration
	)

	payload := map[string]interface{}{
		"contract_id":    "cont-fullstack",
		"deal_id":        "deal-fullstack",
		"tenant_id":      "tenant-1",
		"type":           "SALE",
		"value":          dealValue,
		"commission_pct": 6.0,
		"customer_id":    "cust-fs",
		"owner_id":       "owner-fs",
		"property_id":    "prop-fs",
		"broker_participations": []interface{}{
			map[string]interface{}{
				"broker_id":  "broker-cap",
				"role":       "captador",
				"percentage": 50.0,
			},
			map[string]interface{}{
				"broker_id":  "broker-fec",
				"role":       "fechamento",
				"percentage": 50.0,
			},
		},
	}

	err = handler(context.Background(), mockEvent{name: "contract.signed", payload: payload})
	require.NoError(t, err)

	// ── 4. Commission entities ─────────────────────────────────────
	// Only broker roles with explicit broker_id generate payable Commission
	// entities. Imobiliária (retained) and role-only entries without broker
	// participation are not payables.
	require.Len(t, commRepo.created, 2, "should create 2 commission entities (captador + fechamento)")

	// Verify PlanHash and LedgerTransactionID are embedded in every Commission.
	for _, c := range commRepo.created {
		assert.NotEmpty(t, c.PlanHash, "every commission must carry the plan hash")
		require.Equal(t, previewPlan.PlanHash, c.PlanHash, "preview and signature must freeze the same plan")
		assert.NotEmpty(t, c.LedgerTransactionID, "every commission must link to its ledger transaction")
		assert.Equal(t, commission.StatusPending, c.Status)
		assert.Equal(t, "deal-fullstack", c.DealID)
		assert.Equal(t, "cont-fullstack", c.ContractID)
		assert.Equal(t, "tenant-1", c.TenantID)
	}

	// Captador: 50% of 30,000 = 15,000
	captador := commRepo.created[0]
	assert.Equal(t, "broker-cap", captador.BrokerID)
	assert.Equal(t, string(commission.RoleCaptador), captador.Role)
	assert.InDelta(t, 15000.0, captador.Amount, 0.01)

	// Fechamento: 50% of 30,000 = 15,000
	fechamento := commRepo.created[1]
	assert.Equal(t, "broker-fec", fechamento.BrokerID)
	assert.Equal(t, string(commission.RoleFechamento), fechamento.Role)
	assert.InDelta(t, 15000.0, fechamento.Amount, 0.01)

	// ── 5. Ledger (Transactions) ───────────────────────────────────
	require.Len(t, txRepo.created, 2, "should create 2 transaction rows (captador + fechamento)")

	// Verify amounts match between ledger and commissions.
	captadorTx := txRepo.created[0]
	fechamentoTx := txRepo.created[1]
	assert.InDelta(t, 15000.0, captadorTx.Value, 0.01)
	assert.InDelta(t, 15000.0, fechamentoTx.Value, 0.01)

	// ── 6. Payout provisioning ─────────────────────────────────────
	require.Len(t, payoutRepo.created, 2, "should provision 2 payout orders")

	// Captador payout: 15,000.00
	assert.Equal(t, int64(1500000), payoutRepo.created[0].AmountCents)
	assert.Equal(t, "broker-cap", payoutRepo.created[0].RecipientID)
	// Fechamento payout: 15,000.00
	assert.Equal(t, int64(1500000), payoutRepo.created[1].AmountCents)
	assert.Equal(t, "broker-fec", payoutRepo.created[1].RecipientID)

	// ── 7. Baixa: mark payout as paid → cascade to commission ──────
	// Link commissions to payout orders (normally done in
	// PayoutOrchestrationService, but we set manually for the test).
	for i, po := range payoutRepo.created {
		commRepo.created[i].PayoutOrderID = po.ID
	}

	payoutOrderService := NewPayoutOrderService(payoutRepo)
	payoutOrderService.commissionService = commService
	_, err = payoutOrderService.Approve(context.Background(), "tenant-1", payoutRepo.created[0].ID, "operator-1")
	require.ErrorIs(t, err, payout_order.ErrFundingIncomplete)

	// Receiving the sale funds the existing obligations and creates the owner
	// transfer from the same frozen composition, without another payable.
	paidAt := time.Now().UTC().Truncate(time.Microsecond)
	saleCharge, err := charge.New("cont-fullstack", "cust-fs", "owner-fs", "prop-fs", "tenant-1", charge.TypeSale, "Venda", paidAt.Format("2006-01"), dealValue, paidAt)
	require.NoError(t, err)
	saleCharge.DealID = "deal-fullstack"
	chargeRepo := &mockChargeRepo{ch: saleCharge, charges: []*charge.Charge{saleCharge}, total: 1}
	chargeService := NewChargeService(chargeRepo, nil).
		WithSaleCommissionFunding(NewSaleCommissionFundingService(commService, payoutRepo))
	chargeService.SetPaymentAllocationResolver(NewEconomicAllocationService(settingsService))
	journalRepo := newFakeJournalRepo()
	chargeService.SetPaymentJournal(NewJournalService(journalRepo))
	transferRepo := &chargeIdentityTransferRepo{}
	chargeService.SetTransferHook(NewTransferChargeHook(NewTransferService(transferRepo), settingsService))
	require.NoError(t, chargeService.ApplySaleEconomicComposition(context.Background(), "tenant-1", "cont-fullstack", previewPlan, 0, "", 0, ""))
	_, err = chargeService.MarkPaidIdempotent(context.Background(), "tenant-1", saleCharge.ID, "PIX", dealValue, "sale-payment", paidAt)
	require.NoError(t, err)
	_, err = chargeService.MarkPaidIdempotent(context.Background(), "tenant-1", saleCharge.ID, "PIX", dealValue, "sale-payment", paidAt)
	require.NoError(t, err)
	require.Len(t, journalRepo.byID, 1)
	require.Len(t, transferRepo.items, 1)
	assert.Equal(t, 470000.0, transferRepo.items[0].NetAmount)
	assert.Equal(t, int64(3000000), transferRepo.items[0].EconomicBreakdown.CommissionFundedCents)
	assert.Zero(t, transferRepo.items[0].EconomicBreakdown.AgencyRetainedCents)
	for _, entry := range journalRepo.byID {
		assert.True(t, entry.Balanced())
		for _, leg := range entry.Legs {
			assert.NotEqual(t, journal.AccountCommissionPayable, leg.Account, "the payment must not accrue commissions again")
		}
	}

	// Approve → mark paid → verify cascade.
	// The baixa now settles the linked ledger transaction (LedgerTransactionID),
	// which requires a database transaction context in the DB-less unit test.
	txCtx := database.WithTx(context.Background(), &sqlx.Tx{})
	for i, po := range payoutRepo.created {
		approved, err := payoutOrderService.Approve(txCtx, po.TenantID, po.ID, "operator-1")
		require.NoError(t, err)
		assert.Equal(t, payout_order.StatusApproved, approved.Status)

		// P1/A13: pagador ≠ aprovador — quem aprovou foi `operator-1`.
		paid, err := payoutOrderService.MarkPaid(txCtx, po.TenantID, po.ID, "operator-2", MarkPaidInput{
			Method:   "MANUAL_TED",
			ProofURL: "https://proof.example.com/receipt-" + po.ID + ".pdf",
		})
		require.NoError(t, err)
		assert.Equal(t, payout_order.StatusPaid, paid.Status)

		// Commission must cascade to Paid.
		assert.Equal(t, commission.StatusPaid, commRepo.created[i].Status,
			"commission %s must cascade to Paid when payout is marked paid", commRepo.created[i].ID)
	}
}
