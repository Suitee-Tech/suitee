package transfer

import (
	"encoding/json"
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
	StatusProcessed Status = "PROCESSED"
	StatusCancelled Status = "CANCELLED"
)

type Deduction struct {
	Description string  `json:"description" bson:"description"`
	Amount      float64 `json:"amount" bson:"amount"`
	Type        string  `json:"type" bson:"type"`
	ReferenceID string  `json:"reference_id,omitempty" bson:"reference_id,omitempty"`
	// LedgerAccount é a contrapartida da retenção no razão. A natureza do
	// valor depende de quem adiantou o pagamento (direito a receber da
	// imobiliária ou obrigação com terceiro), então quem aplica a dedução
	// declara a conta; vazio cai no padrão do Type.
	LedgerAccount string `json:"ledger_account,omitempty" bson:"ledger_account,omitempty"`
}

// EconomicBreakdown is the immutable, cent-precision explanation of where a
// received amount is economically destined. It is embedded in the owner
// transfer so the operator can distinguish money held for the owner from the
// agency's revenue and from obligations to brokers/partners.
//
// The fields are deliberately additive to OwnerTransfer's historical
// rent-shaped fields. Existing consumers can keep reading gross_rent and
// net_amount while newer screens use this breakdown for both RENT and SALE.
type EconomicBreakdown struct {
	Flow                       string `json:"flow" bson:"flow"` // rent | sale
	GrossCents                 int64  `json:"gross_cents" bson:"gross_cents"`
	OwnerPayableCents          int64  `json:"owner_payable_cents" bson:"owner_payable_cents"`
	ReservedCents              int64  `json:"reserved_cents" bson:"reserved_cents"`
	AvailableCents             int64  `json:"available_cents" bson:"available_cents"`
	AgencyRevenueCents         int64  `json:"agency_revenue_cents" bson:"agency_revenue_cents"`
	AdminFeeCents              int64  `json:"admin_fee_cents" bson:"admin_fee_cents"`
	IntermediationRevenueCents int64  `json:"intermediation_revenue_cents" bson:"intermediation_revenue_cents"`
	CommissionFundedCents      int64  `json:"commission_funded_cents" bson:"commission_funded_cents"`
	CommissionPayableCents     int64  `json:"commission_payable_cents" bson:"commission_payable_cents"`
	PartnerPayableCents        int64  `json:"partner_payable_cents" bson:"partner_payable_cents"`
	AgencyRetainedCents        int64  `json:"agency_retained_cents" bson:"agency_retained_cents"`
	DiscountCents              int64  `json:"discount_cents" bson:"discount_cents"`
	DeductionsCents            int64  `json:"deductions_cents" bson:"deductions_cents"`
}

