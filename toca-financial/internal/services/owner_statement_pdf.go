package services

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

// OwnerStatementPDFContext acrescenta nomes ao MonthlyStatement antes da
// renderização — handler enriquece via crmClient/dealClient (best-effort)
// e renderer cai de volta no ID quando ausente.
type OwnerStatementPDFContext struct {
	Statement     *MonthlyStatement
	OwnerName     string
	OwnerDocument string
	OrgName       string
}

// RenderMonthlyOwnerStatementPDF é o demonstrativo consolidado do mês para
// um único proprietário. Mostra header com o owner, agregação do mês (gross
// → admin → deductions → líquido), e uma tabela com cada Transfer
// individual, segmentados entre vendas, locações e modalidade a apurar.
func RenderMonthlyOwnerStatementPDF(ctx OwnerStatementPDFContext) ([]byte, error) {
	if ctx.Statement == nil {
		return nil, fmt.Errorf("statement is required")
	}
	st := ctx.Statement

	pdf := newPDFDoc()
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(true, 30)
	cell := func(w, h float64, text, border string, ln int, align string, fill bool, link int, url string) {
		pdf.CellFormat(w, h, (text), border, ln, align, fill, link, url)
	}
	pdf.SetFooterFunc(func() {
		pdf.SetY(-23)
		pdf.SetFont("Arial", "I", 8)
		pdf.SetTextColor(100, 116, 139)
		pdf.MultiCell(0, 4, ("Documento informativo. Valores das ordens de repasse da competência; não representam saldo bancário. O status indica o processamento de cada ordem."), "", "L", false)
		cell(0, 4, fmt.Sprintf("Página %d", pdf.PageNo()), "", 1, "R", false, 0, "")
	})
	pdf.AddPage()

	// Header
	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(31, 41, 55)
	cell(0, 10, "Demonstrativo Mensal de Repasses", "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(100, 116, 139)
	cell(0, 6, ownerStatementIssuerLine(ctx.OrgName, time.Now()), "", 1, "L", false, 0, "")
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
		cell(45, 6, label, "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "B", 10)
		pdf.SetTextColor(31, 41, 55)
		cell(0, 6, value, "", 1, "L", false, 0, "")
	}

	kv("Proprietário:", firstNonEmpty(ctx.OwnerName, st.OwnerLeadID))
	if ctx.OwnerDocument != "" {
		kv("CPF/CNPJ:", ctx.OwnerDocument)
	}
	kv("Mês de referência:", st.ReferenceMonth)
	kv("Repasses no período:", fmt.Sprintf("%d", len(st.Transfers)))

	pdf.Ln(4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(6)

	// Totals block
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(31, 41, 55)
	cell(0, 8, "Resumo do Mês", "", 1, "L", false, 0, "")
	pdf.Ln(2)

	row := func(label string, amount float64, negative bool) {
		pdf.SetFont("Arial", "", 11)
		pdf.SetTextColor(31, 41, 55)
		cell(120, 7, label, "", 0, "L", false, 0, "")
		text := formatBRL(amount)
		if negative {
			text = "-" + text
			pdf.SetTextColor(220, 38, 38)
		}
		cell(0, 7, text, "", 1, "R", false, 0, "")
	}

	row("Total Bruto", st.GrossRent, false)
	if st.BalanceCoveredCount > 0 {
		row("Total destinado ao proprietário", st.OwnerBalanceTotal, false)
		row("Reservado para encargos", st.ReservedAmount, true)
		if st.BlockedAmount > 0 {
			row("Bloqueado", st.BlockedAmount, true)
		}
		row("Disponível para repasse", st.AvailableAmount, false)
	}

	pdf.Ln(2)
	pdf.SetDrawColor(31, 41, 55)
	pdf.SetLineWidth(0.4)
	pdf.Line(15, pdf.GetY(), 195, pdf.GetY())
	pdf.Ln(3)

	pdf.SetFont("Arial", "B", 13)
	pdf.SetTextColor(15, 118, 110)
	cell(120, 9, "Total Líquido dos Repasses", "", 0, "L", false, 0, "")
	cell(0, 9, formatBRL(st.NetAmount), "", 1, "R", false, 0, "")

	pdf.Ln(8)

	// Segmentation uses the same authoritative metadata as the JSON response.
	for _, summary := range st.FlowSummaries {
		if pdf.GetY() > 200 {
			pdf.AddPage()
		}
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(31, 41, 55)
		cell(0, 8, statementFlowLabel(summary.Flow), "", 1, "L", false, 0, "")
		grossLabel := "Bruto a apurar"
		if summary.Flow == "sale" {
			grossLabel = "Venda bruta"
		}
		if summary.Flow == "rent" {
			grossLabel = "Aluguel bruto"
		}
		row(grossLabel, summary.GrossAmount, false)
		if summary.Flow == "sale" {
			if summary.BrokerageAmount != nil {
				row("Corretagem", *summary.BrokerageAmount, true)
			} else {
				kv("Corretagem:", "A apurar")
			}
			if summary.AdminFeeAmount != 0 {
				row("Taxa registrada", summary.AdminFeeAmount, true)
			}
		} else {
			feeLabel := "Taxas registradas"
			if summary.Flow == "rent" {
				feeLabel = "Taxa de Administração"
			}
			row(feeLabel, summary.AdminFeeAmount, true)
		}
		if summary.IntermediationAmount != 0 {
			// Sem esta linha o proprietário via "aluguel bruto − taxa de
			// administração" e não fechava com o líquido: a comissão do primeiro
			// aluguel é diluída nos primeiros meses e sai do próprio aluguel.
			row("Intermediação (comissão do 1º aluguel)", summary.IntermediationAmount, true)
			if summary.IntermediationCommissionFundedAmount != 0 {
				kv("   · para comissões de corretor:", formatBRL(summary.IntermediationCommissionFundedAmount))
			}
		}
		if summary.MaintenanceDeductions != 0 {
			row("Manutenção", summary.MaintenanceDeductions, true)
		}
		if summary.OtherDeductions != 0 {
			row("Outras deduções", summary.OtherDeductions, true)
		}
		if summary.DiscountAmount != 0 {
			row("Descontos", summary.DiscountAmount, true)
		}
		row("Líquido do segmento", summary.NetAmount, false)
		pdf.Ln(4)
	}

	// Per-transfer table
	if len(st.Transfers) > 0 {
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(31, 41, 55)
		cell(0, 8, "Detalhamento dos Repasses", "", 1, "L", false, 0, "")
		pdf.Ln(2)

		header := func() {
			pdf.SetFillColor(248, 250, 252)
			pdf.SetFont("Arial", "B", 8)
			pdf.SetTextColor(71, 85, 105)
			cell(18, 7, "Modalidade", "", 0, "L", true, 0, "")
			cell(40, 7, "Contrato/Imóvel", "", 0, "L", true, 0, "")
			cell(26, 7, "Bruto", "", 0, "R", true, 0, "")
			cell(26, 7, "Taxa/Intermed.", "", 0, "R", true, 0, "")
			cell(24, 7, "Deduções", "", 0, "R", true, 0, "")
			cell(28, 7, "Líquido", "", 0, "R", true, 0, "")
			cell(18, 7, "Status", "", 1, "L", true, 0, "")
		}
		if pdf.GetY() > 238 {
			pdf.AddPage()
		}
		header()
		for _, t := range st.Transfers {
			if pdf.GetY() > 255 {
				pdf.AddPage()
				header()
			}
			pdf.SetFont("Arial", "", 8)
			pdf.SetTextColor(31, 41, 55)
			label := firstNonEmpty(t.ContractID, t.PropertyID)
			fee := formatBRL(t.AdminFeeAmount)
			if t.StatementFlow == "sale" {
				fee = "A apurar"
				if t.BrokerageAmount != nil {
					fee = formatBRL(*t.BrokerageAmount)
				}
			} else if t.IntermediationAmount != nil && *t.IntermediationAmount > 0 {
				// Taxa de administração + intermediação do mês: é a soma que
				// explica a diferença entre bruto e líquido na linha.
				fee = formatBRL(t.AdminFeeAmount) + " / " + formatBRL(*t.IntermediationAmount)
			}
			modality := "A apurar"
			if t.StatementFlow == "sale" {
				modality = "Venda"
			}
			if t.StatementFlow == "rent" {
				modality = "Locação"
			}
			deductions := t.MaintenanceDeductions + t.OtherDeductions
			deductionsLabel := "—"
			if deductions > 0 {
				deductionsLabel = "- " + formatBRL(deductions)
			}
			cell(18, 6, modality, "", 0, "L", false, 0, "")
			cell(40, 6, truncate(label, 22), "", 0, "L", false, 0, "")
			cell(26, 6, formatBRL(t.GrossRent), "", 0, "R", false, 0, "")
			cell(26, 6, fee, "", 0, "R", false, 0, "")
			cell(24, 6, deductionsLabel, "", 0, "R", false, 0, "")
			cell(28, 6, formatBRL(t.NetAmount), "", 0, "R", false, 0, "")
			cell(18, 6, statementStatusLabel(string(t.Status)), "", 1, "L", false, 0, "")
		}
	}

	// Deduções aplicadas ao repasse, com a origem (ex.: manutenção do chamado).
	deductionLines := statementDeductions(st)
	if len(deductionLines) > 0 {
		if pdf.GetY() > 230 {
			pdf.AddPage()
		}
		pdf.Ln(4)
		pdf.SetFont("Arial", "B", 12)
		pdf.SetTextColor(31, 41, 55)
		cell(0, 8, "Deduções do Período", "", 1, "L", false, 0, "")
		pdf.Ln(2)
		pdf.SetFillColor(248, 250, 252)
		pdf.SetFont("Arial", "B", 8)
		pdf.SetTextColor(71, 85, 105)
		cell(25, 7, "Competência", "", 0, "L", true, 0, "")
		cell(30, 7, "Origem", "", 0, "L", true, 0, "")
		cell(85, 7, "Descrição", "", 0, "L", true, 0, "")
		cell(30, 7, "Valor", "", 1, "R", true, 0, "")
		for _, line := range deductionLines {
			if pdf.GetY() > 255 {
				pdf.AddPage()
			}
			pdf.SetFont("Arial", "", 8)
			pdf.SetTextColor(31, 41, 55)
			cell(25, 6, line.ReferenceMonth, "", 0, "L", false, 0, "")
			cell(30, 6, deductionTypeLabel(line.Type), "", 0, "L", false, 0, "")
			cell(85, 6, truncate(line.Description, 52), "", 0, "L", false, 0, "")
			cell(30, 6, "- "+formatBRL(line.Amount), "", 1, "R", false, 0, "")
		}
	}

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// StatementDeductionLine é uma dedução do repasse com a competência de origem.
type StatementDeductionLine struct {
	ReferenceMonth string
	Description    string
	Amount         float64
	Type           string
	ReferenceID    string
}

func statementDeductions(st *MonthlyStatement) []StatementDeductionLine {
	var out []StatementDeductionLine
	for _, t := range st.Transfers {
		if t == nil {
			continue
		}
		for _, deduction := range t.DeductionDetails {
			out = append(out, StatementDeductionLine{
				ReferenceMonth: t.ReferenceMonth,
				Description:    deduction.Description,
				Amount:         deduction.Amount,
				Type:           deduction.Type,
				ReferenceID:    deduction.ReferenceID,
			})
		}
	}
	return out
}

func deductionTypeLabel(kind string) string {
	switch strings.ToUpper(strings.TrimSpace(kind)) {
	case "MAINTENANCE":
		return "Manutenção"
	case "UTILITY":
		return "Consumo"
	case "FEE":
		return "Taxa"
	case "OTHER":
		return "Outras"
	case "":
		return "—"
	default:
		return kind
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func statementFlowLabel(flow string) string {
	switch flow {
	case "sale":
		return "Vendas"
	case "rent":
		return "Aluguéis"
	default:
		return "Modalidade a apurar"
	}
}

func statementStatusLabel(status string) string {
	switch status {
	case "PENDING":
		return "Pendente"
	case "APPROVED":
		return "Aprovado"
	case "PROCESSED":
		return "Processado"
	case "CANCELLED":
		return "Cancelado"
	default:
		return "A apurar"
	}
}

// ownerStatementIssuerLine imprime o emissor somente quando o nome real da
// imobiliária foi resolvido. Nunca usa uma marca fixa como "Toca".
func ownerStatementIssuerLine(org string, now time.Time) string {
	emittedAt := now.Format("02/01/2006 15:04")
	if strings.TrimSpace(org) == "" {
		return fmt.Sprintf("Emitido em %s", emittedAt)
	}
	return fmt.Sprintf("Emitido por %s em %s", strings.TrimSpace(org), emittedAt)
}
