// Package dimob gera o arquivo TXT de importação da DIMOB (Declaração de
// Informações sobre Atividades Imobiliárias) no leiaute oficial do programa
// da Receita Federal (PGD DIMOB).
//
// # Leiaute
//
// Arquivo texto em linhas de LARGURA FIXA terminadas por CRLF (0D0A, o
// "delimitador de registro" de 2 posições de cada registro), apenas
// caracteres ASCII (o programa não aceita acentuação nem separadores).
//
//	Header — identificação do arquivo: "DIMOB" + 369 posições reservadas (374)
//	R01    — Ficha 01, dados do declarante                             (270)
//	R02    — Ficha 02, locação/sublocação e intermediação de locação    (797)
//	         (12 meses × aluguel/comissão/imposto + dados do imóvel)
//	R03    — Ficha 03, construção/incorporação/loteamento (estrutura suportada;
//	         o serviço ainda não emite — depende de listagem de contratos)
//	R04    — Ficha 04, intermediação de venda (estrutura suportada)
//	T9     — encerramento: "T9" + 99 posições reservadas                (102)
//
// Convenções de campo do leiaute: valor monetário em 14 posições à direita,
// zero à esquerda e DUAS casas decimais (centavos, portanto); data em
// DDMMAAAA; ano em 4 posições; campo alfanumérico alinhado à esquerda e
// completado com brancos; campo numérico alinhado à direita e completado com
// zeros; CPF em 11 posições e CNPJ em 14; CPF/CNPJ ocupa 14 posições à
// esquerda quando o documento é CPF (brancos à direita).
//
// Este pacote confere a largura de cada registro antes de devolver o arquivo:
// um caractere a mais ou a menos desloca todos os campos seguintes e o
// programa recusa (ou pior, lê lixo). Datas também são validadas na forma
// DDMMAAAA — o inverso (AAAAMMDD) cai em mês inválido e bloqueia a geração em
// vez de produzir um arquivo errado.
//
// Referências: leiaute F661OIM (documentação de mercado que reproduz o leiaute
// do programa, inclusive as observações de 6 posições para o número do
// contrato), convenções de formato do leiaute DIMOB e as implementações de
// referência de mercado (SpedBr/fiscalbr-net). IN RFB nº 1115/2010 e Perguntas
// e Respostas oficiais para as regras de valoração e prazo.
package dimob

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ── Constantes do leiaute ────────────────────────────────────────────────

const (
	// recordEOL é o delimitador de registro: CRLF, 2 bytes.
	recordEOL = "\r\n"

	// Larguras oficiais de cada registro, SEM o delimitador. Render recusa
	// qualquer linha que não feche exatamente nesta largura.
	widthHeader = 374 // "DIMOB" (5) + reservado (369)
	widthR01    = 270
	widthR02    = 797
	widthT9     = 102 // "T9" (2) + reservado (99)
)

// ── Modelos ───────────────────────────────────────────────────────────────

// Declarante — a pessoa jurídica que entrega a DIMOB (a imobiliária).
type Declarante struct {
	// CNPJ: 14 dígitos, sem máscara.
	CNPJ string
	// Nome empresarial (razão social), 60 posições.
	Nome string
	// AnoBase é o ano-calendário das operações declaradas (ex.: 2025 para a
	// DIMOB entregue em 2026). É o ano do R01 — o header do arquivo não o
	// carrega.
	AnoBase int
	// Retificadora indica que esta declaração substitui uma anterior.
	Retificadora bool
	// Recibo da declaração substituída (obrigatório se retificadora).
	Recibo string
	// SituacaoEspecialCodigo — código de 2 dígitos do evento especial
	// ("00" normal, "01" extinção, "02" fusão, "03" incorporação, "04" cisão).
	SituacaoEspecialCodigo string
	// SituacaoEspecialData — data do evento em DDMMAAAA (quando houver).
	SituacaoEspecialData string
	// CPFResponsavel — CPF (11 dígitos) do responsável da PJ perante a RFB.
	CPFResponsavel string
	// Endereco — endereço completo do contribuinte, 120 posições.
	Endereco string
	// UF do contribuinte.
	UF string
	// CodigoMunicipio — código de município da tabela do próprio programa da
	// DIMOB (Anexo A das instruções de importação), 4 dígitos. NÃO é o código
	// IBGE de 7 dígitos: o programa usa a tabela fiscal própria. Vazio vira
	// advertência (o programa aceita, mas o campo fica em branco).
	CodigoMunicipio string
}

