package dimob

import "fmt"

// Leiaute como DADO: a tabela de campos de cada registro, com posição inicial
// (1-based), largura e formato. Render monta os registros campo a campo e
// confere a largura; ValidateFile usa esta mesma tabela para ler o arquivo de
// volta e apontar, campo a campo, o que está fora do leiaute. Ter a tabela em
// um lugar só evita que o gerador e o conferidor divirjam.

// FieldFormat é o formato do campo no leiaute.
type FieldFormat string

const (
	// FormatAlpha — alfanumérico, alinhado à esquerda, brancos à direita.
	FormatAlpha FieldFormat = "X"
	// FormatNumeric — numérico, alinhado à direita, zeros à esquerda.
	FormatNumeric FieldFormat = "N"
	// FormatMoney — valor com 12 posições de parte inteira e 2 decimais.
	FormatMoney FieldFormat = "R$"
	// FormatDate — data em DDMMAAAA.
	FormatDate FieldFormat = "DATA"
	// FormatDocument — CPF (11) ou CNPJ (14) em campo de 14 posições.
	FormatDocument FieldFormat = "CPF/CNPJ"
	// FormatBlank — campo reservado, que o leiaute exige em branco.
	FormatBlank FieldFormat = "BRANCO"
)

// FieldSpec descreve um campo do leiaute.
type FieldSpec struct {
	Name   string
	Start  int // posição inicial, 1-based
	Width  int
	Format FieldFormat
}

// End é a última posição do campo.
func (f FieldSpec) End() int { return f.Start + f.Width - 1 }

// RecordSpec descreve um registro do arquivo.
type RecordSpec struct {
	Type   string // "header", "R01", "R02", "T9"
	Width  int    // largura oficial, sem o delimitador
	Fields []FieldSpec
}

var monthNames = [12]string{
	"Janeiro", "Fevereiro", "Marco", "Abril", "Maio", "Junho",
	"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro",
}

// Layout devolve a especificação dos registros que o gerador emite.
func Layout() []RecordSpec {
	r01 := RecordSpec{Type: "R01", Width: widthR01, Fields: []FieldSpec{
		{Name: "Tipo", Start: 1, Width: 3, Format: FormatAlpha},
		{Name: "CNPJ do declarante", Start: 4, Width: 14, Format: FormatNumeric},
		{Name: "Ano-calendario", Start: 18, Width: 4, Format: FormatNumeric},
		{Name: "Declaracao retificadora", Start: 22, Width: 1, Format: FormatNumeric},
		{Name: "Numero do recibo", Start: 23, Width: 10, Format: FormatNumeric},
		{Name: "Situacao especial", Start: 33, Width: 1, Format: FormatNumeric},
		{Name: "Data do evento situacao especial", Start: 34, Width: 8, Format: FormatDate},
		{Name: "Codigo da situacao especial", Start: 42, Width: 2, Format: FormatNumeric},
		{Name: "Nome empresarial", Start: 44, Width: 60, Format: FormatAlpha},
		{Name: "CPF do responsavel", Start: 104, Width: 11, Format: FormatNumeric},
		{Name: "Endereco do contribuinte", Start: 115, Width: 120, Format: FormatAlpha},
		{Name: "UF do contribuinte", Start: 235, Width: 2, Format: FormatAlpha},
		{Name: "Codigo do municipio do contribuinte", Start: 237, Width: 4, Format: FormatNumeric},
		{Name: "Reservado", Start: 241, Width: 20, Format: FormatBlank},
		{Name: "Reservado", Start: 261, Width: 10, Format: FormatBlank},
	}}

	r02 := RecordSpec{Type: "R02", Width: widthR02, Fields: []FieldSpec{
		{Name: "Tipo", Start: 1, Width: 3, Format: FormatAlpha},
		{Name: "CNPJ do declarante", Start: 4, Width: 14, Format: FormatNumeric},
		{Name: "Ano-calendario", Start: 18, Width: 4, Format: FormatNumeric},
		{Name: "Sequencial da locacao", Start: 22, Width: 5, Format: FormatNumeric},
		{Name: "CPF/CNPJ do locador", Start: 27, Width: 14, Format: FormatDocument},
		{Name: "Nome do locador", Start: 41, Width: 60, Format: FormatAlpha},
		{Name: "CPF/CNPJ do locatario", Start: 101, Width: 14, Format: FormatDocument},
		{Name: "Nome do locatario", Start: 115, Width: 60, Format: FormatAlpha},
		{Name: "Numero do contrato", Start: 175, Width: 6, Format: FormatAlpha},
		{Name: "Data do contrato", Start: 181, Width: 8, Format: FormatDate},
	}}
	position := 189
	for month := 0; month < 12; month++ {
		for _, column := range []string{"Aluguel", "Comissao", "Imposto"} {
			r02.Fields = append(r02.Fields, FieldSpec{
				Name:   fmt.Sprintf("%s de %s", column, monthNames[month]),
				Start:  position,
				Width:  14,
				Format: FormatMoney,
			})
			position += 14
		}
	}
	r02.Fields = append(r02.Fields,
		FieldSpec{Name: "Tipo do imovel", Start: 693, Width: 1, Format: FormatAlpha},
		FieldSpec{Name: "Endereco do imovel", Start: 694, Width: 60, Format: FormatAlpha},
		FieldSpec{Name: "CEP", Start: 754, Width: 8, Format: FormatNumeric},
		FieldSpec{Name: "Codigo do municipio do imovel", Start: 762, Width: 4, Format: FormatNumeric},
		FieldSpec{Name: "Reservado", Start: 766, Width: 20, Format: FormatBlank},
		FieldSpec{Name: "UF do imovel", Start: 786, Width: 2, Format: FormatAlpha},
		FieldSpec{Name: "Reservado", Start: 788, Width: 10, Format: FormatBlank},
	)

	header := RecordSpec{Type: "header", Width: widthHeader, Fields: []FieldSpec{
		{Name: "Sistema", Start: 1, Width: 5, Format: FormatAlpha},
		{Name: "Reservado", Start: 6, Width: 369, Format: FormatBlank},
	}}

	t9 := RecordSpec{Type: "T9", Width: widthT9, Fields: []FieldSpec{
		{Name: "Tipo", Start: 1, Width: 2, Format: FormatAlpha},
		{Name: "Reservado", Start: 3, Width: 100, Format: FormatBlank},
	}}

	return []RecordSpec{header, r01, r02, t9}
}

// SpecFor devolve a especificação do registro pelo identificador lido do
// arquivo ("DIMOB"/"R01"/"R02"/"R03"/"R04"/"T9").
func SpecFor(recordType string) (RecordSpec, bool) {
	for _, spec := range Layout() {
		if spec.Type == recordType {
			return spec, true
		}
	}
	// R03/R04 existem no leiaute mas ainda não são emitidos; validar um arquivo
	// que os traga é responsabilidade de outra rodada.
	return RecordSpec{}, false
}
