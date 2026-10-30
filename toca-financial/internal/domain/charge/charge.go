package charge

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/pkg/timeutil"
)

type Status string

const (
	StatusPending       Status = "PENDING"
	StatusPaid          Status = "PAID"
	StatusOverdue       Status = "OVERDUE"
	StatusCancelled     Status = "CANCELLED"
	StatusPartiallyPaid Status = "PARTIALLY_PAID"
	StatusReversed      Status = "REVERSED"
)

// IsOutstandingStatus reports whether a charge still participates in the
// receivables/dunning lifecycle. Keeping this predicate in the domain avoids
// accidentally treating terminal REVERSED rows as collectible in reminders,
// late-fee calculation or collection reconciliation.
func IsOutstandingStatus(status Status) bool {
	switch status {
	case StatusPending, StatusOverdue, StatusPartiallyPaid:
		return true
	default:
		return false
	}
}

// IsClosedStatus reports whether a charge no longer has an outstanding
// receivable. PAID may still transition once to REVERSED, but both states are
// closed for billing and delinquency purposes; CANCELLED is terminal as well.
func IsClosedStatus(status Status) bool {
	switch status {
	case StatusPaid, StatusCancelled, StatusReversed:
		return true
	default:
		return false
	}
}

type ChargeType string

const (
	TypeRent        ChargeType = "RENT"
	TypeSale        ChargeType = "SALE"
	TypeCondominio  ChargeType = "CONDOMINIO"
	TypeIPTU        ChargeType = "IPTU"
	TypeUtility     ChargeType = "UTILITY"
	TypeMaintenance ChargeType = "MAINTENANCE"
	TypeOther       ChargeType = "OTHER"
)

// SettlementMode classifies how a line item collected from the payer settles
// to its economic owner (seção 9.4 do compêndio financeiro). The values mirror
// the compendium's snake_case enum verbatim so persisted JSON stays compatible
// with the reference model.
type SettlementMode string

const (
	// SettlementModeCollection: a imobiliária arrecada o valor em nome do dono
	// econômico e depois o repassa (ex.: aluguel).
	SettlementModeCollection SettlementMode = "arrecadacao"
	// SettlementModeDirectLiquidation: a imobiliária paga o credor diretamente
	// em nome do dono econômico (ex.: condomínio, IPTU).
	SettlementModeDirectLiquidation SettlementMode = "liquidacao_direta"
	// SettlementModeReimbursement: o dono econômico paga primeiro e o pagador
	// reembolsa depois.
	SettlementModeReimbursement SettlementMode = "reembolso"
	// SettlementModeOffset: liquidação por compensação contra outra obrigação.
	SettlementModeOffset SettlementMode = "compensacao"
)

// IsValidSettlementMode reports whether m is a known settlement mode. The
// empty value is treated as valid — linhas gravadas antes desta dimensão
// existir não carregam modo.
func IsValidSettlementMode(m SettlementMode) bool {
	switch m {
	case "", SettlementModeCollection, SettlementModeDirectLiquidation, SettlementModeReimbursement, SettlementModeOffset:
		return true
	default:
		return false
	}
}

// AllocationType classifies the destination of a slice of a payment when it
// settles (seção 37.4 do compêndio). The values mirror the compendium's
// snake_case enum verbatim so persisted JSON stays compatible.
type AllocationType string

const (
	// AllocationTypeRevenue: receita própria da imobiliária (taxa de
	// administração, honorários).
	AllocationTypeRevenue AllocationType = "revenue"
	// AllocationTypeOwnerPayable: repasse a pagar ao proprietário.
	AllocationTypeOwnerPayable AllocationType = "owner_payable"
	// AllocationTypeCommissionPayable: comissão a pagar ao corretor/parceiro.
	AllocationTypeCommissionPayable AllocationType = "commission_payable"
	// AllocationTypePartnerPayable: valor devido a empresa parceira.
	AllocationTypePartnerPayable AllocationType = "partner_payable"
	// AllocationTypeRoyaltyPayable: royalty a pagar.
	AllocationTypeRoyaltyPayable AllocationType = "royalty_payable"
	// AllocationTypeTaxWithheld: tributo retido na fonte.
	AllocationTypeTaxWithheld AllocationType = "tax_withheld"
	// AllocationTypeReceivable: direito a receber do responsável por um custo
	// que a imobiliária adiantou (manutenção, vistoria). O valor não é receita
	// própria nem despesa da imobiliária: ela apenas intermediou o pagamento e
	// está sendo reembolsada. O recebimento baixa o adiantamento registrado em
	// contas a receber.
	AllocationTypeReceivable AllocationType = "receivable"
)

