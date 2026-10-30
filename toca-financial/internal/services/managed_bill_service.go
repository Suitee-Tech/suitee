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
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/managed_bill"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

var (
	ErrManagedBillReserveOverallocated  = errors.New("managed bill reserve exceeds the transfer reserved balance")
	ErrManagedBillTransferNotPending    = errors.New("managed bill reserve can only be released while transfer is pending")
	ErrManagedBillIdentityMismatch      = errors.New("managed bill economic identity does not match its owner transfer")
	ErrManagedBillActorConflict         = errors.New("managed bill approver and payer must be different actors")
	ErrManagedBillReversalActorConflict = errors.New("managed bill payer and payment reverser must be different actors")
	ErrManagedBillAtomicUnitOfWork      = errors.New("managed bill payment reversal requires an atomic unit of work")
)

// ManagedBillStaleVersionError é a rejeição por versão defasada: o cliente
// mandou `expected_version` de um estado que já não é o atual.
//
// Carrega a versão vigente para que a interface possa recarregar sem uma segunda
// consulta e para que o operador entenda que o problema é o estado em mãos, não
// os dados que ele informou. Antes este caso era indistinguível dos demais no
// mesmo ErrConcurrentModification genérico.
type ManagedBillStaleVersionError struct {
	Expected int64
	Current  int64
	Status   managed_bill.Status
}

func (e *ManagedBillStaleVersionError) Error() string {
	return fmt.Sprintf(
		"esta conta foi alterada por outra operação (versão enviada %d, versão atual %d, situação %s): recarregue a lista antes de registrar o pagamento",
		e.Expected, e.Current, e.Status,
	)
}

// Is mantém a compatibilidade com quem já checava o sentinela genérico.
func (e *ManagedBillStaleVersionError) Is(target error) bool {
	return target == managed_bill.ErrConcurrentModification
}

// ManagedBillAlreadyPaidError indica que o pagamento já foi registrado, com
// outra referência. Não é uma alteração concorrente qualquer: a conta já está
// paga, e repetir não vai funcionar.
type ManagedBillAlreadyPaidError struct {
	CurrentReference string
	Status           managed_bill.Status
}

func (e *ManagedBillAlreadyPaidError) Error() string {
	return "esta conta já está paga com outra referência de pagamento (situação " +
		string(e.Status) + "): confira a referência registrada ou estorne o pagamento para corrigir"
}

func (e *ManagedBillAlreadyPaidError) Is(target error) bool {
	return target == managed_bill.ErrConcurrentModification
}

type ManagedBillBalanceSnapshot struct {
	TotalCents     int64 `json:"total_cents"`
	ReservedCents  int64 `json:"reserved_cents"`
	BlockedCents   int64 `json:"blocked_cents"`
	AvailableCents int64 `json:"available_cents"`
}

type ManagedBillMutationResult struct {
	Bill             *managed_bill.ManagedBill  `json:"bill"`
	BalanceBefore    ManagedBillBalanceSnapshot `json:"owner_balance_before"`
	BalanceAfter     ManagedBillBalanceSnapshot `json:"owner_balance_after"`
	CoverageComplete bool                       `json:"coverage_complete"`
}

type ManagedBillService struct {
	repo           managed_bill.Repository
	transferRepo   transfer.RepositoryInterface
	journalService *JournalService
	db             *database.PostgresDB
	// atomicTxRunner is intentionally unexported. Production must configure the
	// real database through WithDB; in-memory tests can inject a synchronous
	// unit of work without weakening the production fail-closed check.
	atomicTxRunner func(context.Context, func(context.Context) error) error
}

func NewManagedBillService(repo managed_bill.Repository, transferRepo transfer.RepositoryInterface) *ManagedBillService {
	return &ManagedBillService{repo: repo, transferRepo: transferRepo}
}

func (s *ManagedBillService) WithDB(db *database.PostgresDB) *ManagedBillService {
	s.db = db
	return s
}

func (s *ManagedBillService) WithJournalService(service *JournalService) *ManagedBillService {
	s.journalService = service
	return s
}

func (s *ManagedBillService) GetByID(ctx context.Context, tenantID, id string) (*managed_bill.ManagedBill, error) {
	bill, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if bill == nil {
		return nil, managed_bill.ErrNotFound
	}
	return bill, nil
}

