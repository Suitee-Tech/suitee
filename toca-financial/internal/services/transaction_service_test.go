package services

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// storingTxRepo is a minimal in-memory transaction.RepositoryInterface that
// actually persists Update, so settlement assertions are observable.
type storingTxRepo struct {
	byID        map[string]*transaction.Transaction
	batchCalls  int
	updateCalls int
}

func newStoringTxRepo() *storingTxRepo {
	return &storingTxRepo{byID: map[string]*transaction.Transaction{}}
}

func (r *storingTxRepo) Create(_ context.Context, t *transaction.Transaction) error {
	if t.SourceEventID != "" {
		for _, existing := range r.byID {
			if existing.TenantID == t.TenantID && existing.SourceEventID == t.SourceEventID {
				return transaction.ErrTransactionAlreadyExists
			}
		}
	}
	r.byID[t.ID] = t
	return nil
}

func (r *storingTxRepo) CreateBatch(_ context.Context, txs []*transaction.Transaction) error {
	r.batchCalls++
	for _, tx := range txs {
		duplicate := false
		for _, existing := range r.byID {
			if existing.TenantID == tx.TenantID && existing.SourceEventID == tx.SourceEventID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			r.byID[tx.ID] = tx
		}
	}
	return nil
}

func (r *storingTxRepo) GetByID(_ context.Context, tenantID, id string) (*transaction.Transaction, error) {
	tx := r.byID[id]
	if tx == nil || tx.TenantID != tenantID {
		return nil, nil
	}
	return tx, nil
}

func (r *storingTxRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*transaction.Transaction, error) {
	for _, t := range r.byID {
		if t.TenantID == tenantID && t.SourceEventID == sourceEventID {
			return t, nil
		}
	}
	return nil, nil
}

func (r *storingTxRepo) ListByOperationID(_ context.Context, tenantID, operationID string) ([]*transaction.Transaction, error) {
	var matches []*transaction.Transaction
	for _, item := range r.byID {
		if item != nil && item.TenantID == tenantID && item.OperationID == operationID {
			matches = append(matches, item)
		}
	}
	return matches, nil
}

func (r *storingTxRepo) Update(_ context.Context, t *transaction.Transaction) error {
	r.updateCalls++
	r.byID[t.ID] = t
	return nil
}

func (r *storingTxRepo) Delete(_ context.Context, id, _ string) error {
	delete(r.byID, id)
	return nil
}

func (r *storingTxRepo) List(_ context.Context, _, _ string, _, _ int64) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}

func (r *storingTxRepo) Search(_ context.Context, _ string, _ transaction.SearchFilter) ([]*transaction.Transaction, int64, error) {
	return nil, 0, nil
}

type storingManualCommissionRepo struct {
	byID map[string]*commission.Commission
}

func newStoringManualCommissionRepo() *storingManualCommissionRepo {
	return &storingManualCommissionRepo{byID: map[string]*commission.Commission{}}
}

func (r *storingManualCommissionRepo) Create(_ context.Context, c *commission.Commission) error {
	for _, existing := range r.byID {
		if existing.TenantID == c.TenantID && existing.SourceEventID != "" && existing.SourceEventID == c.SourceEventID {
			return commission.ErrCommissionAlreadyExists
		}
	}
	r.byID[c.ID] = c
	return nil
}

func (r *storingManualCommissionRepo) GetByID(_ context.Context, tenantID, id string) (*commission.Commission, error) {
	c := r.byID[id]
	if c == nil || c.TenantID != tenantID {
		return nil, nil
	}
	return c, nil
}

func (r *storingManualCommissionRepo) GetBySourceEventID(_ context.Context, tenantID, sourceEventID string) (*commission.Commission, error) {
	for _, c := range r.byID {
		if c.TenantID == tenantID && c.SourceEventID == sourceEventID {
			return c, nil
		}
	}
	return nil, nil
}

func (r *storingManualCommissionRepo) GetByLedgerTransactionID(_ context.Context, tenantID, ledgerTransactionID string) (*commission.Commission, error) {
	for _, item := range r.byID {
		if item != nil && item.TenantID == tenantID && item.LedgerTransactionID == ledgerTransactionID {
			return item, nil
		}
	}
	return nil, nil
}

func (r *storingManualCommissionRepo) List(_ context.Context, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	return nil, 0, nil
}
func (r *storingManualCommissionRepo) UpdateStatus(_ context.Context, _, _ string, _ commission.Status) error {
	return nil
}
func (r *storingManualCommissionRepo) UpdateStatusByDealID(_ context.Context, _, _ string, _ commission.Status) error {
	return nil
}
func (r *storingManualCommissionRepo) Update(_ context.Context, c *commission.Commission) error {
	r.byID[c.ID] = c
	return nil
}
func (r *storingManualCommissionRepo) ListByDealID(_ context.Context, _, _ string) ([]*commission.Commission, error) {
	return nil, nil
}
func (r *storingManualCommissionRepo) ListByBrokerID(_ context.Context, _, _ string, _, _ int64) ([]*commission.Commission, int64, error) {
	return nil, 0, nil
}
func (r *storingManualCommissionRepo) Delete(_ context.Context, id, _ string) error {
	delete(r.byID, id)
	return nil
}

type recordingManualReceiptSettlement struct {
	calls            int
	cancelCalls      int
	receipt          *transaction.Transaction
	cancelledReceipt *transaction.Transaction
	err              error
}

func (r *recordingManualReceiptSettlement) EnsureForSettledReceipt(_ context.Context, receipt *transaction.Transaction) error {
	r.calls++
	if receipt != nil {
		copy := *receipt
		if receipt.EconomicBreakdown != nil {
			breakdown := *receipt.EconomicBreakdown
			copy.EconomicBreakdown = &breakdown
		}
		r.receipt = &copy
	}
	return r.err
}

func (r *recordingManualReceiptSettlement) CancelForReceipt(_ context.Context, receipt *transaction.Transaction) error {
	r.cancelCalls++
	if receipt != nil {
		copy := *receipt
		if receipt.EconomicBreakdown != nil {
			breakdown := *receipt.EconomicBreakdown
			copy.EconomicBreakdown = &breakdown
		}
		r.cancelledReceipt = &copy
	}
	return r.err
}

func TestTransactionCreate_AllowsContractlessManualEntry(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	entry := transaction.New("", transaction.TypeReceita, "Recebimento avulso", 500, time.Now(), "tenant-1")
	if err := svc.Create(context.Background(), entry); err != nil {
		t.Fatalf("expected contract-less entry with description to be allowed, got: %v", err)
	}
}