type OwnerTransfer struct {
	ID       string `json:"id" bson:"_id"`
	TenantID string `json:"tenant_id" bson:"tenant_id"`
	// SourceChargeID is the stable identity of the fully paid charge that
	// released this transfer. SALE plans may have multiple installments in the
	// same reference month, so contract+month is not a safe idempotency key.
	// Legacy/manual transfers leave it empty and retain monthly uniqueness.
	SourceChargeID string `json:"source_charge_id,omitempty" bson:"source_charge_id,omitempty"`
	// SourceTransactionID identifies a manual receipt whose settlement released
	// this owner obligation. It is mutually exclusive with SourceChargeID.
	SourceTransactionID string `json:"source_transaction_id,omitempty" bson:"source_transaction_id,omitempty"`
	ContractID          string `json:"contract_id" bson:"contract_id"`
	OwnerID             string `json:"owner_id" bson:"owner_id"`
	PropertyID          string `json:"property_id" bson:"property_id"`
	ReferenceMonth      string `json:"reference_month" bson:"reference_month"`
	// SettlementType distinguishes a monthly rental repasse from a sale
	// settlement. Empty means a legacy row whose semantics predate this field.
	SettlementType        string      `json:"settlement_type,omitempty" bson:"settlement_type,omitempty"`
	GrossRent             float64     `json:"gross_rent" bson:"gross_rent"`
	AdminFeePercent       float64     `json:"admin_fee_percent" bson:"admin_fee_percent"`
	AdminFeeAmount        float64     `json:"admin_fee_amount" bson:"admin_fee_amount"`
	MaintenanceDeductions float64     `json:"maintenance_deductions" bson:"maintenance_deductions"`
	OtherDeductions       float64     `json:"other_deductions" bson:"other_deductions"`
	DeductionDetails      []Deduction `json:"deduction_details,omitempty" bson:"deduction_details,omitempty"`
	NetAmount             float64     `json:"net_amount" bson:"net_amount"`
	// OwnerBalance is populated for allocation-aware transfers. TotalCents is
	// the complete owner payable, ReservedCents holds values collected for
	// direct liquidation/offset, and BlockedCents mirrors transfer deductions.
	// Nil identifies legacy transfers, whose net remains derived from GrossRent.
	OwnerBalance *OwnerBalance `json:"owner_balance,omitempty" bson:"owner_balance,omitempty"`
	// DelinquencyHeldCents marca quanto deste repasse está RETIDO por
	// inadimplência do contrato (política opcional da imobiliária). O valor não
	// sai do repasse: ele deixa de estar disponível enquanto o caso de cobrança do
	// contrato estiver aberto, e volta a ficar quando o caso é resolvido.
	//
	// Fica em campo próprio (e não em ReservedCents, que é a reserva para
	// liquidação direta/abatimento) para não misturar semânticas: quem lê a reserva
	// para calcular teto de abatimento não pode passar a ver inadimplência ali.
	DelinquencyHeldCents int64 `json:"delinquency_held_cents,omitempty" bson:"delinquency_held_cents,omitempty"`
	// DelinquencyHoldReason explica a retenção para quem abre o repasse depois.
	DelinquencyHoldReason string `json:"delinquency_hold_reason,omitempty" bson:"delinquency_hold_reason,omitempty"`
	// DelinquencyCaseID é o caso de cobrança que motivou a retenção.
	DelinquencyCaseID string `json:"delinquency_case_id,omitempty" bson:"delinquency_case_id,omitempty"`
	// DelinquencyHeldAt é quando a retenção foi aplicada.
	DelinquencyHeldAt *time.Time `json:"delinquency_held_at,omitempty" bson:"delinquency_held_at,omitempty"`
	// EconomicBreakdown is populated for charge-backed transfers after the
	// payment allocation closes. It is optional for legacy/manual transfers.
	EconomicBreakdown *EconomicBreakdown `json:"economic_breakdown,omitempty" bson:"economic_breakdown,omitempty"`
	// OwnerBalanceBase* freezes the allocation-derived economic basis. Lifecycle
	// movements never edit that basis: the current buckets are rebuilt by
	// replaying the append-only OwnerBalanceMovements over it. This makes a
	// charge-payment retry deterministic without resurrecting consumed/released
	// reserves.
	OwnerBalanceBaseTotalCents    int64                  `json:"owner_balance_base_total_cents,omitempty" bson:"owner_balance_base_total_cents,omitempty"`
	OwnerBalanceBaseReservedCents int64                  `json:"owner_balance_base_reserved_cents,omitempty" bson:"owner_balance_base_reserved_cents,omitempty"`
	OwnerBalanceMovements         []OwnerBalanceMovement `json:"owner_balance_movements,omitempty" bson:"owner_balance_movements,omitempty"`
	Status                        Status                 `json:"status" bson:"status"`
	ProcessedAt                   *time.Time             `json:"processed_at,omitempty" bson:"processed_at,omitempty"`
	ApprovedAt                    *time.Time             `json:"approved_at,omitempty" bson:"approved_at,omitempty"`
	ApprovedBy                    string                 `json:"approved_by,omitempty" bson:"approved_by,omitempty"`
	ProcessedBy                   string                 `json:"processed_by,omitempty" bson:"processed_by,omitempty"`
	SettlementJournalSourceID     string                 `json:"settlement_journal_source_id,omitempty" bson:"settlement_journal_source_id,omitempty"`
	BankAccount                   string                 `json:"bank_account,omitempty" bson:"bank_account,omitempty"`
	Notes                         string                 `json:"notes,omitempty" bson:"notes,omitempty"`
	// ProofURL é o comprovante anexado pelo operador ao efetivar o repasse
	// manualmente (fluxo externo/banco). Registrado no PROCESSED para auditoria.
	ProofURL string `json:"proof_url,omitempty" bson:"proof_url,omitempty"`
	// SettledWithoutBalance marca um repasse encerrado porque não havia nada a
	// repassar (líquido e saldo disponível/reservado zerados). Essa conclusão não
	// movimenta caixa nem gera lançamento de liquidação, então é provisória por
	// natureza: se o saldo do proprietário voltar a ter valor a repassar
	// (estorno de conta administrada) ou se uma composição econômica real
	// (cobrança paga) precisar adotar o registro, o repasse reabre — ver
	// ReopenWithoutPayableBalance. É o marcador persistido, e não o saldo atual,
	// que sustenta a decisão em retries e conciliações.
	SettledWithoutBalance bool `json:"settled_without_balance,omitempty" bson:"settled_without_balance,omitempty"`
	// History is the append-only audit trail of state transitions
	// (created → approved → processed) plus any future cancel/reopen. UI
	// surfaces this on the repasse detail so operators can see who did what
	// and when.
	History   []HistoryEntry `json:"history,omitempty" bson:"history,omitempty"`
	CreatedAt time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time      `json:"updated_at" bson:"updated_at"`
}

