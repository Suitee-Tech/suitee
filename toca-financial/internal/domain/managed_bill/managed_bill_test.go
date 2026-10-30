package managed_bill

import (
	"github.com/oginoino/toca/pkg/timeutil"

	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validReservedInput() NewReservedInput {
	return NewReservedInput{
		TenantID:           "tenant-1",
		SourceKey:          "managed-bill:charge-item-1",
		SourceTransferID:   "transfer-1",
		SourceChargeID:     "charge-1",
		SourceChargeItemID: "charge-item-1",
		ContractID:         "contract-1",
		PropertyID:         "property-1",
		OwnerID:            "owner-1",
		Component:          "condominio",
		Creditor:           "Condominio Edificio Toca",
		Description:        "Condominio agosto/2026",
		AmountCents:        70000,
		DueDate:            time.Date(2026, time.August, 15, 18, 30, 0, 0, time.FixedZone("BRT", -3*60*60)),
		CompetenceMonth:    "2026-08",
		ReservedBy:         "system:charge-paid",
		ReservedAt:         time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC),
	}
}

func TestNewReserved_NormalizesDateAndPublicJSON(t *testing.T) {
	bill, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatalf("NewReserved: %v", err)
	}
	// O vencimento é normalizado para a meia-noite do DIA CIVIL no fuso
	// configurado (Brasília por padrão). O valor de entrada é 15/08 18:30 BRT:
	// converter para UTC ANTES de ler os componentes (comportamento antigo) dava
	// 21:30 UTC do dia 15 — mas para um valor às 21h BRT o vencimento ia para o
	// dia 16. A asserção é sobre o dia civil, não sobre a parede de relógio UTC.
	if got := timeutil.DateOnly(bill.DueDate); got != "2026-08-15" {
		t.Fatalf("DueDate = %v (dia civil %s), want 2026-08-15", bill.DueDate, got)
	}
	if local := bill.DueDate.In(timeutil.Location()); local.Hour() != 0 || local.Minute() != 0 {
		t.Fatalf("DueDate = %v, want meia-noite do fuso configurado", bill.DueDate)
	}
	if bill.Status != StatusReserved || bill.Version != 1 || len(bill.History) != 1 {
		t.Fatalf("unexpected initial lifecycle: status=%s version=%d history=%+v", bill.Status, bill.Version, bill.History)
	}

	payload, err := json.Marshal(bill)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := fields["charge_item_id"]; !ok {
		t.Fatal("public JSON must expose charge_item_id")
	}
	if _, ok := fields["creditor_name"]; !ok {
		t.Fatal("public JSON must expose creditor_name")
	}
	if _, stale := fields["source_charge_item_id"]; stale {
		t.Fatal("public JSON must not expose stale source_charge_item_id")
	}
	if _, stale := fields["creditor"]; stale {
		t.Fatal("public JSON must not expose stale creditor")
	}
}

func TestNewReserved_TruncatesAuditTimeToPostgresPrecision(t *testing.T) {
	input := validReservedInput()
	input.ReservedAt = time.Date(2026, time.August, 10, 12, 0, 0, 123_456_789, time.UTC)

	bill, err := NewReserved(input)
	if err != nil {
		t.Fatalf("NewReserved: %v", err)
	}
	want := time.Date(2026, time.August, 10, 12, 0, 0, 123_456_000, time.UTC)
	if !bill.ReservedAt.Equal(want) || !bill.CreatedAt.Equal(want) ||
		!bill.UpdatedAt.Equal(want) || len(bill.History) != 1 || !bill.History[0].At.Equal(want) {
		t.Fatalf("audit timestamps were not normalized to microseconds: %+v", bill)
	}
}

func TestNewReserved_RequiresCreditorAndValidCompetence(t *testing.T) {
	in := validReservedInput()
	in.Creditor = " "
	if _, err := NewReserved(in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank creditor: got %v, want ErrInvalidInput", err)
	}
	in = validReservedInput()
	in.CompetenceMonth = "2026-13"
	if _, err := NewReserved(in); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("invalid competence: got %v, want ErrInvalidInput", err)
	}
}

