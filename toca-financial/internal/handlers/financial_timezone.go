package handlers

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/timeutil"
)

// FinancialDayLocation devolve o fuso usado para interpretar o dia civil dos
// relatórios financeiros.
//
// Antes não havia configuração alguma: os limites eram montados em UTC, então
// em São Paulo (UTC-3) um movimento às 22h do dia 9 pertencia ao dia 10 — o
// operador conciliava o próprio dia e o sistema mostrava outro.
//
// A fonte da verdade é `pkg/timeutil`: `APP_TIMEZONE` (nome IANA, padrão
// America/Sao_Paulo) com `FINANCIAL_TZ_OFFSET_MINUTES` como compatibilidade. A
// tzdata vai embarcada no binário, então fusos com horário de verão funcionam.
func FinancialDayLocation() *time.Location {
	return timeutil.Location()
}

// FinancialDayLocationFor devolve o fuso aplicável ao tenant da requisição: o
// que ele configurou (Configurações → "Fuso horário"), com o padrão do produto
// como fallback. É o que faz o campo da tela mandar no dia civil dos relatórios.
func FinancialDayLocationFor(ctx context.Context, tenantID string) *time.Location {
	return timeutil.LocationForTenant(ctx, tenantID)
}

var errInvertedRange = errors.New("'to' must be >= 'from'")

// resolveFinancialRange converte o par from/to da query nos instantes UTC que
// representam o período.
//
// Um limite em data-only significa o dia civil inteiro no fuso da empresa
// (00:00:00 até 23:59:59.999999 locais), não o dia UTC. Um limite em RFC3339 é
// um instante explícito e é preservado — quem informou o fuso não deve ter a
// intenção substituída.
//
// `to` data-only é inclusive; `from` data-only é o início do dia.
func resolveFinancialRange(fromStr, toStr string, loc *time.Location) (time.Time, time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	to := now

	if dateOnly(fromStr) {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(fromStr), loc)
		if err != nil {
			return from, to, err
		}
		from = parsed
	} else if p, err := parseReportDate(fromStr); err != nil {
		return from, to, err
	} else if p != nil {
		from = *p
	}

	if dateOnly(toStr) {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(toStr), loc)
		if err != nil {
			return from, to, err
		}
		// Último instante do dia civil local, com a precisão do Postgres.
		to = parsed.AddDate(0, 0, 1).Add(-time.Microsecond)
	} else if p, err := parseReportDate(toStr); err != nil {
		return from, to, err
	} else if p != nil {
		to = *p
	}

	if to.Before(from) {
		return from, to, errInvertedRange
	}
	return from, to, nil
}

// dateOnly reconhece "YYYY-MM-DD" sem hora. O teste é feito sobre a forma do
// valor, não sobre o resultado do parse, para que a regra não dependa de qual
// layout aceitou por acaso.
func dateOnly(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != len("2006-01-02") {
		return false
	}
	_, err := time.Parse("2006-01-02", trimmed)
	return err == nil
}
