package services

import ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"

type PlatformPlan = ps.Plan

const (
	FeatureCRM                 = "crm"
	FeaturePortfolio           = "portfolio"
	FeatureRentals             = "rentals"
	FeatureFinance             = "finance"
	FeatureCalendar            = "calendar"
	FeatureContracts           = "contracts"
	FeatureDocuments           = "documents"
	FeatureClientPortal        = "client_portal"
	FeatureElectronicSignature = "electronic_signature"
	FeatureReports             = "reports"
	FeatureIntelligence        = "intelligence"
	FeatureMCP                 = "mcp"
)

var starterFeatures = []string{
	FeatureCRM, FeaturePortfolio, FeatureRentals, FeatureFinance,
	FeatureCalendar, FeatureContracts, FeatureDocuments,
}

var proFeatures = []string{
	FeatureCRM, FeaturePortfolio, FeatureRentals, FeatureFinance,
	FeatureCalendar, FeatureContracts, FeatureDocuments, FeatureClientPortal,
	FeatureElectronicSignature, FeatureReports, FeatureIntelligence, FeatureMCP,
}

// TrialDays is the default free period before the first charge. It is part of
// the offer, so it also identifies the recurring product at the provider.
const TrialDays = 15

// Previous Pro prices remain recognizable for provider events already in
// flight. They are intentionally excluded from PlatformPlans, so new checkouts
// only offer the current catalog.
const (
	legacyProMonthlyAmountCents int64 = 229700
	legacyProAnnualAmountCents  int64 = 2205120
)

// Canonical beta pricing, matching the published in-app offer. Annual prices
// represent the complete annual charge (12 months with 20% discount).
func PlatformPlans() []PlatformPlan {
	plans := []PlatformPlan{}
	for _, p := range []PlatformPlan{
		{Tier: "starter", Name: "Starter", AmountCents: 49700, Description: "Para organizar a operação e não perder nenhum lead.",
			Limits:   ps.Limits{Leads: 200, Properties: 500, Users: 5},
			Features: append([]string(nil), starterFeatures...),
			Highlights: []string{"Leads e imóveis sem planilha paralela", "Roleta de leads e agenda da equipe",
				"Contratos, locação e financeiro no mesmo lugar", "Central de Ajuda com 20 assuntos"}},
		{Tier: "pro", Name: "Pro", AmountCents: 98700, Description: "Para imobiliárias que crescem e precisam de escala.",
			Limits:   ps.Limits{Leads: 1000, Properties: 3000, Users: 20},
			Features: append([]string(nil), proFeatures...),
			Highlights: []string{"Tudo do Starter, com limites 5x maiores", "Portal do cliente e assinatura digital",
				"Relatórios, DIMOB e informes de rendimento", "Inteligência e MCP para automações"}},
	} {
		p.TrialDays = TrialDays
		p.Cycle = "MONTHLY"
		plans = append(plans, p)
		p.Cycle = "ANNUALLY"
		p.AmountCents = p.AmountCents * 12 * 80 / 100
		plans = append(plans, p)
	}
	plans = append(plans, PlatformPlan{Tier: "enterprise", Name: "Enterprise", Cycle: "MONTHLY", Description: "Para redes e operações sob medida.", ContactOnly: true, Features: append([]string(nil), proFeatures...)})
	return plans
}
func FindPlatformPlan(tier string) (PlatformPlan, bool) {
	return FindPlatformPlanCycle(tier, "MONTHLY")
}
func FindPlatformPlanCycle(tier, cycle string) (PlatformPlan, bool) {
	for _, p := range PlatformPlans() {
		if p.Tier == tier && p.Cycle == cycle {
			return p, true
		}
	}
	return PlatformPlan{}, false
}

// platformPlanForBillingAmount resolves current prices and the previous Pro
// offer for delayed provider events. The legacy price is never returned by the
// public catalog or used to create a new checkout.
func platformPlanForBillingAmount(amountCents int64, cycle string) (PlatformPlan, bool) {
	for _, plan := range PlatformPlans() {
		if !plan.ContactOnly && plan.AmountCents == amountCents && plan.Cycle == cycle {
			return plan, true
		}
	}

	var legacyAmount int64
	switch cycle {
	case "MONTHLY":
		legacyAmount = legacyProMonthlyAmountCents
	case "ANNUALLY":
		legacyAmount = legacyProAnnualAmountCents
	default:
		return PlatformPlan{}, false
	}
	if amountCents != legacyAmount {
		return PlatformPlan{}, false
	}
	plan, ok := FindPlatformPlanCycle("pro", cycle)
	if !ok {
		return PlatformPlan{}, false
	}
	plan.AmountCents = amountCents
	return plan, true
}
