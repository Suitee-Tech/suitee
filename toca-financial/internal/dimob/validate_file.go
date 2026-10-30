package dimob

import (
	"fmt"
	"strings"

	"github.com/oginoino/toca/pkg/validation"
)

// Conferência de um arquivo DIMOB já gerado contra o leiaute oficial.
//
// É o mesmo olhar que o programa da RFB aplica na importação: primeiro a
// estrutura (delimitador CRLF, ASCII, largura de cada registro, ordem
// header → R01 → R02… → T9) e depois o conteúdo posicional de cada campo.
// Como os campos são de tamanho fixo, um caractere a mais em um registro
// desloca todos os campos seguintes — por isso a conferência é feita por
// POSIÇÃO, e não por separador.

// FieldValidation é o valor lido de um campo, na posição do leiaute.
type FieldValidation struct {
	Spec  FieldSpec
	Value string
}

// RecordValidation é um registro lido do arquivo.
type RecordValidation struct {
	Line   int // linha no arquivo (1-based)
	Type   string
	Width  int
	Fields []FieldValidation
}

// Issue é uma desconformidade encontrada no arquivo. Warning marca o que o
// programa aceita com advertência (ex.: município em branco); o resto bloqueia
// a importação ou, pior, entra errado na declaração.
type Issue struct {
	Line    int
	Record  string
	Field   string
	Start   int
	Width   int
	Message string
	Warning bool
}

// String formata a issue como uma linha de relatório.
func (i Issue) String() string {
	level := "ERRO"
	if i.Warning {
		level = "AVISO"
	}
	location := fmt.Sprintf("linha %d (%s)", i.Line, i.Record)
	if i.Field != "" {
		location += fmt.Sprintf(" campo %q [%d..%d]", i.Field, i.Start, i.Start+i.Width-1)
	}
	return fmt.Sprintf("[%s] %s: %s", level, location, i.Message)
}

// ValidationReport é o resultado da conferência.
type ValidationReport struct {
	Records []RecordValidation
	Issues  []Issue
}

// issueCollector acumula desconformidades de um registro. Existe para que os
// helpers de campo recebam o relatório por referência — um ValidationReport
// copiado por valor perderia os apensos feitos dentro dele.
type issueCollector struct {
	report *ValidationReport
	line   int
	record string
}

func (c issueCollector) add(field string, start, width int, warning bool, format string, args ...any) {
	c.report.Issues = append(c.report.Issues, Issue{
		Line: c.line, Record: c.record, Field: field, Start: start, Width: width,
		Message: fmt.Sprintf(format, args...), Warning: warning,
	})
}

func (c issueCollector) slice(value string, start, width int) string {
	if start-1+width > len(value) {
		return ""
	}
	return value[start-1 : start+width-1]
}

// HasErrors informa se há qualquer desconformidade bloqueante.
func (r ValidationReport) HasErrors() bool {
	for _, issue := range r.Issues {
		if !issue.Warning {
			return true
		}
	}
	return false
}

// Errors devolve as desconformidades bloqueantes.
func (r ValidationReport) Errors() []Issue {
	out := make([]Issue, 0, len(r.Issues))
	for _, issue := range r.Issues {
		if !issue.Warning {
			out = append(out, issue)
		}
	}
	return out
}

