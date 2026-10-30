package dimob

import (
	"strings"
	"testing"
)

func validFile(t *testing.T) string {
	t.Helper()
	res, err := Render(Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental()}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return res.Content
}

func TestValidateFile_ArquivoGeradoEhConforme(t *testing.T) {
	report := ValidateFile(validFile(t))
	if report.HasErrors() {
		t.Fatalf("arquivo gerado deveria ser conforme, veio: %v", report.Errors())
	}
	if len(report.Records) != 4 {
		t.Fatalf("esperava 4 registros, veio %d", len(report.Records))
	}
	wantTypes := []string{"header", "R01", "R02", "T9"}
	for i, want := range wantTypes {
		if report.Records[i].Type != want {
			t.Errorf("registro %d = %q, esperado %q", i+1, report.Records[i].Type, want)
		}
	}
	if got := report.Records[2].Width; got != widthR02 {
		t.Errorf("R02 com %d posições", got)
	}
}

// O arquivo que a Suitee gerava antes desta correção: header com dados no lugar
// do reservado, R02 com sequencial de 7 (2 posições a mais, deslocando todos os
// campos seguintes), data em AAAAMMDD e T9 com CNPJ e contagem. O conferidor
// precisa apontar cada um.
func TestValidateFile_AcusaArquivoForaDoLeiaute(t *testing.T) {
	// Arquivo no formato ANTIGO (o que a Suitee gerava): header com ano/CNPJ/
	// razão social no lugar do reservado, R02 com sequencial de 7 posições
	// (2 a mais, deslocando todos os campos seguintes), data em AAAAMMDD e T9
	// com CNPJ e contagem de linhas.
	cnpj := "11222333000181"
	r02 := "R02" + cnpj + "2025" + "0000001" +
		padRight("12345678909", 14) + padRight("FABIO RAMOS", 60) +
		padRight("98765432100", 14) + padRight("CARLA MENEZES", 60) +
		padLeft("123456", 6) + "20250110"
	for m := 0; m < 12; m++ {
		r02 += amount(3500) + amount(350) + amount(0)
	}
	r02 += "U" + padRight("RUA VITORIA REGIA 152", 60) + "14032000" + "6969" + spaces(20) + "SP" + spaces(10)

	old := strings.Join([]string{
		"DIMOB" + spaces(7) + "2025" + spaces(5) + cnpj + "2020" + padRight("Exemplo IMOVEIS LTDA", 60) + "SP" + spaces(273),
		"R01" + cnpj + "2025" + "0" + padLeft("", 10) + "0" + "00000000" + "00" +
			padRight("Exemplo IMOVEIS LTDA", 60) + "12345678909" +
			padRight("RUA DAS PALMEIRAS 152 JARDINS", 120) + "SP" + "5030" + spaces(20) + spaces(10),
		r02,
		"T9" + cnpj + padLeft("4", 6) + spaces(80),
	}, recordEOL) + recordEOL

	if len(old) == 0 {
		t.Fatal("fixture vazia")
	}
	report := ValidateFile(old)
	messages := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		messages = append(messages, issue.String())
	}
	joined := strings.Join(messages, "\n")

	for _, want := range []string{
		"reservadas",          // header com dados no lugar do branco
		"o leiaute exige 797", // R02 com 799 (sequencial de 7)
		"DDMMAAAA",            // data do contrato em AAAAMMDD
		"T9",                  // T9 com CNPJ e contagem
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("conferência não apontou %q:\n%s", want, joined)
		}
	}
	if !report.HasErrors() {
		t.Fatal("arquivo fora do leiaute deveria ter erro bloqueante")
	}
}

func TestValidateFile_AcusaDelimitadorErradoEDocumentosAusentes(t *testing.T) {
	content := strings.ReplaceAll(validFile(t), recordEOL, "\n")
	report := ValidateFile(content)
	joined := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		joined = append(joined, issue.String())
	}
	if !strings.Contains(strings.Join(joined, "\n"), "CRLF") {
		t.Fatalf("esperava erro de delimitador, veio: %v", joined)
	}

	// Sem CPF/CNPJ das partes o arquivo continua importável, mas o conferidor
	// precisa avisar: é o que a Receita cobra como informação omitida.
	r := sampleRental()
	r.LocadorDoc = ""
	r.LocatarioDoc = ""
	res, err := Render(Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{r}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	report = ValidateFile(res.Content)
	if report.HasErrors() {
		t.Fatalf("documento ausente é advertência, não erro: %v", report.Errors())
	}
	var avisos int
	for _, issue := range report.Issues {
		if issue.Warning && strings.Contains(issue.Field, "CPF/CNPJ") {
			avisos++
		}
	}
	if avisos != 2 {
		t.Fatalf("esperava advertência para locador e locatário, veio %d: %v", avisos, report.Issues)
	}
}

func TestValidateFile_AcusaOrdemDeRegistros(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(validFile(t), recordEOL), recordEOL)
	// R01 antes do header e T9 no meio.
	report := ValidateFile(strings.Join([]string{lines[1], lines[0], lines[3], lines[2]}, recordEOL) + recordEOL)
	joined := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		joined = append(joined, issue.String())
	}
	text := strings.Join(joined, "\n")
	if !strings.Contains(text, "primeiro registro") {
		t.Errorf("esperava erro de header ausente:\n%s", text)
	}
	if !strings.Contains(text, "segundo registro") {
		t.Errorf("esperava erro de R01 ausente:\n%s", text)
	}
	if !strings.Contains(text, "depois do T9") {
		t.Errorf("esperava erro de registro depois do T9:\n%s", text)
	}
}

func TestLayout_TabelaFechaAsLargurasOficiais(t *testing.T) {
	for _, spec := range Layout() {
		next := 1
		for _, f := range spec.Fields {
			if f.Start != next {
				t.Errorf("%s: campo %q começa em %d, esperado %d (sem sobreposição nem buraco)", spec.Type, f.Name, f.Start, next)
			}
			next = f.End() + 1
		}
		if next-1 != spec.Width {
			t.Errorf("%s: campos somam %d posições, largura declarada %d", spec.Type, next-1, spec.Width)
		}
	}
}

// A tabela do leiaute e o gerador não podem divergir: cada registro emitido tem
// exatamente a largura e as posições que o conferidor usa para ler o arquivo.
func TestLayout_TabelaConfereComArquivoGerado(t *testing.T) {
	lines := strings.Split(strings.TrimSuffix(validFile(t), recordEOL), recordEOL)
	for i, spec := range Layout() {
		if len(lines[i]) != spec.Width {
			t.Errorf("%s: arquivo tem %d posições, tabela diz %d", spec.Type, len(lines[i]), spec.Width)
		}
		for _, f := range spec.Fields {
			if f.End() > len(lines[i]) {
				t.Errorf("%s: campo %q termina em %d, fora da linha de %d", spec.Type, f.Name, f.End(), len(lines[i]))
			}
		}
	}
}
