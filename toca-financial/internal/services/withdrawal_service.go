package services

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
	"github.com/oginoino/toca/toca-financial/internal/domain/withdrawal"
)

// WithdrawalService runs the agency-withdrawal flow: request → approve →
// settle MANUALLY. With no payment gateway, settlement is an operator
// action: the money is moved off-platform (PIX, TED) and the operator
// records a manual external reference + optional proof when marking the
// withdrawal processed.
//
// Accounting (double-entry, FIN-E2E-34): approving a withdrawal reserves the
// agency's equity (debit equity ↔ credit withdrawal_payable); settling moves
// cash off-platform (debit withdrawal_payable ↔ credit cash); cancelling an
// approved withdrawal releases the reserve back to equity. The immutable
// idempotency key is ExternalID and never changes — the operator's settlement
// reference lives in SettlementRef.
type WithdrawalService struct {
	repo           withdrawal.Repository
	journalService *JournalService
	db             *database.PostgresDB
}

func NewWithdrawalService(repo withdrawal.Repository) *WithdrawalService {
	return &WithdrawalService{repo: repo}
}

// WithDB injeta o banco para tornar approve/settle/cancel atômicos com o
// journal e para serializar a reserva via advisory lock.
func (s *WithdrawalService) WithDB(db *database.PostgresDB) *WithdrawalService {
	s.db = db
	return s
}

// WithJournalService injeta o ledger. Quando nil, o fluxo degrada ao
// comportamento legado (sem gate de saldo nem lançamentos contábeis).
func (s *WithdrawalService) WithJournalService(j *JournalService) *WithdrawalService {
	s.journalService = j
	return s
}

// runTx envolve fn em uma transação SQL quando há banco e ainda não existe
// transação no contexto. Repositórios tx-aware participam da mesma conexão.
func (s *WithdrawalService) runTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		return s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			return fn(database.WithTx(ctx, tx))
		})
	}
	return fn(ctx)
}

// Request creates a PENDING withdrawal awaiting approval.
func (s *WithdrawalService) Request(ctx context.Context, tenantID string, amountCents int64, description, requestedBy string) (*withdrawal.Withdrawal, error) {
	w, err := withdrawal.New(tenantID, amountCents, description, requestedBy)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, w); err != nil {
		return nil, err
	}
	return w, nil
}

func (s *WithdrawalService) RequestIdempotent(ctx context.Context, tenantID string, amountCents int64, description, requestedBy, idempotencyKey string) (*withdrawal.Withdrawal, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return s.Request(ctx, tenantID, amountCents, description, requestedBy)
	}
	externalID := "toca-withdrawal-" + events.DeriveNotificationID(
		idempotencyKey,
		"withdrawal.request",
		tenantID,
		strconv.FormatInt(amountCents, 10),
		strings.TrimSpace(description),
	)
	existing, err := s.repo.GetByExternalID(ctx, tenantID, externalID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	w, err := withdrawal.New(tenantID, amountCents, description, requestedBy)
	if err != nil {
		return nil, err
	}
	w.ExternalID = externalID
	if err := s.repo.Create(ctx, w); err != nil {
		if errors.Is(err, withdrawal.ErrWithdrawalAlreadyExists) {
			again, gerr := s.repo.GetByExternalID(ctx, tenantID, externalID)
			if gerr != nil {
				return nil, gerr
			}
			return again, nil
		}
		return nil, err
	}
	return w, nil
}

func (s *WithdrawalService) GetByID(ctx context.Context, tenantID, id string) (*withdrawal.Withdrawal, error) {
	w, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, fmt.Errorf("withdrawal not found")
	}
	return w, nil
}

func (s *WithdrawalService) List(ctx context.Context, tenantID, status string, limit, offset int64) ([]*withdrawal.Withdrawal, int64, error) {
	return s.repo.List(ctx, tenantID, status, limit, offset)
}

// Approve approves a PENDING withdrawal, moving it to APPROVED so an
// operator can settle it manually. With a journal wired, approval reserves
// the amount against the agency's equity balance and is serialized per
// tenant so concurrent withdrawals cannot overspend.
func (s *WithdrawalService) Approve(ctx context.Context, tenantID, id, actor string) (*withdrawal.Withdrawal, error) {
	var result *withdrawal.Withdrawal
	err := s.runTx(ctx, func(ctx context.Context) error {
		w, err := s.repo.GetByID(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if w == nil {
			return fmt.Errorf("withdrawal not found")
		}
		if w.Status != withdrawal.StatusPending {
			return withdrawal.ErrNotPending
		}
		// P1/A13 — criador ≠ aprovador.
		if err := requireDistinctRequesterAndApprover(ctx, w.RequestedBy, actor); err != nil {
			return err
		}

		// Serializa o check-and-reserve por tenant para fechar a corrida entre
		// dois approves concorrentes observando o mesmo saldo disponível.
		if s.db != nil {
			if _, err := s.db.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", tenantID+":withdrawal:approve"); err != nil {
				return fmt.Errorf("lock withdrawal approve: %w", err)
			}
		}

		if s.journalService != nil {
			available, err := s.availableEquity(ctx, tenantID)
			if err != nil {
				return err
			}
			if w.AmountCents > available {
				return withdrawal.ErrInsufficientBalance
			}
			entry, err := s.reserveEntry(w)
			if err != nil {
				return err
			}
			if err := s.journalService.CreateWithSourceEventID(ctx, events.DeriveNotificationID("withdrawal.reserve", w.TenantID, w.ID), entry); err != nil {
				return err
			}
		}

		if err := w.Approve(actor); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, w); err != nil {
			return err
		}
		result = w
		return nil
	})
	return result, err
}

