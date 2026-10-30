package services

import (
	"context"
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
)

type memoryCommissionRulesRepository struct {
	rules       []commission.Rule
	upsertCalls int
}

func (r *memoryCommissionRulesRepository) List(_ context.Context, _ string) ([]commission.Rule, error) {
	return append([]commission.Rule(nil), r.rules...), nil
}

func (r *memoryCommissionRulesRepository) Upsert(_ context.Context, _ string, rules []commission.Rule) error {
	r.rules = append([]commission.Rule(nil), rules...)
	r.upsertCalls++
	return nil
}

func TestCommissionRulesServiceUpdateNormalizesPartialReplaceAll(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	// FIN-COM-02: imobiliária é explícita. captador=100 requer
	// imobiliária=0 para fechar 100%.
	got, err := service.Update(context.Background(), "tenant-1", []commission.Rule{
		{Role: commission.RoleCaptador, DefaultPercentage: 100},
		{Role: commission.RoleImobiliaria, DefaultPercentage: 0},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if len(repo.rules) != len(knownCommissionRoles) {
		t.Fatalf("persisted %d roles, want %d", len(repo.rules), len(knownCommissionRoles))
	}
	if len(got) != len(knownCommissionRoles) {
		t.Fatalf("returned %d roles, want %d", len(got), len(knownCommissionRoles))
	}

	percentages := commissionRulePercentages(got)
	if percentages[commission.RoleCaptador] != 100 {
		t.Fatalf("captador = %v, want 100", percentages[commission.RoleCaptador])
	}
	if percentages[commission.RoleImobiliaria] != 0 {
		t.Fatalf("imobiliaria = %v, want explicit 0", percentages[commission.RoleImobiliaria])
	}
	for _, role := range knownCommissionRoles {
		if role == commission.RoleCaptador || role == commission.RoleImobiliaria {
			continue
		}
		if percentages[role] != 0 {
			t.Errorf("%s = %v, want explicit zero", role, percentages[role])
		}
	}
}

func TestCommissionRulesServiceUpdateRequiresExplicitClosedDistribution(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	// FIN-COM-02: distribuição fecha com imobiliária explícita.
	got, err := service.Update(context.Background(), "tenant-1", []commission.Rule{
		{Role: commission.RoleCaptador, DefaultPercentage: 30},
		{Role: commission.RoleFechamento, DefaultPercentage: 50},
		{Role: commission.RoleImobiliaria, DefaultPercentage: 20},
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	percentages := commissionRulePercentages(got)
	if percentages[commission.RoleImobiliaria] != 20 {
		t.Fatalf("imobiliaria = %v, want explicit 20", percentages[commission.RoleImobiliaria])
	}
	if percentages[commission.RoleCaptador] != 30 {
		t.Fatalf("captador = %v, want 30", percentages[commission.RoleCaptador])
	}
	if percentages[commission.RoleFechamento] != 50 {
		t.Fatalf("fechamento = %v, want 50", percentages[commission.RoleFechamento])
	}
	if sum := sumCommissionRulePercentages(got); sum != 100 {
		t.Fatalf("sum = %v, want 100", sum)
	}
}

func TestCommissionRulesServiceUpdateRejectsMissingAgencyWhenNonAgencyRolesPresent(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	// FIN-COM-02: sem imobiliária explícita, rejeita.
	_, err := service.Update(context.Background(), "tenant-1", []commission.Rule{
		{Role: commission.RoleCaptador, DefaultPercentage: 30},
		{Role: commission.RoleFechamento, DefaultPercentage: 50},
	})
	if err == nil || !strings.Contains(err.Error(), "imobiliaria role is required") {
		t.Fatalf("Update() error = %v, want 'imobiliaria role is required'", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("Upsert() calls = %d, want 0", repo.upsertCalls)
	}
}

func TestCommissionRulesServiceUpdateRejectsDistributionNotSummingTo100(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	// FIN-COM-02: soma != 100% rejeita.
	_, err := service.Update(context.Background(), "tenant-1", []commission.Rule{
		{Role: commission.RoleCaptador, DefaultPercentage: 60},
		{Role: commission.RoleFechamento, DefaultPercentage: 50},
		{Role: commission.RoleImobiliaria, DefaultPercentage: 10},
	})
	if err == nil || !strings.Contains(err.Error(), "must sum to 100%") {
		t.Fatalf("Update() error = %v, want 'must sum to 100%%'", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("Upsert() calls = %d, want 0", repo.upsertCalls)
	}
}

func TestCommissionRulesServiceUpdatePreservesZeroDistribution(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	got, err := service.Update(context.Background(), "tenant-1", nil)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if sum := sumCommissionRulePercentages(got); sum != 0 {
		t.Fatalf("sum = %v, want consolidated zero distribution", sum)
	}
}

func TestCommissionRulesServiceUpdateRejectsDuplicateRole(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	_, err := service.Update(context.Background(), "tenant-1", []commission.Rule{
		{Role: commission.RoleCaptador, DefaultPercentage: 30},
		{Role: commission.RoleCaptador, DefaultPercentage: 40},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate commission role") {
		t.Fatalf("Update() error = %v, want duplicate role validation", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("Upsert() calls = %d, want 0", repo.upsertCalls)
	}
}

func TestCommissionRulesServiceUpdateRejectsNonAgencySumOverOneHundred(t *testing.T) {
	repo := &memoryCommissionRulesRepository{}
	service := NewCommissionRulesService(repo)

	// FIN-COM-02: imobiliária explícita + soma > 100%. Rejeita porque
	// captador+imobiliaria=110 > 100.
	_, err := service.Update(context.Background(), "tenant-1", []commission.Rule{
		{Role: commission.RoleCaptador, DefaultPercentage: 60},
		{Role: commission.RoleImobiliaria, DefaultPercentage: 50},
	})
	if err == nil || !strings.Contains(err.Error(), "must sum to 100%") {
		t.Fatalf("Update() error = %v, want 'must sum to 100%%'", err)
	}
	if repo.upsertCalls != 0 {
		t.Fatalf("Upsert() calls = %d, want 0", repo.upsertCalls)
	}
}

func commissionRulePercentages(rules []commission.Rule) map[commission.Role]float64 {
	got := make(map[commission.Role]float64, len(rules))
	for _, rule := range rules {
		got[rule.Role] = rule.DefaultPercentage
	}
	return got
}

func sumCommissionRulePercentages(rules []commission.Rule) float64 {
	var sum float64
	for _, rule := range rules {
		sum += rule.DefaultPercentage
	}
	return sum
}
