package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/invoice"
)

type mockInvoiceRepo struct {
	createErr            error
	getErr               error
	updateErr            error
	listErr              error
	inv                  *invoice.Invoice
	invoices             []*invoice.Invoice
	getTenantID          string
	getReferenceTenantID string
	lockCalls            int
	createCalls          int
	updateCalls          int
	reservationCalls     int
	beforeReservationSum func()
}

func (m *mockInvoiceRepo) Create(ctx context.Context, inv *invoice.Invoice) error {
	m.createCalls++
	if m.createErr != nil {
		return m.createErr
	}
	m.inv = inv
	m.invoices = append(m.invoices, inv)
	return nil
}
func (m *mockInvoiceRepo) GetByID(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	m.getTenantID = tenantID
	if m.inv != nil && m.inv.TenantID != tenantID {
		return nil, m.getErr
	}
	return m.inv, m.getErr
}
func (m *mockInvoiceRepo) GetByIDForUpdate(ctx context.Context, tenantID, id string) (*invoice.Invoice, error) {
	m.lockCalls++
	return m.GetByID(ctx, tenantID, id)
}
func (m *mockInvoiceRepo) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*invoice.Invoice, error) {
	if m.inv == nil || m.inv.TenantID != tenantID || m.inv.SourceEventID != sourceEventID {
		return nil, m.getErr
	}
	return m.inv, m.getErr
}
func (m *mockInvoiceRepo) SumOpenAmountCentsByChargeID(_ context.Context, tenantID, chargeID, excludedInvoiceID string) (int64, error) {
	m.reservationCalls++
	if m.beforeReservationSum != nil {
		m.beforeReservationSum()
	}
	seen := make(map[string]struct{})
	var total int64
	add := func(inv *invoice.Invoice) {
		if inv == nil || inv.ID == excludedInvoiceID || inv.TenantID != tenantID || inv.ChargeID != chargeID || inv.Status == invoice.StatusPaid || inv.Status == invoice.StatusCancelled {
			return
		}
		if _, exists := seen[inv.ID]; exists {
			return
		}
		seen[inv.ID] = struct{}{}
		total += valueobjects.NewMoneyFromFloat(inv.Amount).Cents()
	}
	for _, inv := range m.invoices {
		add(inv)
	}
	add(m.inv)
	return total, m.getErr
}
func (m *mockInvoiceRepo) GetByReferenceID(ctx context.Context, tenantID, refID string) ([]*invoice.Invoice, error) {
	m.getReferenceTenantID = tenantID
	return m.invoices, m.getErr
}
func (m *mockInvoiceRepo) Update(ctx context.Context, inv *invoice.Invoice) error {
	m.updateCalls++
	m.inv = inv
	return m.updateErr
}

type recordingInvoicePaymentApplication struct {
	canonical     *charge.Charge
	lookupCalls   int
	lockCalls     int
	lockSawTx     bool
	lookupErr     error
	calls         int
	tenantID      string
	chargeID      string
	paymentMethod string
	paidAmount    float64
	paymentID     string
	paidAt        time.Time
	err           error
}

type recordingInvoiceEventBus struct {
	reliableEvents []events.Event
	outboxEvents   []events.Event
	outboxCalls    int
	reliableErr    error
	outboxErr      error
}

func (b *recordingInvoiceEventBus) Publish(context.Context, events.Event) error { return nil }
func (b *recordingInvoiceEventBus) PublishReliable(_ context.Context, event events.Event) error {
	if b.reliableErr != nil {
		return b.reliableErr
	}
	b.reliableEvents = append(b.reliableEvents, event)
	return nil
}
func (b *recordingInvoiceEventBus) Subscribe(string, events.EventHandler) error { return nil }
func (b *recordingInvoiceEventBus) SubscribeWithQueue(string, string, events.EventHandler) error {
	return nil
}
func (b *recordingInvoiceEventBus) EnqueueOutboxInTx(_ context.Context, tx *sqlx.Tx, event events.Event) (string, error) {
	if tx == nil {
		return "", errors.New("transaction is required")
	}
	b.outboxCalls++
	if b.outboxErr != nil {
		return "", b.outboxErr
	}
	b.outboxEvents = append(b.outboxEvents, event)
	return "invoice-outbox-1", nil
}

