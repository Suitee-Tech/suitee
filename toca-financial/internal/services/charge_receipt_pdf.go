package services

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/charge"
)

// ChargeReceiptContext bundles os dados que o handler já tem em mãos
// (enriquecimento via crmClient/dealClient) para o renderer. Mantém o
// mesmo formato de TransferReceiptContext: campos opcionais caem para
// IDs quando os nomes não vêm preenchidos.
type ChargeReceiptContext struct {
	Charge          *charge.Charge
	OwnerName       string
	OwnerDocument   string
	PayerName       string
	PayerDocument   string
	PropertyTitle   string
	PropertyAddress string
	OrgName         string
}

// RenderChargeReceiptPDF emite um recibo de uma cobrança paga. Layout
// alinhado com TransferReceiptPDF e InformePDF (gofpdf, sem fontes
// externas). Caller deve garantir Charge.Status == PAID — o renderer
// não valida (deixa fluxo limpo: borda HTTP barra antes).
func RenderChargeReceiptPDF(ctx ChargeReceiptContext) ([]byte, error) {
	if ctx.Charge == nil {
		return nil, fmt.Errorf("charge is required")
	}
	c := ctx.Charge

	pdf := newPDFDoc()
	pdf.SetMargins(15, 15, 15)
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(31, 41, 55)
	pdf.CellFormat(0, 10, "Recibo de Pagamento", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 116, 139)
	// O nome vem da imobiliária logada (OrgName). Sem ele, omitir o emissor é
	// melhor do que imprimir uma marca fixa que não é a da empresa.
	emittedAt := time.Now().Format("02/01/2006 15:04")
	if org := strings.TrimSpace(ctx.OrgName); org != "" {
		pdf.CellFormat(0, 6, fmt.Sprintf("Emitido por %s em %s", org, emittedAt), "", 1, "L", false, 0, "")
	} else {
		pdf.CellFormat(0, 6, fmt.Sprintf("Emitido em %s", emittedAt), "", 1, "L", false, 0, "")
	}
	pdf.Ln(4)

	// Reference block
	pdf.SetDrawColor(226, 232, 240)
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

	kv("Cobrança:", c.ID)
	kv("Tipo:", string(c.Type))
	kv("Descrição:", firstNonEmpty(c.Description, "—"))
	if c.ReferenceMonth != "" {
		kv("Referência:", c.ReferenceMonth)
	}
	kv("Pagador:", firstNonEmpty(ctx.PayerName, c.TenantPartyID))
	if ctx.PayerDocument != "" {
		kv("CPF/CNPJ Pagador:", ctx.PayerDocument)
	}
	kv("Beneficiário:", firstNonEmpty(ctx.OwnerName, c.OwnerID))
	if ctx.OwnerDocument != "" {
		kv("CPF/CNPJ Beneficiário:", ctx.OwnerDocument)
	}
	kv("Imóvel:", firstNonEmpty(ctx.PropertyTitle, c.PropertyID))
	if ctx.PropertyAddress != "" {
		kv("Endereço:", ctx.PropertyAddress)
	}
	kv("Contrato:", firstNonEmpty(c.ContractID, "—"))
	kv("Vencimento:", c.DueDate.Format("02/01/2006"))
	if c.PaidAt != nil {
		kv("Pago em:", c.PaidAt.Format("02/01/2006 15:04"))
	}
	if c.PaymentMethod != "" {
		kv("Forma de pagamento:", c.PaymentMethod)
	}

	pdf.Ln(4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(6)

	// Breakdown
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(31, 41, 55)
	pdf.CellFormat(0, 8, "Detalhamento", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	row := func(label string, amount float64, negative bool) {
		pdf.SetFont("Arial", "", 11)
		pdf.SetTextColor(31, 41, 55)
		pdf.CellFormat(120, 7, label, "", 0, "L", false, 0, "")
		text := formatBRL(amount)
		if negative {
			text = "-" + text
			pdf.SetTextColor(220, 38, 38)
		}
		pdf.CellFormat(0, 7, text, "", 1, "R", false, 0, "")
	}

	items, rentalSplit := chargeReceiptComposition(c)
	if len(items) == 0 {
		row("Valor da Cobrança", c.Amount.Float(), false)
	} else {
		for _, item := range items {
			row(item.Description, float64(item.AmountCents)/100, false)
		}
		if rentalSplit != nil {
			pdf.Ln(2)
			pdf.SetFont("Arial", "", 9)
			pdf.MultiCell(0, 5, "Administração deduzida do aluguel, incluída no valor acima:", "", "L", false)
			row("Taxa de administração", float64(rentalSplit.AdministrationCents)/100, false)
			row("Aluguel líquido do proprietário", float64(rentalSplit.OwnerRentCents)/100, false)
		}
		pdf.Ln(2)
		row("Subtotal da cobrança", c.Amount.Float(), false)
	}
	if c.LateFee.Float() > 0 {
		row("Multa", c.LateFee.Float(), false)
	}
	if c.Interest.Float() > 0 {
		row("Juros", c.Interest.Float(), false)
	}
	if c.Discount.Float() > 0 {
		row("Desconto", c.Discount.Float(), true)
	}

	pdf.Ln(2)
	pdf.SetDrawColor(31, 41, 55)
	pdf.SetLineWidth(0.4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 13)
	pdf.SetTextColor(15, 118, 110)
	pdf.CellFormat(120, 9, "Total Pago", "", 0, "L", false, 0, "")
	paid := c.TotalAmount.Float()
	if c.PaidAmount != nil {
		paid = c.PaidAmount.Float()
	}
	pdf.CellFormat(0, 9, formatBRL(paid), "", 1, "R", false, 0, "")

	// Footer
	pdf.SetY(-25)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(148, 163, 184)
	pdf.MultiCell(0, 4, "Documento de uso interno, sem valor fiscal. Comprovante de pagamento registrado no sistema.", "", "L", false)
	pdf.CellFormat(0, 4, fmt.Sprintf("Recibo ID: %s", c.ID), "", 1, "L", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type chargeReceiptItem struct {
	Description string
	AmountCents int64
}

type chargeReceiptRentalSplit struct {
	GrossRentCents      int64
	AdministrationCents int64
	OwnerRentCents      int64
}

// Generated recurring rental items describe economic destinations. Only the
// generated revenue/admin_fee is included in rental_base; an explicit bundle
// administration item is an additional charge and does not carry this marker.
func chargeReceiptComposition(c *charge.Charge) ([]chargeReceiptItem, *chargeReceiptRentalSplit) {
	items := make([]chargeReceiptItem, len(c.LineItems))
	for i, item := range c.LineItems {
		items[i] = chargeReceiptItem{Description: item.Description, AmountCents: item.AmountCents}
	}
	if c.Type != charge.TypeRent || !strings.HasPrefix(c.SourceEventID, "rental.") || c.TenantID == "" || c.OwnerID == "" || c.TenantID == c.OwnerID {
		return items, nil
	}
	ownerIndex, administrationIndex := -1, -1
	ownerCount, administrationCount := 0, 0
	for i, item := range c.LineItems {
		if item.SourceType == "rental_base" && item.EconomicOwnerID == c.OwnerID && item.SettlementMode == charge.SettlementModeCollection && item.AmountCents >= 0 {
			ownerIndex = i
			ownerCount++
		}
		if item.SourceType == "rental_bundled_item" && item.SourceID == "admin_fee" && item.AllocationType == charge.AllocationTypeRevenue && item.EconomicOwnerID == c.TenantID && item.SettlementMode == charge.SettlementModeCollection && item.AmountCents > 0 {
			administrationIndex = i
			administrationCount++
		}
	}
	if ownerCount != 1 || administrationCount != 1 {
		return items, nil
	}
	owner, administration := c.LineItems[ownerIndex], c.LineItems[administrationIndex]
	if owner.RuleVersionID == "" || owner.RuleVersionID != administration.RuleVersionID {
		return items, nil
	}
	split := &chargeReceiptRentalSplit{
		GrossRentCents:      owner.AmountCents + administration.AmountCents,
		AdministrationCents: administration.AmountCents,
		OwnerRentCents:      owner.AmountCents,
	}
	composition := make([]chargeReceiptItem, 0, len(items)-1)
	for i, item := range items {
		if i == administrationIndex {
			continue
		}
		if i == ownerIndex {
			item.Description = "Aluguel"
			item.AmountCents = split.GrossRentCents
		}
		composition = append(composition, item)
	}
	return composition, split
}
