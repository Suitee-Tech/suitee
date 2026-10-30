package payout_order

import (
	"errors"
	"testing"
)

func newTestOrder(t *testing.T) *PayoutOrder {
	t.Helper()
	o, err := New("tenant-1", SourceCharge, "charge-1", RecipientBroker, "broker-1", "Broker A", 10000, "operator")
	if err != nil {
		t.Fatalf("constructor failed: %v", err)
	}
	if o.Status != StatusPendingReview {
		t.Fatalf("expected initial PENDING_REVIEW, got %s", o.Status)
	}
	return o
}

func TestNew_RejectsZeroAmount(t *testing.T) {
	t.Parallel()
	_, err := New("t", SourceCharge, "x", RecipientBroker, "", "Broker", 0, "op")
	if err == nil {
		t.Fatal("expected error for zero amount")
	}
}

func TestApprove_FromPendingReview(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	if err := o.Approve("operator-1"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if o.Status != StatusApproved {
		t.Fatalf("expected APPROVED, got %s", o.Status)
	}
	if o.ApprovedBy != "operator-1" {
		t.Fatalf("approver not stamped: got %q", o.ApprovedBy)
	}
	if o.ApprovedAt == nil {
		t.Fatal("approved_at not set")
	}
}

func TestApprove_RejectsFromPaid(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	_ = o.Approve("op")
	_ = o.MarkPaid("op", string(SettlementManualPIX), "https://drive/proof", "comprovante.pdf", "", true)
	if err := o.Approve("op"); err == nil {
		t.Fatal("expected error when approving terminal order")
	}
}

func TestMarkPaid_RequiresProof(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	_ = o.Approve("op")
	if err := o.MarkPaid("op", string(SettlementManualPIX), "", "", "", true); err == nil {
		t.Fatal("expected ErrProofRequired")
	}
}

func TestMarkPaid_OnlyFromApproved(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	// directly from PENDING_REVIEW should fail
	if err := o.MarkPaid("op", string(SettlementManualPIX), "https://x", "", "", true); err == nil {
		t.Fatal("expected error when marking PENDING_REVIEW paid")
	}
}

func TestCancel_BlockedAfterPaid(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	_ = o.Approve("op")
	_ = o.MarkPaid("op", string(SettlementManualPIX), "https://x", "x", "", true)
	if err := o.Cancel("op", "oops"); err == nil {
		t.Fatal("expected error when cancelling paid order")
	}
}

func TestAssignGroup_PreservesState(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	if err := o.AssignGroup("group-xyz", "op"); err != nil {
		t.Fatalf("group assignment failed: %v", err)
	}
	if o.GroupID != "group-xyz" {
		t.Fatalf("expected group_id stamped: got %q", o.GroupID)
	}
	if o.Status != StatusPendingReview {
		t.Fatalf("grouping should not change status, got %s", o.Status)
	}
}

func TestTerminal(t *testing.T) {
	t.Parallel()
	o := newTestOrder(t)
	if o.Terminal() {
		t.Fatal("pending order should not be terminal")
	}
	_ = o.Approve("op")
	_ = o.MarkPaid("op", string(SettlementManualPIX), "https://x", "x", "", true)
	if !o.Terminal() {
		t.Fatal("paid order should be terminal")
	}
}

// Cancelar não pode ser um beco sem saída: o operador cancela (destinatário
// errado, valor a corrigir) e precisa regerar a obrigação para lançar de novo.
// A ordem nova preserva a IDENTIDADE da origem — mesmo source_event_id, mesmo
// destinatário, mesmo valor — e aponta para a cancelada.
func TestRegenerate_PreservesObligationIdentity(t *testing.T) {
	original := &PayoutOrder{
		ID:                   "pay-1",
		TenantID:             "tenant-1",
		SourceType:           SourceManual,
		SourceID:             "contract-1",
		SourceRef:            map[string]any{"deal_id": "deal-1"},
		ContractID:           "contract-1",
		RecipientType:        RecipientBroker,
		RecipientID:          "broker-1",
		RecipientLabel:       "Comissão captador (20.0%)",
		RecipientPixKey:      "pix@example.com",
		AmountCents:          2880000,
		FundingRequiredCents: 1000,
		FundedAmountCents:    1000,
		FundingEntries:       []FundingEntry{{PaymentID: "pay-1", ChargeID: "charge-1", AmountCents: 1000}},
		Status:               StatusCancelled,
		SourceEventID:        "source-1",
	}
	fresh, err := original.Regenerate("operator-1")
	if err != nil {
		t.Fatalf("Regenerate: %v", err)
	}
	if fresh.ID == original.ID {
		t.Fatal("a ordem regerada precisa de id próprio")
	}
	if fresh.Status != StatusPendingReview {
		t.Errorf("status = %s, want PENDING_REVIEW (pronta para aprovar e lançar)", fresh.Status)
	}
	if fresh.SourceEventID != original.SourceEventID {
		t.Errorf("source_event_id = %s, want %s (a obrigação é a mesma)", fresh.SourceEventID, original.SourceEventID)
	}
	if fresh.AmountCents != original.AmountCents || fresh.RecipientID != original.RecipientID || fresh.RecipientLabel != original.RecipientLabel {
		t.Errorf("destinatário/valor não preservados: %+v", fresh)
	}
	if fresh.ContractID != original.ContractID || fresh.SourceRef["deal_id"] != "deal-1" {
		t.Errorf("origem não preservada: contract=%s ref=%v", fresh.ContractID, fresh.SourceRef)
	}
	if fresh.FundingRequiredCents != original.FundingRequiredCents {
		t.Errorf("portão de liquidez = %d, want %d (regerar não pode liberar repasse sem recebimento)", fresh.FundingRequiredCents, original.FundingRequiredCents)
	}
	// A cobertura já coletada viaja com a obrigação: sem isso a ordem regerada
	// volta a "Aguardando recebimentos" e o operador não consegue lançar mesmo
	// com o dinheiro na conta.
	if fresh.FundedAmountCents != original.FundedAmountCents || len(fresh.FundingEntries) != 1 {
		t.Errorf("cobertura perdida na regeração: funded=%d entries=%d", fresh.FundedAmountCents, len(fresh.FundingEntries))
	}
	if fresh.FundingOutstandingCents() != 0 {
		t.Errorf("pendência de cobertura = %d, want 0 (obrigação já coberta)", fresh.FundingOutstandingCents())
	}
	if fresh.RegeneratedFromID != original.ID {
		t.Errorf("regenerated_from_id = %s, want %s", fresh.RegeneratedFromID, original.ID)
	}
	if len(fresh.History) == 0 || fresh.History[len(fresh.History)-1].Action != "regenerated" {
		t.Errorf("histórico sem a marca da regeração: %+v", fresh.History)
	}
	if original.Status != StatusCancelled {
		t.Errorf("a ordem cancelada foi alterada: %s", original.Status)
	}
}

func TestRegenerate_RejectsOrdersThatAreNotCancelled(t *testing.T) {
	for _, status := range []Status{StatusPendingReview, StatusApproved, StatusPaid, StatusFailed} {
		order := &PayoutOrder{ID: "pay-1", TenantID: "tenant-1", Status: status, AmountCents: 100, SourceType: SourceManual, RecipientType: RecipientExternal}
		if _, err := order.Regenerate("operator-1"); !errors.Is(err, ErrNotCancelled) {
			t.Errorf("status %s: err = %v, want ErrNotCancelled", status, err)
		}
	}
}

// O relato: "não está sendo possível marcar cobrança como paga" — 500 no
// reconcile-manual com "payout funding gate conflicts with existing payout
// state". A ordem carrega o LÍQUIDO (já com IRRF/INSS/ISS retidos) e o portão de
// liquidez era a obrigação BRUTA da comissão: a igualdade exigida antes recusava
// toda ordem com desconto. Líquido menor é o caso normal; maior é que é conflito.
func TestEnableFundingGate_AcceptsOrderNetOfDeductions(t *testing.T) {
	newOrder := func(amountCents int64) *PayoutOrder {
		return &PayoutOrder{
			ID: "pay-1", TenantID: "tenant-1", Status: StatusPendingReview,
			AmountCents: amountCents, SourceType: SourceManual, RecipientType: RecipientBroker,
		}
	}
	// Bruto R$ 26.400 com IRRF 6% + ISS 2% + INSS 20% → líquido R$ 19.008.
	changed, err := newOrder(1900800).EnableFundingGate(2640000)
	if err != nil || !changed {
		t.Fatalf("ordem com descontos recusada: changed=%v err=%v", changed, err)
	}
	if _, err := newOrder(2640000).EnableFundingGate(2640000); err != nil {
		t.Fatalf("ordem sem descontos (líquido = bruto) recusada: %v", err)
	}
	// Pagável acima da obrigação continua sendo conflito.
	if _, err := newOrder(2700000).EnableFundingGate(2640000); !errors.Is(err, ErrFundingGateConflict) {
		t.Fatalf("pagável acima da obrigação: err=%v, want ErrFundingGateConflict", err)
	}
}