func (s *ManagedBillService) List(ctx context.Context, tenantID string, filter managed_bill.Filter) ([]*managed_bill.ManagedBill, int64, error) {
	return s.repo.List(ctx, tenantID, filter)
}

// EnsureReserved materializes exactly one bill per tenant/source key. The
// reserve itself already exists in OwnerBalance because payment allocations
// classified direct-liquidation/offset money as reserved; this operation only
// claims part of that pocket and must never debit/block it a second time.
func (s *ManagedBillService) EnsureReserved(ctx context.Context, in managed_bill.NewReservedInput) (*ManagedBillMutationResult, error) {
	candidate, err := managed_bill.NewReserved(in)
	if err != nil {
		return nil, err
	}
	var result *ManagedBillMutationResult
	err = s.runManagedBillTx(ctx, func(txCtx context.Context) error {
		if err := s.lockTransfer(txCtx, candidate.TenantID, candidate.SourceTransferID); err != nil {
			return err
		}
		item, err := s.loadTransfer(txCtx, candidate.TenantID, candidate.SourceTransferID)
		if err != nil {
			return err
		}
		if err := validateManagedBillTransferIdentity(candidate, item); err != nil {
			return err
		}
		before := managedBillBalanceSnapshot(item)

		existing, err := s.repo.GetBySourceKey(txCtx, candidate.TenantID, candidate.SourceKey)
		if err != nil {
			return err
		}
		if existing != nil {
			if !existing.SameEconomicIdentity(candidate) {
				return fmt.Errorf("managed bill source key reused with different economic data: %w", managed_bill.ErrAlreadyExists)
			}
			complete, err := s.coverageComplete(txCtx, item)
			if err != nil {
				return err
			}
			result = &ManagedBillMutationResult{Bill: existing, BalanceBefore: before, BalanceAfter: before, CoverageComplete: complete}
			return nil
		}

		activeReserved, err := s.repo.SumActiveReservedCentsByTransfer(txCtx, candidate.TenantID, candidate.SourceTransferID)
		if err != nil {
			return err
		}
		if activeReserved < 0 || activeReserved+candidate.AmountCents > item.OwnerBalance.ReservedCents {
			return ErrManagedBillReserveOverallocated
		}
		if err := s.repo.Create(txCtx, candidate); err != nil {
			if !errors.Is(err, managed_bill.ErrAlreadyExists) {
				return err
			}
			canonical, getErr := s.repo.GetBySourceKey(txCtx, candidate.TenantID, candidate.SourceKey)
			if getErr != nil {
				return getErr
			}
			if canonical == nil || !canonical.SameEconomicIdentity(candidate) {
				return managed_bill.ErrAlreadyExists
			}
			candidate = canonical
		}
		complete, err := s.coverageComplete(txCtx, item)
		if err != nil {
			return err
		}
		result = &ManagedBillMutationResult{Bill: candidate, BalanceBefore: before, BalanceAfter: before, CoverageComplete: complete}
		return nil
	})
	return result, err
}

func (s *ManagedBillService) Approve(ctx context.Context, tenantID, id, actor string, expectedVersion int64) (*ManagedBillMutationResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: approval actor is required", managed_bill.ErrInvalidInput)
	}
	return s.mutateWithoutBalance(ctx, tenantID, id, func(bill *managed_bill.ManagedBill) (bool, error) {
		if bill.Status == managed_bill.StatusApproved || bill.Status == managed_bill.StatusPaid || bill.Status == managed_bill.StatusReconciled {
			return false, nil
		}
		if bill.Version != expectedVersion {
			return false, managed_bill.ErrConcurrentModification
		}
		if err := bill.Approve(actor, time.Now().UTC()); err != nil {
			return false, err
		}
		return true, nil
	})
}

