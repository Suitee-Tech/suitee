package contract

import (
	"errors"
	"math"
	"time"
)

// TerminationFeeBreakdown — saída detalhada da fórmula de multa
// proporcional decrescente. Issue #438. Expor o breakdown (em vez de
// só o valor) deixa o frontend mostrar:
//   - "Multa: R$ 1.500,00 (3x aluguel × 5/12 do prazo)"
//   - "Sem multa — contrato já cumprido (0 meses restantes)"
//
// Sem esse detalhe, o cliente fica sem como reconciliar o número.
type TerminationFeeBreakdown struct {
	// FeeCents — valor da multa em centavos. Sempre >= 0. Centavos para
	// evitar erro de ponto flutuante em soma de cobranças.
	FeeCents int64 `json:"fee_cents"`

	// MonthlyRentCents — aluguel mensal usado no cálculo.
	MonthlyRentCents int64 `json:"monthly_rent_cents"`

	// TotalMonths — duração total contratada.
	TotalMonths int `json:"total_months"`

	// MonthsElapsed — meses já cumpridos (corrigido para min(elapsed, total)).
	MonthsElapsed int `json:"months_elapsed"`

	// MonthsRemaining — meses restantes (TotalMonths - MonthsElapsed).
	MonthsRemaining int `json:"months_remaining"`

	// FractionRemaining — MonthsRemaining / TotalMonths, em [0, 1].
	FractionRemaining float64 `json:"fraction_remaining"`

	// CapApplied — true quando a fórmula bruta excederia o teto de 3
	// alugueis (Lei 8245/91 Art. 4) e foi clipada. Não acontece com a
	// fórmula linear pura, mas defendemos contra extensões futuras.
	CapApplied bool `json:"cap_applied"`

	// Reason — string curta para UI explicar zeros: "contract_already_ended",
	// "exempt_no_monthly_rent", etc. Vazia no caminho feliz.
	Reason string `json:"reason,omitempty"`
}

// ErrInvalidTerminationDate — termination_date fora do intervalo
// [StartDate, EndDate]. Caller decide tratar como 400 ou ajustar pra
// hoje. Antes retornávamos só FeeCents=0 silenciosamente, mas isso
// mascara bugs de UI (ex.: usuário escolheu data no calendar passada
// por engano) que ficaria invisível.
var ErrInvalidTerminationDate = errors.New("termination date outside contract window")

