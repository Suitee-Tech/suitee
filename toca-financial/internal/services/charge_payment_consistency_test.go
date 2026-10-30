package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

type recordingChargeTransferHook struct {
	calls int
}

func (h *recordingChargeTransferHook) EnsureMonthlyForChargePaid(context.Context, *charge.Charge) error {
	h.calls++
	return nil
}

type recordingCollectionResolver struct {
	calls int
}

type fixedOpenInvoiceReservations struct {
	openCents       int64
	excludedInvoice string
	calls           int
}

func (r *fixedOpenInvoiceReservations) SumOpenAmountCentsByChargeID(_ context.Context, _, _, excludedInvoiceID string) (int64, error) {
	r.calls++
	r.excludedInvoice = excludedInvoiceID
	if excludedInvoiceID == "invoice-60" {
		return 0, nil
	}
	return r.openCents, nil
}

func (r *recordingCollectionResolver) ResolveChargePayment(context.Context, string, string) error {
	r.calls++
	return nil
}

func TestMarkPaidIdempotentReleasesDownstreamOnlyAfterFullPayment(t *testing.T) {
	c, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Rent", "2026-08", 100, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	repo := &mockChargeRepo{ch: c}
	hook := &recordingChargeTransferHook{}
	resolver := &recordingCollectionResolver{}
	svc := NewChargeService(repo, nil)
	svc.SetTransferHook(hook)
	svc.SetCollectionResolver(resolver)
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	partial, err := svc.MarkPaidIdempotent(context.Background(), c.TenantID, c.ID, "PIX", 40, "payment-1", paidAt)
	if err != nil {
		t.Fatalf("partial payment: %v", err)
	}
	if partial.Status != charge.StatusPartiallyPaid || hook.calls != 0 || resolver.calls != 0 {
		t.Fatalf("partial payment released downstream: status=%s hook=%d collection=%d", partial.Status, hook.calls, resolver.calls)
	}

	paid, err := svc.MarkPaidIdempotent(context.Background(), c.TenantID, c.ID, "PIX", 60, "payment-2", paidAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("final payment: %v", err)
	}
	if paid.Status != charge.StatusPaid || hook.calls != 1 || resolver.calls != 1 {
		t.Fatalf("full payment did not release downstream once: status=%s hook=%d collection=%d", paid.Status, hook.calls, resolver.calls)
	}
}

func TestMarkPaidIdempotentDoesNotDuplicateBalance(t *testing.T) {
	c, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Rent", "2026-08", 100, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewChargeService(&mockChargeRepo{ch: c}, nil)
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	if _, err := svc.MarkPaidIdempotent(context.Background(), c.TenantID, c.ID, "PIX", 40, "payment-1", paidAt); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.MarkPaidIdempotent(context.Background(), c.TenantID, c.ID, "PIX", 40, "payment-1", paidAt.Add(time.Minute)); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	if c.PaidAmount == nil || c.PaidAmount.Cents() != 4000 || len(c.Payments) != 1 {
		t.Fatalf("retry duplicated payment: paid=%v payments=%d", c.PaidAmount, len(c.Payments))
	}
}

func TestTransactionalChargePaymentLocksAndPreservesOpenInvoiceReservations(t *testing.T) {
	c, err := charge.New("contract-1", "payer-1", "owner-1", "property-1", "tenant-1", charge.TypeRent, "Rent", "2026-08", 100, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	c.ID = "charge-1"
	repo := &mockChargeRepo{ch: c}
	reservations := &fixedOpenInvoiceReservations{openCents: 6_000}
	svc := NewChargeService(repo, nil).WithInvoiceReservations(reservations)
	svc.db = &database.PostgresDB{}
	svc.paymentTx = func(ctx context.Context, work func(context.Context) error) error {
		return work(database.WithTx(ctx, &sqlx.Tx{}))
	}
	svc.paymentAudit = func(context.Context, audit.Entry) {}

	_, err = svc.MarkPaidIdempotent(context.Background(), c.TenantID, c.ID, "PIX", 80, "direct-payment-80", time.Now())
	if !errors.Is(err, ErrChargePaymentExceedsUnreservedBalance) {
		t.Fatalf("direct payment error = %v, want reservation conflict", err)
	}
	if len(c.Payments) != 0 || c.PaidAmount != nil || repo.lockCalls != 1 {
		t.Fatalf("rejected payment mutated or skipped lock: payments=%d paid=%v locks=%d", len(c.Payments), c.PaidAmount, repo.lockCalls)
	}

	paid, err := svc.MarkInvoicePaidIdempotent(context.Background(), c.TenantID, c.ID, "invoice-60", "PIX", 60, "invoice-payment-60", time.Now())
	if err != nil {
		t.Fatalf("consume matching invoice reservation: %v", err)
	}
	if paid.PaidAmount == nil || paid.PaidAmount.Cents() != 6_000 || len(paid.Payments) != 1 || reservations.excludedInvoice != "invoice-60" || repo.lockCalls != 2 {
		t.Fatalf("invoice payment state=%+v reservation=%+v locks=%d", paid, reservations, repo.lockCalls)
	}

	if _, err := svc.MarkInvoicePaidIdempotent(context.Background(), c.TenantID, c.ID, "invoice-60", "PIX", 60, "invoice-payment-60", time.Now()); err != nil {
		t.Fatalf("invoice payment retry: %v", err)
	}
	if paid.PaidAmount.Cents() != 6_000 || len(paid.Payments) != 1 || reservations.calls != 2 || repo.lockCalls != 3 {
		t.Fatalf("retry duplicated payment or reservation lookup: paid=%d payments=%d reservation_calls=%d locks=%d", paid.PaidAmount.Cents(), len(paid.Payments), reservations.calls, repo.lockCalls)
	}
}
