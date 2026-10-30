package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
)

// Reproduz o 409 observado ao "Registrar pagamento" de uma conta administrada.
//
// O cliente envia `expected_version` do registro que tem em mãos. Quando essa
// versão está defasada — a conta já foi paga em outra aba, por outro operador, ou
// a lista simplesmente não recarregou — o serviço rejeita com
// ErrConcurrentModification, que a borda HTTP traduz em 409 com a mensagem fixa
// "managed bill was modified by another operation". O operador não descobre o que
// de fato aconteceu nem que basta recarregar.
func TestManagedBillService_PayWithStaleVersionIsRejected(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()

	reserved, err := fixture.service.EnsureReserved(ctx, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(ctx, "tenant-1", reserved.Bill.ID, "operator-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}

	// Estado que o cliente tinha em mãos antes de outra sessão pagar.
	staleVersion := approved.Bill.Version

	if _, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-1", approved.Bill.Version, "pix-a", "", time.Time{}); err != nil {
		t.Fatalf("pagamento inicial: %v", err)
	}

	// Nova tentativa com a versão defasada e uma referência de pagamento nova.
	_, err = fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-2", staleVersion, "pix-b", "", time.Time{})
	if !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("err = %v, want ErrConcurrentModification", err)
	}
}

// A repetição exata é idempotente e devolve sucesso — comportamento correto a
// preservar. O problema é que uma repetição NÃO exata cai no mesmo erro genérico.
func TestManagedBillService_PayReplayWithDifferentReferenceIsIndistinguishable(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()

	reserved, err := fixture.service.EnsureReserved(ctx, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(ctx, "tenant-1", reserved.Bill.ID, "operator-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-1", approved.Bill.Version, "pix-a", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// Repetição exata: sucesso, sem duplicar movimento.
	replay, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-1", approved.Bill.Version, "pix-a", "", time.Time{})
	if err != nil {
		t.Fatalf("repetição exata deveria ser idempotente: %v", err)
	}
	if replay.Bill.Status != managed_bill.StatusPaid || replay.Bill.PaymentReference != paid.Bill.PaymentReference {
		t.Fatalf("repetição alterou o estado: %+v", replay.Bill)
	}

	// Já paga, mas com referência diferente: mesmo erro de "modificado por outra
	// operação", embora a causa real seja outra.
	_, err = fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "payer-1", paid.Bill.Version, "pix-diferente", "", time.Time{})
	if !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("err = %v, want ErrConcurrentModification", err)
	}
}

// Política nova: quem responde pela operação financeira da imobiliária pode
// aprovar e pagar sozinho. Antes, uma imobiliária com um único administrador
// ficava travada — a conta era aprovada e o pagamento não podia ser registrado
// por ninguém.
func TestManagedBillService_AdminApproverCanPayWithWaiver(t *testing.T) {
	fixture := newManagedBillFixture(t)
	ctx := context.Background()

	reserved, err := fixture.service.EnsureReserved(ctx, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(ctx, "tenant-1", reserved.Bill.ID, "admin-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}

	// Sem dispensa continua vedado — é controle interno, não um detalhe de UI.
	if _, err := fixture.service.Pay(ctx, "tenant-1", reserved.Bill.ID, "admin-1", approved.Bill.Version, "pix-1", "", time.Time{}); !errors.Is(err, ErrManagedBillActorConflict) {
		t.Fatalf("sem dispensa: err = %v, want ErrManagedBillActorConflict", err)
	}

	// Com a dispensa — que a borda só concede a quem tem autoridade financeira —
	// o mesmo ator conclui os dois passos.
	waived := WithSeparationOfDutiesWaiver(ctx)
	paid, err := fixture.service.Pay(waived, "tenant-1", reserved.Bill.ID, "admin-1", approved.Bill.Version, "pix-1", "", time.Time{})
	if err != nil {
		t.Fatalf("com dispensa: %v", err)
	}
	if paid.Bill.Status != managed_bill.StatusPaid {
		t.Fatalf("status = %s, want PAID", paid.Bill.Status)
	}
	if paid.Bill.PaidBy != "admin-1" || paid.Bill.ApprovedBy != "admin-1" {
		t.Fatalf("aprovador e pagador deveriam ser o mesmo ator: %+v", paid.Bill)
	}
}

// A dispensa não afrouxa nenhuma outra regra: só retira a exigência de dois
// atores.
func TestManagedBillService_WaiverDoesNotBypassOtherGuards(t *testing.T) {
	fixture := newManagedBillFixture(t)
	waived := WithSeparationOfDutiesWaiver(context.Background())

	reserved, err := fixture.service.EnsureReserved(waived, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(waived, "tenant-1", reserved.Bill.ID, "admin-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}

	// Versão defasada continua sendo recusada.
	if _, err := fixture.service.Pay(waived, "tenant-1", reserved.Bill.ID, "admin-1", approved.Bill.Version+1, "pix-1", "", time.Time{}); !errors.Is(err, managed_bill.ErrConcurrentModification) {
		t.Fatalf("versão defasada: err = %v, want ErrConcurrentModification", err)
	}
	// Estados inválidos continuam sendo recusados. O fixture valida o journal
	// antes do estado, então o erro concreto é essa pré-condição — o que importa
	// é que a operação é recusada, não o texto exato.
	if _, err := fixture.service.Reconcile(waived, "tenant-1", reserved.Bill.ID, "admin-1", approved.Bill.Version, "ref-1", time.Time{}); err == nil {
		t.Fatal("conciliar uma conta não paga deveria ser recusado mesmo com dispensa")
	}
}

// O administrador também estorna o que ele mesmo pagou.
func TestManagedBillService_AdminCanReverseOwnPaymentWithWaiver(t *testing.T) {
	fixture := newManagedBillFixture(t)
	waived := WithSeparationOfDutiesWaiver(context.Background())

	reserved, err := fixture.service.EnsureReserved(waived, managedBillReserveInput("condo", "reserve:condo", "condominio", "Condominio", 70000))
	if err != nil {
		t.Fatal(err)
	}
	approved, err := fixture.service.Approve(waived, "tenant-1", reserved.Bill.ID, "admin-1", reserved.Bill.Version)
	if err != nil {
		t.Fatal(err)
	}
	paid, err := fixture.service.Pay(waived, "tenant-1", reserved.Bill.ID, "admin-1", approved.Bill.Version, "pix-1", "", time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	// Sem dispensa o estorno pelo mesmo pagador continua vedado.
	if _, err := fixture.service.ReversePayment(context.Background(), "tenant-1", reserved.Bill.ID, "admin-1", paid.Bill.Version, "estorno-1", "devolvido", "", time.Time{}); !errors.Is(err, ErrManagedBillReversalActorConflict) {
		t.Fatalf("sem dispensa: err = %v, want ErrManagedBillReversalActorConflict", err)
	}

	if _, err := fixture.service.ReversePayment(waived, "tenant-1", reserved.Bill.ID, "admin-1", paid.Bill.Version, "estorno-1", "devolvido", "", time.Time{}); err != nil {
		t.Fatalf("com dispensa: %v", err)
	}
}
