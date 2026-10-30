package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/clients"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

type ownerStatementTransferRepo struct {
	items  []*transfer.OwnerTransfer
	filter transfer.SearchFilter
}

func (r *ownerStatementTransferRepo) Create(context.Context, *transfer.OwnerTransfer) error {
	return nil
}
func (r *ownerStatementTransferRepo) GetByID(context.Context, string, string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}
func (r *ownerStatementTransferRepo) GetByContractMonth(context.Context, string, string, string) (*transfer.OwnerTransfer, error) {
	return nil, nil
}
func (r *ownerStatementTransferRepo) Update(context.Context, *transfer.OwnerTransfer) error {
	return nil
}
func (r *ownerStatementTransferRepo) List(context.Context, string, string, string, string, string, int64, int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *ownerStatementTransferRepo) ListByOwnerLeadIDs(context.Context, string, []string, transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *ownerStatementTransferRepo) UpdateStatus(context.Context, string, string, transfer.Status) error {
	return nil
}
func (r *ownerStatementTransferRepo) Search(_ context.Context, _ string, filter transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	r.filter = filter
	return r.items, int64(len(r.items)), nil
}

func TestOwnerStatementAggregatesOnlyAuthoritativeOwnerBalances(t *testing.T) {
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		{OwnerID: "owner-1", ReferenceMonth: "2026-08", GrossRent: 5000, AdminFeeAmount: 400, NetAmount: 4600, OwnerBalance: &balance},
		// Legacy transfer remains in ordinary totals but must not be guessed into
		// the allocation-aware owner-balance summary.
		{OwnerID: "owner-1", ReferenceMonth: "2026-08", GrossRent: 1000, AdminFeeAmount: 100, NetAmount: 900},
	}}
	statement, err := NewOwnerStatementService(repo, nil, nil).GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 8)
	if err != nil {
		t.Fatal(err)
	}
	if repo.filter.OwnerID != "owner-1" || repo.filter.ReferenceMonth != "2026-08" {
		t.Fatalf("unexpected repository filter: %+v", repo.filter)
	}
	if statement.GrossRent != 6000 || statement.AdminFeeAmount != 500 || statement.NetAmount != 5500 {
		t.Fatalf("legacy transfer totals changed: %+v", statement)
	}
	if statement.OwnerBalanceTotal != 5600 || statement.ReservedAmount != 1000 || statement.BlockedAmount != 0 || statement.AvailableAmount != 4600 || statement.BalanceCoveredCount != 1 {
		t.Fatalf("owner balance summary = %+v", statement)
	}
}

type ownerStatementManagedBillRepo struct {
	items  []*managed_bill.ManagedBill
	filter managed_bill.Filter
}

func (r *ownerStatementManagedBillRepo) Create(context.Context, *managed_bill.ManagedBill) error {
	return nil
}
func (r *ownerStatementManagedBillRepo) GetByID(context.Context, string, string) (*managed_bill.ManagedBill, error) {
	return nil, nil
}
func (r *ownerStatementManagedBillRepo) GetBySourceKey(context.Context, string, string) (*managed_bill.ManagedBill, error) {
	return nil, nil
}
func (r *ownerStatementManagedBillRepo) SumActiveReservedCentsByTransfer(context.Context, string, string) (int64, error) {
	return 0, nil
}
func (r *ownerStatementManagedBillRepo) UpdateIfVersion(context.Context, *managed_bill.ManagedBill, int64) (bool, error) {
	return false, nil
}
func (r *ownerStatementManagedBillRepo) List(_ context.Context, _ string, filter managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error) {
	r.filter = filter
	return r.items, int64(len(r.items)), nil
}

type ownerStatementJournalRepo struct {
	items      []*journal.Entry
	listFilter struct {
		TenantID   string
		ContractID string
	}
}

func (r *ownerStatementJournalRepo) Create(context.Context, *journal.Entry) error {
	return nil
}
func (r *ownerStatementJournalRepo) GetByID(context.Context, string, string) (*journal.Entry, error) {
	return nil, nil
}
func (r *ownerStatementJournalRepo) GetBySourceEventID(context.Context, string, string) (*journal.Entry, error) {
	return nil, nil
}
func (r *ownerStatementJournalRepo) UpdateStatus(context.Context, *journal.Entry, time.Time) (bool, error) {
	return false, nil
}
func (r *ownerStatementJournalRepo) List(ctx context.Context, tenantID, contractID string, limit, offset int64) ([]*journal.Entry, int64, error) {
	r.listFilter.TenantID = tenantID
	r.listFilter.ContractID = contractID
	return r.items, int64(len(r.items)), nil
}
func (r *ownerStatementJournalRepo) AccountBalance(context.Context, string, journal.Account) (int64, error) {
	return 0, nil
}