func (s *ManagedBillService) Pay(ctx context.Context, tenantID, id, actor string, expectedVersion int64, paymentReference, proofURL string, paidAt time.Time) (*ManagedBillMutationResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: payment actor is required", managed_bill.ErrInvalidInput)
	}
	paymentReference = strings.TrimSpace(paymentReference)
	if paymentReference == "" {
		return nil, fmt.Errorf("%w: payment_reference is required", managed_bill.ErrInvalidInput)
	}
	var result *ManagedBillMutationResult
	err := s.runManagedBillTx(ctx, func(txCtx context.Context) error {
		bill, item, err := s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.lockTransfer(txCtx, tenantID, item.ID); err != nil {
			return err
		}
		// Re-read after the advisory lock so another managed-bill command cannot
		// leave this operation with a stale aggregate snapshot.
		bill, item, err = s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		before := managedBillBalanceSnapshot(item)
		if bill.Status == managed_bill.StatusPaid || bill.Status == managed_bill.StatusReconciled {
			if bill.PaymentReference != paymentReference {
				// A conta já está paga, mas com outra referência: repetir não
				// resolve. Distinto de "alguém alterou o registro".
				return &ManagedBillAlreadyPaidError{CurrentReference: bill.PaymentReference, Status: bill.Status}
			}
			complete, err := s.coverageComplete(txCtx, item)
			if err != nil {
				return err
			}
			result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: before, BalanceAfter: before, CoverageComplete: complete}
			return nil
		}
		if bill.Version != expectedVersion {
			// O cliente está com um estado defasado. Devolver a versão vigente
			// permite recarregar sem uma segunda consulta e dizer ao operador o
			// que de fato aconteceu.
			return &ManagedBillStaleVersionError{Expected: expectedVersion, Current: bill.Version, Status: bill.Status}
		}
		// Segregação de funções: quem aprova não paga. A exigência de dois
		// operadores é controle interno, e a imobiliária pode dispensá-la para
		// quem responde pela operação financeira — sozinho na função, o
		// administrador precisa concluir os dois passos. A dispensa vem da borda
		// autenticada (WithSeparationOfDutiesWaiver), nunca do corpo da requisição.
		if actor == strings.TrimSpace(bill.ApprovedBy) && !SeparationOfDutiesWaived(ctx) {
			return ErrManagedBillActorConflict
		}
		paidAt = normalizedManagedBillTime(paidAt)
		sourceEventID := events.DeriveNotificationID("managed_bill.payment", tenantID, bill.ID, paymentReference)
		evidence := managedBillProofEvidence(proofURL, actor, paidAt)
		// Validate and stage the aggregate transition before any side effect. The
		// journal and transfer writes still commit atomically in the surrounding
		// transaction, but invalid actors/timestamps now fail before either call.
		if err := bill.MarkPaid(actor, paymentReference, sourceEventID, paidAt, evidence...); err != nil {
			return err
		}
		movement := transfer.OwnerBalanceMovement{
			ID: sourceEventID, ManagedBillID: bill.ID, PaymentAttemptID: sourceEventID,
			Type:        transfer.OwnerBalanceMovementConsumeReserved,
			AmountCents: bill.AmountCents, Actor: actor, At: paidAt,
		}
		expectedTransferVersion := item.UpdatedAt
		changed, err := item.ApplyOwnerBalanceMovement(movement)
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("managed bill payment movement already exists before bill payment")
		}
		if s.journalService == nil {
			return errors.New("journal service is required for managed bill payment")
		}
		entry, err := newManagedBillPaymentJournalEntry(bill, paymentReference, paidAt)
		if err != nil {
			return err
		}
		if _, err := s.journalService.CreateCanonicalWithSourceEventID(txCtx, sourceEventID, entry); err != nil {
			return fmt.Errorf("post managed bill payment journal: %w", err)
		}
		if err := s.persistTransfer(txCtx, item, expectedTransferVersion); err != nil {
			return err
		}
		if err := s.persistBill(txCtx, bill, expectedVersion); err != nil {
			return err
		}
		complete, err := s.coverageComplete(txCtx, item)
		if err != nil {
			return err
		}
		result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: before, BalanceAfter: managedBillBalanceSnapshot(item), CoverageComplete: complete}
		return nil
	})
	return result, err
}