func TestTransactionCreate_RejectsEmptyContractlessEntry(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	entry := transaction.New("", transaction.TypeDespesa, "", 0, time.Now(), "tenant-1")
	entry.Category = ""
	if err := svc.Create(context.Background(), entry); err == nil {
		t.Fatal("expected error for contract-less entry without description or category")
	}
}

func TestTransactionSettle_MarksPago(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	entry := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
	_ = svc.Create(context.Background(), entry)

	pd := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	got, err := svc.Settle(context.Background(), entry.ID, "tenant-1", &pd, "PIX")
	if err != nil {
		t.Fatalf("settle failed: %v", err)
	}
	if got.Status != transaction.StatusPaid {
		t.Fatalf("expected status PAID, got %s", got.Status)
	}
	if got.PaymentDate == nil || !got.PaymentDate.Equal(pd) {
		t.Fatalf("expected payment date %v, got %v", pd, got.PaymentDate)
	}
	if got.PaymentMethod != "PIX" {
		t.Fatalf("expected method PIX, got %s", got.PaymentMethod)
	}
	// Persisted, not just mutated on the returned copy.
	if repo.byID[entry.ID].Status != transaction.StatusPaid {
		t.Fatal("expected settlement to persist via repo.Update")
	}
}

func TestTransactionSettle_TenantMismatchNotFound(t *testing.T) {
	svc := NewTransactionService(newStoringTxRepo())
	entry := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
	_ = svc.Create(context.Background(), entry)

	if _, err := svc.Settle(context.Background(), entry.ID, "tenant-2", nil, ""); err == nil {
		t.Fatal("expected not found for tenant mismatch")
	}
}

func TestSettleReceivableBySourceEventID_SettlesOnlyExactReceivable(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)

	receivable := transaction.New("contract-1", transaction.TypeReceita, "Parcela janeiro", 1000, time.Now(), "tenant-1")
	receivable.SourceEventID = "charge-source-1"
	otherReceivable := transaction.New("contract-1", transaction.TypeReceita, "Parcela fevereiro", 1000, time.Now(), "tenant-1")
	otherReceivable.SourceEventID = "charge-source-2"
	commission := transaction.New("contract-1", transaction.TypeComissao, "Comissão corretor", 60, time.Now(), "tenant-1")
	commission.SourceEventID = "commission-source-1"
	expense := transaction.New("contract-1", transaction.TypeDespesa, "Despesa do contrato", 25, time.Now(), "tenant-1")
	expense.SourceEventID = "expense-source-1"

	for _, tx := range []*transaction.Transaction{receivable, otherReceivable, commission, expense} {
		if err := repo.Create(context.Background(), tx); err != nil {
			t.Fatalf("create transaction: %v", err)
		}
	}

	paidAt := time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC)
	got, err := svc.SettleReceivableBySourceEventID(context.Background(), "tenant-1", "contract-1", "charge-source-1", paidAt, "PIX")
	if err != nil {
		t.Fatalf("settle linked receivable: %v", err)
	}
	if got == nil || got.ID != receivable.ID {
		t.Fatalf("expected linked receivable %s, got %#v", receivable.ID, got)
	}
	if receivable.Status != transaction.StatusPaid {
		t.Fatalf("expected linked receivable PAID, got %s", receivable.Status)
	}
	if receivable.PaymentDate == nil || !receivable.PaymentDate.Equal(paidAt) {
		t.Fatalf("expected payment date %v, got %v", paidAt, receivable.PaymentDate)
	}
	if receivable.PaymentMethod != "PIX" {
		t.Fatalf("expected PIX, got %q", receivable.PaymentMethod)
	}

	for name, tx := range map[string]*transaction.Transaction{
		"other receivable": otherReceivable,
		"commission":       commission,
		"expense":          expense,
	} {
		if tx.Status != transaction.StatusPending {
			t.Errorf("%s must remain PENDING, got %s", name, tx.Status)
		}
	}
}

func TestSettleReceivableBySourceEventID_DoesNotSettleNonReceivableOrGuessByContract(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	commission := transaction.New("contract-1", transaction.TypeComissao, "Comissão corretor", 60, time.Now(), "tenant-1")
	commission.SourceEventID = "shared-source"
	if err := repo.Create(context.Background(), commission); err != nil {
		t.Fatalf("create commission: %v", err)
	}

	got, err := svc.SettleReceivableBySourceEventID(context.Background(), "tenant-1", "contract-1", "shared-source", time.Now(), "PIX")
	if err != nil {
		t.Fatalf("non-receivable link should be a safe no-op: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no settled transaction, got %#v", got)
	}
	if commission.Status != transaction.StatusPending {
		t.Fatalf("commission must remain PENDING, got %s", commission.Status)
	}

	got, err = svc.SettleReceivableBySourceEventID(context.Background(), "tenant-1", "contract-1", "", time.Now(), "PIX")
	if err != nil {
		t.Fatalf("missing link should be a safe no-op: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no contract-wide fallback, got %#v", got)
	}
}

func TestTransactionBatchCreate_RequiresOperationKeyBeforeRepositoryCall(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)

	_, err := svc.BatchCreate(context.Background(), "tenant-1", "   ", []BatchCreateInput{{
		Type:        string(transaction.TypeReceita),
		Description: "Receita manual",
		Value:       100,
		DateStr:     "2026-08-10",
	}})
	if !errors.Is(err, transaction.ErrBatchOperationKeyRequired) {
		t.Fatalf("expected operation-key error, got %v", err)
	}
	if repo.batchCalls != 0 {
		t.Fatalf("repository must not be called, got %d calls", repo.batchCalls)
	}
}

func TestTransactionBatchCreate_ValidatesEveryLineBeforeAnyWrite(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)

	outputs, err := svc.BatchCreate(context.Background(), "tenant-1", "operation-1", []BatchCreateInput{
		{Type: string(transaction.TypeReceita), Description: "Data inválida", Value: 10, DateStr: "10/08/2026"},
		{Type: "not-a-type", Description: "Tipo inválido", Value: 20, DateStr: "2026-08-10"},
		{Type: string(transaction.TypeDespesa), Description: "Linha válida", Value: 30, DateStr: "2026-08-10"},
	})
	if !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("expected batch validation error, got %v", err)
	}
	if len(outputs) != 3 {
		t.Fatalf("expected validation result for every line, got %d", len(outputs))
	}
	if outputs[0].Error == "" || outputs[1].Error == "" {
		t.Fatalf("expected both invalid lines to be reported: %#v", outputs)
	}
	if outputs[1].Transaction != nil {
		t.Fatal("invalid type must not be converted into a transaction")
	}
	if outputs[2].Error != "" {
		t.Fatalf("valid line should have no validation error, got %q", outputs[2].Error)
	}
	if repo.batchCalls != 0 || len(repo.byID) != 0 {
		t.Fatalf("invalid batch must perform no write: calls=%d rows=%d", repo.batchCalls, len(repo.byID))
	}
}

