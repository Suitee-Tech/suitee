// Package partner_royalty models the franchise royalty owed to a franchisor
// when a deal closes on a property captured under an active franchise
// partnership. It mirrors the small commission domain: a flat struct, a
// constructor, and a status lifecycle.
package partner_royalty

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusPending   Status = "PENDING"
	StatusApproved  Status = "APPROVED"
	StatusPaid      Status = "PAID"
	StatusCancelled Status = "CANCELLED"
)

// PartnerRoyalty is one royalty payable. TenantID is the franchisor (payee) —
// the tenant that owns the row and against which it is listed. PayerTenant is
// the franchisee that closed the deal.
type PartnerRoyalty struct {
	Kind          string     `json:"kind,omitempty"`
	ID            string     `json:"id" bson:"_id"`
	TenantID      string     `json:"tenant_id" bson:"tenant_id"`
	PartnershipID string     `json:"partnership_id" bson:"partnership_id"`
	DealID        string     `json:"deal_id" bson:"deal_id"`
	ContractID    string     `json:"contract_id" bson:"contract_id"`
	PayerTenant   string     `json:"payer_tenant" bson:"payer_tenant"`
	PayeeTenant   string     `json:"payee_tenant" bson:"payee_tenant"`
	BaseAmount    float64    `json:"base_amount" bson:"base_amount"`
	Percentage    float64    `json:"percentage" bson:"percentage"`
	Amount        float64    `json:"amount" bson:"amount"`
	Currency      string     `json:"currency" bson:"currency"`
	Status        Status     `json:"status" bson:"status"`
	SourceEventID string     `json:"source_event_id,omitempty"`
	PayoutOrderID string     `json:"payout_order_id,omitempty"`
	CreatedAt     time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at" bson:"updated_at"`
	PaidAt        *time.Time `json:"paid_at,omitempty" bson:"paid_at,omitempty"`
}

func (pr *PartnerRoyalty) Approve() error {
	if pr.Status == StatusApproved {
		return nil
	}
	if pr.Status != StatusPending {
		return errors.New("only PENDING partner royalties can be approved")
	}
	pr.Status = StatusApproved
	pr.UpdatedAt = time.Now()
	return nil
}

func (pr *PartnerRoyalty) MarkPaid() error {
	if pr.Status == StatusPaid {
		return nil
	}
	if pr.Status != StatusPending && pr.Status != StatusApproved {
		return errors.New("only PENDING/APPROVED partner royalties can be marked paid")
	}
	now := time.Now()
	pr.Status = StatusPaid
	pr.PaidAt = &now
	pr.UpdatedAt = now
	return nil
}

// Reopen devolve um royalty CANCELADO a PENDING, espelho de Cancel: é o que
// permite relançar a obrigação quando a ordem de pagamento que o cancelou é
// regerada. Royalty pago não reabre (estorno é o caminho).
func (pr *PartnerRoyalty) Reopen() error {
	if pr.Status == StatusPending {
		return nil
	}
	if pr.Status != StatusCancelled {
		return errors.New("only CANCELLED partner royalties can be reopened")
	}
	pr.Status = StatusPending
	pr.UpdatedAt = time.Now()
	return nil
}

func (pr *PartnerRoyalty) Cancel() error {
	if pr.Status == StatusCancelled {
		return nil
	}
	if pr.Status == StatusPaid {
		return errors.New("cannot cancel a PAID partner royalty")
	}
	pr.Status = StatusCancelled
	pr.UpdatedAt = time.Now()
	return nil
}

// New builds a PENDING royalty. payeeTenant (the franchisor) is also stored as
// TenantID so the row is listed under, and scoped to, the franchisor.
func New(partnershipID, dealID, contractID, payerTenant, payeeTenant string, baseAmount, percentage float64) *PartnerRoyalty {
	now := time.Now()
	return &PartnerRoyalty{
		ID:            uuid.New().String(),
		TenantID:      payeeTenant,
		PartnershipID: partnershipID,
		DealID:        dealID,
		ContractID:    contractID,
		PayerTenant:   payerTenant,
		PayeeTenant:   payeeTenant,
		BaseAmount:    baseAmount,
		Percentage:    percentage,
		Amount:        baseAmount * (percentage / 100),
		Currency:      "BRL",
		Status:        StatusPending,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