// HistoryEntry captures a single state transition on the transfer. Actor is
// the account_id pulled from the JWT context at the moment of the action;
// "system" indicates an automated/event-driven transition. ActorLabel is
// the human-readable snapshot captured at write time so the audit doesn't
// silently degrade when the operator's broker record is removed later.
type HistoryEntry struct {
	At         time.Time `json:"at" bson:"at"`
	Action     string    `json:"action" bson:"action"` // "created" | "approved" | "processed" | "cancelled"
	Actor      string    `json:"actor,omitempty" bson:"actor,omitempty"`
	ActorLabel string    `json:"actor_label,omitempty" bson:"actor_label,omitempty"`
	Note       string    `json:"note,omitempty" bson:"note,omitempty"`
}

type OwnerBalanceMovementType string

const (
	OwnerBalanceMovementConsumeReserved OwnerBalanceMovementType = "consume_reserved"
	OwnerBalanceMovementReleaseReserved OwnerBalanceMovementType = "release_reserved"
	OwnerBalanceMovementRestoreReserved OwnerBalanceMovementType = "restore_reserved"
)

// OwnerBalanceMovement is immutable economic provenance for one ManagedBill.
// ID is a deterministic command/source identity. Reusing it with a different
// bill, payment attempt, type or amount is rejected rather than silently
// treated as a retry. PaymentAttemptID pairs a consume with its compensating
// restore while allowing a later attempt for the same obligation.
type OwnerBalanceMovement struct {
	ID               string                   `json:"id" bson:"id"`
	ManagedBillID    string                   `json:"managed_bill_id" bson:"managed_bill_id"`
	PaymentAttemptID string                   `json:"payment_attempt_id,omitempty" bson:"payment_attempt_id,omitempty"`
	Type             OwnerBalanceMovementType `json:"type" bson:"type"`
	AmountCents      int64                    `json:"amount_cents" bson:"amount_cents"`
	Actor            string                   `json:"actor,omitempty" bson:"actor,omitempty"`
	At               time.Time                `json:"at" bson:"at"`
}

func New(contractID, ownerID, propertyID, tenantID, referenceMonth string, grossRent, adminFeePercent, maintenanceDeductions, otherDeductions float64, deductionDetails []Deduction, notes string) *OwnerTransfer {
	now := time.Now()
	t := &OwnerTransfer{
		ID:                    uuid.New().String(),
		TenantID:              tenantID,
		ContractID:            contractID,
		OwnerID:               ownerID,
		PropertyID:            propertyID,
		ReferenceMonth:        referenceMonth,
		SettlementType:        "rent",
		GrossRent:             grossRent,
		AdminFeePercent:       adminFeePercent,
		MaintenanceDeductions: maintenanceDeductions,
		OtherDeductions:       otherDeductions,
		DeductionDetails:      deductionDetails,
		Status:                StatusPending,
		Notes:                 notes,
		History:               []HistoryEntry{{At: now, Action: "created"}},
		CreatedAt:             now,
		UpdatedAt:             now,
	}
	t.Calculate()
	return t
}

// Approve transitions the transfer to APPROVED and records the actor.
// actorLabel is the human-readable snapshot of the actor at the moment of
// the action — empty is fine for legacy / internal-key callers that don't
// have a JWT name claim.
// IsHeldForDelinquency reporta se o repasse está retido por inadimplência.
// ErrTransferHeldByDelinquency — o repasse está retido por inadimplência do
// contrato e não pode ser aprovado/pago enquanto o caso estiver aberto (ou até o
// operador liberar explicitamente).
var ErrTransferHeldByDelinquency = errors.New("repasse retido por inadimplência do contrato")

func (t *OwnerTransfer) IsHeldForDelinquency() bool {
	return t != nil && t.DelinquencyHeldCents > 0
}

// HoldForDelinquency retém o repasse por inadimplência do contrato.
//
// O valor retido é o TOTAL do repasse disponível: a política só chega aqui quando
// a dívida em aberto é material em relação ao repasse (ver a regra na camada de
// serviço), então reter parcialmente exigiria pagamento em duas etapas — que o
// modelo de liquidação não suporta (o lançamento exige valor = disponível).
func (t *OwnerTransfer) HoldForDelinquency(cents int64, caseID, reason string, now time.Time) error {
	if t == nil {
		return ErrCannotApprove
	}
	if cents <= 0 {
		return nil
	}
	if t.Status == StatusProcessed || t.Status == StatusCancelled {
		// Repasse já pago ou cancelado não é retido: o dinheiro saiu (ou a
		// obrigação foi encerrada) e a retenção viraria cobrança indevida.
		return nil
	}
	t.DelinquencyHeldCents = cents
	t.DelinquencyCaseID = caseID
	t.DelinquencyHoldReason = reason
	t.DelinquencyHeldAt = &now
	t.UpdatedAt = now
	return nil
}