func TestTransactionBatchCreate_RetryReturnsCanonicalDeterministicRows(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	items := []BatchCreateInput{
		{Type: string(transaction.TypeReceita), Description: "Receita", Value: 100, DateStr: "2026-08-10"},
		{Type: string(transaction.TypeDespesa), Description: "Despesa", Value: 25, DateStr: "2026-08-11"},
	}

	first, err := svc.BatchCreate(context.Background(), "tenant-1", "operation-1", items)
	if err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if first[0].Transaction.ID == first[1].Transaction.ID {
		t.Fatal("posting IDs must be unique per line")
	}
	if first[0].Transaction.OperationID == "" || first[0].Transaction.OperationID != first[1].Transaction.OperationID {
		t.Fatalf("all lines must share one operation ID: %#v", first)
	}
	if first[0].Transaction.PostingID != first[0].Transaction.ID || first[1].Transaction.PostingID != first[1].Transaction.ID {
		t.Fatalf("posting_id must expose each persisted line ID: %#v", first)
	}
	if first[0].Transaction.SourceEventID == first[1].Transaction.SourceEventID {
		t.Fatal("source IDs must be unique per line")
	}
	if first[0].Transaction.ID == "" || first[0].Transaction.SourceEventID == "" {
		t.Fatal("posting and source IDs must be populated")
	}
	legacyBatchEventID := events.DeriveNotificationID("operation-1", "batch", "tenant-1")
	legacySourceID := events.DeriveNotificationID(legacyBatchEventID, "batch_line", "0")
	if first[0].Transaction.OperationID != legacyBatchEventID {
		t.Fatalf("operation derivation changed across releases: got %s want %s", first[0].Transaction.OperationID, legacyBatchEventID)
	}
	if first[0].Transaction.SourceEventID != legacySourceID {
		t.Fatalf("source derivation changed across releases: got %s want %s", first[0].Transaction.SourceEventID, legacySourceID)
	}

	// A row written by the previous batch format has the same deterministic
	// ID/source but lacks the newly explicit grouping fields. Its identical
	// retry remains safe and must not become a false idempotency conflict.
	for _, output := range first {
		output.Transaction.OperationID = ""
		output.Transaction.PostingID = ""
		output.Transaction.BatchFingerprint = ""
		output.Transaction.BatchLineCount = 0
		output.Transaction.CommissionCount = 0
	}

	second, err := svc.BatchCreate(context.Background(), "tenant-1", "operation-1", items)
	if err != nil {
		t.Fatalf("retry batch: %v", err)
	}
	if len(repo.byID) != 2 {
		t.Fatalf("retry must not create duplicate rows, got %d", len(repo.byID))
	}
	for i := range first {
		if first[i].Transaction.ID != second[i].Transaction.ID {
			t.Errorf("line %d posting ID changed on retry", i)
		}
		if first[i].Transaction.SourceEventID != second[i].Transaction.SourceEventID {
			t.Errorf("line %d source ID changed on retry", i)
		}
	}
	if second[0].Transaction != repo.byID[first[0].Transaction.ID] {
		t.Fatalf("retry must return canonical persisted entity, got %#v", second[0].Transaction)
	}

	changed := append([]BatchCreateInput(nil), items...)
	changed[0].Description = "same key, different economic command"
	if _, err := svc.BatchCreate(context.Background(), "tenant-1", "operation-1", changed); !errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
		t.Fatalf("same key with changed payload must conflict, got %v", err)
	}
}

func TestTransactionBatchCreate_FingerprintRejectsChangedCardinalityBeforeWrite(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	items := []BatchCreateInput{
		{Type: string(transaction.TypeReceita), Description: "Receita", Value: 100, DateStr: "2026-08-10"},
		{Type: string(transaction.TypeDespesa), Description: "Despesa", Value: 25, DateStr: "2026-08-10"},
	}

	if _, err := svc.BatchCreate(context.Background(), "tenant-1", "cardinality-key", items); err != nil {
		t.Fatalf("first batch: %v", err)
	}
	if repo.batchCalls != 1 || len(repo.byID) != 2 {
		t.Fatalf("unexpected first write: calls=%d rows=%d", repo.batchCalls, len(repo.byID))
	}

	if _, err := svc.BatchCreate(context.Background(), "tenant-1", "cardinality-key", items[:1]); !errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
		t.Fatalf("truncated retry must conflict, got %v", err)
	}
	extended := append(append([]BatchCreateInput(nil), items...), BatchCreateInput{
		Type: string(transaction.TypeRepasse), Description: "Extra", Value: 10, DateStr: "2026-08-10",
	})
	if _, err := svc.BatchCreate(context.Background(), "tenant-1", "cardinality-key", extended); !errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
		t.Fatalf("extended retry must conflict, got %v", err)
	}
	if repo.batchCalls != 1 || len(repo.byID) != 2 {
		t.Fatalf("conflicting retries must not write: calls=%d rows=%d", repo.batchCalls, len(repo.byID))
	}
}

func TestTransactionBatchCreate_RejectsFractionalCentsBeforeWrite(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	_, err := svc.BatchCreate(context.Background(), "tenant-1", "fractional-cent", []BatchCreateInput{{
		Type: string(transaction.TypeReceita), Description: "Valor inválido", Value: 10.005, DateStr: "2026-08-10",
	}})
	if !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("fractional-cent transaction must fail validation, got %v", err)
	}
	if repo.batchCalls != 0 || len(repo.byID) != 0 {
		t.Fatalf("fractional-cent batch wrote data: calls=%d rows=%d", repo.batchCalls, len(repo.byID))
	}
}

