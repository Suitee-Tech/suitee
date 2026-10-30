package transfer

import (
	"testing"
	"time"
)

// O caso real do beta: a cobrança paga aloca R$ 270,95 ao proprietário, mas
// 100% do valor está reservado para contas administradas. Cada conta paga
// consome a reserva; quando a última é paga sobra R$ 0,00 e não há mais nada a
// repassar. Sem a transição automática o repasse fica PENDING para sempre.
func TestApplyOwnerBalanceMovement_settlesZeroTransferAfterLastReservedBill(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 2709.69, 10.000036904590562, 0, 0, nil, "")
	balance, err := NewOwnerBalance(27095, 27095, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 27097); err != nil {
		t.Fatal(err)
	}
	if tr.NetAmount != 0 || tr.HasNoPayableBalance() {
		t.Fatalf("fixture deve ter líquido zerado com reserva ativa: net=%.2f reserved=%d", tr.NetAmount, tr.OwnerBalance.ReservedCents)
	}

	first := tr.CreatedAt.Add(time.Minute)
	changed, err := tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "mv-1", ManagedBillID: "bill-1", PaymentAttemptID: "mv-1",
		Type: OwnerBalanceMovementConsumeReserved, AmountCents: 19354, Actor: "op-1", At: first,
	})
	if err != nil || !changed {
		t.Fatalf("primeiro consumo falhou: changed=%v err=%v", changed, err)
	}
	if tr.Status != StatusPending {
		t.Fatalf("reserva ainda ativa: repasse não pode ser encerrado (status=%s)", tr.Status)
	}

	second := first.Add(18 * time.Second)
	changed, err = tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "mv-2", ManagedBillID: "bill-2", PaymentAttemptID: "mv-2",
		Type: OwnerBalanceMovementConsumeReserved, AmountCents: 7741, Actor: "op-1", At: second,
	})
	if err != nil || !changed {
		t.Fatalf("segundo consumo falhou: changed=%v err=%v", changed, err)
	}
	if tr.Status != StatusProcessed || !tr.SettledWithoutBalance {
		t.Fatalf("repasse zerado deveria encerrar automaticamente: status=%s marker=%v", tr.Status, tr.SettledWithoutBalance)
	}
	if tr.ProcessedAt == nil || !tr.ProcessedAt.Equal(second) {
		t.Fatalf("processed_at deve ser o instante do movimento: %v", tr.ProcessedAt)
	}
	if tr.ProcessedBy != "system" {
		t.Fatalf("fechamento automático é do sistema, got %q", tr.ProcessedBy)
	}
	if tr.ApprovedAt != nil || tr.ApprovedBy != "" {
		t.Fatalf("fechamento sem saldo não inventa aprovação: %+v/%+v", tr.ApprovedAt, tr.ApprovedBy)
	}
	if tr.SettlementJournalSourceID != "" {
		t.Fatalf("fechamento sem saldo não tem lançamento de liquidação: %q", tr.SettlementJournalSourceID)
	}
	last := tr.History[len(tr.History)-1]
	if last.Action != "processed" || last.Note == "" {
		t.Fatalf("histórico do encerramento ausente: %+v", last)
	}
}