// ValidateFile confere o conteúdo de um arquivo DIMOB contra o leiaute.
func ValidateFile(content string) ValidationReport {
	report := ValidationReport{}
	add := func(line int, record, field string, start, width int, warning bool, format string, args ...any) {
		report.Issues = append(report.Issues, Issue{
			Line: line, Record: record, Field: field, Start: start, Width: width,
			Message: fmt.Sprintf(format, args...), Warning: warning,
		})
	}

	if content == "" {
		add(0, "-", "", 0, 0, false, "arquivo vazio")
		return report
	}
	if !strings.HasSuffix(content, recordEOL) {
		add(0, "-", "", 0, 0, true, "arquivo não termina com CRLF (delimitador de registro)")
	}
	// Um LF solto (sem CR) significa que o arquivo foi salvo com fim de linha
	// Unix: o programa lê o CRLF como delimitador e não encontraria registro.
	bareLF := strings.Count(content, "\n") - strings.Count(content, recordEOL)
	if bareLF > 0 {
		add(0, "-", "", 0, 0, false, "%d quebra(s) de linha sem CR — o delimitador de registro do leiaute é CRLF (0D0A)", bareLF)
	}

	lines := strings.Split(strings.TrimSuffix(content, recordEOL), recordEOL)
	seenT9 := false
	for index, line := range lines {
		lineNumber := index + 1
		if line == "" {
			add(lineNumber, "-", "", 0, 0, false, "linha vazia no meio do arquivo")
			continue
		}
		kind := recordKind(line)
		report.Records = append(report.Records, RecordValidation{Line: lineNumber, Type: kind, Width: len(line)})

		if index == 0 && kind != "header" {
			add(lineNumber, kind, "", 0, 0, false, "o primeiro registro do arquivo deve ser o header (\"DIMOB\"), veio %q", truncate(line, 3))
		}
		if index == 1 && kind != "R01" {
			add(lineNumber, kind, "", 0, 0, false, "o segundo registro deve ser o R01 (dados do declarante), veio %q", truncate(line, 3))
		}
		if seenT9 {
			add(lineNumber, kind, "", 0, 0, false, "registro depois do T9 — o T9 encerra o arquivo")
		}
		if kind == "T9" {
			seenT9 = true
		}
		if kind == "unknown" {
			add(lineNumber, kind, "", 0, 0, false, "tipo de registro desconhecido: %q", truncate(line, 3))
			continue
		}

		report.Records[len(report.Records)-1].Fields = fieldsOf(kind, line)

		spec, ok := SpecFor(kind)
		if !ok {
			continue
		}
		if len(line) != spec.Width {
			add(lineNumber, kind, "", 0, 0, false, "registro com %d posições, o leiaute exige %d", len(line), spec.Width)
		}
		for i := 0; i < len(line); i++ {
			if line[i] >= 0x80 {
				add(lineNumber, kind, "", i+1, 1, false, "caractere não-ASCII (0x%02X) — o programa só aceita ASCII", line[i])
				break
			}
		}
		checkRecord(&report, lineNumber, kind, line)
	}

	if last := recordKind(lines[len(lines)-1]); last != "T9" {
		add(len(lines), last, "", 0, 0, false, "o arquivo deve terminar com o registro T9 de encerramento")
	}
	return report
}

// recordKind identifica o registro pelas primeiras posições.
func recordKind(line string) string {
	switch {
	case strings.HasPrefix(line, "DIMOB"):
		return "header"
	case strings.HasPrefix(line, "R01"), strings.HasPrefix(line, "R02"),
		strings.HasPrefix(line, "R03"), strings.HasPrefix(line, "R04"):
		return line[:3]
	case strings.HasPrefix(line, "T9"):
		return "T9"
	default:
		return "unknown"
	}
}

func fieldsOf(kind, line string) []FieldValidation {
	spec, ok := SpecFor(kind)
	if !ok {
		return nil
	}
	out := make([]FieldValidation, 0, len(spec.Fields))
	for _, f := range spec.Fields {
		if f.End() > len(line) {
			break
		}
		out = append(out, FieldValidation{Spec: f, Value: line[f.Start-1 : f.End()]})
	}
	return out
}