// CalculateEarlyTerminationFee implementa a fórmula canônica de multa
// proporcional decrescente da Lei 8245/91 Art. 4:
//
//	multa = 3 × aluguel_mensal × (meses_restantes / meses_totais)
//
// Características:
//   - Decrescente: quanto mais cumprido o contrato, menor a multa.
//   - Linear: cai proporcionalmente ao tempo (não em degraus).
//   - Teto: máximo de 3 alugueis no início; zero ao fim do prazo.
//   - Em meses inteiros: a contagem usa meses cheios (>= dia do mês de
//     início). Fração de mês não conta como mês cumprido — esse é o
//     comportamento conservador a favor do inquilino, padrão de mercado.
//
// Por que NÃO usar a fórmula de degraus (3-2-1 alugueis em faixas)?
//   - A Lei diz "proporcional" sem fixar degraus.
//   - Degraus criam descontinuidade: cumprir 1 dia a mais que a faixa
//     leva multa muito menor. Cliente questionável.
//   - Linear contínua é padrão judicial dominante (jurisprudência STJ).
//
// Por que centavos? Operações financeiras NUNCA devem usar float64 —
// (3 * 1500.00 / 12) com float pode dar 374.999999... que vira R$ 374,99
// em vez de R$ 375,00 dependendo do round. Centavos como int64 é exato.
func CalculateEarlyTerminationFee(monthlyRentCents int64, startDate, endDate, terminationDate time.Time) (TerminationFeeBreakdown, error) {
	out := TerminationFeeBreakdown{
		MonthlyRentCents: monthlyRentCents,
	}

	// Guard: aluguel zero/negativo — pode acontecer em contratos
	// "gratuitos" (cessão de uso, comodato). Multa = 0, sem erro.
	if monthlyRentCents <= 0 {
		out.Reason = "no_monthly_rent"
		return out, nil
	}

	// Guard: contrato sem prazo (open-ended) — não dá pra calcular
	// proporção. Retorna o teto de 3 alugueis (cláusula conservadora) +
	// reason explicando. Caller decide se aceita ou pede end_date.
	// Em locação urbana isso é raro, mas existe em contratos comerciais
	// de longuíssimo prazo.
	if endDate.Before(startDate) || endDate.Equal(startDate) {
		out.Reason = "no_end_date"
		out.FeeCents = monthlyRentCents * 3
		return out, nil
	}

	totalMonths := monthsBetween(startDate, endDate)
	if totalMonths <= 0 {
		// Caso extremo: start e end no mesmo mês. Sem janela pra
		// proporcionalizar. Trata como prazo de 1 mês p/ evitar div/zero.
		totalMonths = 1
	}
	out.TotalMonths = totalMonths

	// Guard: data de encerramento fora do intervalo. Reportamos erro
	// para o caller diferenciar de "0 meses restantes" legítimo (multa
	// zero porque o contrato venceu naturalmente).
	if terminationDate.Before(startDate) {
		return out, ErrInvalidTerminationDate
	}

	// Se a termination_date >= end_date, contrato já cumpriu o prazo —
	// multa zero, não é rescisão antecipada.
	if !terminationDate.Before(endDate) {
		out.MonthsElapsed = totalMonths
		out.MonthsRemaining = 0
		out.FractionRemaining = 0
		out.Reason = "contract_already_ended"
		return out, nil
	}

	elapsed := monthsBetween(startDate, terminationDate)
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed > totalMonths {
		elapsed = totalMonths
	}
	remaining := totalMonths - elapsed
	out.MonthsElapsed = elapsed
	out.MonthsRemaining = remaining
	out.FractionRemaining = float64(remaining) / float64(totalMonths)

	// Multa em centavos, calculada em inteiros até o último passo.
	// Fórmula: (3 * monthlyRentCents * remaining) / totalMonths
	// Ordem das operações importa: multiplicamos antes de dividir para
	// não perder precisão. Risco de overflow: 3 * monthlyRentCents * 600
	// (50 anos) = 1.8M * monthly. Mesmo com aluguel de R$ 1M (cents=1e8),
	// dá 1.8e14, dentro de int64 (~9.2e18). Seguro.
	numerator := int64(3) * monthlyRentCents * int64(remaining)
	fee := numerator / int64(totalMonths)

	// Teto legal: 3 aluguéis. A fórmula linear nunca excede isso (o
	// máximo é quando remaining = totalMonths, dando exatamente 3 *
	// monthlyRent), mas mantemos o cap defensivamente caso algum
	// caller chame com terminationDate < startDate via override.
	maxFee := monthlyRentCents * 3
	if fee > maxFee {
		fee = maxFee
		out.CapApplied = true
	}
	if fee < 0 {
		fee = 0
	}

	out.FeeCents = fee
	return out, nil
}

// monthsBetween retorna meses inteiros completos entre `from` e `to`.
// "Inteiro" = considerando o dia do mês — se to é antes do dia-de-from
// no mês corrente, conta menos um mês. Exemplo:
//
//	from = 2024-01-15, to = 2024-04-10 → 2 meses cheios (jan→fev, fev→mar)
//	from = 2024-01-15, to = 2024-04-20 → 3 meses cheios
//
// Essa semântica favorece o inquilino (conta menos meses cumpridos →
// multa maior) e é padrão jurídico em locação. Para diff exato em
// dias, callers usariam time.Sub() — aqui meses são a unidade.
func monthsBetween(from, to time.Time) int {
	if !to.After(from) {
		return 0
	}
	months := (to.Year()-from.Year())*12 + int(to.Month()-from.Month())
	// Ajuste de dia: se ainda não chegou no dia-do-mês de from, desconta 1.
	if to.Day() < from.Day() {
		months--
	}
	return months
}

// FeeCentsToFloat — helper para callers ainda em float (UI legacy,
// integração com módulos que armazenam value como float64). Idealmente
// não usar — converter o resto do código para centavos é melhor — mas
// existe pra simplificar a transição.
func FeeCentsToFloat(cents int64) float64 {
	return math.Round(float64(cents)) / 100.0
}

// FloatToCents — converte um valor float (BRL) para centavos int64.
// Usa Round para evitar problemas tipo 1.235 virando 1.234999... que
// truncaria para 123 em vez de 124.
func FloatToCents(v float64) int64 {
	return int64(math.Round(v * 100))
}