// Reabertura: restaurar a reserva (estorno do pagamento da conta) devolve a
// obrigação ao estado anterior em vez de deixar o repasse PROCESSED sem lastro.
func TestApplyOwnerBalanceMovement_reopensSettledTransferWhenReserveRestored(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 1000, 0, 0, 0, nil, "")
	balance, err := NewOwnerBalance(100000, 100000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.SetAllocatedOwnerBalance(balance, 0); err != nil {
		t.Fatal(err)
	}
	at := tr.CreatedAt.Add(time.Minute)
	if _, err := tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "mv-1", ManagedBillID: "bill-1", PaymentAttemptID: "mv-1",
		Type: OwnerBalanceMovementConsumeReserved, AmountCents: 100000, Actor: "op-1", At: at,
	}); err != nil {
		t.Fatal(err)
	}
	if !tr.SettledWithoutBalance || tr.Status != StatusProcessed {
		t.Fatalf("setup: repasse deveria estar encerrado sem saldo (status=%s)", tr.Status)
	}
	if _, err := tr.ApplyOwnerBalanceMovement(OwnerBalanceMovement{
		ID: "mv-2", ManagedBillID: "bill-1", PaymentAttemptID: "mv-1",
		Type: OwnerBalanceMovementRestoreReserved, AmountCents: 100000, Actor: "op-1", At: at.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	if tr.SettledWithoutBalance || tr.Status != StatusPending {
		t.Fatalf("estorno da conta deve reabrir o repasse: status=%s marker=%v", tr.Status, tr.SettledWithoutBalance)
	}
	if tr.ProcessedAt != nil || tr.ProcessedBy != "" {
		t.Fatalf("reabertura limpa o encerramento: at=%v by=%q", tr.ProcessedAt, tr.ProcessedBy)
	}
	last := tr.History[len(tr.History)-1]
	if last.Action != "reopened" || last.Note == "" {
		t.Fatalf("histórico da reabertura ausente: %+v", last)
	}
}

func TestSettleWithoutPayableBalance_recordsOperatorWithoutInventingPayment(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 3200, 100, 3200, 0, nil, "")
	if tr.NetAmount != 0 {
		t.Fatalf("fixture deve ter líquido zerado, got %.2f", tr.NetAmount)
	}
	at := tr.CreatedAt.Add(time.Minute)
	if !tr.SettleWithoutPayableBalance(at, "op-42", "Ayla Operadora") {
		t.Fatal("baixa de repasse zerado deveria concluir")
	}
	if tr.Status != StatusProcessed || !tr.SettledWithoutBalance {
		t.Fatalf("estado da conclusão incorreto: status=%s marker=%v", tr.Status, tr.SettledWithoutBalance)
	}
	if tr.ProcessedBy != "op-42" {
		t.Fatalf("operador que concluiu deve ficar registrado, got %q", tr.ProcessedBy)
	}
	if tr.ProcessedAt == nil || !tr.ProcessedAt.Equal(at) {
		t.Fatalf("processed_at incorreto: %v", tr.ProcessedAt)
	}
	if tr.SettlementJournalSourceID != "" {
		t.Fatalf("conclusão sem caixa não pode ter lançamento: %q", tr.SettlementJournalSourceID)
	}
	last := tr.History[len(tr.History)-1]
	if last.Action != "processed" || last.Actor != "op-42" || last.ActorLabel != "Ayla Operadora" {
		t.Fatalf("histórico deve nomear o operador: %+v", last)
	}
	if tr.SettleWithoutPayableBalance(at.Add(time.Minute), "op-42", "Ayla Operadora") {
		t.Fatal("conclusão deve ser idempotente")
	}
}

func TestSettleWithoutPayableBalance_refusesWhenSomethingIsPayable(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 1000, 10, 0, 0, nil, "")
	if tr.HasNoPayableBalance() {
		t.Fatal("fixture deve ter valor a repassar")
	}
	if tr.SettleWithoutPayableBalance(time.Now(), "op-1", "Operador") {
		t.Fatal("repasse com valor a repassar não pode ser encerrado sem saldo")
	}
	if tr.Status != StatusPending {
		t.Fatalf("status não deveria mudar: %s", tr.Status)
	}
}

func TestSettleWithoutPayableBalance_refusesCancelledTransfer(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 1000, 100, 1000, 0, nil, "")
	if err := tr.Cancel("op-1", "Operador", "competência errada"); err != nil {
		t.Fatal(err)
	}
	if tr.SettleWithoutPayableBalance(time.Now(), "op-2", "Outro") {
		t.Fatal("repasse cancelado não é concluído por este caminho")
	}
	if tr.Status != StatusCancelled {
		t.Fatalf("status mudou indevidamente: %s", tr.Status)
	}
}

