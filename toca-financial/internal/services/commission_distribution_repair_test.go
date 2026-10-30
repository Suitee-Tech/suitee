package services

import (
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/commission"
	"github.com/oginoino/toca/toca-financial/internal/domain/journal"
)

// O lastro de caixa da obrigação cancelada é reaplicado à fatia nova na razão
// entre os percentuais: é o que mantém "quem pagou a comissão" rastreável
// depois de corrigir o rateio de uma venda.
func TestScaleFundingEntries(t *testing.T) {
	at := time.Date(2026, 10, 1, 15, 0, 0, 0, time.UTC)
	original := &commission.Commission{
		Percentage: 100,
		FundingEntries: []commission.FundingEntry{
			{PaymentID: "p1", ChargeID: "c1", ReferenceMonth: "2026-10", AmountCents: 2400000, FundedAt: at},
			{PaymentID: "p2", ChargeID: "c2", ReferenceMonth: "2026-11", AmountCents: 6000000, FundedAt: at},
		},
	}

	captador := scaleFundingEntries(original, 20)
	if len(captador) != 2 {
		t.Fatalf("esperava 2 entradas de lastro, veio %d", len(captador))
	}
	if captador[0].AmountCents != 480000 || captador[1].AmountCents != 1200000 {
		t.Errorf("lastro do captador 20%% incorreto: %+v", captador)
	}
	if captador[0].PaymentID != "p1" || captador[0].ChargeID != "c1" || captador[0].ReferenceMonth != "2026-10" {
		t.Errorf("identidade do recebimento deve ser preservada: %+v", captador[0])
	}

	fechamento := scaleFundingEntries(original, 40)
	var total int64
	for _, entry := range fechamento {
		total += entry.AmountCents
	}
	if fechamento[0].AmountCents != 960000 || fechamento[1].AmountCents != 2400000 || total != 3360000 {
		t.Errorf("lastro do fechamento 40%% incorreto: %+v (total %d)", fechamento, total)
	}
}

func TestScaleFundingEntriesSemLastro(t *testing.T) {
	if got := scaleFundingEntries(&commission.Commission{Percentage: 100}, 20); got != nil {
		t.Errorf("comissão sem entradas de lastro não deve gerar lastro: %+v", got)
	}
	if got := scaleFundingEntries(nil, 20); got != nil {
		t.Errorf("comissão ausente não deve gerar lastro: %+v", got)
	}
	if got := scaleFundingEntries(&commission.Commission{Percentage: 0}, 20); got != nil {
		t.Errorf("sem percentual original não há razão para escalar: %+v", got)
	}
}

func TestHasCommissionAccrualLeg(t *testing.T) {
	legs := []journal.Leg{
		{Account: journal.AccountCommissionExpense, Direction: journal.Debit, AmountCents: 8400000},
		{Account: journal.AccountCommissionPayable, Direction: journal.Credit, AmountCents: 8400000},
	}
	if !hasCommissionAccrualLeg(legs, 8400000) {
		t.Error("perna de débito em commission_expense com o valor da comissão deve ser reconhecida")
	}
	if hasCommissionAccrualLeg(legs, 1680000) {
		t.Error("valor diferente não identifica o accrual daquela comissão")
	}
}
