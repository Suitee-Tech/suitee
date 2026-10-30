// Package withdrawal models the real-estate agency withdrawing its balance
// ("saque da imobiliária"). With no payment gateway, an approved withdrawal
// is settled MANUALLY by an operator: the money is moved off-platform (PIX,
// TED) and the operator records a manual external reference plus an optional
// proof URL when marking it processed.
package withdrawal

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// MinAmountCents is the minimum withdrawal amount (R$ 3.50).
const MinAmountCents = 350

type Status string

const (
	StatusPending   Status = "PENDING"   // requested, awaiting approval
	StatusApproved  Status = "APPROVED"  // approved, awaiting manual settlement
	StatusComplete  Status = "COMPLETE"  // operator settled it off-platform
	StatusFailed    Status = "FAILED"    // settlement attempt failed (rare)
	StatusCancelled Status = "CANCELLED" // cancelled before settlement
)

// HistoryEntry records one state transition, for the audit trail.
type HistoryEntry struct {
	At     time.Time `json:"at"`
	Action string    `json:"action"`
	Actor  string    `json:"actor,omitempty"`
	Note   string    `json:"note,omitempty"`
}

// Withdrawal is a request to withdraw the agency's balance, settled manually.
type Withdrawal struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	AmountCents int64  `json:"amount_cents"`
	Description string `json:"description,omitempty"`
	// ExternalID is generated at request time and is the immutable idempotency
	// key. It must never be overwritten — the operator-supplied settlement
	// reference (PIX/TED end-to-end id) lives in SettlementRef instead.
	ExternalID       string `json:"external_id"`
	Status           Status `json:"status"`
	PlatformFeeCents int64  `json:"platform_fee_cents,omitempty"`
	// SettlementRef carries the operator-supplied off-platform transfer
	// reference (PIX/TED end-to-end id) recorded when the withdrawal is
	// settled manually. Distinct from ExternalID, which stays immutable.
	SettlementRef string `json:"settlement_ref,omitempty"`
	// ReceiptURL holds the operator-supplied proof of the off-platform
	// transfer (comprovante PIX/TED).
	ReceiptURL  string         `json:"receipt_url,omitempty"`
	RequestedBy string         `json:"requested_by,omitempty"`
	ApprovedBy  string         `json:"approved_by,omitempty"`
	ApprovedAt  *time.Time     `json:"approved_at,omitempty"`
	SettledAt   *time.Time     `json:"settled_at,omitempty"`
	LastError   string         `json:"last_error,omitempty"`
	History     []HistoryEntry `json:"history,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

// Sentinel errors so the service can branch on cause without string-matching.
var (
	ErrAmountTooLow            = errors.New("withdrawal amount must be at least R$ 3.50")
	ErrNotPending              = errors.New("withdrawal is not pending approval")
	ErrNotApproved             = errors.New("withdrawal is not approved for settlement")
	ErrNotCancelable           = errors.New("withdrawal can no longer be cancelled")
	ErrWithdrawalAlreadyExists = errors.New("withdrawal already exists")
	ErrInsufficientBalance     = errors.New("insufficient balance available to withdraw")
)

// New builds a PENDING withdrawal. Amount is in cents and must meet the
// minimum.
func New(tenantID string, amountCents int64, description, requestedBy string) (*Withdrawal, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if amountCents < MinAmountCents {
		return nil, ErrAmountTooLow
	}
	now := time.Now()
	w := &Withdrawal{
		ID:          uuid.New().String(),
		TenantID:    tenantID,
		AmountCents: amountCents,
		Description: description,
		ExternalID:  "toca-withdrawal-" + uuid.New().String(),
		Status:      StatusPending,
		RequestedBy: requestedBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	w.addHistory("requested", requestedBy, "")
	return w, nil
}

func (w *Withdrawal) addHistory(action, actor, note string) {
	now := time.Now()
	w.History = append(w.History, HistoryEntry{At: now, Action: action, Actor: actor, Note: note})
	w.UpdatedAt = now
}

// Settled reports whether the withdrawal reached a terminal state.
func (w *Withdrawal) Settled() bool {
	return w.Status == StatusComplete || w.Status == StatusFailed || w.Status == StatusCancelled
}

// Approve moves a PENDING withdrawal to APPROVED.
func (w *Withdrawal) Approve(actor string) error {
	if w.Status != StatusPending {
		return ErrNotPending
	}
	now := time.Now()
	w.Status = StatusApproved
	w.ApprovedBy = actor
	w.ApprovedAt = &now
	w.addHistory("approved", actor, "")
	return nil
}

// MarkSettledManually moves an APPROVED withdrawal to its terminal COMPLETE
// state after an operator has moved the money off-platform. externalRef (the
// PIX/TED reference) is stored in SettlementRef — it never touches the
// immutable ExternalID idempotency key. proofURL records the comprovante. Both
// are optional. The transition is only valid from APPROVED.
func (w *Withdrawal) MarkSettledManually(actor, externalRef, proofURL string) error {
	if w.Status != StatusApproved {
		return ErrNotApproved
	}
	now := time.Now()
	w.Status = StatusComplete
	if externalRef != "" {
		w.SettlementRef = externalRef
	}
	if proofURL != "" {
		w.ReceiptURL = proofURL
	}
	w.LastError = ""
	w.SettledAt = &now
	w.addHistory("settled_manually", actor, externalRef)
	return nil
}

// Cancel aborts a withdrawal that has not yet been settled.
func (w *Withdrawal) Cancel(actor string) error {
	if w.Status != StatusPending && w.Status != StatusApproved {
		return ErrNotCancelable
	}
	w.Status = StatusCancelled
	w.addHistory("cancelled", actor, "")
	return nil
}
