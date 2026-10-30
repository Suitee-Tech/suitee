package services

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/pkg/money"
	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

// TransferReceiptContext bundles the data needed to render a "Demonstrativo
// de Repasse" PDF. Fields beyond the transfer itself (owner/tenant names,
// property title) are optional — the renderer falls back to the IDs when
// names aren't supplied, so the endpoint can short-circuit lookups for
// performance without breaking the PDF output.
type TransferReceiptContext struct {
	Transfer      *transfer.OwnerTransfer
	OwnerName     string
	OwnerDocument string
	TenantName    string
	PropertyTitle string
	OrgName       string
}

// RenderTransferReceiptPDF builds a one-page receipt with a header, contract
// references, the breakdown (aluguel bruto → taxa admin → deduções → líquido)
// and a footer with the issue date. Uses gofpdf (pure Go, no system fonts) so
// it builds on any host without extra setup.
func RenderTransferReceiptPDF(ctx TransferReceiptContext) ([]byte, error) {
	if ctx.Transfer == nil {
		return nil, fmt.Errorf("transfer is required")
	}
	t := ctx.Transfer

	pdf := newPDFDoc()
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(31, 41, 55) // slate-800
	pdf.CellFormat(0, 10, "Demonstrativo de Repasse", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 116, 139) // slate-500
	// Mesmo critério do recibo de cobrança: emissor só quando o nome real da
	// imobiliária foi resolvido; nada de marca fixa.
	emittedAt := time.Now().Format("02/01/2006 15:04")
	if org := strings.TrimSpace(ctx.OrgName); org != "" {
		pdf.CellFormat(0, 6, fmt.Sprintf("Emitido por %s em %s", org, emittedAt), "", 1, "L", false, 0, "")
	} else {
		pdf.CellFormat(0, 6, fmt.Sprintf("Emitido em %s", emittedAt), "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	// Reference block — two-column grid of metadata.
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

	kv("Referência:", t.ReferenceMonth)
	kv("Status:", string(t.Status))
	kv("Proprietário:", firstNonEmpty(ctx.OwnerName, t.OwnerID))
	if ctx.OwnerDocument != "" {
		kv("CPF/CNPJ:", ctx.OwnerDocument)
	}
	kv("Inquilino:", firstNonEmpty(ctx.TenantName, "—"))
	kv("Imóvel:", firstNonEmpty(ctx.PropertyTitle, t.PropertyID))
	kv("Contrato:", t.ContractID)
	if t.ProcessedAt != nil {
		kv("Processado em:", t.ProcessedAt.Format("02/01/2006"))
	}

	pdf.Ln(4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(6)

	// Breakdown block
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(31, 41, 55)
	pdf.CellFormat(0, 8, "Composição do Repasse", "", 1, "L", false, 0, "")
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

	row("Aluguel Bruto", t.GrossRent, false)
	row(fmt.Sprintf("Taxa de Administração (%.2f%%)", t.AdminFeePercent), t.AdminFeeAmount, true)
	if t.MaintenanceDeductions > 0 {
		row("Deduções de Manutenção", t.MaintenanceDeductions, true)
	}
	if t.OtherDeductions > 0 {
		row("Outras Deduções", t.OtherDeductions, true)
	}
	for _, d := range t.DeductionDetails {
		label := strings.TrimSpace(d.Description)
		if label == "" {
			label = "Dedução"
		}
		row(label, d.Amount, true)
	}

	pdf.Ln(2)
	pdf.SetDrawColor(31, 41, 55)
	pdf.SetLineWidth(0.4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 13)
	pdf.SetTextColor(15, 118, 110) // teal-700
	pdf.CellFormat(120, 9, "Valor Líquido", "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 9, formatBRL(t.NetAmount), "", 1, "R", false, 0, "")

	if strings.TrimSpace(t.Notes) != "" {
		pdf.Ln(6)
		pdf.SetFont("Arial", "I", 9)
		pdf.SetTextColor(100, 116, 139)
		pdf.MultiCell(0, 5, "Observações: "+t.Notes, "", "L", false)
	}

	if t.BankAccount != "" {
		pdf.Ln(2)
		pdf.SetFont("Arial", "", 9)
		pdf.SetTextColor(100, 116, 139)
		pdf.CellFormat(0, 5, "Conta bancária do destinatário: "+t.BankAccount, "", 1, "L", false, 0, "")
	}

	// Footer fixed at the bottom — printed last so it doesn't drift with
	// variable-length notes above.
	pdf.SetY(-25)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(148, 163, 184) // slate-400
	pdf.MultiCell(0, 4, "Este documento é apenas um demonstrativo. A confirmação de pagamento depende da liquidação bancária correspondente.", "", "L", false)
	pdf.CellFormat(0, 4, fmt.Sprintf("Transfer ID: %s", t.ID), "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// formatBRL delega ao formatador compartilhado (`pkg/money`): o recibo, os
// e-mails de cobrança e as notificações precisam mostrar o MESMO "R$ 6.000,00".
func formatBRL(v float64) string {
	return money.BRL(v)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
