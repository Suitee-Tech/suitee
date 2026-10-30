package services

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	ps "github.com/oginoino/toca/toca-financial/internal/domain/platform_subscription"
)

// PlatformSubscriptionReceiptContext carries what the handler already read:
// the confirmed payment, the plan it paid for and who the customer is.
type PlatformSubscriptionReceiptContext struct {
	Payment            ps.Payment
	Plan               *ps.Plan
	SubscriptionID     string
	PaymentMethod      *ps.PaymentMethod
	OrgName            string
	TenantName         string
	TenantDocument     string
	ProviderReceiptURL string
	Sandbox            bool
}

// RenderPlatformSubscriptionReceiptPDF issues the receipt for a plan payment.
// The provider only exposes receipts for its own charges (and sends none for
// card subscriptions), so the platform documents what it knows: who paid, for
// which plan, how much and when.
func RenderPlatformSubscriptionReceiptPDF(ctx PlatformSubscriptionReceiptContext) ([]byte, error) {
	p := ctx.Payment
	if p.ID == "" || p.AmountCents <= 0 {
		return nil, fmt.Errorf("payment is required")
	}

	pdf := newPDFDoc()
	pdf.SetMargins(15, 15, 15)
	// A receipt is short: a tight bottom margin keeps the footer on the page
	// instead of pushing it to a second, empty one.
	pdf.SetAutoPageBreak(true, 12)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(31, 41, 55)
	pdf.CellFormat(0, 10, "Comprovante de Pagamento", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 116, 139)
	org := strings.TrimSpace(ctx.OrgName)
	if org == "" {
		org = "Suitee"
	}
	pdf.CellFormat(0, 6, fmt.Sprintf("Assinatura da plataforma — emitido por %s em %s", org, time.Now().Format("02/01/2006 15:04")), "", 1, "L", false, 0, "")
	pdf.Ln(4)
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

	kv("Pagamento:", p.ID)
	if ctx.SubscriptionID != "" {
		kv("Assinatura:", ctx.SubscriptionID)
	}
	kv("Cliente:", firstNonEmpty(ctx.TenantName, "—"))
	if ctx.TenantDocument != "" {
		kv("CPF/CNPJ:", formatTaxID(ctx.TenantDocument))
	}
	if ctx.Plan != nil {
		kv("Plano:", fmt.Sprintf("%s (%s)", ctx.Plan.Name, cycleLabel(ctx.Plan.Cycle)))
	}
	kv("Pago em:", p.PaidAt.Format("02/01/2006 15:04"))
	if ctx.PaymentMethod != nil {
		kv("Forma de pagamento:", paymentMethodLabel(ctx.PaymentMethod))
	}
	if ctx.ProviderReceiptURL != "" {
		kv("Recibo do provedor:", ctx.ProviderReceiptURL)
	}

	pdf.Ln(4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(6)

	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(31, 41, 55)
	pdf.CellFormat(0, 8, "Detalhamento", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	row := func(label string, cents int64) {
		pdf.SetFont("Arial", "", 11)
		pdf.SetTextColor(31, 41, 55)
		pdf.CellFormat(120, 7, label, "", 0, "L", false, 0, "")
		pdf.CellFormat(0, 7, formatBRL(float64(cents)/100), "", 1, "R", false, 0, "")
	}
	if ctx.Plan != nil {
		row(fmt.Sprintf("Plano %s — cobrança %s", ctx.Plan.Name, cycleLabel(ctx.Plan.Cycle)), p.AmountCents)
	} else {
		row("Assinatura da plataforma", p.AmountCents)
	}

	pdf.Ln(2)
	pdf.SetDrawColor(31, 41, 55)
	pdf.SetLineWidth(0.4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 13)
	pdf.SetTextColor(15, 118, 110)
	pdf.CellFormat(120, 9, "Total Pago", "", 0, "L", false, 0, "")
	pdf.CellFormat(0, 9, formatBRL(float64(p.AmountCents)/100), "", 1, "R", false, 0, "")

	pdf.SetY(-25)
	pdf.SetFont("Arial", "I", 8)
	pdf.SetTextColor(148, 163, 184)
	note := "Documento de uso interno, sem valor fiscal. Comprovante do pagamento registrado no sistema."
	if ctx.Sandbox {
		note = "Pagamento de teste (sandbox): nenhum valor foi cobrado. " + note
	}
	pdf.MultiCell(0, 4, fmt.Sprintf("%s\nRecibo ID: %s", note, p.ID), "", "L", false)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// formatTaxID prints CPF/CNPJ the way a Brazilian receipt does; anything else
// is returned untouched.
func formatTaxID(raw string) string {
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, raw)
	switch len(digits) {
	case 11:
		return digits[:3] + "." + digits[3:6] + "." + digits[6:9] + "-" + digits[9:]
	case 14:
		return digits[:2] + "." + digits[2:5] + "." + digits[5:8] + "/" + digits[8:12] + "-" + digits[12:]
	default:
		return raw
	}
}

func cycleLabel(cycle string) string {
	switch cycle {
	case "ANNUALLY":
		return "anual"
	case "MONTHLY":
		return "mensal"
	case "SEMIANNUALLY":
		return "semestral"
	case "WEEKLY":
		return "semanal"
	default:
		return strings.ToLower(cycle)
	}
}

func paymentMethodLabel(method *ps.PaymentMethod) string {
	if method == nil {
		return ""
	}
	kind := "Cartão de crédito"
	if method.Kind == "PIX" {
		kind = "Pix"
	}
	parts := []string{kind}
	if method.Brand != "" {
		parts = append(parts, method.Brand)
	}
	if method.Last4 != "" {
		parts = append(parts, "final "+method.Last4)
	}
	return strings.Join(parts, " · ")
}