func (s *ManagedBillService) Reconcile(ctx context.Context, tenantID, id, actor string, expectedVersion int64, reference string, reconciledAt time.Time) (*ManagedBillMutationResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: reconciliation actor is required", managed_bill.ErrInvalidInput)
	}
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, fmt.Errorf("%w: reconciliation reference is required", managed_bill.ErrInvalidInput)
	}
	var result *ManagedBillMutationResult
	err := s.runManagedBillTx(ctx, func(txCtx context.Context) error {
		bill, item, err := s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.lockTransfer(txCtx, tenantID, item.ID); err != nil {
			return err
		}
		bill, item, err = s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		snapshot := managedBillBalanceSnapshot(item)
		if bill.Status == managed_bill.StatusReconciled {
			if bill.ReconciliationReference != reference {
				return managed_bill.ErrConcurrentModification
			}
			complete, err := s.coverageComplete(txCtx, item)
			if err != nil {
				return err
			}
			result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: snapshot, BalanceAfter: snapshot, CoverageComplete: complete}
			return nil
		}
		if bill.Version != expectedVersion {
			return managed_bill.ErrConcurrentModification
		}
		if s.journalService == nil || strings.TrimSpace(bill.PaymentJournalSourceID) == "" {
			return errors.New("managed bill payment journal is required for reconciliation")
		}
		reconciledAt = normalizedManagedBillTime(reconciledAt)
		if err := bill.Reconcile(actor, reference, reconciledAt); err != nil {
			return err
		}
		if _, err := s.journalService.MarkReconciledBySourceEventID(txCtx, tenantID, bill.PaymentJournalSourceID, reconciledAt); err != nil {
			return err
		}
		if err := s.persistBill(txCtx, bill, expectedVersion); err != nil {
			return err
		}
		complete, err := s.coverageComplete(txCtx, item)
		if err != nil {
			return err
		}
		result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: snapshot, BalanceAfter: snapshot, CoverageComplete: complete}
		return nil
	})
	return result, err
}

// ReversePayment compensates one PAID/RECONCILED payment without erasing its
// audit trail. The exact journal posting is reversed, the owner's funds return
// to the reserved pocket, and the bill goes back to APPROVED so a new payment
// attempt can be made. Every write commits in the same SQL transaction.
func (s *ManagedBillService) ReversePayment(
	ctx context.Context,
	tenantID, id, actor string,
	expectedVersion int64,
	reversalReference, reason, proofURL string,
	reversedAt time.Time,
) (*ManagedBillMutationResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: payment reversal actor is required", managed_bill.ErrInvalidInput)
	}
	reversalReference = strings.TrimSpace(reversalReference)
	if reversalReference == "" {
		return nil, fmt.Errorf("%w: reversal_reference is required", managed_bill.ErrInvalidInput)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: payment reversal reason is required", managed_bill.ErrInvalidInput)
	}
	proofURL = strings.TrimSpace(proofURL)
	if s.db == nil && s.atomicTxRunner == nil {
		return nil, ErrManagedBillAtomicUnitOfWork
	}

	var result *ManagedBillMutationResult
	err := s.runManagedBillTx(ctx, func(txCtx context.Context) error {
		bill, item, err := s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.lockTransfer(txCtx, tenantID, item.ID); err != nil {
			return err
		}
		bill, item, err = s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		before := managedBillBalanceSnapshot(item)

		// A retried HTTP command arrives with the pre-reversal version. Accept it
		// only when it identifies the exact reversal already recorded.
		if bill.Status == managed_bill.StatusApproved && len(bill.PaymentAttempts) > 0 {
			last := bill.PaymentAttempts[len(bill.PaymentAttempts)-1]
			retryTimeMatches := reversedAt.IsZero() || normalizedManagedBillTime(reversedAt).Equal(last.ReversedAt)
			if last.ReversalReference == reversalReference && last.ReversalReason == reason &&
				last.ReversalProofReference == proofURL && last.ReversedBy == actor && retryTimeMatches {
				complete, coverageErr := s.coverageComplete(txCtx, item)
				if coverageErr != nil {
					return coverageErr
				}
				result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: before, BalanceAfter: before, CoverageComplete: complete}
				return nil
			}
		}
		if bill.Version != expectedVersion {
			return managed_bill.ErrConcurrentModification
		}
		// Mesma dispensa no estorno: pela nova política o administrador também
		// corrige o que ele mesmo pagou, sem depender de um segundo operador.
		if actor == strings.TrimSpace(bill.PaidBy) && !SeparationOfDutiesWaived(ctx) {
			return ErrManagedBillReversalActorConflict
		}
		if s.journalService == nil || strings.TrimSpace(bill.PaymentJournalSourceID) == "" {
			return errors.New("managed bill payment journal is required for reversal")
		}

		paymentJournal, err := s.journalService.GetBySourceEventID(txCtx, tenantID, bill.PaymentJournalSourceID)
		if err != nil {
			return fmt.Errorf("load managed bill payment journal: %w", err)
		}
		if paymentJournal == nil {
			return errors.New("managed bill payment journal not found")
		}
		canonicalPayment, err := sameManagedBillPaymentPosting(bill, paymentJournal)
		if err != nil {
			return fmt.Errorf("validate managed bill payment journal: %w", err)
		}
		if !canonicalPayment {
			return journal.ErrSourceEventConflict
		}
		reversedAt = normalizedManagedBillTime(reversedAt)
		reversalJournalSourceID := events.DeriveNotificationID("journal.reversal", tenantID, paymentJournal.ID)
		paymentAttemptID := strings.TrimSpace(bill.PaymentJournalSourceID)
		evidence := managedBillReversalProofEvidence(proofURL, actor, reversedAt)

		// Stage every aggregate invariant before the journal or transfer writes.
		if err := bill.ReversePayment(actor, reversalReference, reversalJournalSourceID, reason, reversedAt, evidence...); err != nil {
			return err
		}
		movement := transfer.OwnerBalanceMovement{
			ID: reversalJournalSourceID, ManagedBillID: bill.ID, PaymentAttemptID: paymentAttemptID,
			Type: transfer.OwnerBalanceMovementRestoreReserved, AmountCents: bill.AmountCents,
			Actor: actor, At: reversedAt,
		}
		expectedTransferVersion := item.UpdatedAt
		changed, err := item.ApplyOwnerBalanceMovement(movement)
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("managed bill payment reversal movement already exists before reversal")
		}

		reversalEntry, err := s.journalService.ReverseAt(txCtx, paymentJournal.ID, tenantID, reason, actor, reversedAt)
		if err != nil {
			return fmt.Errorf("reverse managed bill payment journal: %w", err)
		}
		if reversalEntry == nil || reversalEntry.SourceEventID != reversalJournalSourceID ||
			reversalEntry.ReversedBy != actor || !reversalEntry.Date.Equal(reversedAt) {
			return journal.ErrSourceEventConflict
		}
		if err := s.persistTransfer(txCtx, item, expectedTransferVersion); err != nil {
			return err
		}
		if err := s.persistBill(txCtx, bill, expectedVersion); err != nil {
			return err
		}
		complete, err := s.coverageComplete(txCtx, item)
		if err != nil {
			return err
		}
		result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: before, BalanceAfter: managedBillBalanceSnapshot(item), CoverageComplete: complete}
		return nil
	})
	return result, err
}