func TestTransactionBatchCreatePosting_BindsTransactionsAndCommissionsToOneCommand(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	// The in-memory repositories ignore SQL, while the carried marker exercises
	// the service's surrounding-transaction path without opening a database.
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{
		{ContractID: "contract-1", Type: string(transaction.TypeReceita), Description: "Receita", Value: 100, DateStr: "2026-08-10"},
		{ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 10, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento"},
	}
	commissions := []ManualCommissionInput{{
		DealID: " deal-1 ", ContractID: " contract-1 ", BrokerID: " broker-1 ", Role: " FECHAMENTO ", Percentage: 100, Amount: 10,
	}}

	first, err := svc.BatchCreatePosting(ctx, " tenant-1 ", " posting-key ", items, commissions, nil)
	if err != nil {
		t.Fatalf("first combined posting: %v", err)
	}
	if len(first) != 2 || len(txRepo.byID) != 2 || len(commissionRepo.byID) != 1 {
		t.Fatalf("combined posting was not complete: outputs=%d tx=%d commissions=%d", len(first), len(txRepo.byID), len(commissionRepo.byID))
	}
	if first[0].Transaction.BatchFingerprint == "" || first[0].Transaction.BatchLineCount != 2 || first[0].Transaction.CommissionCount != 1 {
		t.Fatalf("command manifest not persisted: %#v", first[0].Transaction)
	}
	for _, c := range commissionRepo.byID {
		if c.DealID != "deal-1" || c.ContractID != "contract-1" || c.BrokerID != "broker-1" || c.Role != "fechamento" {
			t.Fatalf("commission identifiers were not normalized: %#v", c)
		}
	}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "posting-key", items, commissions, nil); err != nil {
		t.Fatalf("identical combined retry: %v", err)
	}
	if txRepo.batchCalls != 1 || len(txRepo.byID) != 2 || len(commissionRepo.byID) != 1 {
		t.Fatalf("identical retry duplicated writes: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
	}

	changed := append([]ManualCommissionInput(nil), commissions...)
	changed[0].Amount = 9
	changedItems := append([]BatchCreateInput(nil), items...)
	changedItems[1].Value = 9
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "posting-key", changedItems, changed, nil); !errors.Is(err, transaction.ErrBatchIdempotencyConflict) {
		t.Fatalf("changed commission under the same key must conflict, got %v", err)
	}
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "posting-key", items, nil, nil); !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("omitting the aggregate for a commission ledger line must fail validation, got %v", err)
	}
	if txRepo.batchCalls != 1 || len(txRepo.byID) != 2 || len(commissionRepo.byID) != 1 {
		t.Fatalf("conflicts must not write: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
	}
}

