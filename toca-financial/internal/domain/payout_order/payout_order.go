// Package payout_order models the split / commission payment that the
// agency owes to a third party (broker, partner agency, royalty payee)
// after one of its charges is paid. The "repasse" to third parties is
// modeled here as a state machine that the operator drives manually —
// PENDING_REVIEW → APPROVED → PAID — and the actual settlement happens
// off-platform (PIX, TED) with a proof captured in proof_url.
//
// Sibling of withdrawal/withdrawal.go but with a richer recipient model
// and a `group_id` slot so operators can consolidate several orders into
// a single physical transfer ("pagar João R$ 12.500 referente a 7
// cobranças").
package payout_order

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft         Status = "DRAFT"          // created but not yet visible to ops
	StatusPendingReview Status = "PENDING_REVIEW" // awaiting operator triage
	StatusApproved      Status = "APPROVED"       // operator signed off, awaiting settlement
	StatusPaid          Status = "PAID"           // settled off-platform with proof captured
	StatusFailed        Status = "FAILED"         // settlement attempt rejected (rare)
	StatusCancelled     Status = "CANCELLED"      // cancelled before settlement
)

// SourceType records what triggered this order.
type SourceType string

const (
	SourceCharge            SourceType = "charge"             // a charge.paid event
	SourceManual            SourceType = "manual"             // operator-created (no upstream)
	SourceRoyaltySettlement SourceType = "royalty_settlement" // partner_royalties batch
)

// RecipientType identifies who is being paid.
type RecipientType string

const (
	RecipientBroker        RecipientType = "broker"         // commissioned agent (account_id)
	RecipientPartnerTenant RecipientType = "partner_tenant" // peer agency in a cross-deal
	RecipientRoyaltyPayee  RecipientType = "royalty_payee"  // franchisor on a royalty
	RecipientExternal      RecipientType = "external"       // ad-hoc external party
)

// SettlementMethod records how the operator actually moved the money
// off-platform.
type SettlementMethod string

const (
	SettlementManualPIX SettlementMethod = "MANUAL_PIX"
	SettlementManualTED SettlementMethod = "MANUAL_TED"
	SettlementOther     SettlementMethod = "OTHER"
)

// HistoryEntry — same shape as Withdrawal.HistoryEntry for cross-tool
// audit consistency.
type HistoryEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor,omitempty"`
	Note   string    `json:"note,omitempty"`
}

// FundingEntry links a rental receipt to the payout liquidity it unlocked.
// It does not change the contractual payout amount; it only controls when the
// order can leave DRAFT and enter operator review.
type FundingEntry struct {
	PaymentID      string    `json:"payment_id"`
	ChargeID       string    `json:"charge_id"`
	ReferenceMonth string    `json:"reference_month"`
	AmountCents    int64     `json:"amount_cents"`
	FundedAt       time.Time `json:"funded_at"`
}

