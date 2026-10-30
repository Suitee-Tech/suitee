package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

// RentalAllocationRepairResult resume o reparo de alocação de uma cobrança de
// locação quitada antes de o plano de intermediação existir.
type RentalAllocationRepairResult struct {
	ChargeID                  string   `json:"charge_id"`
	RepairedPayments          []string `json:"repaired_payments"`
	IntermediationFundedCents int64    `json:"intermediation_funded_cents"`
	JournalEntriesReversed    int      `json:"journal_entries_reversed"`
	JournalEntriesReposted    int      `json:"journal_entries_reposted"`
	TransferReconciled        bool     `json:"transfer_reconciled"`
	SkippedReason             string   `json:"skipped_reason,omitempty"`
	// Warnings registra anomalias não fatais (ex.: conflito de identidade de
	// managed bill passthrough por data civil) que não impedem a correção.
	Warnings []string `json:"warnings,omitempty"`
}

// paymentMissingIntermediation identifica o pagamento que foi alocado por uma
// política antiga: tem alocações (owner_rent/taxa), mas nenhuma fatia de
// intermediação. Pagamentos SEM alocação não entram aqui — esses são cobertos
// pelo reparo histórico (`repairUnallocatedPayments`).
func paymentMissingIntermediation(p *charge.PaymentEntry) bool {
	if p == nil || p.AmountCents <= 0 || len(p.Allocations) == 0 {
		return false
	}
	for _, a := range p.Allocations {
		if a.Purpose == charge.AllocationPurposeRentalIntermediationBroker ||
			a.Purpose == charge.AllocationPurposeRentalIntermediationAgency {
			return false
		}
	}
	return true
}

// intermediationAllocatedCents soma a fatia de intermediação de um pagamento.
func intermediationAllocatedCents(p *charge.PaymentEntry) int64 {
	total := int64(0)
	for _, a := range p.Allocations {
		if a.Purpose == charge.AllocationPurposeRentalIntermediationBroker ||
			a.Purpose == charge.AllocationPurposeRentalIntermediationAgency {
			total += a.AmountCents
		}
	}
	return total
}

// journalCarriesIntermediationLeg informa se o lançamento já reconhece a
// intermediação (crédito em comissão a pagar). O lançamento original de um
// recebimento alocado ANTES do plano existir não tem essa perna — é o sinal de
// que precisa ser estornado e regravado.
func journalCarriesIntermediationLeg(entry *journal.Entry) bool {
	if entry == nil {
		return false
	}
	for _, leg := range entry.Legs {
		if leg.Account == journal.AccountCommissionPayable && leg.Direction == journal.Credit && leg.AmountCents > 0 {
			return true
		}
	}
	return false
}

// RentalAllocationRepairEligibility responde, sem efeito colateral, se há algo
// a reparar numa cobrança de locação — é o gate do botão no painel (evita
// oferecer a ação em cobrança que já está correta).
type RentalAllocationRepairEligibility struct {
	ChargeID         string `json:"charge_id"`
	Eligible         bool   `json:"eligible"`
	Reason           string `json:"reason,omitempty"`
	PendingPayments  int    `json:"pending_payments"`
	OutstandingCents int64  `json:"outstanding_cents"`
	PlannedCents     int64  `json:"planned_cents"`
	FundedCents      int64  `json:"funded_cents"`
}