// Rental — operação de locação/sublocação (registro R02). Intermediação de
// locação entra na mesma ficha: valores de comissão na coluna Comissão.
type Rental struct {
	// LocadorDoc/LocatarioDoc — CPF (11) ou CNPJ (14), sem máscara. O campo
	// tem 14 posições: CPF fica à esquerda com brancos à direita.
	LocadorDoc    string
	LocadorNome   string
	LocatarioDoc  string
	LocatarioNome string
	// Contrato — número do contrato (até 6 posições). O programa suporta
	// exatamente 6 posições nesta ficha: números maiores são truncados pelo
	// começo. Vazio gera advertência.
	Contrato string
	// DataContrato — data do contrato em DDMMAAAA (leiaute). Vazio gera
	// advertência; qualquer outro formato é recusado na validação.
	DataContrato      string
	MonthlyRent       [12]float64
	MonthlyCommission [12]float64
	// MonthlyTax — imposto retido na fonte (IRRF), quando houver.
	MonthlyTax [12]float64
	// TipoImovel: "U" (urbano) ou "R" (rural). Vazio cai em "U".
	TipoImovel string
	// Endereco — logradouro + número (até 60). Obrigatório: sem endereço o
	// programa bloqueia a importação.
	Endereco string
	// CEP — 8 dígitos, sem máscara. Obrigatório (bloqueia se ausente).
	CEP string
	// CodigoMunicipio — código do município do imóvel na tabela do programa
	// (Anexo A), 4 dígitos — não é o código IBGE de 7 dígitos.
	CodigoMunicipio string
	UF              string
}

// SaleConstruction — operação de construção/incorporação/loteamento (R03).
type SaleConstruction struct {
	CompradorDoc    string
	CompradorNome   string
	Contrato        string
	DataContrato    string // AAAAMMDD
	ValorOperacao   float64
	ValorPagoAno    float64
	TipoImovel      string
	Endereco        string
	CEP             string
	CodigoMunicipio string
	UF              string
}

// SaleIntermediation — intermediação de venda (R04).
type SaleIntermediation struct {
	CompradorDoc    string
	CompradorNome   string
	VendedorDoc     string
	VendedorNome    string
	Contrato        string
	DataContrato    string // AAAAMMDD
	ValorVenda      float64
	ValorComissao   float64
	TipoImovel      string
	Endereco        string
	CEP             string
	CodigoMunicipio string
	UF              string
}

// Declaration — entrada completa do gerador.
type Declaration struct {
	Declarante          Declarante
	Rentals             []Rental
	SalesConstruction   []SaleConstruction
	SalesIntermediation []SaleIntermediation
}

// Result — saída da renderização: conteúdo + advertências não bloqueantes.
type Result struct {
	Content  string
	Warnings []string
}

// ErrInvalidDeclaration — entrada inconsistente. Caller trata como 400.
var ErrInvalidDeclaration = errors.New("dimob: declaração inválida")

// ── Validação ─────────────────────────────────────────────────────────────

var cnpjDigits = regexp.MustCompile(`^\d{14}$`)
var docDigits = regexp.MustCompile(`^\d{11}$|^\d{14}$`)
var cepDigits = regexp.MustCompile(`^\d{8}$`)
var municipioDigits = regexp.MustCompile(`^\d{4}$`)

