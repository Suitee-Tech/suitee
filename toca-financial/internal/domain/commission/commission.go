package commission

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusApproved  Status = "APPROVED"
	StatusPaid      Status = "PAID"
	StatusCancelled Status = "CANCELLED"
	StatusReversed  Status = "REVERSED"
)

// DeductionKind cobre os impostos retidos típicos sobre comissão de corretor
// no Brasil. ISS retido na fonte varia por município — quando o operador
// configura retenção, registra como ISS; quando não, fica de fora.
type DeductionKind string

const (
	DeductionIRRF   DeductionKind = "IRRF"
	DeductionINSS   DeductionKind = "INSS"
	DeductionISS    DeductionKind = "ISS"
	DeductionOutros DeductionKind = "OUTROS"
	// DeductionAdvance é o abatimento de ADIANTAMENTO da imobiliária ao corretor.
	// Não é tributo: não gera obrigação a recolher — credita o valor a receber do
	// corretor no lançamento de liquidação (ver transaction_service.go).
	DeductionAdvance DeductionKind = "ADVANCE"
)

func ValidateDeductionKind(k DeductionKind) error {
	switch k {
	case DeductionIRRF, DeductionINSS, DeductionISS, DeductionOutros, DeductionAdvance:
		return nil
	}
	return errors.New("invalid deduction kind")
}

