package services

import (
	"context"
	"errors"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

// TenantFinancialSettingsService expõe Get/Update sobre o port do repo.
// Get sempre devolve um Settings populado (defaults quando ausente);
// Update valida + grava + audit. Consumido por:
//   - tenant_financial_settings_handler (GET/PUT /financial-settings)
//   - delinquency_service.rateForCharge (fallback chain)
//   - charge_service (criação manual pré-preenchida)
//   - rental_charge_config_service (default_due_day)
//   - event_handler.executeContractLogic (default commission_pct rent/sale)
//   - payout_order_service (payout_proof_required guard)
type TenantFinancialSettingsService struct {
	repo tenant_financial_settings.Repository
}

func NewTenantFinancialSettingsService(repo tenant_financial_settings.Repository) *TenantFinancialSettingsService {
	return &TenantFinancialSettingsService{repo: repo}
}

func (s *TenantFinancialSettingsService) Get(ctx context.Context, tenantID string) (*tenant_financial_settings.Settings, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	return s.repo.Get(ctx, tenantID)
}

// Update aceita um payload do handler PUT, valida, persiste e registra
// audit. UpdatedBy vem do account_id do JWT (caller injeta antes).
func (s *TenantFinancialSettingsService) Update(ctx context.Context, settings *tenant_financial_settings.Settings) (*tenant_financial_settings.Settings, error) {
	if settings == nil {
		return nil, errors.New("settings is nil")
	}
	if err := settings.Validate(); err != nil {
		return nil, err
	}
	// Snapshot do estado anterior para o audit "before/after".
	before, err := s.repo.Get(ctx, settings.TenantID)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Upsert(ctx, settings); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "update",
		EntityType: "TenantFinancialSettings",
		EntityID:   settings.TenantID,
		Before: map[string]any{
			"late_fee_pct":          before.DefaultLateFeePct,
			"interest_pct":          before.DefaultMonthlyInterestPct,
			"due_day":               before.DefaultDueDay,
			"commission_pct_rent":   before.DefaultCommissionPctRent,
			"commission_pct_sale":   before.DefaultCommissionPctSale,
			"payout_proof_required": before.PayoutProofRequired,
			"lease_term_months":     before.DefaultLeaseTermMonths,
		},
		After: map[string]any{
			"late_fee_pct":          settings.DefaultLateFeePct,
			"interest_pct":          settings.DefaultMonthlyInterestPct,
			"due_day":               settings.DefaultDueDay,
			"commission_pct_rent":   settings.DefaultCommissionPctRent,
			"commission_pct_sale":   settings.DefaultCommissionPctSale,
			"payout_proof_required": settings.PayoutProofRequired,
			"lease_term_months":     settings.DefaultLeaseTermMonths,
		},
	})
	return settings, nil
}