func TestManagedBill_LifecycleRequiresReferencesAndCannotCancelPaid(t *testing.T) {
	bill, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	approvedAt := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	if err := bill.Approve("approver-1", approvedAt); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if err := bill.MarkPaid("payer-1", "pix-too-early", "journal-source-early", approvedAt.Add(-time.Second)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("payment before approval: got %v, want ErrInvalidInput", err)
	}
	if bill.Status != StatusApproved || bill.Version != 2 {
		t.Fatalf("rejected timestamp mutated lifecycle: status=%s version=%d", bill.Status, bill.Version)
	}
	if err := bill.MarkPaid("payer-1", "", "journal-source-1", approvedAt.Add(time.Hour)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank payment reference: got %v, want ErrInvalidInput", err)
	}
	paidAt := approvedAt.Add(time.Hour)
	if err := bill.MarkPaid("payer-1", "pix-e2e-1", "journal-source-1", paidAt); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}
	if bill.Status != StatusPaid || bill.Version != 3 || bill.PaymentReference != "pix-e2e-1" {
		t.Fatalf("unexpected paid state: %+v", bill)
	}
	if err := bill.Cancel("operator-1", "duplicated bill", paidAt.Add(time.Hour)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("paid bill cancellation: got %v, want ErrInvalidTransition", err)
	}
	if err := bill.Reconcile("operator-1", "", paidAt.Add(time.Hour)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank reconciliation reference: got %v, want ErrInvalidInput", err)
	}
	if err := bill.Reconcile("operator-1", "bank-statement-1", paidAt.Add(time.Hour)); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if bill.Status != StatusReconciled || bill.Version != 4 || len(bill.History) != 4 {
		t.Fatalf("unexpected reconciled lifecycle: status=%s version=%d history=%+v", bill.Status, bill.Version, bill.History)
	}
}

func TestManagedBill_CancelRequiresReason(t *testing.T) {
	bill, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := bill.Cancel("operator-1", "", time.Time{}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank reason: got %v, want ErrInvalidInput", err)
	}
	if err := bill.Cancel("operator-1", "owner will pay directly", time.Time{}); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if bill.Status != StatusCancelled || bill.CancellationReason != "owner will pay directly" || bill.Version != 2 {
		t.Fatalf("unexpected cancelled state: %+v", bill)
	}
}