// IsValidAllocationType reports whether t is a known allocation type. The
// empty value is invalid: an allocation must declare its destination.
func IsValidAllocationType(t AllocationType) bool {
	switch t {
	case AllocationTypeRevenue, AllocationTypeOwnerPayable, AllocationTypeCommissionPayable,
		AllocationTypePartnerPayable, AllocationTypeRoyaltyPayable, AllocationTypeTaxWithheld,
		AllocationTypeReceivable:
		return true
	default:
		return false
	}
}

// AllocationPurpose refines allocations that share the same accounting
// destination. Rental administration and intermediation are both agency
// revenue, but only the former is recurring and only the latter finances the
// first-rent commission/carry-forward waterfall.
type AllocationPurpose string

const (
	AllocationPurposeSaleCommissionBroker       AllocationPurpose = "sale_commission_broker"
	AllocationPurposeAdminFee                   AllocationPurpose = "rental_admin_fee"
	AllocationPurposeRentalIntermediationBroker AllocationPurpose = "rental_intermediation_broker"
	AllocationPurposeRentalIntermediationAgency AllocationPurpose = "rental_intermediation_agency"
	AllocationPurposeOwnerRent                  AllocationPurpose = "owner_rent"
)

func IsValidAllocationPurpose(p AllocationPurpose) bool {
	switch p {
	case "", AllocationPurposeSaleCommissionBroker, AllocationPurposeAdminFee, AllocationPurposeRentalIntermediationBroker,
		AllocationPurposeRentalIntermediationAgency, AllocationPurposeOwnerRent:
		return true
	default:
		return false
	}
}

// AllocationMethod identifies the explicit rule used to distribute a
// payment across charge items. Versioning the method makes partial-payment
// allocation reproducible instead of leaving the appropriation order
// implicit (compendium section 9.1).
type AllocationMethod string

const (
	AllocationMethodProportionalV1 AllocationMethod = "proportional_v1"
)

func IsValidAllocationMethod(method AllocationMethod) bool {
	return method == AllocationMethodProportionalV1
}

