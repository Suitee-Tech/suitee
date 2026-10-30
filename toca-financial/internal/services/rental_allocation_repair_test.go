package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

// --- fakes ---

type fakeChargeRepo struct {
	charge  *charge.Charge
	updates int
}

func (f *fakeChargeRepo) Create(context.Context, *charge.Charge) error { return nil }
func (f *fakeChargeRepo) GetByID(context.Context, string, string) (*charge.Charge, error) {
	return f.charge, nil
}
func (f *fakeChargeRepo) GetBySourceEventID(context.Context, string, string) (*charge.Charge, error) {
	return nil, nil
}
func (f *fakeChargeRepo) Update(_ context.Context, c *charge.Charge) error {
	f.charge = c
	f.updates++
	return nil
}
func (f *fakeChargeRepo) UpdateIfVersion(_ context.Context, c *charge.Charge, _ time.Time) (bool, error) {
	f.charge = c
	f.updates++
	return true, nil
}
func (f *fakeChargeRepo) List(context.Context, string, string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (f *fakeChargeRepo) ListByPartyIDs(context.Context, string, []string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (f *fakeChargeRepo) ListByLeadIDs(context.Context, string, []string, string, string, int64, int64) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (f *fakeChargeRepo) Search(context.Context, string, charge.SearchFilter) ([]*charge.Charge, int64, error) {
	return nil, 0, nil
}
func (f *fakeChargeRepo) ListOverdue(context.Context, string) ([]*charge.Charge, error) {
	return nil, nil
}
func (f *fakeChargeRepo) BulkUpdateStatus(context.Context, string, []string, charge.Status) error {
	return nil
}
func (f *fakeChargeRepo) ListByIDs(context.Context, string, []string) ([]*charge.Charge, error) {
	return nil, nil
}
func (f *fakeChargeRepo) ListPendingPastDue(context.Context, string) ([]*charge.Charge, error) {
	return nil, nil
}
func (f *fakeChargeRepo) ListDistinctTenantIDs(context.Context) ([]string, error) { return nil, nil }
func (f *fakeChargeRepo) ListByDealAndCustomer(context.Context, string, string, string, string) ([]*charge.Charge, error) {
	return nil, nil
}
func (f *fakeChargeRepo) AgingReport(context.Context, string) ([]charge.AgingBucket, error) {
	return nil, nil
}
func (f *fakeChargeRepo) GetDelinquencyStats(context.Context, string) (*charge.DelinquencyStats, error) {
	return nil, nil
}

type fakeConfigRepo struct {
	cfg *rental_charge_config.RentalChargeConfig
}

func (f *fakeConfigRepo) Create(context.Context, *rental_charge_config.RentalChargeConfig) error {
	return nil
}
func (f *fakeConfigRepo) GetByID(context.Context, string, string) (*rental_charge_config.RentalChargeConfig, error) {
	return f.cfg, nil
}
func (f *fakeConfigRepo) GetByContractID(context.Context, string, string) (*rental_charge_config.RentalChargeConfig, error) {
	return f.cfg, nil
}
func (f *fakeConfigRepo) Update(context.Context, *rental_charge_config.RentalChargeConfig) error {
	return nil
}
func (f *fakeConfigRepo) Delete(context.Context, string, string) error { return nil }

// fakeAllocationRebuilder devolve uma composição com intermediação e registra o
// snapshot recebido — o teste verifica que as alocações antigas do pagamento
// foram removidas antes do rebuild (senão o saldo seria contado duas vezes).
type fakeAllocationRebuilder struct {
	snapshotPayment *charge.PaymentEntry
	calls           int
	intermediation  int64
}

func (f *fakeAllocationRebuilder) BuildPaymentAllocations(_ context.Context, _ *charge.Charge, _ string, _ int64) (charge.AllocationMethod, []charge.Allocation, error) {
	return f.RebuildPaymentAllocations(context.Background(), nil, "", 0)
}

func (f *fakeAllocationRebuilder) RebuildPaymentAllocations(_ context.Context, c *charge.Charge, paymentID string, paymentCents int64) (charge.AllocationMethod, []charge.Allocation, error) {
	f.calls++
	if c != nil && len(c.Payments) > 0 {
		copied := c.Payments[0]
		f.snapshotPayment = &copied
	}
	admin := paymentCents / 10
	owner := paymentCents - admin - f.intermediation
	allocs := []charge.Allocation{
		{ID: "alloc-admin", PaymentID: paymentID, ChargeItemID: "item-1", BeneficiaryPartyID: "party-1", RuleVersionID: "rule-1", AmountCents: admin, Purpose: charge.AllocationPurposeAdminFee, AllocationType: charge.AllocationTypeRevenue},
		{ID: "alloc-owner", PaymentID: paymentID, ChargeItemID: "item-1", BeneficiaryPartyID: "party-1", RuleVersionID: "rule-1", AmountCents: owner, Purpose: charge.AllocationPurposeOwnerRent, AllocationType: charge.AllocationTypeOwnerPayable},
	}
	if f.intermediation > 0 {
		allocs = append(allocs, charge.Allocation{
			ID: "alloc-intermediation", PaymentID: paymentID, ChargeItemID: "item-1", BeneficiaryPartyID: "party-1", RuleVersionID: "rule-1",
			AmountCents: f.intermediation, RelatedEntityID: "commission-1",
			Purpose: charge.AllocationPurposeRentalIntermediationBroker, AllocationType: charge.AllocationTypeCommissionPayable,
		})
	}
	return charge.AllocationMethodProportionalV1, allocs, nil
}

type fakeTransferHook struct {
	called *charge.Charge
}

func (f *fakeTransferHook) EnsureMonthlyForChargePaid(_ context.Context, c *charge.Charge) error {
	f.called = c
	return nil
}

// --- helpers ---

func rentalRepairCharge() *charge.Charge {
	paid := valueobjects.NewMoney(280000)
	return &charge.Charge{
		ID:             "charge-1",
		TenantID:       "tenant-1",
		ContractID:     "contract-1",
		Type:           charge.TypeRent,
		Status:         charge.StatusPaid,
		ReferenceMonth: "2026-11",
		TotalAmount:    valueobjects.NewMoney(280000),
		PaidAmount:     &paid,
		LineItems:      []charge.LineItem{{ID: "item-1", AmountCents: 280000}},
		Payments: []charge.PaymentEntry{{
			ID: "payment-1", AmountCents: 280000, Method: "pix", PaidAt: time.Now(),
			Allocations: []charge.Allocation{
				{ID: "old-owner", PaymentID: "payment-1", AmountCents: 252000, Purpose: charge.AllocationPurposeOwnerRent, AllocationType: charge.AllocationTypeOwnerPayable},
				{ID: "old-admin", PaymentID: "payment-1", AmountCents: 28000, Purpose: charge.AllocationPurposeAdminFee, AllocationType: charge.AllocationTypeRevenue},
			},
		}},
	}
}

func frozenPlan(remaining int64) *rental_charge_config.RentalChargeConfig {
	return &rental_charge_config.RentalChargeConfig{
		ID: "cfg-1", TenantID: "tenant-1", ContractID: "contract-1",
		IntermediationPlan: &rental_charge_config.IntermediationPlan{
			PlanHash: "hash", TotalAmountCents: 280000,
			Shares: []rental_charge_config.IntermediationShare{
				{BrokerID: "broker-1", Role: "captador", CommissionID: "comm-1", PayoutOrderID: "payout-1", TargetAmountCents: 280000, FundedAmountCents: 280000 - remaining},
			},
		},
	}
}

// --- tests ---

func TestRepairRentalIntermediationAllocations_HappyPath(t *testing.T) {
	repo := &fakeChargeRepo{charge: rentalRepairCharge()}
	cfgRepo := &fakeConfigRepo{cfg: frozenPlan(280000)}
	rebuilder := &fakeAllocationRebuilder{intermediation: 200000}
	journalRepo := newFakeJournalRepo()
	stale, err := journal.New("tenant-1", "contract-1", "Recebimento alocado da cobrança charge-1", time.Now(), []journal.Leg{
		{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: 280000, Flow: "rent"},
		{Account: journal.AccountOwnerPayable, Direction: journal.Credit, AmountCents: 252000, Flow: "rent"},
		{Account: journal.AccountRevenue, Direction: journal.Credit, AmountCents: 28000, Flow: "rent"},
	})
	if err != nil {
		t.Fatalf("stale journal: %v", err)
	}
	stale.SourceEventID = events.DeriveNotificationID("charge.payment.journal", "tenant-1", "charge-1", "payment-1")
	if err := journalRepo.Create(context.Background(), stale); err != nil {
		t.Fatalf("seed stale journal: %v", err)
	}
	hook := &fakeTransferHook{}
	svc := &ChargeService{
		repo:               repo,
		rentalConfigRepo:   cfgRepo,
		allocationResolver: rebuilder,
		paymentJournal:     NewJournalService(journalRepo),
		transferHook:       hook,
	}

	result, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", "contrato assinado antes do plano")
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if result.JournalEntriesReversed != 1 {
		t.Fatalf("lançamento antigo deveria ser estornado: %+v", result)
	}
	if result.JournalEntriesReposted != 1 {
		t.Fatalf("lançamento corrigido deveria ser gravado: %+v", result)
	}
	if len(result.RepairedPayments) != 1 || result.RepairedPayments[0] != "payment-1" {
		t.Fatalf("repaired payments = %v", result.RepairedPayments)
	}
	if result.IntermediationFundedCents != 200000 {
		t.Fatalf("funded = %d, want 200000", result.IntermediationFundedCents)
	}
	// O snapshot entregue ao rebuild NÃO pode conter as alocações antigas.
	if rebuilder.snapshotPayment == nil || len(rebuilder.snapshotPayment.Allocations) != 0 {
		t.Fatalf("snapshot deveria vir sem alocações, veio %+v", rebuilder.snapshotPayment)
	}
	if repo.updates == 0 {
		t.Fatal("cobrança reparada não foi persistida")
	}
	if hook.called == nil {
		t.Fatal("repasse não foi reconciliado")
	}
	// A cobrança persistida passou a ter a fatia de intermediação.
	if got := intermediationAllocatedCents(&repo.charge.Payments[0]); got != 200000 {
		t.Fatalf("alocação de intermediação ausente na cobrança persistida: %d", got)
	}
}

func TestRepairRentalIntermediationAllocations_SkipsAlreadyIntermediated(t *testing.T) {
	c := rentalRepairCharge()
	c.Payments[0].Allocations = append(c.Payments[0].Allocations, charge.Allocation{
		ID: "already", PaymentID: "payment-1", AmountCents: 0,
		Purpose: charge.AllocationPurposeRentalIntermediationBroker, AllocationType: charge.AllocationTypeCommissionPayable,
	})
	repo := &fakeChargeRepo{charge: c}
	rebuilder := &fakeAllocationRebuilder{intermediation: 200000}
	svc := &ChargeService{
		repo:               repo,
		rentalConfigRepo:   &fakeConfigRepo{cfg: frozenPlan(280000)},
		allocationResolver: rebuilder,
	}

	result, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", "reparo")
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if rebuilder.calls != 0 {
		t.Fatalf("rebuild não deveria rodar para pagamento já intermediado (calls=%d)", rebuilder.calls)
	}
	if result.SkippedReason == "" {
		t.Fatal("esperado motivo de skip")
	}
}

func TestRepairRentalIntermediationAllocations_SkipsFundedPlan(t *testing.T) {
	svc := &ChargeService{
		repo:               &fakeChargeRepo{charge: rentalRepairCharge()},
		rentalConfigRepo:   &fakeConfigRepo{cfg: frozenPlan(0)}, // nada a financiar
		allocationResolver: &fakeAllocationRebuilder{intermediation: 280000},
	}
	result, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", "reparo")
	if err != nil {
		t.Fatalf("repair: %v", err)
	}
	if result.SkippedReason == "" || len(result.RepairedPayments) != 0 {
		t.Fatalf("plano já financiado deveria ser skip: %+v", result)
	}
}

func TestRepairRentalIntermediationAllocations_ToleratesBenignManagedBillConflict(t *testing.T) {
	// O hook reconcilia o repasse antes de re-afirmar os managed bills; um
	// conflito de identidade (data civil) nesses bills não pode derrubar uma
	// correção já aplicada — deve virar aviso.
	repo := &fakeChargeRepo{charge: rentalRepairCharge()}
	svc := &ChargeService{
		repo:               repo,
		rentalConfigRepo:   &fakeConfigRepo{cfg: frozenPlan(280000)},
		allocationResolver: &fakeAllocationRebuilder{intermediation: 200000},
		transferHook:       &failingManagedBillHook{},
	}
	result, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", "reparo")
	if err != nil {
		t.Fatalf("conflito benigno de managed bill não deve ser fatal: %v", err)
	}
	if !result.TransferReconciled {
		t.Fatal("repasse deveria constar reconciliado")
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("esperado 1 aviso, veio %v", result.Warnings)
	}
}

func TestRepairRentalIntermediationAllocations_FatalTransferErrorStillFails(t *testing.T) {
	svc := &ChargeService{
		repo:               &fakeChargeRepo{charge: rentalRepairCharge()},
		rentalConfigRepo:   &fakeConfigRepo{cfg: frozenPlan(280000)},
		allocationResolver: &fakeAllocationRebuilder{intermediation: 200000},
		transferHook:       &failingTransferHook{},
	}
	if _, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", "reparo"); err == nil {
		t.Fatal("erro real de repasse deve continuar fatal")
	}
}

type failingManagedBillHook struct{}

func (f *failingManagedBillHook) EnsureMonthlyForChargePaid(context.Context, *charge.Charge) error {
	return errors.New("ensure managed bill for charge item x: managed bill source key reused with different economic data: managed bill already exists")
}

type failingTransferHook struct{}

func (f *failingTransferHook) EnsureMonthlyForChargePaid(context.Context, *charge.Charge) error {
	return errors.New("database offline")
}

func TestRepairRentalIntermediationAllocations_RequiresPaidRentCharge(t *testing.T) {
	c := rentalRepairCharge()
	c.Status = charge.StatusPending
	svc := &ChargeService{
		repo:               &fakeChargeRepo{charge: c},
		rentalConfigRepo:   &fakeConfigRepo{cfg: frozenPlan(280000)},
		allocationResolver: &fakeAllocationRebuilder{intermediation: 280000},
	}
	if _, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", "reparo"); err == nil {
		t.Fatal("cobrança não quitada deve ser recusada")
	}
	if _, err := svc.RepairRentalIntermediationAllocations(context.Background(), "tenant-1", "charge-1", "operator-1", ""); err == nil {
		t.Fatal("motivo é obrigatório (trilha de auditoria)")
	}
}

