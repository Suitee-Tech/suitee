package transfer

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNew_seedsHistoryAndStatus(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-05", 3000, 8, 0, 0, nil, "")
	if tr.Status != StatusPending {
		t.Fatalf("expected pending, got %s", tr.Status)
	}
	if len(tr.History) != 1 || tr.History[0].Action != "created" {
		t.Fatalf("expected initial created history entry, got %+v", tr.History)
	}
	if tr.NetAmount == 0 {
		t.Fatal("expected NetAmount populated by Calculate()")
	}
}

func TestApprove_happyPath(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-05", 3000, 8, 0, 0, nil, "")
	if err := tr.Approve("user-42", "Ana Souza"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tr.Status != StatusApproved {
		t.Fatalf("expected approved, got %s", tr.Status)
	}
	if tr.ApprovedBy != "user-42" {
		t.Fatalf("expected actor user-42, got %q", tr.ApprovedBy)
	}
	if tr.ApprovedAt == nil {
		t.Fatal("expected ApprovedAt timestamp")
	}
	if len(tr.History) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(tr.History))
	}
	last := tr.History[1]
	if last.Action != "approved" || last.Actor != "user-42" || last.ActorLabel != "Ana Souza" {
		t.Fatalf("expected approved history entry with snapshot label, got %+v", last)
	}
}

func TestApprove_rejectsNonPending(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-05", 3000, 8, 0, 0, nil, "")
	if err := tr.Approve("user-42", ""); err != nil {
		t.Fatal(err)
	}
	if err := tr.Approve("user-42", ""); !errors.Is(err, ErrCannotApprove) {
		t.Fatalf("expected ErrCannotApprove, got %v", err)
	}
}

func TestProcess_requiresApproved(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-05", 3000, 8, 0, 0, nil, "")
	if err := tr.Process("user-42", "Ana"); !errors.Is(err, ErrCannotProcess) {
		t.Fatalf("expected ErrCannotProcess from pending, got %v", err)
	}
	if err := tr.Approve("user-42", "Ana"); err != nil {
		t.Fatal(err)
	}
	if err := tr.Process("user-99", "Carlos"); err != nil {
		t.Fatalf("unexpected error processing: %v", err)
	}
	if tr.Status != StatusProcessed || tr.ProcessedBy != "user-99" || tr.ProcessedAt == nil {
		t.Fatalf("processed state not applied: status=%s by=%s at=%v", tr.Status, tr.ProcessedBy, tr.ProcessedAt)
	}
	if len(tr.History) != 3 {
		t.Fatalf("expected 3 history entries (created+approved+processed), got %d", len(tr.History))
	}
	want := []string{"created", "approved", "processed"}
	for i, w := range want {
		if tr.History[i].Action != w {
			t.Errorf("history[%d]: want %q got %q", i, w, tr.History[i].Action)
		}
	}
	if tr.History[1].ActorLabel != "Ana" || tr.History[2].ActorLabel != "Carlos" {
		t.Errorf("expected actor labels stamped, got approved=%q processed=%q", tr.History[1].ActorLabel, tr.History[2].ActorLabel)
	}
}

func TestSettleIfNoPayableBalance_closesZeroWithoutApprovalOrCash(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 3200, 100, 0, 0, nil, "")
	settledAt := tr.CreatedAt.Add(time.Minute)

	if !tr.SettleIfNoPayableBalance(settledAt) {
		t.Fatal("expected a zero transfer to settle automatically")
	}
	if tr.Status != StatusProcessed || tr.ProcessedAt == nil || !tr.ProcessedAt.Equal(settledAt) || tr.ProcessedBy != "system" {
		t.Fatalf("automatic settlement not recorded: %+v", tr)
	}
	if tr.ApprovedAt != nil || tr.ApprovedBy != "" {
		t.Fatalf("zero settlement must not invent an approval: %+v", tr)
	}
	last := tr.History[len(tr.History)-1]
	if last.Action != "processed" || last.Actor != "system" || last.Note == "" {
		t.Fatalf("expected auditable automatic settlement history, got %+v", last)
	}
	if tr.SettleIfNoPayableBalance(settledAt.Add(time.Minute)) {
		t.Fatal("automatic settlement must be idempotent")
	}
}