// ReleaseDelinquencyHold libera a retenção (caso resolvido ou decisão do
// operador). Devolve true quando havia o que liberar.
func (t *OwnerTransfer) ReleaseDelinquencyHold(now time.Time) bool {
	if t == nil || t.DelinquencyHeldCents == 0 {
		return false
	}
	t.DelinquencyHeldCents = 0
	t.DelinquencyCaseID = ""
	t.DelinquencyHoldReason = ""
	t.DelinquencyHeldAt = nil
	t.UpdatedAt = now
	return true
}

func (t *OwnerTransfer) Approve(actor, actorLabel string) error {
	if t.Status != StatusPending {
		return ErrCannotApprove
	}
	now := time.Now()
	t.Status = StatusApproved
	t.ApprovedAt = &now
	t.ApprovedBy = actor
	t.UpdatedAt = now
	t.History = append(t.History, HistoryEntry{At: now, Action: "approved", Actor: actor, ActorLabel: actorLabel})
	return nil
}

// Process marks the transfer as PROCESSED (banked) and records the actor.
func (t *OwnerTransfer) Process(actor, actorLabel string) error {
	return t.ProcessWithProof(actor, actorLabel, "", "")
}

// ProcessWithProof marks the transfer as PROCESSED (banked), recording the
// actor and, quando o repasse é efetivado manualmente (fluxo externo/banco),
// o comprovante (proofURL) e/ou uma observação (note).
func (t *OwnerTransfer) ProcessWithProof(actor, actorLabel, proofURL, note string) error {
	if t.Status != StatusApproved {
		return ErrCannotProcess
	}
	now := time.Now()
	t.Status = StatusProcessed
	t.ProcessedAt = &now
	t.ProcessedBy = actor
	if proofURL != "" {
		t.ProofURL = proofURL
	}
	if note != "" {
		t.Notes = note
	}
	t.UpdatedAt = now
	t.History = append(t.History, HistoryEntry{At: now, Action: "processed", Actor: actor, ActorLabel: actorLabel})
	return nil
}

// HasNoPayableBalance reports that this transfer has no cash payable to the
// owner and no reserved obligation still waiting for its own settlement. A
// zero available balance alone is not enough: when ReservedCents is positive,
// managed bills can still consume or release that reserve.
func (t *OwnerTransfer) HasNoPayableBalance() bool {
	if t == nil || amountToCents(t.NetAmount) != 0 {
		return false
	}
	if t.OwnerBalance == nil {
		return true
	}
	return t.OwnerBalance.AvailableCents() == 0 && t.OwnerBalance.ReservedCents == 0
}

// SettleIfNoPayableBalance closes a system-generated zero transfer without
// inventing a bank payment or settlement journal. The row remains available
// for economic history, but no longer participates in pending approvals or
// payouts. It returns true only when it performs the transition.
func (t *OwnerTransfer) SettleIfNoPayableBalance(at time.Time) bool {
	if t == nil || (t.Status != StatusPending && t.Status != StatusApproved) || !t.HasNoPayableBalance() {
		return false
	}
	if at.IsZero() {
		at = time.Now()
	}
	if at.Before(t.CreatedAt) {
		at = t.CreatedAt
	}
	t.Status = StatusProcessed
	t.ProcessedAt = &at
	t.ProcessedBy = "system"
	t.SettledWithoutBalance = true
	t.UpdatedAt = at
	t.History = append(t.History, HistoryEntry{
		At:         at,
		Action:     "processed",
		Actor:      "system",
		ActorLabel: "Sistema",
		Note:       "Liquidado automaticamente sem saldo a repassar.",
	})
	return true
}

// SettleWithoutPayableBalance conclui, a pedido do operador, um repasse que não
// tem nada a repassar. Diferente do fechamento automático, aqui a baixa foi
// tentada e não existe pagamento a registrar: nenhum comprovante é exigido,
// nenhum lançamento de caixa é criado e o operador que acionou a conclusão fica
// no histórico (ProcessedBy) e no audit. ProcessedAt/Notas do fechamento
// automático são reaproveitados para manter a mesma semântica de estado.
func (t *OwnerTransfer) SettleWithoutPayableBalance(at time.Time, actor, actorLabel string) bool {
	if !t.SettleIfNoPayableBalance(at) {
		return false
	}
	if t.ProcessedAt == nil || len(t.History) == 0 {
		return true
	}
	last := &t.History[len(t.History)-1]
	last.Note = "Concluído sem saldo a repassar: nenhuma movimentação de caixa foi registrada."
	actor = strings.TrimSpace(actor)
	if actor != "" && actor != "system" {
		last.Actor = actor
		last.ActorLabel = strings.TrimSpace(actorLabel)
		t.ProcessedBy = actor
	}
	return true
}

