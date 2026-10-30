package dimob

import (
	"strings"
	"testing"
)

// O leiaute oficial é uma tabela de campos de tamanho fixo. Estes testes
// verificam o arquivo contra a TABELA (posição e largura de cada campo), e não
// contra o código que o gerou: um deslocamento de um caractere aparece aqui
// como erro de posição, que é exatamente como o programa da RFB o leria.

// sampleDeclarante devolve uma declarante válida e completa.
func sampleDeclarante() Declarante {
	return Declarante{
		CNPJ:            "11.222.333/0001-81",
		Nome:            "Imobiliária Exemplo LTDA",
		AnoBase:         2025,
		Retificadora:    false,
		CPFResponsavel:  "123.456.789-09",
		Endereco:        "Rua das Palmeiras, 152 - Jardins",
		UF:              "SP",
		CodigoMunicipio: "5030",
	}
}

// sampleRental devolve uma locação com todos os campos preenchidos.
func sampleRental() Rental {
	r := Rental{
		LocadorDoc:      "123.456.789-09",
		LocadorNome:     "Fábio Ramos",
		LocatarioDoc:    "987.654.321-00",
		LocatarioNome:   "Carla Menezes",
		Contrato:        "123456",
		DataContrato:    "10012025", // DDMMAAAA
		TipoImovel:      "U",
		Endereco:        "Rua Vitória Régia, 152",
		CEP:             "14032-000",
		CodigoMunicipio: "6969",
		UF:              "SP",
	}
	for m := 0; m < 12; m++ {
		r.MonthlyRent[m] = 3500
		r.MonthlyCommission[m] = 350
	}
	return r
}

func renderLines(t *testing.T, d Declaration) []string {
	t.Helper()
	res, err := Render(d)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.HasSuffix(res.Content, recordEOL) {
		t.Fatalf("arquivo deve terminar em CRLF")
	}
	content := strings.TrimSuffix(res.Content, recordEOL)
	return strings.Split(content, recordEOL)
}

// field recorta um campo pela posição inicial (1-based) e largura do leiaute.
func field(line string, start, width int) string {
	if start-1+width > len(line) {
		return "<fora da linha>"
	}
	return line[start-1 : start-1+width]
}