func TestSettleIfNoPayableBalance_keepsReservedObligationOpen(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 1000, 0, 0, 0, nil, "")
	balance, err := NewOwnerBalance(100000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 0); err != nil {
		t.Fatal(err)
	}
	if tr.NetAmount != 0 {
		t.Fatalf("fixture must have zero available balance, got %.2f", tr.NetAmount)
	}
	if tr.SettleIfNoPayableBalance(time.Now()) {
		t.Fatal("reserved obligation must stay open until its managed bills settle")
	}
	if tr.Status != StatusPending {
		t.Fatalf("reserved obligation changed status to %s", tr.Status)
	}
}

func TestOwnerBalance_AvailableIsDerived(t *testing.T) {
	b, err := NewOwnerBalance(5000, 1000, 500)
	if err != nil {
		t.Fatalf("NewOwnerBalance: %v", err)
	}
	if got := b.AvailableCents(); got != 3500 {
		t.Fatalf("available = %d, want 3500", got)
	}
}

func TestOwnerBalance_RejectsExceedingBuckets(t *testing.T) {
	if _, err := NewOwnerBalance(5000, 4000, 2000); err == nil {
		t.Fatal("expected error when reserved+blocked exceed total")
	}
	if _, err := NewOwnerBalance(-1, 0, 0); err == nil {
		t.Fatal("expected error for negative total")
	}
}

func TestOwnerTransfer_AllocatedBalanceDrivesAvailableNet(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-08", 5000, 8, 0, 0, nil, "")
	balance, err := NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatalf("NewOwnerBalance: %v", err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatalf("SetAllocatedOwnerBalance: %v", err)
	}
	if tr.AdminFeeAmount != 400 || tr.NetAmount != 4600 {
		t.Fatalf("allocated values: fee=%.2f net=%.2f", tr.AdminFeeAmount, tr.NetAmount)
	}
	if tr.OwnerBalance == nil || tr.OwnerBalance.TotalCents != 560000 || tr.OwnerBalance.ReservedCents != 100000 || tr.OwnerBalance.AvailableCents() != 460000 {
		t.Fatalf("owner balance not preserved: %+v", tr.OwnerBalance)
	}

	tr.OtherDeductions = 100
	tr.Calculate()
	if tr.OwnerBalance.BlockedCents != 10000 || tr.OwnerBalance.ReservedCents != 100000 || tr.OwnerBalance.AvailableCents() != 450000 || tr.NetAmount != 4500 {
		t.Fatalf("deduction did not block only available owner balance: transfer=%+v balance=%+v", tr, tr.OwnerBalance)
	}
	if tr.AdminFeeAmount != 400 {
		t.Fatalf("allocation-derived admin fee was recomputed: %.2f", tr.AdminFeeAmount)
	}
}

func TestOwnerTransfer_ManagedBillMovementsConsumeAndReleaseWithoutChangingAvailable(t *testing.T) {
	tr := New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08", 5000, 8, 0, 0, nil, "")
	tr.ID = "transfer-1"
	balance, err := NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}

	paidAt := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	consume := OwnerBalanceMovement{
		ID: "managed-bill.payment:condo", ManagedBillID: "bill-condo",
		Type: OwnerBalanceMovementConsumeReserved, AmountCents: 70000, Actor: "payer-1", At: paidAt,
	}
	changed, err := tr.ApplyOwnerBalanceMovement(consume)
	if err != nil || !changed {
		t.Fatalf("consume reserve: changed=%v err=%v", changed, err)
	}
	if got := *tr.OwnerBalance; got.TotalCents != 490000 || got.ReservedCents != 30000 || got.AvailableCents() != 460000 {
		t.Fatalf("after R$700 payment = %+v, want total=490000 reserved=30000 available=460000", got)
	}
	if tr.NetAmount != 4600 {
		t.Fatalf("net after managed-bill payment = %.2f, want 4600.00", tr.NetAmount)
	}

	changed, err = tr.ApplyOwnerBalanceMovement(consume)
	if err != nil || changed || len(tr.OwnerBalanceMovements) != 1 {
		t.Fatalf("same movement retry: changed=%v movements=%d err=%v", changed, len(tr.OwnerBalanceMovements), err)
	}
	divergentActor := consume
	divergentActor.Actor = "payer-2"
	if changed, err = tr.ApplyOwnerBalanceMovement(divergentActor); err == nil || changed {
		t.Fatalf("same movement id with divergent actor must fail: changed=%v err=%v", changed, err)
	}
	divergentAt := consume
	divergentAt.At = consume.At.Add(time.Second)
	if changed, err = tr.ApplyOwnerBalanceMovement(divergentAt); err == nil || changed {
		t.Fatalf("same movement id with divergent timestamp must fail: changed=%v err=%v", changed, err)
	}
	divergentID := consume
	divergentID.ID = "managed-bill.payment:condo:second-command"
	if changed, err = tr.ApplyOwnerBalanceMovement(divergentID); err == nil || changed {
		t.Fatalf("same bill with another movement id must fail: changed=%v err=%v", changed, err)
	}

	release := OwnerBalanceMovement{
		ID: "managed-bill.cancel:iptu", ManagedBillID: "bill-iptu",
		Type: OwnerBalanceMovementReleaseReserved, AmountCents: 30000, Actor: "operator-1", At: paidAt.Add(time.Hour),
	}
	changed, err = tr.ApplyOwnerBalanceMovement(release)
	if err != nil || !changed {
		t.Fatalf("release reserve: changed=%v err=%v", changed, err)
	}
	if got := *tr.OwnerBalance; got.TotalCents != 490000 || got.ReservedCents != 0 || got.AvailableCents() != 490000 {
		t.Fatalf("after R$300 cancellation = %+v, want total=490000 reserved=0 available=490000", got)
	}
	if tr.OwnerBalanceBaseTotalCents != 560000 || tr.OwnerBalanceBaseReservedCents != 100000 || len(tr.OwnerBalanceMovements) != 2 {
		t.Fatalf("immutable basis/movements lost: base=%d/%d movements=%+v", tr.OwnerBalanceBaseTotalCents, tr.OwnerBalanceBaseReservedCents, tr.OwnerBalanceMovements)
	}
}

