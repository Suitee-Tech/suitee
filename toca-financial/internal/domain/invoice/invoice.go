package invoice

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusDraft     Status = "DRAFT"
	StatusIssued    Status = "ISSUED"
	StatusPaid      Status = "PAID"
	StatusOverdue   Status = "OVERDUE"
	StatusCancelled Status = "CANCELLED"
)

type Type string

const (
	TypeRent  Type = "RENT"
	TypeSale  Type = "SALE"
	TypeFee   Type = "FEE"
	TypeOther Type = "OTHER"
)

var (
	ErrInvoiceAlreadyExists = errors.New("invoice already exists for this source_event_id")
	ErrInvoiceNotFound      = errors.New("invoice not found")
)

type Invoice struct {
	ID          string `json:"id" bson:"_id"`
	ReferenceID string `json:"reference_id" bson:"reference_id"` // DealID or ContractID
	// ChargeID allocates cash received through this billing artifact to the
	// canonical receivable. An invoice without a charge link may still be read
	// for legacy/reporting purposes, but cannot be paid.
	ChargeID      string     `json:"charge_id,omitempty" bson:"charge_id,omitempty"`
	PayerID       string     `json:"payer_id" bson:"payer_id"` // PartyID
	Amount        float64    `json:"amount" bson:"amount"`
	Currency      string     `json:"currency" bson:"currency"`
	DueDate       time.Time  `json:"due_date" bson:"due_date"`
	Status        Status     `json:"status" bson:"status"`
	Type          Type       `json:"type" bson:"type"`
	CreatedAt     time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" bson:"updated_at"`
	PaidAt        *time.Time `json:"paid_at,omitempty" bson:"paid_at,omitempty"`
	TenantID      string     `json:"tenant_id" bson:"tenant_id"`
	SourceEventID string     `json:"source_event_id,omitempty"`
}

func New(referenceID, payerID string, amount float64, dueDate time.Time, invType Type, tenantID string) (*Invoice, error) {
	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}
	if referenceID == "" {
		return nil, errors.New("reference_id is required")
	}
	if payerID == "" {
		return nil, errors.New("payer_id is required")
	}
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	now := time.Now()
	return &Invoice{
		ID:          uuid.New().String(),
		ReferenceID: referenceID,
		PayerID:     payerID,
		Amount:      amount,
		Currency:    "BRL",
		DueDate:     dueDate,
		Status:      StatusDraft,
		Type:        invType,
		CreatedAt:   now,
		UpdatedAt:   now,
		TenantID:    tenantID,
	}, nil
}

func (i *Invoice) Pay() error {
	return i.PayAt(time.Now())
}

// PayAt mirrors the canonical payment timestamp onto the invoice projection.
// The cash itself is recorded append-only on Charge by InvoiceService before
// this projection is advanced.
func (i *Invoice) PayAt(paidAt time.Time) error {
	if i.Status == StatusPaid {
		return errors.New("invoice is already paid")
	}
	if i.Status == StatusCancelled {
		return errors.New("cannot pay a cancelled invoice")
	}
	if paidAt.IsZero() {
		return errors.New("payment date is required")
	}
	i.Status = StatusPaid
	i.PaidAt = &paidAt
	i.UpdatedAt = paidAt
	return nil
}

func (i *Invoice) Cancel() error {
	if i.Status == StatusPaid {
		return errors.New("cannot cancel a paid invoice")
	}
	if i.Status == StatusCancelled {
		return errors.New("invoice is already cancelled")
	}
	i.Status = StatusCancelled
	i.UpdatedAt = time.Now()
	return nil
}

func (i *Invoice) MarkOverdue() error {
	if i.Status == StatusPaid || i.Status == StatusCancelled {
		return errors.New("cannot mark as overdue if paid or cancelled")
	}
	i.Status = StatusOverdue
	i.UpdatedAt = time.Now()
	return nil
}