// checkRecord aplica as regras de conteúdo de cada registro.
func checkRecord(report *ValidationReport, line int, kind, value string) {
	c := issueCollector{report: report, line: line, record: kind}
	slice := func(start, width int) string { return c.slice(value, start, width) }

	switch kind {
	case "header":
		if got := slice(1, 5); got != "DIMOB" {
			c.add("Sistema", 1, 5, false, "deve ser \"DIMOB\", veio %q", got)
		}
		if rest := strings.TrimRight(slice(6, 369), " "); rest != "" {
			c.add("Reservado", 6, 369, false, "o header não carrega dados: as posições 6..374 são reservadas e devem estar em branco, veio %q", truncate(rest, 40))
		}

	case "R01":
		cnpj := digitsOnly(slice(4, 14))
		if len(cnpj) != 14 {
			c.add("CNPJ do declarante", 4, 14, false, "deve ter 14 dígitos, veio %q", slice(4, 14))
		} else if !validation.IsValidCNPJ(cnpj) {
			c.add("CNPJ do declarante", 4, 14, false, "CNPJ %s não passa na validação dos dígitos verificadores", cnpj)
		}
		checkYear(c, slice(18, 4), "Ano-calendario", 18, 4)
		checkFlag(c, slice(22, 1), "Declaracao retificadora", 22, 1)
		if got := slice(23, 10); digitsOnly(got) != got {
			c.add("Numero do recibo", 23, 10, false, "deve ser numérico, veio %q", got)
		}
		checkFlag(c, slice(33, 1), "Situacao especial", 33, 1)
		checkDate(c, slice(34, 8), "Data do evento situacao especial", 34, 8, true)
		if codigo := slice(42, 2); codigo != "00" && codigo != "01" && codigo != "02" && codigo != "03" && codigo != "04" {
			c.add("Codigo da situacao especial", 42, 2, false, "deve ser 00 (normal), 01 (extinção), 02 (fusão), 03 (incorporação) ou 04 (cisão), veio %q", codigo)
		}
		if strings.TrimSpace(slice(44, 60)) == "" {
			c.add("Nome empresarial", 44, 60, false, "obrigatório")
		}
		if cpf := digitsOnly(slice(104, 11)); cpf != "" && !validation.IsValidCPF(cpf) {
			c.add("CPF do responsavel", 104, 11, true, "CPF %q não passa na validação dos dígitos verificadores", cpf)
		}
		if uf := strings.TrimSpace(slice(235, 2)); len(uf) != 2 {
			c.add("UF do contribuinte", 235, 2, true, "UF ausente ou inválida (%q)", slice(235, 2))
		}
		if isBlankCode(slice(237, 4)) {
			c.add("Codigo do municipio do contribuinte", 237, 4, true, "em branco/zerado — o programa usa o código do município da tabela dele (Anexo A das instruções de importação)")
		}

	case "R02":
		checkYear(c, slice(18, 4), "Ano-calendario", 18, 4)
		sequencial := slice(22, 5)
		if digitsOnly(sequencial) != sequencial || sequencial == "00000" {
			c.add("Sequencial da locacao", 22, 5, false, "deve ser numérico e diferente de zero, veio %q", sequencial)
		}
		checkDocument(c, slice(27, 14), "CPF/CNPJ do locador", 27, 14)
		checkDocument(c, slice(101, 14), "CPF/CNPJ do locatario", 101, 14)
		if strings.TrimSpace(slice(41, 60)) == "" {
			c.add("Nome do locador", 41, 60, true, "em branco — a ficha deve identificar a parte")
		}
		if strings.TrimSpace(slice(115, 60)) == "" {
			c.add("Nome do locatario", 115, 60, true, "em branco — a ficha deve identificar a parte")
		}
		if contrato := strings.TrimSpace(slice(175, 6)); contrato == "" || digitsOnly(contrato) == strings.Repeat("0", len(contrato)) {
			c.add("Numero do contrato", 175, 6, true, "em branco/zerado — o programa aceita com advertência, mas a ficha perde a referência do contrato")
		}
		checkDate(c, slice(181, 8), "Data do contrato", 181, 8, true)
		position := 189
		for month := 0; month < 12; month++ {
			for _, column := range []string{"Aluguel", "Comissao", "Imposto"} {
				field := fmt.Sprintf("%s de %s", column, monthNames[month])
				if got := slice(position, 14); digitsOnly(got) != got {
					c.add(field, position, 14, false, "deve ser numérico (12 posições inteiras + 2 decimais), veio %q", got)
				}
				position += 14
			}
		}
		if tipo := slice(693, 1); tipo != "U" && tipo != "R" {
			c.add("Tipo do imovel", 693, 1, false, "deve ser U (urbano) ou R (rural), veio %q", tipo)
		}
		if strings.TrimSpace(slice(694, 60)) == "" {
			c.add("Endereco do imovel", 694, 60, false, "obrigatório — a RFB bloqueia a importação sem o endereço do imóvel")
		}
		if cep := digitsOnly(slice(754, 8)); len(cep) != 8 {
			c.add("CEP", 754, 8, false, "deve ter 8 dígitos, veio %q", slice(754, 8))
		}
		if isBlankCode(slice(762, 4)) {
			c.add("Codigo do municipio do imovel", 762, 4, true, "em branco/zerado — o programa usa o código do município da tabela dele (Anexo A das instruções de importação)")
		}
		if uf := strings.TrimSpace(slice(786, 2)); len(uf) != 2 {
			c.add("UF do imovel", 786, 2, true, "UF ausente ou inválida (%q)", slice(786, 2))
		}

	case "T9":
		if got := slice(1, 2); got != "T9" {
			c.add("Tipo", 1, 2, false, "deve ser \"T9\", veio %q", got)
		}
		// O T9 é só encerramento: o leiaute reserva as posições 3..102.
		if rest := strings.TrimRight(slice(3, 100), " "); rest != "" {
			c.add("Reservado", 3, 100, false, "o T9 não carrega CNPJ nem contagem de registros: as posições 3..102 são reservadas e devem estar em branco, veio %q", truncate(rest, 40))
		}
	}
}