func (s *ManagedBillService) Cancel(ctx context.Context, tenantID, id, actor string, expectedVersion int64, reason string) (*ManagedBillMutationResult, error) {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return nil, fmt.Errorf("%w: cancellation actor is required", managed_bill.ErrInvalidInput)
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("%w: cancellation reason is required", managed_bill.ErrInvalidInput)
	}
	var result *ManagedBillMutationResult
	err := s.runManagedBillTx(ctx, func(txCtx context.Context) error {
		bill, item, err := s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.lockTransfer(txCtx, tenantID, item.ID); err != nil {
			return err
		}
		bill, item, err = s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		before := managedBillBalanceSnapshot(item)
		if bill.Status == managed_bill.StatusCancelled {
			if bill.CancellationReason != reason {
				return managed_bill.ErrConcurrentModification
			}
			complete, err := s.coverageComplete(txCtx, item)
			if err != nil {
				return err
			}
			result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: before, BalanceAfter: before, CoverageComplete: complete}
			return nil
		}
		if bill.Version != expectedVersion {
			return managed_bill.ErrConcurrentModification
		}
		if item.Status != transfer.StatusPending {
			return ErrManagedBillTransferNotPending
		}
		cancelledAt := normalizedManagedBillTime(time.Time{})
		if err := bill.Cancel(actor, reason, cancelledAt); err != nil {
			return err
		}
		movementID := events.DeriveNotificationID("managed_bill.cancel", tenantID, bill.ID, reason)
		expectedTransferVersion := item.UpdatedAt
		changed, err := item.ApplyOwnerBalanceMovement(transfer.OwnerBalanceMovement{
			ID: movementID, ManagedBillID: bill.ID, Type: transfer.OwnerBalanceMovementReleaseReserved,
			AmountCents: bill.AmountCents, Actor: actor, At: cancelledAt,
		})
		if err != nil {
			return err
		}
		if !changed {
			return fmt.Errorf("managed bill release movement already exists before cancellation")
		}
		if err := s.persistTransfer(txCtx, item, expectedTransferVersion); err != nil {
			return err
		}
		if err := s.persistBill(txCtx, bill, expectedVersion); err != nil {
			return err
		}
		complete, err := s.coverageComplete(txCtx, item)
		if err != nil {
			return err
		}
		result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: before, BalanceAfter: managedBillBalanceSnapshot(item), CoverageComplete: complete}
		return nil
	})
	return result, err
}