func TestOwnerStatementIncludesManagedBillsAndJournalTotals(t *testing.T) {
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	due := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	mbRepo := &ownerStatementManagedBillRepo{items: []*managed_bill.ManagedBill{{
		ID: "mb-1", ContractID: "c-1", Component: "iptu", Creditor: "Prefeitura",
		Description: "IPTU 2026", AmountCents: 10000, DueDate: due,
		CompetenceMonth: "2026-08", Status: managed_bill.StatusReserved,
	}}}
	jRepo := &ownerStatementJournalRepo{items: []*journal.Entry{{
		Legs: []journal.Leg{
			{Account: journal.AccountRevenue, Direction: journal.Debit, AmountCents: 400000},
			{Account: journal.AccountOwnerPayable, Direction: journal.Credit, AmountCents: 360000},
			{Account: journal.AccountCash, Direction: journal.Debit, AmountCents: 400000},
		},
	}}}
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{{
		ID: "t-1", OwnerID: "owner-1", ContractID: "c-1", ReferenceMonth: "2026-08",
		GrossRent: 5000, AdminFeeAmount: 400, NetAmount: 4600, OwnerBalance: &balance,
	}}}
	stmt, err := NewOwnerStatementService(repo, mbRepo, jRepo).GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 8)
	if err != nil {
		t.Fatal(err)
	}
	if mbRepo.filter.ContractID != "c-1" || mbRepo.filter.CompetenceMonth != "2026-08" {
		t.Fatalf("unexpected managed bill filter: %+v", mbRepo.filter)
	}
	if len(stmt.ManagedBills) != 1 || stmt.ManagedBills[0].ID != "mb-1" || stmt.ManagedBills[0].Status != "RESERVED" || stmt.ManagedBillsTotal != 100 {
		t.Fatalf("managed bills = %+v total=%v", stmt.ManagedBills, stmt.ManagedBillsTotal)
	}
	if jRepo.listFilter.ContractID != "c-1" {
		t.Fatalf("unexpected journal filter: %+v", jRepo.listFilter)
	}
	totals := stmt.JournalTotals
	if totals.Revenue != 4000 || totals.OwnerPayable != -3600 || totals.Cash != 4000 || totals.AccountsReceivable != 0 {
		t.Fatalf("journal totals = %+v", totals)
	}
}

func TestOwnerStatementLatestBalancePicksNewestRowWithSubledger(t *testing.T) {
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	latest := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		// Mais recente, mas sem subledger (rascunho): deve ser pulado.
		{ID: "t-2", OwnerID: "owner-1", ReferenceMonth: "2026-09", UpdatedAt: latest},
		{ID: "t-1", OwnerID: "owner-1", ReferenceMonth: "2026-08", OwnerBalance: &balance, UpdatedAt: latest.Add(-time.Hour)},
	}}
	summary, err := NewOwnerStatementService(repo, nil, nil).LatestBalance(context.Background(), "tenant-1", "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if !summary.HasBalance || summary.TransferID != "t-1" || summary.ReferenceMonth != "2026-08" {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if summary.TotalCents != 560000 || summary.ReservedCents != 100000 || summary.BlockedCents != 0 || summary.AvailableCents != 460000 {
		t.Fatalf("balance cents = %+v", summary)
	}
}

func TestOwnerStatementLatestBalanceWithoutSubledger(t *testing.T) {
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		{ID: "t-1", OwnerID: "owner-1", ReferenceMonth: "2026-08"},
	}}
	summary, err := NewOwnerStatementService(repo, nil, nil).LatestBalance(context.Background(), "tenant-1", "owner-1")
	if err != nil {
		t.Fatal(err)
	}
	if summary.HasBalance {
		t.Fatalf("expected HasBalance=false, got %+v", summary)
	}
	if summary.TotalCents != 0 || summary.AvailableCents != 0 {
		t.Fatalf("expected zero balance, got %+v", summary)
	}
}

type statementContractLookupStub struct {
	calls    int
	tenantID string
	err      error
}

func (s *statementContractLookupStub) GetContract(_ context.Context, tenantID, _ string) (*clients.ContractDTO, error) {
	s.calls++
	s.tenantID = tenantID
	return &clients.ContractDTO{Type: "SALE"}, s.err
}

