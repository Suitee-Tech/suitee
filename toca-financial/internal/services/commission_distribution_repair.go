package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/payout_order"
	"github.com/oginoino/toca/toca-financial/internal/domain/transaction"
)

// RepairCommissionDistributionInput descreve a correção de um rateio de
// comissão de venda já materializado.
//
// Contexto: o rateio é congelado na assinatura (plano + composição das
// cobranças + comissões/payouts). Alterar `broker_participations` depois disso
// não recalcula o que já existe. Quando o rateio materializado está errado (por
// exemplo `captador 100%` numa venda, sem retenção da imobiliária), a correção
// é: cancelar as obrigações erradas e materializar as corretas com o MESMO
// lastro de caixa (os recebimentos já pagos).
type RepairCommissionDistributionInput struct {
	TenantID   string
	DealID     string
	ContractID string
	/** Motivo registrado no audit trail do cancelamento. */
	Reason string
	/** Identidade estável da operação: garante idempotência do reparo. */
	EventID string
	/** Plano corrigido (resolvido a partir do rateio atual do negócio). */
	PlanHash string
	/** Fatias por corretor, já sem o papel de retenção (imobiliária). */
	Shares []ResolvedCommissionShare
	/** Comissão errada a cancelar (obrigatória). */
	CancelCommissionID string
	/** Payout da comissão errada, quando existir. */
	CancelPayoutOrderID string
	/** Quem executou o reparo (audit trail). */
	Actor string
	/** Rótulo dos lançamentos/payouts corrigidos. */
	Label string
	/** Descreve sem escrever nada. */
	DryRun bool
}

// RepairCommissionDistributionDeps reúne os serviços já configurados pela
// aplicação (mesmas instâncias do runtime, nunca cópias paralelas).
type RepairCommissionDistributionDeps struct {
	DB           *database.PostgresDB
	Commissions  *CommissionService
	Payouts      *PayoutOrderService
	Transactions *TransactionService
	Journal      *JournalService
}

// RepairCommissionDistributionResult é o relatório verificável do reparo.
type RepairCommissionDistributionResult struct {
	OriginalPercentage   float64
	OriginalAmountCents  int64
	CancelledCommission  string
	CancelledPayoutOrder string
	Shares               []RepairCommissionDistributionShare
}

// RepairCommissionDistributionShare é uma fatia corrigida materializada.
type RepairCommissionDistributionShare struct {
	BrokerID          string
	Role              string
	Percentage        float64
	AmountCents       int64
	CommissionID      string
	LedgerID          string
	PayoutOrderID     string
	FundingEntries    int
	FundedAmountCents int64
}

