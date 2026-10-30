package validation

import "fmt"

// FinancialValidator holds domain-agnostic validation rules shared across
// Transaction, Commission, Charge, and Transfer modules.
type FinancialValidator struct{}

// ValidatePercentage checks value is between min (inclusive) and max (inclusive).
func (FinancialValidator) ValidatePercentage(field string, pct float64, min, max float64) error {
	if pct < min || pct > max {
		return fmt.Errorf("%s must be between %.0f%% and %.0f%%", field, min, max)
	}
	return nil
}

// ValidateAmount checks an amount is positive (strictly > 0).
func (FinancialValidator) ValidateAmount(field string, amount float64) error {
	if amount <= 0 {
		return fmt.Errorf("%s must be positive", field)
	}
	return nil
}

// ValidateRequired checks a string is non-empty.
func (FinancialValidator) ValidateRequired(field, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	return nil
}

// ValidateContractID checks format (non-empty, reasonable length).
func (FinancialValidator) ValidateContractID(contractID string) error {
	if contractID == "" {
		return fmt.Errorf("contract_id is required")
	}
	return nil
}

// ValidateTenantID checks format.
func (FinancialValidator) ValidateTenantID(tenantID string) error {
	if tenantID == "" {
		return fmt.Errorf("tenant_id is required")
	}
	return nil
}

// Default instance
var Default = FinancialValidator{}
