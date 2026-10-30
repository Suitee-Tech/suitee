package services

import (
	"context"
	"errors"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// P1/A13 — segregação de funções em repasses, payouts e saques.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A13) mostrou que a segregação de funções existia **apenas** no fluxo de
// managed bills (`managed_bill_service.go`): repasses, payout orders, saques e
// comissões aceitavam o MESMO ator criando, aprovando e pagando — e o papel
// `financeiro` concentra `finance:write` e `finance:approve_transfer`, então um
// único usuário fechava o ciclo do dinheiro.
//
// As regras agora são as mesmas do fluxo já existente:
//
//	criador ≠ aprovador   (Requester/RequestedBy × Approve)
//	aprovador ≠ pagador   (ApprovedBy × Process/MarkPaid/Settle)
//
// com a dispensa explícita (`WithSeparationOfDutiesWaiver`) para quem responde
// pela operação financeira da imobiliária — concedida só na borda HTTP, a partir
// do papel autenticado.

func a13Transfer(t *testing.T) *transfer.OwnerTransfer {
	t.Helper()
	tr := transfer.New("contract-a13", "owner-a13", "prop-a13", "tenant-a13", "2026-09",
		5000, 10, 0, 0, nil, "")
	if err := tr.Approve("approver-a13", "Aprovador"); err != nil {
		t.Fatalf("approve seed: %v", err)
	}
	return tr
}

// Quem aprovou o repasse não efetiva o pagamento.
func TestTransfer_ApproverCannotProcessOwnApproval(t *testing.T) {
	tr := a13Transfer(t)
	svc := NewTransferService(newCaptureRepo(tr))

	err := svc.Process(context.Background(), tr.TenantID, tr.ID, "approver-a13", "Aprovador")
	if !errors.Is(err, ErrSegregationOfDutiesApproverCannotPay) {
		t.Fatalf("esperava ErrSegregationOfDutiesApproverCannotPay, veio %v", err)
	}
}

// Outro operador efetiva normalmente (sem regressão do fluxo legítimo).
func TestTransfer_AnotherOperatorCanProcess(t *testing.T) {
	tr := a13Transfer(t)
	repo := newCaptureRepo(tr)
	svc := NewTransferService(repo)

	if err := svc.Process(context.Background(), tr.TenantID, tr.ID, "operator-2", "Operador 2"); err != nil {
		t.Fatalf("outro operador deveria efetivar o repasse: %v", err)
	}
	if repo.store[tr.ID].Status != transfer.StatusProcessed {
		t.Fatalf("repasse não foi processado: %s", repo.store[tr.ID].Status)
	}
}

// A dispensa (autoridade financeira, concedida na borda) libera o fluxo de um
// ator só — é o caso da imobiliária pequena, como no fluxo de managed bills.
func TestTransfer_FinanceAuthorityWaiverAllowsSingleActor(t *testing.T) {
	tr := a13Transfer(t)
	svc := NewTransferService(newCaptureRepo(tr))
	ctx := WithSeparationOfDutiesWaiver(context.Background())

	if err := svc.Process(ctx, tr.TenantID, tr.ID, "approver-a13", "Aprovador"); err != nil {
		t.Fatalf("com dispensa o mesmo ator deve efetivar: %v", err)
	}
}

func a13PayoutOrder(t *testing.T, requester string) *payout_order.PayoutOrder {
	t.Helper()
	order, err := payout_order.New("tenant-a13", payout_order.SourceManual, "contract-a13",
		payout_order.RecipientExternal, "recipient-1", "Favorecido", 10_000, requester)
	if err != nil {
		t.Fatalf("payout seed: %v", err)
	}
	order.Status = payout_order.StatusPendingReview
	return order
}

// Payout: criador ≠ aprovador e aprovador ≠ pagador.
func TestPayoutOrder_RequesterCannotApproveAndApproverCannotPay(t *testing.T) {
	t.Run("criador não aprova", func(t *testing.T) {
		order := a13PayoutOrder(t, "requester-a13")
		repo := &payoutCASTestRepo{order: order}
		_, err := NewPayoutOrderService(repo).Approve(context.Background(), order.TenantID, order.ID, "requester-a13")
		if !errors.Is(err, ErrSegregationOfDutiesRequesterCannotApprove) {
			t.Fatalf("esperava ErrSegregationOfDutiesRequesterCannotApprove, veio %v", err)
		}
	})

	t.Run("aprovador não paga", func(t *testing.T) {
		order := a13PayoutOrder(t, "requester-a13")
		repo := &payoutCASTestRepo{order: order}
		svc := NewPayoutOrderService(repo)
		if _, err := svc.Approve(context.Background(), order.TenantID, order.ID, "approver-a13"); err != nil {
			t.Fatalf("aprovação por outro ator deveria passar: %v", err)
		}
		_, err := svc.MarkPaid(context.Background(), order.TenantID, order.ID, "approver-a13", MarkPaidInput{
			Method: "MANUAL_PIX", ProofURL: "https://proof.example/a13.pdf",
		})
		if !errors.Is(err, ErrSegregationOfDutiesApproverCannotPay) {
			t.Fatalf("esperava ErrSegregationOfDutiesApproverCannotPay, veio %v", err)
		}
	})

	t.Run("dispensa libera ator único", func(t *testing.T) {
		order := a13PayoutOrder(t, "requester-a13")
		repo := &payoutCASTestRepo{order: order}
		svc := NewPayoutOrderService(repo)
		ctx := WithSeparationOfDutiesWaiver(context.Background())
		if _, err := svc.Approve(ctx, order.TenantID, order.ID, "requester-a13"); err != nil {
			t.Fatalf("com dispensa o autor pode aprovar: %v", err)
		}
	})
}

// Saque: criador ≠ aprovador e aprovador ≠ quem efetiva.
func TestWithdrawal_RequesterCannotApproveAndApproverCannotSettle(t *testing.T) {
	t.Run("criador não aprova", func(t *testing.T) {
		repo := newFakeWithdrawalRepo()
		svc := NewWithdrawalService(repo).WithJournalService(NewJournalService(newFakeJournalRepo()))
		w, err := svc.Request(context.Background(), "tenant-1", 1000, "saque", "requester-a13")
		if err != nil {
			t.Fatalf("Request: %v", err)
		}
		if _, err := svc.Approve(context.Background(), "tenant-1", w.ID, "requester-a13"); !errors.Is(err, ErrSegregationOfDutiesRequesterCannotApprove) {
			t.Fatalf("esperava ErrSegregationOfDutiesRequesterCannotApprove, veio %v", err)
		}
	})

	t.Run("aprovador não efetiva", func(t *testing.T) {
		repo := newFakeWithdrawalRepo()
		journal := NewJournalService(newFakeJournalRepo())
		seedEquity(t, journal, "tenant-1", 10000)
		svc := NewWithdrawalService(repo).WithJournalService(journal)
		w, err := svc.Request(context.Background(), "tenant-1", 1000, "saque", "requester-a13")
		if err != nil {
			t.Fatalf("Request: %v", err)
		}
		if _, err := svc.Approve(context.Background(), "tenant-1", w.ID, "approver-a13"); err != nil {
			t.Fatalf("aprovação por outro ator deveria passar: %v", err)
		}
		if _, err := svc.Settle(context.Background(), "tenant-1", w.ID, "approver-a13", "PIX-A13", ""); !errors.Is(err, ErrSegregationOfDutiesApproverCannotPay) {
			t.Fatalf("esperava ErrSegregationOfDutiesApproverCannotPay, veio %v", err)
		}
		if _, err := svc.Settle(context.Background(), "tenant-1", w.ID, "operator-2", "PIX-A13", ""); err != nil {
			t.Fatalf("outro operador deveria efetivar o saque: %v", err)
		}
	})

	t.Run("dispensa libera ator único", func(t *testing.T) {
		repo := newFakeWithdrawalRepo()
		journal := NewJournalService(newFakeJournalRepo())
		seedEquity(t, journal, "tenant-1", 10000)
		svc := NewWithdrawalService(repo).WithJournalService(journal)
		ctx := WithSeparationOfDutiesWaiver(context.Background())
		w, err := svc.Request(ctx, "tenant-1", 1000, "saque", "requester-a13")
		if err != nil {
			t.Fatalf("Request: %v", err)
		}
		if _, err := svc.Approve(ctx, "tenant-1", w.ID, "requester-a13"); err != nil {
			t.Fatalf("com dispensa o autor pode aprovar: %v", err)
		}
		if _, err := svc.Settle(ctx, "tenant-1", w.ID, "requester-a13", "PIX-A13", ""); err != nil {
			t.Fatalf("com dispensa o mesmo ator pode efetivar: %v", err)
		}
	})
}

// Campo vazio não cria conflito: nem todo registro carrega o autor (dados
// legados, imports, workers) e bloquear por ausência de dado pararia operação
// legítima — a decisão é não inventar conflito.
func TestSeparationOfDuties_EmptyActorsDoNotConflict(t *testing.T) {
	ctx := context.Background()
	if err := requireDistinctRequesterAndApprover(ctx, "", "operator"); err != nil {
		t.Fatalf("requested_by vazio não deveria bloquear: %v", err)
	}
	if err := requireDistinctApproverAndPayer(ctx, "", "operator"); err != nil {
		t.Fatalf("approved_by vazio não deveria bloquear: %v", err)
	}
	if err := requireDistinctRequesterAndApprover(ctx, "operator", ""); err != nil {
		t.Fatalf("ator vazio não deveria bloquear: %v", err)
	}
	if err := requireDistinctRequesterAndApprover(ctx, "operator", "operator"); err == nil {
		t.Fatal("mesmo ator deveria bloquear")
	}
}
