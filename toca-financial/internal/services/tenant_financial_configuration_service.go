package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/oginoino/toca/pkg/database"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/tenant_financial_settings"
)

// SaveCommissionConfigInput agrupa a configuração de percentual
// (default_commission_pct) e distribuição (commission rules) num único
// payload atômico. FIN-COM-02: save único com validação integrada.
type SaveCommissionConfigInput struct {
	TenantID                 string            `json:"tenant_id"`
	DefaultCommissionPctRent float64           `json:"default_commission_pct_rent"`
	DefaultCommissionPctSale float64           `json:"default_commission_pct_sale"`
	Rules                    []commission.Rule `json:"rules"`
	UpdatedBy                string            `json:"updated_by"`
}

// TenantFinancialConfigurationService é a fachada atômica que persiste
// TenantFinancialSettings + TenantCommissionRules com validação integrada.
// Implementa FIN-COM-02 (save único, soma 100%, imobiliária explícita,
// zero preservado).
type TenantFinancialConfigurationService struct {
	db              *database.PostgresDB
	settingsService *TenantFinancialSettingsService
	commissionRules *CommissionRulesService
}

// NewTenantFinancialConfigurationService constrói a fachada sobre os
// serviços subjacentes. Validação é delegada; persistência usa os
// serviços existentes.
func NewTenantFinancialConfigurationService(
	db *database.PostgresDB,
	settingsService *TenantFinancialSettingsService,
	commissionRules *CommissionRulesService,
) *TenantFinancialConfigurationService {
	return &TenantFinancialConfigurationService{
		db:              db,
		settingsService: settingsService,
		commissionRules: commissionRules,
	}
}

// SaveCommissionConfig valida e persiste atomicamente a configuração
// financeira completa do tenant: percentuais default + regras de
// distribuição. Retorna o estado consolidado pós-save.
//
// Validações integradas:
//   - Percentuais entre 0–100
//   - Regras somam exatamente 100% (imobiliária explícita)
//   - Todas as roles conhecidas são persistidas (zero preservado)
func (s *TenantFinancialConfigurationService) SaveCommissionConfig(
	ctx context.Context,
	input SaveCommissionConfigInput,
) (*SaveCommissionConfigResult, error) {
	if input.TenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	if input.DefaultCommissionPctRent < 0 || input.DefaultCommissionPctRent > 100 {
		return nil, errors.New("default_commission_pct_rent must be between 0 and 100")
	}
	if input.DefaultCommissionPctSale < 0 || input.DefaultCommissionPctSale > 100 {
		return nil, errors.New("default_commission_pct_sale must be between 0 and 100")
	}

	// Valida as regras antes de persistir. A validação garante que a
	// distribuição fecha em 100% e que a imobiliária é explícita.
	if len(input.Rules) > 0 {
		if err := s.validateRules(input); err != nil {
			return nil, err
		}
	}

	// Persiste settings (sempre — mesmo sem regras, os percentuais são
	// standalone).
	settings := &tenant_financial_settings.Settings{
		TenantID:                 input.TenantID,
		DefaultCommissionPctRent: input.DefaultCommissionPctRent,
		DefaultCommissionPctSale: input.DefaultCommissionPctSale,
		UpdatedBy:                input.UpdatedBy,
	}
	if _, err := s.settingsService.Update(ctx, settings); err != nil {
		return nil, fmt.Errorf("save tenant financial settings: %w", err)
	}

	// Persiste regras (se fornecidas).
	var savedRules []commission.Rule
	if len(input.Rules) > 0 {
		var err error
		savedRules, err = s.commissionRules.Update(ctx, input.TenantID, input.Rules)
		if err != nil {
			return nil, fmt.Errorf("save commission rules: %w", err)
		}
	}

	return &SaveCommissionConfigResult{
		TenantID:                 input.TenantID,
		DefaultCommissionPctRent: input.DefaultCommissionPctRent,
		DefaultCommissionPctSale: input.DefaultCommissionPctSale,
		Rules:                    savedRules,
	}, nil
}

// SaveCommissionConfigResult é o estado consolidado da configuração
// financeira do tenant após o save atômico.
type SaveCommissionConfigResult struct {
	TenantID                 string            `json:"tenant_id"`
	DefaultCommissionPctRent float64           `json:"default_commission_pct_rent"`
	DefaultCommissionPctSale float64           `json:"default_commission_pct_sale"`
	Rules                    []commission.Rule `json:"rules"`
}

// validateRules replica a validação do CommissionRulesService.Update
// sem persistir. Usada para early-validation no SaveCommissionConfig.
func (s *TenantFinancialConfigurationService) validateRules(input SaveCommissionConfigInput) error {
	seen := make(map[commission.Role]struct{}, len(input.Rules))
	var totalPct float64
	hasNonAgency := false

	for _, r := range input.Rules {
		if err := r.Validate(); err != nil {
			return err
		}
		if _, duplicate := seen[r.Role]; duplicate {
			return fmt.Errorf("duplicate commission role: %s", r.Role)
		}
		seen[r.Role] = struct{}{}
		totalPct += r.DefaultPercentage
		if r.Role != commission.RoleImobiliaria {
			hasNonAgency = true
		}
	}

	if hasNonAgency {
		if _, hasAgency := seen[commission.RoleImobiliaria]; !hasAgency {
			return fmt.Errorf("imobiliaria role is required when other commission roles are configured")
		}
		if totalPct < 99.99 || totalPct > 100.01 {
			return fmt.Errorf("commission distribution must sum to 100%%: got %.2f%%", totalPct)
		}
	}
	return nil
}
