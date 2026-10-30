package services

import (
	"bytes"
	"fmt"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
)

// RenderInformePDF builds a one-page "Informe de Rendimentos" — o
// documento que o proprietário usa na declaração de Imposto de Renda,
// com os totais anuais de aluguel bruto, taxa de administração e valor
// líquido repassado. Uses gofpdf (pure Go, no system fonts) — mesmo
// motor do demonstrativo de repasse, builda em qualquer host.
func RenderInformePDF(inf *fiscal.Informe) ([]byte, error) {
	if inf == nil {
		return nil, fmt.Errorf("informe is required")
	}

	pdf := newPDFDoc()
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(31, 41, 55) // slate-800
	pdf.CellFormat(0, 10, "Informe de Rendimentos", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 116, 139) // slate-500
	pdf.CellFormat(0, 6, fmt.Sprintf("Ano-calendário %d — emitido em %s", inf.Year, time.Now().Format("02/01/2006")), "", 1, "L", false, 0, "")
	pdf.Ln(4)

	// Reference block
	pdf.SetDrawColor(226, 232, 240) // slate-200
	pdf.SetLineWidth(0.2)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	kv := func(label, value string) {
		if value == "" {
			value = "—"
		}
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(45, 6, label, "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "B", 10)
		pdf.SetTextColor(31, 41, 55)
		pdf.CellFormat(0, 6, value, "", 1, "L", false, 0, "")
	}

	kv("Proprietário:", firstNonEmpty(inf.OwnerName, inf.OwnerID))
	kv("Ano-calendário:", fmt.Sprintf("%d", inf.Year))
	if inf.GeneratedAt != nil {
		kv("Gerado em:", inf.GeneratedAt.Format("02/01/2006"))
	}

	pdf.Ln(4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(6)

	// Breakdown block
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(31, 41, 55)
	pdf.CellFormat(0, 8, "Rendimentos do Ano", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	row := func(label string, amount float64, negative bool) {
		pdf.SetFont("Arial", "", 11)
		pdf.SetTextColor(31, 41, 55)
		pdf.CellFormat(120, 7, label, "", 0, "L", false, 0, "")
		text := formatBRL(amount)
		if negative {
			text = "-" + text
			pdf.SetTextColor(220, 38, 38) // red-600
		}
		pdf.CellFormat(0, 7, text, "", 1, "R", false, 0, "")
	}

	section := func(title string) {
		pdf.Ln(2)
		pdf.SetFont("Arial", "B", 11)
		pdf.SetTextColor(100, 116, 139) // slate-500
		pdf.CellFormat(0, 7, title, "", 1, "L", false, 0, "")
	}

	// T-06: locação e venda têm naturezas distintas na declaração do
	// proprietário — renda de aluguel (tributável no ajuste/carnê-leão) e
	// ganho de capital (GCAP). O documento separa os fluxos para não somar
	// repasse de venda ao rendimento de locação.
	hasRental := inf.RentalCount > 0 || inf.RentalGross > 0
	hasSale := inf.SaleCount > 0 || inf.SaleGross > 0

	if hasRental || hasSale {
		if hasRental {
			section("Rendimentos de Locação")
			row("Aluguel Bruto Recebido", inf.RentalGross, false)
			// T-05: linha de intermediação imobiliária — a comissão retida dos
			// repasses explica, junto com a taxa de administração, o líquido
			// menor que o bruto. Aparece mesmo zerada para fechar a equação.
			row("Taxa de Administração", inf.RentalAdminFee, true)
			row("Intermediação de Locação", inf.RentalIntermediation, true)
			row("Líquido de Locação", inf.RentalNet, false)
		}
		if hasSale {
			section("Vendas do Ano (repasse ao proprietário)")
			row("Valor das Vendas", inf.SaleGross, false)
			row("Comissão de Intermediação de Venda", inf.SaleIntermediation, true)
			row("Líquido das Vendas", inf.SaleNet, false)
			pdf.Ln(1)
			pdf.SetFont("Arial", "I", 8)
			pdf.SetTextColor(148, 163, 184)
			pdf.MultiCell(0, 4, "O valor das vendas é repasse de capital (ganho de capital — GCAP), não rendimento de locação. A comissão de intermediação é receita da imobiliária retida do repasse.", "", "L", false)
		}
	} else {
		// Informes antigos (sem separação por fluxo gravada).
		row("Aluguel Bruto Recebido", inf.TotalGross, false)
		row("Taxa de Administração", inf.TotalAdminFee, true)
		row("Intermediação Imobiliária", inf.TotalIntermediation, true)
	}

	pdf.Ln(2)
	pdf.SetDrawColor(31, 41, 55)
	pdf.SetLineWidth(0.4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 13)
	pdf.SetTextColor(15, 118, 110) // teal-700
	pdf.CellFormat(120, 9, "Valor Líquido Repassado no Ano", "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 9, formatBRL(inf.TotalNet), "", 1, "R", false, 0, "")

	// Footer fixed at the bottom.
	pdf.SetY(-30)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(148, 163, 184) // slate-400
	pdf.MultiCell(0, 4, "Documento informativo dos valores administrados no período, separados por locação e venda. Rendimentos de locação entram na declaração de ajuste; o valor das vendas é repasse de capital (GCAP). Confira os valores com seu contador antes de utilizá-los.", "", "L", false)
	pdf.CellFormat(0, 4, fmt.Sprintf("Informe ID: %s", inf.ID), "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