// Validate confere os requisitos bloqueantes do programa da RFB e devolve as
// advertências não bloqueantes. Erros bloqueantes: declarante sem CNPJ/nome,
// documento de contraparte com formato inválido, endereço ou CEP ausente no
// imóvel. Advertências: número/data do contrato ausentes, código de
// município ausente — o programa aceita com aviso.
func (d Declaration) Validate() ([]string, error) {
	var warnings []string

	digits := onlyDigits(d.Declarante.CNPJ)
	if !cnpjDigits.MatchString(digits) {
		return nil, fmt.Errorf("%w: CNPJ da declarante deve ter 14 dígitos", ErrInvalidDeclaration)
	}
	if strings.TrimSpace(d.Declarante.Nome) == "" {
		return nil, fmt.Errorf("%w: nome empresarial da declarante obrigatório", ErrInvalidDeclaration)
	}
	if d.Declarante.AnoBase < 2000 || d.Declarante.AnoBase > 2100 {
		return nil, fmt.Errorf("%w: ano-base %d fora de faixa plausível", ErrInvalidDeclaration, d.Declarante.AnoBase)
	}
	if d.Declarante.Retificadora && onlyDigits(d.Declarante.Recibo) == "" {
		return nil, fmt.Errorf("%w: declaração retificadora exige o número do recibo da declaração substituída", ErrInvalidDeclaration)
	}
	if onlyDigits(d.Declarante.CPFResponsavel) != "" && !docDigits.MatchString(onlyDigits(d.Declarante.CPFResponsavel)) {
		return nil, fmt.Errorf("%w: CPF do responsável deve ter 11 dígitos", ErrInvalidDeclaration)
	}
	// A data do evento de situação especial é DDMMAAAA no leiaute. Validar a
	// forma evita entregar um arquivo com dia/ano invertidos (o programa leria
	// um mês inválido, ou pior, aceitaria silenciosamente).
	if strings.TrimSpace(d.Declarante.SituacaoEspecialData) != "" && !validDimobDate(d.Declarante.SituacaoEspecialData) {
		return nil, fmt.Errorf("%w: data do evento de situação especial %q fora do formato DDMMAAAA do leiaute", ErrInvalidDeclaration, d.Declarante.SituacaoEspecialData)
	}

	for i, r := range d.Rentals {
		if err := validateDoc(fmt.Sprintf("locação %d (locador)", i+1), r.LocadorDoc); err != nil {
			return nil, err
		}
		if err := validateDoc(fmt.Sprintf("locação %d (locatário)", i+1), r.LocatarioDoc); err != nil {
			return nil, err
		}
		if strings.TrimSpace(r.Endereco) == "" {
			return nil, fmt.Errorf("%w: locação %d sem endereço do imóvel — a RFB bloqueia a importação sem endereço", ErrInvalidDeclaration, i+1)
		}
		if !cepDigits.MatchString(onlyDigits(r.CEP)) {
			return nil, fmt.Errorf("%w: locação %d com CEP ausente ou inválido (%q) — a RFB bloqueia a importação sem CEP válido", ErrInvalidDeclaration, i+1, r.CEP)
		}
		if onlyDigits(r.LocadorDoc) == "" {
			warnings = append(warnings, fmt.Sprintf("locação %d sem CPF/CNPJ do locador — a RFB pode exigir o documento", i+1))
		}
		if onlyDigits(r.LocatarioDoc) == "" {
			warnings = append(warnings, fmt.Sprintf("locação %d sem CPF/CNPJ do locatário — a RFB pode exigir o documento", i+1))
		}
		if strings.TrimSpace(onlyDigits(r.Contrato)) == "" {
			warnings = append(warnings, fmt.Sprintf("locação %d sem número de contrato — o programa aceita com advertência", i+1))
		}
		if strings.TrimSpace(r.DataContrato) == "" {
			warnings = append(warnings, fmt.Sprintf("locação %d sem data de contrato — o programa aceita com advertência", i+1))
		} else if !validDimobDate(r.DataContrato) {
			return nil, fmt.Errorf("%w: locação %d com data de contrato %q fora do formato DDMMAAAA do leiaute", ErrInvalidDeclaration, i+1, r.DataContrato)
		}
		if onlyDigits(r.CodigoMunicipio) != "" && !municipioDigits.MatchString(onlyDigits(r.CodigoMunicipio)) {
			warnings = append(warnings, fmt.Sprintf("locação %d com código de município inválido (%q)", i+1, r.CodigoMunicipio))
		}
	}
	return warnings, nil
}

func validateDoc(label, doc string) error {
	d := onlyDigits(doc)
	if d != "" && !docDigits.MatchString(d) {
		return fmt.Errorf("%w: %s documento %q não é CPF/CNPJ válido — a RFB exige CPF/CNPJ brasileiro, inclusive para não residentes", ErrInvalidDeclaration, label, doc)
	}
	return nil
}

// ── Renderização ──────────────────────────────────────────────────────────

