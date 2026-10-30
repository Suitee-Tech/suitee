package services

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/pkg/events"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

// JournalService é o ponto de escrita do ledger de dupla entrada. Ele só
// aceita lançamentos balanceados e nunca edita ou remove uma entrada: a única
// correção é a reversão estruturada, que cria um Entry espelhado e marca o
// original como REVERSED dentro da mesma transação SQL.
type JournalService struct {
	repo journal.Repository
	db   *database.PostgresDB
}

// NewJournalService constrói o serviço de journal.
func NewJournalService(repo journal.Repository) *JournalService {
	return &JournalService{repo: repo}
}

// WithDB injeta o banco para tornar a reversão atômica.
func (s *JournalService) WithDB(db *database.PostgresDB) *JournalService {
	s.db = db
	return s
}

// Create valida e persiste um lançamento. Um lançamento já persistido sob o
// mesmo source_event_id é tratado como idempotente e não duplica.
func (s *JournalService) Create(ctx context.Context, entry *journal.Entry) error {
	if entry == nil {
		return fmt.Errorf("journal entry is required")
	}
	if !entry.Balanced() {
		return fmt.Errorf("%w: entry must be balanced", journal.ErrUnbalanced)
	}
	if entry.Status != journal.StatusPosted {
		return fmt.Errorf("only POSTED entries can be created")
	}
	if err := s.repo.Create(ctx, entry); err != nil {
		return err
	}
	return nil
}

// CreateWithSourceEventID vincula o lançamento a um fato econômico e torna a
// persistência idempotente por source_event_id.
func (s *JournalService) CreateWithSourceEventID(ctx context.Context, sourceEventID string, entry *journal.Entry) error {
	if strings.TrimSpace(sourceEventID) != "" && entry != nil {
		entry.SourceEventID = sourceEventID
	}
	err := s.Create(ctx, entry)
	if errors.Is(err, journal.ErrEntryAlreadyExists) {
		return nil
	}
	return err
}

// CreateCanonicalWithSourceEventID is the strict idempotency variant used by
// money-moving workflows. A duplicate source is accepted only when the
// canonical posting has the exact same economic payload.
func (s *JournalService) CreateCanonicalWithSourceEventID(ctx context.Context, sourceEventID string, entry *journal.Entry) (*journal.Entry, error) {
	sourceEventID = strings.TrimSpace(sourceEventID)
	if sourceEventID == "" || entry == nil {
		return nil, fmt.Errorf("source_event_id and journal entry are required")
	}
	entry.SourceEventID = sourceEventID
	if existing, err := s.repo.GetBySourceEventID(ctx, entry.TenantID, sourceEventID); err != nil {
		return nil, err
	} else if existing != nil {
		if !sameJournalPosting(existing, entry) {
			return nil, journal.ErrSourceEventConflict
		}
		return existing, nil
	}
	if err := s.Create(ctx, entry); err != nil {
		if !errors.Is(err, journal.ErrEntryAlreadyExists) {
			return nil, err
		}
		canonical, getErr := s.repo.GetBySourceEventID(ctx, entry.TenantID, sourceEventID)
		if getErr != nil {
			return nil, getErr
		}
		if canonical == nil || !sameJournalPosting(canonical, entry) {
			return nil, journal.ErrSourceEventConflict
		}
		return canonical, nil
	}
	return entry, nil
}

func sameJournalPosting(left, right *journal.Entry) bool {
	return left != nil && right != nil && left.TenantID == right.TenantID && left.ContractID == right.ContractID &&
		left.OperationID == right.OperationID && left.SourceEventID == right.SourceEventID && left.Date.Equal(right.Date) &&
		left.Description == right.Description && left.Status == right.Status && left.ReversalOf == right.ReversalOf &&
		left.ReversedBy == right.ReversedBy && reflect.DeepEqual(left.Legs, right.Legs)
}

