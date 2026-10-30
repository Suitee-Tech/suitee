package journal

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNew_BalancedEntry(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	entry, err := New("tenant-1", "contract-1", "comissão de corretagem", now, []Leg{
		{Account: AccountCommissionExpense, Direction: Debit, AmountCents: 15000},
		{Account: AccountCommissionPayable, Direction: Credit, AmountCents: 15000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.Status != StatusPosted {
		t.Errorf("expected POSTED, got %s", entry.Status)
	}
	if entry.ID == "" {
		t.Error("expected generated ID")
	}
	if !entry.Balanced() {
		t.Error("expected balanced entry")
	}
}

func TestNew_NormalizesDateToPostgresPrecision(t *testing.T) {
	local := time.FixedZone("BRT", -3*60*60)
	input := time.Date(2026, 8, 30, 15, 42, 10, 123456789, local)
	entry, err := New("tenant-1", "contract-1", "precisão canônica", input, []Leg{
		{Account: AccountCash, Direction: Debit, AmountCents: 100},
		{Account: AccountRevenue, Direction: Credit, AmountCents: 100},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := input.UTC().Truncate(time.Microsecond)
	if !entry.Date.Equal(want) || entry.Date.Location() != time.UTC {
		t.Fatalf("journal date = %v, want UTC microsecond %v", entry.Date, want)
	}
}

func TestNew_RejectsUnbalanced(t *testing.T) {
	now := time.Now().UTC()
	_, err := New("tenant-1", "contract-1", "desequilibrado", now, []Leg{
		{Account: AccountCommissionExpense, Direction: Debit, AmountCents: 15000},
		{Account: AccountCommissionPayable, Direction: Credit, AmountCents: 14000},
	})
	if err == nil {
		t.Fatal("expected error for unbalanced entry")
	}
}

func TestNew_RejectsSingleLeg(t *testing.T) {
	now := time.Now().UTC()
	_, err := New("tenant-1", "contract-1", "uma perna só", now, []Leg{
		{Account: AccountCash, Direction: Debit, AmountCents: 100},
	})
	if err == nil {
		t.Fatal("expected error for single-leg entry")
	}
}

func TestNew_RejectsUnknownAccount(t *testing.T) {
	now := time.Now().UTC()
	_, err := New("tenant-1", "contract-1", "conta desconhecida", now, []Leg{
		{Account: Account("nao_existe"), Direction: Debit, AmountCents: 100},
		{Account: AccountCash, Direction: Credit, AmountCents: 100},
	})
	if err == nil {
		t.Fatal("expected error for unknown account")
	}
}

func TestNew_RejectsAccountOnBothSides(t *testing.T) {
	now := time.Now().UTC()
	_, err := New("tenant-1", "contract-1", "conta nos dois lados", now, []Leg{
		{Account: AccountCash, Direction: Debit, AmountCents: 100},
		{Account: AccountCash, Direction: Credit, AmountCents: 100},
	})
	if err == nil {
		t.Fatal("expected error for account on both sides")
	}
}

func TestNew_RejectsNonPositiveAmount(t *testing.T) {
	now := time.Now().UTC()
	_, err := New("tenant-1", "contract-1", "valor zerado", now, []Leg{
		{Account: AccountCash, Direction: Debit, AmountCents: 0},
		{Account: AccountRevenue, Direction: Credit, AmountCents: 0},
	})
	if err == nil {
		t.Fatal("expected error for non-positive amount")
	}
}

func TestReverse_ProducesBalancedMirror(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	original, err := New("tenant-1", "contract-1", "comissão", now, []Leg{
		{Account: AccountCommissionExpense, Direction: Debit, AmountCents: 15000},
		{Account: AccountCommissionPayable, Direction: Credit, AmountCents: 15000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	effectiveAt := time.Date(2026, 8, 11, 23, 59, 59, 123456000, time.UTC)
	reversed, err := original.ReverseAt("correção", "operator-1", effectiveAt)
	if err != nil {
		t.Fatalf("unexpected error reversing: %v", err)
	}
	if reversed.ReversalOf != original.ID {
		t.Errorf("expected ReversalOf %s, got %s", original.ID, reversed.ReversalOf)
	}
	if reversed.ReversedBy != "operator-1" || !reversed.Date.Equal(effectiveAt) {
		t.Errorf("expected canonical operator/date, got by=%q date=%v", reversed.ReversedBy, reversed.Date)
	}
	if !reversed.Balanced() {
		t.Error("reversal must be balanced")
	}
	// A perna de débito original vira crédito e vice-versa.
	if reversed.Legs[0].Account != AccountCommissionExpense || reversed.Legs[0].Direction != Credit {
		t.Errorf("expected mirrored debit leg, got %+v", reversed.Legs[0])
	}
	if reversed.Legs[1].Account != AccountCommissionPayable || reversed.Legs[1].Direction != Debit {
		t.Errorf("expected mirrored credit leg, got %+v", reversed.Legs[1])
	}
}

func TestReverse_RequiresAuditIdentity(t *testing.T) {
	original, err := New("tenant-1", "contract-1", "comissão", time.Now(), []Leg{
		{Account: AccountCommissionExpense, Direction: Debit, AmountCents: 100},
		{Account: AccountCommissionPayable, Direction: Credit, AmountCents: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ reason, operator string }{{"", "operator"}, {"reason", ""}} {
		if _, err := original.Reverse(tc.reason, tc.operator); !errors.Is(err, ErrInvalidEntry) {
			t.Fatalf("Reverse(%q, %q) = %v, want ErrInvalidEntry", tc.reason, tc.operator, err)
		}
	}
}

func TestReverse_PreservesManagedBillAndAllocationTrace(t *testing.T) {
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	original, err := New("tenant-1", "contract-1", "conta administrada", now, []Leg{
		{
			Account: AccountOwnerPayable, Direction: Debit, AmountCents: 70000,
			AllocationID: "allocation-1", ChargeItemID: "charge-item-1", ManagedBillID: "managed-bill-1", TransferID: "transfer-1",
			BeneficiaryPartyID: "owner-1", RuleVersionID: "rule-v1",
		},
		{
			Account: AccountCash, Direction: Credit, AmountCents: 70000,
			AllocationID: "allocation-1", ChargeItemID: "charge-item-1", ManagedBillID: "managed-bill-1", TransferID: "transfer-1",
			BeneficiaryPartyID: "owner-1", RuleVersionID: "rule-v1",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reversed, err := original.Reverse("bank rejection", "operator-1")
	if err != nil {
		t.Fatal(err)
	}
	for i, leg := range reversed.Legs {
		if leg.ManagedBillID != "managed-bill-1" || leg.AllocationID != "allocation-1" ||
			leg.ChargeItemID != "charge-item-1" || leg.TransferID != "transfer-1" ||
			leg.BeneficiaryPartyID != "owner-1" || leg.RuleVersionID != "rule-v1" {
			t.Fatalf("reversal leg %d lost economic trace: %+v", i, leg)
		}
	}
}

func TestReverse_And_MarkReversed_NetZero(t *testing.T) {
	now := time.Now().UTC()
	original, _ := New("tenant-1", "contract-1", "receita", now, []Leg{
		{Account: AccountAccountsReceivable, Direction: Debit, AmountCents: 250000},
		{Account: AccountRevenue, Direction: Credit, AmountCents: 250000},
	})
	reversed, _ := original.Reverse("estorno", "op")

	if err := original.MarkReversed(); err != nil {
		t.Fatalf("mark reversed: %v", err)
	}
	if original.Status != StatusReversed {
		t.Errorf("expected REVERSED, got %s", original.Status)
	}

	// O par original+reversão zera todas as contas.
	balance := AccountBalance([]*Entry{original, reversed})
	for account, b := range balance {
		if b != 0 {
			t.Errorf("account %s not zero after reversal: %d", account, b)
		}
	}
}

func TestMarkReversed_OnlyFromPosted(t *testing.T) {
	now := time.Now().UTC()
	entry, _ := New("tenant-1", "contract-1", "x", now, []Leg{
		{Account: AccountCash, Direction: Debit, AmountCents: 100},
		{Account: AccountRevenue, Direction: Credit, AmountCents: 100},
	})
	if err := entry.MarkReversed(); err != nil {
		t.Fatalf("first mark reversed: %v", err)
	}
	if err := entry.MarkReversed(); err == nil {
		t.Fatal("expected error reversing an already-reversed entry")
	}
}

func TestMarkReconciled_RecordsDateAndBlocksReversed(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	entry, _ := New("tenant-1", "contract-1", "receita", now, []Leg{
		{Account: AccountAccountsReceivable, Direction: Debit, AmountCents: 250000},
		{Account: AccountRevenue, Direction: Credit, AmountCents: 250000},
	})
	if entry.ReconciliationDate != nil {
		t.Fatalf("new entry should start unreconciled, got %v", entry.ReconciliationDate)
	}

	reconciledAt := time.Date(2026, 8, 12, 10, 0, 0, 987654321, time.FixedZone("BRT", -3*60*60))
	if err := entry.MarkReconciled(reconciledAt); err != nil {
		t.Fatalf("mark reconciled: %v", err)
	}
	wantReconciledAt := reconciledAt.UTC().Truncate(time.Microsecond)
	if entry.ReconciliationDate == nil || !entry.ReconciliationDate.Equal(wantReconciledAt) || entry.ReconciliationDate.Location() != time.UTC {
		t.Fatalf("reconciliation date = %v, want UTC microsecond %v", entry.ReconciliationDate, wantReconciledAt)
	}

	if err := entry.MarkReversed(); err != nil {
		t.Fatalf("mark reversed: %v", err)
	}
	if err := entry.MarkReconciled(time.Now()); err == nil {
		t.Fatal("expected error reconciling a reversed entry")
	}
}

func TestEntryJSONUsesSnakeCase(t *testing.T) {
	now := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	entry, err := New("tenant-1", "contract-1", "comissão", now, []Leg{
		{Account: AccountCommissionExpense, Direction: Debit, AmountCents: 15000},
		{Account: AccountCommissionPayable, Direction: Credit, AmountCents: 15000},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	entry.SourceEventID = "evt-1"
	entry.OperationID = "op-1"

	payload, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal entry JSON: %v", err)
	}
	want := map[string]bool{
		"id": true, "tenant_id": true, "contract_id": true, "operation_id": true,
		"source_event_id": true, "date": true, "description": true, "legs": true,
		"status": true, "created_at": true, "updated_at": true,
	}
	if len(got) != len(want) {
		t.Fatalf("entry JSON key count: got=%d want=%d keys=%v", len(got), len(want), got)
	}
	for key := range got {
		if !want[key] {
			t.Fatalf("unexpected entry JSON key %q", key)
		}
	}
}