func TestOwnerTransfer_MovementsSurviveEconomicCompositionRetryAndRejectChangedBase(t *testing.T) {
	tr := New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08", 5000, 8, 0, 0, nil, "")
	balance, err := NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "payment-1", ManagedBillID: "bill-1", Type: OwnerBalanceMovementConsumeReserved,
		AmountCents: 70000, At: time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}

	if err := tr.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatalf("same composition retry: %v", err)
	}
	if got := tr.OwnerBalance; got.TotalCents != 490000 || got.ReservedCents != 30000 || got.AvailableCents() != 460000 {
		t.Fatalf("retry resurrected reserve: %+v", got)
	}

	changedBase, err := NewOwnerBalance(570000, 110000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(changedBase, 40000); !errors.Is(err, ErrOwnerBalanceBaseChanged) {
		t.Fatalf("changed economic base: got %v, want ErrOwnerBalanceBaseChanged", err)
	}
	if got := tr.OwnerBalance; got.TotalCents != 490000 || got.ReservedCents != 30000 {
		t.Fatalf("failed base change mutated current balance: %+v", got)
	}
}

func TestOwnerTransfer_FirstMovementFreezesLegacyAllocationAwareBasis(t *testing.T) {
	tr := New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08", 5000, 8, 0, 0, nil, "")
	// Simulates JSON written before the explicit base and movement fields were
	// introduced: owner_balance exists, but base fields are absent/zero.
	legacyBalance, err := NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	tr.OwnerBalance = &legacyBalance
	tr.OwnerBalanceBaseTotalCents = 0
	tr.OwnerBalanceBaseReservedCents = 0
	changed, err := tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "payment-legacy", ManagedBillID: "bill-legacy", Type: OwnerBalanceMovementConsumeReserved,
		AmountCents: 70000, At: time.Date(2026, time.August, 12, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || !changed {
		t.Fatalf("first movement on rolling-deployment row: changed=%v err=%v", changed, err)
	}
	if tr.OwnerBalanceBaseTotalCents != 560000 || tr.OwnerBalanceBaseReservedCents != 100000 ||
		tr.OwnerBalance.TotalCents != 490000 || tr.OwnerBalance.ReservedCents != 30000 {
		t.Fatalf("legacy basis was not frozen/replayed: %+v", tr)
	}
	if len(tr.OwnerBalanceMovements) != 1 || tr.OwnerBalanceMovements[0].PaymentAttemptID != "payment-legacy" {
		t.Fatalf("legacy consume did not infer payment attempt identity: %+v", tr.OwnerBalanceMovements)
	}
	changed, err = tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "restore-legacy", ManagedBillID: "bill-legacy", PaymentAttemptID: "payment-legacy",
		Type: OwnerBalanceMovementRestoreReserved, AmountCents: 70000,
		At: time.Date(2026, time.August, 13, 0, 0, 0, 0, time.UTC),
	})
	if err != nil || !changed || tr.OwnerBalance.TotalCents != 560000 || tr.OwnerBalance.ReservedCents != 100000 {
		t.Fatalf("restore of inferred legacy attempt: changed=%v err=%v balance=%+v", changed, err, tr.OwnerBalance)
	}
}