// ReopenWithoutPayableBalance desfaz uma conclusão sem saldo. Existe para os
// dois casos em que o registro volta a ter conteúdo econômico: o estorno de uma
// conta administrada devolve saldo ao proprietário, e a cobrança paga que
// originou o repasse pode chegar com uma composição diferente da que zerou o
// valor. Falha fechado: só reabre o que foi encerrado como "sem saldo" e nunca
// movimentou caixa — um repasse efetivamente pago não é reaberto por aqui.
func (t *OwnerTransfer) ReopenWithoutPayableBalance(at time.Time, reason string) bool {
	if t == nil || !t.SettledWithoutBalance || t.Status != StatusProcessed {
		return false
	}
	if strings.TrimSpace(t.SettlementJournalSourceID) != "" {
		return false
	}
	if at.IsZero() {
		at = time.Now()
	}
	if at.Before(t.CreatedAt) {
		at = t.CreatedAt
	}
	// A aprovação anterior continua válida: o repasse volta para o estado que
	// tinha antes de ser encerrado sem saldo.
	restored := StatusPending
	if strings.TrimSpace(t.ApprovedBy) != "" || t.ApprovedAt != nil {
		restored = StatusApproved
	}
	t.Status = restored
	t.ProcessedAt = nil
	t.ProcessedBy = ""
	t.SettledWithoutBalance = false
	t.UpdatedAt = at
	t.History = append(t.History, HistoryEntry{
		At:         at,
		Action:     "reopened",
		Actor:      "system",
		ActorLabel: "Sistema",
		Note:       strings.TrimSpace(reason),
	})
	return true
}

// SyncNoPayableBalanceState reavalia a conclusão sem saldo depois de qualquer
// mudança no que há a repassar — dedução aplicada ou estornada, reserva de
// conta administrada consumida/liberada/estornada, composição adotada. É o
// único ponto que decide entre encerrar e reabrir, para nenhum caminho de
// escrita deixar o repasse preso em PENDING sem valor ou PROCESSED sem lastro.
// Devolve true quando muda o estado.
func (t *OwnerTransfer) SyncNoPayableBalanceState(at time.Time) bool {
	if t == nil {
		return false
	}
	if t.SettledWithoutBalance {
		if t.HasNoPayableBalance() {
			return false
		}
		return t.ReopenWithoutPayableBalance(at, "Saldo do proprietário voltou a ter valor a repassar.")
	}
	return t.SettleIfNoPayableBalance(at)
}

// Cancel voids an unbanked transfer (PENDING or APPROVED). A PROCESSED
// transfer is a cash movement already completed and cannot be cancelled
// here — it requires an explicit estorno of the owner settlement.
func (t *OwnerTransfer) Cancel(actor, actorLabel, reason string) error {
	if t.Status == StatusCancelled {
		return nil
	}
	if t.Status != StatusPending && t.Status != StatusApproved {
		return fmt.Errorf("only PENDING or APPROVED transfers can be cancelled (current: %s)", t.Status)
	}
	now := time.Now()
	t.Status = StatusCancelled
	t.UpdatedAt = now
	note := strings.TrimSpace(reason)
	t.History = append(t.History, HistoryEntry{At: now, Action: "cancelled", Actor: actor, ActorLabel: actorLabel, Note: note})
	return nil
}

// Sentinel errors so the service can branch on cause without string-matching.
var (
	ErrCannotApprove           = ownerTransferStateErr("transfer must be in PENDING status to approve")
	ErrCannotProcess           = ownerTransferStateErr("transfer must be in APPROVED status to process")
	ErrOwnerBalanceBaseChanged = ownerTransferStateErr("allocation-derived owner balance changed after managed-bill movements")
)

type ownerTransferStateErr string

func (e ownerTransferStateErr) Error() string { return string(e) }

func (t *OwnerTransfer) Calculate() {
	if t.OwnerBalance != nil {
		// Allocation-aware transfers use the owner subledger as the canonical
		// payable amount. Deductions block only the otherwise available pocket;
		// reserved obligations are never released by this calculation.
		blockedCents := amountToCents(t.MaintenanceDeductions + t.OtherDeductions)
		maxBlockedCents := t.OwnerBalance.TotalCents - t.OwnerBalance.ReservedCents
		if maxBlockedCents < 0 {
			maxBlockedCents = 0
		}
		if blockedCents > maxBlockedCents {
			blockedCents = maxBlockedCents
		}
		t.OwnerBalance.BlockedCents = blockedCents
		t.NetAmount = centsToAmount(t.OwnerBalance.AvailableCents())
		t.refreshEconomicBreakdown()
		return
	}

	t.AdminFeeAmount = t.GrossRent * t.AdminFeePercent / 100
	totalDeductions := t.AdminFeeAmount + t.MaintenanceDeductions + t.OtherDeductions
	t.NetAmount = t.GrossRent - totalDeductions
	if t.NetAmount < 0 {
		t.NetAmount = 0
	}
	t.refreshEconomicBreakdown()
}

