// Command check-permission-enforcement garante que toda permissão do catálogo
// tem QUEM A APLIQUE.
//
// Achado A7 (revisão de 2026-09-19) e N3 (reauditoria de 2026-09-20): o catálogo
// (`pkg/auth/permission_catalog.go`) alimenta a tela de Permissões, e uma
// permissão sem nenhum ponto de aplicação é promessa falsa — o admin revoga, a
// tela confirma e nada muda. A revisão original mediu 19 nessa situação; a
// reauditoria, 2 problemáticas. Este verificador existe para o número não voltar
// a crescer sem alguém decidir.
//
// Regra: toda permissão do catálogo precisa aparecer como literal de string em
// algum arquivo Go de PRODUÇÃO (fora do próprio catálogo) OU estar na allowlist
// abaixo, com o motivo. A allowlist é deliberada e pequena.
package main

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// uiOnlyPermissions são permissões que governam NAVEGAÇÃO/UI no cliente, não
// autorização de servidor. O frontend as lê (`toka/src/apps/admin/
// adminRouteRegistry.tsx`, `shared/permissions/Can.tsx`) e nenhum guard de rota
// as consulta — o que protege os dados por trás delas é a permissão de AÇÃO de
// cada endpoint.
//
// Cada entrada aqui PRECISA ter consumidor no frontend, e isso é verificado:
// declarar "é capacidade de UI" sem nenhuma tela usando era exatamente o defeito
// de `reports:export` (a doc afirmava que escondia o botão e nenhuma tela a
// consultava, então o toggle salvava e nada mudava).
var uiOnlyPermissions = map[string]string{
	"module:crm":          "navegação: habilita o módulo no cliente",
	"module:portfolio":    "navegação: habilita o módulo no cliente",
	"module:rentals":      "navegação: habilita o módulo no cliente",
	"module:finance":      "navegação: habilita o módulo no cliente",
	"module:intelligence": "navegação: habilita o módulo no cliente",
	"module:admin":        "navegação: habilita o módulo no cliente",
	"finance:export":      "capacidade de UI (esconde o botão); a exportação roda no navegador sobre dados já carregados — declarado em docs/features/ROLES-MATRIX.md",
	"reports:export":      "capacidade de UI: esconde o botão Exportar em Reports.tsx; não é controle de confidencialidade, pelo mesmo motivo",
}

var catalogPath = filepath.Join("pkg", "auth", "permission_catalog.go")

var catalogIDRe = regexp.MustCompile(`\{ID:\s*"([^"]+)"`)

// permissionLiteralRe casa o formato dos ids do catálogo (`recurso:acao`) usado
// como literal em qualquer arquivo Go de produção.
var permissionLiteralRe = regexp.MustCompile(`"([a-z0-9_]+:[a-z0-9_]+)"`)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	if _, err := os.Stat(filepath.Join(*root, "go.work")); err != nil {
		fmt.Fprintf(os.Stderr, "check-permission-enforcement: %s does not look like the repo root (no go.work)\n", *root)
		os.Exit(2)
	}

	catalogSrc, err := os.ReadFile(filepath.Join(*root, catalogPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-permission-enforcement: ler catálogo: %v\n", err)
		os.Exit(2)
	}
	ids := map[string]struct{}{}
	for _, m := range catalogIDRe.FindAllStringSubmatch(string(catalogSrc), -1) {
		ids[m[1]] = struct{}{}
	}
	if len(ids) == 0 {
		fmt.Fprintln(os.Stderr, "check-permission-enforcement: catálogo não devolveu nenhuma permissão")
		os.Exit(2)
	}

	referenced, err := collectReferences(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-permission-enforcement: %v\n", err)
		os.Exit(2)
	}

	var violations []string
	for id := range ids {
		if _, ok := referenced[id]; ok {
			continue
		}
		if _, uiOnly := uiOnlyPermissions[id]; uiOnly {
			continue
		}
		violations = append(violations, id)
	}
	sort.Strings(violations)

	// A allowlist também é contrato: quem está nela se declara capacidade de UI,
	// então precisa de uma tela consumindo. Sem esta checagem, uma entrada na
	// allowlist esconderia o defeito em vez de documentá-lo.
	frontendRefs, err := collectFrontendReferences(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-permission-enforcement: %v\n", err)
		os.Exit(2)
	}
	var orphanUI []string
	for id := range uiOnlyPermissions {
		if _, ok := frontendRefs[id]; !ok {
			orphanUI = append(orphanUI, id)
		}
	}
	sort.Strings(orphanUI)

	for _, id := range orphanUI {
		fmt.Printf("scripts/check-permission-enforcement/main.go: [A7] %q está declarada como capacidade de UI, "+
			"mas NENHUMA tela do frontend a consome — o toggle salva e nada muda. Ligue-a a uma tela "+
			"(ex.: `<Can permission=%q>`) ou remova-a do catálogo\n", id, id)
	}

	for _, id := range violations {
		fmt.Printf("%s: [A7] a permissão %q existe no catálogo mas nenhum código de produção a aplica — "+
			"quem revogá-la na tela de Permissões não muda nada. Aplique-a em um guard/serviço ou declare o "+
			"motivo em scripts/check-permission-enforcement/main.go (uiOnlyPermissions)\n",
			catalogPath, id)
	}
	if len(violations)+len(orphanUI) > 0 {
		fmt.Printf("\ncheck-permission-enforcement: %d permissão(ões) sem enforcement\n", len(violations)+len(orphanUI))
		os.Exit(1)
	}
	fmt.Printf("check-permission-enforcement: ok (%d permissões no catálogo, %d declaradas como UI/navegação e todas com consumidor no frontend)\n",
		len(ids), len(uiOnlyPermissions))
}

// collectReferences varre os arquivos Go de produção e devolve todo literal no
// formato `recurso:acao`.
func collectReferences(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	// `scripts` fica de fora: os verificadores citam ids em allowlists e não são
	// consumidores. Testes também não contam — o que se mede é produção.
	skipDirs := map[string]struct{}{
		".git": {}, "node_modules": {}, "graphify-out": {}, "scripts": {},
		"dist": {}, "test-results": {}, ".venv": {}, ".tools": {}, "docs": {},
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if _, skip := skipDirs[d.Name()]; skip {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		// O catálogo declara todos os ids: contá-lo zeraria o check.
		if filepath.ToSlash(rel) == filepath.ToSlash(catalogPath) {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range permissionLiteralRe.FindAllStringSubmatch(string(src), -1) {
			out[m[1]] = struct{}{}
		}
		return nil
	})
	return out, err
}

// collectFrontendReferences varre o frontend e devolve os ids citados como
// literal (a forma usada por `<Can permission="...">` e pelo registro de rotas).
func collectFrontendReferences(root string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	srcRoot := filepath.Join(root, "toka", "src")
	if _, err := os.Stat(srcRoot); err != nil {
		// Sem frontend no checkout: não há como verificar consumidor de UI.
		return out, nil
	}
	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".tsx") {
			return nil
		}
		if strings.Contains(name, ".test.") || strings.Contains(name, ".spec.") {
			return nil
		}
		// O catálogo gerado declara todos os ids: contá-lo zeraria o check.
		if name == "catalog.generated.ts" {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range permissionLiteralRe.FindAllStringSubmatch(string(src), -1) {
			out[m[1]] = struct{}{}
		}
		return nil
	})
	return out, err
}