func TestOwnerTransfer_PaymentAttemptConsumeRestoreRetryAndCancel(t *testing.T) {
	tr := New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08", 5000, 8, 0, 0, nil, "")
	balance, err := NewOwnerBalance(560000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 40000); err != nil {
		t.Fatal(err)
	}
	baseAt := time.Date(2026, time.August, 12, 12, 0, 0, 0, time.UTC)
	consume1 := OwnerBalanceMovement{
		ID: "consume-1", ManagedBillID: "bill-condo", PaymentAttemptID: "payment-attempt-1",
		Type: OwnerBalanceMovementConsumeReserved, AmountCents: 70000, Actor: "payer-1", At: baseAt,
	}
	restore1 := OwnerBalanceMovement{
		ID: "restore-1", ManagedBillID: "bill-condo", PaymentAttemptID: "payment-attempt-1",
		Type: OwnerBalanceMovementRestoreReserved, AmountCents: 70000, Actor: "reverser-1", At: baseAt.Add(time.Hour),
	}
	consume2 := OwnerBalanceMovement{
		ID: "consume-2", ManagedBillID: "bill-condo", PaymentAttemptID: "payment-attempt-2",
		Type: OwnerBalanceMovementConsumeReserved, AmountCents: 70000, Actor: "payer-2", At: baseAt.Add(2 * time.Hour),
	}
	restore2 := OwnerBalanceMovement{
		ID: "restore-2", ManagedBillID: "bill-condo", PaymentAttemptID: "payment-attempt-2",
		Type: OwnerBalanceMovementRestoreReserved, AmountCents: 70000, Actor: "reverser-2", At: baseAt.Add(3 * time.Hour),
	}
	release := OwnerBalanceMovement{
		ID: "release-after-reverse", ManagedBillID: "bill-condo",
		Type: OwnerBalanceMovementReleaseReserved, AmountCents: 70000, Actor: "operator-1", At: baseAt.Add(4 * time.Hour),
	}

	for _, movement := range []OwnerBalanceMovement{consume1, restore1, consume2, restore2, release} {
		changed, err := tr.ApplyOwnerBalanceMovement(movement)
		if err != nil || !changed {
			t.Fatalf("apply %s/%s: changed=%v err=%v", movement.Type, movement.PaymentAttemptID, changed, err)
		}
		switch movement.Type {
		case OwnerBalanceMovementConsumeReserved:
			if got := *tr.OwnerBalance; got.TotalCents != 490000 || got.ReservedCents != 30000 || got.AvailableCents() != 460000 {
				t.Fatalf("consume balance = %+v", got)
			}
		case OwnerBalanceMovementRestoreReserved:
			if got := *tr.OwnerBalance; got.TotalCents != 560000 || got.ReservedCents != 100000 || got.AvailableCents() != 460000 {
				t.Fatalf("restore balance = %+v", got)
			}
		}
	}
	if got := *tr.OwnerBalance; got.TotalCents != 560000 || got.ReservedCents != 30000 || got.AvailableCents() != 530000 {
		t.Fatalf("cancel after reversal balance = %+v, want total=560000 reserved=30000 available=530000", got)
	}
	if len(tr.OwnerBalanceMovements) != 5 {
		t.Fatalf("movements = %d, want 5", len(tr.OwnerBalanceMovements))
	}

	for _, retry := range []OwnerBalanceMovement{consume1, restore1, release} {
		changed, err := tr.ApplyOwnerBalanceMovement(retry)
		if err != nil || changed || len(tr.OwnerBalanceMovements) != 5 {
			t.Fatalf("retry %s: changed=%v movements=%d err=%v", retry.ID, changed, len(tr.OwnerBalanceMovements), err)
		}
	}
}