func TestTransactionBatchCreatePosting_RequiresBijectiveCommissionMirrors(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	base := BatchCreateInput{
		ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 10, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento",
	}
	matching := ManualCommissionInput{
		DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "fechamento", Percentage: 10, Amount: 10,
	}

	tests := []struct {
		name        string
		items       []BatchCreateInput
		commissions []ManualCommissionInput
	}{
		{name: "commission aggregate without ledger mirror", items: []BatchCreateInput{{ContractID: "contract-1", Type: string(transaction.TypeRepasse), Description: "Repasse", Value: 10, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento"}}, commissions: []ManualCommissionInput{matching}},
		{name: "different amount", items: []BatchCreateInput{base}, commissions: []ManualCommissionInput{{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "fechamento", Amount: 9}}},
		{name: "different broker", items: []BatchCreateInput{base}, commissions: []ManualCommissionInput{{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-2", Role: "fechamento", Amount: 10}}},
		{name: "different role", items: []BatchCreateInput{base}, commissions: []ManualCommissionInput{{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "captador", Amount: 10}}},
		{name: "missing role", items: []BatchCreateInput{{ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 10, DateStr: "2026-08-10", BrokerID: "broker-1"}}, commissions: []ManualCommissionInput{{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Amount: 10}}},
		{name: "invalid role", items: []BatchCreateInput{{ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 10, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "qualquer"}}, commissions: []ManualCommissionInput{{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "qualquer", Amount: 10}}},
		{name: "commission ledger line without aggregate", items: []BatchCreateInput{base}, commissions: nil},
		{name: "duplicate aggregate", items: []BatchCreateInput{base}, commissions: []ManualCommissionInput{matching, matching}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.BatchCreatePosting(ctx, "tenant-1", "mirror:"+tc.name, tc.items, tc.commissions, nil)
			if !errors.Is(err, transaction.ErrBatchValidation) {
				t.Fatalf("expected mirror validation error, got %v", err)
			}
		})
	}
	if txRepo.batchCalls != 0 || len(txRepo.byID) != 0 || len(commissionRepo.byID) != 0 {
		t.Fatalf("mismatched mirrors wrote data: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
	}
}

func TestTransactionBatchCreatePosting_ReceiptDistributionRetainsAgencyRemainder(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{
		{ContractID: "contract-1", Type: string(transaction.TypeReceita), Description: "Receita", Value: 10_000, DateStr: "2026-08-10"},
		{ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão captador", Value: 300, DateStr: "2026-08-10", BrokerID: "broker-cap", Role: "captador"},
		{ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão fechamento", Value: 500, DateStr: "2026-08-10", BrokerID: "broker-close", Role: "fechamento"},
	}
	commissions := []ManualCommissionInput{
		{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-cap", Role: "captador", Percentage: 30, Amount: 300},
		{DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-close", Role: "fechamento", Percentage: 50, Amount: 500},
	}
	retention := &ManualCommissionRetentionInput{Percentage: 20, Amount: 200}
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-retained", items, commissions, retention); err != nil {
		t.Fatalf("partial broker distribution with agency retention: %v", err)
	}
	if len(txRepo.byID) != 3 || len(commissionRepo.byID) != 2 {
		t.Fatalf("posting = %d transactions/%d commissions, want receipt + two broker mirrors and only two payables", len(txRepo.byID), len(commissionRepo.byID))
	}
	for _, created := range commissionRepo.byID {
		if created.Role == "imobiliaria" || created.BrokerID == "" {
			t.Fatalf("agency remainder became a payable: %+v", created)
		}
	}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-missing-retention", items, commissions, nil); !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("partial distribution without explicit retention must fail, got %v", err)
	}
	badRetentionPercentage := &ManualCommissionRetentionInput{Percentage: 19, Amount: 200}
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-wrong-retention-percentage", items, commissions, badRetentionPercentage); !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("retention percentage other than the exact remainder must fail, got %v", err)
	}
	badRetentionAmount := &ManualCommissionRetentionInput{Percentage: 20, Amount: 201}
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-wrong-retention-amount", items, commissions, badRetentionAmount); !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("retention cents inconsistent with the commission pool must fail, got %v", err)
	}
	mismatchedItems := append([]BatchCreateInput(nil), items...)
	mismatchedItems[1].Value = 350
	mismatchedItems[2].Value = 450
	mismatchedCommissions := append([]ManualCommissionInput(nil), commissions...)
	mismatchedCommissions[0].Amount = 350
	mismatchedCommissions[1].Amount = 450
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-wrong-broker-allocation", mismatchedItems, mismatchedCommissions, retention); !errors.Is(err, transaction.ErrBatchValidation) || !strings.Contains(err.Error(), "deterministic broker allocation") {
		t.Fatalf("individually inconsistent broker amounts must fail, got %v", err)
	}

	commissions[0].Percentage = 50.02
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-overallocated", items, commissions, retention); !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("distribution above 100%% must fail, got %v", err)
	}
	commissions[0].Percentage = 0
	commissions[1].Percentage = 0
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-empty", items, commissions, retention); !errors.Is(err, transaction.ErrBatchValidation) || !strings.Contains(err.Error(), "(0, 100]") {
		t.Fatalf("zero-percentage broker payable must fail explicitly, got %v", err)
	}

	commissions[0].Percentage = 50
	commissions[1].Percentage = 50
	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "distribution-complete", items, commissions, nil); err != nil {
		t.Fatalf("complete receipt distribution: %v", err)
	}
}

func TestTransactionSettle_ManualSaleReceiptHydratesLegacyWaterfallAndReleasesOwnerTransfer(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	settlement := &recordingManualReceiptSettlement{}
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo)).
		WithManualReceiptSettlement(settlement)
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{
		{ContractID: "contract-sale", Type: string(transaction.TypeReceita), Description: "Recebimento - Venda", Value: 980_000, DateStr: "2026-08-29", Flow: "sale"},
		{ContractID: "contract-sale", Type: string(transaction.TypeComissao), Description: "Comissão - captador", Value: 11_760, DateStr: "2026-08-29", BrokerID: "broker-cap", Role: "captador"},
		{ContractID: "contract-sale", Type: string(transaction.TypeComissao), Description: "Comissão - fechamento", Value: 23_520, DateStr: "2026-08-29", BrokerID: "broker-close", Role: "fechamento"},
	}
	commissions := []ManualCommissionInput{
		{DealID: "deal-sale", ContractID: "contract-sale", BrokerID: "broker-cap", Role: "captador", Percentage: 20, Amount: 11_760},
		{DealID: "deal-sale", ContractID: "contract-sale", BrokerID: "broker-close", Role: "fechamento", Percentage: 40, Amount: 23_520},
	}
	retention := &ManualCommissionRetentionInput{Percentage: 40, Amount: 23_520}

	created, err := svc.BatchCreatePosting(ctx, "tenant-sale", "sale-waterfall", items, commissions, retention)
	if err != nil {
		t.Fatalf("create manual sale posting: %v", err)
	}
	var receipt *transaction.Transaction
	for _, output := range created {
		if output.Transaction != nil && output.Transaction.Type == transaction.TypeReceita {
			receipt = output.Transaction
			break
		}
	}
	if receipt == nil || receipt.EconomicBreakdown == nil {
		t.Fatalf("manual sale receipt has no economic waterfall: %+v", receipt)
	}
	if receipt.DealID != "deal-sale" || receipt.EconomicBreakdown.GrossCents != 98_000_000 ||
		receipt.EconomicBreakdown.CommissionPoolCents != 5_880_000 ||
		receipt.EconomicBreakdown.CommissionPayableCents != 3_528_000 ||
		receipt.EconomicBreakdown.AgencyRetainedCents != 2_352_000 ||
		receipt.EconomicBreakdown.OwnerPayableCents != 92_120_000 {
		t.Fatalf("unexpected sale waterfall: deal=%s breakdown=%+v", receipt.DealID, receipt.EconomicBreakdown)
	}

	// Simulate a row created before economic_breakdown/deal_id were persisted.
	// Settlement must reconstruct it from the immutable operation manifest and
	// its linked commission obligations before any money is released.
	receipt.DealID = ""
	receipt.EconomicBreakdown = nil
	if err := txRepo.Update(ctx, receipt); err != nil {
		t.Fatalf("prepare legacy receipt: %v", err)
	}
	paymentDate := time.Date(2026, time.August, 29, 17, 30, 0, 0, time.UTC)
	settled, err := svc.Settle(ctx, receipt.ID, "tenant-sale", &paymentDate, "PIX")
	if err != nil {
		t.Fatalf("settle legacy manual sale receipt: %v", err)
	}
	if settled.Status != transaction.StatusPaid || settlement.calls != 1 || settlement.receipt == nil {
		t.Fatalf("settlement hook was not called with the paid receipt: status=%s calls=%d receipt=%+v", settled.Status, settlement.calls, settlement.receipt)
	}
	if settlement.receipt.DealID != "deal-sale" || settlement.receipt.EconomicBreakdown == nil ||
		settlement.receipt.EconomicBreakdown.OwnerPayableCents != 92_120_000 ||
		settlement.receipt.EconomicBreakdown.AgencyRetainedCents != 2_352_000 {
		t.Fatalf("legacy waterfall was not reconstructed: deal=%s breakdown=%+v", settlement.receipt.DealID, settlement.receipt.EconomicBreakdown)
	}
	if persisted := txRepo.byID[receipt.ID]; persisted.EconomicBreakdown == nil || persisted.DealID != "deal-sale" {
		t.Fatalf("hydrated waterfall was not persisted with the baixa: %+v", persisted)
	}

	// Lost-response retries intentionally call the idempotent hook again. The
	// transfer service below proves that this still creates exactly one repasse.
	if _, err := svc.Settle(ctx, receipt.ID, "tenant-sale", &paymentDate, "PIX"); err != nil {
		t.Fatalf("retry settled sale receipt: %v", err)
	}
	if settlement.calls != 2 {
		t.Fatalf("retry must repair through the idempotent hook, calls=%d", settlement.calls)
	}
}

func TestTransactionSettle_ManualSaleWithoutBrokerKeepsAgencyCommissionAndOwnerObligation(t *testing.T) {
	txRepo := newStoringTxRepo()
	settlement := &recordingManualReceiptSettlement{}
	svc := NewTransactionService(txRepo).WithManualReceiptSettlement(settlement)
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{{
		ContractID:  "contract-agency-only",
		DealID:      "deal-agency-only",
		Type:        string(transaction.TypeReceita),
		Description: "Recebimento - Venda",
		Value:       980_000,
		DateStr:     "2026-08-29",
		Flow:        "sale",
	}}
	retention := &ManualCommissionRetentionInput{Percentage: 100, Amount: 58_800}

	created, err := svc.BatchCreatePosting(ctx, "tenant-sale", "agency-only-waterfall", items, nil, retention)
	if err != nil {
		t.Fatalf("create agency-only sale receipt: %v", err)
	}
	receipt := created[0].Transaction
	if receipt.CommissionCount != 0 || receipt.DealID != "deal-agency-only" || receipt.EconomicBreakdown == nil {
		t.Fatalf("agency-only receipt identity was not frozen: %+v", receipt)
	}
	if receipt.EconomicBreakdown.CommissionPoolCents != 5_880_000 ||
		receipt.EconomicBreakdown.CommissionPayableCents != 0 ||
		receipt.EconomicBreakdown.AgencyRetainedCents != 5_880_000 ||
		receipt.EconomicBreakdown.OwnerPayableCents != 92_120_000 {
		t.Fatalf("agency-only receipt waterfall is wrong: %+v", receipt.EconomicBreakdown)
	}

	settled, err := svc.Settle(ctx, receipt.ID, "tenant-sale", nil, "PIX")
	if err != nil {
		t.Fatalf("settle agency-only sale receipt: %v", err)
	}
	if settled.Status != transaction.StatusPaid || settlement.calls != 1 {
		t.Fatalf("agency-only owner obligation was not released: status=%s calls=%d", settled.Status, settlement.calls)
	}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-sale", "agency-only-missing-retention", items, nil, nil); !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("sale receipt without a closed commission composition must fail, got %v", err)
	}
}