func TestCheckRentalIntermediationRepairEligibility(t *testing.T) {
	base := func(chargeStatus charge.Status, cfg *rental_charge_config.RentalChargeConfig, intermediated bool) *ChargeService {
		c := rentalRepairCharge()
		c.Status = chargeStatus
		if intermediated {
			c.Payments[0].Allocations = append(c.Payments[0].Allocations, charge.Allocation{
				ID: "already", PaymentID: "payment-1", ChargeItemID: "item-1", BeneficiaryPartyID: "party-1",
				RuleVersionID: "rule-1", AmountCents: 0, RelatedEntityID: "commission-1",
				Purpose: charge.AllocationPurposeRentalIntermediationBroker, AllocationType: charge.AllocationTypeCommissionPayable,
			})
		}
		return &ChargeService{
			repo:             &fakeChargeRepo{charge: c},
			rentalConfigRepo: &fakeConfigRepo{cfg: cfg},
		}
	}

	t.Run("elegível quando há plano congelado com saldo e pagamento sem intermediação", func(t *testing.T) {
		result, err := base(charge.StatusPaid, frozenPlan(280000), false).
			CheckRentalIntermediationRepairEligibility(context.Background(), "tenant-1", "charge-1")
		if err != nil {
			t.Fatalf("eligibility: %v", err)
		}
		if !result.Eligible || result.PendingPayments != 1 {
			t.Fatalf("esperado elegível com 1 pagamento pendente: %+v", result)
		}
		if result.OutstandingCents != 280000 || result.FundedCents != 0 || result.PlannedCents != 280000 {
			t.Fatalf("valores do plano inesperados: %+v", result)
		}
	})

	cases := []struct {
		name   string
		svc    *ChargeService
		reason string
	}{
		{"não quitada", base(charge.StatusPending, frozenPlan(280000), false), "cobrança ainda não foi quitada"},
		{"plano já financiado", base(charge.StatusPaid, frozenPlan(0), false), "intermediação já integralmente financiada"},
		{"pagamento já intermedia do", base(charge.StatusPaid, frozenPlan(280000), true), "pagamentos já contemplam a intermediação"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := tc.svc.CheckRentalIntermediationRepairEligibility(context.Background(), "tenant-1", "charge-1")
			if err != nil {
				t.Fatalf("eligibility: %v", err)
			}
			if result.Eligible {
				t.Fatalf("não deveria ser elegível: %+v", result)
			}
			if result.Reason != tc.reason {
				t.Fatalf("reason = %q, want %q", result.Reason, tc.reason)
			}
		})
	}

	t.Run("cobrança de venda não é elegível", func(t *testing.T) {
		c := rentalRepairCharge()
		c.Type = charge.TypeSale
		svc := &ChargeService{repo: &fakeChargeRepo{charge: c}, rentalConfigRepo: &fakeConfigRepo{cfg: frozenPlan(280000)}}
		result, err := svc.CheckRentalIntermediationRepairEligibility(context.Background(), "tenant-1", "charge-1")
		if err != nil || result.Eligible || result.Reason != "cobrança não é de locação" {
			t.Fatalf("esperado skip de venda: %+v err=%v", result, err)
		}
	})
}

func TestPaymentMissingIntermediation(t *testing.T) {
	withIntermediation := &charge.PaymentEntry{AmountCents: 100, Allocations: []charge.Allocation{
		{AmountCents: 100, Purpose: charge.AllocationPurposeRentalIntermediationAgency},
	}}
	if paymentMissingIntermediation(withIntermediation) {
		t.Error("pagamento com intermediação não deve precisar de reparo")
	}
	withoutIntermediation := &charge.PaymentEntry{AmountCents: 100, Allocations: []charge.Allocation{
		{AmountCents: 100, Purpose: charge.AllocationPurposeOwnerRent},
	}}
	if !paymentMissingIntermediation(withoutIntermediation) {
		t.Error("pagamento sem intermediação deve precisar de reparo")
	}
	// Sem alocação nenhuma é o reparo histórico (outro caminho).
	if paymentMissingIntermediation(&charge.PaymentEntry{AmountCents: 100}) {
		t.Error("pagamento sem alocações não entra neste reparo")
	}
	if paymentMissingIntermediation(nil) {
		t.Error("nil não deve precisar de reparo")
	}
}