type Charge struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	ContractID string `json:"contract_id"`
	// DealID is the upstream deal that originated this charge (when applicable).
	// Persisted as `metadata->>'deal_id'` since the table uses JSONB storage;
	// indexed via supabase migration 20260510120000_charge_rental_indexes.sql.
	// Lets queries like "all charges of deal X" or "lead financial summary"
	// traverse the chain without joining via contract.
	DealID string `json:"deal_id,omitempty"`
	// PaymentURL é o link de cobrança (Pix/boleto) emitido pelo provedor. Ficava
	// só na resposta da emissão e se perdia: o portal do cliente exibia os botões
	// de pagar/2ª via SEM destino porque nada gravava o link. Agora é persistido
	// com o identificador externo e a validade, para o cliente reabrir o mesmo
	// link em vez de gerar um novo a cada visita.
	PaymentURL        string     `json:"payment_url,omitempty"`
	PaymentExternalID string     `json:"payment_external_id,omitempty"`
	PaymentProvider   string     `json:"payment_provider,omitempty"`
	PaymentExpiresAt  *time.Time `json:"payment_expires_at,omitempty"`
	// LeadID denormalizes the customer lead id for the same reason — we want
	// to answer "everything this lead owes" without going through deal+contract.
	LeadID         string              `json:"lead_id,omitempty"`
	TenantPartyID  string              `json:"tenant_party_id"`
	OwnerID        string              `json:"owner_id"`
	PropertyID     string              `json:"property_id"`
	Type           ChargeType          `json:"type"`
	Description    string              `json:"description"`
	ReferenceMonth string              `json:"reference_month"`
	Amount         valueobjects.Money  `json:"amount"`
	LateFee        valueobjects.Money  `json:"late_fee"`
	Interest       valueobjects.Money  `json:"interest"`
	Discount       valueobjects.Money  `json:"discount"`
	TotalAmount    valueobjects.Money  `json:"total_amount"`
	DueDate        time.Time           `json:"due_date"`
	PaidAt         *time.Time          `json:"paid_at,omitempty"`
	PaidAmount     *valueobjects.Money `json:"paid_amount,omitempty"`
	PaymentMethod  string              `json:"payment_method,omitempty"`
	// Payments is the append-only log of every payment received against
	// this charge. The accumulated PaidAmount is Σ Payments.AmountCents.
	Payments []PaymentEntry `json:"payments,omitempty"`
	Status   Status         `json:"status"`
	// RemindersSent is an append-only log of notification dispatches for this
	// charge. Populated by ChargeService.Notify after a successful send. UI
	// uses the count + last timestamp to badge the row and warn operators
	// before triggering duplicates within a short window.
	RemindersSent   []ReminderEntry `json:"reminders_sent,omitempty"`
	AppliedEventIDs []string        `json:"applied_event_ids,omitempty"`
	// SourceEventID is a deterministic identifier for the upstream event
	// that produced this charge — set by handlers that consume cross-service
	// events (maintenance.requested, inspection.deductions_recorded, etc.).
	// Empty for charges created directly by operators. Backed by a partial
	// unique index in supabase (migration 20260518120000) so retries from
	// the broker don't create duplicates. Derived via
	// events.DeriveNotificationID — see pkg/events/dedup.go.
	SourceEventID string `json:"source_event_id,omitempty"`
	// LineItems discriminates the components that make up Amount when a
	// single charge is a *bundle* of conceptual line items — e.g. rental
	// charges that fold in fixed bundled extras (condomínio, IPTU, taxa de
	// administração). When set, Σ LineItems.AmountCents must equal
	// Amount.Cents(); enforced by ValidateLineItems. The frontend renders
	// the breakdown so the customer sees "Aluguel R$ 2.500 + Condomínio
	// R$ 800" on a single charge. Empty for charges that are not bundles.
	LineItems []LineItem `json:"line_items,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// LineItem represents one component inside a charge bundle. SourceType +
// SourceID let the UI link back to the configuration that originated the
// item (e.g. "rental_bundled_item" with the bundled item key, or
// "utility_account" when an UtilityAccount in BUNDLED mode was inlined).
type LineItem struct {
	// ID is the stable identity used by PaymentEntry.Allocations. It is
	// generated server-side from the charge and item position so retries and
	// regenerated JSON keep the same relationship.
	ID          string `json:"id"`
	Description string `json:"description"`
	AmountCents int64  `json:"amount_cents"`
	SourceType  string `json:"source_type,omitempty"`
	SourceID    string `json:"source_id,omitempty"`
	// EconomicOwnerID is the party that economically benefits from this item
	// ("todo valor tem dono econômico", seção 9.4). Defaults to the charge
	// OwnerID when empty, preserving the pre-titularidade behavior.
	EconomicOwnerID string `json:"economic_owner_id,omitempty"`
	// SettlementMode classifies how the collected value settles to the
	// economic owner (arrecadacao, liquidacao_direta, reembolso, compensacao).
	SettlementMode SettlementMode `json:"settlement_mode,omitempty"`
	// RuleVersionID freezes the configuration/rule revision that produced
	// this item. Legacy/manual rows may use the charge-item policy version.
	RuleVersionID string `json:"rule_version_id"`
	// AllocationType lets a persisted item override the default destination
	// inferred from EconomicOwnerID. This is needed when one SALE receipt
	// contains both a third-party commission payable and the agency's retained
	// share, even though both are economically owned by the tenant.
	AllocationType AllocationType `json:"allocation_type,omitempty"`
}

// Allocation records how one slice of a payment settles to a charge item and
// its beneficiary (seção 37.4). It materializes o princípio "todo valor tem
// dono econômico": cada alocação aponta o item quitado, a parte com direito
// ao valor e a versão da regra que a produziu. Alimentada pelo waterfall de
// comissão/repasse (F2 do plano de gaps).
type Allocation struct {
	ID                 string            `json:"id"`
	PaymentID          string            `json:"payment_id"`
	ChargeItemID       string            `json:"charge_item_id"`
	AmountCents        int64             `json:"amount_cents"`
	AllocationType     AllocationType    `json:"allocation_type"`
	BeneficiaryPartyID string            `json:"beneficiary_party_id"`
	RuleVersionID      string            `json:"rule_version_id"`
	Purpose            AllocationPurpose `json:"purpose,omitempty"`
	// RelatedEntityID points to the Commission financed by an
	// rental_intermediation_broker slice. Other purposes leave it empty.
	RelatedEntityID string `json:"related_entity_id,omitempty"`
}

// ReminderEntry records one notification dispatch attempt. Channels stores
// the channels that actually delivered (e.g. ["email"], ["email","in_app"]).
// Recipient is the primary destination (email address, account_id, etc.) —
// kept loose so the audit works regardless of which channel won the race.
//
// SentBy is the typed token ("user:<account_id>", "rule:<rule_id>",
// "system"). SentByLabel is a human-readable snapshot captured at dispatch
// time so the audit stays interpretable even if the broker is removed or
// the rule renamed later. Frontend prefers the label and falls back to the
// live lookup when missing (older entries).
type ReminderEntry struct {
	SentAt      time.Time `json:"sent_at"`
	Channels    []string  `json:"channels"`
	Recipient   string    `json:"recipient,omitempty"`
	SentBy      string    `json:"sent_by,omitempty"`
	SentByLabel string    `json:"sent_by_label,omitempty"`
}

// PaymentEntry records a single payment event. Payments are append-only
// so every partial and full payment is preserved independently. The
// accumulated PaidAmount on the charge is the sum of all entries.
type PaymentEntry struct {
	// ID is the stable identity of the payment command that produced this
	// entry. Keeping it in the append-only log lets retries return the
	// original result without adding the same money twice.
	ID          string    `json:"id,omitempty"`
	AmountCents int64     `json:"amount_cents"`
	Method      string    `json:"method"`
	PaidAt      time.Time `json:"paid_at"`
	// AllocationMethod makes the payment-partial appropriation rule explicit.
	AllocationMethod AllocationMethod `json:"allocation_method,omitempty"`
	// Allocations split this payment into destination slices (owner_payable,
	// revenue, commission_payable, ...) per charge item. Empty for payments
	// recorded before the titularidade econômica dimension existed; Σ
	// Allocations.AmountCents must equal AmountCents once populated.
	Allocations []Allocation `json:"allocations,omitempty"`
}

func New(contractID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType ChargeType, description, referenceMonth string, amount float64, dueDate time.Time) (*Charge, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	now := time.Now()
	moneyAmount := valueobjects.NewMoneyFromFloat(amount)

	return &Charge{
		ID:             uuid.New().String(),
		TenantID:       tenantID,
		ContractID:     contractID,
		TenantPartyID:  tenantPartyID,
		OwnerID:        ownerID,
		PropertyID:     propertyID,
		Type:           chargeType,
		Description:    description,
		ReferenceMonth: referenceMonth,
		Amount:         moneyAmount,
		TotalAmount:    moneyAmount,
		DueDate:        dueDate,
		Status:         StatusPending,
		CreatedAt:      now,
		UpdatedAt:      now,
	}, nil
}

// NewWithLineItems mirrors New but also sets a non-empty LineItems slice.
// The sum of LineItems.AmountCents must equal amount (in cents). The
// charge is otherwise initialized identically to New — type, dueDate and
// status all flow through the same construction.
func NewWithLineItems(contractID, tenantPartyID, ownerID, propertyID, tenantID string, chargeType ChargeType, description, referenceMonth string, amount float64, dueDate time.Time, items []LineItem) (*Charge, error) {
	c, err := New(contractID, tenantPartyID, ownerID, propertyID, tenantID, chargeType, description, referenceMonth, amount, dueDate)
	if err != nil {
		return nil, err
	}
	if err := c.SetLineItems(items); err != nil {
		return nil, err
	}
	return c, nil
}

// SetLineItems normalizes server-owned identities/defaults and replaces the
// charge composition atomically. Callers must still enforce lifecycle rules
// (paid charges cannot be edited) before invoking it.
func (c *Charge) SetLineItems(items []LineItem) error {
	normalized := make([]LineItem, len(items))
	for i, item := range items {
		item.ID = deterministicLineItemID(c, i, item)
		if strings.TrimSpace(item.EconomicOwnerID) == "" {
			item.EconomicOwnerID = c.OwnerID
		}
		if item.SettlementMode == "" {
			item.SettlementMode = SettlementModeCollection
		}
		if strings.TrimSpace(item.RuleVersionID) == "" {
			item.RuleVersionID = deterministicRuleVersionID(c, item)
		}
		if item.AllocationType != "" && !IsValidAllocationType(item.AllocationType) {
			return fmt.Errorf("line_items[%d]: invalid allocation_type %q", i, item.AllocationType)
		}
		normalized[i] = item
	}
	if err := ValidateLineItems(normalized, c.Amount.Cents()); err != nil {
		return err
	}
	c.LineItems = normalized
	return nil
}

func deterministicLineItemID(c *Charge, index int, item LineItem) string {
	// Contract + competence are stable when a recurring charge is rebuilt in
	// memory; the aggregate UUID is intentionally not used because a repair
	// creates a fresh candidate before updating the existing row in place.
	key := fmt.Sprintf("toca/charge-item/v1|%s|%s|%s|%s|%d|%s|%s",
		strings.TrimSpace(c.TenantID), strings.TrimSpace(c.ContractID), strings.TrimSpace(c.ReferenceMonth), c.Type,
		index, strings.TrimSpace(item.SourceType), strings.TrimSpace(item.SourceID))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

func deterministicRuleVersionID(c *Charge, item LineItem) string {
	key := fmt.Sprintf("toca/charge-item-rule/v1|%s|%s|%s|%s|%s", strings.TrimSpace(c.TenantID), strings.TrimSpace(c.ContractID), strings.TrimSpace(c.ReferenceMonth), strings.TrimSpace(item.SourceType), strings.TrimSpace(item.SourceID))
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(key)).String()
}

// ValidateLineItems checks that the breakdown matches the parent amount.
// A 1-cent residual is tolerated to accommodate rounding when bundled
// percentages are involved; anything larger is rejected so we don't
// silently bill the customer a different value than the items display.
func ValidateLineItems(items []LineItem, totalCents int64) error {
	if len(items) == 0 {
		return nil
	}
	var sum int64
	for i, it := range items {
		if strings.TrimSpace(it.ID) == "" {
			return fmt.Errorf("line_items[%d]: id is required", i)
		}
		if it.AmountCents <= 0 {
			return fmt.Errorf("line_items[%d]: amount_cents must be > 0", i)
		}
		if it.Description == "" {
			return fmt.Errorf("line_items[%d]: description is required", i)
		}
		if !IsValidSettlementMode(it.SettlementMode) {
			return fmt.Errorf("line_items[%d]: invalid settlement_mode %q", i, it.SettlementMode)
		}
		if strings.TrimSpace(it.EconomicOwnerID) == "" {
			return fmt.Errorf("line_items[%d]: economic_owner_id is required", i)
		}
		if strings.TrimSpace(it.RuleVersionID) == "" {
			return fmt.Errorf("line_items[%d]: rule_version_id is required", i)
		}
		if it.AllocationType != "" && !IsValidAllocationType(it.AllocationType) {
			return fmt.Errorf("line_items[%d]: invalid allocation_type %q", i, it.AllocationType)
		}
		sum += it.AmountCents
	}
	delta := sum - totalCents
	if delta < 0 {
		delta = -delta
	}
	if delta > 1 {
		return fmt.Errorf("line_items sum (%d) does not match charge amount (%d cents)", sum, totalCents)
	}
	return nil
}

// NewAllocation builds one destination slice of a payment. ID is generated
// here; the caller supplies the linkage (payment, charge item, beneficiary)
// and the amount. A non-positive amount is rejected.
func NewAllocation(paymentID, chargeItemID, beneficiaryPartyID string, allocationType AllocationType, amountCents int64, ruleVersionID string) (*Allocation, error) {
	return NewAllocationWithPurpose(paymentID, chargeItemID, beneficiaryPartyID, allocationType, amountCents, ruleVersionID, "", "")
}

// NewAllocationWithPurpose creates a destination slice with an explicit
// business purpose. Purpose and related entity participate in the stable ID so
// several revenue slices on the same rent item never collide.
func NewAllocationWithPurpose(paymentID, chargeItemID, beneficiaryPartyID string, allocationType AllocationType, amountCents int64, ruleVersionID string, purpose AllocationPurpose, relatedEntityID string) (*Allocation, error) {
	if strings.TrimSpace(paymentID) == "" {
		return nil, errors.New("allocation payment_id is required")
	}
	if strings.TrimSpace(chargeItemID) == "" {
		return nil, errors.New("allocation charge_item_id is required")
	}
	if strings.TrimSpace(beneficiaryPartyID) == "" {
		return nil, errors.New("allocation beneficiary_party_id is required")
	}
	if !IsValidAllocationType(allocationType) {
		return nil, fmt.Errorf("invalid allocation_type %q", allocationType)
	}
	if amountCents <= 0 {
		return nil, errors.New("allocation amount_cents must be > 0")
	}
	if strings.TrimSpace(ruleVersionID) == "" {
		return nil, errors.New("allocation rule_version_id is required")
	}
	if !IsValidAllocationPurpose(purpose) {
		return nil, fmt.Errorf("invalid allocation purpose %q", purpose)
	}
	relatedEntityID = strings.TrimSpace(relatedEntityID)
	if (purpose == AllocationPurposeRentalIntermediationBroker || purpose == AllocationPurposeSaleCommissionBroker) && relatedEntityID == "" {
		return nil, errors.New("broker intermediation allocation requires related_entity_id")
	}
	if purpose != AllocationPurposeRentalIntermediationBroker && purpose != AllocationPurposeSaleCommissionBroker && relatedEntityID != "" {
		return nil, errors.New("related_entity_id is only valid for broker intermediation")
	}
	allocationID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(strings.Join([]string{
		"toca/payment-allocation/v2", strings.TrimSpace(paymentID), strings.TrimSpace(chargeItemID),
		string(allocationType), strings.TrimSpace(beneficiaryPartyID), strings.TrimSpace(ruleVersionID),
		string(purpose), relatedEntityID,
	}, "|"))).String()
	return &Allocation{
		ID:                 allocationID,
		PaymentID:          paymentID,
		ChargeItemID:       chargeItemID,
		AmountCents:        amountCents,
		AllocationType:     allocationType,
		BeneficiaryPartyID: beneficiaryPartyID,
		RuleVersionID:      strings.TrimSpace(ruleVersionID),
		Purpose:            purpose,
		RelatedEntityID:    relatedEntityID,
	}, nil
}

// ValidateAllocations checks that the slices of a payment are individually
// valid and that their sum matches the payment amount. A payment without
// allocations is valid (legacy); a populated set must reconcile exactly.
func ValidateAllocations(allocations []Allocation, paymentCents int64) error {
	if len(allocations) == 0 {
		return nil
	}
	var sum int64
	for i, a := range allocations {
		if strings.TrimSpace(a.ID) == "" {
			return fmt.Errorf("allocations[%d]: id is required", i)
		}
		if a.AmountCents <= 0 {
			return fmt.Errorf("allocations[%d]: amount_cents must be > 0", i)
		}
		if strings.TrimSpace(a.PaymentID) == "" {
			return fmt.Errorf("allocations[%d]: payment_id is required", i)
		}
		if strings.TrimSpace(a.ChargeItemID) == "" {
			return fmt.Errorf("allocations[%d]: charge_item_id is required", i)
		}
		if strings.TrimSpace(a.BeneficiaryPartyID) == "" {
			return fmt.Errorf("allocations[%d]: beneficiary_party_id is required", i)
		}
		if !IsValidAllocationType(a.AllocationType) {
			return fmt.Errorf("allocations[%d]: invalid allocation_type %q", i, a.AllocationType)
		}
		if strings.TrimSpace(a.RuleVersionID) == "" {
			return fmt.Errorf("allocations[%d]: rule_version_id is required", i)
		}
		if !IsValidAllocationPurpose(a.Purpose) {
			return fmt.Errorf("allocations[%d]: invalid purpose %q", i, a.Purpose)
		}
		if (a.Purpose == AllocationPurposeRentalIntermediationBroker || a.Purpose == AllocationPurposeSaleCommissionBroker) && strings.TrimSpace(a.RelatedEntityID) == "" {
			return fmt.Errorf("allocations[%d]: broker intermediation requires related_entity_id", i)
		}
		if a.Purpose != AllocationPurposeRentalIntermediationBroker && a.Purpose != AllocationPurposeSaleCommissionBroker && strings.TrimSpace(a.RelatedEntityID) != "" {
			return fmt.Errorf("allocations[%d]: related_entity_id is not valid for purpose %q", i, a.Purpose)
		}
		sum += a.AmountCents
	}
	if sum != paymentCents {
		return fmt.Errorf("allocations sum (%d) does not match payment amount (%d cents)", sum, paymentCents)
	}
	return nil
}

func (c *Charge) CalculateTotal() {
	// Total = Amount + LateFee + Interest - Discount
	sub, _ := c.Amount.Add(c.LateFee).Add(c.Interest).Subtract(c.Discount)
	c.TotalAmount = sub
}

// CalculateLateFees recalcula multa e juros de mora da cobrança a partir do
// vencimento. É idempotente por natureza: sempre recalcula do zero sobre
// `Amount` (nunca acumula sobre `TotalAmount`), então pode rodar a cada tick.
//
// Regras (ver docs/features/financeiro/inadimplencia-diagnostico-e-plano.md §7.9):
//   - a multa é devida a partir do PRIMEIRO dia de atraso (antes, um atraso de
//     menos de 24h saía sem multa e, como o motor não relistava a cobrança já
//     marcada OVERDUE, o encargo congelava em zero para sempre);
//   - os juros são pro rata die, com fração de dia (a cobrança de hoje à tarde
//     já conta as horas decorridas desde o vencimento).
func (c *Charge) CalculateLateFees(lateFeePercent, monthlyInterestPercent float64) {
	if !IsOutstandingStatus(c.Status) {
		return
	}
	overdueDays := time.Since(c.DueDate).Hours() / 24
	if overdueDays <= 0 {
		// Ainda não venceu: nada a aplicar (e o total volta ao valor original,
		// caso uma cobrança tenha sido antecipada no cadastro).
		return
	}

	c.LateFee = c.Amount.Percentage(lateFeePercent)

	dailyInterestRate := (monthlyInterestPercent / 100) / 30
	c.Interest = valueobjects.NewMoneyFromFloat(c.Amount.Float() * dailyInterestRate * overdueDays)

	c.CalculateTotal()
}

func (c *Charge) Pay(amount float64, method string) error {
	_, err := c.ApplyPayment(uuid.NewString(), amount, method, time.Now())
	return err
}

// ApplyPayment appends one uniquely identified payment and returns whether
// state changed. A retry with the same ID and identical payload is a no-op;
// reusing the ID with different data is rejected. All comparisons are made
// in cents so rounding cannot turn a partial payment into an overpayment.
func (c *Charge) ApplyPayment(paymentID string, amount float64, method string, paidAt time.Time) (bool, error) {
	return c.ApplyPaymentWithAllocations(paymentID, amount, method, paidAt, "", nil)
}

// ApplyPaymentWithAllocations appends a payment together with the explicit,
// cent-conserving appropriation that produced its economic destinations.
// Empty allocations remain supported for legacy charges without typed items.
func (c *Charge) ApplyPaymentWithAllocations(paymentID string, amount float64, method string, paidAt time.Time, allocationMethod AllocationMethod, allocations []Allocation) (bool, error) {
	paymentID = strings.TrimSpace(paymentID)
	method = strings.TrimSpace(method)
	if paymentID == "" {
		return false, errors.New("payment id is required")
	}
	if method == "" {
		return false, errors.New("payment method is required")
	}
	if paidAt.IsZero() {
		return false, errors.New("payment date is required")
	}

	moneyAmount := valueobjects.NewMoneyFromFloat(amount)
	if moneyAmount.Cents() <= 0 {
		return false, errors.New("payment amount must be greater than zero")
	}

	for _, entry := range c.Payments {
		if entry.ID != paymentID {
			continue
		}
		// PaidAt is deliberately not compared: automatic payment endpoints
		// stamp server time, which naturally differs on an HTTP retry. The
		// first accepted timestamp remains authoritative.
		if entry.AmountCents != moneyAmount.Cents() || entry.Method != method ||
			entry.AllocationMethod != allocationMethod || !sameAllocations(entry.Allocations, allocations) {
			return false, errors.New("payment id was already used with different data")
		}
		return false, nil
	}

	if c.Status == StatusPaid {
		return false, errors.New("charge is already paid")
	}
	if c.Status == StatusCancelled {
		return false, errors.New("cannot pay a cancelled charge")
	}
	if c.Status == StatusReversed {
		return false, errors.New("cannot pay a reversed charge")
	}

	paidCents := int64(0)
	if c.PaidAmount != nil {
		paidCents = c.PaidAmount.Cents()
	}
	remainingCents := c.TotalAmount.Cents() - paidCents
	if remainingCents <= 0 {
		return false, errors.New("charge has no outstanding balance")
	}
	if moneyAmount.Cents() > remainingCents {
		return false, fmt.Errorf("payment exceeds outstanding balance by %d cents", moneyAmount.Cents()-remainingCents)
	}
	if len(allocations) > 0 {
		if !IsValidAllocationMethod(allocationMethod) {
			return false, fmt.Errorf("invalid allocation_method %q", allocationMethod)
		}
		if err := ValidateAllocations(allocations, moneyAmount.Cents()); err != nil {
			return false, err
		}
		if err := c.validateAllocationCoverage(paymentID, allocations); err != nil {
			return false, err
		}
	} else if allocationMethod != "" {
		return false, errors.New("allocation_method requires allocations")
	}

	c.Payments = append(c.Payments, PaymentEntry{
		ID:               paymentID,
		AmountCents:      moneyAmount.Cents(),
		Method:           method,
		PaidAt:           paidAt,
		AllocationMethod: allocationMethod,
		Allocations:      append([]Allocation(nil), allocations...),
	})
	accumulated := valueobjects.NewMoney(paidCents + moneyAmount.Cents())
	c.PaidAmount = &accumulated
	c.PaidAt = &paidAt
	c.PaymentMethod = method
	c.UpdatedAt = time.Now()
	if accumulated.Cents() == c.TotalAmount.Cents() {
		c.Status = StatusPaid
	} else {
		c.Status = StatusPartiallyPaid
	}
	return true, nil
}

func sameAllocations(left, right []Allocation) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (c *Charge) validateAllocationCoverage(paymentID string, allocations []Allocation) error {
	capacity := make(map[string]int64, len(c.LineItems))
	for _, item := range c.LineItems {
		capacity[item.ID] = item.AmountCents
	}
	used := make(map[string]int64, len(capacity))
	for _, payment := range c.Payments {
		for _, allocation := range payment.Allocations {
			used[allocation.ChargeItemID] += allocation.AmountCents
		}
	}
	for i, allocation := range allocations {
		if allocation.PaymentID != paymentID {
			return fmt.Errorf("allocations[%d]: payment_id does not match payment", i)
		}
		limit, ok := capacity[allocation.ChargeItemID]
		if !ok {
			return fmt.Errorf("allocations[%d]: unknown charge_item_id %q", i, allocation.ChargeItemID)
		}
		used[allocation.ChargeItemID] += allocation.AmountCents
		if used[allocation.ChargeItemID] > limit {
			return fmt.Errorf("allocations[%d]: charge item %s exceeds %d cents", i, allocation.ChargeItemID, limit)
		}
	}
	return nil
}

func (c *Charge) Cancel() error {
	if c.Status == StatusCancelled {
		return nil
	}
	if c.Status != StatusPending && c.Status != StatusOverdue {
		return fmt.Errorf("%w: only PENDING or OVERDUE charges can be cancelled (current: %s)", ErrInvalidState, c.Status)
	}
	c.Status = StatusCancelled
	c.UpdatedAt = time.Now()
	return nil
}

// Reverse transita de PAID para REVERSED, permitindo a reversão contábil
// de uma charge já paga. Idempotente se já estiver REVERSED.
func (c *Charge) Reverse(reason string) error {
	if c.Status == StatusReversed {
		return nil
	}
	if c.Status != StatusPaid {
		return errors.New("only PAID charges can be reversed")
	}
	c.Status = StatusReversed
	c.UpdatedAt = time.Now()
	return nil
}

func (c *Charge) MarkOverdue() error {
	if c.Status == StatusOverdue {
		return nil
	}
	if c.Status == StatusPartiallyPaid {
		// Preserve the partial-payment state: overdue reporting derives lateness
		// from DueDate, while PARTIALLY_PAID is required to calculate only the
		// remaining receivable instead of the original gross amount.
		return nil
	}
	if c.Status != StatusPending {
		return fmt.Errorf("%w: only PENDING charges can be marked overdue (current: %s)", ErrInvalidState, c.Status)
	}
	c.Status = StatusOverdue
	c.UpdatedAt = time.Now()
	return nil
}

// RevertOverdue devolve a cobrança para PENDING e zera os encargos.
//
// Existe para o caso em que a marcação de atraso foi feita pela regra ANTIGA (que
// considerava vencida a cobrança do próprio dia de vencimento): quando o motor
// encontra uma cobrança OVERDUE que a regra atual diz que ainda não venceu, ele
// desfaz a marcação e o encargo indevido. Sem isso, a correção da regra não
// alcançaria o que já estava gravado — o caso real de produção (uma conta que
// vencia no dia e aparecia como vencida, com multa aplicada).
//
// Só de OVERDUE para PENDING: cobrança paga, cancelada, estornada ou parcial não
// é tocada aqui.
func (c *Charge) RevertOverdue() error {
	if c == nil {
		return fmt.Errorf("%w: charge is nil", ErrInvalidState)
	}
	if c.Status != StatusOverdue {
		return fmt.Errorf("%w: only OVERDUE charges can be reverted (current: %s)", ErrInvalidState, c.Status)
	}
	c.Status = StatusPending
	c.LateFee = valueobjects.NewMoneyFromFloat(0)
	c.Interest = valueobjects.NewMoneyFromFloat(0)
	c.CalculateTotal()
	c.UpdatedAt = time.Now()
	return nil
}

// OverdueDays conta os dias de atraso em DIAS CIVIS no fuso informado.
//
// A regra é de calendário, não de relógio: a cobrança vence NO DIA do vencimento
// e só passa a contar atraso no dia seguinte. Antes o cálculo era
// `horas/24` a partir do instante do vencimento, então uma cobrança que vence
// hoje às 00:00 já aparecia como vencida durante o próprio dia do vencimento (e,
// como a data é gravada em UTC, às 21h da véspera no horário de Brasília) — foi o
// que o dono do produto viu em produção: "uma conta que vence hoje já aparece como
// vencida".
//
// A data de vencimento é comparada pelo seu DIA em UTC (é assim que ela é
// gravada: `2026-10-10T00:00:00Z` significa "vence em 10/10", e a tela mostra
// 10/10), enquanto "hoje" vem do fuso do produto/tenant.
func (c *Charge) OverdueDays(now time.Time, loc *time.Location) int {
	if c == nil || !IsOutstandingStatus(c.Status) {
		return 0
	}
	if loc == nil {
		loc = time.UTC
	}
	vencimento := c.DueDate.UTC()
	venc := time.Date(vencimento.Year(), vencimento.Month(), vencimento.Day(), 0, 0, 0, 0, time.UTC)
	hojeLocal := now.In(loc)
	hoje := time.Date(hojeLocal.Year(), hojeLocal.Month(), hojeLocal.Day(), 0, 0, 0, 0, time.UTC)
	dias := int(hoje.Sub(venc).Hours() / 24)
	if dias < 0 {
		return 0
	}
	return dias
}

// DaysOverdue devolve os dias de atraso no fuso padrão do produto.
func (c *Charge) DaysOverdue() int {
	return c.OverdueDays(time.Now(), timeutil.Location())
}

// IsOverdue reporta se a cobrança já está atrasada (um dia ou mais após o
// vencimento). No dia do vencimento ela NÃO está atrasada.
func (c *Charge) IsOverdue() bool {
	return c.DaysOverdue() >= 1
}
