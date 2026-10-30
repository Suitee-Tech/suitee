// Package journal define o ledger contábil de dupla entrada do domínio
// financeiro. Um Entry é um lançamento imutável e append-only composto por
// pernas (Legs) de débito e crédito cuja soma é sempre zero. A correção de um
// lançamento é feita por reversão estruturada (um novo Entry balanceado que
// espelha o original com sinais invertidos), nunca por edição ou remoção.
//
// Política de caixa/competência: os lançamentos registram o fato econômico no
// momento em que ele nasce (competência). A liquidação em caixa é um lançamento
// separado que move o saldo entre a conta patrimonial de caixa e a conta de
// recebíveis/passivos correspondente. Relatórios de caixa somam apenas pernas
// que tocam a conta de caixa; relatórios de competência somam pernas de
// receita/despesa pelo período de competência (Date), independentemente da data
// de pagamento.
package journal

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Account identifica uma conta do plano de contas. O sinal econômico da conta
// é derivado da direção da perna (débito/crédito), não do próprio Account.
type Account string

// Plano de contas mínimo do domínio financeiro.
const (
	// Ativos.
	AccountCash               Account = "cash"                // caixa
	AccountAccountsReceivable Account = "accounts_receivable" // contas a receber (inquilino/comprador)
	// Passivos.
	AccountCommissionPayable Account = "commission_payable" // comissão a pagar ao corretor
	AccountOwnerPayable      Account = "owner_payable"      // repasse a pagar ao proprietário
	AccountPartnerPayable    Account = "partner_payable"    // valor devido a empresa parceira
	AccountRoyaltyPayable    Account = "royalty_payable"    // royalty devido a franqueadora
	AccountTaxWithheld       Account = "tax_withheld"       // tributo retido a recolher
	AccountOtherPayable      Account = "other_payable"      // outras retenções a destinar
	AccountWithdrawalPayable Account = "withdrawal_payable" // saque a pagar
	// Patrimônio líquido.
	AccountEquity Account = "equity" // patrimônio líquido da imobiliária (disponível para saque)
	// Resultado.
	AccountRevenue           Account = "revenue"            // receita (aluguel/venda/encargos)
	AccountCommissionExpense Account = "commission_expense" // despesa de comissão
	AccountExpense           Account = "expense"            // despesa genérica
)

// ValidAccounts é o conjunto de contas conhecidas.
var ValidAccounts = map[Account]bool{
	AccountCash:               true,
	AccountAccountsReceivable: true,
	AccountCommissionPayable:  true,
	AccountOwnerPayable:       true,
	AccountPartnerPayable:     true,
	AccountRoyaltyPayable:     true,
	AccountTaxWithheld:        true,
	AccountOtherPayable:       true,
	AccountWithdrawalPayable:  true,
	AccountEquity:             true,
	AccountRevenue:            true,
	AccountCommissionExpense:  true,
	AccountExpense:            true,
}

// Direction é o lado do lançamento em uma perna.
type Direction string

const (
	Debit  Direction = "debit"
	Credit Direction = "credit"
)

// Status é o ciclo de vida de um Entry. POSTED é o estado canônico e
// imutável; REVERSED marca que um Entry de reversão o compensou.
type Status string

const (
	StatusPosted   Status = "POSTED"
	StatusReversed Status = "REVERSED"
)

// Leg é uma perna de um lançamento: uma conta, uma direção e um valor em
// centavos estritamente positivo. A direção carrega o sinal econômico.
type Leg struct {
	Account            Account   `json:"account"`
	Direction          Direction `json:"direction"`
	AmountCents        int64     `json:"amount_cents"`
	AllocationID       string    `json:"allocation_id,omitempty"`
	ChargeItemID       string    `json:"charge_item_id,omitempty"`
	ManagedBillID      string    `json:"managed_bill_id,omitempty"`
	TransferID         string    `json:"transfer_id,omitempty"`
	BeneficiaryPartyID string    `json:"beneficiary_party_id,omitempty"`
	RuleVersionID      string    `json:"rule_version_id,omitempty"`
	AllocationPurpose  string    `json:"allocation_purpose,omitempty"`
	RelatedEntityID    string    `json:"related_entity_id,omitempty"`
	// Flow tags the economic modality of the posting ("sale" | "rent") so the
	// plano de contas can break down revenue (and other economic accounts) by
	// intermediação de compra/venda versus locação without duplicating accounts.
	Flow string `json:"flow,omitempty"`
}