func (s *ManagedBillService) mutateWithoutBalance(ctx context.Context, tenantID, id string, mutate func(*managed_bill.ManagedBill) (bool, error)) (*ManagedBillMutationResult, error) {
	var result *ManagedBillMutationResult
	err := s.runManagedBillTx(ctx, func(txCtx context.Context) error {
		bill, item, err := s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		if err := s.lockTransfer(txCtx, tenantID, item.ID); err != nil {
			return err
		}
		bill, item, err = s.loadBillAndTransfer(txCtx, tenantID, id)
		if err != nil {
			return err
		}
		snapshot := managedBillBalanceSnapshot(item)
		expected := bill.Version
		changed, err := mutate(bill)
		if err != nil {
			return err
		}
		if changed {
			if err := s.persistBill(txCtx, bill, expected); err != nil {
				return err
			}
		}
		complete, err := s.coverageComplete(txCtx, item)
		if err != nil {
			return err
		}
		result = &ManagedBillMutationResult{Bill: bill, BalanceBefore: snapshot, BalanceAfter: snapshot, CoverageComplete: complete}
		return nil
	})
	return result, err
}

func (s *ManagedBillService) runManagedBillTx(ctx context.Context, work func(context.Context) error) error {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		return s.db.RunInTx(ctx, func(tx *sqlx.Tx) error { return work(database.WithTx(ctx, tx)) })
	}
	if s.atomicTxRunner != nil && database.TxFromCtx(ctx) == nil {
		return s.atomicTxRunner(ctx, work)
	}
	return work(ctx)
}

func (s *ManagedBillService) lockTransfer(ctx context.Context, tenantID, transferID string) error {
	if s.db == nil {
		return nil
	}
	key := strings.TrimSpace(tenantID) + ":managed-bill:" + strings.TrimSpace(transferID)
	if _, err := s.db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", key); err != nil {
		return fmt.Errorf("lock managed bill transfer: %w", err)
	}
	return nil
}

func (s *ManagedBillService) loadBillAndTransfer(ctx context.Context, tenantID, id string) (*managed_bill.ManagedBill, *transfer.OwnerTransfer, error) {
	bill, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, nil, err
	}
	if bill == nil {
		return nil, nil, managed_bill.ErrNotFound
	}
	item, err := s.loadTransfer(ctx, tenantID, bill.SourceTransferID)
	if err != nil {
		return nil, nil, err
	}
	if err := validateManagedBillTransferIdentity(bill, item); err != nil {
		return nil, nil, err
	}
	return bill, item, nil
}

func (s *ManagedBillService) loadTransfer(ctx context.Context, tenantID, id string) (*transfer.OwnerTransfer, error) {
	if s.transferRepo == nil {
		return nil, errors.New("transfer repository is required")
	}
	item, err := s.transferRepo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if item == nil || item.OwnerBalance == nil {
		return nil, ErrManagedBillIdentityMismatch
	}
	return item, nil
}

func validateManagedBillTransferIdentity(bill *managed_bill.ManagedBill, item *transfer.OwnerTransfer) error {
	if bill == nil || item == nil || bill.TenantID != item.TenantID || bill.SourceTransferID != item.ID ||
		bill.SourceChargeID != item.SourceChargeID || bill.ContractID != item.ContractID ||
		bill.PropertyID != item.PropertyID || bill.OwnerID != item.OwnerID {
		return ErrManagedBillIdentityMismatch
	}
	return nil
}