func (a *recordingInvoicePaymentApplication) GetByID(_ context.Context, tenantID, chargeID string) (*charge.Charge, error) {
	a.lookupCalls++
	if a.lookupErr != nil {
		return nil, a.lookupErr
	}
	if a.canonical == nil || a.canonical.TenantID != tenantID || a.canonical.ID != chargeID {
		return nil, charge.ErrNotFound
	}
	return a.canonical, nil
}

func (a *recordingInvoicePaymentApplication) GetByIDForUpdate(ctx context.Context, tenantID, chargeID string) (*charge.Charge, error) {
	a.lockCalls++
	a.lockSawTx = database.TxFromCtx(ctx) != nil
	return a.GetByID(ctx, tenantID, chargeID)
}

func (a *recordingInvoicePaymentApplication) MarkInvoicePaidIdempotent(_ context.Context, tenantID, chargeID, _ string, paymentMethod string, paidAmount float64, paymentID string, paidAt time.Time) (*charge.Charge, error) {
	a.calls++
	a.tenantID = tenantID
	a.chargeID = chargeID
	a.paymentMethod = paymentMethod
	a.paidAmount = paidAmount
	a.paymentID = paymentID
	a.paidAt = paidAt
	if a.err != nil {
		return nil, a.err
	}
	return a.canonical, nil
}

func canonicalChargeForInvoice(t *testing.T, inv *invoice.Invoice, total float64) *charge.Charge {
	t.Helper()
	c, err := charge.New(inv.ReferenceID, inv.PayerID, "owner-1", "property-1", inv.TenantID, charge.TypeRent, "canonical receivable", "2026-08", total, inv.DueDate)
	if err != nil {
		t.Fatalf("create canonical charge: %v", err)
	}
	c.ID = inv.ChargeID
	return c
}
func (m *mockInvoiceRepo) List(ctx context.Context, tenantID string, status *invoice.Status, referenceID *string) ([]*invoice.Invoice, error) {
	return m.invoices, m.listErr
}
func (m *mockInvoiceRepo) Search(ctx context.Context, tenantID string, filter invoice.SearchFilter) ([]*invoice.Invoice, int64, error) {
	return m.invoices, int64(len(m.invoices)), m.listErr
}

func TestInvoiceService_Create(t *testing.T) {
	repo := &mockInvoiceRepo{}
	svc := NewInvoiceService(repo, nil)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		inv, err := svc.Create(ctx, "ref-1", "payer-1", 100.0, time.Now(), invoice.TypeRent, "tenant-1")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if inv == nil {
			t.Fatal("expected invoice, got nil")
		}
		if inv.Status != invoice.StatusIssued {
			t.Errorf("expected status ISSUED, got %s", inv.Status)
		}
	})

	t.Run("invalid amount", func(t *testing.T) {
		_, err := svc.Create(ctx, "ref-1", "payer-1", 0, time.Now(), invoice.TypeRent, "tenant-1")
		if err == nil {
			t.Fatal("expected error for zero amount")
		}
	})
}

func TestInvoiceServiceCreateWithChargeValidatesBeforePersisting(t *testing.T) {
	dueDate := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	seed, err := invoice.New("contract-1", "payer-1", 40, dueDate, invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create invoice seed: %v", err)
	}
	seed.ChargeID = "charge-1"
	canonical := canonicalChargeForInvoice(t, seed, 100)
	canonical.ContractID = "another-contract"
	repo := &mockInvoiceRepo{}
	application := &recordingInvoicePaymentApplication{canonical: canonical}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)

	_, err = svc.CreateWithSourceAndCharge(context.Background(), "contract-1", "payer-1", 40, dueDate, invoice.TypeRent, "tenant-1", "invoice-command-1", "charge-1")
	if !errors.Is(err, ErrInvoiceChargeContractMismatch) {
		t.Fatalf("CreateWithSourceAndCharge() error = %v, want contract mismatch", err)
	}
	if application.lookupCalls != 1 || repo.createCalls != 0 {
		t.Fatalf("invalid link persisted: lookups=%d creates=%d", application.lookupCalls, repo.createCalls)
	}
}

