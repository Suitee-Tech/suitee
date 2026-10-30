package services

import (
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
	"github.com/oginoino/toca/toca-financial/internal/domain/rental_charge_config"
)

// TestBuildBundledLineItemsSetsEconomicOwnership garante que a dimensão de
// titularidade econômica (seção 9.4 do compêndio) seja preenchida nos itens
// gerados: aluguel/condomínio/IPTU liquidam para o dono do imóvel, enquanto a
// taxa de administração pertence à imobiliária (TenantID).
func TestBuildBundledLineItemsSetsEconomicOwnership(t *testing.T) {
	cfg := &rental_charge_config.RentalChargeConfig{
		ID:            "cfg-1",
		TenantID:      "imobiliaria-1",
		OwnerID:       "owner-1",
		MonthlyAmount: 500000,
		BundledItems: []rental_charge_config.BundledItem{
			{Key: "condominio", Description: "Condomínio", AmountCents: 70000, Enabled: true},
			{Key: "iptu", Description: "IPTU", AmountCents: 30000, Enabled: true},
			{Key: "admin_fee", Description: "Taxa de administração", AmountCents: 40000, Enabled: true},
			{Key: "seguro_fianca", Description: "Seguro fiança", AmountCents: 10000, Enabled: false},
		},
	}

	monthlyTotal := cfg.TotalMonthlyCents()
	items := buildBundledLineItems(cfg, monthlyTotal, monthlyTotal, 0)
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4 (base + 3 enabled)", len(items))
	}

	byDesc := make(map[string]charge.LineItem, len(items))
	for _, it := range items {
		byDesc[it.Description] = it
		if it.RuleVersionID == "" {
			t.Errorf("item %q lacks rule_version_id", it.Description)
		}
	}

	base := byDesc["Aluguel"]
	if base.EconomicOwnerID != "owner-1" || base.SettlementMode != charge.SettlementModeCollection {
		t.Errorf("base = owner %q mode %q, want owner-1/arrecadacao", base.EconomicOwnerID, base.SettlementMode)
	}

	cond := byDesc["Condomínio"]
	if cond.EconomicOwnerID != "owner-1" || cond.SettlementMode != charge.SettlementModeDirectLiquidation {
		t.Errorf("condominio = owner %q mode %q, want owner-1/liquidacao_direta", cond.EconomicOwnerID, cond.SettlementMode)
	}

	iptu := byDesc["IPTU"]
	if iptu.EconomicOwnerID != "owner-1" || iptu.SettlementMode != charge.SettlementModeDirectLiquidation {
		t.Errorf("iptu = owner %q mode %q, want owner-1/liquidacao_direta", iptu.EconomicOwnerID, iptu.SettlementMode)
	}

	admin := byDesc["Taxa de administração"]
	if admin.EconomicOwnerID != "imobiliaria-1" || admin.SettlementMode != charge.SettlementModeCollection {
		t.Errorf("admin_fee = owner %q mode %q, want imobiliaria-1/arrecadacao", admin.EconomicOwnerID, admin.SettlementMode)
	}

	if _, ok := byDesc["Seguro fiança"]; ok {
		t.Errorf("disabled bundled item should not be emitted")
	}
}

func TestBuildBundledLineItemsEmitsEconomicBaseWithoutExtras(t *testing.T) {
	cfg := &rental_charge_config.RentalChargeConfig{
		ID:                     "cfg-base",
		TenantID:               "imobiliaria-1",
		OwnerID:                "owner-1",
		MonthlyAmount:          500000,
		MonthlyProductRevision: 3,
	}
	items := buildBundledLineItems(cfg, 500000, 500000, 0)
	if len(items) != 1 {
		t.Fatalf("items = %d, want the typed rental base", len(items))
	}
	base := items[0]
	if base.SourceType != "rental_base" || base.SourceID != cfg.ID || base.AmountCents != 500000 ||
		base.EconomicOwnerID != cfg.OwnerID || base.SettlementMode != charge.SettlementModeCollection || base.RuleVersionID == "" {
		t.Fatalf("unexpected base item: %+v", base)
	}
	if got := rentalChargeRuleVersionID(cfg); got != base.RuleVersionID {
		t.Fatalf("rule version = %q, want %q", base.RuleVersionID, got)
	}
}

func TestBuildBundledLineItemsSplitsAdminFeeToImobiliaria(t *testing.T) {
	cfg := &rental_charge_config.RentalChargeConfig{
		ID:                     "cfg-admin",
		TenantID:               "imobiliaria-1",
		OwnerID:                "owner-1",
		MonthlyAmount:          500000, // R$ 5.000
		MonthlyProductRevision: 1,
		AdminFeePercent:        8, // 8%
	}
	// Sem extras: aluguel = R$5.000; admin = R$400 (imobiliária); owner = R$4.600.
	items := buildBundledLineItems(cfg, 500000, 500000, 8)
	if len(items) != 2 {
		t.Fatalf("items = %d, want [Aluguel, Taxa de administração]", len(items))
	}
	var ownerRent, admin int64
	var adminAllocationType charge.AllocationType
	for _, it := range items {
		switch it.Description {
		case "Aluguel":
			ownerRent = it.AmountCents
		case "Taxa de administração":
			admin = it.AmountCents
			adminAllocationType = it.AllocationType
		}
	}
	if ownerRent != 460000 || admin != 40000 {
		t.Fatalf("owner=%d admin=%d, want 460000/40000", ownerRent, admin)
	}
	if ownerRent+admin != 500000 {
		t.Fatalf("split does not preserve charge total: %d+%d != 500000", ownerRent, admin)
	}
	if adminAllocationType != charge.AllocationTypeRevenue {
		t.Fatalf("admin allocation_type = %q, want revenue (receita da imobiliária)", adminAllocationType)
	}

	// Com condomínio/IPTU: o admin escala pro-rata e não muda o total.
	cfg2 := &rental_charge_config.RentalChargeConfig{
		ID: "cfg2", TenantID: "imobiliaria-1", OwnerID: "owner-1",
		MonthlyAmount: 500000, MonthlyProductRevision: 1, AdminFeePercent: 8,
		BundledItems: []rental_charge_config.BundledItem{
			{Key: "condominio", Description: "Condomínio", AmountCents: 70000, Enabled: true},
			{Key: "iptu", Description: "IPTU", AmountCents: 30000, Enabled: true},
		},
	}
	items2 := buildBundledLineItems(cfg2, 600000, 600000, 8)
	var total int64
	for _, it := range items2 {
		total += it.AmountCents
	}
	if total != 600000 {
		t.Fatalf("total = %d, want 600000 (total da cobrança inalterado)", total)
	}
}

func TestValidateLineItemsRejectsUnknownSettlementMode(t *testing.T) {
	err := charge.ValidateLineItems([]charge.LineItem{
		{Description: "Aluguel", AmountCents: 100, SettlementMode: charge.SettlementMode("foo")},
	}, 100)
	if err == nil {
		t.Fatal("expected error for unknown settlement_mode")
	}
}
