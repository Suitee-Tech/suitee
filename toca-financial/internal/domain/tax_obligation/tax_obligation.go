// Package tax_obligation materializa o tributo retido na comissão do corretor
// como obrigação a recolher da imobiliária.
//
// O plano de contas já prevê "Tributos retidos a recolher" (2.1.05) e a
// liquidação da comissão credita essa conta quando há desconto de IRRF/INSS/ISS.
// O que faltava era o passivo ter vida: saber quanto foi retido, em que
// competência, até quando deve ser recolhido e quando foi pago — e é isso que
// esta agregação representa, uma linha por (tipo, competência).
//
// A obrigação é DERIVADA da comissão: os itens apontam para o desconto que a
// originou, então uma recomputação (retry, correção, remoção do desconto) sempre
// reconstrói o mesmo total. Nada aqui é digitado à mão.
package tax_obligation

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Kind string

const (
	KindIRRF   Kind = "IRRF"
	KindINSS   Kind = "INSS"
	KindISS    Kind = "ISS"
	KindOutros Kind = "OUTROS"
)

// ValidKinds espelha commission.DeductionKind: o que o operador pode lançar na
// comissão é o que pode virar obrigação.
func ValidKind(k Kind) bool {
	switch k {
	case KindIRRF, KindINSS, KindISS, KindOutros:
		return true
	default:
		return false
	}
}

type Status string

const (
	StatusOpen      Status = "OPEN"      // retido e ainda não recolhido
	StatusPaid      Status = "PAID"      // recolhido com comprovante
	StatusCancelled Status = "CANCELLED" // sem itens (descontos removidos antes do recolhimento)
)

var (
	ErrInvalidInput  = errors.New("invalid tax obligation input")
	ErrInvalidKind   = errors.New("invalid tax obligation kind")
	ErrNotFound      = errors.New("tax obligation not found")
	ErrNotOpen       = errors.New("tax obligation is not open")
	ErrProofRequired = errors.New("proof is required to settle a tax obligation")
	// ErrAlreadySettled: o tributo já foi recolhido e o desconto que o originou
	// não pode mais ser removido — o dinheiro saiu. Estorno é o caminho, não a
	// edição.
	ErrAlreadySettled = errors.New("tax obligation was already settled; reverse it instead of removing the deduction")
)

// Item liga a obrigação à origem: a comissão e o desconto que a produziram.
type Item struct {
	CommissionID string    `json:"commission_id"`
	DeductionID  string    `json:"deduction_id"`
	ContractID   string    `json:"contract_id,omitempty"`
	BrokerID     string    `json:"broker_id,omitempty"`
	AmountCents  int64     `json:"amount_cents"`
	Competence   string    `json:"competence"` // YYYY-MM do lançamento do desconto
	At           time.Time `json:"at"`
}

type HistoryEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor,omitempty"`
	Note   string    `json:"note,omitempty"`
}

