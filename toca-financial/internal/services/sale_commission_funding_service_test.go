package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Return detached snapshots and enforce revisions so a failed funding write
// cannot accidentally succeed by mutating a pointer owned by the fake.
type saleFundingCommissionRepo struct {
	CommissionRepository
	items  map[string]*commission.Commission
	writes int
}

func (r *saleFundingCommissionRepo) ListByDealID(_ context.Context, tenantID, dealID string) ([]*commission.Commission, error) {
	var items []*commission.Commission
	for _, item := range r.items {
		if item.TenantID == tenantID && item.DealID == dealID {
			items = append(items, cloneCommissionForTest(item))
		}
	}
	return items, nil
}

func (r *saleFundingCommissionRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	item := r.items[id]
	if item == nil || item.TenantID != tenantID {
		return nil, nil
	}
	return cloneCommissionForTest(item), nil
}

func (r *saleFundingCommissionRepo) UpdateIfStatus(_ context.Context, item *commission.Commission, expected commission.Status, revision time.Time) error {
	current := r.items[item.ID]
	if current == nil || current.Status != expected || !current.UpdatedAt.Equal(revision) {
		return commission.ErrConcurrentTransition
	}
	r.items[item.ID] = cloneCommissionForTest(item)
	r.writes++
	return nil
}

type saleFundingPayoutRepo struct {
	payout_order.Repository
	items  map[string]*payout_order.PayoutOrder
	writes int
}

func (r *saleFundingPayoutRepo) GetByID(_ context.Context, tenantID, id string) (*payout_order.PayoutOrder, error) {
	item := r.items[id]
	if item == nil || item.TenantID != tenantID {
		return nil, nil
	}
	return clonePayoutOrderForTest(item), nil
}

func (r *saleFundingPayoutRepo) UpdateIfStatus(_ context.Context, item *payout_order.PayoutOrder, expected payout_order.Status, revision time.Time) error {
	current := r.items[item.ID]
	if current == nil || current.Status != expected || !current.UpdatedAt.Equal(revision) {
		return payout_order.ErrConcurrentTransition
	}
	r.items[item.ID] = clonePayoutOrderForTest(item)
	r.writes++
	return nil
}

type saleFundingFixture struct {
	service     *SaleCommissionFundingService
	commissions *saleFundingCommissionRepo
	payouts     *saleFundingPayoutRepo
	paidAt      time.Time
}

func newSaleFundingFixture(t *testing.T, amounts ...int64) *saleFundingFixture {
	t.Helper()
	f := &saleFundingFixture{
		commissions: &saleFundingCommissionRepo{items: make(map[string]*commission.Commission)},
		payouts:     &saleFundingPayoutRepo{items: make(map[string]*payout_order.PayoutOrder)},
		paidAt:      time.Date(2026, time.September, 8, 12, 0, 0, 0, time.UTC),
	}
	for i, amount := range amounts {
		id := string(rune('a' + i))
		item := commission.NewWithRole("deal-sale", "broker-"+id, "contract-sale", "role-"+id, 100, float64(amount)/100, "tenant-sale")
		item.ID, item.PlanHash, item.PayoutOrderID = "commission-"+id, "frozen-sale-plan", "payout-"+id
		_, err := item.EnableFundingGate(amount)
		require.NoError(t, err)
		order, err := payout_order.New(item.TenantID, payout_order.SourceManual, item.ContractID, payout_order.RecipientBroker, item.BrokerID, "Corretor", amount, "system")
		require.NoError(t, err)
		order.ID, order.ContractID = item.PayoutOrderID, item.ContractID
		order.SourceRef = map[string]any{"deal_id": item.DealID, "charge_type": "SALE", "commission_id": item.ID, "plan_hash": item.PlanHash, "role": item.Role}
		_, err = order.EnableFundingGate(amount)
		require.NoError(t, err)
		f.commissions.items[item.ID], f.payouts.items[order.ID] = item, order
	}
	f.service = NewSaleCommissionFundingService(NewCommissionService(f.commissions), f.payouts)
	return f
}