func TestRender_LayoutOficial(t *testing.T) {
	d := Declaration{
		Declarante: sampleDeclarante(),
		Rentals:    []Rental{sampleRental()},
	}
	lines := renderLines(t, d)

	if len(lines) != 4 {
		t.Fatalf("esperava 4 linhas (header + R01 + R02 + T9), veio %d", len(lines))
	}

	// Larguras oficiais (sem o delimitador CRLF).
	for _, tc := range []struct {
		index int
		name  string
		width int
	}{
		{0, "header", widthHeader},
		{1, "R01", widthR01},
		{2, "R02", widthR02},
		{3, "T9", widthT9},
	} {
		if len(lines[tc.index]) != tc.width {
			t.Errorf("%s com %d posições, o leiaute exige %d", tc.name, len(lines[tc.index]), tc.width)
		}
	}

	// Header: identificação do arquivo — "DIMOB" e o resto reservado em branco.
	// Não carrega ano, CNPJ, razão social nem versão do leiaute.
	header := lines[0]
	if got := field(header, 1, 5); got != "DIMOB" {
		t.Errorf("header[1..5] = %q, esperado DIMOB", got)
	}
	if rest := strings.TrimRight(field(header, 6, widthHeader-5), " "); rest != "" {
		t.Errorf("header[6..374] deve ser reservado em branco, veio %q", rest)
	}

	// R01 — Ficha 01 (posições do leiaute).
	r01 := lines[1]
	for _, tc := range []struct {
		start, width int
		name, want   string
	}{
		{1, 3, "tipo", "R01"},
		{4, 14, "CNPJ do declarante", "11222333000181"},
		{18, 4, "ano-calendário", "2025"},
		{22, 1, "declaração retificadora", "0"},
		{23, 10, "número do recibo", "0000000000"},
		{33, 1, "situação especial", "0"},
		{34, 8, "data do evento", "00000000"},
		{42, 2, "código da situação especial", "00"},
		{104, 11, "CPF do responsável", "12345678909"},
		{235, 2, "UF do contribuinte", "SP"},
		{237, 4, "código do município", "5030"},
	} {
		if got := field(r01, tc.start, tc.width); got != tc.want {
			t.Errorf("R01 %s [%d..%d] = %q, esperado %q", tc.name, tc.start, tc.start+tc.width-1, got, tc.want)
		}
	}
	if got := strings.TrimSpace(field(r01, 44, 60)); got != "IMOBILIARIA EXEMPLO LTDA" {
		t.Errorf("R01 nome empresarial = %q", got)
	}
	if got := strings.TrimSpace(field(r01, 115, 120)); got != "RUA DAS PALMEIRAS 152 JARDINS" {
		t.Errorf("R01 endereço = %q", got)
	}
	for _, tc := range []struct{ start, width int }{{241, 20}, {261, 10}} {
		if got := field(r01, tc.start, tc.width); strings.TrimSpace(got) != "" {
			t.Errorf("R01 reservado [%d..%d] deve ser branco, veio %q", tc.start, tc.start+tc.width-1, got)
		}
	}

	// R02 — Ficha 02 (posições do leiaute).
	r02 := lines[2]
	for _, tc := range []struct {
		start, width int
		name, want   string
	}{
		{1, 3, "tipo", "R02"},
		{4, 14, "CNPJ do declarante", "11222333000181"},
		{18, 4, "ano-calendário", "2025"},
		{22, 5, "sequencial da locação", "00001"},
		{175, 6, "número do contrato", "123456"},
		{181, 8, "data do contrato", "10012025"},
		{189, 14, "aluguel de janeiro", "00000000350000"},
		{203, 14, "comissão de janeiro", "00000000035000"},
		{217, 14, "imposto de janeiro", "00000000000000"},
		{651, 14, "aluguel de dezembro", "00000000350000"},
		{679, 14, "imposto de dezembro", "00000000000000"},
		{693, 1, "tipo do imóvel", "U"},
		{754, 8, "CEP", "14032000"},
		{762, 4, "código do município do imóvel", "6969"},
		{786, 2, "UF do imóvel", "SP"},
	} {
		if got := field(r02, tc.start, tc.width); got != tc.want {
			t.Errorf("R02 %s [%d..%d] = %q, esperado %q", tc.name, tc.start, tc.start+tc.width-1, got, tc.want)
		}
	}
	// CPF ocupa 11 posições à esquerda e o resto em branco (campo CPF/CNPJ 14).
	if got := field(r02, 27, 14); got != "12345678909   " {
		t.Errorf("R02 CPF/CNPJ do locador [27..40] = %q", got)
	}
	if got := strings.TrimSpace(field(r02, 41, 60)); got != "FABIO RAMOS" {
		t.Errorf("R02 nome do locador = %q", got)
	}
	if got := field(r02, 101, 14); got != "98765432100   " {
		t.Errorf("R02 CPF/CNPJ do locatário [101..114] = %q", got)
	}
	if got := strings.TrimSpace(field(r02, 115, 60)); got != "CARLA MENEZES" {
		t.Errorf("R02 nome do locatário = %q", got)
	}
	if got := strings.TrimSpace(field(r02, 694, 60)); got != "RUA VITORIA REGIA 152" {
		t.Errorf("R02 endereço do imóvel = %q", got)
	}
	for _, tc := range []struct{ start, width int }{{766, 20}, {788, 10}} {
		if got := field(r02, tc.start, tc.width); strings.TrimSpace(got) != "" {
			t.Errorf("R02 reservado [%d..%d] deve ser branco, veio %q", tc.start, tc.start+tc.width-1, got)
		}
	}

	// T9 — encerramento: "T9" e o resto reservado em branco. O leiaute não
	// prevê CNPJ nem contagem de linhas neste registro.
	t9 := lines[3]
	if got := field(t9, 1, 2); got != "T9" {
		t.Errorf("T9[1..2] = %q, esperado T9", got)
	}
	if rest := strings.TrimRight(field(t9, 3, widthT9-2), " "); rest != "" {
		t.Errorf("T9[3..102] deve ser reservado em branco, veio %q", rest)
	}
}

