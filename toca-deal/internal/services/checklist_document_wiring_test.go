package services_test

import (
	"os"
	"strings"
	"testing"
)

// Guarda de fiação: o upload do cliente na esteira precisa espelhar o arquivo no
// painel de documentos. Se alguém remover a chamada (ou o wiring da rota), o
// documento volta a existir só no metadata do negócio — invisível para a
// imobiliária, exatamente o problema original. Roda no CI sem banco.
func TestChecklistUploadPathsMirrorIntoDocumentsPanel(t *testing.T) {
	source, err := os.ReadFile("deal_portal_service.go")
	if err != nil {
		t.Fatalf("ler deal_portal_service.go: %v", err)
	}
	content := string(source)

	// Os dois caminhos que recebem ARQUIVO (upload direto e PUT presigned).
	for _, marker := range []string{
		`"Checklist item uploaded via client portal"`,
		`"Checklist item uploaded via client portal (presigned)"`,
	} {
		idx := strings.Index(content, marker)
		if idx < 0 {
			t.Fatalf("marcador %s não encontrado: o fluxo de upload mudou de forma, revise o espelho", marker)
		}
		// A chamada do espelho precisa vir logo depois da gravação do item
		// (mesma função), antes do bloco seguinte.
		window := content[idx:]
		if end := strings.Index(window, "\n}\n"); end > 0 {
			window = window[:end]
		}
		if !strings.Contains(window, "s.mirrorChecklistDocument(ctx, d, item,") {
			t.Errorf("o upload (%s) não chama o espelho no painel de documentos", marker)
		}
	}

	// A rota precisa ligar o espelho ao serviço de documentos.
	routes, err := os.ReadFile("../api/v1/routes/routes.go")
	if err != nil {
		t.Fatalf("ler routes.go: %v", err)
	}
	if !strings.Contains(string(routes), "WithChecklistDocumentSync(docService)") {
		t.Error("routes.go não liga o espelho (dealService.WithChecklistDocumentSync)")
	}
	if !strings.Contains(string(routes), "backfill-checklist-documents") {
		t.Error("rota de backfill dos documentos da esteira não está registrada")
	}
}
