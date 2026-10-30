package services

import "github.com/jung-kurt/gofpdf"

// pdfDoc encapsula o gofpdf para garantir o encoding correto dos textos.
//
// Os PDFs financeiros usam core fonts (Helvetica) — o gofpdf espera cp1252
// nesses casos, não UTF-8. Sem tradução, "Cobrança"/"Proprietário" saem no
// arquivo exportado como "CobranÃ§a"/"ProprietÃ¡rio" (mojibake clássico de
// UTF-8 interpretado como Latin-1). O wrapper aplica o tradutor em todo texto
// de célula, então os geradores continuam escrevendo UTF-8 normalmente.
type pdfDoc struct {
	*gofpdf.Fpdf
	tr func(string) string
}

// newPDFDoc cria o documento já com o tradutor UTF-8→cp1252 configurado.
func newPDFDoc() *pdfDoc {
	f := gofpdf.New("P", "mm", "A4", "")
	// Descriptor "" usa a tabela cp1252 padrão dos core fonts. Cobre os
	// acentos do pt-BR e os sinais usados nos recibos (—, –, º, §).
	return &pdfDoc{Fpdf: f, tr: f.UnicodeTranslatorFromDescriptor("")}
}

// CellFormat traduz o texto antes de desenhar (mesma assinatura do gofpdf).
func (d *pdfDoc) CellFormat(w, h float64, txtStr, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string) {
	d.Fpdf.CellFormat(w, h, d.tr(txtStr), borderStr, ln, alignStr, fill, link, linkStr)
}

// MultiCell traduz o texto antes de desenhar (mesma assinatura do gofpdf).
func (d *pdfDoc) MultiCell(w, h float64, txtStr, borderStr, alignStr string, fill bool) {
	d.Fpdf.MultiCell(w, h, d.tr(txtStr), borderStr, alignStr, fill)
}

// Text traduz o texto de posicionamento absoluto (mesma assinatura do gofpdf).
func (d *pdfDoc) Text(x, y float64, txtStr string) {
	d.Fpdf.Text(x, y, d.tr(txtStr))
}