// Deduction é um abatimento aplicado sobre o valor bruto da comissão antes
// do pagamento ao corretor. Valor em reais (não em centavos) para manter
// alinhamento com Commission.Amount (float64) — refator para cents seria
// uma mudança de domain ortogonal a esta entrega.
type Deduction struct {
	ID          string        `json:"id,omitempty"`
	Kind        DeductionKind `json:"kind"`
	Description string        `json:"description,omitempty"`
	Amount      float64       `json:"amount"`
	// RatePct/BaseCents registram COMO o valor foi calculado quando o lançamento
	// é por alíquota (INSS de autônomo ~20%, ISS municipal 2%–5%): sem eles a
	// auditoria não distingue um valor digitado de um valor calculado, e uma
	// correção de alíquota não teria base para recalcular.
	RatePct   float64 `json:"rate_pct,omitempty"`
	BaseCents int64   `json:"base_cents,omitempty"`
	// CreatedAt é a competência do lançamento: define em que mês o valor retido
	// vira obrigação a recolher (o IRRF retido na comissão paga em setembro é
	// recolhido com competência de setembro, no vencimento do mês seguinte).
	// Sem ele, uma obrigação derivada não teria como ser recalculada de forma
	// estável — o mesmo desconto cairia em competências diferentes a cada
	// recomputação.
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// FundingEntry records how a rental receipt backed this commission. It is
// separate from the contractual amount: a pro-rata first rent can fund only a
// slice while the remaining obligation carries into subsequent competences.
type FundingEntry struct {
	PaymentID      string    `json:"payment_id"`
	ChargeID       string    `json:"charge_id"`
	ReferenceMonth string    `json:"reference_month"`
	AmountCents    int64     `json:"amount_cents"`
	FundedAt       time.Time `json:"funded_at"`
}

// Commission representa uma fatia da comissão de um deal devida a um
// broker específico. Antes desta entrega, a struct era genérica
// (DealID, BrokerID, Amount); a versão expandida carrega:
//
//   - Role (captador/visitador/fechamento/co_captador) e Percentage para
//     reconstituir como o split foi feito.
//   - GrossAmount + Deductions[] + NetAmount para mostrar transparência
//     ao broker no detalhamento e para a base "net_commission" do royalty
//     (event_handler.go royaltyBaseAmount).
//   - ContractID denormalizado: o contrato sobrevive ao deal e a auditoria
//     fiscal (DIMOB / Informe) precisa achar commissions a partir dele.
//   - PayoutOrderID: quando o settlement materializa um payout_order para
//     pagar a commission, gravamos aqui o vínculo para consulta reversa.
//
// Amount permanece como total bruto pré-deduções (para não quebrar quem
// já lê esse campo); NetAmount é a fonte da verdade depois de deductions.
//
// OwnerID, CustomerID, PropertyID são desnormalizados do deal para
// auditoria fiscal (DIMOB / Informe de Rendimentos) sem necessidade
// de lookup adicional ao toca-deal.
type Commission struct {
	ID            string      `json:"id" bson:"_id"`
	DealID        string      `json:"deal_id" bson:"deal_id"`
	BrokerID      string      `json:"broker_id" bson:"broker_id"`
	Role          string      `json:"role,omitempty"`
	Percentage    float64     `json:"percentage,omitempty"`
	Amount        float64     `json:"amount" bson:"amount"`
	GrossAmount   float64     `json:"gross_amount,omitempty"`
	Deductions    []Deduction `json:"deductions,omitempty"`
	NetAmount     float64     `json:"net_amount,omitempty"`
	Currency      string      `json:"currency" bson:"currency"`
	Status        Status      `json:"status" bson:"status"`
	ContractID    string      `json:"contract_id,omitempty"`
	OwnerID       string      `json:"owner_id,omitempty"`
	CustomerID    string      `json:"customer_id,omitempty"`
	PropertyID    string      `json:"property_id,omitempty"`
	PayoutOrderID string      `json:"payout_order_id,omitempty"`
	SourceEventID string      `json:"source_event_id,omitempty"`
	// LedgerTransactionID is the stable posting created for this obligation by
	// the manual posting command. It is deliberately the Transaction primary
	// key: one deterministic identity is enough to make lifecycle synchronization
	// and reconciliation unambiguous, including when plan_hash is absent.
	LedgerTransactionID string `json:"ledger_transaction_id,omitempty"`
	PlanHash            string `json:"plan_hash,omitempty"` // FIN-COM-04: links commission to its resolved plan
	// FundingRequiredCents gates automatic sale and rental commissions on
	// collected receipts. Explicit manual obligations may keep zero.
	FundingRequiredCents int64          `json:"funding_required_cents,omitempty"`
	FundedAmountCents    int64          `json:"funded_amount_cents,omitempty"`
	FundingEntries       []FundingEntry `json:"funding_entries,omitempty"`
	ProofURL             string         `json:"proof_url,omitempty"`
	Note                 string         `json:"note,omitempty"`
	CreatedAt            time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at" bson:"updated_at"`
	PaidAt               *time.Time     `json:"paid_at,omitempty" bson:"paid_at,omitempty"`
	CancelledAt          *time.Time     `json:"cancelled_at,omitempty"`
	TenantID             string         `json:"tenant_id" bson:"tenant_id"`
	// RequestedBy / ApprovedBy — fechamento do achado A13 (segregação de funções)
	// no caminho STANDALONE. Antes, o ator existia apenas na trilha de auditoria:
	// sem o autor no estado não havia como comparar criador × aprovador nem
	// aprovador × pagador, e `financeiro` (que tem `finance:write` e
	// `finance:approve_transfer`) criava e aprovava a própria comissão.
	// Vazio é dado legado/worker e NÃO cria conflito (mesma decisão dos fluxos de
	// transfer/payout/withdrawal).
	RequestedBy string `json:"requested_by,omitempty" bson:"requested_by,omitempty"`
	ApprovedBy  string `json:"approved_by,omitempty" bson:"approved_by,omitempty"`
}

// FundingOutstandingCents reports how much of a rental commission still
// needs collected cash before it can be settled. Zero funding requirement is
// the backward-compatible "not funding-gated" state.
func (c *Commission) FundingOutstandingCents() int64 {
	if c == nil || c.FundingRequiredCents <= 0 {
		return 0
	}
	outstanding := c.FundingRequiredCents - c.FundedAmountCents
	if outstanding < 0 {
		return 0
	}
	return outstanding
}

// EnableRentalFundingGate upgrades a still-pending legacy rental commission
// to the collected-cash lifecycle. It is deliberately fail-closed for
// approved/paid rows: without a receipt trail there is no safe way to infer
// whether that historical obligation was already funded.
func (c *Commission) EnableRentalFundingGate(requiredCents int64) (bool, error) {
	return c.EnableFundingGate(requiredCents)
}

// EnableFundingGate freezes the receipt coverage required before settlement.
func (c *Commission) EnableFundingGate(requiredCents int64) (bool, error) {
	if c == nil || requiredCents <= 0 {
		return false, fmt.Errorf("%w: funding requirement must be positive", ErrFundingGateConflict)
	}
	gross := c.GrossAmount
	if gross == 0 {
		gross = c.Amount
	}
	if grossCents := int64(math.Round(gross * 100)); grossCents != requiredCents {
		return false, fmt.Errorf("%w: contractual amount is %d cents, requirement is %d", ErrFundingGateConflict, grossCents, requiredCents)
	}
	if c.FundingRequiredCents != 0 && c.FundingRequiredCents != requiredCents {
		return false, fmt.Errorf("%w: requirement changed from %d to %d cents", ErrFundingGateConflict, c.FundingRequiredCents, requiredCents)
	}
	if c.FundedAmountCents < 0 || c.FundedAmountCents > requiredCents {
		return false, fmt.Errorf("%w: funded amount is outside the contractual requirement", ErrFundingGateConflict)
	}
	if c.FundingRequiredCents == requiredCents {
		if c.FundingOutstandingCents() > 0 && c.Status != StatusPending {
			return false, fmt.Errorf("%w: %s commission still has an unfunded balance", ErrFundingGateConflict, c.Status)
		}
		return false, nil
	}
	if c.Status != StatusPending || c.FundedAmountCents != 0 || len(c.FundingEntries) != 0 {
		return false, fmt.Errorf("%w: only an unfunded PENDING commission can be upgraded", ErrFundingGateConflict)
	}
	c.FundingRequiredCents = requiredCents
	c.UpdatedAt = time.Now()
	return true, nil
}

// RecordFunding appends one idempotent rental-payment slice. Reusing a
// payment ID with different data is rejected and aggregate funding can never
// exceed the frozen contractual requirement.
func (c *Commission) RecordFunding(entry FundingEntry) (bool, error) {
	entry.PaymentID = strings.TrimSpace(entry.PaymentID)
	entry.ChargeID = strings.TrimSpace(entry.ChargeID)
	entry.ReferenceMonth = strings.TrimSpace(entry.ReferenceMonth)
	if c.FundingRequiredCents <= 0 {
		return false, errors.New("commission is not funding-gated")
	}
	// ChargeID is provenance for charge payments; a manual receipt may leave it
	// empty and still fund the obligation via its payment ID.
	if entry.PaymentID == "" || entry.AmountCents <= 0 || entry.FundedAt.IsZero() {
		return false, errors.New("commission funding entry is invalid")
	}
	for _, existing := range c.FundingEntries {
		if existing.PaymentID != entry.PaymentID {
			continue
		}
		if existing.ChargeID != entry.ChargeID || existing.ReferenceMonth != entry.ReferenceMonth || existing.AmountCents != entry.AmountCents {
			return false, errors.New("commission funding payment_id was reused with different data")
		}
		return false, nil
	}
	if entry.AmountCents > c.FundingOutstandingCents() {
		return false, errors.New("commission funding exceeds contractual requirement")
	}
	c.FundingEntries = append(c.FundingEntries, entry)
	c.FundedAmountCents += entry.AmountCents
	c.UpdatedAt = time.Now()
	return true, nil
}

// New cria uma commission "bruta" com role/percentage vazios — caller
// responsável por preencher quando vier de event_handler. A maioria dos
// callers legados (testes, criação manual) usa só dealID/brokerID/amount.
func New(dealID, brokerID string, amount float64, tenantID string) *Commission {
	now := time.Now()
	return &Commission{
		ID:          uuid.New().String(),
		DealID:      dealID,
		BrokerID:    brokerID,
		Amount:      amount,
		GrossAmount: amount,
		NetAmount:   amount,
		Currency:    "BRL",
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		TenantID:    tenantID,
	}
}

// NewWithRole é o construtor preferido pelo event_handler em contract.signed:
// já carrega role + percentage + contractID. Mantém New() puro para os
// callers legados (testes, criação manual sem role definido).
func NewWithRole(dealID, brokerID, contractID, role string, percentage, amount float64, tenantID string) *Commission {
	c := New(dealID, brokerID, amount, tenantID)
	c.Role = strings.ToLower(strings.TrimSpace(role))
	c.Percentage = percentage
	c.ContractID = contractID
	return c
}

// AddDeduction acrescenta uma dedução e recalcula NetAmount.
// validateDeductionRate garante que alíquota, base e valor contam a MESMA
// história. Sem isso um payload torto (ou uma edição manual) gravaria "INSS 20%
// de R$ 28.800" com valor de R$ 100 — e como o valor é o que vira obrigação a
// recolher e o que sai do líquido do corretor, a divergência ia direto para o
// pagamento. Tolerância de 1 centavo cobre o arredondamento do cliente.
func validateDeductionRate(d Deduction) error {
	if d.RatePct == 0 && d.BaseCents == 0 {
		return nil // lançamento por valor fixo (o caso legado continua valendo)
	}
	if d.RatePct <= 0 || d.RatePct > 100 {
		return fmt.Errorf("%w: alíquota deve estar entre 0 e 100 (recebido %.4f)", ErrInvalidDeductionRate, d.RatePct)
	}
	if d.BaseCents <= 0 {
		return fmt.Errorf("%w: base de cálculo é obrigatória no lançamento por alíquota", ErrInvalidDeductionRate)
	}
	expected := int64(math.Round(float64(d.BaseCents) * d.RatePct / 100))
	actual := int64(math.Round(d.Amount * 100))
	diff := expected - actual
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		return fmt.Errorf("%w: valor não corresponde à alíquota (esperado %.2f para %.4f%% de %.2f)",
			ErrInvalidDeductionRate, float64(expected)/100, d.RatePct, float64(d.BaseCents)/100)
	}
	return nil
}

