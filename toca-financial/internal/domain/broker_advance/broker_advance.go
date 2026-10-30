// Package broker_advance modela o adiantamento que a imobiliária faz ao corretor
// e a recuperação do valor por abatimento nas comissões seguintes.
//
// O adiantamento é um ATIVO: dinheiro que saiu do caixa e será recebido de volta
// (em dinheiro ou em trabalho). A concessão debita o valor a receber e credita
// caixa; cada abatimento credita o valor a receber e reduz o que o corretor
// recebe na comissão. Diferente do tributo retido, NÃO existe obrigação a
// recolher aqui — é dinheiro da própria imobiliária voltando.
package broker_advance

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusOpen      Status = "OPEN"
	StatusSettled   Status = "SETTLED"
	StatusCancelled Status = "CANCELLED"
)

var (
	ErrInvalidInput = errors.New("invalid broker advance input")
	ErrNotFound     = errors.New("broker advance not found")
	// ErrHasSettlements: adiantamento já abatido não se cancela — os abatimentos
	// estão nas comissões e desfazê-los mudaria o líquido já lançado.
	ErrHasSettlements = errors.New("broker advance already has settlements")
)

// Settlement registra quanto de uma comissão abateu este adiantamento.
type Settlement struct {
	CommissionID string    `json:"commission_id"`
	DeductionID  string    `json:"deduction_id"`
	ContractID   string    `json:"contract_id,omitempty"`
	AmountCents  int64     `json:"amount_cents"`
	At           time.Time `json:"at"`
}

type HistoryEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor,omitempty"`
	Note   string    `json:"note,omitempty"`
}

type Advance struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	BrokerID     string `json:"broker_id"`
	ContractID   string `json:"contract_id,omitempty"`
	DealID       string `json:"deal_id,omitempty"`
	AmountCents  int64  `json:"amount_cents"`
	SettledCents int64  `json:"settled_cents"`
	Status       Status `json:"status"`
	Description  string `json:"description,omitempty"`
	// GrantedAt é a data do adiantamento (competência do fato econômico).
	GrantedAt time.Time `json:"granted_at"`
	// JournalEntryID é o lançamento da concessão (débito a receber / crédito
	// caixa). Vazio quando o serviço roda sem journal ligado (testes).
	JournalEntryID string         `json:"journal_entry_id,omitempty"`
	Settlements    []Settlement   `json:"settlements,omitempty"`
	History        []HistoryEntry `json:"history,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

func (a *Advance) OutstandingCents() int64 {
	if a == nil {
		return 0
	}
	outstanding := a.AmountCents - a.SettledCents
	if outstanding < 0 {
		return 0
	}
	return outstanding
}

func New(tenantID, brokerID string, amountCents int64, description string, grantedAt time.Time) (*Advance, error) {
	if strings.TrimSpace(tenantID) == "" || strings.TrimSpace(brokerID) == "" {
		return nil, fmt.Errorf("%w: tenant_id e broker_id são obrigatórios", ErrInvalidInput)
	}
	if amountCents <= 0 {
		return nil, fmt.Errorf("%w: valor do adiantamento deve ser maior que zero", ErrInvalidInput)
	}
	if grantedAt.IsZero() {
		grantedAt = time.Now()
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	a := &Advance{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		BrokerID:    strings.TrimSpace(brokerID),
		AmountCents: amountCents,
		Status:      StatusOpen,
		Description: strings.TrimSpace(description),
		GrantedAt:   grantedAt.UTC().Truncate(time.Microsecond),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	a.addHistory("granted", "", a.Description)
	return a, nil
}

// ApplySettlement abate até o saldo e devolve quanto foi abatido (0 quando já
// estava quitado). Idempotente pelo par (comissão, desconto): repetir o mesmo
// lançamento não abate duas vezes.
func (a *Advance) ApplySettlement(s Settlement, at time.Time) (int64, error) {
	if a.Status == StatusCancelled {
		return 0, ErrNotFound
	}
	if s.AmountCents <= 0 || strings.TrimSpace(s.CommissionID) == "" || strings.TrimSpace(s.DeductionID) == "" {
		return 0, fmt.Errorf("%w: abatimento exige comissão, desconto e valor", ErrInvalidInput)
	}
	for _, existing := range a.Settlements {
		if existing.CommissionID == s.CommissionID && existing.DeductionID == s.DeductionID {
			return 0, nil
		}
	}
	outstanding := a.OutstandingCents()
	if outstanding <= 0 {
		return 0, nil
	}
	applied := s.AmountCents
	if applied > outstanding {
		applied = outstanding
	}
	if at.IsZero() {
		at = time.Now()
	}
	s.AmountCents = applied
	s.At = at.UTC().Truncate(time.Microsecond)
	a.Settlements = append(a.Settlements, s)
	a.SettledCents += applied
	a.touch(at)
	if a.OutstandingCents() == 0 {
		a.Status = StatusSettled
		a.addHistory("settled", "", "saldo quitado")
	}
	return applied, nil
}

// ReverseSettlement devolve o abatimento de um lançamento (desconto removido).
func (a *Advance) ReverseSettlement(commissionID, deductionID string, at time.Time) (int64, error) {
	remaining := make([]Settlement, 0, len(a.Settlements))
	var reversed int64
	for _, settlement := range a.Settlements {
		if settlement.CommissionID == commissionID && (deductionID == "" || settlement.DeductionID == deductionID) {
			reversed += settlement.AmountCents
			continue
		}
		remaining = append(remaining, settlement)
	}
	if reversed == 0 {
		return 0, nil
	}
	a.Settlements = remaining
	a.SettledCents -= reversed
	if a.SettledCents < 0 {
		a.SettledCents = 0
	}
	if a.Status == StatusSettled {
		a.Status = StatusOpen
	}
	a.touch(at)
	a.addHistory("settlement_reversed", "", "abatimento desfeito")
	return reversed, nil
}

// Cancel encerra um adiantamento sem abatimentos.
func (a *Advance) Cancel(actor, reason string, at time.Time) error {
	if a.Status == StatusCancelled {
		return nil
	}
	if len(a.Settlements) > 0 || a.SettledCents > 0 {
		return ErrHasSettlements
	}
	if at.IsZero() {
		at = time.Now()
	}
	a.Status = StatusCancelled
	a.touch(at)
	a.addHistory("cancelled", actor, reason)
	return nil
}

func (a *Advance) touch(at time.Time) {
	if at.IsZero() {
		at = time.Now()
	}
	a.UpdatedAt = at.UTC().Truncate(time.Microsecond)
}

func (a *Advance) addHistory(action, actor, note string) {
	a.History = append(a.History, HistoryEntry{At: a.UpdatedAt, Action: action, Actor: actor, Note: note})
}