func TestInvoiceServiceCreateIdempotencyKeyOwnsOneImmutableCommand(t *testing.T) {
	dueDate := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	existing, err := invoice.New("contract-1", "payer-1", 40, dueDate, invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create canonical invoice: %v", err)
	}
	existing.ID = "invoice-canonical"
	existing.Status = invoice.StatusIssued
	existing.ChargeID = "charge-1"
	existing.SourceEventID = "invoice-command-1"
	repo := &mockInvoiceRepo{inv: existing}
	svc := NewInvoiceService(repo, nil)

	got, err := svc.CreateWithSourceAndCharge(context.Background(), "contract-1", "payer-1", 40, dueDate, invoice.TypeRent, "tenant-1", "invoice-command-1", "charge-1")
	if err != nil || got != existing {
		t.Fatalf("identical retry = (%+v, %v), want canonical row", got, err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("identical retry attempted %d creates", repo.createCalls)
	}

	_, err = svc.CreateWithSourceAndCharge(context.Background(), "contract-1", "payer-1", 41, dueDate, invoice.TypeRent, "tenant-1", "invoice-command-1", "charge-1")
	if !errors.Is(err, ErrInvoiceIdempotencyConflict) {
		t.Fatalf("changed retry error = %v, want idempotency conflict", err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("conflicting retry attempted %d creates", repo.createCalls)
	}
}

func TestInvoiceServiceAtomicIssuanceReservesOnlyOpenInvoices(t *testing.T) {
	dueDate := time.Date(2026, time.September, 10, 0, 0, 0, 0, time.UTC)
	seed, err := invoice.New("contract-1", "payer-1", 40, dueDate, invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create invoice seed: %v", err)
	}
	seed.ChargeID = "charge-1"
	canonical := canonicalChargeForInvoice(t, seed, 100)
	paid := valueobjects.NewMoney(3_000)
	canonical.PaidAmount = &paid
	canonical.Status = charge.StatusPartiallyPaid

	linked := func(id string, amount float64, status invoice.Status) *invoice.Invoice {
		inv, newErr := invoice.New(seed.ReferenceID, seed.PayerID, amount, dueDate, seed.Type, seed.TenantID)
		if newErr != nil {
			t.Fatalf("create %s invoice: %v", status, newErr)
		}
		inv.ID = id
		inv.ChargeID = seed.ChargeID
		inv.Status = status
		return inv
	}
	repo := &mockInvoiceRepo{invoices: []*invoice.Invoice{
		linked("issued-20", 20, invoice.StatusIssued),
		linked("overdue-10", 10, invoice.StatusOverdue),
		linked("paid-30", 30, invoice.StatusPaid),
		linked("cancelled-50", 50, invoice.StatusCancelled),
	}}
	application := &recordingInvoicePaymentApplication{canonical: canonical}
	repo.beforeReservationSum = func() {
		if application.lockCalls == 0 || !application.lockSawTx {
			t.Fatal("reservation was summed before the Charge lock inside a transaction")
		}
	}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)
	svc.db = &database.PostgresDB{}
	svc.creationTx = func(ctx context.Context, work func(context.Context) error) error {
		return work(database.WithTx(ctx, &sqlx.Tx{}))
	}

	created, err := svc.CreateWithSourceAndCharge(
		context.Background(), seed.ReferenceID, seed.PayerID, 40, dueDate, seed.Type,
		seed.TenantID, "command-reserves-last-40", seed.ChargeID,
	)
	if err != nil {
		t.Fatalf("create within unreserved balance: %v", err)
	}
	if created.Status != invoice.StatusIssued || repo.createCalls != 1 {
		t.Fatalf("created invoice = %+v creates=%d", created, repo.createCalls)
	}

	// A retry resolves the immutable command before re-validating today's
	// balance. It must return the row above without acquiring another lock.
	retried, err := svc.CreateWithSourceAndCharge(
		context.Background(), seed.ReferenceID, seed.PayerID, 40, dueDate, seed.Type,
		seed.TenantID, "command-reserves-last-40", seed.ChargeID,
	)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("canonical retry = (%+v, %v), want invoice %s", retried, err, created.ID)
	}

	_, err = svc.CreateWithSourceAndCharge(
		context.Background(), seed.ReferenceID, seed.PayerID, 0.01, dueDate, seed.Type,
		seed.TenantID, "different-command-over-reserves", seed.ChargeID,
	)
	if !errors.Is(err, ErrInvoiceAmountExceedsChargeBalance) {
		t.Fatalf("over-reservation error = %v, want ErrInvoiceAmountExceedsChargeBalance", err)
	}
	if repo.createCalls != 1 || application.lockCalls != 2 || repo.reservationCalls != 2 {
		t.Fatalf("atomic issuance calls: creates=%d locks=%d sums=%d", repo.createCalls, application.lockCalls, repo.reservationCalls)
	}
}

