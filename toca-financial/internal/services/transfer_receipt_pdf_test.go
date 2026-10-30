package services

import (
	"strings"
	"testing"
	"time"

	"github.com/oginoino/toca/toca-financial/internal/domain/transfer"
)

func TestRenderTransferReceiptPDF_minimal(t *testing.T) {
	ot := transfer.New(
		"contract-abc",
		"owner-xyz",
		"prop-1",
		"tenant-foo",
		"2026-05",
		3500.00,
		8.0,
		0,
		0,
		nil,
		"",
	)
	out, err := RenderTransferReceiptPDF(TransferReceiptContext{Transfer: ot})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) < 1000 {
		t.Fatalf("PDF output suspiciously small: %d bytes", len(out))
	}
	if !strings.HasPrefix(string(out[:5]), "%PDF-") {
		t.Fatalf("output does not look like a PDF: %q", string(out[:8]))
	}
}

func TestRenderTransferReceiptPDF_withDeductionsAndDates(t *testing.T) {
	deductions := []transfer.Deduction{
		{Description: "Reparo encanamento", Amount: 250.50, Type: "maintenance"},
		{Description: "Multa contratual", Amount: 100, Type: "fine"},
	}
	ot := transfer.New("c1", "o1", "p1", "t1", "2026-06", 4000.00, 10.0, 250.50, 0, deductions, "Pagamento atrasado pelo banco")
	processed := time.Now()
	ot.ProcessedAt = &processed
	ot.BankAccount = "Itaú 1234/56789-0"

	out, err := RenderTransferReceiptPDF(TransferReceiptContext{
		Transfer:      ot,
		OwnerName:     "Maria Souza",
		OwnerDocument: "123.456.789-00",
		TenantName:    "Carlos Lima",
		PropertyTitle: "Edifício Pacaembu apto 401",
		OrgName:       "Imobiliária Teste",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) < 1500 {
		t.Fatalf("PDF output too small with deductions: %d bytes", len(out))
	}
}

func TestRenderTransferReceiptPDF_nilTransferRejected(t *testing.T) {
	if _, err := RenderTransferReceiptPDF(TransferReceiptContext{}); err == nil {
		t.Fatal("expected error for nil transfer")
	}
}

func TestFormatBRL(t *testing.T) {
	tests := map[string]float64{
		"R$ 0,00":         0,
		"R$ 1,50":         1.5,
		"R$ 1.234,56":     1234.56,
		"R$ 1.000.000,00": 1000000,
		"R$ 12,34":        12.34,
	}
	for want, in := range tests {
		got := formatBRL(in)
		if got != want {
			t.Errorf("formatBRL(%v) = %q; want %q", in, got, want)
		}
	}
}
