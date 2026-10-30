package handlers

import (
	"github.com/oginoino/toca/pkg/timeutil"

	"testing"
	"time"
)

// FIN-11: o dia financeiro era o dia civil UTC. Em São Paulo (UTC-3) um
// movimento às 22h do dia 9 pertence ao dia 10 em UTC, então o operador
// conciliava o próprio dia e o relatório mostrava outro.
func TestFinancialDayFollowsCompanyTimezone(t *testing.T) {
	loc := FinancialDayLocation()
	from, to, err := resolveFinancialRange("2026-09-09", "2026-09-09", loc)
	if err != nil {
		t.Fatal(err)
	}

	// 22h de 09/09 no horário local é 01h de 10/09 em UTC: precisa entrar.
	lateEvening := time.Date(2026, 9, 9, 22, 0, 0, 0, loc)
	if lateEvening.Before(from) || lateEvening.After(to) {
		t.Fatalf("movimento das 22h locais ficou fora de %s - %s", from, to)
	}
	// E 01h de 10/09 local (04h UTC) precisa ficar fora.
	nextDayEarly := time.Date(2026, 9, 10, 1, 0, 0, 0, loc)
	if !nextDayEarly.After(to) {
		t.Fatalf("movimento do dia seguinte entrou: to=%s inclui %s", to, nextDayEarly)
	}
	// O dia inteiro do operador está contido no intervalo.
	if !from.Equal(time.Date(2026, 9, 9, 0, 0, 0, 0, loc)) {
		t.Fatalf("from = %s, want início do dia local", from)
	}
	if !to.Equal(time.Date(2026, 9, 9, 23, 59, 59, 999999000, loc)) {
		t.Fatalf("to = %s, want fim do dia local", to)
	}
}

// Um instante explícito traz o próprio fuso e não deve ser reinterpretado.
func TestFinancialRangePreservesExplicitInstant(t *testing.T) {
	loc := FinancialDayLocation()
	_, to, err := resolveFinancialRange("2026-09-09", "2026-09-09T12:00:00-03:00", loc)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 9, 9, 15, 0, 0, 0, time.UTC); !to.Equal(want) {
		t.Fatalf("to = %s, want %s (instante explícito preservado)", to.UTC(), want)
	}
}

func TestFinancialRangeRejectsInvertedRange(t *testing.T) {
	loc := FinancialDayLocation()
	if _, _, err := resolveFinancialRange("2026-09-10", "2026-09-09", loc); err == nil {
		t.Fatal("intervalo invertido deveria falhar")
	}
}

// O deslocamento é configurável para não ficar preso a um fuso no código.
// (O NOME da zona é detalhe da implementação — a asserção é sobre o
// comportamento: com deslocamento zero o dia civil volta a coincidir com UTC.)
func TestFinancialDayLocationIsConfigurable(t *testing.T) {
	t.Setenv("APP_TIMEZONE", "")
	t.Setenv("FINANCIAL_TZ_OFFSET_MINUTES", "0")
	// Location() é resolvida uma vez por processo: sem recarregar aqui, o valor
	// já cacheado pelos outros testes venceria o ambiente deste.
	timeutil.ReloadLocation()
	t.Cleanup(func() {
		timeutil.ReloadLocation()
	})
	if _, offset := time.Now().In(FinancialDayLocation()).Zone(); offset != 0 {
		t.Fatalf("offset do fuso = %d, want 0", offset)
	}
	_, to, err := resolveFinancialRange("2026-09-09", "2026-09-09", FinancialDayLocation())
	if err != nil {
		t.Fatal(err)
	}
	// Com deslocamento zero o dia volta a ser o dia UTC.
	if want := time.Date(2026, 9, 9, 23, 59, 59, 999999000, time.UTC); !to.Equal(want) {
		t.Fatalf("to = %s, want %s", to.UTC(), want)
	}
}