func TestInvoiceService_Pay(t *testing.T) {
	inv, _ := invoice.New("ref-1", "payer-1", 100.0, time.Now(), invoice.TypeRent, "tenant-1")
	inv.ChargeID = "charge-1"
	repo := &mockInvoiceRepo{
		inv: inv,
	}
	paymentApplication := &recordingInvoicePaymentApplication{canonical: canonicalChargeForInvoice(t, inv, 100)}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(paymentApplication)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		res, err := svc.Pay(ctx, "tenant-1", inv.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.Status != invoice.StatusPaid {
			t.Errorf("expected status PAID, got %s", res.Status)
		}
		if paymentApplication.calls != 1 || paymentApplication.chargeID != "charge-1" || paymentApplication.paidAmount != 100 {
			t.Fatalf("canonical payment application = %+v", paymentApplication)
		}
	})

	t.Run("already paid is idempotent", func(t *testing.T) {
		res, err := svc.Pay(ctx, "tenant-1", inv.ID)
		if err != nil || res.Status != invoice.StatusPaid {
			t.Fatalf("idempotent retry = (%+v, %v)", res, err)
		}
		if paymentApplication.calls != 1 {
			t.Fatalf("retry reapplied canonical payment: calls=%d", paymentApplication.calls)
		}
	})

	t.Run("unauthorized tenant", func(t *testing.T) {
		inv.Status = invoice.StatusIssued // reset
		_, err := svc.Pay(ctx, "tenant-2", inv.ID)
		if err == nil {
			t.Fatal("expected error for unauthorized tenant")
		}
	})
}

func TestInvoiceServicePayIdempotentAllocatesThroughCanonicalCharge(t *testing.T) {
	paidAt := time.Date(2026, time.August, 13, 12, 30, 0, 0, time.UTC)
	inv, _ := invoice.New("contract-1", "payer-1", 75.25, paidAt.Add(24*time.Hour), invoice.TypeRent, "tenant-1")
	inv.Status = invoice.StatusIssued
	inv.ChargeID = "charge-1"
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{canonical: canonicalChargeForInvoice(t, inv, 100)}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)

	got, err := svc.PayIdempotent(context.Background(), "tenant-1", inv.ID, "payment-command-1", "PIX", paidAt)
	if err != nil {
		t.Fatalf("PayIdempotent() error = %v", err)
	}
	if got.Status != invoice.StatusPaid || got.PaidAt == nil || !got.PaidAt.Equal(paidAt) {
		t.Fatalf("invoice projection not settled with canonical timestamp: %+v", got)
	}
	if application.calls != 1 || application.tenantID != "tenant-1" || application.chargeID != "charge-1" || application.paymentID != "payment-command-1" || application.paymentMethod != "PIX" || application.paidAmount != 75.25 || !application.paidAt.Equal(paidAt) {
		t.Fatalf("unexpected canonical payment command: %+v", application)
	}
	if repo.updateCalls != 1 {
		t.Fatalf("invoice projection updates = %d, want 1", repo.updateCalls)
	}

	if _, err := svc.PayIdempotent(context.Background(), "tenant-1", inv.ID, "payment-command-1", "PIX", paidAt); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if application.calls != 1 || repo.updateCalls != 1 {
		t.Fatalf("retry duplicated writes: payment calls=%d invoice updates=%d", application.calls, repo.updateCalls)
	}
}

