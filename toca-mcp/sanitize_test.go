package main

import (
	"os"
	"strings"
	"testing"
)

func TestSanitizarRemoveDocumentoDoContexto(t *testing.T) {
	raw, err := os.ReadFile("/tmp/contratos-reais.json")
	if err != nil {
		t.Skip("sem amostra do payload")
	}
	limpo := sanitizarRespostaParaModelo(raw)
	t.Logf("antes: %d bytes | depois: %d bytes | reducao: %.1f%%",
		len(raw), len(limpo), 100*(1-float64(len(limpo))/float64(len(raw))))

	if len(limpo) >= len(raw) {
		t.Fatalf("payload nao foi reduzido: antes=%d depois=%d", len(raw), len(limpo))
	}
	if strings.Contains(string(limpo), "UEsDBBQ") {
		t.Error("o base64 do DOCX continua no payload enviado ao modelo")
	}
	for _, campo := range []string{"\"id\"", "\"status\"", "\"deal_id\""} {
		if !strings.Contains(string(limpo), campo) {
			t.Errorf("campo util %s foi removido junto; o modelo precisa dos metadados", campo)
		}
	}
}