func TestManagedBill_ReversePaidPaymentSnapshotsAndReopensForRetry(t *testing.T) {
	bill, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	approvedAt := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	paidAt := approvedAt.Add(time.Hour)
	reversedAt := paidAt.Add(2 * time.Hour)
	if err := bill.Approve("approver-1", approvedAt); err != nil {
		t.Fatal(err)
	}
	if err := bill.MarkPaid("payer-1", "pix-payment-1", "journal-payment-1", paidAt,
		Evidence{Kind: "payment_proof", Reference: "proof://payment-1"}); err != nil {
		t.Fatal(err)
	}

	if err := bill.ReversePayment(
		"reverser-1", "refund-1", "journal-reversal-1", "payment rejected",
		reversedAt, Evidence{Kind: "payment_reversal_proof", Reference: "proof://refund-1"},
	); err != nil {
		t.Fatalf("ReversePayment: %v", err)
	}
	if bill.Status != StatusApproved || bill.Version != 4 {
		t.Fatalf("reversed payment must reopen approved revision 4: status=%s version=%d", bill.Status, bill.Version)
	}
	if bill.ApprovedBy != "approver-1" || bill.ApprovedAt == nil || !bill.ApprovedAt.Equal(approvedAt) {
		t.Fatalf("approval milestone was not preserved: by=%q at=%v", bill.ApprovedBy, bill.ApprovedAt)
	}
	if bill.PaidBy != "" || bill.PaidAt != nil || bill.PaymentReference != "" || bill.PaymentJournalSourceID != "" ||
		bill.ReconciledBy != "" || bill.ReconciledAt != nil || bill.ReconciliationReference != "" {
		t.Fatalf("active payment/reconciliation fields were not cleared: %+v", bill)
	}
	if len(bill.PaymentAttempts) != 1 {
		t.Fatalf("payment attempts = %d, want 1", len(bill.PaymentAttempts))
	}
	attempt := bill.PaymentAttempts[0]
	if attempt.ID != "journal-payment-1" || attempt.PaymentReference != "pix-payment-1" ||
		attempt.PaymentJournalSourceID != "journal-payment-1" || attempt.PaidBy != "payer-1" || !attempt.PaidAt.Equal(paidAt) ||
		attempt.ReconciledAt != nil || attempt.ReversalReference != "refund-1" ||
		attempt.ReversalJournalSourceID != "journal-reversal-1" || attempt.ReversalProofReference != "proof://refund-1" || attempt.ReversedBy != "reverser-1" ||
		!attempt.ReversedAt.Equal(reversedAt) || attempt.ReversalReason != "payment rejected" {
		t.Fatalf("unexpected payment attempt snapshot: %+v", attempt)
	}
	last, ok := bill.LastPaymentAttempt()
	if !ok || last.ID != attempt.ID {
		t.Fatalf("LastPaymentAttempt = (%+v, %v)", last, ok)
	}
	last.ID = "mutated-copy"
	if bill.PaymentAttempts[0].ID != "journal-payment-1" {
		t.Fatal("LastPaymentAttempt exposed a mutable slice element")
	}
	if got := bill.History[len(bill.History)-1]; got.Action != "payment_reversed" || got.Actor != "reverser-1" ||
		got.Reference != "refund-1" || got.Reason != "payment rejected" || got.Version != 4 {
		t.Fatalf("unexpected reversal history: %+v", got)
	}
	if len(bill.Evidence) != 2 || bill.Evidence[1].Actor != "reverser-1" || !bill.Evidence[1].At.Equal(reversedAt) {
		t.Fatalf("reversal evidence was not appended canonically: %+v", bill.Evidence)
	}

	version, historyLen, evidenceLen := bill.Version, len(bill.History), len(bill.Evidence)
	if err := bill.ReversePayment(
		"reverser-1", "refund-1", "journal-reversal-1", "payment rejected",
		reversedAt, Evidence{Kind: "payment_reversal_proof", Reference: "proof://refund-1"},
	); err != nil {
		t.Fatalf("canonical reversal retry: %v", err)
	}
	if bill.Version != version || len(bill.History) != historyLen || len(bill.Evidence) != evidenceLen || len(bill.PaymentAttempts) != 1 {
		t.Fatalf("reversal retry mutated aggregate: version=%d history=%d evidence=%d attempts=%d", bill.Version, len(bill.History), len(bill.Evidence), len(bill.PaymentAttempts))
	}
	if err := bill.ReversePayment(
		"reverser-1", "refund-1", "journal-reversal-1", "payment rejected",
		reversedAt.Add(time.Hour), Evidence{Kind: "payment_reversal_proof", Reference: "proof://refund-1"},
	); !errors.Is(err, ErrConcurrentModification) {
		t.Fatalf("divergent reversal timestamp retry: got %v, want ErrConcurrentModification", err)
	}
	if err := bill.ReversePayment("reverser-1", "refund-divergent", "journal-reversal-1", "payment rejected", reversedAt); !errors.Is(err, ErrConcurrentModification) {
		t.Fatalf("divergent reversal retry: got %v, want ErrConcurrentModification", err)
	}
	if err := bill.ReversePayment(
		"reverser-1", "refund-1", "journal-reversal-1", "payment rejected", reversedAt,
		Evidence{Kind: "payment_reversal_proof", Reference: "proof://different"},
	); !errors.Is(err, ErrConcurrentModification) {
		t.Fatalf("divergent reversal proof retry: got %v, want ErrConcurrentModification", err)
	}
	if err := bill.MarkPaid("payer-2", "pix-payment-too-early", "journal-payment-too-early", reversedAt.Add(-time.Microsecond)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("new payment before reversal: got %v, want ErrInvalidInput", err)
	}

	if err := bill.MarkPaid("payer-2", "pix-payment-2", "journal-payment-2", reversedAt.Add(time.Hour)); err != nil {
		t.Fatalf("new payment attempt after restore: %v", err)
	}
	if bill.Status != StatusPaid || bill.Version != 5 || bill.PaymentJournalSourceID != "journal-payment-2" {
		t.Fatalf("new attempt did not become active: %+v", bill)
	}
}