func TestInvoiceServicePublishesRealInvoicePaidOnceForPartialChargeAllocation(t *testing.T) {
	paidAt := time.Date(2026, time.August, 13, 12, 30, 0, 0, time.UTC)
	inv, err := invoice.New("contract-1", "payer-1", 40, paidAt.Add(24*time.Hour), invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	inv.ID = "invoice-1"
	inv.Status = invoice.StatusIssued
	inv.ChargeID = "charge-1"
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{canonical: canonicalChargeForInvoice(t, inv, 100)}
	bus := &recordingInvoiceEventBus{}
	svc := NewInvoiceService(repo, bus).WithPaymentApplication(application)

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := svc.PayIdempotent(context.Background(), inv.TenantID, inv.ID, "payment-1", "PIX", paidAt); err != nil {
			t.Fatalf("PayIdempotent() attempt %d: %v", attempt+1, err)
		}
	}
	if len(bus.reliableEvents) != 1 || len(bus.outboxEvents) != 0 {
		t.Fatalf("invoice events reliable=%d outbox=%d, want exactly one reliable event", len(bus.reliableEvents), len(bus.outboxEvents))
	}
	event := bus.reliableEvents[0]
	payload, ok := event.Payload().(contracts.InvoicePaidEventPayload)
	if !ok {
		t.Fatalf("invoice.paid payload type = %T", event.Payload())
	}
	if event.Name() != "invoice.paid" || payload.SchemaVersion != 2 || payload.InvoiceID != inv.ID || payload.ChargeID != inv.ChargeID || payload.Amount != inv.Amount || payload.TenantID != inv.TenantID || payload.ReferenceID != inv.ReferenceID {
		t.Fatalf("unexpected invoice.paid event: name=%s payload=%+v", event.Name(), payload)
	}
	if application.calls != 1 || application.canonical.Status == charge.StatusPaid {
		t.Fatalf("partial invoice allocation duplicated or fabricated full charge: application=%+v charge=%+v", application, application.canonical)
	}
}

func TestInvoiceServicePropagatesTransactionalOutboxFailure(t *testing.T) {
	inv, err := invoice.New("contract-1", "payer-1", 40, time.Now(), invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	inv.ID = "invoice-1"
	inv.Status = invoice.StatusIssued
	inv.ChargeID = "charge-1"
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{canonical: canonicalChargeForInvoice(t, inv, 100)}
	wantErr := errors.New("outbox insert failed")
	bus := &recordingInvoiceEventBus{outboxErr: wantErr}
	svc := NewInvoiceService(repo, bus).WithPaymentApplication(application)
	svc.db = &database.PostgresDB{}
	svc.paymentTx = func(ctx context.Context, work func(context.Context) error) error {
		snapshot := *repo.inv
		err := work(database.WithTx(ctx, &sqlx.Tx{}))
		if err != nil {
			repo.inv = &snapshot
		}
		return err
	}

	_, err = svc.PayIdempotent(context.Background(), inv.TenantID, inv.ID, "payment-1", "PIX", time.Now())
	if !errors.Is(err, wantErr) {
		t.Fatalf("PayIdempotent() error = %v, want outbox failure", err)
	}
	if repo.inv.Status != invoice.StatusIssued || repo.updateCalls != 1 || bus.outboxCalls != 1 || len(bus.reliableEvents) != 0 || len(bus.outboxEvents) != 0 {
		t.Fatalf("outbox failure escaped transaction: invoice=%+v updates=%d attempts=%d reliable=%d outbox=%d", repo.inv, repo.updateCalls, bus.outboxCalls, len(bus.reliableEvents), len(bus.outboxEvents))
	}
}

func TestInvoiceServiceRejectsUnallocatedInvoicePayment(t *testing.T) {
	inv, _ := invoice.New("contract-legacy", "payer-1", 50, time.Now(), invoice.TypeRent, "tenant-1")
	inv.Status = invoice.StatusIssued
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)

	_, err := svc.PayIdempotent(context.Background(), "tenant-1", inv.ID, "payment-1", "PIX", time.Now())
	if !errors.Is(err, ErrInvoiceChargeLinkRequired) {
		t.Fatalf("PayIdempotent() error = %v, want ErrInvoiceChargeLinkRequired", err)
	}
	if application.calls != 0 || repo.updateCalls != 0 || inv.Status != invoice.StatusIssued {
		t.Fatalf("unallocated invoice mutated: app=%d updates=%d status=%s", application.calls, repo.updateCalls, inv.Status)
	}
}

