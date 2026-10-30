package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

type failOnceAuditTransferHook struct {
	calls int
}

func (h *failOnceAuditTransferHook) EnsureMonthlyForChargePaid(context.Context, *charge.Charge) error {
	h.calls++
	if h.calls == 1 {
		return errors.New("injected transfer failure")
	}
	return nil
}

type auditPaymentCollectionResolver struct {
	calls int
}

func (r *auditPaymentCollectionResolver) ResolveChargePayment(context.Context, string, string) error {
	r.calls++
	return nil
}

type auditPaymentBus struct {
	outboxCalls  int
	publishCalls int
	outboxEvents []events.Event
}

func (b *auditPaymentBus) Publish(context.Context, events.Event) error {
	b.publishCalls++
	return nil
}

func (b *auditPaymentBus) PublishReliable(context.Context, events.Event) error {
	b.publishCalls++
	return nil
}

func (b *auditPaymentBus) Subscribe(string, events.EventHandler) error { return nil }

func (b *auditPaymentBus) SubscribeWithQueue(string, string, events.EventHandler) error { return nil }

func (b *auditPaymentBus) EnqueueOutboxInTx(_ context.Context, tx *sqlx.Tx, event events.Event) (string, error) {
	if tx == nil {
		return "", errors.New("transaction is required")
	}
	b.outboxCalls++
	b.outboxEvents = append(b.outboxEvents, event)
	return "outbox-payment", nil
}

func TestMarkPaidAuditRunsOnlyAfterCommitAndOnceAcrossRetry(t *testing.T) {
	c, err := charge.New(
		"contract-audit",
		"payer-audit",
		"owner-audit",
		"property-audit",
		"tenant-audit",
		charge.TypeRent,
		"Aluguel",
		"2026-08",
		100,
		time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("create charge fixture: %v", err)
	}
	c.ID = "charge-audit"
	repo := &mockChargeRepo{ch: c}
	hook := &failOnceAuditTransferHook{}
	resolver := &auditPaymentCollectionResolver{}
	bus := &auditPaymentBus{}
	svc := NewChargeService(repo, nil).WithInvoiceReservations(&fixedOpenInvoiceReservations{})
	// A non-nil DB activates the production atomicity guards. The injected
	// runner snapshots the in-memory repository and restores it on error,
	// mirroring the commit/rollback boundary of Postgres without a socket.
	svc.db = &database.PostgresDB{}
	svc.paymentTx = func(ctx context.Context, work func(context.Context) error) error {
		snapshot := clonePaymentCharge(repo.ch)
		txCtx := database.WithTx(ctx, &sqlx.Tx{})
		if err := work(txCtx); err != nil {
			repo.ch = snapshot
			return err
		}
		return nil
	}
	svc.SetTransferHook(hook)
	svc.SetCollectionResolver(resolver)
	svc.WithEventBus(bus)

	var auditEntries []audit.Entry
	auditObservedInsideTx := false
	svc.paymentAudit = func(ctx context.Context, entry audit.Entry) {
		if database.TxFromCtx(ctx) != nil {
			auditObservedInsideTx = true
		}
		auditEntries = append(auditEntries, entry)
	}
	paidAt := time.Date(2026, time.August, 10, 14, 0, 0, 0, time.UTC)

	_, err = svc.MarkPaidIdempotent(
		context.Background(), c.TenantID, c.ID, "PIX", 100, "payment-audit", paidAt,
	)
	if err == nil {
		t.Fatal("expected downstream failure to roll back payment")
	}
	if len(auditEntries) != 0 {
		t.Fatalf("rolled-back payment emitted audit entries: %d", len(auditEntries))
	}
	if repo.ch.Status != charge.StatusPending || repo.ch.PaidAmount != nil || len(repo.ch.Payments) != 0 {
		t.Fatalf("rollback did not restore charge snapshot: %+v", repo.ch)
	}

	paid, err := svc.MarkPaidIdempotent(
		context.Background(), c.TenantID, c.ID, "PIX", 100, "payment-audit", paidAt,
	)
	if err != nil {
		t.Fatalf("retry after rollback: %v", err)
	}
	if paid.Status != charge.StatusPaid {
		t.Fatalf("expected committed retry to pay charge, got %s", paid.Status)
	}
	if len(auditEntries) != 1 {
		t.Fatalf("committed payment must emit exactly one audit entry, got %d", len(auditEntries))
	}
	if auditObservedInsideTx {
		t.Fatal("payment audit ran before the transaction owner returned commit success")
	}
	entry := auditEntries[0]
	if entry.Action != "charge.paid" || entry.EntityID != c.ID {
		t.Fatalf("unexpected payment audit entry: %+v", entry)
	}
	if entry.Before["status"] != string(charge.StatusPending) || entry.After["status"] != string(charge.StatusPaid) {
		t.Fatalf("unexpected audit transition: before=%v after=%v", entry.Before, entry.After)
	}
	if entry.Metadata["payment_id"] != "payment-audit" {
		t.Fatalf("audit lost idempotency identity: %+v", entry.Metadata)
	}

	// A client retry after the successful commit reads the canonical payment,
	// returns it, and must not emit another audit or downstream side effect.
	paid, err = svc.MarkPaidIdempotent(
		context.Background(), c.TenantID, c.ID, "PIX", 100, "payment-audit", paidAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("idempotent retry after commit: %v", err)
	}
	if paid.Status != charge.StatusPaid || len(paid.Payments) != 1 {
		t.Fatalf("retry did not return canonical paid charge: %+v", paid)
	}
	if len(auditEntries) != 1 {
		t.Fatalf("idempotent retry duplicated audit: %d entries", len(auditEntries))
	}
	if hook.calls != 2 || resolver.calls != 1 || bus.outboxCalls != 1 || bus.publishCalls != 0 {
		t.Fatalf(
			"unexpected downstream calls across rollback/retry: hook=%d collection=%d outbox=%d publish=%d",
			hook.calls, resolver.calls, bus.outboxCalls, bus.publishCalls,
		)
	}
	if len(bus.outboxEvents) != 1 || bus.outboxEvents[0].Name() != "charge.paid" {
		t.Fatalf("canonical settlement event = %+v, want one charge.paid", bus.outboxEvents)
	}
	payload, ok := bus.outboxEvents[0].Payload().(contracts.ChargePaidEventPayload)
	if !ok {
		t.Fatalf("charge.paid payload type = %T", bus.outboxEvents[0].Payload())
	}
	if payload.ChargeID != c.ID || payload.ContractID != c.ContractID || payload.TenantID != c.TenantID || payload.Amount != 100 {
		t.Fatalf("unexpected charge.paid payload: %+v", payload)
	}
}