func TestManagedBill_ReverseReconciledPaymentPreservesReconciliationSnapshot(t *testing.T) {
	bill, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	approvedAt := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	paidAt := approvedAt.Add(time.Hour)
	reconciledAt := paidAt.Add(time.Hour)
	if err := bill.Approve("approver-1", approvedAt); err != nil {
		t.Fatal(err)
	}
	if err := bill.MarkPaid("payer-1", "pix-payment-1", "journal-payment-1", paidAt); err != nil {
		t.Fatal(err)
	}
	if err := bill.Reconcile("reconciler-1", "statement-1", reconciledAt); err != nil {
		t.Fatal(err)
	}

	if err := bill.ReversePayment("reverser-1", "refund-1", "journal-reversal-1", "credited back", reconciledAt.Add(time.Hour)); err != nil {
		t.Fatalf("ReversePayment reconciled: %v", err)
	}
	if bill.Status != StatusApproved || bill.Version != 5 || len(bill.PaymentAttempts) != 1 {
		t.Fatalf("unexpected reopened reconciled bill: status=%s version=%d attempts=%d", bill.Status, bill.Version, len(bill.PaymentAttempts))
	}
	attempt := bill.PaymentAttempts[0]
	if attempt.ReconciledBy != "reconciler-1" || attempt.ReconciledAt == nil || !attempt.ReconciledAt.Equal(reconciledAt) ||
		attempt.ReconciliationReference != "statement-1" {
		t.Fatalf("reconciliation snapshot lost: %+v", attempt)
	}
	last, ok := bill.LastPaymentAttempt()
	if !ok || last.ReconciledAt == nil {
		t.Fatalf("LastPaymentAttempt = (%+v, %v)", last, ok)
	}
	mutated := last.ReconciledAt.Add(24 * time.Hour)
	*last.ReconciledAt = mutated
	if !bill.PaymentAttempts[0].ReconciledAt.Equal(reconciledAt) {
		t.Fatal("LastPaymentAttempt exposed its reconciled_at pointer")
	}
}

func TestManagedBill_ReversePaymentRejectsInvalidStateFieldsAndTimes(t *testing.T) {
	newBill := func(t *testing.T, reconcile bool) (*ManagedBill, time.Time, time.Time) {
		t.Helper()
		bill, err := NewReserved(validReservedInput())
		if err != nil {
			t.Fatal(err)
		}
		approvedAt := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
		paidAt := approvedAt.Add(time.Hour)
		if err := bill.Approve("approver-1", approvedAt); err != nil {
			t.Fatal(err)
		}
		if err := bill.MarkPaid("payer-1", "pix-payment-1", "journal-payment-1", paidAt); err != nil {
			t.Fatal(err)
		}
		reconciledAt := paidAt.Add(time.Hour)
		if reconcile {
			if err := bill.Reconcile("reconciler-1", "statement-1", reconciledAt); err != nil {
				t.Fatal(err)
			}
		}
		return bill, paidAt, reconciledAt
	}

	reserved, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := reserved.ReversePayment("actor", "refund", "journal-reversal", "reason", time.Time{}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reverse RESERVED: got %v, want ErrInvalidTransition", err)
	}
	if err := reserved.Approve("approver", time.Time{}); err != nil {
		t.Fatal(err)
	}
	if err := reserved.ReversePayment("actor", "refund", "journal-reversal", "reason", time.Time{}); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("reverse APPROVED without archived attempt: got %v, want ErrInvalidTransition", err)
	}

	invalidFields := []struct {
		name, actor, reference, journalSource, reason string
	}{
		{name: "actor", actor: " ", reference: "refund", journalSource: "journal-reversal", reason: "reason"},
		{name: "reference", actor: "actor", reference: " ", journalSource: "journal-reversal", reason: "reason"},
		{name: "journal source", actor: "actor", reference: "refund", journalSource: " ", reason: "reason"},
		{name: "reason", actor: "actor", reference: "refund", journalSource: "journal-reversal", reason: " "},
	}
	for _, tc := range invalidFields {
		t.Run(tc.name, func(t *testing.T) {
			bill, _, reconciledAt := newBill(t, false)
			version := bill.Version
			err := bill.ReversePayment(tc.actor, tc.reference, tc.journalSource, tc.reason, reconciledAt)
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("got %v, want ErrInvalidInput", err)
			}
			if bill.Status != StatusPaid || bill.Version != version || len(bill.PaymentAttempts) != 0 {
				t.Fatalf("invalid reversal mutated bill: %+v", bill)
			}
		})
	}

	paidBill, paidAt, _ := newBill(t, false)
	if err := paidBill.ReversePayment("actor", "refund", "journal-reversal", "reason", paidAt.Add(-time.Microsecond)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reversal before paid_at: got %v, want ErrInvalidInput", err)
	}
	reconciledBill, _, reconciledAt := newBill(t, true)
	if err := reconciledBill.ReversePayment("actor", "refund", "journal-reversal", "reason", reconciledAt.Add(-time.Microsecond)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reversal before reconciled_at: got %v, want ErrInvalidInput", err)
	}
}