func (c *Commission) AddDeduction(d Deduction) error {
	if c.Status != StatusPending {
		return ErrDeductionFrozen
	}
	if err := ValidateDeductionKind(d.Kind); err != nil {
		return err
	}
	if d.Amount < 0 {
		return errors.New("deduction amount cannot be negative")
	}
	if err := validateDeductionRate(d); err != nil {
		return err
	}
	d.ID = strings.TrimSpace(d.ID)
	if d.ID != "" {
		for _, existing := range c.Deductions {
			if existing.ID != d.ID {
				continue
			}
			if existing.Kind == d.Kind && existing.Description == d.Description && existing.Amount == d.Amount {
				return nil
			}
			return ErrDeductionConflict
		}
	}
	c.Deductions = append(c.Deductions, d)
	c.RecomputeNet()
	c.UpdatedAt = time.Now()
	return nil
}

// DeductionByID resolves a stable deduction identity. IDs make a retried POST
// a no-op without conflating two intentional deductions that happen to have
// the same kind and amount.
func (c *Commission) DeductionByID(id string) (Deduction, bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Deduction{}, false
	}
	for _, d := range c.Deductions {
		if d.ID == id {
			return d, true
		}
	}
	return Deduction{}, false
}

// RemoveDeduction descarta a dedução em `idx` (0-based) e recalcula
// NetAmount. Out-of-range vira erro — caller deve validar antes.
func (c *Commission) RemoveDeduction(idx int) error {
	if c.Status != StatusPending {
		return ErrDeductionFrozen
	}
	if idx < 0 || idx >= len(c.Deductions) {
		return errors.New("deduction index out of range")
	}
	c.Deductions = append(c.Deductions[:idx], c.Deductions[idx+1:]...)
	c.RecomputeNet()
	c.UpdatedAt = time.Now()
	return nil
}

