package managed_bill

import (
	"github.com/oginoino/toca/pkg/timeutil"

	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Status is the deliberately small first vertical slice of the managed-bill
// lifecycle. Earlier scheduling/receipt states can be added without weakening
// the financial transitions below: this aggregate starts only after a paid
// charge allocation has made the reserve real.
type Status string

const (
	StatusReserved   Status = "RESERVED"
	StatusApproved   Status = "APPROVED"
	StatusPaid       Status = "PAID"
	StatusReconciled Status = "RECONCILED"
	StatusCancelled  Status = "CANCELLED"
)

var (
	ErrInvalidInput           = errors.New("invalid managed bill input")
	ErrInvalidTransition      = errors.New("invalid managed bill status transition")
	ErrAlreadyExists          = errors.New("managed bill already exists")
	ErrNotFound               = errors.New("managed bill not found")
	ErrConcurrentModification = errors.New("managed bill was modified concurrently")
)

// Evidence snapshots a proof/reference attached to one lifecycle action.
// References are intentionally opaque: the file/object itself belongs to the
// document store while this aggregate preserves who attached it and when.
type Evidence struct {
	Kind      string    `json:"kind"`
	Reference string    `json:"reference"`
	Actor     string    `json:"actor,omitempty"`
	At        time.Time `json:"at"`
}

// HistoryEntry is append-only. Version records the aggregate revision produced
// by the action and is used together with Repository.UpdateIfVersion.
type HistoryEntry struct {
	Action    string    `json:"action"`
	Actor     string    `json:"actor"`
	At        time.Time `json:"at"`
	Version   int64     `json:"version"`
	Reference string    `json:"reference,omitempty"`
	Reason    string    `json:"reason,omitempty"`
}

// PaymentAttempt preserves one completed payment cycle after it is reversed.
// The top-level payment fields on ManagedBill always describe the active
// attempt; moving them into this append-only snapshot lets the obligation
// return to APPROVED and be paid again without rewriting the prior audit trail.
type PaymentAttempt struct {
	ID                      string     `json:"id"`
	PaymentReference        string     `json:"payment_reference"`
	PaymentJournalSourceID  string     `json:"payment_journal_source_id"`
	PaidBy                  string     `json:"paid_by"`
	PaidAt                  time.Time  `json:"paid_at"`
	ReconciledBy            string     `json:"reconciled_by,omitempty"`
	ReconciledAt            *time.Time `json:"reconciled_at,omitempty"`
	ReconciliationReference string     `json:"reconciliation_reference,omitempty"`
	ReversalReference       string     `json:"reversal_reference"`
	ReversalJournalSourceID string     `json:"reversal_journal_source_id"`
	ReversalProofReference  string     `json:"reversal_proof_reference,omitempty"`
	ReversedBy              string     `json:"reversed_by"`
	ReversedAt              time.Time  `json:"reversed_at"`
	ReversalReason          string     `json:"reversal_reason"`
}

// ManagedBill is one concrete creditor obligation, not the recurring
// UtilityAccount configuration. All money is represented as integer cents.
type ManagedBill struct {
	ID                 string    `json:"id"`
	TenantID           string    `json:"tenant_id"`
	SourceKey          string    `json:"source_key"`
	SourceTransferID   string    `json:"source_transfer_id"`
	SourceChargeID     string    `json:"source_charge_id"`
	SourceChargeItemID string    `json:"charge_item_id"`
	ContractID         string    `json:"contract_id"`
	PropertyID         string    `json:"property_id"`
	OwnerID            string    `json:"owner_id"`
	Component          string    `json:"component"`
	Creditor           string    `json:"creditor_name"`
	Description        string    `json:"description"`
	AmountCents        int64     `json:"amount_cents"`
	DueDate            time.Time `json:"due_date"`
	CompetenceMonth    string    `json:"competence_month"`
	Status             Status    `json:"status"`
	Version            int64     `json:"version"`

	ReservedBy   string     `json:"reserved_by"`
	ReservedAt   time.Time  `json:"reserved_at"`
	ApprovedBy   string     `json:"approved_by,omitempty"`
	ApprovedAt   *time.Time `json:"approved_at,omitempty"`
	PaidBy       string     `json:"paid_by,omitempty"`
	PaidAt       *time.Time `json:"paid_at,omitempty"`
	ReconciledBy string     `json:"reconciled_by,omitempty"`
	ReconciledAt *time.Time `json:"reconciled_at,omitempty"`
	CancelledBy  string     `json:"cancelled_by,omitempty"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`

	PaymentReference        string           `json:"payment_reference,omitempty"`
	PaymentJournalSourceID  string           `json:"payment_journal_source_id,omitempty"`
	ReconciliationReference string           `json:"reconciliation_reference,omitempty"`
	CancellationReason      string           `json:"cancellation_reason,omitempty"`
	Evidence                []Evidence       `json:"evidence,omitempty"`
	History                 []HistoryEntry   `json:"history"`
	PaymentAttempts         []PaymentAttempt `json:"payment_attempts,omitempty"`
	CreatedAt               time.Time        `json:"created_at"`
	UpdatedAt               time.Time        `json:"updated_at"`
}

type NewReservedInput struct {
	TenantID           string
	SourceKey          string
	SourceTransferID   string
	SourceChargeID     string
	SourceChargeItemID string
	ContractID         string
	PropertyID         string
	OwnerID            string
	Component          string
	Creditor           string
	Description        string
	AmountCents        int64
	DueDate            time.Time
	CompetenceMonth    string
	ReservedBy         string
	ReservedAt         time.Time
	Evidence           []Evidence
}

func NewReserved(in NewReservedInput) (*ManagedBill, error) {
	if strings.TrimSpace(in.TenantID) == "" || strings.TrimSpace(in.SourceKey) == "" ||
		strings.TrimSpace(in.SourceTransferID) == "" || strings.TrimSpace(in.SourceChargeID) == "" ||
		strings.TrimSpace(in.SourceChargeItemID) == "" || strings.TrimSpace(in.ContractID) == "" ||
		strings.TrimSpace(in.PropertyID) == "" || strings.TrimSpace(in.OwnerID) == "" {
		return nil, fmt.Errorf("%w: tenant, source and economic identity are required", ErrInvalidInput)
	}
	if in.AmountCents <= 0 {
		return nil, fmt.Errorf("%w: amount_cents must be positive", ErrInvalidInput)
	}
	if in.DueDate.IsZero() {
		return nil, fmt.Errorf("%w: due_date is required", ErrInvalidInput)
	}
	if !validCompetenceMonth(in.CompetenceMonth) {
		return nil, fmt.Errorf("%w: competence_month must use YYYY-MM", ErrInvalidInput)
	}
	if strings.TrimSpace(in.Component) == "" || strings.TrimSpace(in.Creditor) == "" || strings.TrimSpace(in.Description) == "" {
		return nil, fmt.Errorf("%w: component, creditor and description are required", ErrInvalidInput)
	}
	if strings.TrimSpace(in.ReservedBy) == "" {
		return nil, fmt.Errorf("%w: reserved_by is required", ErrInvalidInput)
	}
	now := normalizedTime(in.ReservedAt)
	b := &ManagedBill{
		ID: uuid.NewString(), TenantID: strings.TrimSpace(in.TenantID), SourceKey: strings.TrimSpace(in.SourceKey),
		SourceTransferID: strings.TrimSpace(in.SourceTransferID), SourceChargeID: strings.TrimSpace(in.SourceChargeID),
		SourceChargeItemID: strings.TrimSpace(in.SourceChargeItemID), ContractID: strings.TrimSpace(in.ContractID),
		PropertyID: strings.TrimSpace(in.PropertyID), OwnerID: strings.TrimSpace(in.OwnerID),
		Component: strings.TrimSpace(in.Component), Creditor: strings.TrimSpace(in.Creditor),
		Description: strings.TrimSpace(in.Description), AmountCents: in.AmountCents,
		DueDate: civilDate(in.DueDate), CompetenceMonth: strings.TrimSpace(in.CompetenceMonth),
		Status: StatusReserved, Version: 1, ReservedBy: strings.TrimSpace(in.ReservedBy), ReservedAt: now,
		Evidence: append([]Evidence(nil), in.Evidence...), CreatedAt: now, UpdatedAt: now,
	}
	b.History = []HistoryEntry{{Action: "reserved", Actor: b.ReservedBy, At: now, Version: b.Version}}
	return b, nil
}

func (b *ManagedBill) Approve(actor string, at time.Time, evidence ...Evidence) error {
	if b.Status != StatusReserved {
		return fmt.Errorf("%w: only RESERVED bills can be approved", ErrInvalidTransition)
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		return fmt.Errorf("%w: approval actor is required", ErrInvalidInput)
	}
	at = normalizedTime(at)
	if at.Before(b.ReservedAt) {
		return fmt.Errorf("%w: approved_at cannot precede reserved_at", ErrInvalidInput)
	}
	b.Status, b.ApprovedBy, b.ApprovedAt = StatusApproved, actor, &at
	b.appendMutation("approved", actor, at, "", "", evidence)
	return nil
}

func (b *ManagedBill) MarkPaid(actor, paymentReference, journalSourceID string, at time.Time, evidence ...Evidence) error {
	if b.Status != StatusApproved {
		return fmt.Errorf("%w: only APPROVED bills can be paid", ErrInvalidTransition)
	}
	actor, paymentReference, journalSourceID = strings.TrimSpace(actor), strings.TrimSpace(paymentReference), strings.TrimSpace(journalSourceID)
	if actor == "" || paymentReference == "" || journalSourceID == "" {
		return fmt.Errorf("%w: payment actor, payment_reference and journal source are required", ErrInvalidInput)
	}
	for _, attempt := range b.PaymentAttempts {
		if strings.TrimSpace(attempt.PaymentReference) == paymentReference {
			return fmt.Errorf("%w: payment_reference was already used by a reversed attempt", ErrInvalidInput)
		}
		if strings.TrimSpace(attempt.PaymentJournalSourceID) == journalSourceID || strings.TrimSpace(attempt.ID) == journalSourceID {
			return fmt.Errorf("%w: payment journal source was already used by a reversed attempt", ErrInvalidInput)
		}
	}
	at = normalizedTime(at)
	if b.ApprovedAt == nil || at.Before(*b.ApprovedAt) {
		return fmt.Errorf("%w: paid_at cannot precede approved_at", ErrInvalidInput)
	}
	if !b.UpdatedAt.IsZero() && at.Before(b.UpdatedAt) {
		return fmt.Errorf("%w: paid_at cannot precede the latest bill mutation", ErrInvalidInput)
	}
	b.Status, b.PaidBy, b.PaidAt = StatusPaid, actor, &at
	b.PaymentReference, b.PaymentJournalSourceID = paymentReference, journalSourceID
	b.appendMutation("paid", actor, at, paymentReference, "", evidence)
	return nil
}

func (b *ManagedBill) Reconcile(actor, reference string, at time.Time, evidence ...Evidence) error {
	if b.Status != StatusPaid {
		return fmt.Errorf("%w: only PAID bills can be reconciled", ErrInvalidTransition)
	}
	actor, reference = strings.TrimSpace(actor), strings.TrimSpace(reference)
	if actor == "" || reference == "" {
		return fmt.Errorf("%w: reconciliation actor and reference are required", ErrInvalidInput)
	}
	at = normalizedTime(at)
	if b.PaidAt == nil || at.Before(*b.PaidAt) {
		return fmt.Errorf("%w: reconciled_at cannot precede paid_at", ErrInvalidInput)
	}
	b.Status, b.ReconciledBy, b.ReconciledAt = StatusReconciled, actor, &at
	b.ReconciliationReference = reference
	b.appendMutation("reconciled", actor, at, reference, "", evidence)
	return nil
}

// LastPaymentAttempt returns a copy of the most recently reversed payment
// attempt. Returning a value instead of a slice pointer keeps callers from
// mutating the aggregate's append-only history accidentally.
func (b *ManagedBill) LastPaymentAttempt() (PaymentAttempt, bool) {
	if b == nil || len(b.PaymentAttempts) == 0 {
		return PaymentAttempt{}, false
	}
	last := b.PaymentAttempts[len(b.PaymentAttempts)-1]
	if last.ReconciledAt != nil {
		reconciledAt := *last.ReconciledAt
		last.ReconciledAt = &reconciledAt
	}
	return last, true
}

// ReversePayment compensates the currently active PAID/RECONCILED attempt and
// reopens the same obligation as APPROVED. The caller is responsible for
// atomically reversing the exact journal entry and restoring the corresponding
// owner-balance reserve before persisting this aggregate revision.
//
// A retry after a successful reversal is a no-op only when it carries the same
// canonical reversal identity and audit payload. Divergent retries fail closed
// instead of attaching a second interpretation to the already-frozen attempt.
func (b *ManagedBill) ReversePayment(actor, reversalReference, reversalJournalSourceID, reason string, at time.Time, evidence ...Evidence) error {
	actor = strings.TrimSpace(actor)
	reversalReference = strings.TrimSpace(reversalReference)
	reversalJournalSourceID = strings.TrimSpace(reversalJournalSourceID)
	reason = strings.TrimSpace(reason)
	if actor == "" || reversalReference == "" || reversalJournalSourceID == "" || reason == "" {
		return fmt.Errorf("%w: reversal actor, reference, journal source and reason are required", ErrInvalidInput)
	}
	reversalProofReference, err := paymentReversalProofReference(evidence)
	if err != nil {
		return err
	}

	if b.Status == StatusApproved && b.PaidAt == nil && b.PaymentReference == "" && b.PaymentJournalSourceID == "" {
		last, ok := b.LastPaymentAttempt()
		if !ok {
			return fmt.Errorf("%w: only PAID or RECONCILED bills can have a payment reversed", ErrInvalidTransition)
		}
		retryTimeMatches := at.IsZero() || normalizedTime(at).Equal(last.ReversedAt)
		if last.ReversalReference == reversalReference &&
			last.ReversalJournalSourceID == reversalJournalSourceID &&
			last.ReversalProofReference == reversalProofReference &&
			last.ReversedBy == actor && last.ReversalReason == reason && retryTimeMatches {
			return nil
		}
		return ErrConcurrentModification
	}
	if b.Status != StatusPaid && b.Status != StatusReconciled {
		return fmt.Errorf("%w: only PAID or RECONCILED bills can have a payment reversed", ErrInvalidTransition)
	}

	if b.PaidAt == nil || strings.TrimSpace(b.PaidBy) == "" ||
		strings.TrimSpace(b.PaymentReference) == "" || strings.TrimSpace(b.PaymentJournalSourceID) == "" {
		return fmt.Errorf("%w: active payment snapshot is incomplete", ErrInvalidInput)
	}
	if b.Status == StatusReconciled && (b.ReconciledAt == nil || strings.TrimSpace(b.ReconciledBy) == "" || strings.TrimSpace(b.ReconciliationReference) == "") {
		return fmt.Errorf("%w: active reconciliation snapshot is incomplete", ErrInvalidInput)
	}

	at = normalizedTime(at)
	if at.Before(*b.PaidAt) {
		return fmt.Errorf("%w: reversed_at cannot precede paid_at", ErrInvalidInput)
	}
	if b.ReconciledAt != nil && at.Before(*b.ReconciledAt) {
		return fmt.Errorf("%w: reversed_at cannot precede reconciled_at", ErrInvalidInput)
	}

	attemptID := strings.TrimSpace(b.PaymentJournalSourceID)
	for _, existing := range b.PaymentAttempts {
		if existing.ID == attemptID {
			return ErrConcurrentModification
		}
		if strings.TrimSpace(existing.ReversalReference) == reversalReference {
			return fmt.Errorf("%w: reversal_reference was already used by a prior attempt", ErrInvalidInput)
		}
		if strings.TrimSpace(existing.ReversalJournalSourceID) == reversalJournalSourceID {
			return fmt.Errorf("%w: reversal journal source was already used by a prior attempt", ErrInvalidInput)
		}
	}

	attempt := PaymentAttempt{
		ID:                      attemptID,
		PaymentReference:        strings.TrimSpace(b.PaymentReference),
		PaymentJournalSourceID:  attemptID,
		PaidBy:                  strings.TrimSpace(b.PaidBy),
		PaidAt:                  b.PaidAt.UTC().Truncate(time.Microsecond),
		ReconciledBy:            strings.TrimSpace(b.ReconciledBy),
		ReconciliationReference: strings.TrimSpace(b.ReconciliationReference),
		ReversalReference:       reversalReference,
		ReversalJournalSourceID: reversalJournalSourceID,
		ReversalProofReference:  reversalProofReference,
		ReversedBy:              actor,
		ReversedAt:              at,
		ReversalReason:          reason,
	}
	if b.ReconciledAt != nil {
		reconciledAt := b.ReconciledAt.UTC().Truncate(time.Microsecond)
		attempt.ReconciledAt = &reconciledAt
	}
	b.PaymentAttempts = append(b.PaymentAttempts, attempt)

	b.Status = StatusApproved
	b.PaidBy, b.PaidAt = "", nil
	b.PaymentReference, b.PaymentJournalSourceID = "", ""
	b.ReconciledBy, b.ReconciledAt = "", nil
	b.ReconciliationReference = ""
	b.appendMutation("payment_reversed", actor, at, reversalReference, reason, evidence)
	return nil
}

func paymentReversalProofReference(evidence []Evidence) (string, error) {
	var reference string
	for _, item := range evidence {
		if strings.TrimSpace(item.Kind) != "payment_reversal_proof" {
			continue
		}
		candidate := strings.TrimSpace(item.Reference)
		if candidate == "" {
			return "", fmt.Errorf("%w: payment reversal proof reference cannot be blank", ErrInvalidInput)
		}
		if reference != "" && reference != candidate {
			return "", fmt.Errorf("%w: payment reversal proof is ambiguous", ErrInvalidInput)
		}
		reference = candidate
	}
	return reference, nil
}

func (b *ManagedBill) Cancel(actor, reason string, at time.Time, evidence ...Evidence) error {
	if b.Status != StatusReserved && b.Status != StatusApproved {
		return fmt.Errorf("%w: only RESERVED or APPROVED bills can be cancelled", ErrInvalidTransition)
	}
	actor, reason = strings.TrimSpace(actor), strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return fmt.Errorf("%w: cancellation actor and reason are required", ErrInvalidInput)
	}
	at = normalizedTime(at)
	if at.Before(b.ReservedAt) {
		return fmt.Errorf("%w: cancelled_at cannot precede reserved_at", ErrInvalidInput)
	}
	b.Status, b.CancelledBy, b.CancelledAt = StatusCancelled, actor, &at
	b.CancellationReason = reason
	b.appendMutation("cancelled", actor, at, "", reason, evidence)
	return nil
}