func TestManagedBill_MarkPaidRejectsReferencesArchivedByReversal(t *testing.T) {
	bill, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	approvedAt := time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC)
	paidAt := approvedAt.Add(time.Hour)
	if err := bill.Approve("approver-1", approvedAt); err != nil {
		t.Fatal(err)
	}
	if err := bill.MarkPaid("payer-1", "pix-payment-1", "journal-payment-1", paidAt); err != nil {
		t.Fatal(err)
	}
	if err := bill.ReversePayment("reverser-1", "refund-1", "journal-reversal-1", "rejected", paidAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	if err := bill.MarkPaid("payer-2", "pix-payment-1", "journal-payment-2", paidAt.Add(2*time.Hour)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reused payment_reference: got %v, want ErrInvalidInput", err)
	}
	if bill.Status != StatusApproved || bill.Version != 4 {
		t.Fatalf("reused reference mutated bill: status=%s version=%d", bill.Status, bill.Version)
	}
	if err := bill.MarkPaid("payer-2", "pix-payment-2", "journal-payment-1", paidAt.Add(2*time.Hour)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reused journal source: got %v, want ErrInvalidInput", err)
	}
	if err := bill.MarkPaid("payer-2", "pix-payment-2", "journal-payment-2", paidAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("new canonical references: %v", err)
	}
	secondPaidVersion := bill.Version
	if err := bill.ReversePayment("reverser-2", "refund-1", "journal-reversal-2", "second rejection", paidAt.Add(3*time.Hour)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reused reversal_reference: got %v, want ErrInvalidInput", err)
	}
	if err := bill.ReversePayment("reverser-2", "refund-2", "journal-reversal-1", "second rejection", paidAt.Add(3*time.Hour)); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("reused reversal journal source: got %v, want ErrInvalidInput", err)
	}
	if bill.Status != StatusPaid || bill.Version != secondPaidVersion || len(bill.PaymentAttempts) != 1 {
		t.Fatalf("reused reversal identity mutated bill: %+v", bill)
	}
}

func TestManagedBill_SameEconomicIdentityUsesCalendarDueDate(t *testing.T) {
	first, err := NewReserved(validReservedInput())
	if err != nil {
		t.Fatal(err)
	}
	otherInput := validReservedInput()
	otherInput.DueDate = time.Date(2026, time.August, 15, 23, 59, 0, 0, time.UTC)
	second, err := NewReserved(otherInput)
	if err != nil {
		t.Fatal(err)
	}
	if !first.SameEconomicIdentity(second) {
		t.Fatalf("same calendar due date should be idempotent: first=%v second=%v", first.DueDate, second.DueDate)
	}
	second.AmountCents++
	if first.SameEconomicIdentity(second) {
		t.Fatal("different amount must not share economic identity")
	}
}