// RepairCommissionDistribution cancela as obrigações erradas e materializa as
// corretas com o mesmo lastro de caixa. Cada fatia recebe:
//   - lançamento no ledger (financial.transactions, tipo comissão) com o plano
//     corrigido;
//   - lançamento contábil (débito commission_expense / crédito
//     commission_payable);
//   - a obrigação (commission) com FundingRequiredCents = valor;
//   - o payout do corretor, com o funding coberto pelas MESMAS entradas de
//     caixa da obrigação cancelada, na proporção do novo percentual.
//
// Tudo roda em uma transação: uma falha não deixa metade do rateio aplicado.
func RepairCommissionDistribution(
	ctx context.Context,
	deps RepairCommissionDistributionDeps,
	in RepairCommissionDistributionInput,
) (*RepairCommissionDistributionResult, error) {
	if deps.Commissions == nil {
		return nil, errors.New("commission service is required")
	}
	if strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.DealID) == "" ||
		strings.TrimSpace(in.ContractID) == "" || strings.TrimSpace(in.CancelCommissionID) == "" {
		return nil, errors.New("tenant, deal, contract and commission to cancel are required")
	}
	if !in.DryRun && len(in.Shares) == 0 {
		return nil, errors.New("corrected shares are required")
	}
	original, err := deps.Commissions.GetByID(ctx, in.TenantID, in.CancelCommissionID)
	if err != nil {
		return nil, fmt.Errorf("load commission to cancel: %w", err)
	}
	if original == nil {
		return nil, errors.New("commission to cancel was not found")
	}
	if original.Status == commission.StatusPaid {
		return nil, errors.New("commission already paid cannot be redistributed; reverse it first")
	}

	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = "Rateio de comissão corrigido"
	}
	result := &RepairCommissionDistributionResult{
		OriginalPercentage:   original.Percentage,
		OriginalAmountCents:  int64(math.Round(original.Amount * 100)),
		CancelledCommission:  original.ID,
		CancelledPayoutOrder: in.CancelPayoutOrderID,
	}
	for _, share := range in.Shares {
		result.Shares = append(result.Shares, RepairCommissionDistributionShare{
			BrokerID:    share.BrokerID,
			Role:        share.Role,
			Percentage:  share.Percentage,
			AmountCents: share.AmountCents,
		})
	}
	if in.DryRun {
		return result, nil
	}

	work := func(txCtx context.Context) error {
		// 1. Cancela a obrigação errada. Comissão ligada a payout só pode ser
		// cancelada pelo próprio payout (lifecycle pertence ao agregado).
		if in.CancelPayoutOrderID != "" && deps.Payouts != nil {
			if _, err := deps.Payouts.CancelIdempotent(txCtx, in.TenantID, in.CancelPayoutOrderID, in.Actor, in.Reason); err != nil {
				return fmt.Errorf("cancel wrong payout order: %w", err)
			}
		}
		var cancelErr error
		if original.PayoutOrderID != "" {
			_, cancelErr = deps.Commissions.CancelFromPayout(txCtx, in.TenantID, original.ID, original.PayoutOrderID, in.Reason)
		} else {
			_, cancelErr = deps.Commissions.Cancel(txCtx, in.TenantID, original.ID, in.Reason)
		}
		if cancelErr != nil {
			return fmt.Errorf("cancel wrong commission: %w", cancelErr)
		}
		// O accrual da comissão errada precisa ser estornado junto: cancelar a
		// obrigação reverte o lançamento do razão, mas a perna contábil do
		// accrual foi gravada com outra identidade de origem e continuaria
		// POSTED — dobrando a despesa de comissão depois do reparo.
		if err := reverseCommissionAccrual(txCtx, deps, original, in.Reason, in.Actor); err != nil {
			return err
		}

		// 2. Materializa cada fatia corrigida.
		for i := range in.Shares {
			share := in.Shares[i]
			amountCents := share.AmountCents
			if amountCents <= 0 {
				return fmt.Errorf("share %s/%s has no amount", share.BrokerID, share.Role)
			}
			if err := materializeRepairedShare(txCtx, deps, in, original, share, label, &result.Shares[i]); err != nil {
				return err
			}
		}
		return nil
	}

	if database.TxFromCtx(ctx) != nil || deps.DB == nil {
		if err := work(ctx); err != nil {
			return nil, err
		}
		return result, nil
	}
	// RunInServiceContext (e não RunInTx): as escritas privilegiadas do razão —
	// `financial.journal_entries` só tem policy de SELECT — passam pelo pool de
	// serviço e precisam da MESMA transação para o estorno do accrual gravar a
	// reversão e a entrada compensatória juntas (constraint do banco).
	if err := deps.DB.RunInServiceContext(ctx, work); err != nil {
		return nil, err
	}
	return result, nil
}

