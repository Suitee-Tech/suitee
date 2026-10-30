package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestRunInTxReusesTransactionFromContext(t *testing.T) {
	existing := &sqlx.Tx{}
	db := &PostgresDB{}

	called := false
	err := db.RunInTx(WithTx(context.Background(), existing), func(got *sqlx.Tx) error {
		called = true
		if got != existing {
			t.Fatalf("RunInTx transaction = %p, want existing %p", got, existing)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("RunInTx returned error: %v", err)
	}
	if !called {
		t.Fatal("RunInTx did not invoke callback")
	}
}

func TestRunInTxPropagatesNestedErrorToOuterOwner(t *testing.T) {
	existing := &sqlx.Tx{}
	db := &PostgresDB{}
	want := errors.New("nested write failed")

	err := db.RunInTx(WithTx(context.Background(), existing), func(got *sqlx.Tx) error {
		if got != existing {
			t.Fatalf("RunInTx transaction = %p, want existing %p", got, existing)
		}
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("RunInTx error = %v, want %v", err, want)
	}
}

func TestDeferTxUntilCommitQueuesOnlyRegisteredTx(t *testing.T) {
	called := 0
	if DeferTxUntilCommit(nil, func() { called++ }) {
		t.Fatal("callback without transaction was queued")
	}

	tx := &sqlx.Tx{}
	if DeferTxUntilCommit(tx, func() { called++ }) {
		t.Fatal("callback for unregistered tx was queued")
	}

	hooks := &afterCommitCallbacks{}
	afterCommitByTx.Store(tx, hooks)
	t.Cleanup(func() { afterCommitByTx.Delete(tx) })

	if !DeferTxUntilCommit(tx, func() { called++ }) {
		t.Fatal("managed transaction callback was not queued")
	}
	if called != 0 {
		t.Fatalf("callback ran before commit: %d", called)
	}
	hooks.run()
	if called != 1 {
		t.Fatalf("callback count after commit = %d, want 1", called)
	}
}

func TestDeferUntilCommitQueuesOnlyManagedTransactions(t *testing.T) {
	called := 0
	if DeferUntilCommit(context.Background(), func() { called++ }) {
		t.Fatal("callback without transaction was queued")
	}

	tx := &sqlx.Tx{}
	hooks := &afterCommitCallbacks{}
	afterCommitByTx.Store(tx, hooks)
	t.Cleanup(func() { afterCommitByTx.Delete(tx) })

	ctx := WithTx(context.Background(), tx)
	if !DeferUntilCommit(ctx, func() { called++ }) {
		t.Fatal("managed transaction callback was not queued")
	}
	if called != 0 {
		t.Fatalf("callback ran before commit: %d", called)
	}
	hooks.run()
	if called != 1 {
		t.Fatalf("callback count after commit = %d, want 1", called)
	}
}

func TestRunInServiceContextRejectsTenantTransaction(t *testing.T) {
	db := &PostgresDB{serviceRoleBypassesRLS: true}
	err := db.RunInServiceContext(WithTx(context.Background(), &sqlx.Tx{}), func(context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrServiceRoleRequired) {
		t.Fatalf("RunInServiceContext() error = %v, want ErrServiceRoleRequired", err)
	}
}

func TestRunInServiceContextReusesOnlyMarkedServiceTransaction(t *testing.T) {
	db := &PostgresDB{serviceRoleBypassesRLS: true}
	tx := &sqlx.Tx{}
	ctx := withServiceTx(context.Background(), tx)
	called := false
	err := db.RunInServiceContext(ctx, func(got context.Context) error {
		called = true
		if TxFromCtx(got) != tx || !ServiceTxFromCtx(got) {
			t.Fatal("service transaction marker was not preserved")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("RunInServiceContext() = (called=%v, err=%v)", called, err)
	}
}