func newSaleFundingCharge(t *testing.T, id, month string) *charge.Charge {
	t.Helper()
	c, err := charge.NewWithLineItems("contract-sale", "buyer-sale", "owner-sale", "property-sale", "tenant-sale",
		charge.TypeSale, "Parcela da venda", month, 1000, time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC),
		[]charge.LineItem{
			{ID: id + "-owner", Description: "Proprietário", AmountCents: 65000, SourceType: "sale_principal", EconomicOwnerID: "owner-sale", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeOwnerPayable, RuleVersionID: "frozen-sale-plan"},
			{ID: id + "-agency", Description: "Imobiliária", AmountCents: 10000, SourceType: "sale_agency_retained", EconomicOwnerID: "tenant-sale", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue, RuleVersionID: "frozen-sale-plan"},
			{ID: id + "-brokers", Description: "Corretores", AmountCents: 25000, SourceType: "sale_commission_payable", EconomicOwnerID: "tenant-sale", SettlementMode: charge.SettlementModeCollection, AllocationType: charge.AllocationTypeRevenue, RuleVersionID: "frozen-sale-plan"},
		})
	require.NoError(t, err)
	c.ID, c.DealID = id, "deal-sale"
	return c
}

func saleFundingAllocations(t *testing.T, c *charge.Charge, paymentID string, cents int64) []charge.Allocation {
	t.Helper()
	_, allocations, err := NewEconomicAllocationService(nil).BuildPaymentAllocations(context.Background(), c, paymentID, cents)
	require.NoError(t, err)
	return allocations
}

func TestSaleCommissionFundingAccumulatesPartialReceiptsAcrossInstallments(t *testing.T) {
	f := newSaleFundingFixture(t, 30000, 20000)
	first := newSaleFundingCharge(t, "installment-1", "2026-09")
	second := newSaleFundingCharge(t, "installment-2", "2026-10")
	steps := []struct {
		charge      *charge.Charge
		paymentID   string
		cents       int64
		wantFunding map[string]int64
	}{
		{first, "partial-1", 40000, map[string]int64{"commission-a": 6000, "commission-b": 4000}},
		{first, "partial-2", 60000, map[string]int64{"commission-a": 15000, "commission-b": 10000}},
		{second, "installment-2-payment", 100000, map[string]int64{"commission-a": 30000, "commission-b": 20000}},
	}
	for i, step := range steps {
		allocations, err := f.service.FundAllocations(context.Background(), step.charge, step.paymentID, f.paidAt,
			saleFundingAllocations(t, step.charge, step.paymentID, step.cents))
		require.NoError(t, err)
		require.NoError(t, charge.ValidateAllocations(allocations, step.cents))
		applied, err := step.charge.ApplyPaymentWithAllocations(step.paymentID, float64(step.cents)/100, "PIX", f.paidAt, charge.AllocationMethodProportionalV1, allocations)
		require.NoError(t, err)
		require.True(t, applied)
		for id, want := range step.wantFunding {
			item := f.commissions.items[id]
			order := f.payouts.items[item.PayoutOrderID]
			assert.Equal(t, want, item.FundedAmountCents)
			assert.Equal(t, want, order.FundedAmountCents)
			assert.Equal(t, commission.StatusPending, item.Status)
			assert.Len(t, item.FundingEntries, i+1)
			assert.Len(t, order.FundingEntries, i+1)
			assert.Equal(t, step.charge.ID, item.FundingEntries[i].ChargeID)
			assert.Equal(t, step.charge.ReferenceMonth, item.FundingEntries[i].ReferenceMonth)
			assert.Equal(t, f.paidAt, item.FundingEntries[i].FundedAt)
			if i < len(steps)-1 {
				assert.Equal(t, payout_order.StatusDraft, order.Status)
				assert.ErrorIs(t, cloneCommissionForTest(item).Approve(), commission.ErrFundingIncomplete)
				assert.ErrorIs(t, clonePayoutOrderForTest(order).Approve("operator"), payout_order.ErrFundingIncomplete)
			} else {
				assert.Equal(t, payout_order.StatusPendingReview, order.Status)
				assert.NoError(t, cloneCommissionForTest(item).Approve())
				assert.NoError(t, clonePayoutOrderForTest(order).Approve("operator"))
			}
		}
	}
	assert.Equal(t, 6, f.commissions.writes)
	assert.Equal(t, 6, f.payouts.writes)
}