// CheckRentalIntermediationRepairEligibility devolve eligible=true apenas
// quando a ação mudaria algo: cobrança de locação quitada, contrato com plano
// de intermediação congelado e ainda com saldo a financiar, e pelo menos um
// pagamento alocado sem a fatia de intermediação.
func (s *ChargeService) CheckRentalIntermediationRepairEligibility(ctx context.Context, tenantID, chargeID string) (*RentalAllocationRepairEligibility, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("charge service is not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	chargeID = strings.TrimSpace(chargeID)
	if tenantID == "" || chargeID == "" {
		return nil, errors.New("tenant_id and charge_id are required")
	}

	loaded, err := s.repo.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("charge %s not found", chargeID)
	}

	result := &RentalAllocationRepairEligibility{ChargeID: chargeID}
	if loaded.Type != charge.TypeRent {
		result.Reason = "cobrança não é de locação"
		return result, nil
	}
	if loaded.Status != charge.StatusPaid {
		result.Reason = "cobrança ainda não foi quitada"
		return result, nil
	}
	if s.rentalConfigRepo == nil {
		result.Reason = "configuração de locação indisponível"
		return result, nil
	}
	cfg, err := s.rentalConfigRepo.GetByContractID(ctx, tenantID, loaded.ContractID)
	if err != nil {
		return nil, fmt.Errorf("load rental charge config: %w", err)
	}
	if cfg == nil || cfg.IntermediationPlan == nil || cfg.IntermediationPlan.TotalAmountCents <= 0 {
		result.Reason = "contrato sem plano de intermediação congelado"
		return result, nil
	}
	result.PlannedCents = cfg.IntermediationPlan.TotalAmountCents
	result.FundedCents = cfg.IntermediationPlan.TotalAmountCents - cfg.IntermediationOutstandingCents()
	result.OutstandingCents = cfg.IntermediationOutstandingCents()
	if result.OutstandingCents <= 0 {
		result.Reason = "intermediação já integralmente financiada"
		return result, nil
	}

	pending := 0
	for i := range loaded.Payments {
		if paymentMissingIntermediation(&loaded.Payments[i]) {
			pending++
		}
	}
	result.PendingPayments = pending
	if pending == 0 {
		result.Reason = "pagamentos já contemplam a intermediação"
		return result, nil
	}
	result.Eligible = true
	return result, nil
}

