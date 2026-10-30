// Package utility_account stores the recurring utility/service accounts
// (água, energia, gás, condomínio, IPTU, internet, seguro, outros) tied to
// a rental contract. Each row represents an external account with a
// provider that periodically issues bills. The actual bill records flow
// through the existing `charge` domain — this entity is the configuration
// catalog used by the admin UI in Rentals → Contas de Consumo.
package utility_account

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypeAgua       Type = "AGUA"
	TypeEnergia    Type = "ENERGIA"
	TypeGas        Type = "GAS"
	TypeCondominio Type = "CONDOMINIO"
	TypeIPTU       Type = "IPTU"
	TypeInternet   Type = "INTERNET"
	TypeSeguro     Type = "SEGURO"
	TypeOutros     Type = "OUTROS"
)

type ResponsibleParty string

const (
	ResponsibleTenant ResponsibleParty = "TENANT"
	ResponsibleOwner  ResponsibleParty = "OWNER"
	ResponsibleAgency ResponsibleParty = "AGENCY"
)

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
)

// BillingMode controls how this utility account participates in the
// monthly billing pipeline:
//
//   - BillingModeBundled: the value is folded into the parent rental's
//     RentalChargeConfig.BundledItems and appears as a line item on the
//     monthly rent charge. The actual amount lives on the BundledItem,
//     this row is just a reference for the catalog UI.
//   - BillingModeVariableSeparate: each month the operator launches the
//     consumption value (água, luz medidos) which materializes a new
//     standalone charge.TypeUtility cobrança.
//   - BillingModeInformational: legacy/catalog-only. No charges generated;
//     used when the tenant pays the provider directly and the agency only
//     tracks the account for context.
type BillingMode string

const (
	BillingModeBundled          BillingMode = "BUNDLED"
	BillingModeVariableSeparate BillingMode = "VARIABLE_SEPARATE"
	BillingModeInformational    BillingMode = "INFORMATIONAL"
)

func ValidateBillingMode(m BillingMode) error {
	switch m {
	case BillingModeBundled, BillingModeVariableSeparate, BillingModeInformational, "":
		return nil
	}
	return errors.New("invalid billing_mode")
}

// UtilityAccount is the configuration record. Money fields are stored in
// cents (int64) to avoid float drift over a contract lifetime — same
// convention as Charge/RentalChargeConfig.
type UtilityAccount struct {
	ID               string           `json:"id"`
	TenantID         string           `json:"tenant_id"`
	ContractID       string           `json:"contract_id"`
	PropertyID       string           `json:"property_id"`
	Type             Type             `json:"type"`
	Provider         string           `json:"provider"`
	AccountNumber    string           `json:"account_number"`
	ResponsibleParty ResponsibleParty `json:"responsible_party"`
	EstimatedAmount  int64            `json:"estimated_amount_cents"`
	PaymentMethod    string           `json:"payment_method,omitempty"`
	AutoPayEnabled   bool             `json:"auto_pay_enabled"`
	DueDay           int              `json:"due_day,omitempty"`
	LastPaymentDate  *time.Time       `json:"last_payment_date,omitempty"`
	NextDueDate      *time.Time       `json:"next_due_date,omitempty"`
	Status           Status           `json:"status"`
	Notes            string           `json:"notes,omitempty"`
	// BillingMode determines whether this account drives charge emission
	// (VARIABLE_SEPARATE), is folded into the parent rent charge as a
	// bundled item reference (BUNDLED) or is purely informational
	// (INFORMATIONAL — the historical default). Empty == INFORMATIONAL.
	BillingMode BillingMode `json:"billing_mode,omitempty"`
	// BundledItemKey ties this account to the matching entry in
	// RentalChargeConfig.BundledItems when BillingMode=BUNDLED. The actual
	// monthly value lives on the bundled item; this is just a cross-ref.
	BundledItemKey string    `json:"bundled_item_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func New(tenantID, contractID, propertyID string, t Type, provider, accountNumber string, responsible ResponsibleParty, estimatedAmount int64) (*UtilityAccount, error) {
	if tenantID == "" || contractID == "" || propertyID == "" {
		return nil, errors.New("tenant_id, contract_id and property_id are required")
	}
	if err := ValidateType(t); err != nil {
		return nil, err
	}
	if err := ValidateResponsible(responsible); err != nil {
		return nil, err
	}
	if strings.TrimSpace(provider) == "" {
		return nil, errors.New("provider is required")
	}
	if estimatedAmount < 0 {
		return nil, errors.New("estimated_amount must be >= 0")
	}
	now := time.Now()
	return &UtilityAccount{
		ID:               uuid.New().String(),
		TenantID:         tenantID,
		ContractID:       contractID,
		PropertyID:       propertyID,
		Type:             t,
		Provider:         strings.TrimSpace(provider),
		AccountNumber:    strings.TrimSpace(accountNumber),
		ResponsibleParty: responsible,
		EstimatedAmount:  estimatedAmount,
		Status:           StatusActive,
		BillingMode:      BillingModeInformational,
		CreatedAt:        now,
		UpdatedAt:        now,
	}, nil
}

func ValidateType(t Type) error {
	switch t {
	case TypeAgua, TypeEnergia, TypeGas, TypeCondominio, TypeIPTU, TypeInternet, TypeSeguro, TypeOutros:
		return nil
	}
	return errors.New("invalid utility account type")
}

func ValidateResponsible(r ResponsibleParty) error {
	switch r {
	case ResponsibleTenant, ResponsibleOwner, ResponsibleAgency:
		return nil
	}
	return errors.New("invalid responsible party")
}
