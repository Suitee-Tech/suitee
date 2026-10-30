package services

import (
	"github.com/oginoino/toca/pkg/domain/valueobjects"

	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
	"github.com/oginoino/toca/toca-financial/internal/providers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockChargeRepo struct {
	createErr       error
	getErr          error
	updateErr       error
	listErr         error
	ch              *charge.Charge
	charges         []*charge.Charge
	total           int64
	overdueErr      error
	overdueCalls    int
	overdueTenantID string
	lockCalls       int
}

func (m *mockChargeRepo) Create(ctx context.Context, c *charge.Charge) error {
	m.ch = c
	return m.createErr
}
func (m *mockChargeRepo) GetByID(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	return m.ch, m.getErr
}
func (m *mockChargeRepo) GetByIDForUpdate(ctx context.Context, tenantID, id string) (*charge.Charge, error) {
	m.lockCalls++
	return m.GetByID(ctx, tenantID, id)
}
func (m *mockChargeRepo) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*charge.Charge, error) {
	if m.ch != nil && m.ch.SourceEventID == sourceEventID {
		return m.ch, nil
	}
	return nil, nil
}
func (m *mockChargeRepo) Update(ctx context.Context, c *charge.Charge) error {
	m.ch = c
	return m.updateErr
}
func (m *mockChargeRepo) List(ctx context.Context, tenantID, contractID, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return m.charges, m.total, m.listErr
}
func (m *mockChargeRepo) ListByPartyIDs(ctx context.Context, tenantID string, partyIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return m.charges, m.total, m.listErr
}
func (m *mockChargeRepo) ListByLeadIDs(ctx context.Context, tenantID string, leadIDs []string, status, referenceMonth string, limit, offset int64) ([]*charge.Charge, int64, error) {
	return m.charges, m.total, m.listErr
}
func (m *mockChargeRepo) Search(ctx context.Context, tenantID string, filter charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return m.charges, m.total, m.listErr
}
func (m *mockChargeRepo) ListOverdue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	m.overdueCalls++
	m.overdueTenantID = tenantID
	return m.charges, m.overdueErr
}
func (m *mockChargeRepo) BulkUpdateStatus(ctx context.Context, tenantID string, ids []string, status charge.Status) error {
	return nil
}
func (m *mockChargeRepo) ListByIDs(ctx context.Context, tenantID string, ids []string) ([]*charge.Charge, error) {
	return m.charges, nil
}
func (m *mockChargeRepo) ListPendingPastDue(ctx context.Context, tenantID string) ([]*charge.Charge, error) {
	return m.charges, nil
}
func (m *mockChargeRepo) ListDistinctTenantIDs(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (m *mockChargeRepo) ListByDealAndCustomer(ctx context.Context, tenantID, dealID, accountID, role string) ([]*charge.Charge, error) {
	return nil, nil
}
func (m *mockChargeRepo) AgingReport(ctx context.Context, tenantID string) ([]charge.AgingBucket, error) {
	return nil, nil
}
func (m *mockChargeRepo) GetDelinquencyStats(ctx context.Context, tenantID string) (*charge.DelinquencyStats, error) {
	return nil, nil
}

func TestChargeService_Create(t *testing.T) {
	repo := &mockChargeRepo{}
	svc := NewChargeService(repo, nil)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		ch, err := svc.Create(ctx, "c-1", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2024-01", 1000.0, time.Now().Add(time.Hour*24))
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ch == nil {
			t.Fatal("expected charge, got nil")
		}
		if ch.Status != charge.StatusPending {
			t.Errorf("expected PENDING status, got %s", ch.Status)
		}
	})

	t.Run("invalid amount", func(t *testing.T) {
		_, err := svc.Create(ctx, "c-1", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2024-01", 0, time.Now())
		if err == nil {
			t.Fatal("expected error for zero amount")
		}
	})
}

func TestChargeService_CreateWithComposition(t *testing.T) {
	repo := &mockChargeRepo{}
	svc := NewChargeService(repo, nil)
	ctx := context.Background()
	due := time.Now().Add(24 * time.Hour)
	items := []charge.LineItem{
		{Description: "Aluguel", AmountCents: 250000},
		{Description: "Condomínio", AmountCents: 80000},
		{Description: "IPTU", AmountCents: 20000},
	}

	t.Run("line items derive amount and discount applies to total", func(t *testing.T) {
		// amount arg (1.0) is intentionally wrong — service must override it
		// with Σ items (R$ 3.500,00) so ValidateLineItems holds.
		ch, err := svc.CreateWithComposition(ctx, "c-1", "d-1", "l-1", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "Aluguel jan", "2026-01", 1.0, 350.0, due, items)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ch.Amount.Cents() != 350000 {
			t.Errorf("expected amount 350000 cents, got %d", ch.Amount.Cents())
		}
		if ch.TotalAmount.Cents() != 315000 { // 3500 - 350 desconto
			t.Errorf("expected total 315000 cents, got %d", ch.TotalAmount.Cents())
		}
		if len(ch.LineItems) != 3 {
			t.Errorf("expected 3 line items, got %d", len(ch.LineItems))
		}
		if ch.DealID != "d-1" || ch.LeadID != "l-1" {
			t.Errorf("expected deal/lead refs to be set, got %q/%q", ch.DealID, ch.LeadID)
		}
	})

	t.Run("discount cannot exceed amount", func(t *testing.T) {
		_, err := svc.CreateWithComposition(ctx, "c-1", "", "", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2026-01", 0, 4000.0, due, items)
		if err == nil {
			t.Fatal("expected error when discount exceeds amount")
		}
	})

	t.Run("no items falls back to plain amount", func(t *testing.T) {
		ch, err := svc.CreateWithComposition(ctx, "c-1", "", "", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2026-01", 1000.0, 0, due, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if ch.Amount.Cents() != 100000 || ch.TotalAmount.Cents() != 100000 {
			t.Errorf("expected 100000/100000 cents, got %d/%d", ch.Amount.Cents(), ch.TotalAmount.Cents())
		}
		if len(ch.LineItems) != 0 {
			t.Errorf("expected no line items, got %d", len(ch.LineItems))
		}
	})
}

func TestChargeServiceFreezesSaleEconomicComposition(t *testing.T) {
	first, err := charge.New("contract-sale", "buyer-1", "owner-1", "property-1", "tenant-1", charge.TypeSale, "Entrada", "2026-09", 500000, time.Now())
	require.NoError(t, err)
	second, err := charge.New("contract-sale", "buyer-1", "owner-1", "property-1", "tenant-1", charge.TypeSale, "Saldo", "2026-10", 480000, time.Now())
	require.NoError(t, err)
	repo := &mockChargeRepo{charges: []*charge.Charge{first, second}, total: 2}
	svc := NewChargeService(repo, nil)
	plan := ResolvedCommissionPlan{
		TotalAmountCents:    5880000,
		RetainedAmountCents: 2352000,
		Shares: []ResolvedCommissionShare{
			{BrokerID: "broker-captador", Role: "captador", Percentage: 20, AmountCents: 1176000},
			{BrokerID: "broker-fechamento", Role: "fechamento", Percentage: 40, AmountCents: 2352000},
		},
	}

	err = svc.ApplySaleEconomicComposition(context.Background(), "tenant-1", "contract-sale", plan, 0, "", 0, "")
	require.NoError(t, err)

	assert.Equal(t, int64(92120000), sumSaleLineItemsByType([]*charge.Charge{first, second}, charge.AllocationTypeOwnerPayable))
	assert.Zero(t, sumSaleLineItemsByType([]*charge.Charge{first, second}, charge.AllocationTypeCommissionPayable), "signing already accrued the broker obligation")
	assert.Equal(t, int64(5880000), sumSaleLineItemsByType([]*charge.Charge{first, second}, charge.AllocationTypeRevenue))
	for _, saleCharge := range []*charge.Charge{first, second} {
		assert.NoError(t, charge.ValidateLineItems(saleCharge.LineItems, saleCharge.Amount.Cents()))
	}
}

func TestChargeServiceKeepsFranchiseRoyaltyInsideCommissionPool(t *testing.T) {
	sale, err := charge.New("contract-franchise", "buyer-1", "owner-1", "property-1", "tenant-closer", charge.TypeSale, "Venda", "2026-09", 980000, time.Now())
	require.NoError(t, err)
	repo := &mockChargeRepo{charges: []*charge.Charge{sale}, total: 1}
	plan := ResolvedCommissionPlan{
		// Closing-company share after the 30% owner-company split.
		TotalAmountCents: 4116000,
		Shares: []ResolvedCommissionShare{
			{BrokerID: "broker-close", Role: "fechamento", Percentage: 42.8571, AmountCents: 1764000},
		},
	}

	err = NewChargeService(repo, nil).ApplySaleEconomicComposition(
		context.Background(),
		"tenant-closer",
		"contract-franchise",
		plan,
		1764000,
		"tenant-owner-company",
		588000,
		"tenant-franchisor",
	)
	require.NoError(t, err)

	assert.Equal(t, int64(92120000), sumSaleLineItemsByType([]*charge.Charge{sale}, charge.AllocationTypeOwnerPayable))
	assert.Zero(t, sumSaleLineItemsByType([]*charge.Charge{sale}, charge.AllocationTypeCommissionPayable))
	assert.Equal(t, int64(1764000), sumSaleLineItemsByType([]*charge.Charge{sale}, charge.AllocationTypePartnerPayable))
	assert.Equal(t, int64(588000), sumSaleLineItemsByType([]*charge.Charge{sale}, charge.AllocationTypeRoyaltyPayable))
	assert.Equal(t, int64(3528000), sumSaleLineItemsByType([]*charge.Charge{sale}, charge.AllocationTypeRevenue))
	assert.NoError(t, charge.ValidateLineItems(sale.LineItems, sale.Amount.Cents()))
}

func sumSaleLineItemsByType(charges []*charge.Charge, allocationType charge.AllocationType) int64 {
	var total int64
	for _, saleCharge := range charges {
		for _, item := range saleCharge.LineItems {
			if item.AllocationType == allocationType {
				total += item.AmountCents
			}
		}
	}
	return total
}

func TestChargeService_GetByID(t *testing.T) {
	ch, _ := charge.New("c-1", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2024-01", 1000.0, time.Now())
	repo := &mockChargeRepo{ch: ch}
	svc := NewChargeService(repo, nil)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		res, err := svc.GetByID(ctx, "tenant-1", ch.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.ID != ch.ID {
			t.Errorf("expected %s, got %s", ch.ID, res.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		repo.ch = nil
		_, err := svc.GetByID(ctx, "tenant-1", "invalid")
		if err == nil {
			t.Fatal("expected error")
		}
		repo.ch = ch // reset
	})

	// Note: tenant isolation is now enforced at the repository layer (MongoDB query filter).
	// In production, GetByID with wrong tenantID returns "not found" from the DB.
}

func TestChargeService_Update(t *testing.T) {
	ch, _ := charge.New("c-1", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2024-01", 1000.0, time.Now())
	repo := &mockChargeRepo{ch: ch}
	svc := NewChargeService(repo, nil)
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		ch.Amount = valueobjects.NewMoneyFromFloat(1500.0)
		err := svc.Update(ctx, ch)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if repo.ch.Amount.Float() != 1500.0 {
			t.Errorf("expected 1500.0, got %f", repo.ch.Amount.Float())
		}
	})

	t.Run("db error", func(t *testing.T) {
		repo.updateErr = errors.New("db error")
		err := svc.Update(ctx, ch)
		if err == nil {
			t.Fatal("expected error")
		}
		repo.updateErr = nil
	})
}

func TestChargeService_ListOverdue_DelegatesToRepository(t *testing.T) {
	expected := []*charge.Charge{{ID: "overdue-1"}, {ID: "past-due-pending"}}
	repo := &mockChargeRepo{charges: expected}
	svc := NewChargeService(repo, nil)

	got, err := svc.ListOverdue(context.Background(), "tenant-overdue")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if repo.overdueCalls != 1 {
		t.Fatalf("expected one repository call, got %d", repo.overdueCalls)
	}
	if repo.overdueTenantID != "tenant-overdue" {
		t.Fatalf("expected tenant-overdue, got %q", repo.overdueTenantID)
	}
	if len(got) != len(expected) || got[0] != expected[0] || got[1] != expected[1] {
		t.Fatalf("service must preserve repository result and ordering: got %#v", got)
	}

	wantErr := errors.New("overdue query failed")
	repo.overdueErr = wantErr
	if _, err := svc.ListOverdue(context.Background(), "tenant-overdue"); !errors.Is(err, wantErr) {
		t.Fatalf("expected repository error to be preserved, got %v", err)
	}
}

func TestChargeServiceMarkPaid_SettlesOnlySourceLinkedReceivable(t *testing.T) {
	ctx := context.Background()
	ch, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Aluguel", "2026-08", 1000, time.Now())
	if err != nil {
		t.Fatalf("create charge: %v", err)
	}
	ch.SourceEventID = "charge-source-1"

	chargeRepo := &mockChargeRepo{ch: ch}
	txRepo := newStoringTxRepo()
	receivable := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
	receivable.SourceEventID = ch.SourceEventID
	commission := transaction.New("contract-1", transaction.TypeComissao, "Comissão corretor", 60, time.Now(), "tenant-1")
	commission.SourceEventID = "commission-source-1"
	expense := transaction.New("contract-1", transaction.TypeDespesa, "Despesa", 25, time.Now(), "tenant-1")
	expense.SourceEventID = "expense-source-1"
	for _, tx := range []*transaction.Transaction{receivable, commission, expense} {
		if err := txRepo.Create(ctx, tx); err != nil {
			t.Fatalf("create transaction: %v", err)
		}
	}

	svc := NewChargeService(chargeRepo, nil)
	svc.SetTxService(NewTransactionService(txRepo))
	paid, err := svc.MarkPaid(ctx, "tenant-1", ch.ID, "PIX", 1000)
	if err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if paid.Status != charge.StatusPaid {
		t.Fatalf("expected charge PAID, got %s", paid.Status)
	}
	if receivable.Status != transaction.StatusPaid {
		t.Fatalf("expected linked receivable PAID, got %s", receivable.Status)
	}
	if commission.Status != transaction.StatusPending {
		t.Fatalf("commission must remain PENDING, got %s", commission.Status)
	}
	if expense.Status != transaction.StatusPending {
		t.Fatalf("expense must remain PENDING, got %s", expense.Status)
	}
}

func TestChargeServiceMarkPaid_WithoutSourceLinkDoesNotGuessByContract(t *testing.T) {
	ctx := context.Background()
	ch, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Aluguel", "2026-08", 1000, time.Now())
	if err != nil {
		t.Fatalf("create charge: %v", err)
	}

	chargeRepo := &mockChargeRepo{ch: ch}
	txRepo := newStoringTxRepo()
	receivable := transaction.New("contract-1", transaction.TypeReceita, "Lançamento sem vínculo", 1000, time.Now(), "tenant-1")
	commission := transaction.New("contract-1", transaction.TypeComissao, "Comissão corretor", 60, time.Now(), "tenant-1")
	for _, tx := range []*transaction.Transaction{receivable, commission} {
		if err := txRepo.Create(ctx, tx); err != nil {
			t.Fatalf("create transaction: %v", err)
		}
	}

	svc := NewChargeService(chargeRepo, nil)
	svc.SetTxService(NewTransactionService(txRepo))
	if _, err := svc.MarkPaid(ctx, "tenant-1", ch.ID, "PIX", 1000); err != nil {
		t.Fatalf("mark paid: %v", err)
	}
	if receivable.Status != transaction.StatusPending {
		t.Fatalf("unlinked receivable must remain PENDING, got %s", receivable.Status)
	}
	if commission.Status != transaction.StatusPending {
		t.Fatalf("commission must remain PENDING, got %s", commission.Status)
	}
}

// P1-05 — GeneratePaymentLink: sem provedor responde "não configurado";
// com o sandbox gera link determinístico; cobrança paga é recusada.
func TestChargeService_GeneratePaymentLink(t *testing.T) {
	ctx := context.Background()
	ch, _ := charge.New("c-1", "t-1", "o-1", "p-1", "tenant-1", charge.TypeRent, "desc", "2024-01", 1000.0, time.Now())

	t.Run("sem provedor", func(t *testing.T) {
		svc := NewChargeService(&mockChargeRepo{ch: ch}, nil)
		_, err := svc.GeneratePaymentLink(ctx, "tenant-1", ch.ID)
		if !errors.Is(err, ErrPaymentProviderNotConfigured) {
			t.Fatalf("err = %v, want ErrPaymentProviderNotConfigured", err)
		}
	})

	t.Run("sandbox determinístico", func(t *testing.T) {
		svc := NewChargeService(&mockChargeRepo{ch: ch}, nil).
			WithPaymentProvider(providers.NewSandboxPaymentProvider())
		first, err := svc.GeneratePaymentLink(ctx, "tenant-1", ch.ID)
		if err != nil {
			t.Fatalf("GeneratePaymentLink: %v", err)
		}
		second, err := svc.GeneratePaymentLink(ctx, "tenant-1", ch.ID)
		if err != nil {
			t.Fatalf("GeneratePaymentLink (replay): %v", err)
		}
		if first.URL != second.URL {
			t.Fatalf("link deveria ser determinístico: %q vs %q", first.URL, second.URL)
		}
		if first.Provider != "sandbox" {
			t.Fatalf("provider = %q", first.Provider)
		}
	})

	t.Run("cobrança paga recusada", func(t *testing.T) {
		paid := *ch
		paid.Status = charge.StatusPaid
		svc := NewChargeService(&mockChargeRepo{ch: &paid}, nil).
			WithPaymentProvider(providers.NewSandboxPaymentProvider())
		if _, err := svc.GeneratePaymentLink(ctx, "tenant-1", ch.ID); err == nil {
			t.Fatal("cobrança paga não deveria gerar link")
		}
	})

	t.Run("charge inexistente", func(t *testing.T) {
		svc := NewChargeService(&mockChargeRepo{}, nil).
			WithPaymentProvider(providers.NewSandboxPaymentProvider())
		if _, err := svc.GeneratePaymentLink(ctx, "tenant-1", "missing"); err == nil {
			t.Fatal("charge inexistente deveria falhar")
		}
	})
}

// O link de pagamento precisa ser PERSISTIDO: era devolvido na resposta e
// perdido, e o portal do cliente mostrava os botões de pagar/2ª via sem destino.
func TestGeneratePaymentLinkPersisteEReusaoLink(t *testing.T) {
	reps := &paymentLinkRepo{}
	svc := NewChargeService(reps, nil).WithPaymentProvider(providers.NewSandboxPaymentProvider())

	cobranca := novaCobrancaFase2(t, "ch-link", time.Now().AddDate(0, 0, 5), 150, charge.StatusPending)
	reps.porID = map[string]*charge.Charge{cobranca.ID: cobranca}

	link, err := svc.GeneratePaymentLink(context.Background(), "t-1", cobranca.ID)
	if err != nil {
		t.Fatalf("GeneratePaymentLink() erro = %v", err)
	}
	if link == nil || link.URL == "" {
		t.Fatalf("link vazio")
	}
	persistida := reps.porID[cobranca.ID]
	if persistida.PaymentURL != link.URL || persistida.PaymentProvider != "sandbox" || persistida.PaymentExternalID == "" {
		t.Fatalf("link não foi persistido: url=%q provider=%q external=%q",
			persistida.PaymentURL, persistida.PaymentProvider, persistida.PaymentExternalID)
	}
	if persistida.PaymentExpiresAt == nil {
		t.Fatalf("validade do link não foi gravada")
	}

	// Segunda chamada REUSA o link gravado (gerar outro invalidaria o que o
	// cliente já recebeu).
	segundo, err := svc.GeneratePaymentLink(context.Background(), "t-1", cobranca.ID)
	if err != nil {
		t.Fatalf("segunda chamada erro = %v", err)
	}
	if segundo.URL != link.URL || segundo.ExternalID != link.ExternalID {
		t.Fatalf("esperava reuso do link: %q != %q", segundo.URL, link.URL)
	}
}

// paymentLinkRepo é um repositório mínimo de cobrança para o teste do link.
type paymentLinkRepo struct {
	*mockChargeRepo
	porID map[string]*charge.Charge
}

func (r *paymentLinkRepo) GetByID(_ context.Context, _, id string) (*charge.Charge, error) {
	return r.porID[id], nil
}
func (r *paymentLinkRepo) Update(_ context.Context, c *charge.Charge) error {
	r.porID[c.ID] = c
	return nil
}
func (r *paymentLinkRepo) UpdateIfVersion(_ context.Context, c *charge.Charge, _ time.Time) (bool, error) {
	r.porID[c.ID] = c
	return true, nil
}