// Render produz o arquivo de importação no leiaute oficial.
//
// Cada registro é montado campo a campo e conferido contra a largura oficial:
// um caractere a mais ou a menos desloca todos os campos seguintes, então o
// desvio vira erro em vez de arquivo errado.
func Render(d Declaration) (Result, error) {
	warnings, err := d.Validate()
	if err != nil {
		return Result{}, err
	}

	var b strings.Builder
	// writeRecord fecha um registro: confere a largura e acrescenta o
	// delimitador (CRLF).
	writeRecord := func(expectedWidth int, fields ...string) error {
		record := strings.Join(fields, "")
		if len(record) != expectedWidth {
			return fmt.Errorf("%w: registro %q com %d posições, o leiaute exige %d", ErrInvalidDeclaration, recordType(record), len(record), expectedWidth)
		}
		b.WriteString(record)
		b.WriteString(recordEOL)
		return nil
	}

	ano := padLeft(fmt.Sprintf("%d", d.Declarante.AnoBase), 4)
	cnpj := padLeft(onlyDigits(d.Declarante.CNPJ), 14)
	nome := sanitizeASCII(d.Declarante.Nome)

	// Header — identificação do arquivo: "DIMOB" e o resto RESERVADO em branco.
	// O leiaute não carrega ano, CNPJ, razão social nem versão neste registro:
	// quem declara isso é o R01.
	if err := writeRecord(widthHeader, "DIMOB", spaces(widthHeader-len("DIMOB"))); err != nil {
		return Result{}, err
	}

	// R01 — Ficha 01, dados iniciais.
	retificadora := "0"
	if d.Declarante.Retificadora {
		retificadora = "1"
	}
	situacao := "0"
	sitCodigo := padLeft(onlyDigits(d.Declarante.SituacaoEspecialCodigo), 2)
	sitData := padLeft(onlyDigits(d.Declarante.SituacaoEspecialData), 8)
	if onlyDigits(d.Declarante.SituacaoEspecialCodigo) != "" {
		situacao = "1"
	} else {
		sitCodigo, sitData = "00", "00000000"
	}
	if err := writeRecord(widthR01,
		"R01",
		cnpj,
		ano,
		retificadora,
		padLeft(onlyDigits(d.Declarante.Recibo), 10),
		situacao,
		sitData,
		sitCodigo,
		padRight(nome, 60),
		padLeft(onlyDigits(d.Declarante.CPFResponsavel), 11),
		padRight(sanitizeASCII(d.Declarante.Endereco), 120),
		padRight(sanitizeASCII(d.Declarante.UF), 2),
		padLeft(onlyDigits(d.Declarante.CodigoMunicipio), 4),
		spaces(20),
		spaces(10),
	); err != nil {
		return Result{}, err
	}

	// R02 — Ficha 02, locação: uma linha por locação, com o sequencial de 5
	// posições (22..26) e os 12 meses (aluguel, comissão e imposto) em 14
	// posições cada.
	for i, r := range d.Rentals {
		fields := []string{
			"R02",
			cnpj,
			ano,
			padLeft(fmt.Sprintf("%d", i+1), 5),
			padRight(onlyDigits(r.LocadorDoc), 14),
			padRight(sanitizeASCII(r.LocadorNome), 60),
			padRight(onlyDigits(r.LocatarioDoc), 14),
			padRight(sanitizeASCII(r.LocatarioNome), 60),
			contractNumber(r.Contrato),
			padLeft(onlyDigits(r.DataContrato), 8),
		}
		for m := 0; m < 12; m++ {
			fields = append(fields, amount(r.MonthlyRent[m]), amount(r.MonthlyCommission[m]), amount(r.MonthlyTax[m]))
		}
		fields = append(fields,
			tipoImovel(r.TipoImovel),
			padRight(sanitizeASCII(r.Endereco), 60),
			padLeft(onlyDigits(r.CEP), 8),
			padLeft(onlyDigits(r.CodigoMunicipio), 4),
			spaces(20),
			padRight(sanitizeASCII(r.UF), 2),
			spaces(10),
		)
		if err := writeRecord(widthR02, fields...); err != nil {
			return Result{}, err
		}
	}

	// T9 — encerramento: "T9" e o resto RESERVADO em branco. O leiaute não
	// prevê CNPJ nem contagem de linhas aqui; o programa conta os registros
	// ao importar o arquivo.
	if err := writeRecord(widthT9, "T9", spaces(widthT9-len("T9"))); err != nil {
		return Result{}, err
	}

	// As fichas de venda (R03/R04) ainda não são emitidas. Se o chamador
	// passou operações, avisar em vez de omiti-las em silêncio: omissão de
	// operação na DIMOB é infração com multa sobre o valor omitido.
	if omitted := len(d.SalesConstruction) + len(d.SalesIntermediation); omitted > 0 {
		warnings = append(warnings, fmt.Sprintf("%d operação(ões) de venda não foram emitidas: as fichas R03/R04 ainda não são geradas — declare-as no programa antes de transmitir", omitted))
	}

	return Result{Content: b.String(), Warnings: warnings}, nil
}