func TestInvoiceServiceDoesNotAdvanceProjectionWhenChargePaymentFails(t *testing.T) {
	inv, _ := invoice.New("contract-1", "payer-1", 50, time.Now(), invoice.TypeRent, "tenant-1")
	inv.Status = invoice.StatusIssued
	inv.ChargeID = "charge-1"
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{
		canonical: canonicalChargeForInvoice(t, inv, 100),
		err:       errors.New("charge write failed"),
	}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)

	_, err := svc.PayIdempotent(context.Background(), "tenant-1", inv.ID, "payment-1", "PIX", time.Now())
	if err == nil {
		t.Fatal("expected canonical charge payment failure")
	}
	if repo.updateCalls != 0 || inv.Status != invoice.StatusIssued || inv.PaidAt != nil {
		t.Fatalf("failed charge payment advanced invoice: updates=%d invoice=%+v", repo.updateCalls, inv)
	}
}

func TestInvoiceServiceLocksProjectionBeforeAllocatingInsideTransaction(t *testing.T) {
	inv, _ := invoice.New("contract-1", "payer-1", 50, time.Now(), invoice.TypeRent, "tenant-1")
	inv.Status = invoice.StatusIssued
	inv.ChargeID = "charge-1"
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{canonical: canonicalChargeForInvoice(t, inv, 100)}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)
	ctx := database.WithTx(context.Background(), &sqlx.Tx{})

	if _, err := svc.PayIdempotent(ctx, "tenant-1", inv.ID, "payment-1", "PIX", time.Now()); err != nil {
		t.Fatalf("PayIdempotent() error = %v", err)
	}
	if repo.lockCalls != 1 {
		t.Fatalf("locked invoice reads = %d, want 1", repo.lockCalls)
	}
}

func TestInvoiceServiceRejectsCanonicalChargeMismatchBeforeApplyingPayment(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*charge.Charge)
		wantErr error
	}{
		{
			name: "contract",
			mutate: func(c *charge.Charge) {
				c.ContractID = "other-contract"
			},
			wantErr: ErrInvoiceChargeContractMismatch,
		},
		{
			name: "payer",
			mutate: func(c *charge.Charge) {
				c.TenantPartyID = "other-payer"
			},
			wantErr: ErrInvoiceChargePayerMismatch,
		},
		{
			name: "amount exceeds remaining balance",
			mutate: func(c *charge.Charge) {
				paid := valueobjects.NewMoney(7_000)
				c.PaidAmount = &paid
				c.Status = charge.StatusPartiallyPaid
			},
			wantErr: ErrInvoiceAmountExceedsChargeBalance,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, err := invoice.New("contract-1", "payer-1", 40, time.Now(), invoice.TypeRent, "tenant-1")
			if err != nil {
				t.Fatalf("create invoice: %v", err)
			}
			inv.Status = invoice.StatusIssued
			inv.ChargeID = "charge-1"
			canonical := canonicalChargeForInvoice(t, inv, 100)
			tt.mutate(canonical)
			repo := &mockInvoiceRepo{inv: inv}
			application := &recordingInvoicePaymentApplication{canonical: canonical}
			svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)

			_, err = svc.PayIdempotent(context.Background(), inv.TenantID, inv.ID, "payment-1", "PIX", time.Now())
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("PayIdempotent() error = %v, want %v", err, tt.wantErr)
			}
			if application.lookupCalls != 1 || application.calls != 0 || repo.updateCalls != 0 || inv.Status != invoice.StatusIssued {
				t.Fatalf("mismatch mutated state: lookup=%d payment=%d updates=%d status=%s", application.lookupCalls, application.calls, repo.updateCalls, inv.Status)
			}
		})
	}
}

