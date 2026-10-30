package services

import (
	"context"
	"testing"
	"time"

	"github.com/oginoino/toca/pkg/domain/valueobjects"
	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

// A manutenção somada na próxima cobrança do inquilino é um item que NÃO
// pertence ao plano-base. Regravar o plano (reajuste, mudança de itens
// empacotados) substitui a composição inteira — se o item extra não for
// preservado, a obrigação continua no chamado mas desaparece do financeiro.
func TestRegenerateChargesPreservesNonPlanItems(t *testing.T) {
	cfg := readjustmentFixtureConfig()
	configRepo := &readjustmentConfigRepo{cfg: cfg}
	nextMonth := time.Date(2026, time.February, 5, 0, 0, 0, 0, time.UTC)

	rent := readjustmentFixtureCharge(t, cfg, time.February, charge.StatusPending)
	rent.DueDate = nextMonth
	// Plano: aluguel + taxa de administração.
	if err := rent.SetLineItems([]charge.LineItem{
		{
			Description:     "Aluguel",
			AmountCents:     90000,
			SourceType:      "rental_base",
			SourceID:        cfg.ID,
			EconomicOwnerID: cfg.OwnerID,
			AllocationType:  charge.AllocationTypeOwnerPayable,
		},
		{
			Description:     "Taxa de administração",
			AmountCents:     10000,
			SourceType:      "rental_bundled_item",
			SourceID:        "admin_fee",
			EconomicOwnerID: cfg.TenantID,
			AllocationType:  charge.AllocationTypeRevenue,
		},
	}); err != nil {
		t.Fatalf("composição inicial: %v", err)
	}
	// Manutenção somada depois, com o mesmo formato que o handler grava.
	maintenance := charge.LineItem{
		Description:     "Manutenção: Elétrica (#os-1) — reembolso à imobiliária",
		AmountCents:     13000,
		SourceType:      "maintenance",
		SourceID:        "os-1",
		EconomicOwnerID: cfg.TenantID,
		AllocationType:  charge.AllocationTypeRevenue,
	}
	rent.Amount = valueobjects.NewMoney(113000)
	rent.TotalAmount = rent.Amount
	if err := rent.SetLineItems(append(rent.LineItems, maintenance)); err != nil {
		t.Fatalf("composição com manutenção: %v", err)
	}

	chargeRepo := &readjustmentChargeRepo{charges: []*charge.Charge{rent}}
	svc := NewRentalChargeConfigService(configRepo, chargeRepo, nil)

	// effectiveFrom anterior ao vencimento para a cobrança entrar na regravação.
	effectiveFrom := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, err := svc.RegenerateChargesFrom(context.Background(), cfg, effectiveFrom); err != nil {
		t.Fatalf("RegenerateChargesFrom: %v", err)
	}

	if len(chargeRepo.charges) != 1 {
		t.Fatalf("cobranças = %d, want 1", len(chargeRepo.charges))
	}
	updated := chargeRepo.charges[0]

	var found *charge.LineItem
	for index := range updated.LineItems {
		if updated.LineItems[index].SourceType == "maintenance" && updated.LineItems[index].SourceID == "os-1" {
			found = &updated.LineItems[index]
		}
	}
	if found == nil {
		t.Fatalf("item de manutenção foi apagado na regravação do plano: %+v", updated.LineItems)
	}
	if found.AmountCents != 13000 {
		t.Fatalf("valor da manutenção = %d, want 13000", found.AmountCents)
	}
	// O total continua fechando com a soma dos itens.
	var sum int64
	for _, item := range updated.LineItems {
		sum += item.AmountCents
	}
	if got := updated.Amount.Cents(); got != sum {
		t.Fatalf("total da cobrança = %d, soma dos itens = %d", got, sum)
	}
}
