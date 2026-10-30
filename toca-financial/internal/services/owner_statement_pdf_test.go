package services

import (
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func statementFixtureWithDeductions() *MonthlyStatement {
	return &MonthlyStatement{
		TenantID:              "tenant-1",
		OwnerLeadID:           "owner-1",
		Year:                  2026,
		Month:                 10,
		ReferenceMonth:        "2026-10",
		GrossRent:             4128,
		AdminFeeAmount:        412,
		MaintenanceDeductions: 130,
		NetAmount:             3586,
		FlowSummaries: []StatementFlowSummary{{
			Flow:                  "rent",
			TransferCount:         1,
			GrossAmount:           4128,
			AdminFeeAmount:        412,
			MaintenanceDeductions: 130,
			NetAmount:             3586,
		}},
		Transfers: []*StatementTransfer{{
			OwnerTransfer: &transfer.OwnerTransfer{
				ID:                    "transfer-1",
				TenantID:              "tenant-1",
				ContractID:            "contract-1",
				OwnerID:               "owner-1",
				PropertyID:            "property-1",
				ReferenceMonth:        "2026-10",
				GrossRent:             4128,
				AdminFeeAmount:        412,
				MaintenanceDeductions: 130,
				NetAmount:             3586,
				Status:                transfer.StatusPending,
				DeductionDetails: []transfer.Deduction{{
					Description: "Manutenção: Elétrica (#os-1)",
					Amount:      130,
					Type:        "MAINTENANCE",
					ReferenceID: "os-1",
				}},
			},
			StatementFlow: "rent",
		}},
		GeneratedAt: time.Now(),
	}
}

// A dedução de manutenção precisa aparecer no extrato com a origem — não só
// como um total agregado.
func TestStatementDeductionsCarryOrigin(t *testing.T) {
	lines := statementDeductions(statementFixtureWithDeductions())

	require.Len(t, lines, 1)
	assert.Equal(t, "2026-10", lines[0].ReferenceMonth)
	assert.Equal(t, "Manutenção: Elétrica (#os-1)", lines[0].Description)
	assert.Equal(t, "os-1", lines[0].ReferenceID)
	assert.InDelta(t, 130.0, lines[0].Amount, 0.001)
}

func TestStatementDeductionsEmptyWithoutDetails(t *testing.T) {
	statement := statementFixtureWithDeductions()
	statement.Transfers[0].DeductionDetails = nil

	assert.Empty(t, statementDeductions(statement))
}

func TestDeductionTypeLabel(t *testing.T) {
	assert.Equal(t, "Manutenção", deductionTypeLabel("maintenance"))
	assert.Equal(t, "Consumo", deductionTypeLabel("UTILITY"))
	assert.Equal(t, "Taxa", deductionTypeLabel("fee"))
	assert.Equal(t, "—", deductionTypeLabel("  "))
	// Tipo desconhecido é exibido como veio, sem inventar rótulo.
	assert.Equal(t, "personalizado", deductionTypeLabel("personalizado"))
}

// O PDF do extrato precisa renderizar com as deduções sem quebrar.
func TestOwnerStatementPDFRendersDeductions(t *testing.T) {
	content, err := RenderMonthlyOwnerStatementPDF(OwnerStatementPDFContext{
		Statement: statementFixtureWithDeductions(),
		OwnerName: "João Proprietário",
		OrgName:   "Exemplo Imóveis",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, content)
	// Um PDF válido começa com a assinatura %PDF.
	assert.Equal(t, "%PDF", string(content[:4]))
}

func TestOwnerStatementIssuerLineUsesRealCompany(t *testing.T) {
	now := time.Date(2026, 10, 21, 23, 58, 0, 0, time.UTC)
	assert.Equal(t, "Emitido por Imobiliária Real em 21/10/2026 23:58", ownerStatementIssuerLine(" Imobiliária Real ", now))
	assert.Equal(t, "Emitido em 21/10/2026 23:58", ownerStatementIssuerLine("", now))
	assert.NotContains(t, ownerStatementIssuerLine("", now), "Toca")
}
