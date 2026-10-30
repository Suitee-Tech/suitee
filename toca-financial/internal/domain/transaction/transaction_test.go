package transaction

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestMarkPaid_OnlyPendingOrOverdue(t *testing.T) {
	paidAt := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		status  Status
		wantErr bool
	}{
		{name: "pending", status: StatusPending},
		{name: "overdue", status: StatusOverdue},
		{name: "legacy pending", status: StatusPendente},
		{name: "legacy overdue", status: StatusEmAtraso},
		{name: "paid", status: StatusPaid, wantErr: true},
		{name: "reversed", status: StatusReversed, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &Transaction{Status: tc.status}
			err := tx.MarkPaid(paidAt, "PIX")
			if tc.wantErr {
				if !errors.Is(err, ErrInvalidStatusTransition) {
					t.Fatalf("expected invalid transition, got %v", err)
				}
				if tx.Status != tc.status {
					t.Fatalf("invalid transition changed status to %s", tx.Status)
				}
				return
			}
			if err != nil {
				t.Fatalf("mark paid: %v", err)
			}
			if tx.Status != StatusPaid {
				t.Fatalf("expected canonical PAID, got %s", tx.Status)
			}
		})
	}
}

func TestMarkReversed_IsTerminal(t *testing.T) {
	tx := &Transaction{Status: StatusPaid, Type: TypeReceita}
	if err := tx.MarkReversed(); err != nil {
		t.Fatalf("mark reversed: %v", err)
	}
	if tx.Status != StatusReversed {
		t.Fatalf("expected REVERSED, got %s", tx.Status)
	}
	if err := tx.MarkReversed(); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("second reversal must fail, got %v", err)
	}
	if err := tx.MarkPaid(time.Now(), "PIX"); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("settling a reversal must fail, got %v", err)
	}
	if err := ValidateStatusTransition(StatusReversed, StatusPending); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("generic update must not reopen a reversal, got %v", err)
	}
}

func TestReverse_RejectsNonPaidAndEstornoTransactions(t *testing.T) {
	pending := &Transaction{Status: StatusPending, Type: TypeReceita}
	if _, err := pending.Reverse("reason", "original"); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("pending transaction must not create a reversal, got %v", err)
	}
	estorno := &Transaction{Status: StatusPaid, Type: TypeEstorno}
	if _, err := estorno.Reverse("reason", "original"); !errors.Is(err, ErrInvalidStatusTransition) {
		t.Fatalf("estorno transaction must not be reversed, got %v", err)
	}
}

func TestValidateUsesTypeForDirectionAndNegativeOnlyForReversal(t *testing.T) {
	base := Transaction{
		Description: "posting",
		Status:      StatusPending,
		TenantID:    "tenant-1",
	}
	for _, txType := range []Type{TypeReceita, TypeDespesa, TypeRepasse, TypeComissao} {
		tx := base
		tx.Type = txType
		tx.Value = -10
		if err := tx.Validate(); !errors.Is(err, ErrInvalidTransactionValue) {
			t.Fatalf("type %s accepted negative normal value: %v", txType, err)
		}
		tx.Value = 10
		if err := tx.Validate(); err != nil {
			t.Fatalf("type %s rejected positive magnitude: %v", txType, err)
		}
	}

	reversal := base
	reversal.Type = TypeEstorno
	reversal.Status = StatusPaid
	reversal.Value = 10
	if err := reversal.Validate(); !errors.Is(err, ErrInvalidTransactionValue) {
		t.Fatalf("reversal accepted non-negative value: %v", err)
	}
	reversal.Value = -10
	if err := reversal.Validate(); err != nil {
		t.Fatalf("reversal rejected negative counter-posting: %v", err)
	}
}

func TestValidateRejectsOversizedPaymentPlan(t *testing.T) {
	tx := Transaction{
		Type:        TypeReceita,
		Status:      StatusPending,
		Value:       100,
		TenantID:    "tenant-1",
		Description: "posting",
		PaymentPlan: strings.Repeat("x", 513),
	}
	if err := tx.Validate(); err == nil || !strings.Contains(err.Error(), "payment_plan") {
		t.Fatalf("oversized payment plan must be rejected, got %v", err)
	}

	tx.PaymentPlan = "Entrada + 11x de R$ 8.333,33"
	if err := tx.Validate(); err != nil {
		t.Fatalf("legitimate payment plan must be accepted: %v", err)
	}
}

// O lançamento contábil da comissão acompanha a reabertura: se a comissão volta
// a PENDING, o lançamento cancelado precisa voltar a PENDING também, senão o
// razão fica com uma obrigação viva sem lastro.
func TestReopen_OnlyCancelledTransactionComesBack(t *testing.T) {
	cancelled := &Transaction{ID: "t1", Status: StatusCancelled}
	if err := cancelled.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if CanonicalStatus(cancelled.Status) != StatusPending {
		t.Errorf("status = %s, want PENDING", cancelled.Status)
	}
	if err := cancelled.Reopen(); err != nil {
		t.Errorf("Reopen idempotente devolveu erro: %v", err)
	}
	paid := &Transaction{ID: "t2", Status: StatusPaid}
	if err := paid.Reopen(); err == nil {
		t.Error("lançamento PAGO foi reaberto — estorno é o caminho")
	}
}