// Entry é um lançamento contábil imutável de dupla entrada.
type Entry struct {
	ID            string `json:"id"`
	TenantID      string `json:"tenant_id"`
	ContractID    string `json:"contract_id,omitempty"`
	OperationID   string `json:"operation_id,omitempty"`
	SourceEventID string `json:"source_event_id,omitempty"`
	// CashFlowSource marca entradas cujo movimento de caixa não tem documento
	// operacional (cobrança, repasse, transação) — o relatório de fluxo de
	// caixa passa a lê-las do razão. Vazio nas entradas que já são contadas por
	// outra fonte, evitando duplicidade.
	CashFlowSource string    `json:"cash_flow_source,omitempty"`
	Date           time.Time `json:"date"`
	// ReconciliationDate marca quando o lançamento foi conciliado com uma
	// fonte externa (extrato/manual). Nulo enquanto não conciliado — ausente
	// nos fluxos que ainda dependem de conciliação manual (FIN-E2E-35).
	ReconciliationDate *time.Time `json:"reconciliation_date,omitempty"`
	Description        string     `json:"description"`
	Legs               []Leg      `json:"legs"`
	Status             Status     `json:"status"`
	ReversalOf         string     `json:"reversal_of,omitempty"`
	ReversedBy         string     `json:"reversed_by,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// New monta um Entry balanceado. Retorna erro se o lançamento não fechar em
// zero (soma de débitos == soma de créditos), se não houver ao menos duas
// pernas, se alguma conta for desconhecida, se alguma perna tiver valor não
// positivo ou se uma mesma conta aparecer nos dois lados do mesmo lançamento.
func New(tenantID, contractID, description string, date time.Time, legs []Leg) (*Entry, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidEntry)
	}
	// PostgreSQL timestamptz persists microseconds. Canonicalizing before
	// serializing the JSON witness keeps journal dates identical to the typed
	// lifecycle timestamps used by payment/reversal guards.
	date = date.UTC().Truncate(time.Microsecond)
	entry := &Entry{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		ContractID:  strings.TrimSpace(contractID),
		Date:        date,
		Description: description,
		Legs:        append([]Leg(nil), legs...),
		Status:      StatusPosted,
	}
	if err := entry.validateBalance(); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	entry.CreatedAt = now
	entry.UpdatedAt = now
	return entry, nil
}

// validateBalance aplica as invariantes de dupla entrada.
func (e *Entry) validateBalance() error {
	if len(e.Legs) < 2 {
		return fmt.Errorf("%w: an entry requires at least two legs", ErrInvalidEntry)
	}
	var debits, credits int64
	seen := make(map[Account]Direction, len(e.Legs))
	for _, leg := range e.Legs {
		if !ValidAccounts[leg.Account] {
			return fmt.Errorf("%w: unknown account %q", ErrInvalidEntry, leg.Account)
		}
		if leg.AmountCents <= 0 {
			return fmt.Errorf("%w: leg on account %q must have a positive amount", ErrInvalidEntry, leg.Account)
		}
		switch leg.Direction {
		case Debit:
			debits += leg.AmountCents
		case Credit:
			credits += leg.AmountCents
		default:
			return fmt.Errorf("%w: unknown direction %q", ErrInvalidEntry, leg.Direction)
		}
		if prev, ok := seen[leg.Account]; ok && prev != leg.Direction {
			return fmt.Errorf("%w: account %q appears on both sides of the same entry", ErrInvalidEntry, leg.Account)
		}
		seen[leg.Account] = leg.Direction
	}
	if debits != credits {
		return fmt.Errorf("%w: entry is not balanced (debit=%d credit=%d)", ErrUnbalanced, debits, credits)
	}
	return nil
}

// Balanced verifica se o lançamento fecha em zero sem validar contas.
func (e *Entry) Balanced() bool {
	var sum int64
	for _, leg := range e.Legs {
		switch leg.Direction {
		case Debit:
			sum += leg.AmountCents
		case Credit:
			sum -= leg.AmountCents
		}
	}
	return sum == 0
}

// Reverse produz um Entry de reversão balanceado que espelha este lançamento
// com as pernas invertidas. O lançamento original permanece intacto; o caller
// deve persistir o Entry retornado e marcar o original como REVERSED.
func (e *Entry) Reverse(reason, operatorID string) (*Entry, error) {
	return e.ReverseAt(reason, operatorID, time.Time{})
}

// ReverseAt creates the immutable mirror at one caller-supplied effective
// timestamp. Money-moving workflows use the same timestamp for the journal,
// aggregate history and balance movement so a reversal cannot straddle two
// accounting periods at a clock boundary.
func (e *Entry) ReverseAt(reason, operatorID string, effectiveAt time.Time) (*Entry, error) {
	if e.Status != StatusPosted {
		return nil, fmt.Errorf("%w: only POSTED entries can be reversed (current: %s)", ErrInvalidStatusTransition, e.Status)
	}
	reason, operatorID = strings.TrimSpace(reason), strings.TrimSpace(operatorID)
	if reason == "" || operatorID == "" {
		return nil, fmt.Errorf("%w: reversal reason and operator are required", ErrInvalidEntry)
	}
	legs := make([]Leg, len(e.Legs))
	for i, leg := range e.Legs {
		legs[i] = leg
		switch leg.Direction {
		case Debit:
			legs[i].Direction = Credit
		case Credit:
			legs[i].Direction = Debit
		}
	}
	if effectiveAt.IsZero() {
		effectiveAt = time.Now()
	}
	effectiveAt = effectiveAt.UTC().Truncate(time.Microsecond)
	reversed, err := New(e.TenantID, e.ContractID, "Estorno: "+reason+" (ref: "+e.ID+")", effectiveAt, legs)
	if err != nil {
		return nil, err
	}
	reversed.ReversalOf = e.ID
	reversed.ReversedBy = operatorID
	reversed.OperationID = e.OperationID
	return reversed, nil
}

// MarkReversed move um Entry POSTED para o estado terminal REVERSED. É a única
// mutação permitida em um lançamento já persistido.
func (e *Entry) MarkReversed() error {
	if e.Status != StatusPosted {
		return fmt.Errorf("%w: only POSTED entries can be reversed (current: %s)", ErrInvalidStatusTransition, e.Status)
	}
	e.Status = StatusReversed
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// MarkReconciled registra a data de conciliação do lançamento. Aceita apenas
// entradas POSTED; chamadas repetidas apenas atualizam a data.
func (e *Entry) MarkReconciled(reconciledAt time.Time) error {
	if e.Status != StatusPosted {
		return fmt.Errorf("%w: only POSTED entries can be reconciled (current: %s)", ErrInvalidStatusTransition, e.Status)
	}
	if reconciledAt.IsZero() {
		reconciledAt = time.Now().UTC()
	}
	reconciledAt = reconciledAt.UTC().Truncate(time.Microsecond)
	e.ReconciliationDate = &reconciledAt
	e.UpdatedAt = time.Now().UTC()
	return nil
}

// AccountBalance agrega as pernas de um conjunto de lançamentos por conta,
// devolvendo o saldo em débito (positivo) e crédito (negativo) em centavos.
func AccountBalance(entries []*Entry) map[Account]int64 {
	out := make(map[Account]int64)
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		for _, leg := range entry.Legs {
			switch leg.Direction {
			case Debit:
				out[leg.Account] += leg.AmountCents
			case Credit:
				out[leg.Account] -= leg.AmountCents
			}
		}
	}
	return out
}

// IsZeroReports se o saldo líquido de todas as pernas de um conjunto de
// lançamentos é zero — a invariante global de um journal íntegro.
func IsZeroReports(entries []*Entry) bool {
	var sum int64
	for account, balance := range AccountBalance(entries) {
		_ = account
		sum += balance
	}
	return sum == 0
}

// Erros sentinela do domínio.
var (
	ErrUnbalanced              = errors.New("journal entry is not balanced")
	ErrInvalidEntry            = errors.New("invalid journal entry")
	ErrInvalidStatusTransition = errors.New("invalid journal status transition")
	ErrEntryAlreadyExists      = errors.New("journal entry already exists")
	ErrSourceEventConflict     = errors.New("journal source event already exists with different posting")
)
