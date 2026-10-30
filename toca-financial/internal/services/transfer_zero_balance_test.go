package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Repasse zerado por ausência de saldo: a baixa do operador encerra a obrigação
// sem lançamento de caixa, sem comprovante e sem exigir aprovação prévia — e
// sem anunciar um pagamento de R$ 0,00 ao proprietário.
func TestTransferService_ProcessZeroTransferSettlesWithoutCashOrEvent(t *testing.T) {
	ctx := context.Background()
	owner := transfer.New("contract-1", "owner-lead-1", "prop-1", "tenant-A", "2026-10",
		1000, 100, 0, 0, nil, "")
	require.True(t, owner.HasNoPayableBalance(), "fixture deve ter líquido zerado")

	entries := make([]audit.Entry, 0, 1)
	cleanup := audit.SetTestSink(func(entry audit.Entry) { entries = append(entries, entry) })
	t.Cleanup(cleanup)

	repo := newCaptureRepo(owner)
	bus := &captureBus{}
	svc := NewTransferService(repo).WithEventBus(bus)

	require.NoError(t, svc.ProcessWithEvidence(ctx, "tenant-A", owner.ID, "op-9", "Operadora Nove", "", "sem saldo na competência"))

	assert.Equal(t, transfer.StatusProcessed, owner.Status)
	assert.True(t, owner.SettledWithoutBalance)
	assert.Equal(t, "op-9", owner.ProcessedBy)
	assert.Nil(t, owner.ApprovedAt, "conclusão sem saldo não passa por aprovação")
	assert.Empty(t, owner.SettlementJournalSourceID, "não há perna de caixa a lançar")
	assert.Equal(t, "sem saldo na competência", owner.Notes)
	require.NotEmpty(t, owner.History)
	assert.Equal(t, "processed", owner.History[len(owner.History)-1].Action)
	assert.Empty(t, bus.get(), "nenhum pagamento foi feito: não há transfer.processed a publicar")

	require.Len(t, entries, 1)
	assert.Equal(t, "transfer.processed", entries[0].Action)
	assert.Equal(t, true, entries[0].After["settled_without_balance"])
	assert.Equal(t, "no_payable_balance", entries[0].Metadata["reason"])

	// Reentrega (operador clicou duas vezes / retry de rede) é sucesso
	// idempotente e não duplica histórico, audit ou evento.
	historyBefore := len(owner.History)
	require.NoError(t, svc.ProcessWithEvidence(ctx, "tenant-A", owner.ID, "op-9", "Operadora Nove", "", ""))
	assert.Len(t, owner.History, historyBefore)
	assert.Len(t, entries, 1)
	assert.Empty(t, bus.get())
}

func TestTransferService_ProcessZeroTransferAfterApprovalKeepsHistory(t *testing.T) {
	ctx := context.Background()
	owner := transfer.New("contract-2", "owner-lead-2", "prop-2", "tenant-A", "2026-10",
		1000, 100, 0, 0, nil, "")
	require.NoError(t, owner.Approve("approver-1", "Aprovadora"))

	svc := NewTransferService(newCaptureRepo(owner))
	require.NoError(t, svc.ProcessWithEvidence(ctx, "tenant-A", owner.ID, "op-9", "Operadora Nove", "", ""))

	assert.Equal(t, transfer.StatusProcessed, owner.Status)
	assert.True(t, owner.SettledWithoutBalance)
	assert.NotNil(t, owner.ApprovedAt, "a aprovação anterior permanece no registro")
	actions := make([]string, 0, len(owner.History))
	for _, entry := range owner.History {
		actions = append(actions, entry.Action)
	}
	assert.Equal(t, []string{"created", "approved", "processed"}, actions)
}

