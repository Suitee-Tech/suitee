package transaction

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
)

type Type string

const (
	TypeReceita  Type = "receita"
	TypeDespesa  Type = "despesa"
	TypeRepasse  Type = "repasse"
	TypeComissao Type = "comissao"
	TypeEstorno  Type = "estorno"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusPaid      Status = "PAID"
	StatusOverdue   Status = "OVERDUE"
	StatusReversed  Status = "REVERSED"
	StatusCancelled Status = "CANCELLED"

	// Legacy Portuguese aliases — kept for backward compatibility with
	// existing database records and API consumers that have not migrated.
	// These constants have the SAME underlying string values as the
	// canonical English names (e.g. StatusPendente == "PENDING"), so
	// they work as aliases in code but NOT for JSON unmarshalling from
	// Portuguese frontend strings.
	_StatusPendente  Status = StatusPending
	_StatusPago      Status = StatusPaid
	_StatusEmAtraso  Status = StatusOverdue
	_StatusEstornado Status = StatusReversed
	_StatusCancelado Status = StatusCancelled

	// Portuguese frontend values — the frontend (NovaTransacaoModal,
	// NovoLancamentoModal) sends these literal strings.
	StatusPendente  Status = "Pendente"
	StatusPago      Status = "Pago"
	StatusEmAtraso  Status = "Em Atraso"
	StatusEstornado Status = "Estornado"
	StatusCancelado Status = "Cancelado"
)

// ValidTypes maps all known transaction types for validation.
var ValidTypes = map[Type]bool{
	TypeReceita:  true,
	TypeDespesa:  true,
	TypeRepasse:  true,
	TypeComissao: true,
	TypeEstorno:  true,
}

// ValidStatuses maps the canonical persisted lifecycle statuses. Legacy
// Portuguese strings sent by older API consumers are normalized to these
// values at the handler boundary via CanonicalStatus before validation, so
// they are intentionally not accepted as valid persisted values anymore.
var ValidStatuses = map[Status]bool{
	StatusPending:   true,
	StatusPaid:      true,
	StatusOverdue:   true,
	StatusReversed:  true,
	StatusCancelled: true,
}

// IsValidType checks whether a type string is a known Type.
func IsValidType(t Type) bool {
	return ValidTypes[t]
}

// IsValidStatus checks whether a status string is a known Status.
func IsValidStatus(s Status) bool {
	return ValidStatuses[s]
}

// CanonicalStatus maps legacy Portuguese API values to the canonical
// lifecycle values persisted by current writers.
func CanonicalStatus(s Status) Status {
	switch s {
	case StatusPendente:
		return StatusPending
	case StatusPago:
		return StatusPaid
	case StatusEmAtraso:
		return StatusOverdue
	case StatusEstornado:
		return StatusReversed
	case StatusCancelado:
		return StatusCancelled
	default:
		return s
	}
}

// IsTerminalStatus reports whether a transaction has reached an irreversible
// reversed/cancelled state. Legacy Portuguese rows are terminal as well.
func IsTerminalStatus(s Status) bool {
	canonical := CanonicalStatus(s)
	return canonical == StatusReversed || canonical == StatusCancelled
}

// ValidateStatusTransition protects terminal lifecycle states when a generic
// update changes transaction fields.
func ValidateStatusTransition(from, to Status) error {
	if IsTerminalStatus(from) && CanonicalStatus(to) != CanonicalStatus(from) {
		return fmt.Errorf("%w: %s is terminal", ErrInvalidStatusTransition, CanonicalStatus(from))
	}
	return nil
}