// RepairRentalIntermediationAllocations re-deriva as alocações de uma cobrança
// de locação JÁ QUITADA cujos pagamentos foram alocados antes de o plano de
// intermediação ser congelado (cenário do contrato assinado cujo evento
// `deal.contract.signed` chegou corrompido: sem Rental/plano, a baixa alocou
// só `owner_rent` + taxa de administração; o repasse ao proprietário saiu com
// residual cheio e a comissão/payout ficaram sem funding).
//
// Operação idempotente e segura para retry:
//  1. re-deriva as alocações dos pagamentos que não têm a fatia de
//     intermediação (financia comissão + payout pelo plano congelado);
//  2. estorna o lançamento de recebimento antigo (sem a perna de comissão) e
//     grava o lançamento corrigido com chave de idempotência própria;
//  3. reconcilia o repasse do proprietário com a nova composição.
func (s *ChargeService) RepairRentalIntermediationAllocations(ctx context.Context, tenantID, chargeID, operatorID, reason string) (*RentalAllocationRepairResult, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("charge service is not configured")
	}
	tenantID = strings.TrimSpace(tenantID)
	chargeID = strings.TrimSpace(chargeID)
	if tenantID == "" || chargeID == "" {
		return nil, errors.New("tenant_id and charge_id are required")
	}
	if strings.TrimSpace(reason) == "" {
		return nil, errors.New("repair reason is required")
	}
	if s.rentalConfigRepo == nil || s.allocationResolver == nil {
		return nil, errors.New("rental charge config and allocation resolver are required for the repair")
	}

	loaded, err := s.repo.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("charge %s not found", chargeID)
	}
	if loaded.Status != charge.StatusPaid {
		return nil, fmt.Errorf("charge %s is not paid (status %s); repair only applies to settled charges", chargeID, loaded.Status)
	}
	if loaded.Type != charge.TypeRent {
		return nil, fmt.Errorf("charge %s is not a rent charge", chargeID)
	}

	cfg, err := s.rentalConfigRepo.GetByContractID(ctx, tenantID, loaded.ContractID)
	if err != nil {
		return nil, fmt.Errorf("load rental charge config: %w", err)
	}
	if cfg == nil {
		return nil, fmt.Errorf("rental charge config not found for contract %s", loaded.ContractID)
	}
	if cfg.IntermediationPlan == nil || cfg.IntermediationPlan.TotalAmountCents <= 0 {
		return &RentalAllocationRepairResult{ChargeID: chargeID, SkippedReason: "contrato sem plano de intermediação congelado"}, nil
	}
	if cfg.IntermediationOutstandingCents() <= 0 {
		return &RentalAllocationRepairResult{ChargeID: chargeID, SkippedReason: "plano de intermediação já integralmente financiado"}, nil
	}

	result := &RentalAllocationRepairResult{ChargeID: chargeID, RepairedPayments: []string{}}

	// ---- Fase 1: re-deriva as alocações e persiste (transação única) ----
	runRebuild := func(txCtx context.Context) error {
		current := loaded
		expectedVersion := current.UpdatedAt
		if locking, ok := s.repo.(chargeLockingRepository); ok {
			locked, lockErr := locking.GetByIDForUpdate(txCtx, tenantID, chargeID)
			if lockErr != nil {
				return lockErr
			}
			if locked != nil {
				current = locked
				expectedVersion = locked.UpdatedAt
			}
		}

		rebuilt := false
		for i := range current.Payments {
			payment := &current.Payments[i]
			if !paymentMissingIntermediation(payment) {
				continue
			}
			// Snapshot sem as alocações DESTE pagamento: o builder distribui o
			// valor do pagamento pelo saldo remanescente dos itens e contaria
			// duas vezes se mantivéssemos as alocações antigas no snapshot.
			snapshot := *current
			snapshot.Payments = append([]charge.PaymentEntry(nil), current.Payments...)
			snapshot.Payments[i].Allocations = nil
			snapshot.Payments[i].AllocationMethod = ""

			allocMethod, allocations, rebuildErr := s.rebuildAllocationsForPayment(txCtx, &snapshot, payment)
			if rebuildErr != nil {
				return fmt.Errorf("rebuild allocations for payment %s: %w", payment.ID, rebuildErr)
			}
			if len(allocations) == 0 {
				continue
			}
			payment.Allocations = allocations
			payment.AllocationMethod = allocMethod
			result.RepairedPayments = append(result.RepairedPayments, payment.ID)
			result.IntermediationFundedCents += intermediationAllocatedCents(payment)
			rebuilt = true
		}
		if !rebuilt {
			result.SkippedReason = "nenhum pagamento sem intermediação para reparar"
			return nil
		}
		if err := s.persistRepairedCharge(txCtx, current, expectedVersion); err != nil {
			return err
		}
		return nil
	}
	// Sem DB wired (testes unitários) o callback roda direto; em produção a
	// transação é obrigatória porque o funding da intermediação exige
	// TxFromCtx (RentalIntermediationService.RequirePaymentTransaction).
	if s.db != nil {
		err = s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			return runRebuild(database.WithTx(ctx, tx))
		})
	} else {
		err = runRebuild(ctx)
	}
	if err != nil {
		return nil, err
	}
	// ---- Fase 2: ledger e repasse (idempotentes) ----
	// Roda mesmo quando a fase 1 não mudou nada: uma execução anterior pode ter
	// commitado as alocações e falhado no ledger/repasse (retry precisa
	// completar o que faltou).
	persisted, err := s.repo.GetByID(ctx, tenantID, chargeID)
	if err != nil {
		return nil, err
	}
	if persisted == nil {
		return nil, fmt.Errorf("charge %s vanished after repair", chargeID)
	}
	runJournalRepair := func(journalCtx context.Context) error {
		for i := range persisted.Payments {
			payment := &persisted.Payments[i]
			if payment.AmountCents <= 0 || intermediationAllocatedCents(payment) <= 0 {
				continue
			}
			reversed, reposted, journalErr := s.repairAllocatedPaymentJournal(journalCtx, tenantID, persisted, payment, operatorID, reason)
			if journalErr != nil {
				return journalErr
			}
			result.JournalEntriesReversed += reversed
			result.JournalEntriesReposted += reposted
		}
		return nil
	}
	if s.paymentJournal != nil {
		// O ledger é append-only e só aceita escrita pelo pool de SERVIÇO: uma
		// transação do pool de aplicação é ignorada de propósito pelo repositório
		// e o estorno commitaria sem o lançamento compensatório (SQLSTATE 23514).
		if s.db != nil {
			if err := s.db.RunInServiceContext(ctx, runJournalRepair); err != nil {
				return nil, err
			}
		} else if err := runJournalRepair(ctx); err != nil {
			return nil, err
		}
	}
	if s.transferHook != nil {
		if err := s.transferHook.EnsureMonthlyForChargePaid(ctx, persisted); err != nil {
			// O hook reconcilia o repasse ANTES de re-afirmar os managed bills
			// dos itens passthrough. Quando esses bills já existem com a MESMA
			// composição, a comparação de identidade pode divergir apenas na
			// data civil (coluna DATE interpretada em UTC vs due_date do
			// metadata) — os valores do proprietário continuam corretos e o
			// conflito não pode derrubar uma correção já aplicada. Qualquer
			// outro erro segue fatal.
			if !isBenignManagedBillIdentityConflict(err) {
				return nil, fmt.Errorf("reconcile owner transfer: %w", err)
			}
			result.Warnings = append(result.Warnings,
				"managed bill passthrough preservado (conflito de identidade por data civil): "+err.Error())
		}
		result.TransferReconciled = true
	}
	if len(result.RepairedPayments) == 0 && result.JournalEntriesReversed == 0 && result.JournalEntriesReposted == 0 {
		result.SkippedReason = "nada a reparar (alocações, ledger e repasse já reconciliados)"
	}
	return result, nil
}