func materializeRepairedShare(
	ctx context.Context,
	deps RepairCommissionDistributionDeps,
	in RepairCommissionDistributionInput,
	original *commission.Commission,
	share ResolvedCommissionShare,
	label string,
	out *RepairCommissionDistributionShare,
) error {
	role := strings.ToLower(strings.TrimSpace(share.Role))
	brokerID := strings.TrimSpace(share.BrokerID)
	if brokerID == "" || role == "" {
		return fmt.Errorf("corrected share requires broker and role")
	}
	amount := float64(share.AmountCents) / 100.0

	// Lançamento no ledger: é o espelho da obrigação e a base do accrual.
	ledgerTx := transaction.New(
		in.ContractID, transaction.TypeComissao,
		fmt.Sprintf("%s - %s (%.1f%%)", label, role, share.Percentage),
		amount, time.Now().UTC(), in.TenantID,
	)
	ledgerTx.Category = "Comissão"
	ledgerTx.BrokerID = brokerID
	ledgerTx.Role = role
	ledgerTx.PlanHash = in.PlanHash
	if deps.Transactions != nil {
		sourceID := events.DeriveNotificationID(in.EventID, "commission_tx", in.ContractID, brokerID, role)
		if err := deps.Transactions.CreateWithSourceEventID(ctx, sourceID, ledgerTx); err != nil {
			return fmt.Errorf("create corrected commission ledger row for %s/%s: %w", brokerID, role, err)
		}
		out.LedgerID = ledgerTx.ID
	}

	// Accrual contábil: a imobiliária incorre a despesa e deve ao corretor.
	if deps.Journal != nil {
		entry, err := journal.New(in.TenantID, in.ContractID,
			fmt.Sprintf("%s - %s (%.1f%%)", label, role, share.Percentage),
			time.Now().UTC(),
			[]journal.Leg{
				{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: share.AmountCents},
				{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: share.AmountCents},
			},
		)
		if err != nil {
			return fmt.Errorf("build corrected commission journal entry: %w", err)
		}
		// Mesma identidade usada por TransactionService ao reverter um lançamento
		// de comissão: um cancelamento/estorno futuro precisa encontrar esta
		// entrada para estornar o accrual.
		sourceID := events.DeriveNotificationID("transaction.commission.accrual", in.TenantID, ledgerTx.ID)
		if err := deps.Journal.CreateWithSourceEventID(ctx, sourceID, entry); err != nil {
			return fmt.Errorf("create corrected commission journal entry: %w", err)
		}
	}

	// Obrigação, com o plano corrigido e o lastro das mesmas entradas de caixa.
	funding := scaleFundingEntries(original, share.Percentage)
	obligation, err := deps.Commissions.createWithRoleFromEventAndLedger(
		ctx,
		events.DeriveNotificationID(in.EventID, "commission", in.ContractID, brokerID, role),
		in.DealID, brokerID, in.ContractID, role, share.Percentage, amount, in.TenantID,
		original.OwnerID, original.CustomerID, original.PropertyID,
		in.PlanHash, out.LedgerID, share.AmountCents, in.Actor,
	)
	if err != nil {
		return fmt.Errorf("create corrected commission for %s/%s: %w", brokerID, role, err)
	}
	out.CommissionID = obligation.ID

	for _, fundingEntry := range funding {
		if _, err := deps.Commissions.RecordRentalFunding(ctx, in.TenantID, obligation.ID, fundingEntry); err != nil {
			return fmt.Errorf("record corrected commission funding for %s/%s: %w", brokerID, role, err)
		}
		out.FundingEntries++
		out.FundedAmountCents += fundingEntry.AmountCents
	}

	if deps.Payouts == nil || deps.DB == nil {
		return nil
	}
	order, err := payout_order.New(
		in.TenantID, payout_order.SourceManual, in.ContractID,
		payout_order.RecipientBroker, brokerID,
		fmt.Sprintf("%s - %s (%.1f%%)", label, role, share.Percentage),
		share.AmountCents, in.Actor,
	)
	if err != nil {
		return fmt.Errorf("build corrected payout order: %w", err)
	}
	order.SourceEventID = events.DeriveNotificationID(in.EventID, "commission_payout", in.ContractID, brokerID, role)
	order.SourceRef = map[string]any{
		"deal_id": in.DealID, "charge_type": "SALE", "split_pct": share.Percentage,
		"trigger": "commission.repair", "plan_hash": in.PlanHash,
	}
	if err := deps.Payouts.Create(ctx, order); err != nil {
		return fmt.Errorf("create corrected payout order for %s/%s: %w", brokerID, role, err)
	}
	// O CAS de transição compara a coluna `updated_at`, que o INSERT preenche
	// com now() do banco: relê o agregado antes de alterar o gate/funding.
	persisted, err := deps.Payouts.GetByID(ctx, in.TenantID, order.ID)
	if err != nil {
		return fmt.Errorf("reload corrected payout order: %w", err)
	}
	if persisted == nil {
		return fmt.Errorf("corrected payout order %s not found after create", order.ID)
	}
	order = persisted
	expectedStatus, expectedUpdatedAt := order.Status, order.UpdatedAt
	if _, err := order.EnableFundingGate(share.AmountCents); err != nil {
		return fmt.Errorf("configure corrected payout gate: %w", err)
	}
	for _, fundingEntry := range funding {
		if _, err := order.RecordFunding(payout_order.FundingEntry{
			PaymentID:      fundingEntry.PaymentID,
			ChargeID:       fundingEntry.ChargeID,
			ReferenceMonth: fundingEntry.ReferenceMonth,
			AmountCents:    fundingEntry.AmountCents,
			FundedAt:       fundingEntry.FundedAt,
		}); err != nil {
			return fmt.Errorf("record corrected payout funding for %s/%s: %w", brokerID, role, err)
		}
	}
	if err := deps.Payouts.persistTransition(ctx, order, expectedStatus, expectedUpdatedAt); err != nil {
		return fmt.Errorf("persist corrected payout order: %w", err)
	}
	if _, err := deps.Commissions.LinkPayoutOrder(ctx, in.TenantID, obligation.ID, order.ID); err != nil {
		return fmt.Errorf("link corrected payout to commission: %w", err)
	}
	out.PayoutOrderID = order.ID
	return nil
}

