package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/pkg/database"
)

// TestRunWithLock_NoDB_BypassesLock cobre o modo legacy (single-instance):
// quando WithDB não foi chamado, fn deve rodar normalmente sem tentar
// pegar advisory lock — útil para dev/test sem Postgres real.
func TestRunWithLock_NoDB_BypassesLock(t *testing.T) {
	svc := &DelinquencyService{}
	fnCalled := false
	res, err, ran := svc.runWithLock(context.Background(), "test", func(ctx context.Context) (*ProcessResult, error) {
		fnCalled = true
		return &ProcessResult{ChargesMarkedOverdue: 3}, nil
	})

	if !fnCalled {
		t.Error("fn should be called when db is nil (legacy bypass)")
	}
	if !ran {
		t.Error("ran should be true when fn executed")
	}
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if res == nil || res.ChargesMarkedOverdue != 3 {
		t.Errorf("result not propagated correctly: %+v", res)
	}
}

// An uninitialised database wrapper must use the safe single-instance path.
// Exact one-slot capacity is covered by pkg/database's pool tests.
func TestRunWithLock_UninitializedDBBypassesLock(t *testing.T) {
	svc := &DelinquencyService{db: &database.PostgresDB{}}
	fnCalled := false

	res, err, ran := svc.runWithLock(context.Background(), "single-connection", func(context.Context) (*ProcessResult, error) {
		fnCalled = true
		return &ProcessResult{ChargesMarkedOverdue: 1}, nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ran || !fnCalled {
		t.Fatal("protected function must run when advisory locking is unsafe")
	}
	if res == nil || res.ChargesMarkedOverdue != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

// TestRunWithLock_NoDB_PropagatesError garante que erros de fn não são
// silenciados no bypass path.
func TestRunWithLock_NoDB_PropagatesError(t *testing.T) {
	svc := &DelinquencyService{}
	expected := errors.New("boom")
	_, err, ran := svc.runWithLock(context.Background(), "test", func(ctx context.Context) (*ProcessResult, error) {
		return nil, expected
	})

	if err != expected {
		t.Errorf("expected propagated error, got %v", err)
	}
	if !ran {
		t.Error("ran should still be true even when fn returns error (bypass mode)")
	}
}

// Nota: integração com pg_try_advisory_lock real precisa de Postgres
// containerizado (testcontainers / pgx) — fora do escopo desta suite.
// O guard de WithDB == nil mantém a função testável aqui sem infra,
// e os logs em main.go capturam o behavior em staging para validação
// manual final.