func checkYear(c issueCollector, value, field string, start, width int) {
	digits := digitsOnly(value)
	if len(digits) != 4 {
		c.add(field, start, width, false, "deve ter 4 dígitos, veio %q", value)
		return
	}
	if digits < "2005" || digits > "2100" {
		c.add(field, start, width, false, "ano %s fora da faixa aceita pelo programa (2005 a ano declarado)", digits)
	}
}

func checkFlag(c issueCollector, value, field string, start, width int) {
	if value != "0" && value != "1" {
		c.add(field, start, width, false, "deve ser 0 (não) ou 1 (sim), veio %q", value)
	}
}

// checkDate confere a forma DDMMAAAA. Zero é aceito quando o campo é opcional
// (data do evento de situação especial) ou quando a data do contrato não foi
// informada — os dois casos o programa aceita com advertência.
func checkDate(c issueCollector, value, field string, start, width int, zeroAllowed bool) {
	digits := digitsOnly(value)
	if len(digits) != 8 {
		c.add(field, start, width, false, "deve ter 8 dígitos (DDMMAAAA), veio %q", value)
		return
	}
	if digits == "00000000" {
		if !zeroAllowed {
			c.add(field, start, width, false, "data zerada")
		}
		return
	}
	if !validDimobDate(digits) {
		c.add(field, start, width, false, "data %q fora do formato DDMMAAAA do leiaute (dia/mês/ano inválidos — o formato AAAAMMDD cai aqui)", value)
	}
}

// checkDocument valida o campo CPF/CNPJ de 14 posições: CPF (11 dígitos) fica à
// esquerda com brancos à direita; CNPJ ocupa as 14. Vazio é advertência (o
// programa pode exigir o documento das partes).
func checkDocument(c issueCollector, value, field string, start, width int) {
	digits := digitsOnly(value)
	switch {
	case digits == "":
		c.add(field, start, width, true, "em branco — o programa pode exigir o CPF/CNPJ da parte")
	case len(digits) == 11:
		if !validation.IsValidCPF(digits) {
			c.add(field, start, width, false, "CPF %s não passa na validação dos dígitos verificadores", digits)
		}
		if strings.TrimRight(value, " ") != digits {
			c.add(field, start, width, false, "CPF deve ocupar as 11 primeiras posições com brancos à direita, veio %q", value)
		}
	case len(digits) == 14:
		if !validation.IsValidCNPJ(digits) {
			c.add(field, start, width, false, "CNPJ %s não passa na validação dos dígitos verificadores", digits)
		}
	default:
		c.add(field, start, width, false, "documento %q não é CPF (11) nem CNPJ (14)", value)
	}
}

// isBlankCode informa se um campo de código está preenchido com brancos ou com
// zeros — as duas formas que o gerador usa quando o dado não existe.
func isBlankCode(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == "" || trimmed == strings.Repeat("0", len(trimmed))
}

func digitsOnly(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] >= '0' && value[i] <= '9' {
			b.WriteByte(value[i])
		}
	}
	return b.String()
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "…"
}
