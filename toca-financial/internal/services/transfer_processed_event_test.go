package services

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// captureBus é um events.EventBus mínimo que captura tudo publicado
// em memória — suficiente para asserts no PublishReliable que o
// transfer_service faz no path de Process.
type captureBus struct {
	mu        sync.Mutex
	published []events.Event
}

func (b *captureBus) Publish(ctx context.Context, evt events.Event) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, evt)
	return nil
}
func (b *captureBus) PublishReliable(ctx context.Context, evt events.Event) error {
	return b.Publish(ctx, evt)
}
func (b *captureBus) Subscribe(eventName string, handler events.EventHandler) error {
	return nil
}
func (b *captureBus) SubscribeWithQueue(queue, eventName string, handler events.EventHandler) error {
	return nil
}
func (b *captureBus) Close() error { return nil }

func (b *captureBus) get() []events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]events.Event, len(b.published))
	copy(out, b.published)
	return out
}

// captureRepo é uma transfer.RepositoryInterface in-memory.
type captureRepo struct {
	store map[string]*transfer.OwnerTransfer
}

func newCaptureRepo(seed ...*transfer.OwnerTransfer) *captureRepo {
	r := &captureRepo{store: map[string]*transfer.OwnerTransfer{}}
	for _, t := range seed {
		r.store[t.ID] = t
	}
	return r
}

func (r *captureRepo) Create(ctx context.Context, t *transfer.OwnerTransfer) error {
	r.store[t.ID] = t
	return nil
}
func (r *captureRepo) GetByID(ctx context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	if t, ok := r.store[id]; ok && (tenantID == "" || t.TenantID == tenantID) {
		return t, nil
	}
	return nil, nil
}
func (r *captureRepo) Update(ctx context.Context, t *transfer.OwnerTransfer) error {
	r.store[t.ID] = t
	return nil
}
func (r *captureRepo) List(ctx context.Context, tenantID, contractID, ownerID, status, referenceMonth string, limit, offset int64) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *captureRepo) UpdateStatus(ctx context.Context, tenantID, id string, status transfer.Status) error {
	return nil
}
func (r *captureRepo) Search(ctx context.Context, tenantID string, filter transfer.SearchFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *captureRepo) ListByOwnerLeadIDs(ctx context.Context, tenantID string, ownerLeadIDs []string, filter transfer.PortalListFilter) ([]*transfer.OwnerTransfer, int64, error) {
	return nil, 0, nil
}
func (r *captureRepo) GetByContractMonth(ctx context.Context, tenantID, contractID, referenceMonth string) (*transfer.OwnerTransfer, error) {
	for _, t := range r.store {
		if t.TenantID == tenantID && t.ContractID == contractID && t.ReferenceMonth == referenceMonth {
			return t, nil
		}
	}
	return nil, nil
}

func TestTransferService_Process_PublishesEvent(t *testing.T) {
	ctx := context.Background()
	owner := transfer.New("contract-1", "lead-owner-1", "prop-1", "tenant-A", "2026-05",
		1000.0, 10.0, 0, 0, nil, "")
	// Marca como Approved (Process exige passar pelo Approve antes).
	if err := owner.Approve("admin-x", "Admin User"); err != nil {
		t.Fatalf("setup: approve failed: %v", err)
	}
	repo := newCaptureRepo(owner)
	bus := &captureBus{}
	svc := NewTransferService(repo).WithEventBus(bus)

	if err := svc.Process(ctx, "tenant-A", owner.ID, "admin-y", "Other Admin"); err != nil {
		t.Fatalf("expected Process to succeed, got %v", err)
	}

	pubs := bus.get()
	if len(pubs) != 1 {
		t.Fatalf("expected exactly 1 published event, got %d", len(pubs))
	}
	evt, ok := pubs[0].(TransferProcessedEvent)
	if !ok {
		t.Fatalf("expected TransferProcessedEvent, got %T", pubs[0])
	}
	if evt.Name() != "transfer.processed" {
		t.Errorf("Name() = %q, want %q", evt.Name(), "transfer.processed")
	}
	if evt.TransferID != owner.ID {
		t.Errorf("TransferID mismatch: got %q, want %q", evt.TransferID, owner.ID)
	}
	if evt.TenantID != "tenant-A" {
		t.Errorf("TenantID mismatch: got %q", evt.TenantID)
	}
	if evt.OwnerLeadID != "lead-owner-1" {
		t.Errorf("OwnerLeadID mismatch: got %q", evt.OwnerLeadID)
	}
	if evt.NetAmount <= 0 {
		t.Errorf("NetAmount should be positive after process, got %f", evt.NetAmount)
	}
	if evt.ProcessedAt.IsZero() {
		t.Error("ProcessedAt should be populated")
	}
}