// recordType devolve o identificador do registro para mensagens de erro.
func recordType(record string) string {
	if len(record) >= 3 {
		return record[:3]
	}
	return record
}

// amount formata um valor monetário em 14 posições, alinhado à direita e com
// zeros à esquerda (ex.: R$ 15.000,50 → "000000001500050"). É a convenção do
// leiaute — 12 posições de parte inteira e 2 de decimais — escrita a partir dos
// centavos, que produzem exatamente a mesma cadeia.
func amount(v float64) string {
	cents := int64(v*100 + 0.5)
	return padLeft(fmt.Sprintf("%d", cents), 14)
}

// contractNumber escreve o número do contrato nas 6 posições da ficha (o
// programa suporta exatamente 6 — números maiores são truncados pelo começo).
// O leiaute não define alinhamento para este campo: número puro vai como campo
// numérico (zeros à esquerda) e valor com letras fica à esquerda, como o campo
// alfanumérico que é. Vazio vira zeros.
func contractNumber(raw string) string {
	trimmed := sanitizeASCII(raw)
	if trimmed == "" {
		// Sem número: zeros, como o campo numérico do leiaute (o programa
		// aceita com advertência).
		return padLeft("", 6)
	}
	if onlyDigits(trimmed) == trimmed {
		return padLeft(trimmed, 6)
	}
	return padRight(trimmed, 6)
}

// validDimobDate confere a forma DDMMAAAA do leiaute. Além de exigir 8
// dígitos, valida dia/mês/ano plausíveis: é o que pega uma data entregue no
// formato invertido (AAAAMMDD), que de outra forma viraria mês 20 ou 26 no
// arquivo.
func validDimobDate(value string) bool {
	digits := onlyDigits(value)
	if len(digits) != 8 {
		return false
	}
	day, dayErr := strconv.Atoi(digits[0:2])
	month, monthErr := strconv.Atoi(digits[2:4])
	year, yearErr := strconv.Atoi(digits[4:8])
	if dayErr != nil || monthErr != nil || yearErr != nil {
		return false
	}
	return day >= 1 && day <= 31 && month >= 1 && month <= 12 && year >= 1900 && year <= 2200
}

// tipoImovel normaliza U/R, caindo em "U" (urbano) por padrão.
func tipoImovel(t string) string {
	if onlyASCIIUpper(t) == "R" {
		return "R"
	}
	return "U"
}

// ── Helpers de formatação ─────────────────────────────────────────────────

func padLeft(s string, width int) string {
	if len(s) > width {
		return s[len(s)-width:]
	}
	return strings.Repeat("0", width-len(s)) + s
}

func padRight(s string, width int) string {
	if len(s) > width {
		return s[:width]
	}
	return s + spaces(width-len(s))
}

func spaces(n int) string {
	return strings.Repeat(" ", n)
}

func onlyASCIIUpper(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// onlyDigits remove tudo que não é dígito (máscaras de CNPJ/CPF/CEP).
func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// sanitizeASCII converte o texto para o padrão do programa: caixa alta, sem
// acentuação (transliterada), sem pontuação — só letras, números e espaços.
// Caracteres fora do conjunto viram espaço; espaços múltiplos colapsam.
func sanitizeASCII(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ':
			b.WriteRune(r)
		case r == 'Á' || r == 'À' || r == 'Ã' || r == 'Â' || r == 'Ä':
			b.WriteRune('A')
		case r == 'É' || r == 'È' || r == 'Ê' || r == 'Ë':
			b.WriteRune('E')
		case r == 'Í' || r == 'Ì' || r == 'Î' || r == 'Ï':
			b.WriteRune('I')
		case r == 'Ó' || r == 'Ò' || r == 'Õ' || r == 'Ô' || r == 'Ö':
			b.WriteRune('O')
		case r == 'Ú' || r == 'Ù' || r == 'Û' || r == 'Ü':
			b.WriteRune('U')
		case r == 'Ç':
			b.WriteRune('C')
		case r == 'Ñ':
			b.WriteRune('N')
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