// ReverseCancelledCommissionAccrual estorna o accrual de uma comissão já
// cancelada. Existe para corrigir cancelamentos feitos antes deste reparo (ou
// por outra via): sem isso a despesa de comissão daquela fatia continuaria
// POSTED e o resultado do período ficaria subestimado.
func ReverseCancelledCommissionAccrual(
	ctx context.Context,
	deps RepairCommissionDistributionDeps,
	tenantID, commissionID, reason, actor string,
) error {
	if deps.Commissions == nil || deps.Journal == nil {
		return errors.New("commission and journal services are required")
	}
	c, err := deps.Commissions.GetByID(ctx, tenantID, commissionID)
	if err != nil {
		return err
	}
	if c == nil {
		return errors.New("commission not found")
	}
	if c.Status != commission.StatusCancelled && c.Status != commission.StatusReversed {
		return fmt.Errorf("commission %s is %s; only cancelled/reversed commissions are eligible", c.ID, c.Status)
	}
	return reverseCommissionAccrual(ctx, deps, c, reason, actor)
}

// reverseCommissionAccrual estorna a perna contábil de accrual da comissão
// cancelada. A busca é restrita ao contrato, ao valor e ao papel: são os dados
// que identificam o accrual daquela fatia sem depender de qual evento o
// materializou.
func reverseCommissionAccrual(
	ctx context.Context,
	deps RepairCommissionDistributionDeps,
	original *commission.Commission,
	reason, actor string,
) error {
	if deps.Journal == nil || original == nil {
		return nil
	}
	entries, _, err := deps.Journal.List(ctx, original.TenantID, original.ContractID, 500, 0)
	if err != nil {
		return fmt.Errorf("list journal entries for accrual reversal: %w", err)
	}
	amountCents := int64(math.Round(original.Amount * 100))
	role := strings.ToLower(strings.TrimSpace(original.Role))
	for _, entry := range entries {
		if entry == nil || entry.Status != journal.StatusPosted || entry.ReversalOf != "" {
			continue
		}
		if amountCents <= 0 || !strings.Contains(strings.ToLower(entry.Description), role) {
			continue
		}
		if !hasCommissionAccrualLeg(entry.Legs, amountCents) {
			continue
		}
		if _, err := deps.Journal.Reverse(ctx, entry.ID, original.TenantID,
			"Rateio corrigido: "+reason, actor); err != nil {
			return fmt.Errorf("reverse commission accrual %s: %w", entry.ID, err)
		}
		return nil
	}
	return nil
}

func hasCommissionAccrualLeg(legs []journal.Leg, amountCents int64) bool {
	for _, leg := range legs {
		if leg.Account == journal.AccountCommissionExpense && leg.Direction == journal.Debit && leg.AmountCents == amountCents {
			return true
		}
	}
	return false
}

// scaleFundingEntries reaplica o lastro de caixa já registrado na obrigação
// original à nova fatia, na razão entre os percentuais. Os valores originais
// vêm dos recebimentos que pagaram a venda; a fatia nova é a parte daquele
// caixa que passa a cobrir o novo rateio.
func scaleFundingEntries(original *commission.Commission, targetPct float64) []commission.FundingEntry {
	if original == nil || len(original.FundingEntries) == 0 || original.Percentage <= 0 {
		return nil
	}
	out := make([]commission.FundingEntry, 0, len(original.FundingEntries))
	for _, entry := range original.FundingEntries {
		scaled := int64(math.Round(float64(entry.AmountCents) * targetPct / original.Percentage))
		if scaled <= 0 {
			continue
		}
		out = append(out, commission.FundingEntry{
			PaymentID:      entry.PaymentID,
			ChargeID:       entry.ChargeID,
			ReferenceMonth: entry.ReferenceMonth,
			AmountCents:    scaled,
			FundedAt:       entry.FundedAt,
		})
	}
	return out
}