func TestTransferService_Process_NoBus_NoError(t *testing.T) {
	ctx := context.Background()
	owner := transfer.New("contract-2", "lead-owner-2", "prop-2", "tenant-A", "2026-05",
		500.0, 10.0, 0, 0, nil, "")
	_ = owner.Approve("admin", "Admin")
	repo := newCaptureRepo(owner)
	// Service criado SEM event bus — Process não deve falhar nem publicar.
	svc := NewTransferService(repo)

	// P1/A13: quem aprovou ("admin") não efetiva o pagamento — o ator do
	// Process é outro operador (segregação de funções).
	if err := svc.Process(ctx, "tenant-A", owner.ID, "operator-2", "Operator 2"); err != nil {
		t.Fatalf("Process without bus should still succeed, got %v", err)
	}
}

func TestTransferService_Process_NotFound(t *testing.T) {
	ctx := context.Background()
	bus := &captureBus{}
	svc := NewTransferService(newCaptureRepo()).WithEventBus(bus)

	if err := svc.Process(ctx, "tenant-A", "missing-id", "admin", "Admin"); err == nil {
		t.Fatal("expected error for missing transfer, got nil")
	}
	if len(bus.get()) != 0 {
		t.Errorf("no event should be published when transfer not found")
	}
}

func TestOwnerTransferSettlementJournalEntryUsesAvailableCentsAndTrace(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "")
	owner.ID = "transfer-1"
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	if err := owner.Approve("approver-1", "Ana"); err != nil {
		t.Fatal(err)
	}
	if err := owner.Process("payer-1", "Carlos"); err != nil {
		t.Fatal(err)
	}

	entry, err := newOwnerTransferSettlementJournalEntry(owner)
	if err != nil {
		t.Fatalf("build settlement journal: %v", err)
	}
	if entry == nil || entry.OperationID != owner.ID || entry.ContractID != owner.ContractID {
		t.Fatalf("settlement identity = %+v", entry)
	}
	if owner.ProcessedAt == nil || entry.Date.Unix() != owner.ProcessedAt.Unix() {
		t.Fatalf("entry date %v differs from processed_at %v", entry.Date, owner.ProcessedAt)
	}
	if len(entry.Legs) != 2 {
		t.Fatalf("settlement legs = %d, want 2", len(entry.Legs))
	}
	for i, leg := range entry.Legs {
		if leg.AmountCents != 460000 || leg.TransferID != owner.ID || leg.BeneficiaryPartyID != owner.OwnerID {
			t.Fatalf("leg %d lost available amount/trace: %+v", i, leg)
		}
	}
	if entry.Legs[0].Account != journal.AccountOwnerPayable || entry.Legs[0].Direction != journal.Debit ||
		entry.Legs[1].Account != journal.AccountCash || entry.Legs[1].Direction != journal.Credit {
		t.Fatalf("unexpected settlement posting: %+v", entry.Legs)
	}
	if got := ownerTransferSettlementSourceEventID(owner.TenantID, owner.ID); got == "" || got != ownerTransferSettlementSourceEventID(owner.TenantID, owner.ID) {
		t.Fatalf("settlement source is not deterministic: %q", got)
	}
}

func TestOwnerTransferSettlementJournalEntryRejectsNetBalanceDrift(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "")
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	owner.NetAmount = 5600
	if _, err := newOwnerTransferSettlementJournalEntry(owner); err == nil {
		t.Fatal("expected allocation-aware net/available mismatch to fail closed")
	}
}

func TestOwnerTransferSettlementJournalEntryRequiresAllocationCoverage(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "")

	if _, err := newOwnerTransferSettlementJournalEntry(owner); !errors.Is(err, ErrTransferSettlementCoverageRequired) {
		t.Fatalf("settlement without allocation coverage error = %v", err)
	}
}

func TestOwnerTransferSettlementJournalEntryRejectsZeroAmount(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		0, 0, 0, 0, nil, "")
	balance, err := transfer.NewOwnerBalance(0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.SetAllocatedOwnerBalance(balance, 0); err != nil {
		t.Fatal(err)
	}

	if _, err := newOwnerTransferSettlementJournalEntry(owner); !errors.Is(err, ErrTransferSettlementAmountInvalid) {
		t.Fatalf("zero settlement error = %v", err)
	}
}