func (t *OwnerTransfer) refreshEconomicBreakdown() {
	if t.EconomicBreakdown == nil {
		return
	}
	t.EconomicBreakdown.DeductionsCents = t.EconomicBreakdown.DiscountCents +
		t.EconomicBreakdown.IntermediationRevenueCents +
		amountToCents(t.AdminFeeAmount+t.MaintenanceDeductions+t.OtherDeductions)
	if t.OwnerBalance != nil {
		t.EconomicBreakdown.AvailableCents = t.OwnerBalance.AvailableCents()
		t.EconomicBreakdown.OwnerPayableCents = t.OwnerBalance.TotalCents
		t.EconomicBreakdown.ReservedCents = t.OwnerBalance.ReservedCents
	} else {
		t.EconomicBreakdown.AvailableCents = amountToCents(t.NetAmount)
	}
}

// SetAllocatedOwnerBalance switches a transfer to the allocation-aware
// calculation without changing the legacy constructor signature. The exact
// admin-fee amount comes from accumulated payment allocations, so Calculate
// must not recompute it from a percentage and reintroduce rounding/base drift.
func (t *OwnerTransfer) SetAllocatedOwnerBalance(balance OwnerBalance, adminFeeCents int64) error {
	validated, err := NewOwnerBalance(balance.TotalCents, balance.ReservedCents, balance.BlockedCents)
	if err != nil {
		return err
	}
	if adminFeeCents < 0 {
		return errors.New("admin fee cannot be negative")
	}
	if err := t.rebuildAllocatedOwnerBalance(validated.TotalCents, validated.ReservedCents); err != nil {
		return err
	}
	t.AdminFeeAmount = centsToAmount(adminFeeCents)
	t.Calculate()
	return nil
}

// ApplyOwnerBalanceMovement appends one reserve consumption/restoration or
// release exactly once and derives the current buckets from the immutable
// allocation basis.
func (t *OwnerTransfer) ApplyOwnerBalanceMovement(movement OwnerBalanceMovement) (bool, error) {
	var err error
	movement, err = normalizeOwnerBalanceMovement(movement)
	if err != nil {
		return false, err
	}
	for _, existing := range t.OwnerBalanceMovements {
		existing, err = normalizeOwnerBalanceMovement(existing)
		if err != nil {
			return false, fmt.Errorf("invalid persisted owner balance movement: %w", err)
		}
		if existing.ID == movement.ID {
			// An omitted retry timestamp adopts the already-frozen timestamp;
			// an explicit divergent timestamp or actor is a provenance conflict.
			if movement.At.IsZero() {
				movement.At = existing.At
			}
			if !sameOwnerBalanceMovementData(existing, movement) {
				return false, errors.New("owner balance movement id reused with different data")
			}
			return false, nil
		}
	}
	if movement.At.IsZero() {
		movement.At = time.Now().UTC()
	} else {
		movement.At = movement.At.UTC()
	}
	if t.OwnerBalance == nil {
		return false, errors.New("owner balance movement requires an allocation-aware transfer")
	}
	baseTotal, baseReserved := t.ownerBalanceBase()
	// Rolling-deployment compatibility: older allocation-aware transfers have
	// the current balance but not the explicit immutable basis. Freeze it before
	// appending the first movement so replay validation does not mistake the
	// zero-valued missing fields for an economic change.
	previousBaseTotal, previousBaseReserved := t.OwnerBalanceBaseTotalCents, t.OwnerBalanceBaseReservedCents
	if len(t.OwnerBalanceMovements) == 0 && previousBaseTotal == 0 && previousBaseReserved == 0 {
		t.OwnerBalanceBaseTotalCents = baseTotal
		t.OwnerBalanceBaseReservedCents = baseReserved
	}
	t.OwnerBalanceMovements = append(t.OwnerBalanceMovements, movement)
	if err := t.rebuildAllocatedOwnerBalance(baseTotal, baseReserved); err != nil {
		t.OwnerBalanceMovements = t.OwnerBalanceMovements[:len(t.OwnerBalanceMovements)-1]
		t.OwnerBalanceBaseTotalCents = previousBaseTotal
		t.OwnerBalanceBaseReservedCents = previousBaseReserved
		return false, err
	}
	t.Calculate()
	t.UpdatedAt = movement.At
	t.History = append(t.History, HistoryEntry{
		At: movement.At, Action: "managed_bill_" + string(movement.Type), Actor: movement.Actor,
		Note: movement.ManagedBillID,
	})
	// A movimentação pode ter mudado o que há a repassar nas duas direções: o
	// consumo da reserva libera o repasse para encerramento, e o estorno devolve
	// saldo a um repasse que já tinha sido encerrado sem saldo. Sem isso o
	// repasse fica preso em PENDING para sempre (ou PROCESSED sem lastro),
	// porque nenhuma outra transição reavalia o saldo depois da conta
	// administrada.
	t.SyncNoPayableBalanceState(movement.At)
	return true, nil
}