// isBenignManagedBillIdentityConflict reconhece o conflito de re-afirmação de
// um managed bill já existente (mesma composição econômica), disparado apenas
// pela normalização de data civil entre a coluna DATE e o due_date do metadata.
func isBenignManagedBillIdentityConflict(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "managed bill") && strings.Contains(msg, "different economic data")
}

// persistRepairedCharge grava a cobrança corrigida respeitando o CAS do
// repositório versionado (mesma proteção dos pagamentos concorrentes).
func (s *ChargeService) persistRepairedCharge(ctx context.Context, c *charge.Charge, expectedVersion time.Time) error {
	if versioned, ok := s.repo.(chargeVersionedRepository); ok {
		updated, err := versioned.UpdateIfVersion(ctx, c, expectedVersion)
		if err != nil {
			return err
		}
		if !updated {
			return charge.ErrConcurrentModification
		}
		return nil
	}
	return s.repo.Update(ctx, c)
}

// repairAllocatedPaymentJournal estorna o lançamento de recebimento que não
// reconhece a intermediação e grava o lançamento corrigido. A chave de
// idempotência do relançamento é própria do reparo, então retries colapsam.
func (s *ChargeService) repairAllocatedPaymentJournal(ctx context.Context, tenantID string, c *charge.Charge, payment *charge.PaymentEntry, operatorID, reason string) (int, int, error) {
	if s.paymentJournal == nil {
		return 0, 0, nil
	}
	reversed, reposted := 0, 0

	originalSourceID := events.DeriveNotificationID("charge.payment.journal", tenantID, c.ID, payment.ID)
	original, err := s.paymentJournal.GetBySourceEventID(ctx, tenantID, originalSourceID)
	if err != nil {
		return 0, 0, fmt.Errorf("load receipt journal for payment %s: %w", payment.ID, err)
	}
	if original != nil && original.Status == journal.StatusPosted && !journalCarriesIntermediationLeg(original) {
		if _, err := s.paymentJournal.Reverse(ctx, original.ID, tenantID, reason, operatorID); err != nil {
			return 0, 0, fmt.Errorf("reverse stale receipt journal %s: %w", original.ID, err)
		}
		reversed++
	}

	repairSourceID := events.DeriveNotificationID("charge.payment.journal.repair", tenantID, c.ID, payment.ID)
	alreadyReposted, err := s.paymentJournal.GetBySourceEventID(ctx, tenantID, repairSourceID)
	if err != nil {
		return 0, 0, fmt.Errorf("load repair journal for payment %s: %w", payment.ID, err)
	}
	if alreadyReposted != nil {
		return reversed, reposted, nil
	}
	entry, err := newAllocatedPaymentJournalEntry(c, payment)
	if err != nil {
		return 0, 0, err
	}
	if err := s.paymentJournal.CreateWithSourceEventID(ctx, repairSourceID, entry); err != nil {
		return 0, 0, fmt.Errorf("post repaired receipt journal for payment %s: %w", payment.ID, err)
	}
	reposted++
	return reversed, reposted, nil
}