func (b *ManagedBill) appendMutation(action, actor string, at time.Time, reference, reason string, evidence []Evidence) {
	b.Version++
	b.UpdatedAt = at
	for i := range evidence {
		if evidence[i].At.IsZero() {
			evidence[i].At = at
		}
		if strings.TrimSpace(evidence[i].Actor) == "" {
			evidence[i].Actor = actor
		}
		b.Evidence = append(b.Evidence, evidence[i])
	}
	b.History = append(b.History, HistoryEntry{Action: action, Actor: actor, At: at, Version: b.Version, Reference: reference, Reason: reason})
}

func (b *ManagedBill) SameEconomicIdentity(other *ManagedBill) bool {
	return b != nil && other != nil && b.TenantID == other.TenantID && b.SourceKey == other.SourceKey &&
		b.SourceTransferID == other.SourceTransferID && b.SourceChargeID == other.SourceChargeID &&
		b.SourceChargeItemID == other.SourceChargeItemID && b.ContractID == other.ContractID &&
		b.PropertyID == other.PropertyID && b.OwnerID == other.OwnerID && b.Component == other.Component &&
		b.Creditor == other.Creditor && b.Description == other.Description && b.AmountCents == other.AmountCents &&
		b.CompetenceMonth == other.CompetenceMonth && civilDate(b.DueDate).Equal(civilDate(other.DueDate))
}

func normalizedTime(at time.Time) time.Time {
	if at.IsZero() {
		at = time.Now()
	}
	// PostgreSQL timestamptz stores microseconds. Truncating before both the
	// typed column and JSON history are serialized keeps their audit timestamps
	// byte-for-byte comparable in the database guards.
	return at.UTC().Truncate(time.Microsecond)
}

// civilDate normaliza para a meia-noite do DIA CIVIL no fuso configurado (ver
// o mesmo helper no repositório: `.UTC()` antes de ler os componentes deslocava
// o vencimento para o dia seguinte).
func civilDate(value time.Time) time.Time {
	return timeutil.StartOfDay(value)
}

func validCompetenceMonth(value string) bool {
	if len(strings.TrimSpace(value)) != 7 {
		return false
	}
	_, err := time.Parse("2006-01", strings.TrimSpace(value))
	return err == nil
}