type Obligation struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	Kind       Kind   `json:"kind"`
	Competence string `json:"competence"`
	// AmountCents é o acumulado da competência (soma dos itens ativos).
	AmountCents int64     `json:"amount_cents"`
	Status      Status    `json:"status"`
	DueDate     time.Time `json:"due_date"`
	Items       []Item    `json:"items,omitempty"`

	PaidAt        *time.Time `json:"paid_at,omitempty"`
	PaidBy        string     `json:"paid_by,omitempty"`
	ProofURL      string     `json:"proof_url,omitempty"`
	ProofFilename string     `json:"proof_filename,omitempty"`
	Note          string     `json:"note,omitempty"`
	// JournalEntryID é o lançamento do recolhimento (débito 2.1.05 / crédito
	// caixa). Vazio enquanto não houve baixa.
	JournalEntryID string `json:"journal_entry_id,omitempty"`

	History   []HistoryEntry `json:"history,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// CompetenceOf devolve a competência (YYYY-MM) de um instante em UTC.
func CompetenceOf(at time.Time) string {
	return at.UTC().Format("2006-01")
}

// DueDateFor devolve o vencimento padrão do tributo na competência dada.
//
// Regras padrão do contexto da imobiliária (documentadas porque variam por
// município/tipo de retenção e podem ser ajustadas depois):
//
//   - IRRF e INSS: dia 20 do mês seguinte à competência;
//   - ISS: dia 10 do mês seguinte (o município pode antecipar ou prorrogar);
//   - OUTROS: dia 20.
//
// Vencimento que cai em dia não útil é PRORROGADO para o primeiro dia útil
// seguinte (regra usual dos tributos federais: "se o vencimento cair em dia não
// útil, prorroga-se para o dia útil seguinte"). Antecipar seria dizer ao operador
// que o prazo acabou antes de acabar — e marcar como atrasado o que a lei ainda
// não considera atrasado. Feriados municipais/nacionais não entram na conta:
// não há calendário de feriados no sistema, então o ajuste cobre fim de semana.
func DueDateFor(kind Kind, competence string) (time.Time, error) {
	month, err := time.ParseInLocation("2006-01", strings.TrimSpace(competence), time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: competência inválida %q (esperado AAAA-MM)", ErrInvalidInput, competence)
	}
	day := 20
	if kind == KindISS {
		day = 10
	} else if !ValidKind(kind) {
		return time.Time{}, fmt.Errorf("%w: %q", ErrInvalidKind, kind)
	}
	due := time.Date(month.Year(), month.Month(), day, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	for due.Weekday() == time.Saturday || due.Weekday() == time.Sunday {
		due = due.AddDate(0, 0, 1)
	}
	return due, nil
}

// New cria uma obrigação aberta para a competência.
func New(tenantID string, kind Kind, competence string, at time.Time) (*Obligation, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, fmt.Errorf("%w: tenant_id is required", ErrInvalidInput)
	}
	if !ValidKind(kind) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidKind, kind)
	}
	due, err := DueDateFor(kind, competence)
	if err != nil {
		return nil, err
	}
	if at.IsZero() {
		at = time.Now()
	}
	now := at.UTC().Truncate(time.Microsecond)
	o := &Obligation{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		Kind:       kind,
		Competence: strings.TrimSpace(competence),
		Status:     StatusOpen,
		DueDate:    due,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	o.addHistory("opened", "", "")
	return o, nil
}

// ApplyItem registra (ou atualiza) o item de um desconto e recalcula o total.
// Idempotente pelo par (comissão, desconto): recomputar não duplica valor.
func (o *Obligation) ApplyItem(item Item, at time.Time) (bool, error) {
	if o.Status != StatusOpen {
		return false, ErrNotOpen
	}
	if strings.TrimSpace(item.CommissionID) == "" || strings.TrimSpace(item.DeductionID) == "" {
		return false, fmt.Errorf("%w: commission_id and deduction_id are required", ErrInvalidInput)
	}
	if item.AmountCents < 0 {
		return false, fmt.Errorf("%w: amount must not be negative", ErrInvalidInput)
	}
	if item.At.IsZero() {
		item.At = at.UTC().Truncate(time.Microsecond)
	}
	changed := false
	for i := range o.Items {
		if o.Items[i].CommissionID == item.CommissionID && o.Items[i].DeductionID == item.DeductionID {
			if o.Items[i].AmountCents == item.AmountCents && o.Items[i].Competence == item.Competence {
				return false, nil
			}
			o.Items[i] = item
			changed = true
			break
		}
	}
	if !changed {
		o.Items = append(o.Items, item)
	}
	o.recompute(at)
	return true, nil
}

// RemoveCommissionItems tira os itens de uma comissão (desconto removido ou
// valor alterado) e devolve quanto saiu do total.
func (o *Obligation) RemoveCommissionItems(commissionID string, at time.Time) (int64, error) {
	if o.Status == StatusPaid {
		return 0, ErrAlreadySettled
	}
	if o.Status == StatusCancelled {
		return 0, nil
	}
	remaining := make([]Item, 0, len(o.Items))
	var removed int64
	for _, item := range o.Items {
		if item.CommissionID == commissionID {
			removed += item.AmountCents
			continue
		}
		remaining = append(remaining, item)
	}
	if removed == 0 {
		return 0, nil
	}
	o.Items = remaining
	o.recompute(at)
	return removed, nil
}

func (o *Obligation) recompute(at time.Time) {
	var total int64
	for _, item := range o.Items {
		total += item.AmountCents
	}
	o.AmountCents = total
	if at.IsZero() {
		at = time.Now()
	}
	o.UpdatedAt = at.UTC().Truncate(time.Microsecond)
	if len(o.Items) == 0 && o.Status == StatusOpen {
		o.Status = StatusCancelled
		o.addHistory("cancelled", "", "sem descontos ativos")
	}
}

// Pay marca o recolhimento. Exige comprovante: o pagamento acontece fora do
// sistema (DARF/GPS/guia municipal) e o comprovante é a evidência.
func (o *Obligation) Pay(actor, proofURL, proofFilename, note string, at time.Time) error {
	if o.Status == StatusPaid {
		return nil
	}
	if o.Status != StatusOpen {
		return ErrNotOpen
	}
	if o.AmountCents <= 0 {
		return fmt.Errorf("%w: nothing to settle", ErrInvalidInput)
	}
	if strings.TrimSpace(proofURL) == "" {
		return ErrProofRequired
	}
	if at.IsZero() {
		at = time.Now()
	}
	paid := at.UTC().Truncate(time.Microsecond)
	o.Status = StatusPaid
	o.PaidAt = &paid
	o.PaidBy = actor
	o.ProofURL = strings.TrimSpace(proofURL)
	o.ProofFilename = strings.TrimSpace(proofFilename)
	o.Note = strings.TrimSpace(note)
	o.UpdatedAt = paid
	o.addHistory("paid", actor, o.ProofURL)
	return nil
}

// Cancel encerra uma obrigação sem itens (nunca chegou a ser recolhida).
func (o *Obligation) Cancel(actor, reason string, at time.Time) error {
	if o.Status == StatusCancelled {
		return nil
	}
	if o.Status == StatusPaid {
		return ErrAlreadySettled
	}
	if len(o.Items) > 0 {
		return fmt.Errorf("%w: obrigação com descontos ativos não pode ser cancelada", ErrInvalidInput)
	}
	if at.IsZero() {
		at = time.Now()
	}
	o.Status = StatusCancelled
	o.UpdatedAt = at.UTC().Truncate(time.Microsecond)
	o.addHistory("cancelled", actor, reason)
	return nil
}

// Overdue informa se a obrigação passou do vencimento sem recolhimento.
func (o *Obligation) Overdue(now time.Time) bool {
	return o.Status == StatusOpen && o.AmountCents > 0 && now.UTC().After(o.DueDate)
}

func (o *Obligation) addHistory(action, actor, note string) {
	o.History = append(o.History, HistoryEntry{At: o.UpdatedAt, Action: action, Actor: actor, Note: note})
}
