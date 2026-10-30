package contract

import (
	"errors"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestCalculateEarlyTerminationFee_HappyPath_Midway(t *testing.T) {
	// Contrato de 12 meses, encerrar no mês 6. Restam 6 meses.
	// Fórmula: 3 × 1500 × (6/12) = 2250
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2026-07-15") // 6 meses depois

	b, err := CalculateEarlyTerminationFee(150000, start, end, terminate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.TotalMonths != 12 {
		t.Errorf("TotalMonths = %d, want 12", b.TotalMonths)
	}
	if b.MonthsElapsed != 6 {
		t.Errorf("MonthsElapsed = %d, want 6", b.MonthsElapsed)
	}
	if b.MonthsRemaining != 6 {
		t.Errorf("MonthsRemaining = %d, want 6", b.MonthsRemaining)
	}
	if b.FeeCents != 225000 {
		t.Errorf("FeeCents = %d, want 225000 (R$ 2.250,00)", b.FeeCents)
	}
	if b.CapApplied {
		t.Error("CapApplied should be false at midway")
	}
}

func TestCalculateEarlyTerminationFee_StartOfContract_MaxFee(t *testing.T) {
	// Sair no dia 1 do contrato: paga 3 alugueis cheios.
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2026-01-15") // mesmo dia do start

	b, err := CalculateEarlyTerminationFee(150000, start, end, terminate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// MonthsElapsed = 0 → remaining = 12 → fee = 3 * 1500 * 12/12 = 4500
	if b.FeeCents != 450000 {
		t.Errorf("FeeCents = %d, want 450000 (3 alugueis cheios)", b.FeeCents)
	}
	if b.MonthsRemaining != 12 {
		t.Errorf("MonthsRemaining = %d, want 12", b.MonthsRemaining)
	}
}

func TestCalculateEarlyTerminationFee_AtEndOfContract_NoFee(t *testing.T) {
	// terminate >= end_date → contrato cumpriu → multa zero.
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2027-01-15")

	b, err := CalculateEarlyTerminationFee(150000, start, end, terminate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.FeeCents != 0 {
		t.Errorf("FeeCents = %d, want 0 at end of contract", b.FeeCents)
	}
	if b.Reason != "contract_already_ended" {
		t.Errorf("Reason = %q, want contract_already_ended", b.Reason)
	}
}

func TestCalculateEarlyTerminationFee_AfterEndOfContract_NoFee(t *testing.T) {
	// terminate > end_date também = sem multa.
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2027-06-15")

	b, _ := CalculateEarlyTerminationFee(150000, start, end, terminate)
	if b.FeeCents != 0 {
		t.Errorf("FeeCents = %d, want 0 past end", b.FeeCents)
	}
}

func TestCalculateEarlyTerminationFee_BeforeStart_Error(t *testing.T) {
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2025-12-01") // antes do start
	_, err := CalculateEarlyTerminationFee(150000, start, end, terminate)
	if !errors.Is(err, ErrInvalidTerminationDate) {
		t.Errorf("expected ErrInvalidTerminationDate, got %v", err)
	}
}

func TestCalculateEarlyTerminationFee_ZeroRent_NoFee(t *testing.T) {
	// Contrato de comodato (aluguel zero) — multa zero.
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2026-07-15")
	b, err := CalculateEarlyTerminationFee(0, start, end, terminate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if b.FeeCents != 0 {
		t.Errorf("FeeCents = %d, want 0 for zero rent", b.FeeCents)
	}
	if b.Reason != "no_monthly_rent" {
		t.Errorf("Reason = %q, want no_monthly_rent", b.Reason)
	}
}

func TestCalculateEarlyTerminationFee_NegativeRent_NoFee(t *testing.T) {
	// Defensivo: caller passou aluguel negativo (bug upstream).
	b, _ := CalculateEarlyTerminationFee(-10000, date("2026-01-15"), date("2027-01-15"), date("2026-07-15"))
	if b.FeeCents != 0 {
		t.Errorf("FeeCents = %d, want 0 for negative rent (defensive)", b.FeeCents)
	}
}

func TestCalculateEarlyTerminationFee_OpenEnded_ReturnsMaxWithReason(t *testing.T) {
	// EndDate == StartDate (contrato sem prazo definido). Multa = 3
	// alugueis com reason="no_end_date" para o UI exibir aviso.
	t1 := date("2026-01-15")
	b, _ := CalculateEarlyTerminationFee(150000, t1, t1, t1)
	if b.FeeCents != 450000 {
		t.Errorf("FeeCents = %d, want 450000 (3 alugueis)", b.FeeCents)
	}
	if b.Reason != "no_end_date" {
		t.Errorf("Reason = %q, want no_end_date", b.Reason)
	}
}

func TestCalculateEarlyTerminationFee_TwoYearContract_Quarter(t *testing.T) {
	// Contrato de 24 meses, sair com 18 meses restantes (após 6 meses).
	// Fórmula: 3 × 2000 × (18/24) = 4500
	start := date("2026-01-01")
	end := date("2028-01-01")
	terminate := date("2026-07-01")

	b, _ := CalculateEarlyTerminationFee(200000, start, end, terminate)
	if b.TotalMonths != 24 {
		t.Errorf("TotalMonths = %d, want 24", b.TotalMonths)
	}
	if b.MonthsElapsed != 6 {
		t.Errorf("MonthsElapsed = %d, want 6", b.MonthsElapsed)
	}
	if b.FeeCents != 450000 {
		t.Errorf("FeeCents = %d, want 450000", b.FeeCents)
	}
}

func TestCalculateEarlyTerminationFee_FractionalMonthRoundDown(t *testing.T) {
	// Saída no dia 14 quando start é dia 15 → ainda não completou o mês.
	// Contrato 12 meses, sair 5 meses + 29 dias depois = 5 meses cheios.
	start := date("2026-01-15")
	end := date("2027-01-15")
	terminate := date("2026-07-14") // 14 < 15, faltou 1 dia
	b, _ := CalculateEarlyTerminationFee(150000, start, end, terminate)
	if b.MonthsElapsed != 5 {
		t.Errorf("MonthsElapsed = %d, want 5 (faltou 1 dia pro 6º mês)", b.MonthsElapsed)
	}
	if b.MonthsRemaining != 7 {
		t.Errorf("MonthsRemaining = %d, want 7", b.MonthsRemaining)
	}
	// Fee = 3 * 1500 * 7/12 = 2625
	if b.FeeCents != 262500 {
		t.Errorf("FeeCents = %d, want 262500", b.FeeCents)
	}
}

func TestCalculateEarlyTerminationFee_MonotonicallyDecreasing(t *testing.T) {
	// Property test: a cada mês que passa, a multa só pode CAIR.
	start := date("2026-01-01")
	end := date("2027-01-01")
	monthly := int64(150000)

	var prevFee int64 = -1
	for m := 0; m <= 12; m++ {
		terminate := start.AddDate(0, m, 0)
		b, _ := CalculateEarlyTerminationFee(monthly, start, end, terminate)
		if prevFee >= 0 && b.FeeCents > prevFee {
			t.Errorf("month %d: fee %d > prev %d — fórmula deveria ser decrescente", m, b.FeeCents, prevFee)
		}
		prevFee = b.FeeCents
	}
}

func TestCalculateEarlyTerminationFee_NeverExceedsThreeMonths(t *testing.T) {
	// Property: nunca passa do teto de 3 alugueis, em nenhum cenário.
	start := date("2026-01-01")
	end := date("2027-01-01")
	monthly := int64(150000)
	cap := monthly * 3

	for m := -2; m <= 14; m++ {
		terminate := start.AddDate(0, m, 0)
		b, _ := CalculateEarlyTerminationFee(monthly, start, end, terminate)
		if b.FeeCents > cap {
			t.Errorf("month %d: fee %d > cap %d (3 alugueis)", m, b.FeeCents, cap)
		}
	}
}

func TestMonthsBetween(t *testing.T) {
	cases := []struct {
		name string
		from string
		to   string
		want int
	}{
		{"same day", "2026-01-15", "2026-01-15", 0},
		{"exact 1 month", "2026-01-15", "2026-02-15", 1},
		{"1 month minus 1 day", "2026-01-15", "2026-02-14", 0},
		{"1 month plus 1 day", "2026-01-15", "2026-02-16", 1},
		{"6 months", "2026-01-15", "2026-07-15", 6},
		{"12 months", "2026-01-15", "2027-01-15", 12},
		{"24 months", "2026-01-15", "2028-01-15", 24},
		{"backward returns 0", "2026-07-15", "2026-01-15", 0},
		{"different year diff month", "2026-12-15", "2027-02-15", 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := monthsBetween(date(tc.from), date(tc.to))
			if got != tc.want {
				t.Errorf("monthsBetween(%s, %s) = %d, want %d", tc.from, tc.to, got, tc.want)
			}
		})
	}
}

func TestFloatCentsConversion_Roundtrip(t *testing.T) {
	cases := []float64{
		0.00, 0.01, 1.00, 1.50, 1500.00, 1500.50, 1234.56,
		999999.99,
	}
	for _, v := range cases {
		cents := FloatToCents(v)
		back := FeeCentsToFloat(cents)
		if back != v {
			t.Errorf("roundtrip %.2f → %d → %.2f", v, cents, back)
		}
	}
}

func TestFloatToCents_RoundsHalfAwayFromZero(t *testing.T) {
	// Edge case clássico: 1.235 vira 124 cents, não 123 (truncate).
	if got := FloatToCents(1.235); got != 124 {
		t.Errorf("FloatToCents(1.235) = %d, want 124 (rounded)", got)
	}
}

func TestCalculateEarlyTerminationFee_TypicalScenario(t *testing.T) {
	// Cenário real de mercado: contrato 30 meses (CRECI padrão), sair
	// com 12 meses cumpridos, aluguel R$ 2.500.
	// Esperado: 3 × 2500 × 18/30 = 4500
	start := date("2026-01-01")
	end := date("2028-07-01")
	terminate := date("2027-01-01")

	b, _ := CalculateEarlyTerminationFee(250000, start, end, terminate)
	if b.TotalMonths != 30 {
		t.Errorf("TotalMonths = %d, want 30", b.TotalMonths)
	}
	if b.MonthsElapsed != 12 {
		t.Errorf("MonthsElapsed = %d, want 12", b.MonthsElapsed)
	}
	if b.MonthsRemaining != 18 {
		t.Errorf("MonthsRemaining = %d, want 18", b.MonthsRemaining)
	}
	if b.FeeCents != 450000 {
		t.Errorf("FeeCents = %d, want 450000 (R$ 4.500,00)", b.FeeCents)
	}
	if b.FractionRemaining != 0.6 {
		t.Errorf("FractionRemaining = %f, want 0.6", b.FractionRemaining)
	}
}
