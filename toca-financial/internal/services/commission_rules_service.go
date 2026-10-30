package services

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/oginoino/toca/pkg/audit"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	repository "github.com/oginoino/toca/toca-financial/internal/repositories"
)

// CommissionRulesService combina o repo (overrides por tenant) com o
// fallback de commission.DefaultPercentages. Consumido por:
//   - commission_rules_handler (GET/PUT /commission-rules)
//   - ResolveCommissionPlan (substitui o
//     PercentageFor(nil, ...) hardcoded — fecha o TODO #420).
type CommissionRulesService struct {
	repo repository.TenantCommissionRulesRepository
}

var knownCommissionRoles = []commission.Role{
	commission.RoleCaptador,
	commission.RoleCoCaptador,
	commission.RoleVisitador,
	commission.RoleAtendente,
	commission.RoleFechamento,
	commission.RoleImobiliaria,
}

func NewCommissionRulesService(repo repository.TenantCommissionRulesRepository) *CommissionRulesService {
	return &CommissionRulesService{repo: repo}
}

// Get devolve todas as regras conhecidas para o tenant: os overrides
// gravados + entradas default para roles que ainda não foram persistidas.
// Update sempre grava as seis roles, inclusive zeros, de modo que um
// replace-all parcial nunca reintroduza defaults na leitura seguinte.
func (s *CommissionRulesService) Get(ctx context.Context, tenantID string) ([]commission.Rule, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}
	overrides, err := s.repo.List(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	byRole := make(map[commission.Role]commission.Rule, len(overrides))
	for _, r := range overrides {
		byRole[r.Role] = r
	}
	out := make([]commission.Rule, 0, len(knownCommissionRoles))
	for _, role := range knownCommissionRoles {
		if rule, ok := byRole[role]; ok {
			out = append(out, rule)
			continue
		}
		out = append(out, commission.Rule{
			TenantID:          tenantID,
			Role:              role,
			DefaultPercentage: commission.DefaultPercentages[role],
		})
	}
	return out, nil
}

// Update grava o conjunto novo de regras como replace-all normalizado.
//
// FIN-COM-02: a distribuição efetiva deve somar exatamente 100% e a
// imobiliária deve ser explícita. O serviço NÃO calcula residual — o
// caller deve fornecer todas as roles com percentuais que fecham em 100%.
// Uma role omitida é persistida com zero explícito; assim, zero ≠ ausente
// e Get não volta a injetar o default do pacote.
//
// Validação:
//   - Soma das regras fornecidas deve ser 100% ± 1e-6.
//   - Imobiliária deve estar presente explicitamente quando há outras roles.
//   - Cada Rule.Validate() individual deve passar.
func (s *CommissionRulesService) Update(ctx context.Context, tenantID string, rules []commission.Rule) ([]commission.Rule, error) {
	if tenantID == "" {
		return nil, errors.New("tenant_id is required")
	}

	byRole := make(map[commission.Role]commission.Rule, len(knownCommissionRoles))
	seen := make(map[commission.Role]struct{}, len(rules))
	var totalPct float64
	hasNonAgency := false

	for i := range rules {
		r := rules[i]
		r.TenantID = tenantID
		if err := r.Validate(); err != nil {
			return nil, err
		}
		if _, duplicate := seen[r.Role]; duplicate {
			return nil, fmt.Errorf("duplicate commission role: %s", r.Role)
		}
		seen[r.Role] = struct{}{}
		byRole[r.Role] = r
		totalPct += r.DefaultPercentage
		if r.Role != commission.RoleImobiliaria {
			hasNonAgency = true
		}
	}

	// FIN-COM-02: distribuição deve fechar em 100%. Imobiliária é
	// explícita — se há outras roles, a imobiliária também precisa
	// estar no payload com o percentual que fecha a conta.
	if hasNonAgency {
		if _, hasAgency := seen[commission.RoleImobiliaria]; !hasAgency {
			return nil, fmt.Errorf("imobiliaria role is required when other commission roles are configured — provide an explicit percentage that sums to 100%%")
		}
		if math.Abs(totalPct-100) > 1e-6 {
			return nil, fmt.Errorf("commission distribution must sum to 100%%: got %.2f%%", totalPct)
		}
	}

	cleaned := make([]commission.Rule, 0, len(knownCommissionRoles))
	for _, role := range knownCommissionRoles {
		rule, ok := byRole[role]
		if !ok {
			// Role omitida → zero explícito. Zero é uma escolha
			// (não um fallback para default), preservado pelo repo.
			rule = commission.Rule{TenantID: tenantID, Role: role, DefaultPercentage: 0}
		}
		cleaned = append(cleaned, rule)
	}

	before, _ := s.repo.List(ctx, tenantID)
	if err := s.repo.Upsert(ctx, tenantID, cleaned); err != nil {
		return nil, err
	}
	audit.Record(ctx, audit.Entry{
		Action:     "update",
		EntityType: "TenantCommissionRules",
		EntityID:   tenantID,
		Before:     map[string]any{"rules": ruleMap(before)},
		After:      map[string]any{"rules": ruleMap(cleaned)},
	})
	return s.Get(ctx, tenantID)
}

// PercentageFor é o helper que o event_handler consome no caminho crítico
// do contract.signed. Reusa commission.PercentageFor (já lida com fallback
// para DefaultPercentages quando rules está vazio). Encapsula a leitura
// do repo + a lógica de fallback num único call.
func (s *CommissionRulesService) PercentageFor(ctx context.Context, tenantID string, role commission.Role) (float64, error) {
	rules, err := s.repo.List(ctx, tenantID)
	if err != nil {
		return 0, err
	}
	return commission.PercentageFor(rules, role), nil
}

func ruleMap(rules []commission.Rule) map[string]float64 {
	m := make(map[string]float64, len(rules))
	for _, r := range rules {
		// Trunca a 4 casas para evitar ruído de comparação em audit.
		m[string(r.Role)] = math.Round(r.DefaultPercentage*10000) / 10000
	}
	return m
}