func TestTransferService_ProcessRequiresTransactionWhenJournalIsWired(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "")
	if err := owner.Approve("approver-1", "Ana"); err != nil {
		t.Fatal(err)
	}
	repo := newCaptureRepo(owner)
	journalRepo := newFakeJournalRepo()
	svc := NewTransferService(repo).WithJournalService(NewJournalService(journalRepo))

	err := svc.Process(context.Background(), owner.TenantID, owner.ID, "payer-1", "Carlos")
	if !errors.Is(err, ErrTransferSettlementTransactionRequired) {
		t.Fatalf("transaction guard error = %v", err)
	}
	if len(journalRepo.byID) != 0 {
		t.Fatalf("journal changed without transaction: %+v", journalRepo.byID)
	}
	if owner.Status != transfer.StatusApproved {
		t.Fatalf("transfer changed without transaction: %s", owner.Status)
	}
}

func TestTransferService_ProcessSettlementIsTenantScoped(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-A", "2026-08",
		5000, 8, 0, 0, nil, "")
	if err := owner.Approve("approver-1", "Ana"); err != nil {
		t.Fatal(err)
	}
	repo := newCaptureRepo(owner)
	journalRepo := newFakeJournalRepo()
	svc := NewTransferService(repo).WithJournalService(NewJournalService(journalRepo))
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	err := svc.Process(ctx, "tenant-B", owner.ID, "payer-1", "Carlos")
	if err == nil || err.Error() != "transfer not found" {
		t.Fatalf("cross-tenant settlement error = %v", err)
	}
	if len(journalRepo.byID) != 0 || owner.Status != transfer.StatusApproved {
		t.Fatalf("cross-tenant settlement changed state: journal=%d status=%s", len(journalRepo.byID), owner.Status)
	}
}

func TestTransferService_ProcessUsesCallerTransactionAndRetryIsExact(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "")
	owner.ID = "transfer-1"
	balance, err := transfer.NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	if err := owner.Approve("approver-1", "Ana"); err != nil {
		t.Fatal(err)
	}
	repo := newCaptureRepo(owner)
	journalRepo := newFakeJournalRepo()
	bus := &captureBus{}
	svc := NewTransferService(repo).
		WithJournalService(NewJournalService(journalRepo)).
		WithEventBus(bus)
	// Focused repositories do not execute SQL, so a transaction marker is
	// enough to exercise composition inside a caller-owned transaction. Real
	// commit/rollback semantics are covered by the PostgreSQL integration test.
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	if err := svc.Process(ctx, owner.TenantID, owner.ID, "payer-1", "Carlos"); err != nil {
		t.Fatalf("process with caller transaction: %v", err)
	}
	sourceID := ownerTransferSettlementSourceEventID(owner.TenantID, owner.ID)
	entry := journalRepo.bySource[sourceID]
	if entry == nil {
		t.Fatalf("canonical settlement %s was not created", sourceID)
	}
	if entry.Legs[0].AmountCents != 460000 || entry.Legs[1].AmountCents != 460000 {
		t.Fatalf("settlement used total instead of available: %+v", entry.Legs)
	}

	if err := svc.Process(ctx, owner.TenantID, owner.ID, "payer-1", "Carlos"); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	if len(journalRepo.byID) != 1 || len(bus.get()) != 1 {
		t.Fatalf("retry duplicated journal/event: journal=%d events=%d", len(journalRepo.byID), len(bus.get()))
	}
	stored, err := repo.GetByID(ctx, owner.TenantID, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.SettlementJournalSourceID != sourceID || countTransferHistoryAction(stored, "processed") != 1 {
		t.Fatalf("retry changed settlement identity/history: %+v", stored)
	}
}

func TestValidateProcessedTransferSettlementDoesNotBackfillLegacy(t *testing.T) {
	owner := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08",
		5000, 8, 0, 0, nil, "")
	owner.ID = "transfer-legacy"
	if err := owner.Approve("approver-1", "Ana"); err != nil {
		t.Fatal(err)
	}
	if err := owner.Process("payer-1", "Carlos"); err != nil {
		t.Fatal(err)
	}
	journalRepo := newFakeJournalRepo()
	svc := NewTransferService(newCaptureRepo(owner)).WithJournalService(NewJournalService(journalRepo))

	if err := svc.validateProcessedTransferSettlement(context.Background(), owner); !errors.Is(err, ErrTransferSettlementJournalMissing) {
		t.Fatalf("legacy processed transfer error = %v, want ErrTransferSettlementJournalMissing", err)
	}
	if len(journalRepo.byID) != 0 {
		t.Fatalf("legacy validation silently backfilled journal: %+v", journalRepo.byID)
	}
}
