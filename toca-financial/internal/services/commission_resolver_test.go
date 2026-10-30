package services

import (
	"testing"

	"github.com/oginoino/toca/pkg/events/contracts"
	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
)

func TestResolveCommissionPlanSeparatesAgencyRetentionFromPayables(t *testing.T) {
	plan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:      "SALE",
		DealValue:     100_000,
		CommissionPct: 6,
		BrokerParticipations: []contracts.BrokerParticipation{
			{BrokerID: "broker-captador", Role: "captador", Percentage: 30},
			{BrokerID: "broker-fechamento", Role: "fechamento", Percentage: 50},
			{Role: "imobiliaria", Percentage: 20},
		},
	})
	if err != nil {
		t.Fatalf("ResolveCommissionPlan() error = %v", err)
	}
	if len(plan.Shares) != 2 {
		t.Fatalf("payable shares = %d, want 2", len(plan.Shares))
	}
	if plan.RetainedPercentage != 20 || plan.RetainedAmountCents != 120_000 {
		t.Fatalf("retention = %.2f%% / %d, want 20%% / 120000", plan.RetainedPercentage, plan.RetainedAmountCents)
	}
}

func TestResolveCommissionPlanRejectsOverallocatedAndDuplicateParticipants(t *testing.T) {
	tests := []struct {
		name           string
		participations []contracts.BrokerParticipation
	}{
		{
			name: "overallocated",
			participations: []contracts.BrokerParticipation{
				{BrokerID: "broker-1", Role: "captador", Percentage: 60},
				{BrokerID: "broker-2", Role: "fechamento", Percentage: 60},
			},
		},
		{
			name: "duplicate broker role",
			participations: []contracts.BrokerParticipation{
				{BrokerID: "broker-1", Role: "captador", Percentage: 50},
				{BrokerID: "broker-1", Role: "captador", Percentage: 50},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ResolveCommissionPlan(CommissionPlanInput{
				DealType:             "SALE",
				DealValue:            100_000,
				CommissionPct:        6,
				BrokerParticipations: tt.participations,
			})
			if err == nil {
				t.Fatal("ResolveCommissionPlan() expected validation error")
			}
		})
	}
}

// O rateio sempre fecha 100%: o que não for atribuído a corretor fica retido
// pela imobiliária (mesma leitura de Configurações › Comissões, que mostra o
// restante como receita retida). Nenhuma fatia de corretor é inflada.
func TestResolveCommissionPlanFillsAgencyRetentionWhenDistributionIsIncomplete(t *testing.T) {
	plan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:      "SALE",
		DealValue:     1_400_000,
		CommissionPct: 6,
		BrokerParticipations: []contracts.BrokerParticipation{
			{BrokerID: "broker-captador", Role: "captador", Percentage: 20},
			{BrokerID: "broker-fechamento", Role: "fechamento", Percentage: 40},
		},
	})
	if err != nil {
		t.Fatalf("ResolveCommissionPlan() error = %v", err)
	}
	if len(plan.Shares) != 2 {
		t.Fatalf("payable shares = %d, want 2", len(plan.Shares))
	}
	if plan.Shares[0].AmountCents != 1_680_000 || plan.Shares[1].AmountCents != 3_360_000 {
		t.Fatalf("broker shares = %+v, want 1680000 and 3360000 cents", plan.Shares)
	}
	if plan.RetainedPercentage != 40 || plan.RetainedAmountCents != 3_360_000 {
		t.Fatalf("retention = %.2f%% / %d, want 40%% / 3360000", plan.RetainedPercentage, plan.RetainedAmountCents)
	}
	// A fatia declarada da imobiliária continua explícita: nada é somado duas vezes.
	planWithAgency, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:      "SALE",
		DealValue:     1_400_000,
		CommissionPct: 6,
		BrokerParticipations: []contracts.BrokerParticipation{
			{BrokerID: "broker-captador", Role: "captador", Percentage: 20},
			{BrokerID: "broker-fechamento", Role: "fechamento", Percentage: 40},
			{Role: "imobiliaria", Percentage: 40},
		},
	})
	if err != nil {
		t.Fatalf("ResolveCommissionPlan() with explicit agency error = %v", err)
	}
	if planWithAgency.RetainedAmountCents != plan.RetainedAmountCents || len(planWithAgency.Shares) != len(plan.Shares) {
		t.Fatalf("retenção explícita e retenção residual devem coincidir: %+v vs %+v", planWithAgency, plan)
	}
}

// Sem nenhum participante não existe beneficiário: a comissão inteira fica
// retida, em vez de o plano nascer sem destino.
func TestResolveCommissionPlanWithoutParticipantsRetainsEverything(t *testing.T) {
	plan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:      "SALE",
		DealValue:     500_000,
		CommissionPct: 6,
	})
	if err != nil {
		t.Fatalf("ResolveCommissionPlan() error = %v", err)
	}
	if len(plan.Shares) != 0 {
		t.Fatalf("payable shares = %d, want 0", len(plan.Shares))
	}
	if plan.RetainedPercentage != 100 || plan.RetainedAmountCents != 3_000_000 {
		t.Fatalf("retention = %.2f%% / %d, want 100%% / 3000000", plan.RetainedPercentage, plan.RetainedAmountCents)
	}
}

func TestResolveCommissionPlanUsesRulesAndRequiresClosedDistribution(t *testing.T) {
	plan, err := ResolveCommissionPlan(CommissionPlanInput{
		DealType:      "SALE",
		DealValue:     2_000,
		CommissionPct: 7.5,
		Rules: []commission.Rule{
			{Role: commission.RoleCaptador, DefaultPercentage: 40},
			{Role: commission.RoleImobiliaria, DefaultPercentage: 60},
		},
		BrokerParticipations: []contracts.BrokerParticipation{
			{BrokerID: "broker-1", Role: "captador"},
			{Role: "imobiliaria"},
		},
	})
	if err != nil {
		t.Fatalf("ResolveCommissionPlan() error = %v", err)
	}
	if len(plan.Shares) != 1 || plan.Shares[0].AmountCents != 6000 {
		t.Fatalf("broker share = %+v, want one share of 6000 cents", plan.Shares)
	}
	if plan.RetainedAmountCents != 9000 {
		t.Fatalf("retained amount = %d, want 9000", plan.RetainedAmountCents)
	}
}