func (s *WithdrawalService) ApproveIdempotent(ctx context.Context, tenantID, id, actor string) (*withdrawal.Withdrawal, error) {
	w, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, fmt.Errorf("withdrawal not found")
	}
	if w.Status == withdrawal.StatusApproved || w.Status == withdrawal.StatusComplete {
		return w, nil
	}
	return s.Approve(ctx, tenantID, id, actor)
}

// Settle records the MANUAL settlement of an APPROVED withdrawal: the
// operator moved the money off-platform and supplies an optional external
// reference (PIX/TED end-to-end id) and proof URL (comprovante). The
// withdrawal transitions to its terminal COMPLETE state. With a journal
// wired, settlement posts the debit withdrawal_payable ↔ credit cash entry.
func (s *WithdrawalService) Settle(ctx context.Context, tenantID, id, actor, externalRef, proofURL string) (*withdrawal.Withdrawal, error) {
	var result *withdrawal.Withdrawal
	err := s.runTx(ctx, func(ctx context.Context) error {
		w, err := s.repo.GetByID(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if w == nil {
			return fmt.Errorf("withdrawal not found")
		}
		if w.Status != withdrawal.StatusApproved {
			return withdrawal.ErrNotApproved
		}
		// P1/A13 — pagador ≠ aprovador.
		if err := requireDistinctApproverAndPayer(ctx, w.ApprovedBy, actor); err != nil {
			return err
		}

		if s.journalService != nil {
			entry, err := s.settleEntry(w)
			if err != nil {
				return err
			}
			if err := s.journalService.CreateWithSourceEventID(ctx, events.DeriveNotificationID("withdrawal.settle", w.TenantID, w.ID), entry); err != nil {
				return err
			}
		}

		if err := w.MarkSettledManually(actor, externalRef, proofURL); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, w); err != nil {
			return err
		}
		result = w
		return nil
	})
	return result, err
}

func (s *WithdrawalService) SettleIdempotent(ctx context.Context, tenantID, id, actor, externalRef, proofURL string) (*withdrawal.Withdrawal, error) {
	w, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, fmt.Errorf("withdrawal not found")
	}
	if w.Status == withdrawal.StatusComplete {
		return w, nil
	}
	return s.Settle(ctx, tenantID, id, actor, externalRef, proofURL)
}

// Cancel aborts a withdrawal that has not been settled yet. Cancelling an
// APPROVED withdrawal releases its reserve back to equity in the journal.
func (s *WithdrawalService) Cancel(ctx context.Context, tenantID, id, actor string) (*withdrawal.Withdrawal, error) {
	var result *withdrawal.Withdrawal
	err := s.runTx(ctx, func(ctx context.Context) error {
		w, err := s.repo.GetByID(ctx, tenantID, id)
		if err != nil {
			return err
		}
		if w == nil {
			return fmt.Errorf("withdrawal not found")
		}

		wasApproved := w.Status == withdrawal.StatusApproved
		if s.journalService != nil && wasApproved {
			entry, err := s.releaseEntry(w)
			if err != nil {
				return err
			}
			if err := s.journalService.CreateWithSourceEventID(ctx, events.DeriveNotificationID("withdrawal.release", w.TenantID, w.ID), entry); err != nil {
				return err
			}
		}

		if err := w.Cancel(actor); err != nil {
			return err
		}
		if err := s.repo.Update(ctx, w); err != nil {
			return err
		}
		result = w
		return nil
	})
	return result, err
}

func (s *WithdrawalService) CancelIdempotent(ctx context.Context, tenantID, id, actor string) (*withdrawal.Withdrawal, error) {
	w, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, fmt.Errorf("withdrawal not found")
	}
	if w.Status == withdrawal.StatusCancelled {
		return w, nil
	}
	return s.Cancel(ctx, tenantID, id, actor)
}

// availableEquity devolve o saldo disponível para saque: o crédito líquido da
// conta de patrimônio. AccountBalance devolve débito positivo/crédito
// negativo, então o disponível é o valor negado.
func (s *WithdrawalService) availableEquity(ctx context.Context, tenantID string) (int64, error) {
	balance, err := s.journalService.AccountBalance(ctx, tenantID, journal.AccountEquity)
	if err != nil {
		return 0, err
	}
	return -balance, nil
}

func (s *WithdrawalService) reserveEntry(w *withdrawal.Withdrawal) (*journal.Entry, error) {
	return journal.New(w.TenantID, "", fmt.Sprintf("Reserva de saque %s", w.ID), time.Now().UTC(), []journal.Leg{
		{Account: journal.AccountEquity, Direction: journal.Debit, AmountCents: w.AmountCents},
		{Account: journal.AccountWithdrawalPayable, Direction: journal.Credit, AmountCents: w.AmountCents},
	})
}

func (s *WithdrawalService) settleEntry(w *withdrawal.Withdrawal) (*journal.Entry, error) {
	return journal.New(w.TenantID, "", fmt.Sprintf("Saque %s", w.ID), time.Now().UTC(), []journal.Leg{
		{Account: journal.AccountWithdrawalPayable, Direction: journal.Debit, AmountCents: w.AmountCents},
		{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: w.AmountCents},
	})
}

func (s *WithdrawalService) releaseEntry(w *withdrawal.Withdrawal) (*journal.Entry, error) {
	return journal.New(w.TenantID, "", fmt.Sprintf("Cancelamento de saque %s", w.ID), time.Now().UTC(), []journal.Leg{
		{Account: journal.AccountWithdrawalPayable, Direction: journal.Debit, AmountCents: w.AmountCents},
		{Account: journal.AccountEquity, Direction: journal.Credit, AmountCents: w.AmountCents},
	})
}