func normalizeOwnerBalanceMovement(movement OwnerBalanceMovement) (OwnerBalanceMovement, error) {
	movement.ID = strings.TrimSpace(movement.ID)
	movement.ManagedBillID = strings.TrimSpace(movement.ManagedBillID)
	movement.PaymentAttemptID = strings.TrimSpace(movement.PaymentAttemptID)
	movement.Actor = strings.TrimSpace(movement.Actor)
	if !movement.At.IsZero() {
		movement.At = movement.At.UTC()
	}
	if movement.ID == "" || movement.ManagedBillID == "" || movement.AmountCents <= 0 {
		return OwnerBalanceMovement{}, errors.New("owner balance movement requires id, managed_bill_id and positive amount")
	}
	switch movement.Type {
	case OwnerBalanceMovementConsumeReserved:
		// Rolling-deployment compatibility: pre-attempt consume movements used
		// the payment journal source as their movement ID. That identity is a
		// safe, deterministic attempt ID for replay and future reversal.
		if movement.PaymentAttemptID == "" {
			movement.PaymentAttemptID = movement.ID
		}
	case OwnerBalanceMovementRestoreReserved:
		if movement.PaymentAttemptID == "" {
			return OwnerBalanceMovement{}, errors.New("restore_reserved movement requires payment_attempt_id")
		}
	case OwnerBalanceMovementReleaseReserved:
		if movement.PaymentAttemptID != "" {
			return OwnerBalanceMovement{}, errors.New("release_reserved movement cannot reference a payment attempt")
		}
	default:
		return OwnerBalanceMovement{}, errors.New("invalid owner balance movement type")
	}
	return movement, nil
}

func sameOwnerBalanceMovementData(left, right OwnerBalanceMovement) bool {
	return left.ManagedBillID == right.ManagedBillID &&
		left.PaymentAttemptID == right.PaymentAttemptID &&
		left.Type == right.Type && left.AmountCents == right.AmountCents &&
		left.Actor == right.Actor && left.At.Equal(right.At)
}

// ExpectedOwnerBalanceForBase replays lifecycle movements without mutating the
// transfer. When movements already exist, changing their allocation-derived
// basis fails closed.
func (t *OwnerTransfer) ExpectedOwnerBalanceForBase(baseTotalCents, baseReservedCents int64) (OwnerBalance, error) {
	if len(t.OwnerBalanceMovements) > 0 &&
		(t.OwnerBalanceBaseTotalCents != baseTotalCents || t.OwnerBalanceBaseReservedCents != baseReservedCents) {
		return OwnerBalance{}, ErrOwnerBalanceBaseChanged
	}
	if _, err := NewOwnerBalance(baseTotalCents, baseReservedCents, 0); err != nil {
		return OwnerBalance{}, err
	}
	currentTotal, currentReserved := baseTotalCents, baseReservedCents
	seen := make(map[string]OwnerBalanceMovement, len(t.OwnerBalanceMovements))
	type managedBillMovementState struct {
		amountCents   int64
		activeAttempt string
		attempts      map[string]bool // false=consumed, true=restored
		released      bool
	}
	bills := make(map[string]*managedBillMovementState)
	for _, persisted := range t.OwnerBalanceMovements {
		movement, err := normalizeOwnerBalanceMovement(persisted)
		if err != nil {
			return OwnerBalance{}, fmt.Errorf("invalid persisted owner balance movement: %w", err)
		}
		if previous, ok := seen[movement.ID]; ok {
			if !sameOwnerBalanceMovementData(previous, movement) {
				return OwnerBalance{}, errors.New("duplicated owner balance movement has divergent data")
			}
			continue
		}
		seen[movement.ID] = movement
		state := bills[movement.ManagedBillID]
		if state == nil {
			state = &managedBillMovementState{amountCents: movement.AmountCents, attempts: make(map[string]bool)}
			bills[movement.ManagedBillID] = state
		} else if state.amountCents != movement.AmountCents {
			return OwnerBalance{}, errors.New("managed bill owner balance movement amount changed between attempts")
		}
		switch movement.Type {
		case OwnerBalanceMovementConsumeReserved:
			if state.released {
				return OwnerBalance{}, errors.New("cannot consume reserve after managed bill release")
			}
			if state.activeAttempt != "" {
				return OwnerBalance{}, errors.New("managed bill already has an active consumed payment attempt")
			}
			if _, exists := state.attempts[movement.PaymentAttemptID]; exists {
				return OwnerBalance{}, errors.New("payment attempt reserve was already consumed")
			}
			currentTotal -= movement.AmountCents
			currentReserved -= movement.AmountCents
			state.activeAttempt = movement.PaymentAttemptID
			state.attempts[movement.PaymentAttemptID] = false
		case OwnerBalanceMovementRestoreReserved:
			restored, exists := state.attempts[movement.PaymentAttemptID]
			if state.released || !exists || restored || state.activeAttempt != movement.PaymentAttemptID {
				return OwnerBalance{}, errors.New("restore_reserved requires the matching active consumed payment attempt")
			}
			currentTotal += movement.AmountCents
			currentReserved += movement.AmountCents
			state.activeAttempt = ""
			state.attempts[movement.PaymentAttemptID] = true
		case OwnerBalanceMovementReleaseReserved:
			if state.released {
				return OwnerBalance{}, errors.New("managed bill reserve was already released")
			}
			if state.activeAttempt != "" {
				return OwnerBalance{}, errors.New("cannot release reserve while a payment attempt is consumed")
			}
			currentReserved -= movement.AmountCents
			state.released = true
		}
		if currentTotal < 0 || currentReserved < 0 || currentReserved > currentTotal {
			return OwnerBalance{}, errors.New("owner balance movements exceed the allocation-derived reserve")
		}
		if currentTotal > baseTotalCents || currentReserved > baseReservedCents {
			return OwnerBalance{}, errors.New("owner balance restoration exceeds the allocation-derived basis")
		}
	}
	blocked := amountToCents(t.MaintenanceDeductions + t.OtherDeductions)
	maxBlocked := currentTotal - currentReserved
	if blocked > maxBlocked {
		blocked = maxBlocked
	}
	return NewOwnerBalance(currentTotal, currentReserved, blocked)
}