// EconomicBreakdown freezes the ownership of a manually posted receipt. It
// lets settlement create the owner obligation without reinterpreting a batch
// from its display labels or from mutable UI state.
type EconomicBreakdown struct {
	Flow                   string `json:"flow" bson:"flow"`
	GrossCents             int64  `json:"gross_cents" bson:"gross_cents"`
	CommissionPoolCents    int64  `json:"commission_pool_cents" bson:"commission_pool_cents"`
	CommissionPayableCents int64  `json:"commission_payable_cents" bson:"commission_payable_cents"`
	// Funding is a receipt-specific subset of the existing broker obligation,
	// never a second payable. The flag distinguishes known zero from legacy data.
	CommissionFundedCents  int64 `json:"commission_funded_cents" bson:"commission_funded_cents"`
	CommissionFundingKnown bool  `json:"commission_funding_known,omitempty" bson:"commission_funding_known,omitempty"`
	PartnerPayableCents    int64 `json:"partner_payable_cents" bson:"partner_payable_cents"`
	AgencyRetainedCents    int64 `json:"agency_retained_cents" bson:"agency_retained_cents"`
	OwnerPayableCents      int64 `json:"owner_payable_cents" bson:"owner_payable_cents"`
	DiscountCents          int64 `json:"discount_cents" bson:"discount_cents"`
	// AdminFeeCents is the recurring administration fee (taxa de administração)
	// retained by the agency on RENT receipts. It is zero for SALE receipts and
	// for legacy rows predating rental manual receipts.
	AdminFeeCents int64 `json:"admin_fee_cents" bson:"admin_fee_cents"`
}

