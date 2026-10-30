package main

import (
	"strings"
	"testing"
)

// URL pré-assinada de storage carrega credencial temporária na query. Medido num
// `signed_document_url` real: 1.870 caracteres, sendo 1.316 do
// `X-Amz-Security-Token` — credencial da AWS.
//
// Isso não pode ir para o modelo: gasta contexto com algo que não responde nada
// e grava segredo no histórico da conversa, nos logs e no prompt do provedor.
func TestSanitizarRemoveCredenciaisDeURLAssinada(t *testing.T) {
	url := "https://clicksign-content-production.s3.amazonaws.com/2026/09/14/contrato.pdf" +
		"?X-Amz-Expires=299&X-Amz-Date=20260914T163816Z" +
		"&X-Amz-Security-Token=" + strings.Repeat("A", 1316) +
		"&X-Amz-Credential=ASIAEXAMPLE%2F20260914%2Fus-east-1%2Fs3%2Faws4_request" +
		"&X-Amz-Signature=deadbeef"
	entrada := `{"signed_document_url":"` + url + `","id":"c1","status":"SIGNED"}`

	saida := string(sanitizarRespostaParaModelo([]byte(entrada)))

	if strings.Contains(saida, "X-Amz-Security-Token") {
		t.Error("o token de segurança da AWS continua no payload enviado ao modelo")
	}
	if strings.Contains(saida, "X-Amz-Signature") {
		t.Error("a assinatura da URL continua no payload enviado ao modelo")
	}
	if strings.Contains(saida, strings.Repeat("A", 100)) {
		t.Error("o valor do token continua no payload")
	}
	// O que importa para o modelo permanece: o documento existe e onde está.
	if !strings.Contains(saida, "clicksign-content-production.s3.amazonaws.com") {
		t.Error("o caminho base da URL foi removido; o modelo perde a referência do documento")
	}
	for _, campo := range []string{`"id"`, `"status"`} {
		if !strings.Contains(saida, campo) {
			t.Errorf("campo %s foi removido junto", campo)
		}
	}
	t.Logf("entrada=%d bytes saida=%d bytes", len(entrada), len(saida))
}

// URL comum com filtros não pode ser mutilada: só a query com cara de
// credencial é removida.
func TestSanitizarPreservaURLComum(t *testing.T) {
	entrada := `{"next":"https://api.example.com/api/v1/contracts?page=2&status=SIGNED"}`
	saida := string(sanitizarRespostaParaModelo([]byte(entrada)))
	if !strings.Contains(saida, "page=2") || !strings.Contains(saida, "status=SIGNED") {
		t.Errorf("URL comum perdeu seus filtros: %s", saida)
	}
}
