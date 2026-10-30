package pdf

import (
	"encoding/base64"
	"testing"
)

func TestIsBareBase64Docx(t *testing.T) {
	// DOCX real: ZIP começa com PK\x03\x04.
	zipHead := []byte("PK\x03\x04\x14\x00\x00\x00\x08\x00")
	docx := base64.StdEncoding.EncodeToString(append(zipHead, make([]byte, 200)...))
	if !isBareBase64Docx(docx) {
		t.Error("DOCX em base64 puro não foi reconhecido")
	}
	// PDF em base64 não pode ser confundido com DOCX.
	pdf := base64.StdEncoding.EncodeToString(append([]byte("%PDF-1.7"), make([]byte, 300)...))
	if isBareBase64Docx(pdf) {
		t.Error("PDF foi confundido com DOCX")
	}
	// Texto comum em base64 também não.
	txt := base64.StdEncoding.EncodeToString([]byte("apenas um texto qualquer, sem zip nenhum aqui dentro ok"))
	if isBareBase64Docx(txt) {
		t.Error("texto foi confundido com DOCX")
	}
	// Data URL já é tratado antes; não deve ser reconhecido aqui.
	if isBareBase64Docx(docxDataURLPrefix + docx) {
		t.Error("data URL não deveria passar pelo detector de base64 puro")
	}
}
