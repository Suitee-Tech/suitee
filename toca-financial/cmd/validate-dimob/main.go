// Command validate-dimob confere um arquivo de importação da DIMOB contra o
// leiaute oficial, campo a campo, e diz o que impede (ou compromete) a
// importação no programa da Receita Federal.
//
// A conferência é posicional: cada registro é lido nas posições exatas do
// leiaute (header de 374, R01 de 270, R02 de 797 e T9 de 102 posições, todos
// terminados por CRLF), porque um caractere a mais em um registro desloca todos
// os campos seguintes — foi assim que um R02 com sequencial de 7 posições (em
// vez de 5) passou a ser lido errado pelo programa.
//
// Uso:
//
//	go run ./cmd/validate-dimob -file dimob-2026.txt
//	curl -s .../fiscal/dimob/2026/file | go run ./cmd/validate-dimob -file -
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oginoino/toca/toca-financial/internal/dimob"
)

func main() {
	file := flag.String("file", "", "arquivo a conferir (use - para ler da entrada padrão)")
	fields := flag.Bool("fields", true, "imprime o valor de cada campo, na posição do leiaute")
	flag.Parse()

	if strings.TrimSpace(*file) == "" {
		fmt.Fprintln(os.Stderr, "-file é obrigatório (use - para ler da entrada padrão)")
		os.Exit(2)
	}

	var (
		content []byte
		err     error
	)
	if *file == "-" {
		content, err = io.ReadAll(os.Stdin)
	} else {
		content, err = os.ReadFile(*file)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ler arquivo: %v\n", err)
		os.Exit(1)
	}

	report := dimob.ValidateFile(string(content))

	fmt.Printf("arquivo: %s (%d bytes)\n", *file, len(content))
	fmt.Printf("registros: %d\n", len(report.Records))
	for _, record := range report.Records {
		fmt.Printf("  linha %d: %-6s %d posições\n", record.Line, record.Type, record.Width)
	}
	if *fields {
		for _, record := range report.Records {
			fmt.Printf("\n── linha %d — %s (%d posições)\n", record.Line, record.Type, record.Width)
			for _, field := range record.Fields {
				value := field.Value
				if strings.TrimSpace(value) == "" {
					value = "(branco)"
				} else {
					value = fmt.Sprintf("%q", strings.TrimRight(value, " "))
				}
				fmt.Printf("  [%3d..%3d] %-38s %-9s %s\n",
					field.Spec.Start, field.Spec.End(), field.Spec.Name, field.Spec.Format, value)
			}
		}
	}

	errors := report.Errors()
	if len(report.Issues) == 0 {
		fmt.Println("\nnenhuma desconformidade encontrada: arquivo conforme o leiaute.")
		return
	}
	fmt.Println()
	for _, issue := range report.Issues {
		fmt.Println(issue)
	}
	fmt.Printf("\nerros: %d | avisos: %d\n", len(errors), len(report.Issues)-len(errors))
	if len(errors) > 0 {
		fmt.Println("resultado: REPROVADO — o programa da RFB recusa (ou lê errado) arquivo assim.")
		os.Exit(1)
	}
	fmt.Println("resultado: APROVADO com avisos — o programa importa, mas confira os pontos acima.")
}