func TestSaleCommissionFundingReceiptRecognizesRevenueWithoutDuplicatingPayable(t *testing.T) {
	f := newSaleFundingFixture(t, 15000, 10000)
	c := newSaleFundingCharge(t, "sale-charge", "2026-09")
	allocations, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, saleFundingAllocations(t, c, "receipt", 100000))
	require.NoError(t, err)
	entry, err := newAllocatedPaymentJournalEntry(c, &charge.PaymentEntry{
		ID: "receipt", AmountCents: 100000, PaidAt: f.paidAt, Allocations: allocations,
	})
	require.NoError(t, err)
	assert.True(t, entry.Balanced())
	assert.Equal(t, "receipt", entry.OperationID)
	assertJournalLeg(t, entry, journal.AccountCash, journal.Debit, 100000, "")
	assertJournalLeg(t, entry, journal.AccountOwnerPayable, journal.Credit, 65000, c.OwnerID)
	var revenue int64
	brokerRevenue := map[string]int64{}
	for _, leg := range entry.Legs {
		assert.Equal(t, "sale", leg.Flow)
		assert.NotEqual(t, journal.AccountCommissionPayable, leg.Account, "the signing accrual already owns the broker liability")
		assert.NotEqual(t, journal.AccountCommissionExpense, leg.Account, "the receipt must not accrue the commission twice")
		if leg.Account == journal.AccountRevenue {
			revenue += leg.AmountCents
			assert.Equal(t, journal.Credit, leg.Direction)
			assert.Equal(t, c.TenantID, leg.BeneficiaryPartyID)
		}
		if leg.AllocationPurpose == string(charge.AllocationPurposeSaleCommissionBroker) {
			brokerRevenue[leg.RelatedEntityID] += leg.AmountCents
			assert.Equal(t, c.LineItems[2].ID, leg.ChargeItemID)
			assert.NotEmpty(t, leg.AllocationID)
			assert.NotEmpty(t, leg.RuleVersionID)
		}
	}
	assert.EqualValues(t, 35000, revenue)
	assert.Equal(t, map[string]int64{"commission-a": 15000, "commission-b": 10000}, brokerRevenue)
}

func TestSaleCommissionFundingRetryKeepsAllocationsAndFundingAfterApproval(t *testing.T) {
	f := newSaleFundingFixture(t, 15000, 10000)
	c := newSaleFundingCharge(t, "sale-charge", "2026-09")
	input := saleFundingAllocations(t, c, "receipt", 100000)
	first, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, input)
	require.NoError(t, err)
	for _, order := range f.payouts.items {
		require.NoError(t, order.Approve("operator"))
	}
	retry, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt.Add(time.Hour), input)
	require.NoError(t, err)
	assert.Equal(t, first, retry)
	assert.Equal(t, 2, f.commissions.writes)
	assert.Equal(t, 2, f.payouts.writes)
	for _, item := range f.commissions.items {
		assert.Len(t, item.FundingEntries, 1)
		assert.Equal(t, f.paidAt, item.FundingEntries[0].FundedAt)
		assert.Len(t, f.payouts.items[item.PayoutOrderID].FundingEntries, 1)
		assert.Equal(t, payout_order.StatusApproved, f.payouts.items[item.PayoutOrderID].Status)
	}
}

func TestSaleCommissionFundingRejectsInconsistentPayoutLinks(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*commission.Commission, *payout_order.PayoutOrder)
	}{
		{"wrong contract", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.ContractID = "another-contract" }},
		{"wrong tenant", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.TenantID = "another-tenant" }},
		{"wrong broker", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.RecipientID = "another-broker" }},
		{"wrong recipient type", func(_ *commission.Commission, p *payout_order.PayoutOrder) {
			p.RecipientType = payout_order.RecipientExternal
		}},
		{"wrong requirement", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.FundingRequiredCents++ }},
		{"wrong payout gross", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.AmountCents++ }},
		{"wrong commission gross", func(c *commission.Commission, _ *payout_order.PayoutOrder) { c.GrossAmount++ }},
		{"another commission source", func(_ *commission.Commission, p *payout_order.PayoutOrder) {
			p.SourceRef["commission_id"] = "another-commission"
		}},
		{"another plan source", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.SourceRef["plan_hash"] = "another-plan" }},
		{"another deal source", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.SourceRef["deal_id"] = "another-deal" }},
		{"another role source", func(_ *commission.Commission, p *payout_order.PayoutOrder) { p.SourceRef["role"] = "another-role" }},
		{"missing commission gate", func(c *commission.Commission, _ *payout_order.PayoutOrder) { c.FundingRequiredCents = 0 }},
		{"cancelled commission", func(c *commission.Commission, _ *payout_order.PayoutOrder) { c.Status = commission.StatusCancelled }},
		{"unmatched plan", func(c *commission.Commission, _ *payout_order.PayoutOrder) { c.PlanHash = "another-plan" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaleFundingFixture(t, 25000)
			tc.mutate(f.commissions.items["commission-a"], f.payouts.items["payout-a"])
			c := newSaleFundingCharge(t, "sale-charge", "2026-09")
			_, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, saleFundingAllocations(t, c, "receipt", 100000))
			require.Error(t, err)
			assert.Zero(t, f.commissions.writes)
			assert.Zero(t, f.payouts.writes)
		})
	}
}

