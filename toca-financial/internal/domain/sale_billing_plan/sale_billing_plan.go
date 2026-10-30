// Package sale_billing_plan stores the billing plan attached to a sale
// contract. Sibling of rental_charge_config but with a different cadence:
// instead of recurring monthly until the contract ends, a sale is either a
// single payment (SINGLE) or a fixed number of installments
// (INSTALLMENTS). Charges for every installment are pre-generated at
// contract.signed time so the operator sees the full payment calendar from
// day one and the partial unique index on source_event_id guards against
// duplicate emissions on broker retries.
package sale_billing_plan

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

type Mode string

const (
	ModeSingle       Mode = "SINGLE"
	ModeInstallments Mode = "INSTALLMENTS"
)

// SaleBillingPlan is the persisted aggregate. The total sale value is
// composed of up to five legs — three cash legs that materialize charges
// and two non-cash legs that are recorded for accounting/contractual
// completeness only (no charge is emitted):
//
//	Cash legs (generate charges):
//	  - DownPaymentCents     → "Sinal / Entrada" charge on FirstDueDate
//	  - InstallmentAmount × InstallmentCount → "Parcela i/N" charges
//	  - SaldoAmountCents     → "Saldo" charge on SaldoDueDate
//
//	Non-cash legs (informational, no charge):
//	  - DacaoAmountCents     → bens/imóvel entregue em dação em pagamento
//	  - TrocaAmountCents     → propriedade(s) trocada(s)
//
// Invariant: DownPayment + Installments*Count + Saldo + Dacao + Troca
// must equal TotalAmount (with a 1-cent-per-installment rounding
// tolerance). Mode SINGLE means InstallmentCount=1 and only one of the
// legs is actually populated; INSTALLMENTS is the general N-parcela case.
//
// RecurrenceIntervalDays is retained for API/JSON compatibility. The
// installment cadence is a calendar month (same day-of-month, clamped for
// short months) — see SaleBillingPlanService.buildCharges. The field is
// no longer used to advance due dates.
type SaleBillingPlan struct {
	ID                     string    `json:"id"`
	TenantID               string    `json:"tenant_id"`
	ContractID             string    `json:"contract_id"`
	DealID                 string    `json:"deal_id,omitempty"`
	SourceEventID          string    `json:"source_event_id,omitempty"` // FIN-COM-11: idempotência via source_event_id
	BuyerPartyID           string    `json:"buyer_party_id,omitempty"`
	OwnerID                string    `json:"owner_id,omitempty"`
	PropertyID             string    `json:"property_id,omitempty"`
	Mode                   Mode      `json:"mode"`
	TotalAmountCents       int64     `json:"total_amount_cents"`
	DownPaymentCents       int64     `json:"down_payment_cents"`
	InstallmentCount       int       `json:"installment_count"`
	InstallmentAmountCents int64     `json:"installment_amount_cents"`
	FirstDueDate           time.Time `json:"first_due_date"`
	RecurrenceIntervalDays int       `json:"recurrence_interval_days"`
	// Non-cash legs — no charge is emitted, only recorded for
	// accounting/contractual completeness. The asset transfer is handled
	// off-platform; the description is what gets exhibited to the
	// operator and printed on the demonstrativo.
	DacaoAmountCents int64  `json:"dacao_amount_cents,omitempty"`
	DacaoDescription string `json:"dacao_description,omitempty"`
	TrocaAmountCents int64  `json:"troca_amount_cents,omitempty"`
	TrocaDescription string `json:"troca_description,omitempty"`
	// Saldo (deferred balance) — a single charge whose due date is
	// typically beyond the installment calendar (post-financing,
	// post-escritura). When SaldoAmountCents > 0 the wizard must also
	// supply SaldoDueDate.
	SaldoAmountCents int64      `json:"saldo_amount_cents,omitempty"`
	SaldoDueDate     *time.Time `json:"saldo_due_date,omitempty"`
	SaldoDescription string     `json:"saldo_description,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// CompositeInput is the structured payload of New when callers need to
// declare more than the simple "downpayment + installments" structure.
// Zero-valued fields mean "leg not used" and are excluded from charges
// and validation totals. Passed by value to keep call sites readable.
type CompositeInput struct {
	DacaoAmountCents int64      `json:"dacao_amount_cents,omitempty"`
	DacaoDescription string     `json:"dacao_description,omitempty"`
	TrocaAmountCents int64      `json:"troca_amount_cents,omitempty"`
	TrocaDescription string     `json:"troca_description,omitempty"`
	SaldoAmountCents int64      `json:"saldo_amount_cents,omitempty"`
	SaldoDueDate     *time.Time `json:"saldo_due_date,omitempty"`
	SaldoDescription string     `json:"saldo_description,omitempty"`
}

// New constructs a plan and validates the composite invariant:
//
//	DownPayment + Installments*Count + Saldo + Dacao + Troca = TotalAmount
//
// (with a per-installment cent tolerance for rounding residuals).
//
//   - SINGLE: installment_count must be 1; cash legs that aren't the
//     single installment can still be non-zero (e.g. an "à vista" sale
//     with dação cobrindo metade has DownPayment=0, Installment=metade,
//     Dacao=metade).
//   - INSTALLMENTS: count >= 2.
//
// When SaldoAmountCents > 0 a SaldoDueDate must be provided so the saldo
// charge has a deterministic due date. recurrenceIntervalDays defaults
// to 30 when <= 0.
func New(tenantID, contractID, dealID, buyerPartyID, ownerID, propertyID string, mode Mode, totalCents, downPaymentCents, installmentAmountCents int64, installmentCount int, firstDueDate time.Time, recurrenceIntervalDays int, composite CompositeInput) (*SaleBillingPlan, error) {
	if tenantID == "" || contractID == "" {
		return nil, errors.New("tenant_id and contract_id are required")
	}
	if totalCents <= 0 {
		return nil, errors.New("total_amount_cents must be > 0")
	}
	if downPaymentCents < 0 {
		return nil, errors.New("down_payment_cents must be >= 0")
	}
	if composite.DacaoAmountCents < 0 || composite.TrocaAmountCents < 0 || composite.SaldoAmountCents < 0 {
		return nil, errors.New("composite leg amounts must be >= 0")
	}
	if composite.SaldoAmountCents > 0 && composite.SaldoDueDate == nil {
		return nil, errors.New("saldo_due_date is required when saldo_amount_cents > 0")
	}
	if recurrenceIntervalDays <= 0 {
		recurrenceIntervalDays = 30
	}
	switch mode {
	case ModeSingle:
		if installmentCount != 1 {
			return nil, errors.New("SINGLE mode requires installment_count = 1")
		}
		// In SINGLE mode the "installment" can be zero when the cash
		// portion is paid entirely by entrada+saldo or by dação/troca.
		if installmentAmountCents < 0 {
			return nil, errors.New("installment_amount_cents must be >= 0")
		}
	case ModeInstallments:
		if installmentCount < 2 {
			return nil, errors.New("INSTALLMENTS mode requires installment_count >= 2")
		}
		if installmentAmountCents <= 0 {
			return nil, errors.New("installment_amount_cents must be > 0")
		}
	default:
		return nil, errors.New("invalid mode")
	}

	sum := downPaymentCents +
		installmentAmountCents*int64(installmentCount) +
		composite.DacaoAmountCents +
		composite.TrocaAmountCents +
		composite.SaldoAmountCents
	delta := sum - totalCents
	if delta < 0 {
		delta = -delta
	}
	tolerance := int64(installmentCount)
	if tolerance < 1 {
		tolerance = 1
	}
	if delta > tolerance {
		return nil, errors.New("composite legs (entrada + parcelas + dacao + troca + saldo) do not sum to total_amount_cents")
	}

	now := time.Now()
	firstDueDate = time.Date(firstDueDate.Year(), firstDueDate.Month(), firstDueDate.Day(), 0, 0, 0, 0, firstDueDate.Location())
	var normalizedSaldoDue *time.Time
	if composite.SaldoDueDate != nil {
		d := *composite.SaldoDueDate
		nd := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, d.Location())
		normalizedSaldoDue = &nd
	}

	return &SaleBillingPlan{
		ID:                     uuid.New().String(),
		TenantID:               tenantID,
		ContractID:             contractID,
		DealID:                 dealID,
		BuyerPartyID:           buyerPartyID,
		OwnerID:                ownerID,
		PropertyID:             propertyID,
		Mode:                   mode,
		TotalAmountCents:       totalCents,
		DownPaymentCents:       downPaymentCents,
		InstallmentCount:       installmentCount,
		InstallmentAmountCents: installmentAmountCents,
		FirstDueDate:           firstDueDate,
		RecurrenceIntervalDays: recurrenceIntervalDays,
		DacaoAmountCents:       composite.DacaoAmountCents,
		DacaoDescription:       composite.DacaoDescription,
		TrocaAmountCents:       composite.TrocaAmountCents,
		TrocaDescription:       composite.TrocaDescription,
		SaldoAmountCents:       composite.SaldoAmountCents,
		SaldoDueDate:           normalizedSaldoDue,
		SaldoDescription:       composite.SaldoDescription,
		CreatedAt:              now,
		UpdatedAt:              now,
	}, nil
}

// HasSaldo reports whether the plan reserves a deferred balance charge.
func (p *SaleBillingPlan) HasSaldo() bool {
	return p.SaldoAmountCents > 0
}

// HasNonCashLegs reports whether the plan includes dação or troca.
// Useful for the UI to badge the plan as "composite" and surface the
// asset descriptions on the contract demonstrativo.
func (p *SaleBillingPlan) HasNonCashLegs() bool {
	return p.DacaoAmountCents > 0 || p.TrocaAmountCents > 0
}

func ValidateMode(m Mode) error {
	switch m {
	case ModeSingle, ModeInstallments:
		return nil
	}
	return errors.New("invalid mode")
}

// HasDownPayment returns true when the plan reserves a discrete down
// payment that emits its own charge ahead of the regular installments.
func (p *SaleBillingPlan) HasDownPayment() bool {
	return p.DownPaymentCents > 0
}