type Transaction struct {
	ID                    string     `json:"id" bson:"_id"`
	ContractID            string     `json:"contract_id" bson:"contract_id"`
	Type                  Type       `json:"type" bson:"type"`
	Description           string     `json:"description" bson:"description"`
	Value                 float64    `json:"value" bson:"value"`
	Date                  time.Time  `json:"date" bson:"date"`
	Status                Status     `json:"status" bson:"status"`
	DueDate               *time.Time `json:"due_date,omitempty" bson:"due_date,omitempty"`
	PaymentDate           *time.Time `json:"payment_date,omitempty" bson:"payment_date,omitempty"`
	PaymentMethod         string     `json:"payment_method,omitempty" bson:"payment_method,omitempty"`
	Category              string     `json:"category,omitempty" bson:"category,omitempty"`
	PayerName             string     `json:"payer_name,omitempty" bson:"payer_name,omitempty"`
	PayerKind             string     `json:"payer_kind,omitempty" bson:"payer_kind,omitempty"`
	Month                 string     `json:"month,omitempty" bson:"month,omitempty"`
	GrossValue            *float64   `json:"gross_value,omitempty" bson:"gross_value,omitempty"`
	AdminFee              *float64   `json:"admin_fee,omitempty" bson:"admin_fee,omitempty"`
	MaintenanceDeductions *float64   `json:"maintenance_deductions,omitempty" bson:"maintenance_deductions,omitempty"`
	UtilityDeductions     *float64   `json:"utility_deductions,omitempty" bson:"utility_deductions,omitempty"`
	// PaymentPlan carries the payment plan programmed in the pipeline when the
	// operator posted a manual receipt (ex.: "Entrada + 11x de R$ 8.333,33").
	// It is informational provenance and does not affect the posting math.
	PaymentPlan string `json:"payment_plan,omitempty" bson:"payment_plan,omitempty"`
	// ProofURL é o comprovante (foto/PDF/link) anexado na baixa do lançamento.
	// Padronizado com o fluxo de cobranças/comissões/repasse: qualquer baixa
	// aceita documento e o guarda no lançamento para o recibo e auditoria.
	ProofURL string `json:"proof_url,omitempty" bson:"proof_url,omitempty"`
	// Note é a observação do operador na baixa (ex.: "TED 14:30, conferido").
	Note string `json:"note,omitempty" bson:"note,omitempty"`
	// OperationID groups every posting created by the same manual action. It is
	// deliberately distinct from SourceEventID, which identifies one line for
	// idempotency, and from ID/PostingID, which identifies the persisted entry.
	OperationID   string `json:"operation_id,omitempty" bson:"operation_id,omitempty"`
	PostingID     string `json:"posting_id,omitempty" bson:"posting_id,omitempty"`
	SourceEventID string `json:"source_event_id,omitempty" bson:"source_event_id,omitempty"`
	// BatchFingerprint binds an idempotency key to the complete manual command,
	// including its transaction and Commission cardinalities. The counters make
	// that invariant inspectable without inferring it from row presence.
	BatchFingerprint  string             `json:"batch_fingerprint,omitempty" bson:"batch_fingerprint,omitempty"`
	BatchLineCount    int                `json:"batch_line_count,omitempty" bson:"batch_line_count,omitempty"`
	CommissionCount   int                `json:"commission_count,omitempty" bson:"commission_count,omitempty"`
	DealID            string             `json:"deal_id,omitempty" bson:"deal_id,omitempty"`
	EconomicBreakdown *EconomicBreakdown `json:"economic_breakdown,omitempty" bson:"economic_breakdown,omitempty"`
	PlanHash          string             `json:"plan_hash,omitempty" bson:"plan_hash,omitempty"` // FIN-COM-04: links transaction to its resolved commission plan
	// BrokerID and Role make each commission ledger line correlate exactly with
	// its Commission aggregate. PlanHash alone identifies the whole plan and is
	// therefore insufficient when a contract has more than one beneficiary.
	BrokerID  string    `json:"broker_id,omitempty" bson:"broker_id,omitempty"`
	Role      string    `json:"role,omitempty" bson:"role,omitempty"`
	TenantID  string    `json:"tenant_id" bson:"tenant_id"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

func New(contractID string, txType Type, description string, value float64, date time.Time, tenantID string) *Transaction {
	// Normalize legacy Portuguese statuses — the canonical form is English.
	now := time.Now()
	return &Transaction{
		ID:          uuid.New().String(),
		ContractID:  contractID,
		Type:        txType,
		Description: description,
		Value:       value,
		Date:        date,
		Status:      StatusPending,
		TenantID:    tenantID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// NewCreditNote registra uma nota de crédito: devolução de valor de terceiro
// recebido a maior (ex.: manutenção cobrada acima do custo final). É o único
// posting negativo fora do Reverse — a direção continua vindo do Type, e a
// nota não é receita nem despesa: apenas desfaz o valor que não pertencia à
// imobiliária.
func NewCreditNote(contractID, description string, amount float64, date time.Time, tenantID string) *Transaction {
	tx := New(contractID, TypeEstorno, description, -math.Abs(amount), date, tenantID)
	tx.Status = StatusPending
	return tx
}

// NewFromMoney creates a Transaction using a Money value object (cents
// precision) — bridges the gap with Charge entities that use Money.
func NewFromMoney(contractID string, txType Type, description string, money valueobjects.Money, date time.Time, tenantID string) *Transaction {
	return New(contractID, txType, description, money.Float(), date, tenantID)
}

// SetValueFromMoney overwrites the Value field from a Money value object.
func (t *Transaction) SetValueFromMoney(m valueobjects.Money) {
	t.Value = m.Float()
}

// ValueAsMoney returns the Value field as a Money value object.
func (t *Transaction) ValueAsMoney() valueobjects.Money {
	return valueobjects.NewMoneyFromFloat(t.Value)
}

// SetGrossValueFromMoney sets the optional GrossValue from a Money.
func (t *Transaction) SetGrossValueFromMoney(m valueobjects.Money) {
	v := m.Float()
	t.GrossValue = &v
}

// GrossValueAsMoney returns GrossValue as Money, or zero Money if nil.
func (t *Transaction) GrossValueAsMoney() valueobjects.Money {
	if t.GrossValue == nil {
		return valueobjects.Money{}
	}
	return valueobjects.NewMoneyFromFloat(*t.GrossValue)
}

// Validate performs domain-level validation on the transaction fields.
func (t *Transaction) Validate() error {
	if !IsValidType(t.Type) {
		return fmt.Errorf("invalid transaction type: %s", t.Type)
	}
	if !IsValidStatus(t.Status) {
		return fmt.Errorf("invalid transaction status: %s", t.Status)
	}
	// Direction is carried by Type, never by the sign of a normal posting.
	// This prevents expenses/repasses from being double-negated by consumers.
	// A reversal is the only negative posting and is created by Reverse.
	if t.Type == TypeEstorno {
		if t.Value >= 0 {
			return fmt.Errorf("%w: reversal value must be negative", ErrInvalidTransactionValue)
		}
	} else if t.Value <= 0 {
		return fmt.Errorf("%w: normal transaction value must be positive", ErrInvalidTransactionValue)
	}
	if t.TenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(t.Description) == "" && strings.TrimSpace(t.Category) == "" && t.ContractID == "" {
		return fmt.Errorf("a manual transaction requires a description or category when contract_id is empty")
	}
	// PaymentPlan é proveniência informacional; um limite evita abuso no
	// JSONB sem perder nenhum valor legítimo de plano de pagamento.
	if len(t.PaymentPlan) > 512 {
		return fmt.Errorf("payment_plan must be at most 512 characters")
	}
	return nil
}

// MarkPaid records a manual baixa. Only PENDING and OVERDUE entries can be
// settled; PAID and REVERSED are not reopened or rewritten by retries.
func (t *Transaction) MarkPaid(paymentDate time.Time, method string) error {
	status := CanonicalStatus(t.Status)
	if status != StatusPending && status != StatusOverdue {
		return fmt.Errorf("%w: only PENDING or OVERDUE transactions can be settled (current: %s)", ErrInvalidStatusTransition, t.Status)
	}
	if paymentDate.IsZero() {
		paymentDate = time.Now()
	}
	t.Status = StatusPaid
	t.PaymentDate = &paymentDate
	if method != "" {
		t.PaymentMethod = method
	}
	t.UpdatedAt = time.Now()
	return nil
}

// MarkReversed moves a paid transaction to the terminal REVERSED state.
func (t *Transaction) MarkReversed() error {
	if t.Type == TypeEstorno {
		return fmt.Errorf("%w: an estorno transaction cannot be reversed", ErrInvalidStatusTransition)
	}
	if CanonicalStatus(t.Status) != StatusPaid {
		return fmt.Errorf("%w: only PAID transactions can be reversed (current: %s)", ErrInvalidStatusTransition, t.Status)
	}
	t.Status = StatusReversed
	t.UpdatedAt = time.Now()
	return nil
}

// MarkCancelled closes an unsettled posting without generating cash movement.
// It is used by the Commission aggregate lifecycle for a linked manual
// obligation; paid postings must follow the explicit reversal path instead.
func (t *Transaction) MarkCancelled() error {
	status := CanonicalStatus(t.Status)
	if status == StatusCancelled {
		return nil
	}
	if status != StatusPending && status != StatusOverdue {
		return fmt.Errorf("%w: only PENDING or OVERDUE transactions can be cancelled (current: %s)", ErrInvalidStatusTransition, t.Status)
	}
	t.Status = StatusCancelled
	t.UpdatedAt = time.Now()
	return nil
}

// Reopen devolve um lançamento CANCELADO ao estado PENDING, espelho do
// MarkCancelled. Existe para o relançamento de uma obrigação cuja ordem de
// pagamento foi cancelada e regerada: a comissão volta a PENDING e o
// lançamento que a representa precisa voltar junto, senão o razão fica com uma
// obrigação viva sem lastro contábil (ou com um cancelamento órfão).
//
// Um lançamento PAGO não reabre: estorno é o caminho (`Reverse`).
func (t *Transaction) Reopen() error {
	status := CanonicalStatus(t.Status)
	if status == StatusPending {
		return nil
	}
	if status != StatusCancelled {
		return fmt.Errorf("%w: only CANCELLED transactions can be reopened (current: %s)", ErrInvalidStatusTransition, t.Status)
	}
	t.Status = StatusPending
	t.UpdatedAt = time.Now()
	return nil
}

// Reverse creates a new Transaction that negates this one (estorno). The
// caller must persist the returned transaction via the repository.
func (t *Transaction) Reverse(reason string, reversedID string) (*Transaction, error) {
	if t.Type == TypeEstorno {
		return nil, fmt.Errorf("%w: an estorno transaction cannot be reversed", ErrInvalidStatusTransition)
	}
	if CanonicalStatus(t.Status) != StatusPaid {
		return nil, fmt.Errorf("%w: only PAID transactions can be reversed (current: %s)", ErrInvalidStatusTransition, t.Status)
	}
	now := time.Now()
	return &Transaction{
		ID:          uuid.New().String(),
		ContractID:  t.ContractID,
		Type:        TypeEstorno,
		Description: "Estorno: " + reason + " (ref: " + reversedID + ")",
		Value:       -t.Value,
		Date:        now,
		Status:      StatusPaid,
		Category:    t.Category,
		TenantID:    t.TenantID,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

type Repository interface {
	Create(ctx interface{}, t *Transaction) error
	GetByID(ctx interface{}, tenantID, id string) (*Transaction, error)
	Update(ctx interface{}, t *Transaction) error
	Delete(ctx interface{}, id, tenantID string) error
	List(ctx interface{}, tenantID string, contractID string, limit, offset int64) ([]*Transaction, int64, error)
}
