// check-internal-caller-key garante que todo serviço que faz chamada interna
// (usa pkg/clients) tenha, no compose de produção, a variável que permite dar a
// ele a própria credencial interna.
//
// Por que: hoje a frota compartilha uma única INTERNAL_API_KEY, aceita como
// identidade "legacy" por todo receptor — qualquer serviço com essa chave pode
// chamar qualquer rota interna de qualquer tenant. pkg/auth/service_identity.go
// já sabe resolver chaves NOMEADAS (INTERNAL_SERVICE_KEYS, com allowlist de
// tenant por chamador), e pkg/clients passa a preferir INTERNAL_CALLER_KEY
// quando ela existe. Sem a variável no compose, a migração de um serviço novo
// fica impossível sem editar YAML — que é exatamente o tipo de passo que se
// esquece.
//
// Regra: módulo que importa pkg/clients (fora de testes) → o bloco do mesmo
// serviço em docker-compose.prod.yml precisa declarar INTERNAL_CALLER_KEY.
//
// Usage:
//
//	go run ./scripts/check-internal-caller-key -root ../..
//
// Exit codes: 0 ok, 1 violação, 2 erro de uso/IO.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var clientsImportRe = regexp.MustCompile(`"github\.com/oginoino/toca/pkg/clients"`)

// composeServiceRe delimita os blocos de serviço do compose (dois espaços).
var composeServiceRe = regexp.MustCompile(`(?m)^  ([a-z0-9][a-z0-9-]*):\s*$`)

func main() {
	root := flag.String("root", ".", "raiz do repositório")
	flag.Parse()

	modules := []string{
		"gateway", "toca-auth", "toca-core", "toca-crm", "toca-deal",
		"toca-comm", "toca-financial", "toca-signature", "toca-ops",
		"toca-calendar", "toca-portal", "toca-ai", "toca-mcp",
	}

	composePath := filepath.Join(*root, "docker-compose.prod.yml")
	compose, err := os.ReadFile(composePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-internal-caller-key: %v\n", err)
		os.Exit(2)
	}
	blocks := composeBlocks(string(compose))

	var violations []string
	for _, module := range modules {
		uses, err := moduleImportsClients(filepath.Join(*root, module))
		if err != nil {
			fmt.Fprintf(os.Stderr, "check-internal-caller-key: %v\n", err)
			os.Exit(2)
		}
		if !uses {
			continue
		}
		block, ok := blocks[module]
		if !ok {
			violations = append(violations, fmt.Sprintf("%s usa pkg/clients mas não tem bloco em docker-compose.prod.yml", module))
			continue
		}
		if !strings.Contains(block, "INTERNAL_CALLER_KEY") {
			violations = append(violations, fmt.Sprintf("%s usa pkg/clients mas o bloco do compose não declara INTERNAL_CALLER_KEY (a migração para chave por serviço fica impossível sem editar YAML)", module))
		}
	}
	sort.Strings(violations)
	for _, v := range violations {
		fmt.Println(v)
	}
	if len(violations) > 0 {
		fmt.Printf("check-internal-caller-key: %d violação(ões)\n", len(violations))
		os.Exit(1)
	}
	fmt.Println("check-internal-caller-key: ok")
}

// moduleImportsClients procura o import de pkg/clients no código do módulo
// (arquivos de teste não contam: são dublês, não chamada real).
func moduleImportsClients(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return false, nil // serviço sem módulo no checkout
	}
	found := false
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil || found {
			return walkErr
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if clientsImportRe.Match(data) {
			found = true
		}
		return nil
	})
	return found, err
}

// composeBlocks separa o compose por serviço de primeiro nível.
func composeBlocks(content string) map[string]string {
	blocks := map[string]string{}
	matches := composeServiceRe.FindAllStringSubmatchIndex(content, -1)
	for i, m := range matches {
		name := content[m[2]:m[3]]
		start := m[1]
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		blocks[name] = content[start:end]
	}
	return blocks
}