func (t *OwnerTransfer) rebuildAllocatedOwnerBalance(baseTotalCents, baseReservedCents int64) error {
	expected, err := t.ExpectedOwnerBalanceForBase(baseTotalCents, baseReservedCents)
	if err != nil {
		return err
	}
	t.OwnerBalanceBaseTotalCents = baseTotalCents
	t.OwnerBalanceBaseReservedCents = baseReservedCents
	t.OwnerBalance = &expected
	return nil
}

func (t *OwnerTransfer) ownerBalanceBase() (int64, int64) {
	if t.OwnerBalanceBaseTotalCents > 0 || t.OwnerBalanceBaseReservedCents > 0 {
		return t.OwnerBalanceBaseTotalCents, t.OwnerBalanceBaseReservedCents
	}
	// Rolling-deployment compatibility for allocation-aware rows written before
	// the immutable base fields existed. Such rows cannot yet contain movements.
	if t.OwnerBalance != nil {
		return t.OwnerBalance.TotalCents, t.OwnerBalance.ReservedCents
	}
	return 0, 0
}

func amountToCents(amount float64) int64 {
	if amount <= 0 {
		return 0
	}
	return int64(math.Round(amount * 100))
}

func centsToAmount(cents int64) float64 {
	return float64(cents) / 100
}

// OwnerBalance representa os bolsões do subledger da conta corrente do
// proprietário (seções 10.1.1 e 10.2 do compêndio): saldo total, reservado
// para despesas administradas e bloqueado. O disponível para repasse é sempre
// derivado (Total - Reservado - Bloqueado), nunca persistido separadamente,
// para evitar que reserva e disponível divirjam.
type OwnerBalance struct {
	TotalCents    int64 `json:"total_cents"`
	ReservedCents int64 `json:"reserved_cents"`
	BlockedCents  int64 `json:"blocked_cents"`
}

// AvailableCents devolve o saldo disponível para repasse, nunca negativo.
func (b OwnerBalance) AvailableCents() int64 {
	available := b.TotalCents - b.ReservedCents - b.BlockedCents
	if available < 0 {
		return 0
	}
	return available
}

// MarshalJSON expõe available_cents (derivado) no payload de API/UI. O valor
// nunca é persistido — é sempre computado a partir dos buckets para que
// reserva e disponível não divirjam — mas o frontend/consumo usa o campo para
// decidir se o repasse tem cobertura econômica exata.
func (b OwnerBalance) MarshalJSON() ([]byte, error) {
	type balanceAlias OwnerBalance
	return json.Marshal(&struct {
		AvailableCents int64 `json:"available_cents"`
		balanceAlias
	}{
		AvailableCents: b.AvailableCents(),
		balanceAlias:   balanceAlias(b),
	})
}

// NewOwnerBalance monta o subledger validando que os bolsões não são negativos
// e que reservado + bloqueado não excedem o saldo total.
func NewOwnerBalance(totalCents, reservedCents, blockedCents int64) (OwnerBalance, error) {
	if totalCents < 0 || reservedCents < 0 || blockedCents < 0 {
		return OwnerBalance{}, errors.New("owner balance buckets cannot be negative")
	}
	if reservedCents+blockedCents > totalCents {
		return OwnerBalance{}, errors.New("reserved + blocked cannot exceed total owner balance")
	}
	return OwnerBalance{
		TotalCents:    totalCents,
		ReservedCents: reservedCents,
		BlockedCents:  blockedCents,
	}, nil
}
