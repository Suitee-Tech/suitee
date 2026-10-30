package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/logger"
	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-financial/internal/domain/broker_advance"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

// BrokerAdvanceService registra o adiantamento que a imobiliária faz ao corretor
// e recupera o valor abatendo nas comissões seguintes.
//
// A concessão é um fato de caixa: entra no razão como débito em "contas a
// receber" e crédito em caixa (o corretor deve esse valor). Cada abatimento
// credita o mesmo ativo e reduz o que o corretor recebe — o lançamento de
// liquidação da comissão é quem faz essa perna (ver transaction_service.go),
// porque é ali que o líquido é apurado.
//
// Diferente do tributo retido, aqui NÃO existe obrigação a recolher: o dinheiro
// é da própria imobiliária voltando, e o controle é o saldo por corretor.
type BrokerAdvanceService struct {
	repo    broker_advance.Repository
	journal *JournalService
	now     func() time.Time
}

func NewBrokerAdvanceService(repo broker_advance.Repository) *BrokerAdvanceService {
	return &BrokerAdvanceService{repo: repo, now: time.Now}
}

func (s *BrokerAdvanceService) WithJournal(journalService *JournalService) *BrokerAdvanceService {
	s.journal = journalService
	return s
}

// Grant registra o adiantamento e escritura a saída de caixa.
func (s *BrokerAdvanceService) Grant(ctx context.Context, tenantID, brokerID string, amountCents int64, description, actor string, grantedAt time.Time) (*broker_advance.Advance, error) {
	advance, err := broker_advance.New(tenantID, brokerID, amountCents, description, grantedAt)
	if err != nil {
		return nil, err
	}
	if s.journal != nil {
		entry, err := journal.New(tenantID, "", "Adiantamento ao corretor "+brokerID, advance.GrantedAt, []journal.Leg{
			{Account: journal.AccountAccountsReceivable, Direction: journal.Debit, AmountCents: advance.AmountCents, BeneficiaryPartyID: brokerID},
			{Account: journal.AccountCash, Direction: journal.Credit, AmountCents: advance.AmountCents},
		})
		if err != nil {
			return nil, err
		}
		entry.OperationID = advance.ID
		created, err := s.journal.CreateCanonicalWithSourceEventID(ctx, "broker_advance.granted:"+advance.ID, entry)
		if err != nil {
			return nil, fmt.Errorf("escriturar adiantamento no razão: %w", err)
		}
		if created != nil {
			advance.JournalEntryID = created.ID
		}
	}
	if err := s.repo.Create(ctx, advance); err != nil {
		return nil, err
	}
	_ = actor
	return advance, nil
}

// SettleFromDeduction abate o adiantamento a partir de um desconto lançado na
// comissão, do mais antigo para o mais novo (FIFO), até o valor do desconto.
func (s *BrokerAdvanceService) SettleFromDeduction(ctx context.Context, c *commission.Commission, d commission.Deduction) error {
	if s == nil || s.repo == nil || c == nil {
		return nil
	}
	if commission.DeductionKind(d.Kind) != commission.DeductionAdvance {
		return nil
	}
	remaining := int64(d.Amount * 100)
	if remaining <= 0 {
		return nil
	}
	advances, err := s.repo.ListByBroker(ctx, c.TenantID, c.BrokerID, broker_advance.ListFilter{OpenOnly: true})
	if err != nil {
		return err
	}
	settledAny := false
	for _, advance := range advances {
		if remaining <= 0 {
			break
		}
		if advance.OutstandingCents() <= 0 {
			continue
		}
		expected := advance.UpdatedAt
		applied, err := advance.ApplySettlement(broker_advance.Settlement{
			CommissionID: c.ID,
			DeductionID:  d.ID,
			ContractID:   c.ContractID,
			AmountCents:  remaining,
		}, s.now())
		if err != nil {
			return err
		}
		if applied == 0 {
			continue
		}
		if err := s.repo.Update(ctx, advance, expected); err != nil {
			return err
		}
		remaining -= applied
		settledAny = true
	}
	if remaining > 0 {
		// Abater mais do que o saldo deixaria o corretor "devendo" um desconto que
		// não existe: o operador precisa saber antes de salvar a comissão.
		return fmt.Errorf("desconto de adiantamento excede o saldo do corretor em %s", money.BRL(float64(remaining)/100))
	}
	if settledAny {
		logger.FromContext(ctx).Info("broker_advance.settled",
			"event", "broker_advance.settled", "commission_id", c.ID, "broker_id", c.BrokerID, "amount_cents", int64(d.Amount*100))
	}
	return nil
}

// ReverseFromDeduction devolve o abatimento quando o desconto é removido da
// comissão (antes do pagamento): o saldo do adiantamento volta a existir.
func (s *BrokerAdvanceService) ReverseFromDeduction(ctx context.Context, tenantID, brokerID, commissionID, deductionID string) error {
	if s == nil || s.repo == nil || strings.TrimSpace(brokerID) == "" {
		return nil
	}
	advances, err := s.repo.ListByBroker(ctx, tenantID, brokerID, broker_advance.ListFilter{})
	if err != nil {
		return err
	}
	for _, advance := range advances {
		expected := advance.UpdatedAt
		reversed, err := advance.ReverseSettlement(commissionID, deductionID, s.now())
		if err != nil {
			return err
		}
		if reversed == 0 {
			continue
		}
		if err := s.repo.Update(ctx, advance, expected); err != nil {
			return err
		}
	}
	return nil
}

// Cancel encerra um adiantamento sem abatimentos e estorna a saída de caixa.
func (s *BrokerAdvanceService) Cancel(ctx context.Context, tenantID, id, actor, reason string) (*broker_advance.Advance, error) {
	advance, err := s.repo.GetByID(ctx, tenantID, id)
	if err != nil {
		return nil, err
	}
	if advance == nil {
		return nil, broker_advance.ErrNotFound
	}
	expected := advance.UpdatedAt
	if err := advance.Cancel(actor, reason, s.now()); err != nil {
		return nil, err
	}
	if s.journal != nil && advance.JournalEntryID != "" {
		if _, err := s.journal.Reverse(ctx, advance.JournalEntryID, tenantID, "cancelamento do adiantamento: "+reason, actor); err != nil {
			return nil, fmt.Errorf("estornar adiantamento no razão: %w", err)
		}
	}
	if err := s.repo.Update(ctx, advance, expected); err != nil {
		return nil, err
	}
	return advance, nil
}

func (s *BrokerAdvanceService) GetByID(ctx context.Context, tenantID, id string) (*broker_advance.Advance, error) {
	return s.repo.GetByID(ctx, tenantID, id)
}

func (s *BrokerAdvanceService) List(ctx context.Context, tenantID string, filter broker_advance.ListFilter) ([]*broker_advance.Advance, error) {
	return s.repo.List(ctx, tenantID, filter)
}

func (s *BrokerAdvanceService) Balances(ctx context.Context, tenantID string) ([]broker_advance.BrokerBalance, error) {
	return s.repo.Balances(ctx, tenantID)
}

var _ = errors.Is
