package services

import (
	"bytes"
	"strings"
	"testing"
)

// TestPDFDocTranslatesAccents garante o contrato do wrapper: os core fonts do
// gofpdf usam cp1252, então texto UTF-8 precisa ser traduzido antes de ir para
// o arquivo. Sem isso os recibos/informes exportados saem com mojibake
// ("Cobrança" → "CobranÃ§a"), que foi o defeito relatado no fluxo financeiro.
func TestPDFDocTranslatesAccents(t *testing.T) {
	doc := newPDFDoc()
	if doc == nil || doc.tr == nil {
		t.Fatal("newPDFDoc deveria configurar o tradutor")
	}

	cases := []struct {
		in   string
		want string // byte cp1252 esperado no lugar do caractere acentuado
	}{
		{"Cobrança", "\xE7"},                 // ç
		{"Administração", "\xE7"},            // ç
		{"Proprietário", "\xE1"},             // á
		{"Condomínio", "\xED"},               // ô
		{"Não", "\xE3"},                      // ã
		{"Comissão", "\xE3"},                 // ã
		{"Seguro Fiança — R$ 1.000", "\xE7"}, // ç + travessão
	}

	for _, tc := range cases {
		got := doc.tr(tc.in)
		if !strings.Contains(got, tc.want) {
			t.Errorf("tr(%q) = %q, esperado byte cp1252 %q", tc.in, got, tc.want)
		}
		// O mojibake clássico é o UTF-8 do acento interpretado como Latin-1:
		// "ç" vira "Ã§" (0xC3 0xA7). Não pode aparecer no resultado.
		if strings.Contains(got, "\xC3") {
			t.Errorf("tr(%q) manteve bytes UTF-8 (mojibake): %q", tc.in, got)
		}
	}
}

// TestPDFDocWritesCp1252Bytes é a prova em nível de ARQUIVO: gera um PDF com
// acentos e verifica os bytes gravados. Sem a tradução o stream conteria o
// UTF-8 do acento (0xC3 0xA7 para "ç"), que é exatamente o mojibake visto nos
// documentos exportados.
func TestPDFDocWritesCp1252Bytes(t *testing.T) {
	doc := newPDFDoc()
	doc.SetCompression(false) // mantém o stream legível para a inspeção
	doc.AddPage()
	doc.SetFont("Helvetica", "", 12)
	doc.CellFormat(0, 10, "Cobrança de aluguel — Comissão do corretor", "", 1, "L", false, 0, "")

	var out bytes.Buffer
	if err := doc.Output(&out); err != nil {
		t.Fatalf("gerar PDF: %v", err)
	}
	raw := out.Bytes()

	if bytes.Contains(raw, []byte("\xC3\xA7")) {
		t.Fatal("PDF contém o UTF-8 de ç (0xC3 0xA7) — mojibake no arquivo exportado")
	}
	if !bytes.Contains(raw, []byte{0xE7}) {
		t.Fatal("PDF não contém o byte cp1252 de ç (0xE7)")
	}
}