// O sequencial da locação tem 5 posições: com 7 (como já esteve) todos os
// campos seguintes andam dois caracteres e o programa lê outro registro.
func TestRender_SequencialComCincoPosicoesNaoDeslocaOsDemais(t *testing.T) {
	lines := renderLines(t, Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental(), sampleRental()}})

	if len(lines) != 5 {
		t.Fatalf("esperava 5 linhas (header + R01 + 2 R02 + T9), veio %d", len(lines))
	}
	if got := field(lines[2], 22, 5); got != "00001" {
		t.Errorf("primeiro sequencial = %q, esperado 00001", got)
	}
	if got := field(lines[3], 22, 5); got != "00002" {
		t.Errorf("segundo sequencial = %q, esperado 00002", got)
	}
	// O campo seguinte começa imediatamente depois do sequencial.
	if got := field(lines[3], 27, 14); got != "12345678909   " {
		t.Errorf("CPF/CNPJ do locador após o sequencial = %q", got)
	}
	for i := 2; i <= 3; i++ {
		if len(lines[i]) != widthR02 {
			t.Errorf("R02 %d com %d posições, o leiaute exige %d", i-1, len(lines[i]), widthR02)
		}
	}
}

func TestRender_SanitizaAcentosEPontuacao(t *testing.T) {
	r := sampleRental()
	r.LocadorNome = "José Álvares Filho"
	r.Endereco = "Av. Presidente Antônio Carlos, nº 123 - 5º andar"
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{r}}

	lines := renderLines(t, d)
	if strings.Contains(lines[2], "José") || strings.Contains(lines[2], "Álvares") {
		t.Errorf("R02 não deveria conter acentuação: %q", lines[2][32:220])
	}
	if got := strings.TrimSpace(field(lines[2], 41, 60)); got != "JOSE ALVARES FILHO" {
		t.Errorf("nome transliterado ausente: %q", got)
	}
	for _, line := range lines {
		for i := 0; i < len(line); i++ {
			if line[i] >= 0x80 {
				t.Fatalf("caractere não-ASCII na linha %q (posição %d)", line[:30], i)
			}
		}
	}
}

func TestRender_ValoresEmCentavos(t *testing.T) {
	r := sampleRental()
	r.MonthlyRent = [12]float64{}
	r.MonthlyRent[0] = 15000.5 // R$ 15.000,50 -> parte inteira 12 + 2 decimais
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{r}}

	lines := renderLines(t, d)
	if got := field(lines[2], 189, 14); got != "00000001500050" {
		t.Errorf("aluguel de janeiro = %q, esperado 00000001500050", got)
	}
}

func TestRender_Retificadora(t *testing.T) {
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental()}}
	d.Declarante.Retificadora = true
	d.Declarante.Recibo = "1234567890"

	lines := renderLines(t, d)
	if got := field(lines[1], 22, 1); got != "1" {
		t.Errorf("retificadora = %q, esperado 1", got)
	}
	if got := field(lines[1], 23, 10); got != "1234567890" {
		t.Errorf("recibo = %q", got)
	}
}

func TestRender_SituacaoEspecialComDataDDMMAAAA(t *testing.T) {
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental()}}
	d.Declarante.SituacaoEspecialCodigo = "01"
	d.Declarante.SituacaoEspecialData = "31012025"

	lines := renderLines(t, d)
	if got := field(lines[1], 33, 1); got != "1" {
		t.Errorf("situação especial = %q, esperado 1", got)
	}
	if got := field(lines[1], 34, 8); got != "31012025" {
		t.Errorf("data do evento = %q, esperado 31012025", got)
	}
	if got := field(lines[1], 42, 2); got != "01" {
		t.Errorf("código da situação especial = %q, esperado 01", got)
	}
}