func TestSaleCommissionFundingRetryRejectsDivergentReceiptHistories(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*saleFundingFixture, *charge.Charge, []charge.Allocation)
	}{
		{"different charge", func(_ *saleFundingFixture, c *charge.Charge, _ []charge.Allocation) { c.ID = "different-charge" }},
		{"different month", func(_ *saleFundingFixture, c *charge.Charge, _ []charge.Allocation) { c.ReferenceMonth = "2026-10" }},
		{"different amount", func(_ *saleFundingFixture, _ *charge.Charge, a []charge.Allocation) { a[2].AmountCents++ }},
		{"missing payout entry", func(f *saleFundingFixture, _ *charge.Charge, _ []charge.Allocation) {
			f.payouts.items["payout-a"].FundingEntries = nil
		}},
		{"different payout entry", func(f *saleFundingFixture, _ *charge.Charge, _ []charge.Allocation) {
			f.payouts.items["payout-a"].FundingEntries[0].AmountCents++
		}},
		{"different payout balance", func(f *saleFundingFixture, _ *charge.Charge, _ []charge.Allocation) {
			f.payouts.items["payout-a"].FundedAmountCents++
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newSaleFundingFixture(t, 25000)
			c := newSaleFundingCharge(t, "sale-charge", "2026-09")
			input := saleFundingAllocations(t, c, "receipt", 100000)
			_, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, input)
			require.NoError(t, err)
			tc.mutate(f, c, input)
			_, err = f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, input)
			require.Error(t, err)
			assert.Equal(t, 1, f.commissions.writes)
			assert.Equal(t, 1, f.payouts.writes)
		})
	}
}

func TestSaleCommissionFundingKeepsSeparateRolesForTheSameBroker(t *testing.T) {
	f := newSaleFundingFixture(t, 12500, 12500)
	f.commissions.items["commission-b"].BrokerID = "broker-a"
	f.payouts.items["payout-b"].RecipientID = "broker-a"
	c := newSaleFundingCharge(t, "sale-charge", "2026-09")
	allocations, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, saleFundingAllocations(t, c, "receipt", 100000))
	require.NoError(t, err)
	byCommission := map[string]int64{}
	for _, allocation := range allocations {
		if allocation.Purpose == charge.AllocationPurposeSaleCommissionBroker {
			byCommission[allocation.RelatedEntityID] += allocation.AmountCents
		}
	}
	assert.Equal(t, map[string]int64{"commission-a": 12500, "commission-b": 12500}, byCommission)
	assert.EqualValues(t, 12500, f.payouts.items["payout-a"].FundedAmountCents)
	assert.EqualValues(t, 12500, f.payouts.items["payout-b"].FundedAmountCents)
}

func TestSaleCommissionFundingRejectsSharedPayoutForDifferentRoles(t *testing.T) {
	f := newSaleFundingFixture(t, 12500, 12500)
	f.commissions.items["commission-b"].BrokerID = "broker-a"
	f.commissions.items["commission-b"].PayoutOrderID = "payout-a"
	f.payouts.items["payout-a"].SourceRef = nil // Older orders have no role or commission snapshot.
	c := newSaleFundingCharge(t, "sale-charge", "2026-09")
	_, err := f.service.FundAllocations(context.Background(), c, "receipt", f.paidAt, saleFundingAllocations(t, c, "receipt", 100000))
	require.Error(t, err)
	assert.Zero(t, f.commissions.writes)
	assert.Zero(t, f.payouts.writes)
}

func TestSaleCommissionFundingRequiresPaymentTransactionBeforeWriting(t *testing.T) {
	f := newSaleFundingFixture(t, 25000)
	c := newSaleFundingCharge(t, "sale-charge", "2026-09")
	_, err := f.service.RequirePaymentTransaction().FundAllocations(context.Background(), c, "receipt", f.paidAt, saleFundingAllocations(t, c, "receipt", 100000))
	require.ErrorContains(t, err, "requires the charge payment transaction")
	assert.Zero(t, f.commissions.writes)
	assert.Zero(t, f.payouts.writes)
}