func TestOwnerTransfer_PaymentAttemptMovementSequencesFailClosed(t *testing.T) {
	newTransfer := func(t *testing.T) *OwnerTransfer {
		t.Helper()
		tr := New("contract-1", "owner-1", "property-1", "tenant-1", "2026-08", 5000, 8, 0, 0, nil, "")
		balance, err := NewOwnerBalance(560000, 100000, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := tr.SetAllocatedOwnerBalance(balance, 40000); err != nil {
			t.Fatal(err)
		}
		return tr
	}
	consume := func(id, attempt string, amount int64) OwnerBalanceMovement {
		return OwnerBalanceMovement{
			ID: id, ManagedBillID: "bill-condo", PaymentAttemptID: attempt,
			Type: OwnerBalanceMovementConsumeReserved, AmountCents: amount,
		}
	}
	restore := func(id, attempt string, amount int64) OwnerBalanceMovement {
		return OwnerBalanceMovement{
			ID: id, ManagedBillID: "bill-condo", PaymentAttemptID: attempt,
			Type: OwnerBalanceMovementRestoreReserved, AmountCents: amount,
		}
	}
	release := func(id string, amount int64) OwnerBalanceMovement {
		return OwnerBalanceMovement{
			ID: id, ManagedBillID: "bill-condo", Type: OwnerBalanceMovementReleaseReserved, AmountCents: amount,
		}
	}
	applyOK := func(t *testing.T, tr *OwnerTransfer, movement OwnerBalanceMovement) {
		t.Helper()
		changed, err := tr.ApplyOwnerBalanceMovement(movement)
		if err != nil || !changed {
			t.Fatalf("setup movement %s: changed=%v err=%v", movement.ID, changed, err)
		}
	}
	assertRejectedWithoutMutation := func(t *testing.T, tr *OwnerTransfer, movement OwnerBalanceMovement) {
		t.Helper()
		beforeLen := len(tr.OwnerBalanceMovements)
		before := *tr.OwnerBalance
		changed, err := tr.ApplyOwnerBalanceMovement(movement)
		if err == nil || changed {
			t.Fatalf("movement should fail: %+v changed=%v err=%v", movement, changed, err)
		}
		if len(tr.OwnerBalanceMovements) != beforeLen || *tr.OwnerBalance != before {
			t.Fatalf("rejected movement mutated transfer: before=%+v after=%+v movements=%d/%d", before, *tr.OwnerBalance, beforeLen, len(tr.OwnerBalanceMovements))
		}
	}

	t.Run("restore requires attempt id", func(t *testing.T) {
		assertRejectedWithoutMutation(t, newTransfer(t), restore("restore", "", 70000))
	})
	t.Run("release cannot reference attempt", func(t *testing.T) {
		movement := release("release", 70000)
		movement.PaymentAttemptID = "attempt-1"
		assertRejectedWithoutMutation(t, newTransfer(t), movement)
	})
	t.Run("restore requires matching consume", func(t *testing.T) {
		assertRejectedWithoutMutation(t, newTransfer(t), restore("restore", "attempt-1", 70000))
	})
	t.Run("only one consumed attempt can be active", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, consume("consume-1", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, consume("consume-2", "attempt-2", 70000))
	})
	t.Run("restore must target active attempt", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, consume("consume-1", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, restore("restore-2", "attempt-2", 70000))
	})
	t.Run("cannot release active consumption", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, consume("consume-1", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, release("release", 70000))
	})
	t.Run("attempt cannot be consumed twice", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, consume("consume-1", "attempt-1", 70000))
		applyOK(t, tr, restore("restore-1", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, consume("consume-duplicate", "attempt-1", 70000))
	})
	t.Run("amount is stable between attempts", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, consume("consume-1", "attempt-1", 70000))
		applyOK(t, tr, restore("restore-1", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, consume("consume-2", "attempt-2", 60000))
	})
	t.Run("release is terminal", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, release("release", 70000))
		assertRejectedWithoutMutation(t, tr, consume("consume-1", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, release("release-2", 70000))
	})
	t.Run("movement id cannot diverge", func(t *testing.T) {
		tr := newTransfer(t)
		applyOK(t, tr, consume("same-id", "attempt-1", 70000))
		assertRejectedWithoutMutation(t, tr, OwnerBalanceMovement{
			ID: "same-id", ManagedBillID: "another-bill", PaymentAttemptID: "another-attempt",
			Type: OwnerBalanceMovementConsumeReserved, AmountCents: 30000,
		})
	})
}

func TestOwnerBalanceMarshalJSONExposesAvailableCents(t *testing.T) {
	b, err := NewOwnerBalance(106220000, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["total_cents"] != 106220000 || got["reserved_cents"] != 0 || got["blocked_cents"] != 0 {
		t.Fatalf("buckets drifted in JSON: %s", data)
	}
	if got["available_cents"] != 106220000 {
		t.Fatalf("available_cents must be exposed (derived), got %s", data)
	}

	// Reservado+bloqueado reduz o disponível; never negative.
	reserved, _ := NewOwnerBalance(50000, 30000, 10000)
	data, _ = json.Marshal(reserved)
	_ = json.Unmarshal(data, &got)
	if got["available_cents"] != 10000 {
		t.Fatalf("available_cents must respect reserved+blocked, got %s", data)
	}
}