func (s *ManagedBillService) persistBill(ctx context.Context, bill *managed_bill.ManagedBill, expectedVersion int64) error {
	updated, err := s.repo.UpdateIfVersion(ctx, bill, expectedVersion)
	if err != nil {
		return err
	}
	if !updated {
		return managed_bill.ErrConcurrentModification
	}
	return nil
}

func (s *ManagedBillService) persistTransfer(ctx context.Context, item *transfer.OwnerTransfer, expectedVersion time.Time) error {
	if versioned, ok := s.transferRepo.(transferVersionedRepository); ok {
		updated, err := versioned.UpdateIfVersion(ctx, item, expectedVersion)
		if err != nil {
			return err
		}
		if !updated {
			return transfer.ErrConcurrentModification
		}
		return nil
	}
	return s.transferRepo.Update(ctx, item)
}

func (s *ManagedBillService) coverageComplete(ctx context.Context, item *transfer.OwnerTransfer) (bool, error) {
	active, err := s.repo.SumActiveReservedCentsByTransfer(ctx, item.TenantID, item.ID)
	if err != nil {
		return false, err
	}
	return item.OwnerBalance != nil && active == item.OwnerBalance.ReservedCents, nil
}

func managedBillBalanceSnapshot(item *transfer.OwnerTransfer) ManagedBillBalanceSnapshot {
	if item == nil || item.OwnerBalance == nil {
		return ManagedBillBalanceSnapshot{}
	}
	return ManagedBillBalanceSnapshot{
		TotalCents: item.OwnerBalance.TotalCents, ReservedCents: item.OwnerBalance.ReservedCents,
		BlockedCents: item.OwnerBalance.BlockedCents, AvailableCents: item.OwnerBalance.AvailableCents(),
	}
}

func newManagedBillPaymentJournalEntry(bill *managed_bill.ManagedBill, paymentReference string, paidAt time.Time) (*journal.Entry, error) {
	entry, err := journal.New(bill.TenantID, bill.ContractID, "Pagamento de conta administrada: "+bill.Description, paidAt, []journal.Leg{
		{Account: journal.AccountOwnerPayable, Direction: journal.Debit, AmountCents: bill.AmountCents, ManagedBillID: bill.ID},
		{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: bill.AmountCents, ManagedBillID: bill.ID},
	})
	if err != nil {
		return nil, err
	}
	entry.OperationID = strings.TrimSpace(paymentReference)
	return entry, nil
}

func sameManagedBillPaymentPosting(bill *managed_bill.ManagedBill, entry *journal.Entry) (bool, error) {
	if bill == nil || entry == nil || bill.PaidAt == nil || strings.TrimSpace(entry.ID) == "" {
		return false, nil
	}
	expected, err := newManagedBillPaymentJournalEntry(bill, bill.PaymentReference, *bill.PaidAt)
	if err != nil {
		return false, err
	}
	expected.SourceEventID = strings.TrimSpace(bill.PaymentJournalSourceID)
	if !sameJournalPosting(entry, expected) || entry.ReversalOf != "" || entry.ReversedBy != "" {
		return false, nil
	}
	switch bill.Status {
	case managed_bill.StatusPaid:
		return entry.ReconciliationDate == nil, nil
	case managed_bill.StatusReconciled:
		return bill.ReconciledAt != nil && entry.ReconciliationDate != nil && entry.ReconciliationDate.Equal(*bill.ReconciledAt), nil
	default:
		return false, nil
	}
}

func managedBillProofEvidence(proofURL, actor string, at time.Time) []managed_bill.Evidence {
	proofURL = strings.TrimSpace(proofURL)
	if proofURL == "" {
		return nil
	}
	return []managed_bill.Evidence{{Kind: "payment_proof", Reference: proofURL, Actor: strings.TrimSpace(actor), At: at}}
}

func managedBillReversalProofEvidence(proofURL, actor string, at time.Time) []managed_bill.Evidence {
	proofURL = strings.TrimSpace(proofURL)
	if proofURL == "" {
		return nil
	}
	return []managed_bill.Evidence{{Kind: "payment_reversal_proof", Reference: proofURL, Actor: strings.TrimSpace(actor), At: at}}
}

func normalizedManagedBillTime(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now()
	}
	return value.UTC().Truncate(time.Microsecond)
}