func TestTransactionBatchCreatePosting_RejectsAgencyRetentionAsCommissionPayable(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{
		{ContractID: "contract-1", Type: string(transaction.TypeReceita), Description: "Receita", Value: 100, DateStr: "2026-08-10"},
		{ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Retenção imobiliária", Value: 20, DateStr: "2026-08-10", BrokerID: "tenant-1", Role: "imobiliaria"},
	}
	commissions := []ManualCommissionInput{{
		DealID: "deal-1", ContractID: "contract-1", BrokerID: "tenant-1", Role: "imobiliaria", Percentage: 20, Amount: 20,
	}}

	_, err := svc.BatchCreatePosting(ctx, "tenant-1", "agency-not-payable", items, commissions, nil)
	if !errors.Is(err, transaction.ErrBatchValidation) || !strings.Contains(err.Error(), "agency retention") {
		t.Fatalf("agency retention payable must fail explicitly, got %v", err)
	}
	if txRepo.batchCalls != 0 || len(txRepo.byID) != 0 || len(commissionRepo.byID) != 0 {
		t.Fatalf("invalid agency payable wrote data: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
	}
}

func TestTransactionBatchCreatePosting_CommissionRetentionRoundingBoundaries(t *testing.T) {
	newService := func() (*TransactionService, context.Context) {
		return NewTransactionService(newStoringTxRepo()).
				WithManualCommissionService(NewCommissionService(newStoringManualCommissionRepo())),
			database.WithTx(context.Background(), &sqlx.Tx{})
	}

	t.Run("retention within percentage tolerance remains explicit", func(t *testing.T) {
		svc, ctx := newService()
		items := []BatchCreateInput{
			{ContractID: "contract-boundary", Type: string(transaction.TypeReceita), Description: "Receita", Value: 100_000, DateStr: "2026-08-10"},
			{ContractID: "contract-boundary", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 19_999, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento"},
		}
		commissions := []ManualCommissionInput{{
			DealID: "deal-boundary", ContractID: "contract-boundary", BrokerID: "broker-1", Role: "fechamento", Percentage: 99.995, Amount: 19_999,
		}}
		retention := &ManualCommissionRetentionInput{Percentage: 0.005, Amount: 1}

		if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "retention-percentage-boundary", items, commissions, retention); err != nil {
			t.Fatalf("99.995%% broker + 0.005%% retention: %v", err)
		}
	})

	t.Run("one-cent pool keeps zero-cent retention explicit", func(t *testing.T) {
		svc, ctx := newService()
		items := []BatchCreateInput{
			{ContractID: "contract-cent", Type: string(transaction.TypeReceita), Description: "Receita", Value: 1, DateStr: "2026-08-10"},
			{ContractID: "contract-cent", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 0.01, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento"},
		}
		commissions := []ManualCommissionInput{{
			DealID: "deal-cent", ContractID: "contract-cent", BrokerID: "broker-1", Role: "fechamento", Percentage: 80, Amount: 0.01,
		}}
		retention := &ManualCommissionRetentionInput{Percentage: 20, Amount: 0}

		if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "retention-cent-boundary", items, commissions, retention); err != nil {
			t.Fatalf("one-cent 80/20 distribution: %v", err)
		}
	})

	t.Run("four-decimal canonical total matches largest-remainder allocation", func(t *testing.T) {
		svc, ctx := newService()
		items := []BatchCreateInput{
			{ContractID: "contract-parity", Type: string(transaction.TypeReceita), Description: "Receita", Value: 100_000, DateStr: "2026-08-10"},
			{ContractID: "contract-parity", Type: string(transaction.TypeComissao), Description: "Comissão 1", Value: 10_000.01, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "captador"},
			{ContractID: "contract-parity", Type: string(transaction.TypeComissao), Description: "Comissão 2", Value: 10_000, DateStr: "2026-08-10", BrokerID: "broker-2", Role: "fechamento"},
		}
		commissions := []ManualCommissionInput{
			{DealID: "deal-parity", ContractID: "contract-parity", BrokerID: "broker-1", Role: "captador", Percentage: 33.33336, Amount: 10_000.01},
			{DealID: "deal-parity", ContractID: "contract-parity", BrokerID: "broker-2", Role: "fechamento", Percentage: 33.33336, Amount: 10_000},
		}
		retention := &ManualCommissionRetentionInput{Percentage: 33.3333, Amount: 9_999.99}

		if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "retention-four-decimal-parity", items, commissions, retention); err != nil {
			t.Fatalf("four-decimal canonical distribution: %v", err)
		}
	})
}

func TestTransactionBatchCreatePosting_RejectsAmbiguousReceiptContractScope(t *testing.T) {
	tests := []struct {
		name  string
		items []BatchCreateInput
	}{
		{
			name: "commission belongs to another contract",
			items: []BatchCreateInput{
				{ContractID: "contract-receipt", Type: string(transaction.TypeReceita), Description: "Receita", Value: 100, DateStr: "2026-08-10"},
				{ContractID: "contract-other", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 80, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento"},
			},
		},
		{
			name: "receipts span contracts",
			items: []BatchCreateInput{
				{ContractID: "contract-receipt", Type: string(transaction.TypeReceita), Description: "Receita 1", Value: 50, DateStr: "2026-08-10"},
				{ContractID: "contract-other", Type: string(transaction.TypeReceita), Description: "Receita 2", Value: 50, DateStr: "2026-08-10"},
				{ContractID: "contract-receipt", Type: string(transaction.TypeComissao), Description: "Comissão", Value: 80, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			txRepo := newStoringTxRepo()
			commissionRepo := newStoringManualCommissionRepo()
			svc := NewTransactionService(txRepo).
				WithManualCommissionService(NewCommissionService(commissionRepo))
			ctx := database.WithTx(context.Background(), &sqlx.Tx{})
			commissions := []ManualCommissionInput{{
				DealID: "deal-1", ContractID: "contract-other", BrokerID: "broker-1", Role: "fechamento", Percentage: 80, Amount: 80,
			}}
			if tt.name == "receipts span contracts" {
				commissions[0].ContractID = "contract-receipt"
			}

			_, err := svc.BatchCreatePosting(ctx, "tenant-1", "ambiguous-receipt:"+tt.name, tt.items, commissions, &ManualCommissionRetentionInput{Percentage: 20, Amount: 20})
			if !errors.Is(err, transaction.ErrBatchValidation) {
				t.Fatalf("ambiguous receipt scope must fail, got %v", err)
			}
			if txRepo.batchCalls != 0 || len(txRepo.byID) != 0 || len(commissionRepo.byID) != 0 {
				t.Fatalf("ambiguous receipt scope wrote data: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
			}
		})
	}
}

func TestManualPostingFingerprintIncludesExplicitRetentionWithoutChangingLegacyMode(t *testing.T) {
	entry := transaction.New("contract-1", transaction.TypeComissao, "Comissão", 80, time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC), "tenant-1")
	entry.BrokerID = "broker-1"
	entry.Role = "fechamento"
	commissions := []ManualCommissionInput{{
		DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "fechamento", Percentage: 80, Amount: 80,
	}}

	legacy := manualPostingFingerprint([]*transaction.Transaction{entry}, commissions, nil)
	explicit := manualPostingFingerprint([]*transaction.Transaction{entry}, commissions, &ManualCommissionRetentionInput{Percentage: 20, Amount: 20})
	changed := manualPostingFingerprint([]*transaction.Transaction{entry}, commissions, &ManualCommissionRetentionInput{Percentage: 20, Amount: 19.99})
	if legacy == explicit {
		t.Fatal("explicit retention must upgrade and change the command fingerprint")
	}
	if explicit == changed {
		t.Fatal("retention cents must participate in the command fingerprint")
	}
	if legacy != manualPostingFingerprint([]*transaction.Transaction{entry}, commissions, nil) {
		t.Fatal("legacy v1 fingerprint must remain deterministic when retention is absent")
	}
}

func TestTransactionBatchCreatePosting_AllowsLegacyAmountOnlyCommissionWithoutReceipt(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})
	items := []BatchCreateInput{{
		ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão avulsa", Value: 10,
		DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento",
	}}
	commissions := []ManualCommissionInput{{
		DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "fechamento", Amount: 10,
	}}

	if _, err := svc.BatchCreatePosting(ctx, "tenant-1", "legacy-amount-only", items, commissions, nil); err != nil {
		t.Fatalf("legacy amount-only commission without receipt: %v", err)
	}
	if len(txRepo.byID) != 1 || len(commissionRepo.byID) != 1 {
		t.Fatalf("posting = %d transactions/%d commissions, want 1/1", len(txRepo.byID), len(commissionRepo.byID))
	}
}

func TestTransactionBatchCreatePosting_RejectsNonFiniteAmountsBeforeWrite(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	_, err := svc.BatchCreatePosting(ctx, "tenant-1", "non-finite", []BatchCreateInput{{
		Type: string(transaction.TypeReceita), Description: "Inválida", Value: math.NaN(), DateStr: "2026-08-10",
	}}, []ManualCommissionInput{{DealID: "deal-1", BrokerID: "broker-1", Amount: 1}}, nil)
	if !errors.Is(err, transaction.ErrBatchValidation) {
		t.Fatalf("non-finite transaction must be rejected, got %v", err)
	}
	if txRepo.batchCalls != 0 || len(txRepo.byID) != 0 || len(commissionRepo.byID) != 0 {
		t.Fatalf("invalid command wrote data: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
	}
}

func TestTransactionBatchCreatePosting_RejectsFractionalCentsBeforeWrite(t *testing.T) {
	txRepo := newStoringTxRepo()
	commissionRepo := newStoringManualCommissionRepo()
	svc := NewTransactionService(txRepo).
		WithManualCommissionService(NewCommissionService(commissionRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	tests := []struct {
		name             string
		transactionValue float64
		commissionAmount float64
	}{
		{name: "transaction", transactionValue: 10.005, commissionAmount: 10.00},
		{name: "commission", transactionValue: 10.00, commissionAmount: 10.005},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.BatchCreatePosting(ctx, "tenant-1", "fractional-cent:"+tc.name, []BatchCreateInput{{
				ContractID: "contract-1", Type: string(transaction.TypeComissao), Description: "Comissão", Value: tc.transactionValue, DateStr: "2026-08-10", BrokerID: "broker-1", Role: "fechamento",
			}}, []ManualCommissionInput{{
				DealID: "deal-1", ContractID: "contract-1", BrokerID: "broker-1", Role: "fechamento", Percentage: 10, Amount: tc.commissionAmount,
			}}, nil)
			if !errors.Is(err, transaction.ErrBatchValidation) {
				t.Fatalf("fractional cent must fail validation, got %v", err)
			}
		})
	}
	if txRepo.batchCalls != 0 || len(txRepo.byID) != 0 || len(commissionRepo.byID) != 0 {
		t.Fatalf("fractional-cent command wrote data: calls=%d tx=%d commissions=%d", txRepo.batchCalls, len(txRepo.byID), len(commissionRepo.byID))
	}
}

func TestTransactionSettle_RejectsPaidAndReversedStates(t *testing.T) {
	for _, status := range []transaction.Status{transaction.StatusPaid, transaction.StatusReversed} {
		t.Run(string(status), func(t *testing.T) {
			repo := newStoringTxRepo()
			svc := NewTransactionService(repo)
			entry := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
			entry.Status = status
			if err := repo.Create(context.Background(), entry); err != nil {
				t.Fatalf("seed: %v", err)
			}

			_, err := svc.Settle(context.Background(), entry.ID, "tenant-1", nil, "PIX")
			if !errors.Is(err, transaction.ErrInvalidStatusTransition) {
				t.Fatalf("expected invalid transition for %s, got %v", status, err)
			}
			if entry.Status != status {
				t.Fatalf("status changed from %s to %s", status, entry.Status)
			}
			if repo.updateCalls != 0 {
				t.Fatalf("invalid transition must not be persisted, got %d updates", repo.updateCalls)
			}
		})
	}
}

func TestTransactionSettle_AllowsOverdueState(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	entry := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
	entry.Status = transaction.StatusOverdue
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("seed: %v", err)
	}

	got, err := svc.Settle(context.Background(), entry.ID, "tenant-1", nil, "PIX")
	if err != nil {
		t.Fatalf("settle overdue transaction: %v", err)
	}
	if got.Status != transaction.StatusPaid {
		t.Fatalf("expected PAID, got %s", got.Status)
	}
}

func TestTransactionReverse_IsSingleAndReversedCannotBeSettled(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	entry := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
	entry.Status = transaction.StatusPaid
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("seed: %v", err)
	}

	first, err := svc.Reverse(context.Background(), entry.ID, "tenant-1", "duplicidade", "operator-1")
	if err != nil {
		t.Fatalf("first reverse: %v", err)
	}
	second, err := svc.Reverse(context.Background(), entry.ID, "tenant-1", "retry", "operator-1")
	if err != nil {
		t.Fatalf("retry reverse: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("retry created another reversal: first=%s second=%s", first.ID, second.ID)
	}
	if len(repo.byID) != 2 {
		t.Fatalf("expected original plus one reversal, got %d rows", len(repo.byID))
	}
	if entry.Status != transaction.StatusReversed {
		t.Fatalf("expected terminal REVERSED status, got %s", entry.Status)
	}
	if _, err := svc.Settle(context.Background(), entry.ID, "tenant-1", nil, "PIX"); !errors.Is(err, transaction.ErrInvalidStatusTransition) {
		t.Fatalf("reversed transaction must not be reopened by settle, got %v", err)
	}
}

func TestTransactionUpdate_CannotLeaveReversedState(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	entry := transaction.New("contract-1", transaction.TypeReceita, "Aluguel", 1000, time.Now(), "tenant-1")
	entry.Status = transaction.StatusReversed
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("seed: %v", err)
	}

	updated := *entry
	updated.Status = transaction.StatusPending
	if err := svc.Update(context.Background(), &updated); !errors.Is(err, transaction.ErrInvalidStatusTransition) {
		t.Fatalf("expected terminal-state error, got %v", err)
	}
	if entry.Status != transaction.StatusReversed {
		t.Fatalf("stored transaction was reopened: %s", entry.Status)
	}
}