func TestReopenWithoutPayableBalance_preservesApprovalAndRefusesPaidTransfer(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 1000, 100, 1000, 0, nil, "")
	if err := tr.Approve("approver-1", "Aprovadora"); err != nil {
		t.Fatal(err)
	}
	at := tr.CreatedAt.Add(2 * time.Minute)
	if !tr.SettleWithoutPayableBalance(at, "op-2", "Operador") {
		t.Fatal("setup: conclusão sem saldo falhou")
	}
	if !tr.ReopenWithoutPayableBalance(at.Add(time.Minute), "saldo devolvido") {
		t.Fatal("repasse encerrado sem saldo deveria reabrir")
	}
	if tr.Status != StatusApproved {
		t.Fatalf("aprovação anterior continua válida: status=%s", tr.Status)
	}
	if tr.SettledWithoutBalance || tr.ProcessedAt != nil || tr.ProcessedBy != "" {
		t.Fatalf("reabertura deve limpar a conclusão: %+v", tr)
	}
	if last := tr.History[len(tr.History)-1]; last.Action != "reopened" || last.Note != "saldo devolvido" {
		t.Fatalf("histórico da reabertura: %+v", last)
	}
	if tr.ReopenWithoutPayableBalance(at.Add(2*time.Minute), "de novo") {
		t.Fatal("reabertura deve ser idempotente")
	}

	// Um repasse realmente pago (com lançamento de liquidação) nunca reabre por
	// este caminho, mesmo que o líquido esteja zerado.
	paid := New("c-2", "o-1", "p-2", "t-1", "2026-10", 1000, 100, 0, 0, nil, "")
	paid.SettledWithoutBalance = true
	paid.Status = StatusProcessed
	paid.SettlementJournalSourceID = "owner_transfer.settlement:algum"
	if paid.ReopenWithoutPayableBalance(time.Now(), "tentativa") {
		t.Fatal("repasse com lançamento de liquidação não pode reabrir")
	}
}

// A reavaliação é o ponto único usado por todos os caminhos de escrita: ela
// encerra quando o valor a repassar some (dedução) e reabre quando ele volta
// (crédito/estorno), sem depender de quem escreveu.
func TestSyncNoPayableBalanceState_closesOnDeductionAndReopensOnCredit(t *testing.T) {
	tr := New("c-1", "o-1", "p-1", "t-1", "2026-10", 1000, 10, 0, 0, nil, "")
	if !tr.HasNoPayableBalance() && tr.NetAmount != 900 {
		t.Fatalf("fixture inesperada: net=%.2f", tr.NetAmount)
	}
	if tr.SyncNoPayableBalanceState(time.Now()) {
		t.Fatal("repasse com valor a repassar não pode ser encerrado")
	}
	tr.MaintenanceDeductions = 900
	tr.Calculate()
	if !tr.SyncNoPayableBalanceState(time.Now()) {
		t.Fatal("dedução que consome todo o líquido deve encerrar o repasse")
	}
	if !tr.SettledWithoutBalance || tr.Status != StatusProcessed {
		t.Fatalf("estado após a dedução: status=%s marker=%v", tr.Status, tr.SettledWithoutBalance)
	}
	if tr.SyncNoPayableBalanceState(time.Now()) {
		t.Fatal("reavaliação deve ser idempotente")
	}

	tr.MaintenanceDeductions = 400
	tr.Calculate()
	if !tr.SyncNoPayableBalanceState(time.Now()) {
		t.Fatal("crédito que devolve valor a repassar deve reabrir o repasse")
	}
	if tr.Status != StatusPending || tr.SettledWithoutBalance {
		t.Fatalf("estado após o crédito: status=%s marker=%v", tr.Status, tr.SettledWithoutBalance)
	}
	if tr.NetAmount != 500 {
		t.Fatalf("líquido esperado 500, got %.2f", tr.NetAmount)
	}
}