// MarkReconciledBySourceEventID marks the exact payment posting reconciled.
// Replays are harmless; optimistic persistence prevents a concurrent reversal
// or reconciliation from being overwritten.
func (s *JournalService) MarkReconciledBySourceEventID(ctx context.Context, tenantID, sourceEventID string, reconciledAt time.Time) (*journal.Entry, error) {
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		var result *journal.Entry
		err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			var err error
			result, err = s.markReconciledBySourceEventID(database.WithTx(ctx, tx), tenantID, sourceEventID, reconciledAt)
			return err
		})
		return result, err
	}
	return s.markReconciledBySourceEventID(ctx, tenantID, sourceEventID, reconciledAt)
}

func (s *JournalService) markReconciledBySourceEventID(ctx context.Context, tenantID, sourceEventID string, reconciledAt time.Time) (*journal.Entry, error) {
	entry, err := s.repo.GetBySourceEventID(ctx, tenantID, strings.TrimSpace(sourceEventID))
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("journal entry not found")
	}
	if entry.ReconciliationDate != nil {
		return entry, nil
	}
	expected := entry.UpdatedAt
	if err := entry.MarkReconciled(reconciledAt); err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateStatus(ctx, entry, expected)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("journal entry was modified concurrently")
	}
	return entry, nil
}

// Reverse cria um lançamento de reversão balanceado para uma entrada POSTED e
// marca a original como REVERSED, atomicamente. Rechamadas idempotentes
// retornam o lançamento de reversão já existente.
func (s *JournalService) Reverse(ctx context.Context, id, tenantID, reason, operatorID string) (*journal.Entry, error) {
	return s.ReverseAt(ctx, id, tenantID, reason, operatorID, time.Time{})
}

// ReverseAt uses one explicit effective timestamp for the mirror entry. This
// keeps the ledger date aligned with the aggregate/history timestamp in
// workflows that span multiple persisted projections.
func (s *JournalService) ReverseAt(ctx context.Context, id, tenantID, reason, operatorID string, effectiveAt time.Time) (*journal.Entry, error) {
	reason, operatorID = strings.TrimSpace(reason), strings.TrimSpace(operatorID)
	if reason == "" || operatorID == "" {
		return nil, fmt.Errorf("%w: reversal reason and operator are required", journal.ErrInvalidEntry)
	}
	dateIsCanonical := !effectiveAt.IsZero()
	if dateIsCanonical {
		effectiveAt = effectiveAt.UTC().Truncate(time.Microsecond)
	}
	if s.db != nil && database.TxFromCtx(ctx) == nil {
		var reversed *journal.Entry
		err := s.db.RunInTx(ctx, func(tx *sqlx.Tx) error {
			var err error
			reversed, err = s.reverse(database.WithTx(ctx, tx), id, tenantID, reason, operatorID, effectiveAt, dateIsCanonical)
			return err
		})
		return reversed, err
	}
	return s.reverse(ctx, id, tenantID, reason, operatorID, effectiveAt, dateIsCanonical)
}

func (s *JournalService) reverse(
	ctx context.Context,
	id, tenantID, reason, operatorID string,
	effectiveAt time.Time,
	dateIsCanonical bool,
) (*journal.Entry, error) {
	original, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if original == nil || original.TenantID != tenantID {
		return nil, fmt.Errorf("journal entry not found")
	}

	reversalSourceEventID := events.DeriveNotificationID("journal.reversal", tenantID, original.ID)
	if existing, err := s.repo.GetBySourceEventID(ctx, tenantID, reversalSourceEventID); err != nil {
		return nil, fmt.Errorf("lookup existing reversal: %w", err)
	} else if existing != nil {
		if !sameJournalReversal(original, existing, reversalSourceEventID, reason, operatorID, effectiveAt, dateIsCanonical) {
			return nil, journal.ErrSourceEventConflict
		}
		return existing, nil
	}

	reversed, err := original.ReverseAt(reason, operatorID, effectiveAt)
	if err != nil {
		return nil, err
	}
	reversed.SourceEventID = reversalSourceEventID

	expectedUpdatedAt := original.UpdatedAt
	if err := original.MarkReversed(); err != nil {
		return nil, err
	}
	updated, err := s.repo.UpdateStatus(ctx, original, expectedUpdatedAt)
	if err != nil {
		return nil, err
	}
	if !updated {
		return nil, fmt.Errorf("journal entry was modified concurrently")
	}

	if err := s.repo.Create(ctx, reversed); err != nil {
		if errors.Is(err, journal.ErrEntryAlreadyExists) {
			canonical, getErr := s.repo.GetBySourceEventID(ctx, tenantID, reversalSourceEventID)
			if getErr != nil {
				return nil, fmt.Errorf("lookup canonical reversal: %w", getErr)
			}
			if canonical == nil || !sameJournalReversal(original, canonical, reversalSourceEventID, reason, operatorID, effectiveAt, dateIsCanonical) {
				return nil, journal.ErrSourceEventConflict
			}
			return canonical, nil
		}
		return nil, fmt.Errorf("failed to create reversal entry: %w", err)
	}
	return reversed, nil
}

