package repositories

import (
	"encoding/json"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

func TestFillFinancialSettingsDefaultsPreservesExplicitZeroAndFalse(t *testing.T) {
	metadata := []byte(`{
		"tenant_id":"tenant-1",
		"default_late_fee_pct":0,
		"default_monthly_interest_pct":0,
		"default_due_day":10,
		"default_commission_pct_rent":0,
		"default_commission_pct_sale":0,
		"default_currency":"BRL",
		"payout_proof_required":false,
		"admin_fee_pct":0
	}`)
	var settings tenant_financial_settings.Settings
	if err := json.Unmarshal(metadata, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if err := fillDefaults(&settings, metadata); err != nil {
		t.Fatalf("fill defaults: %v", err)
	}

	if settings.DefaultCommissionPctSale != 0 || settings.DefaultCommissionPctRent != 0 {
		t.Fatalf("explicit zero commission must be preserved: %+v", settings)
	}
	if settings.DefaultLateFeePct != 0 || settings.DefaultMonthlyInterestPct != 0 || settings.AdminFeePct != 0 {
		t.Fatalf("explicit zero rates must be preserved: %+v", settings)
	}
	if settings.PayoutProofRequired {
		t.Fatal("explicit payout_proof_required=false must be preserved")
	}
}

func TestFillFinancialSettingsDefaultsOnlyBackfillsMissingKeys(t *testing.T) {
	metadata := []byte(`{"tenant_id":"tenant-1"}`)
	var settings tenant_financial_settings.Settings
	if err := json.Unmarshal(metadata, &settings); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	if err := fillDefaults(&settings, metadata); err != nil {
		t.Fatalf("fill defaults: %v", err)
	}

	defaults := tenant_financial_settings.New("tenant-1")
	if settings.DefaultCommissionPctSale != defaults.DefaultCommissionPctSale ||
		settings.DefaultCommissionPctRent != defaults.DefaultCommissionPctRent ||
		settings.DefaultLateFeePct != defaults.DefaultLateFeePct ||
		settings.DefaultMonthlyInterestPct != defaults.DefaultMonthlyInterestPct ||
		settings.DefaultDueDay != defaults.DefaultDueDay ||
		settings.DefaultCurrency != defaults.DefaultCurrency ||
		settings.PayoutProofRequired != defaults.PayoutProofRequired ||
		settings.AdminFeePct != defaults.AdminFeePct {
		t.Fatalf("missing keys were not backfilled from defaults: got=%+v want=%+v", settings, defaults)
	}
}

// Prazo padrão da locação: campo novo com default de 30 meses. Row antiga (sem
// a chave) recebe o default; valor configurado é preservado.
func TestFillFinancialSettingsDefaultsBackfillsLeaseTermMonths(t *testing.T) {
	legacy := []byte(`{"tenant_id":"tenant-1","admin_fee_pct":8}`)
	var settings tenant_financial_settings.Settings
	if err := json.Unmarshal(legacy, &settings); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if settings.DefaultLeaseTermMonths != 0 {
		t.Fatalf("sem a chave no JSON o campo deveria começar zerado, got %d", settings.DefaultLeaseTermMonths)
	}
	if err := fillDefaults(&settings, legacy); err != nil {
		t.Fatalf("fill defaults: %v", err)
	}
	if settings.DefaultLeaseTermMonths != tenant_financial_settings.DefaultLeaseTermMonths {
		t.Fatalf("esperava default %d, got %d", tenant_financial_settings.DefaultLeaseTermMonths, settings.DefaultLeaseTermMonths)
	}

	configured := []byte(`{"tenant_id":"tenant-1","default_lease_term_months":12}`)
	var s2 tenant_financial_settings.Settings
	if err := json.Unmarshal(configured, &s2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := fillDefaults(&s2, configured); err != nil {
		t.Fatalf("fill defaults: %v", err)
	}
	if s2.DefaultLeaseTermMonths != 12 {
		t.Fatalf("valor configurado deve ser preservado, got %d", s2.DefaultLeaseTermMonths)
	}
}

func TestTenantFinancialSettingsValidateLeaseTerm(t *testing.T) {
	valid := tenant_financial_settings.New("tenant-1")
	if err := valid.Validate(); err != nil {
		t.Fatalf("defaults devem validar: %v", err)
	}
	for _, months := range []int{0, -1, 121} {
		s := tenant_financial_settings.New("tenant-1")
		s.DefaultLeaseTermMonths = months
		if err := s.Validate(); err == nil {
			t.Fatalf("prazo %d deveria ser recusado", months)
		}
	}
	for _, months := range []int{1, 12, 120} {
		s := tenant_financial_settings.New("tenant-1")
		s.DefaultLeaseTermMonths = months
		if err := s.Validate(); err != nil {
			t.Fatalf("prazo %d deveria ser aceito: %v", months, err)
		}
	}
}