// RemoveDeductionByID removes one deduction by its stable identity. A retry
// after a successful removal is intentionally a no-op, so it can never remove
// the line that shifted into a former array position.
func (c *Commission) RemoveDeductionByID(id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, ErrDeductionIDRequired
	}
	if c.Status != StatusPending {
		return false, ErrDeductionFrozen
	}
	for idx, deduction := range c.Deductions {
		if deduction.ID != id {
			continue
		}
		c.Deductions = append(c.Deductions[:idx], c.Deductions[idx+1:]...)
		c.RecomputeNet()
		c.UpdatedAt = time.Now()
		return true, nil
	}
	return false, nil
}

// SumDeductions devolve o total abatido. Útil para o royalty base
// "net_commission" em event_handler.royaltyBaseAmount.
func (c *Commission) SumDeductions() float64 {
	var sum float64
	for _, d := range c.Deductions {
		sum += d.Amount
	}
	return sum
}

// RecomputeNet recalcula NetAmount a partir de GrossAmount e Deductions.
// Chamado automaticamente por Add/RemoveDeduction; exposto para callers
// que reescrevem Deductions em batch (ex.: PUT /commissions/:id que
// substitui o slice inteiro).
func (c *Commission) RecomputeNet() {
	gross := c.GrossAmount
	if gross == 0 {
		gross = c.Amount // backward-compat: commissions antigas só tinham Amount
	}
	c.NetAmount = gross - c.SumDeductions()
	if c.NetAmount < 0 {
		c.NetAmount = 0
	}
}

