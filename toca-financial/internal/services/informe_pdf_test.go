package services

import (
	"strings"
	"testing"

	"github.com/oginoino/toca/toca-financial/internal/domain/fiscal"
)

func TestRenderInformePDF_ProducesPDF(t *testing.T) {
	inf := fiscal.NewInforme("tenant-1", 2025, "owner-1", "Maria Proprietária", 12000, 1200, 1000, 10800)
	out, err := RenderInformePDF(inf)
	if err != nil {
		t.Fatalf("RenderInformePDF: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("PDF vazio")
	}
	if !strings.HasPrefix(string(out), "%PDF-") {
		t.Error("saída não começa com o magic %PDF-")
	}
}

func TestRenderInformePDF_NilInforme(t *testing.T) {
	if _, err := RenderInformePDF(nil); err == nil {
		t.Error("esperava erro para informe nil")
	}
}