func TestTransferService_ProcessZeroTransferCancelledIsConflict(t *testing.T) {
	ctx := context.Background()
	owner := transfer.New("contract-3", "owner-lead-3", "prop-3", "tenant-A", "2026-10",
		1000, 100, 0, 0, nil, "")
	require.NoError(t, owner.Cancel("op-1", "Operadora", "competência errada"))

	svc := NewTransferService(newCaptureRepo(owner))
	err := svc.ProcessWithEvidence(ctx, "tenant-A", owner.ID, "op-9", "Operadora Nove", "", "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTransferWithoutPayableBalanceNotOpen), "esperado conflito de estado, got %v", err)
	assert.Equal(t, transfer.StatusCancelled, owner.Status)
}

// O reparo operacional só toca repasses que já estão sem nada a repassar.
func TestTransferService_SettleDanglingWithoutPayableBalance(t *testing.T) {
	ctx := context.Background()
	zero := transfer.New("contract-1", "owner-1", "prop-1", "tenant-A", "2026-10", 1000, 100, 0, 0, nil, "")
	withBalance := transfer.New("contract-2", "owner-1", "prop-2", "tenant-A", "2026-10", 1000, 10, 0, 0, nil, "")
	already := transfer.New("contract-3", "owner-1", "prop-3", "tenant-A", "2026-10", 1000, 100, 0, 0, nil, "")
	require.True(t, already.SettleIfNoPayableBalance(time.Now()))
	cancelled := transfer.New("contract-4", "owner-1", "prop-4", "tenant-A", "2026-10", 1000, 100, 0, 0, nil, "")
	require.NoError(t, cancelled.Cancel("op-1", "Operadora", "duplicado"))

	svc := NewTransferService(newCaptureRepo(zero, withBalance, already, cancelled))

	settled, err := svc.SettleDanglingWithoutPayableBalance(ctx, "tenant-A", zero.ID, "op-9", "Operadora Nove")
	require.NoError(t, err)
	assert.True(t, settled)
	assert.Equal(t, transfer.StatusProcessed, zero.Status)
	assert.True(t, zero.SettledWithoutBalance)
	assert.Equal(t, "op-9", zero.ProcessedBy)

	settled, err = svc.SettleDanglingWithoutPayableBalance(ctx, "tenant-A", already.ID, "op-9", "Operadora Nove")
	require.NoError(t, err)
	assert.False(t, settled, "repasse já encerrado não é tocado de novo")

	settled, err = svc.SettleDanglingWithoutPayableBalance(ctx, "tenant-A", withBalance.ID, "op-9", "Operadora Nove")
	require.NoError(t, err)
	assert.False(t, settled, "repasse com valor a repassar não é encerrado")
	assert.Equal(t, transfer.StatusPending, withBalance.Status)

	settled, err = svc.SettleDanglingWithoutPayableBalance(ctx, "tenant-A", cancelled.ID, "op-9", "Operadora Nove")
	require.Error(t, err)
	assert.False(t, settled)
	assert.Equal(t, transfer.StatusCancelled, cancelled.Status)
}

// A cobrança paga que originou um repasse zerado pode chegar com composição
// diferente (nova taxa, novo item): o repasse encerrado sem saldo é adotado em
// vez de derrubar o recebimento.
func TestReconcilePaidChargeAdoptsTransferSettledWithoutBalance(t *testing.T) {
	item := transfer.New("contract-1", "owner-1", "prop-1", "tenant-1", "2026-10", 0, 0, 0, 0, nil, "")
	require.True(t, item.SettleIfNoPayableBalance(time.Now()))
	require.True(t, item.SettledWithoutBalance)

	paidCharge := &charge.Charge{
		ID: "charge-1", TenantID: "tenant-1", ContractID: "contract-1",
		OwnerID: "owner-1", PropertyID: "prop-1", ReferenceMonth: "2026-10",
	}
	composition := paidChargeTransferComposition{
		grossRentCents: 270969, adminFeePercent: 10, adminFeeCents: 27097,
		ownerPayableCents: 27095, ownerReservedCents: 27095, allocationAware: true,
	}

	changed, err := reconcileTransferForPaidCharge(item, paidCharge, composition, true)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.False(t, item.SettledWithoutBalance, "a composição real substitui o encerramento sem saldo")
	assert.Equal(t, transfer.StatusPending, item.Status)
	assert.Nil(t, item.ProcessedAt)
	assert.Empty(t, item.ProcessedBy)
	assert.Equal(t, "charge-1", item.SourceChargeID)
	assert.Equal(t, 2709.69, item.GrossRent)
	assert.Equal(t, int64(27095), item.OwnerBalance.ReservedCents)
	assert.Equal(t, "reopened", item.History[len(item.History)-1].Action)
}

func TestReconcilePaidChargeStillRejectsPaidTransferWithDivergentComposition(t *testing.T) {
	item := transfer.New("contract-1", "owner-1", "prop-1", "tenant-1", "2026-10", 0, 0, 0, 0, nil, "")
	item.Status = transfer.StatusProcessed
	item.SettlementJournalSourceID = "owner_transfer.settlement:algum"

	paidCharge := &charge.Charge{
		ID: "charge-1", TenantID: "tenant-1", ContractID: "contract-1",
		OwnerID: "owner-1", PropertyID: "prop-1", ReferenceMonth: "2026-10",
	}
	composition := paidChargeTransferComposition{
		grossRentCents: 270969, adminFeePercent: 10, adminFeeCents: 27097,
		ownerPayableCents: 27095, ownerReservedCents: 27095, allocationAware: true,
	}

	_, err := reconcileTransferForPaidCharge(item, paidCharge, composition, true)
	require.Error(t, err)
	assert.Equal(t, transfer.StatusProcessed, item.Status)
	assert.NotEmpty(t, item.SettlementJournalSourceID)
}

// Dedução pelo serviço (imposto retido, manutenção, consumo): quando ela
// consome todo o líquido, o repasse encerra na mesma transação em vez de ficar
// zerado e pendente para sempre.
func TestTransferService_ApplyDeductionClosesZeroTransfer(t *testing.T) {
	ctx := context.Background()
	item := transfer.New("contract-1", "owner-1", "prop-1", "tenant-A", "2026-10", 1000, 10, 0, 0, nil, "")
	require.Equal(t, 900.0, item.NetAmount)
	repo := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{item}}
	svc := NewTransferService(repo)

	require.NoError(t, svc.ApplyDeductionDetailed(ctx, "tenant-A", "contract-1", "2026-10", transfer.Deduction{
		Description: "Imposto retido", Amount: 900, Type: "TAX", ReferenceID: "tax-ref-1",
	}))

	stored, err := repo.GetByID(ctx, "tenant-A", item.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, 0.0, stored.NetAmount)
	assert.Equal(t, transfer.StatusProcessed, stored.Status)
	assert.True(t, stored.SettledWithoutBalance)
	assert.Empty(t, stored.SettlementJournalSourceID)
}

// Dedução parcial mantém o repasse aberto para o saldo restante.
func TestTransferService_ApplyDeductionKeepsPartialTransferPending(t *testing.T) {
	ctx := context.Background()
	item := transfer.New("contract-2", "owner-2", "prop-2", "tenant-A", "2026-10", 1000, 10, 0, 0, nil, "")
	repo := &chargeIdentityTransferRepo{items: []*transfer.OwnerTransfer{item}}
	svc := NewTransferService(repo)

	require.NoError(t, svc.ApplyDeductionDetailed(ctx, "tenant-A", "contract-2", "2026-10", transfer.Deduction{
		Description: "Imposto retido", Amount: 300, Type: "TAX", ReferenceID: "tax-ref-2",
	}))

	stored, err := repo.GetByID(ctx, "tenant-A", item.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, 600.0, stored.NetAmount)
	assert.Equal(t, transfer.StatusPending, stored.Status)
	assert.False(t, stored.SettledWithoutBalance)
}
