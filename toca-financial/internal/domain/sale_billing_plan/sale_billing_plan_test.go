package sale_billing_plan

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCompositeInputJSONUsesSnakeCase(t *testing.T) {
	t.Parallel()
	due := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	input := CompositeInput{
		DacaoAmountCents: 100000,
		DacaoDescription: "Imóvel em dação",
		TrocaAmountCents: 200000,
		TrocaDescription: "Veículo em troca",
		SaldoAmountCents: 300000,
		SaldoDueDate:     &due,
		SaldoDescription: "Saldo final",
	}

	payload, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal composite input: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal composite JSON: %v", err)
	}
	for _, key := range []string{
		"dacao_amount_cents", "dacao_description", "troca_amount_cents",
		"troca_description", "saldo_amount_cents", "saldo_due_date", "saldo_description",
	} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing snake_case JSON key %q in %s", key, payload)
		}
	}
	if _, ok := got["DacaoAmountCents"]; ok {
		t.Fatalf("unexpected Go field name in JSON: %s", payload)
	}
}

func TestNew_SingleMode(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	plan, err := New("tenant-1", "contract-1", "deal-1", "buyer-1", "owner-1", "prop-1", ModeSingle, 100000, 0, 100000, 1, due, 30, CompositeInput{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if plan.Mode != ModeSingle {
		t.Fatalf("expected ModeSingle, got %s", plan.Mode)
	}
	if plan.InstallmentCount != 1 {
		t.Fatalf("expected 1 installment, got %d", plan.InstallmentCount)
	}
}

func TestNew_SingleMode_RejectsWrongCount(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err := New("t", "c", "", "", "", "", ModeSingle, 100000, 0, 100000, 5, due, 30, CompositeInput{})
	if err == nil {
		t.Fatal("expected error for SINGLE with installment_count != 1")
	}
}

func TestNew_Installments_MatchSum(t *testing.T) {
	t.Parallel()
	// 100k total, 10k down, 12 installments of 7500 = 100k. Exact.
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	plan, err := New("t", "c", "", "", "", "", ModeInstallments, 100000, 10000, 7500, 12, due, 30, CompositeInput{})
	if err != nil {
		t.Fatalf("expected exact sum to be accepted, got %v", err)
	}
	if !plan.HasDownPayment() {
		t.Fatal("expected HasDownPayment to be true")
	}
}

func TestNew_Installments_ToleratesCentRounding(t *testing.T) {
	t.Parallel()
	// 99999 total, no down, 3 installments of 33333 = 99999. Exact.
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err := New("t", "c", "", "", "", "", ModeInstallments, 99999, 0, 33333, 3, due, 30, CompositeInput{})
	if err != nil {
		t.Fatalf("expected exact 3-installment match, got %v", err)
	}
	// 100000 / 3 = 33333.33, rounded 33333 — sum 99999, delta 1, tolerance count=3 → accepted.
	_, err = New("t", "c", "", "", "", "", ModeInstallments, 100000, 0, 33333, 3, due, 30, CompositeInput{})
	if err != nil {
		t.Fatalf("expected 1-cent residual to be tolerated, got %v", err)
	}
}

func TestNew_Installments_RejectsLargeDelta(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// 100000 total, 10 installments of 9000 = 90000 — 10k off, way past tolerance.
	_, err := New("t", "c", "", "", "", "", ModeInstallments, 100000, 0, 9000, 10, due, 30, CompositeInput{})
	if err == nil {
		t.Fatal("expected error when installments don't sum to total")
	}
}

func TestNew_DefaultsRecurrenceTo30Days(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	plan, _ := New("t", "c", "", "", "", "", ModeSingle, 1000, 0, 1000, 1, due, 0, CompositeInput{})
	if plan.RecurrenceIntervalDays != 30 {
		t.Fatalf("expected default recurrence 30, got %d", plan.RecurrenceIntervalDays)
	}
}

// Composite leg coverage — entrada + parcelas + dação + troca + saldo == total.
func TestNew_Composite_DacaoAndTroca(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saldoDue := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	// Imóvel de R$ 500k: 50k entrada + 24 × 7.500 (= 180k) + 100k dação +
	// 120k troca + 50k saldo = 500k. Cabe na tolerância.
	plan, err := New("t", "c", "", "", "", "", ModeInstallments,
		500000_00, 50000_00, 7500_00, 24, due, 30,
		CompositeInput{
			DacaoAmountCents: 100000_00,
			DacaoDescription: "Apartamento na Praia Grande",
			TrocaAmountCents: 120000_00,
			TrocaDescription: "Casa em Sorocaba",
			SaldoAmountCents: 50000_00,
			SaldoDueDate:     &saldoDue,
			SaldoDescription: "Saldo pós-financiamento",
		},
	)
	if err != nil {
		t.Fatalf("expected composite to validate, got %v", err)
	}
	if !plan.HasSaldo() {
		t.Fatal("expected HasSaldo to be true")
	}
	if !plan.HasNonCashLegs() {
		t.Fatal("expected HasNonCashLegs to be true")
	}
	if plan.SaldoDueDate == nil {
		t.Fatal("saldo_due_date should be persisted on the plan")
	}
}

func TestNew_Composite_SaldoRequiresDueDate(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	_, err := New("t", "c", "", "", "", "", ModeInstallments,
		100000_00, 0, 5000_00, 10, due, 30,
		CompositeInput{SaldoAmountCents: 50000_00}, // missing SaldoDueDate
	)
	if err == nil {
		t.Fatal("expected error when saldo > 0 but saldo_due_date is nil")
	}
}

func TestNew_Composite_LegsMustSumToTotal(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	saldoDue := time.Date(2027, 6, 1, 0, 0, 0, 0, time.UTC)
	// 500k total mas entrada 50k + 24×5000 (120k) + 100k dação + 50k saldo
	// = 320k. Faltam 180k → erro.
	_, err := New("t", "c", "", "", "", "", ModeInstallments,
		500000_00, 50000_00, 5000_00, 24, due, 30,
		CompositeInput{
			DacaoAmountCents: 100000_00,
			SaldoAmountCents: 50000_00,
			SaldoDueDate:     &saldoDue,
		},
	)
	if err == nil {
		t.Fatal("expected error when composite legs underflow the total")
	}
}

func TestNew_Composite_PureDacaoTroca(t *testing.T) {
	t.Parallel()
	due := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Permuta pura: 300k total = 200k dação + 100k troca. Sem charges.
	plan, err := New("t", "c", "", "", "", "", ModeSingle,
		300000_00, 0, 0, 1, due, 30,
		CompositeInput{
			DacaoAmountCents: 200000_00,
			TrocaAmountCents: 100000_00,
		},
	)
	if err != nil {
		t.Fatalf("expected pure dação/troca to validate, got %v", err)
	}
	if plan.HasDownPayment() {
		t.Fatal("pure dação/troca should not have down payment")
	}
}