// Approve transita PENDING → APPROVED. Idempotente: re-approve é no-op.
func (c *Commission) Approve() error {
	switch c.Status {
	case StatusApproved:
		return nil
	case StatusPending:
		if c.FundingOutstandingCents() > 0 {
			return ErrFundingIncomplete
		}
		c.Status = StatusApproved
		c.UpdatedAt = time.Now()
		return nil
	}
	return errors.New("only PENDING commissions can be approved")
}

// MarkPaid transita APPROVED → PAID. Aceita PENDING também porque alguns
// fluxos manuais (conciliação) marcam direto como pago.
func (c *Commission) MarkPaid(proofURL, note string) error {
	if c.Status == StatusPaid {
		changed := false
		if proofURL != "" && proofURL != c.ProofURL {
			c.ProofURL = proofURL
			changed = true
		}
		if note != "" && note != c.Note {
			c.Note = note
			changed = true
		}
		if changed {
			c.UpdatedAt = time.Now()
		}
		return nil
	}
	if c.Status != StatusPending && c.Status != StatusApproved {
		return errors.New("only PENDING/APPROVED commissions can be marked paid")
	}
	if c.FundingOutstandingCents() > 0 {
		return ErrFundingIncomplete
	}
	now := time.Now()
	c.Status = StatusPaid
	c.PaidAt = &now
	c.UpdatedAt = now
	if proofURL != "" {
		c.ProofURL = proofURL
	}
	if note != "" {
		c.Note = note
	}
	return nil
}

// Cancel marca como CANCELLED com timestamp e nota. Bloqueado se já paga
// — cancelar uma commission paga exige estorno explícito (fora do escopo).
func (c *Commission) Cancel(reason string) error {
	if c.Status == StatusPaid {
		return errors.New("cannot cancel a PAID commission; estorno required")
	}
	if c.Status == StatusCancelled {
		return nil
	}
	now := time.Now()
	c.Status = StatusCancelled
	c.CancelledAt = &now
	c.UpdatedAt = now
	if reason != "" {
		c.Note = reason
	}
	return nil
}

// Reopen devolve uma comissão CANCELADA ao estado PENDING para que a obrigação
// possa ser lançada de novo.
//
// Nasceu do fluxo de regeração da ordem de pagamento: cancelar a ordem cancela
// a comissão vinculada (cascade), e sem este caminho a obrigação ficava morta —
// o operador não tinha como corrigir e relançar o repasse. A comissão paga
// continua exigindo estorno (`Reverse`), não reabertura: dinheiro que já saiu
// não volta a ser uma obrigação pendente por um clique.
func (c *Commission) Reopen(actor, reason string) error {
	if c.Status == StatusPaid {
		return errors.New("cannot reopen a PAID commission; estorno required")
	}
	if c.Status == StatusPending {
		return nil
	}
	if c.Status != StatusCancelled {
		return errors.New("only CANCELLED commissions can be reopened")
	}
	now := time.Now()
	c.Status = StatusPending
	c.CancelledAt = nil
	c.UpdatedAt = now
	if reason != "" {
		c.Note = reason
	}
	if actor != "" {
		c.RequestedBy = actor
	}
	return nil
}

// Reverse transita de PAID ou APPROVED para REVERSED, armazenando o motivo
// da reversão. Este é o caminho correto para estornos — comissões que já
// foram pagas ou aprovadas e precisam ser revertidas contabilmente.
func (c *Commission) Reverse(reason string) error {
	if c.Status == StatusReversed {
		return nil
	}
	if c.Status != StatusPaid && c.Status != StatusApproved {
		return errors.New("only PAID or APPROVED commissions can be reversed")
	}
	now := time.Now()
	c.Status = StatusReversed
	c.UpdatedAt = now
	if reason != "" {
		c.Note = reason
	}
	return nil
}