func sameJournalReversal(
	original, reversal *journal.Entry,
	sourceEventID, reason, operatorID string,
	effectiveAt time.Time,
	dateIsCanonical bool,
) bool {
	if original == nil || reversal == nil || original.Status != journal.StatusReversed {
		return false
	}
	expectedLegs := make([]journal.Leg, len(original.Legs))
	for index, leg := range original.Legs {
		expectedLegs[index] = leg
		switch leg.Direction {
		case journal.Debit:
			expectedLegs[index].Direction = journal.Credit
		case journal.Credit:
			expectedLegs[index].Direction = journal.Debit
		default:
			return false
		}
	}
	if dateIsCanonical && !reversal.Date.Equal(effectiveAt) {
		return false
	}
	return reversal.ID != "" && reversal.TenantID == original.TenantID && reversal.ContractID == original.ContractID &&
		reversal.OperationID == original.OperationID && reversal.SourceEventID == sourceEventID &&
		reversal.Description == "Estorno: "+reason+" (ref: "+original.ID+")" &&
		reversal.Status == journal.StatusPosted && reversal.ReversalOf == original.ID && reversal.ReversedBy == operatorID && !reversal.Date.IsZero() &&
		reversal.ReconciliationDate == nil && reflect.DeepEqual(reversal.Legs, expectedLegs)
}

// GetByID expõe a leitura por ID com isolamento por tenant.
func (s *JournalService) GetByID(ctx context.Context, tenantID, id string) (*journal.Entry, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

// GetBySourceEventID expõe a leitura por fato econômico.
func (s *JournalService) GetBySourceEventID(ctx context.Context, tenantID, sourceEventID string) (*journal.Entry, error) {
	return s.repo.GetBySourceEventID(ctx, tenantID, sourceEventID)
}

// List pagina os lançamentos de um tenant (opcionalmente por contrato).
func (s *JournalService) List(ctx context.Context, tenantID, contractID string, limit, offset int64) ([]*journal.Entry, int64, error) {
	return s.repo.List(ctx, tenantID, contractID, limit, offset)
}

// AccountBalance expõe o saldo de uma conta para um tenant (débito positivo,
// crédito negativo em centavos).
func (s *JournalService) AccountBalance(ctx context.Context, tenantID string, account journal.Account) (int64, error) {
	return s.repo.AccountBalance(ctx, tenantID, account)
}

// AccountBalanceByFlow expõe o saldo de uma conta agrupado por modalidade
// econômica (sale/rent/vazio). Retorna um mapa vazio quando o repositório não
// implementa o desdobramento por flow.
func (s *JournalService) AccountBalanceByFlow(ctx context.Context, tenantID string, account journal.Account) (map[string]int64, error) {
	repo, ok := s.repo.(journal.FlowBalanceRepository)
	if !ok {
		return map[string]int64{}, nil
	}
	return repo.AccountBalanceByFlow(ctx, tenantID, account)
}
