package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// Regressão do caso real (tenant Exemplo): a cobrança paga aloca R$ 270,95 ao
// proprietário com 100% reservado para contas administradas. Quando a última
// conta é paga, a reserva zera e não sobra nada a repassar — o repasse precisa
// ser encerrado na mesma transação do pagamento, senão fica PENDING para sempre
// e o operador não tem baixa possível.
func TestManagedBillPaymentClosesZeroOwnerTransferAndReversalReopensIt(t *testing.T) {
	ctx := context.Background()
	item := transfer.New("contract-1", "owner-1", "property-1", "tenant-1", "2026-10",
		2709.69, 10.000036904590562, 0, 0, nil, "Repasse automático originado pela charge charge-1")
	item.ID = "transfer-1"
	item.SourceChargeID = "charge-1"
	item.CreatedAt = time.Date(2026, time.October, 2, 13, 27, 48, 0, time.UTC)
	item.UpdatedAt = item.CreatedAt
	balance, err := transfer.NewOwnerBalance(27095, 27095, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := item.SetAllocatedOwnerBalance(balance, 27097); err != nil {
		t.Fatal(err)
	}
	if item.HasNoPayableBalance() {
		t.Fatal("fixture precisa começar com reserva ativa, não zerada")
	}

	bills := newFakeManagedBillRepo()
	transfers := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{cloneOwnerTransfer(item)}}
	service := NewManagedBillService(bills, transfers).WithJournalService(NewJournalService(newFakeJournalRepo()))
	service.atomicTxRunner = func(ctx context.Context, work func(context.Context) error) error { return work(ctx) }

	condoInput := managedBillReserveInput("condo-item", "reserve:condo", "condominio", "Condominio", 19354)
	iptuInput := managedBillReserveInput("iptu-item", "reserve:iptu", "iptu", "Prefeitura", 7741)
	condo, err := service.EnsureReserved(ctx, condoInput)
	if err != nil {
		t.Fatalf("reserve condo: %v", err)
	}
	iptu, err := service.EnsureReserved(ctx, iptuInput)
	if err != nil {
		t.Fatalf("reserve IPTU: %v", err)
	}
	if !iptu.CoverageComplete {
		t.Fatal("as duas contas devem cobrir a reserva integral")
	}

	approvedCondo, err := service.Approve(ctx, "tenant-1", condo.Bill.ID, "approver-1", condo.Bill.Version)
	if err != nil {
		t.Fatalf("approve condo: %v", err)
	}
	_, err = service.Pay(ctx, "tenant-1", condo.Bill.ID, "payer-1", approvedCondo.Bill.Version, "pix-condo-1", "proof://condo", time.Time{})
	if err != nil {
		t.Fatalf("pay condo: %v", err)
	}
	afterFirst := findOwnerTransfer(t, transfers, "transfer-1")
	if afterFirst.Status != transfer.StatusPending || afterFirst.SettledWithoutBalance {
		t.Fatalf("reserva ainda ativa: repasse não pode encerrar (status=%s marker=%v)", afterFirst.Status, afterFirst.SettledWithoutBalance)
	}
	approvedIptu, err := service.Approve(ctx, "tenant-1", iptu.Bill.ID, "approver-1", iptu.Bill.Version)
	if err != nil {
		t.Fatalf("approve IPTU: %v", err)
	}
	paidIptu, err := service.Pay(ctx, "tenant-1", iptu.Bill.ID, "payer-1", approvedIptu.Bill.Version, "pix-iptu-1", "proof://iptu", time.Time{})
	if err != nil {
		t.Fatalf("pay IPTU: %v", err)
	}
	settled := findOwnerTransfer(t, transfers, "transfer-1")
	if settled.Status != transfer.StatusProcessed || !settled.SettledWithoutBalance {
		t.Fatalf("repasse zerado deve encerrar com o último pagamento: status=%s marker=%v", settled.Status, settled.SettledWithoutBalance)
	}
	if settled.SettledWithoutBalance && settled.SettlementJournalSourceID != "" {
		t.Fatalf("encerramento sem saldo não tem lançamento: %q", settled.SettlementJournalSourceID)
	}
	if settled.OwnerBalance.AvailableCents() != 0 || settled.OwnerBalance.ReservedCents != 0 {
		t.Fatalf("saldo do proprietário deveria estar zerado: %+v", settled.OwnerBalance)
	}

	// Estorno do pagamento da última conta devolve a reserva e reabre o repasse:
	// o proprietário volta a ter uma obrigação a liquidar.
	reconciled, err := service.Reconcile(ctx, "tenant-1", iptu.Bill.ID, "reconciler-1", paidIptu.Bill.Version, "statement-iptu", time.Time{})
	if err != nil {
		t.Fatalf("reconcile IPTU: %v", err)
	}
	if _, err := service.ReversePayment(ctx, "tenant-1", iptu.Bill.ID, "controller-1", reconciled.Bill.Version,
		"bank-return-iptu", "beneficiário recusou o crédito", "proof://return-iptu", time.Time{}); err != nil {
		t.Fatalf("reverse IPTU payment: %v", err)
	}
	reopened := findOwnerTransfer(t, transfers, "transfer-1")
	if reopened.Status != transfer.StatusPending || reopened.SettledWithoutBalance {
		t.Fatalf("estorno deve reabrir o repasse: status=%s marker=%v", reopened.Status, reopened.SettledWithoutBalance)
	}
	if reopened.OwnerBalance.ReservedCents != 7741 {
		t.Fatalf("reserva restaurada esperada 7741, got %d", reopened.OwnerBalance.ReservedCents)
	}
	if last := reopened.History[len(reopened.History)-1]; last.Action != "reopened" {
		t.Fatalf("histórico da reabertura ausente: %+v", last)
	}
}

func findOwnerTransfer(t *testing.T, repo *chargeIdentityTransferRepo, id string) *transfer.OwnerTransfer {
	t.Helper()
	for _, candidate := range repo.items {
		if candidate.ID == id {
			return candidate
		}
	}
	t.Fatalf("transfer %s não encontrado no repositório", id)
	return nil
}