func TestRender_NumeroDoContratoNasSeisPosicoes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contrato string
		want     string
	}{
		{"numérico curto é completado com zeros", "123", "000123"},
		{"numérico com 6 posições", "123456", "123456"},
		{"numérico maior é truncado pelo começo", "1234567", "234567"},
		{"alfanumérico fica à esquerda", "A123", "A123  "},
		{"vazio vira zeros", "", "000000"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := sampleRental()
			r.Contrato = tc.contrato
			lines := renderLines(t, Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{r}})
			if got := field(lines[2], 175, 6); got != tc.want {
				t.Errorf("número do contrato = %q, esperado %q", got, tc.want)
			}
		})
	}
}

func TestValidate_Bloqueantes(t *testing.T) {
	cases := map[string]func(*Declaration){
		"CNPJ declarante inválido":       func(d *Declaration) { d.Declarante.CNPJ = "123" },
		"nome da declarante vazio":       func(d *Declaration) { d.Declarante.Nome = " " },
		"endereço do imóvel vazio":       func(d *Declaration) { d.Rentals[0].Endereco = " " },
		"CEP inválido":                   func(d *Declaration) { d.Rentals[0].CEP = "12" },
		"documento da contraparte torto": func(d *Declaration) { d.Rentals[0].LocadorDoc = "1234" },
		"retificadora sem recibo": func(d *Declaration) {
			d.Declarante.Retificadora = true
			d.Declarante.Recibo = ""
		},
		// O formato invertido (AAAAMMDD) cai em mês inválido e bloqueia a
		// geração: melhor recusar do que entregar um arquivo com data trocada.
		"data do contrato em AAAAMMDD": func(d *Declaration) { d.Rentals[0].DataContrato = "20250110" },
		"data do contrato impossível":  func(d *Declaration) { d.Rentals[0].DataContrato = "32132025" },
		"situação especial com data invertida": func(d *Declaration) {
			d.Declarante.SituacaoEspecialCodigo = "01"
			d.Declarante.SituacaoEspecialData = "20250131"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental()}}
			mutate(&d)
			if _, err := d.Validate(); err == nil {
				t.Fatalf("esperava erro bloqueante, veio nil")
			}
		})
	}

	// CEP com máscara passa pela normalização.
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental()}}
	d.Rentals[0].CEP = "14032-000"
	if _, err := d.Validate(); err != nil {
		t.Fatalf("CEP mascarado deveria passar, veio %v", err)
	}
}

func TestValidate_Advertencias(t *testing.T) {
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{sampleRental()}}
	d.Rentals[0].Contrato = ""
	d.Rentals[0].DataContrato = ""
	d.Rentals[0].CodigoMunicipio = ""

	warnings, err := d.Validate()
	if err != nil {
		t.Fatalf("esperava validação com advertências, veio erro: %v", err)
	}
	if len(warnings) != 2 {
		t.Fatalf("esperava 2 advertências (contrato e data), veio %d: %v", len(warnings), warnings)
	}
}

func TestRender_AdverteSobreFichasDeVendaNaoEmitidas(t *testing.T) {
	d := Declaration{
		Declarante:          sampleDeclarante(),
		Rentals:             []Rental{sampleRental()},
		SalesIntermediation: []SaleIntermediation{{CompradorDoc: "12345678909", CompradorNome: "Comprador"}},
	}

	res, err := Render(d)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "R03/R04") {
		t.Fatalf("esperava advertência sobre R03/R04, veio %v", res.Warnings)
	}
}

func TestRender_ContratoEmBranco(t *testing.T) {
	r := sampleRental()
	r.Contrato = ""
	r.DataContrato = ""
	d := Declaration{Declarante: sampleDeclarante(), Rentals: []Rental{r}}

	lines := renderLines(t, d)
	if got := field(lines[2], 175, 6); got != "000000" {
		t.Errorf("contrato em branco deveria ser zeros: %q", got)
	}
	if got := field(lines[2], 181, 8); got != "00000000" {
		t.Errorf("data em branco deveria ser zeros: %q", got)
	}
}
