// Package money formata valores monetários para exibição em texto.
//
// Existe porque a formatação estava espalhada: `fmt.Sprintf("R$ %.2f", v)`
// aparecia em e-mails, notificações e descrições de lançamento e produzia
// "R$ 6000.00" — separador de milhar inglês e ponto decimal — enquanto o
// produto inteiro (painel, portal, PDF de recibo) mostra "R$ 6.000,00". Uma
// única implementação evita que cada superfície nova reaprenda a errar.
package money

import (
	"math"
	"strconv"
	"strings"
)

// BRL formata em reais no padrão brasileiro: ponto como separador de milhar,
// vírgula como separador decimal e sempre dois centavos ("R$ 6.000,00").
//
// O arredondamento é feito em centavos (e não truncando o float), então valores
// como 1234.5 saem "R$ 1.234,50". O limite é o do próprio float: um literal como
// 1.005 é, em binário, 1.00499999… e arredonda para 1,00 — os valores do sistema
// vêm de centavos ou de entradas com duas casas, não de literais assim.
func BRL(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	total := int64(math.Round(math.Abs(v) * 100))
	return assemble(v < 0 && total != 0, strconv.FormatInt(total/100, 10), int(total%100), true)
}

// BRLWhole formata em reais sem centavos ("R$ 60.000"), para espaços onde o
// centavo é ruído — cartões compactos, títulos e resumos. Arredonda para o real
// mais próximo.
func BRLWhole(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		v = 0
	}
	total := int64(math.Round(math.Abs(v)))
	return assemble(v < 0 && total != 0, strconv.FormatInt(total, 10), 0, false)
}

func assemble(negative bool, integer string, cents int, withCents bool) string {
	var b strings.Builder
	b.WriteString("R$ ")
	if negative {
		b.WriteByte('-')
	}
	b.WriteString(groupThousands(integer))
	if withCents {
		b.WriteByte(',')
		if cents < 10 {
			b.WriteByte('0')
		}
		b.WriteString(strconv.Itoa(cents))
	}
	return b.String()
}

// groupThousands insere o ponto a cada três dígitos ("60000" → "60.000").
func groupThousands(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte('.')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}