func TestOwnerStatementSegmentsSaleRentalAndUnknown(t *testing.T) {
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		{ID: "sale-1", SettlementType: "sale", GrossRent: 1130000, NetAmount: 1062200,
			EconomicBreakdown: &transfer.EconomicBreakdown{Flow: "sale", AgencyRevenueCents: 6780000, AgencyRetainedCents: 2712000, CommissionPayableCents: 4068000, CommissionFundedCents: 4068000}},
		{ID: "rent-1", EconomicBreakdown: &transfer.EconomicBreakdown{Flow: "rent"}, GrossRent: 5000, AdminFeeAmount: 400, MaintenanceDeductions: 100, NetAmount: 4500},
		{ID: "legacy-1", ContractID: "legacy-sale", GrossRent: 2000, NetAmount: 1900},
		{ID: "legacy-2", ContractID: "legacy-sale", GrossRent: 3000, NetAmount: 2800},
		{ID: "unknown-1", GrossRent: 1000, NetAmount: 900},
	}}
	lookup := &statementContractLookupStub{}
	stmt, err := NewOwnerStatementService(repo, nil, nil).WithContractLookup(lookup).GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if lookup.calls != 1 || lookup.tenantID != "tenant-1" {
		t.Fatalf("lookup = %+v", lookup)
	}
	if len(stmt.FlowSummaries) != 3 {
		t.Fatalf("summaries = %+v", stmt.FlowSummaries)
	}
	sale, rent, unknown := stmt.FlowSummaries[0], stmt.FlowSummaries[1], stmt.FlowSummaries[2]
	if sale.Flow != "sale" || sale.TransferCount != 3 || sale.GrossAmount != 1135000 || sale.NetAmount != 1066900 || sale.BrokerageAmount != nil {
		t.Fatalf("sale = %+v", sale)
	}
	if rent.Flow != "rent" || rent.GrossAmount != 5000 || rent.AdminFeeAmount != 400 || rent.MaintenanceDeductions != 100 || rent.NetAmount != 4500 {
		t.Fatalf("rent = %+v", rent)
	}
	if unknown.Flow != "unknown" || unknown.GrossAmount != 1000 || unknown.NetAmount != 900 {
		t.Fatalf("unknown = %+v", unknown)
	}
	if stmt.Transfers[0].BrokerageAmount == nil || *stmt.Transfers[0].BrokerageAmount != 67800 {
		t.Fatalf("funded brokerage counted twice: %+v", stmt.Transfers[0])
	}
	if repo.items[2].SettlementType != "" {
		t.Fatal("statement enrichment mutated stored transfer")
	}
	if sale.GrossAmount+rent.GrossAmount+unknown.GrossAmount != stmt.GrossRent || sale.NetAmount+rent.NetAmount+unknown.NetAmount != stmt.NetAmount {
		t.Fatal("segments do not reconcile with totals")
	}
	payload, err := json.Marshal(stmt)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Transfers []struct {
			ID   string `json:"id"`
			Flow string `json:"statement_flow"`
		} `json:"transfers"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Transfers[0].ID != "sale-1" || decoded.Transfers[0].Flow != "sale" {
		t.Fatalf("embedded transfer contract lost: %s", payload)
	}
}

func TestOwnerStatementFailedLegacyLookupDoesNotInventRental(t *testing.T) {
	lookup := &statementContractLookupStub{err: errors.New("deal unavailable")}
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{{ContractID: "legacy", GrossRent: 2000, NetAmount: 1900}}}
	stmt, err := NewOwnerStatementService(repo, nil, nil).WithContractLookup(lookup).GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	if stmt.FlowSummaries[0].Flow != "unknown" {
		t.Fatal("failed lookup fabricated rental")
	}
}

func TestOwnerStatementPDFMixedFlows(t *testing.T) {
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		{ID: "sale", ContractID: "contrato-venda", SettlementType: "sale", GrossRent: 1130000, NetAmount: 1062200, Status: transfer.StatusPending,
			EconomicBreakdown: &transfer.EconomicBreakdown{Flow: "sale", AgencyRetainedCents: 2712000, CommissionPayableCents: 4068000}},
		{ID: "rent", ContractID: "contrato-locacao", SettlementType: "rent", GrossRent: 5000, AdminFeeAmount: 400, NetAmount: 4600, Status: transfer.StatusProcessed},
		{ID: "legacy", ContractID: "contrato-legado", GrossRent: 2000, NetAmount: 1900, Status: transfer.StatusPending},
	}}
	stmt, err := NewOwnerStatementService(repo, nil, nil).GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 9)
	if err != nil {
		t.Fatal(err)
	}
	data, err := RenderMonthlyOwnerStatementPDF(OwnerStatementPDFContext{Statement: stmt, OwnerName: "Márcia Gonçalves"})
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 1000 {
		t.Fatal("empty PDF")
	}
	// Optional output used for visual QA, not a production write.
	if path := os.Getenv("OWNER_STATEMENT_QA_PDF"); path != "" {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

// fakeRentalConfigLookup devolve o plano congelado de intermediação por contrato.
type fakeRentalConfigLookup struct {
	byContract map[string]*rental_charge_config.RentalChargeConfig
	calls      int
}

func (f *fakeRentalConfigLookup) GetByContractID(_ context.Context, _, contractID string) (*rental_charge_config.RentalChargeConfig, error) {
	f.calls++
	return f.byContract[contractID], nil
}

// O demonstrativo mostrava só a taxa de administração: o proprietário via
// "aluguel bruto − taxa" e não fechava com o líquido, porque a comissão do
// primeiro aluguel (intermediação) também sai do aluguel nos primeiros meses.
func TestOwnerStatementExposesIntermediationDiscountAndOutstanding(t *testing.T) {
	balance, err := transfer.NewOwnerBalance(206128, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		{
			ID:             "transfer-1",
			OwnerID:        "owner-1",
			ContractID:     "contract-1",
			ReferenceMonth: "2026-10",
			GrossRent:      2800,
			AdminFeeAmount: 280,
			NetAmount:      2061.28,
			OwnerBalance:   &balance,
			SettlementType: "rent",
			EconomicBreakdown: &transfer.EconomicBreakdown{
				Flow:                       "rent",
				GrossCents:                 280000,
				OwnerPayableCents:          206128,
				AdminFeeCents:              28000,
				IntermediationRevenueCents: 45744,
				CommissionFundedCents:      36128,
			},
		},
	}}

	plan := &rental_charge_config.IntermediationPlan{
		PlanHash:         "hash-1",
		TotalAmountCents: 243872,
		Shares: []rental_charge_config.IntermediationShare{
			{BrokerID: "b1", TargetAmountCents: 243872, FundedAmountCents: 36128},
		},
	}
	lookup := &fakeRentalConfigLookup{byContract: map[string]*rental_charge_config.RentalChargeConfig{
		"contract-1": {ID: "cfg-1", ContractID: "contract-1", IntermediationPlan: plan},
	}}

	statement, err := NewOwnerStatementService(repo, nil, nil).
		WithIntermediationLookup(lookup).
		GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 10)
	if err != nil {
		t.Fatalf("GenerateMonthly: %v", err)
	}

	if statement.IntermediationAmount != 457.44 {
		t.Fatalf("intermediação do período = %v, want 457.44", statement.IntermediationAmount)
	}
	if statement.IntermediationCommissionFundedAmount != 361.28 {
		t.Fatalf("parte para comissões = %v, want 361.28", statement.IntermediationCommissionFundedAmount)
	}
	// Falta cobrir 2.438,72 − 361,28 = 2.077,44 nos próximos repasses.
	if statement.IntermediationOutstandingAmount != 2077.44 {
		t.Fatalf("saldo de intermediação = %v, want 2077.44", statement.IntermediationOutstandingAmount)
	}

	line := statement.Transfers[0]
	if line.IntermediationAmount == nil || *line.IntermediationAmount != 457.44 {
		t.Fatalf("linha do repasse sem intermediação: %+v", line.IntermediationAmount)
	}
	if line.IntermediationCommissionFundedAmount == nil || *line.IntermediationCommissionFundedAmount != 361.28 {
		t.Fatalf("linha do repasse sem comissão financiada: %+v", line.IntermediationCommissionFundedAmount)
	}
	if line.IntermediationOutstandingAmount == nil || *line.IntermediationOutstandingAmount != 2077.44 {
		t.Fatalf("linha do repasse sem saldo de intermediação: %+v", line.IntermediationOutstandingAmount)
	}

	// Resumo por modalidade carrega os mesmos valores (é o que a UI/PDF mostram).
	var rent StatementFlowSummary
	for _, summary := range statement.FlowSummaries {
		if summary.Flow == "rent" {
			rent = summary
		}
	}
	if rent.IntermediationAmount != 457.44 || rent.IntermediationCommissionFundedAmount != 361.28 {
		t.Fatalf("resumo da locação sem intermediação: %+v", rent)
	}
}

// Repasse legado (sem breakdown) não inventa intermediação nem quebra o resumo.
func TestOwnerStatementWithoutIntermediationKeepsLegacyNumbers(t *testing.T) {
	repo := &ownerStatementTransferRepo{items: []*transfer.OwnerTransfer{
		{ID: "transfer-legacy", OwnerID: "owner-1", ReferenceMonth: "2026-10", GrossRent: 1000, AdminFeeAmount: 100, NetAmount: 900, SettlementType: "rent"},
	}}
	statement, err := NewOwnerStatementService(repo, nil, nil).GenerateMonthly(context.Background(), "tenant-1", "owner-1", 2026, 10)
	if err != nil {
		t.Fatalf("GenerateMonthly: %v", err)
	}
	if statement.IntermediationAmount != 0 || statement.IntermediationOutstandingAmount != 0 {
		t.Fatalf("legado não deve ganhar intermediação: %+v", statement)
	}
	if statement.Transfers[0].IntermediationAmount != nil {
		t.Fatalf("linha legada deve ficar sem intermediação (nil = não se aplica)")
	}
	if statement.NetAmount != 900 {
		t.Fatalf("líquido legado alterado: %v", statement.NetAmount)
	}
}