// PayoutOrder is the persisted aggregate. Money in cents to avoid float
// drift; SourceRef holds a snapshot of the origin (charge metadata,
// royalty ledger row) so the UI can render context even if the source row
// is later edited.
type PayoutOrder struct {
	ID                   string           `json:"id"`
	TenantID             string           `json:"tenant_id"`
	SourceType           SourceType       `json:"source_type"`
	SourceID             string           `json:"source_id,omitempty"`
	SourceRef            map[string]any   `json:"source_ref,omitempty"`
	ContractID           string           `json:"contract_id,omitempty"`
	RecipientType        RecipientType    `json:"recipient_type"`
	RecipientID          string           `json:"recipient_id,omitempty"`
	RecipientLabel       string           `json:"recipient_label,omitempty"`
	RecipientPixKey      string           `json:"recipient_pix_key,omitempty"`
	AmountCents          int64            `json:"amount_cents"`
	FundingRequiredCents int64            `json:"funding_required_cents,omitempty"`
	FundedAmountCents    int64            `json:"funded_amount_cents,omitempty"`
	FundingEntries       []FundingEntry   `json:"funding_entries,omitempty"`
	Currency             string           `json:"currency"`
	Status               Status           `json:"status"`
	SettlementMethod     SettlementMethod `json:"settlement_method,omitempty"`
	ExternalID           string           `json:"external_id,omitempty"`
	SourceEventID        string           `json:"source_event_id,omitempty"`
	ProofURL             string           `json:"proof_url,omitempty"`
	ProofFilename        string           `json:"proof_filename,omitempty"`
	GroupID              string           `json:"group_id,omitempty"`
	// RegeneratedFromID aponta para a ordem cancelada que originou esta: o
	// operador cancela (destinatário errado, valor a corrigir) e REGERA a
	// obrigação a partir dela, mantendo a trilha de auditoria dos dois lados.
	RegeneratedFromID string         `json:"regenerated_from_id,omitempty"`
	RequestedBy       string         `json:"requested_by,omitempty"`
	ApprovedBy        string         `json:"approved_by,omitempty"`
	ApprovedAt        *time.Time     `json:"approved_at,omitempty"`
	SettledAt         *time.Time     `json:"settled_at,omitempty"`
	History           []HistoryEntry `json:"history,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

// PayoutOrderTotals agrega totais server-side para KPIs da inbox
// financeira, independente da paginação (FIN-COM-13). A soma de
// amount_cents e a contagem por status cobrem todos os registros
// que batem com o filtro, não apenas os primeiros N.
type StatusBreakdown struct {
	Count       int64 `json:"count"`
	AmountCents int64 `json:"amount_cents"`
}

type PayoutOrderTotals struct {
	TotalCount       int64                      `json:"total_count"`
	TotalAmountCents int64                      `json:"total_amount_cents"`
	ByStatus         map[Status]StatusBreakdown `json:"by_status"`
}

// Sentinel errors — service layer branches on these instead of string
// matching.
var (
	ErrAmountTooLow        = errors.New("payout amount must be > 0")
	ErrInvalidStatus       = errors.New("payout is not in a status that allows this transition")
	ErrInvalidRecipient    = errors.New("invalid recipient_type")
	ErrInvalidSource       = errors.New("invalid source_type")
	ErrInvalidSettlement   = errors.New("invalid settlement_method")
	ErrProofRequired       = errors.New("proof is required when marking the order paid")
	ErrFundingIncomplete   = errors.New("payout is waiting for collected funds")
	ErrFundingGateConflict = errors.New("payout funding gate conflicts with existing payout state")
	// ErrNotCancelled: só uma ordem CANCELADA pode ser regerada. Uma ordem viva
	// já é a obrigação; uma paga exige estorno, não uma segunda ordem.
	ErrNotCancelled = errors.New("only cancelled payouts can be regenerated")
)

// New builds a payout order in PENDING_REVIEW state — the default starting
// status for orchestration-emitted orders. Operators can also create
// DRAFT-then-promote orders for manual cases, but the orchestration path
// skips DRAFT.
func New(tenantID string, sourceType SourceType, sourceID string, recipientType RecipientType, recipientID, recipientLabel string, amountCents int64, requestedBy string) (*PayoutOrder, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if amountCents <= 0 {
		return nil, ErrAmountTooLow
	}
	if err := ValidateSourceType(sourceType); err != nil {
		return nil, err
	}
	if err := ValidateRecipientType(recipientType); err != nil {
		return nil, err
	}
	now := time.Now()
	p := &PayoutOrder{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		SourceType:     sourceType,
		SourceID:       sourceID,
		RecipientType:  recipientType,
		RecipientID:    recipientID,
		RecipientLabel: recipientLabel,
		AmountCents:    amountCents,
		Currency:       "BRL",
		Status:         StatusPendingReview,
		ExternalID:     "toca-payout-" + uuid.New().String(),
		RequestedBy:    requestedBy,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	p.addHistory("created", requestedBy, "")
	return p, nil
}

func ValidateSourceType(t SourceType) error {
	switch t {
	case SourceCharge, SourceManual, SourceRoyaltySettlement:
		return nil
	}
	return ErrInvalidSource
}

func ValidateRecipientType(t RecipientType) error {
	switch t {
	case RecipientBroker, RecipientPartnerTenant, RecipientRoyaltyPayee, RecipientExternal:
		return nil
	}
	return ErrInvalidRecipient
}

func ValidateSettlementMethod(m SettlementMethod) error {
	switch m {
	case SettlementManualPIX, SettlementManualTED, SettlementOther, "":
		return nil
	}
	return ErrInvalidSettlement
}

func (p *PayoutOrder) addHistory(action, actor, note string) {
	now := time.Now()
	p.History = append(p.History, HistoryEntry{At: now, Action: action, Actor: actor, Note: note})
	p.UpdatedAt = now
}

// FundingOutstandingCents returns the rental cash still required before this
// payout can be reviewed. Zero requirement preserves legacy and sale behavior.
func (p *PayoutOrder) FundingOutstandingCents() int64 {
	if p == nil || p.FundingRequiredCents <= 0 {
		return 0
	}
	outstanding := p.FundingRequiredCents - p.FundedAmountCents
	if outstanding < 0 {
		return 0
	}
	return outstanding
}

// EnableRentalFundingGate puts a broker payout behind the collected-rent
// boundary. Besides configuring new orders, it safely upgrades legacy orders
// that are still awaiting review. Advanced historical states require explicit
// reconciliation because their funding source cannot be inferred.
func (p *PayoutOrder) EnableRentalFundingGate(requiredCents int64) (bool, error) {
	return p.EnableFundingGate(requiredCents)
}

// EnableFundingGate holds automatic obligations until collected cash covers them.
func (p *PayoutOrder) EnableFundingGate(requiredCents int64) (bool, error) {
	if p == nil || requiredCents <= 0 {
		return false, ErrFundingGateConflict
	}
	// O valor da ordem é o LÍQUIDO a transferir; a exigência de liquidez é a
	// obrigação BRUTA da comissão. Com retenções (IRRF/INSS/ISS) e adiantamentos
	// abatidos o líquido é legitimamente MENOR que a obrigação — a igualdade
	// antiga passou a recusar ordens corretas e derrubava a baixa da cobrança
	// ("mark charge as paid" → 500). O que continua inconsistente é o pagável
	// EXCEDER a obrigação: aí sim é conflito.
	if p.AmountCents > requiredCents {
		return false, ErrFundingGateConflict
	}
	if p.FundingRequiredCents != 0 && p.FundingRequiredCents != requiredCents {
		return false, ErrFundingGateConflict
	}
	if p.FundedAmountCents < 0 || p.FundedAmountCents > requiredCents {
		return false, ErrFundingGateConflict
	}

	changed := false
	if p.FundingRequiredCents == 0 {
		if p.Status != StatusDraft && p.Status != StatusPendingReview {
			return false, ErrFundingGateConflict
		}
		if p.FundedAmountCents != 0 || len(p.FundingEntries) != 0 {
			return false, ErrFundingGateConflict
		}
		p.FundingRequiredCents = requiredCents
		changed = true
	}

	outstanding := p.FundingOutstandingCents()
	if outstanding > 0 {
		if p.Status != StatusDraft && p.Status != StatusPendingReview {
			return false, ErrFundingGateConflict
		}
		if p.Status == StatusPendingReview {
			p.Status = StatusDraft
			changed = true
		}
	} else if p.Status == StatusDraft {
		p.Status = StatusPendingReview
		changed = true
	}
	if changed {
		p.addHistory("funding_gate_enabled", "system", "Pagamento condicionado à cobertura por recebimentos")
	}
	return changed, nil
}

func (p *PayoutOrder) requiresFundingReconciliation() bool {
	if p == nil || p.RecipientType != RecipientBroker || p.FundingRequiredCents > 0 {
		return false
	}
	for _, key := range []string{"charge_type", "deal_type"} {
		value, _ := p.SourceRef[key].(string)
		normalized := strings.ToLower(strings.TrimSpace(value))
		if normalized == "sale" || normalized == "venda" || normalized == "rent" || normalized == "rental" || normalized == "locacao" || normalized == "locação" {
			return true
		}
	}
	return false
}

// RecordFunding appends an idempotent rental-receipt slice. A fully funded
// DRAFT order is promoted to PENDING_REVIEW automatically; until then it is
// visible as an anticipated obligation but cannot be approved or paid.
func (p *PayoutOrder) RecordFunding(entry FundingEntry) (bool, error) {
	entry.PaymentID = strings.TrimSpace(entry.PaymentID)
	entry.ChargeID = strings.TrimSpace(entry.ChargeID)
	entry.ReferenceMonth = strings.TrimSpace(entry.ReferenceMonth)
	if p.FundingRequiredCents <= 0 {
		return false, errors.New("payout is not funding-gated")
	}
	if p.Status != StatusDraft && p.Status != StatusPendingReview {
		return false, ErrInvalidStatus
	}
	// ChargeID is provenance for charge payments; a manual receipt may leave it
	// empty and still fund the order via its payment ID.
	if entry.PaymentID == "" || entry.AmountCents <= 0 || entry.FundedAt.IsZero() {
		return false, errors.New("payout funding entry is invalid")
	}
	for _, existing := range p.FundingEntries {
		if existing.PaymentID != entry.PaymentID {
			continue
		}
		if existing.ChargeID != entry.ChargeID || existing.ReferenceMonth != entry.ReferenceMonth || existing.AmountCents != entry.AmountCents {
			return false, errors.New("payout funding payment_id was reused with different data")
		}
		return false, nil
	}
	if entry.AmountCents > p.FundingOutstandingCents() {
		return false, errors.New("payout funding exceeds required amount")
	}
	p.FundingEntries = append(p.FundingEntries, entry)
	p.FundedAmountCents += entry.AmountCents
	p.UpdatedAt = time.Now()
	if p.FundingOutstandingCents() == 0 && p.Status == StatusDraft {
		p.Status = StatusPendingReview
		p.addHistory("funded", "system", "Comissão integralmente coberta por recebimentos")
	} else {
		p.addHistory("funding_received", "system", "Cobertura parcial por recebimento")
	}
	return true, nil
}

// Approve moves PENDING_REVIEW → APPROVED. Approval signals the operator
// is ready to settle but does NOT call any external API — the actual
// settlement happens off-platform.
func (p *PayoutOrder) Approve(actor string) error {
	if p.Status != StatusPendingReview && p.Status != StatusDraft {
		return ErrInvalidStatus
	}
	if p.requiresFundingReconciliation() {
		return ErrFundingGateConflict
	}
	if p.FundingOutstandingCents() > 0 {
		return ErrFundingIncomplete
	}
	now := time.Now()
	p.Status = StatusApproved
	p.ApprovedBy = actor
	p.ApprovedAt = &now
	p.addHistory("approved", actor, "")
	return nil
}

// MarkPaid records that the operator settled the payout off-platform. The
// actual money movement happened outside our system, so the proof is our audit
// trail.
//
// `requireProof` é decidido pelo serviço, que conhece a configuração do
// tenant. Antes a exigência era incondicional aqui, e por isso o setting
// payout_proof_required não conseguia governá-la: desligá-lo não tinha efeito
// algum. A entidade valida a invariante; a política é do chamador.
func (p *PayoutOrder) MarkPaid(actor, method, proofURL, proofFilename, note string, requireProof bool) error {
	if p.Status != StatusApproved {
		return ErrInvalidStatus
	}
	if p.requiresFundingReconciliation() {
		return ErrFundingGateConflict
	}
	if p.FundingOutstandingCents() > 0 {
		return ErrFundingIncomplete
	}
	if err := ValidateSettlementMethod(SettlementMethod(method)); err != nil {
		return err
	}
	if requireProof && proofURL == "" {
		return ErrProofRequired
	}
	now := time.Now()
	p.Status = StatusPaid
	p.SettlementMethod = SettlementMethod(method)
	p.ProofURL = proofURL
	p.ProofFilename = proofFilename
	p.SettledAt = &now
	p.addHistory("paid", actor, note)
	return nil
}

// MarkFailed records that a settlement attempt failed (rare — usually a
// retry happens off-platform too). Allowed from APPROVED only.
func (p *PayoutOrder) MarkFailed(actor, reason string) error {
	if p.Status != StatusApproved {
		return ErrInvalidStatus
	}
	p.Status = StatusFailed
	p.addHistory("failed", actor, reason)
	return nil
}

// Cancel aborts the payout while still cancellable (DRAFT, PENDING_REVIEW
// or APPROVED). PAID and FAILED are terminal.
func (p *PayoutOrder) Cancel(actor, reason string) error {
	if p.Status == StatusPaid || p.Status == StatusFailed || p.Status == StatusCancelled {
		return ErrInvalidStatus
	}
	p.Status = StatusCancelled
	p.addHistory("cancelled", actor, reason)
	return nil
}

// Regenerate constrói uma NOVA ordem a partir de uma cancelada, preservando a
// identidade da obrigação: mesma origem (source_type/source_id/source_event_id),
// mesmo destinatário, mesmo valor e o mesmo portão de liquidez dos aluguéis.
//
// Por que existe: cancelar encerra a ordem (e a comissão vinculada) sem nenhum
// caminho de volta, então um cancelamento por engano ou uma correção forçavam o
// operador a recriar a obrigação por fora do sistema. Com a regeração, a ordem
// cancelada vira histórico e a nova nasce em PENDING_REVIEW — pronta para ser
// aprovada e lançada de novo, com `regenerated_from_id` ligando as duas.
//
// O `source_event_id` é o MESMO: a obrigação é a mesma, e é isso que impede
// duas ordens vivas para a mesma origem (índice único parcial que ignora
// CANCELLED).
func (p *PayoutOrder) Regenerate(actor string) (*PayoutOrder, error) {
	if p == nil {
		return nil, ErrNotCancelled
	}
	if p.Status != StatusCancelled {
		return nil, ErrNotCancelled
	}
	fresh, err := New(p.TenantID, p.SourceType, p.SourceID, p.RecipientType, p.RecipientID, p.RecipientLabel, p.AmountCents, actor)
	if err != nil {
		return nil, err
	}
	fresh.ContractID = p.ContractID
	fresh.RecipientPixKey = p.RecipientPixKey
	fresh.SourceEventID = p.SourceEventID
	fresh.SourceRef = p.SourceRef
	// O portão de liquidez viaja com a obrigação — exigência E cobertura.
	//
	// A exigência sozinha não basta: as ordens de aluguel nascem com a cobertura
	// sendo acumulada recebimento a recebimento, e uma obrigação já coberta (o
	// caso real: as duas ordens do beta estavam 100% cobertas quando foram
	// canceladas) voltaria a "Aguardando recebimentos" — o operador não
	// conseguiria aprovar mesmo com o dinheiro já recebido, e "lançar novamente"
	// continuaria impossível por outro motivo. Os recebimentos são fatos da
	// obrigação, não da linha antiga.
	fresh.FundingRequiredCents = p.FundingRequiredCents
	fresh.FundedAmountCents = p.FundedAmountCents
	fresh.FundingEntries = append([]FundingEntry(nil), p.FundingEntries...)
	fresh.RegeneratedFromID = p.ID
	fresh.addHistory("regenerated", actor, "from "+p.ID)
	return fresh, nil
}

// AssignGroup attaches a consolidation group_id. Operators use this to
// collapse several individual payouts to the same recipient into a single
// physical transfer. The constituent orders still exist as separate
// records; only the group_id ties them together for accounting.
func (p *PayoutOrder) AssignGroup(groupID, actor string) error {
	if p.Status != StatusPendingReview && p.Status != StatusApproved {
		return ErrInvalidStatus
	}
	p.GroupID = groupID
	p.addHistory("grouped", actor, groupID)
	return nil
}

// Terminal reports whether the order has reached an end state.
func (p *PayoutOrder) Terminal() bool {
	return p.Status == StatusPaid || p.Status == StatusFailed || p.Status == StatusCancelled
}