func TestTransactionDelete_RejectsImmutableFinancialRows(t *testing.T) {
	tests := []struct {
		name   string
		status transaction.Status
		source string
	}{
		{name: "source linked pending", status: transaction.StatusPending, source: "event-1"},
		{name: "paid manual", status: transaction.StatusPaid},
		{name: "reversed manual", status: transaction.StatusReversed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newStoringTxRepo()
			svc := NewTransactionService(repo)
			entry := transaction.New("contract-1", transaction.TypeReceita, "Lançamento", 100, time.Now(), "tenant-1")
			entry.Status = tt.status
			entry.SourceEventID = tt.source
			if err := repo.Create(context.Background(), entry); err != nil {
				t.Fatalf("seed: %v", err)
			}

			err := svc.Delete(context.Background(), entry.ID, entry.TenantID)
			if !errors.Is(err, transaction.ErrInvalidStatusTransition) {
				t.Fatalf("Delete() error = %v, want immutable lifecycle conflict", err)
			}
			if repo.byID[entry.ID] == nil {
				t.Fatal("immutable transaction was deleted")
			}
		})
	}
}

func TestTransactionDelete_AllowsUnsettledUnsourcedManualRow(t *testing.T) {
	repo := newStoringTxRepo()
	svc := NewTransactionService(repo)
	entry := transaction.New("", transaction.TypeDespesa, "Rascunho manual", 100, time.Now(), "tenant-1")
	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := svc.Delete(context.Background(), entry.ID, entry.TenantID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if repo.byID[entry.ID] != nil {
		t.Fatal("manual pending transaction was not deleted")
	}
}

func TestSettlePlainEntradaSaidaRecordsCashJournalAndProof(t *testing.T) {
	txRepo := newStoringTxRepo()
	journalRepo := &recordingManualJournalRepo{}
	svc := NewTransactionService(txRepo).WithJournalService(NewJournalService(journalRepo))

	// Entrada avulsa (sem breakdown/link): a baixa registra caixa D / receita C.
	receipt := transaction.New("contract-1", transaction.TypeReceita, "Entrada manual", 10_000, time.Now(), "tenant-1")
	if err := svc.Create(context.Background(), receipt); err != nil {
		t.Fatal(err)
	}
	pd := time.Now()
	got, err := svc.SettleWithProof(context.Background(), receipt.ID, "tenant-1", &pd, "PIX", "https://files.example/entrada.pdf", "TED recebido 14:30")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProofURL != "https://files.example/entrada.pdf" || got.Note != "TED recebido 14:30" {
		t.Fatalf("settlement proof not persisted: %q %q", got.ProofURL, got.Note)
	}
	if len(journalRepo.entries) != 1 {
		t.Fatalf("plain entrada must post exactly one cash journal entry, got %d", len(journalRepo.entries))
	}
	entry := journalRepo.entries[0]
	if len(entry.Legs) != 2 || entry.Legs[0].Account != journal.AccountCash || entry.Legs[0].Direction != journal.Debit ||
		entry.Legs[1].Account != journal.AccountRevenue || entry.Legs[1].Direction != journal.Credit ||
		entry.Legs[0].AmountCents != 1_000_000 || entry.Legs[1].AmountCents != 1_000_000 {
		t.Fatalf("unexpected cash journal legs for entrada: %+v", entry.Legs)
	}

	// Saída avulsa: a baixa registra despesa D / caixa C.
	expense := transaction.New("contract-1", transaction.TypeDespesa, "Pagamento IPTU", 2_500, time.Now(), "tenant-1")
	if err := svc.Create(context.Background(), expense); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Settle(context.Background(), expense.ID, "tenant-1", &pd, "TED"); err != nil {
		t.Fatal(err)
	}
	if len(journalRepo.entries) != 2 {
		t.Fatalf("plain saída must post exactly one cash journal entry, got %d total", len(journalRepo.entries))
	}
	expenseEntry := journalRepo.entries[1]
	if expenseEntry.Legs[0].Account != journal.AccountExpense || expenseEntry.Legs[0].Direction != journal.Debit ||
		expenseEntry.Legs[1].Account != journal.AccountCash || expenseEntry.Legs[1].Direction != journal.Credit ||
		expenseEntry.Legs[0].AmountCents != 250_000 {
		t.Fatalf("unexpected cash journal legs for saída: %+v", expenseEntry.Legs)
	}

	// Comprovante anexado depois da baixa (retry idempotente com proof).
	attached, err := svc.SettleWithProof(context.Background(), expense.ID, "tenant-1", nil, "", "https://files.example/iptu.pdf", "boleto conferido")
	if err != nil {
		t.Fatal(err)
	}
	if attached.ProofURL != "https://files.example/iptu.pdf" {
		t.Fatalf("late proof attach failed: %q", attached.ProofURL)
	}
	if len(journalRepo.entries) != 2 {
		t.Fatalf("late proof attach must not duplicate the journal, got %d entries", len(journalRepo.entries))
	}
}