func TestInvoiceServiceAllowsPartialAmountWithinOutstandingBalance(t *testing.T) {
	inv, err := invoice.New("contract-1", "payer-1", 40, time.Now(), invoice.TypeRent, "tenant-1")
	if err != nil {
		t.Fatalf("create invoice: %v", err)
	}
	inv.Status = invoice.StatusIssued
	inv.ChargeID = "charge-1"
	canonical := canonicalChargeForInvoice(t, inv, 100)
	paid := valueobjects.NewMoney(6_000)
	canonical.PaidAmount = &paid
	canonical.Status = charge.StatusPartiallyPaid
	repo := &mockInvoiceRepo{inv: inv}
	application := &recordingInvoicePaymentApplication{canonical: canonical}
	svc := NewInvoiceService(repo, nil).WithPaymentApplication(application)

	got, err := svc.PayIdempotent(context.Background(), inv.TenantID, inv.ID, "payment-1", "PIX", time.Now())
	if err != nil {
		t.Fatalf("PayIdempotent() error = %v", err)
	}
	if got.Status != invoice.StatusPaid || application.lookupCalls != 1 || application.calls != 1 || application.paidAmount != 40 {
		t.Fatalf("valid partial allocation = invoice=%+v application=%+v", got, application)
	}
}

func TestInvoiceService_GetByID(t *testing.T) {
	inv, _ := invoice.New("ref-1", "payer-1", 100.0, time.Now(), invoice.TypeRent, "tenant-1")
	repo := &mockInvoiceRepo{
		inv: inv,
	}
	svc := NewInvoiceService(repo, nil)

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		res, err := svc.GetByID(ctx, "tenant-1", inv.ID)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if res.ID != inv.ID {
			t.Errorf("expected id %s, got %s", inv.ID, res.ID)
		}
		if repo.getTenantID != "tenant-1" {
			t.Errorf("repository tenant: got %q, want tenant-1", repo.getTenantID)
		}
	})

	t.Run("tenant scoped not found", func(t *testing.T) {
		_, err := svc.GetByID(ctx, "tenant-2", inv.ID)
		if err == nil {
			t.Fatal("expected foreign-tenant invoice to be hidden")
		}
		if repo.getTenantID != "tenant-2" {
			t.Errorf("repository tenant: got %q, want tenant-2", repo.getTenantID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		repo.inv = nil
		_, err := svc.GetByID(ctx, "tenant-1", "invalid-id")
		if err == nil {
			t.Fatal("expected error for not found")
		}
		repo.inv = inv // reset
	})

	t.Run("db error", func(t *testing.T) {
		repo.getErr = errors.New("db error")
		_, err := svc.GetByID(ctx, "tenant-1", inv.ID)
		if err == nil {
			t.Fatal("expected db error")
		}
		repo.getErr = nil // reset
	})
}

func TestInvoiceService_GetByReferenceIDScopesTenant(t *testing.T) {
	t.Parallel()
	inv, _ := invoice.New("ref-1", "payer-1", 100.0, time.Now(), invoice.TypeRent, "tenant-1")
	repo := &mockInvoiceRepo{invoices: []*invoice.Invoice{inv}}
	svc := NewInvoiceService(repo, nil)

	got, err := svc.GetByReferenceID(context.Background(), "tenant-1", "ref-1")
	if err != nil {
		t.Fatalf("get by reference: %v", err)
	}
	if len(got) != 1 || got[0].ID != inv.ID {
		t.Fatalf("unexpected invoices: %#v", got)
	}
	if repo.getReferenceTenantID != "tenant-1" {
		t.Fatalf("repository tenant: got %q, want tenant-1", repo.getReferenceTenantID)
	}
}
