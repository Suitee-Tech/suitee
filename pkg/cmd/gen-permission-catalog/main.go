// gen-permission-catalog gera o catálogo TypeScript de permissões a partir do
// catálogo Go canônico (`pkg/auth/permission_catalog.go` + `roles.go`).
//
// MOTIVAÇÃO (achado A6 da revisão de permissões de 2026-09-19): backend e
// frontend mantinham dois catálogos escritos à mão. Eles divergiram em 46 de 63
// permissões — sempre na direção "frontend mais permissivo" — e cinco ids
// existiam em apenas um dos lados. Consequências reais:
//
//   - salvar a tela de Permissões persistia os defaults do frontend e ESCALAVA
//     privilégios (ex.: `cliente` ganhava `documents:write`, `operacoes` ganhava
//     `settings:manage_general`);
//   - toggles do frontend para ids que o backend não conhece eram descartados em
//     silêncio (`leads:receive_roulette`, `settings:manage_commission`,
//     `company:manage_groups`, `company:manage_group_members`);
//   - `properties:capture` existia só no backend e o tenant não podia configurá-la.
//
// A partir daqui existe UMA fonte: o catálogo Go. O arquivo TS é artefato de
// build — quem quiser mudar defaults/permissões edita o Go e roda este gerador.
//
// Uso:
//
//	go run ./pkg/cmd/gen-permission-catalog                 # escreve o arquivo
//	go run ./pkg/cmd/gen-permission-catalog -check          # falha se estiver desatualizado
//	go run ./pkg/cmd/gen-permission-catalog -out=outro.ts
//
// Exit codes: 0 ok, 1 desatualizado (-check), 2 erro de IO/uso.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/oginoino/toca/pkg/auth"
)

const defaultOut = "toka/src/shared/permissions/catalog.generated.ts"

func main() {
	out := flag.String("out", defaultOut, "caminho do arquivo TS de saída")
	check := flag.Bool("check", false, "apenas verifica se o arquivo está atualizado (não escreve)")
	flag.Parse()

	if err := run(*out, *check); err != nil {
		fmt.Fprintf(os.Stderr, "gen-permission-catalog: %v\n", err)
		os.Exit(2)
	}
}

func run(out string, check bool) error {
	if err := validateCatalog(); err != nil {
		return err
	}
	// O caminho de saída é relativo à RAIZ DO REPO (não ao CWD): o gerador pode
	// ser chamado de `pkg/` ou da raiz, e gravar em `pkg/toka/...` por acidente
	// já aconteceu uma vez.
	if !filepath.IsAbs(out) {
		root, err := repoRoot()
		if err != nil {
			return err
		}
		out = filepath.Join(root, out)
	}
	generated := render()

	current, err := os.ReadFile(out)
	switch {
	case err == nil && bytes.Equal(current, generated):
		fmt.Printf("gen-permission-catalog: %s está atualizado (%d permissões, %d papéis)\n",
			out, len(auth.PermissionCatalog), len(auth.AllFunctionalRoles))
		return nil
	case err == nil && check:
		fmt.Fprintf(os.Stderr,
			"gen-permission-catalog: %s está DESATUALIZADO em relação a pkg/auth/permission_catalog.go.\n"+
				"Rode: go run ./pkg/cmd/gen-permission-catalog\n", out)
		os.Exit(1)
	case err != nil && check:
		return fmt.Errorf("ler %s: %w", out, err)
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return fmt.Errorf("criar diretório de %s: %w", out, err)
	}
	if err := os.WriteFile(out, generated, 0o644); err != nil {
		return fmt.Errorf("escrever %s: %w", out, err)
	}
	fmt.Printf("gen-permission-catalog: %s gerado (%d permissões, %d papéis)\n",
		out, len(auth.PermissionCatalog), len(auth.AllFunctionalRoles))
	return nil
}

// repoRoot sobe diretórios a partir do CWD até encontrar o go.work do monorepo.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("raiz do repo não encontrada (nenhum go.work acima de %s)", dir)
		}
		dir = parent
	}
}

// validateCatalog recusa um catálogo incoerente ANTES de gerar o TS: ids
// duplicados, papel desconhecido em DefaultRoles ou rótulo ausente.
func validateCatalog() error {
	seen := map[string]bool{}
	for _, p := range auth.PermissionCatalog {
		if strings.TrimSpace(p.ID) == "" {
			return fmt.Errorf("permissão sem ID no catálogo Go")
		}
		if seen[p.ID] {
			return fmt.Errorf("permissão duplicada no catálogo Go: %s", p.ID)
		}
		seen[p.ID] = true
		if strings.TrimSpace(p.Label) == "" {
			return fmt.Errorf("permissão %s sem label", p.ID)
		}
		if strings.TrimSpace(p.Module) == "" {
			return fmt.Errorf("permissão %s sem module", p.ID)
		}
		for role := range p.DefaultRoles {
			if !auth.IsValidFunctionalRole(role) {
				return fmt.Errorf("permissão %s concede papel desconhecido %q", p.ID, role)
			}
		}
	}
	if len(auth.AllFunctionalRoles) == 0 {
		return fmt.Errorf("AllFunctionalRoles vazio")
	}
	for _, role := range auth.AllFunctionalRoles {
		if _, ok := auth.RoleLabels[role]; !ok {
			return fmt.Errorf("papel %s sem rótulo em RoleLabels", role)
		}
	}
	// Todo papel do catálogo precisa existir em AllFunctionalRoles — a ordem e o
	// conjunto precisam bater com o que o middleware aceita.
	roles := map[string]bool{}
	for _, r := range auth.AllFunctionalRoles {
		roles[r] = true
	}
	for _, p := range auth.PermissionCatalog {
		for role := range p.DefaultRoles {
			if !roles[role] {
				return fmt.Errorf("permissão %s referencia papel %q fora de AllFunctionalRoles", p.ID, role)
			}
		}
	}
	return nil
}

func render() []byte {
	var b strings.Builder

	b.WriteString("// Code generated by `go run ./pkg/cmd/gen-permission-catalog` from\n")
	b.WriteString("// pkg/auth/permission_catalog.go and pkg/auth/roles.go. DO NOT EDIT.\n")
	b.WriteString("//\n")
	b.WriteString("// O catálogo Go é a ÚNICA fonte de verdade (achado A6 da revisão de\n")
	b.WriteString("// permissões de 2026-09-19). Para mudar permissões, papéis ou defaults:\n")
	b.WriteString("// edite o Go e rode o gerador. `make check-permission-catalog` falha no CI\n")
	b.WriteString("// quando este arquivo está defasado.\n\n")

	// Papéis funcionais
	b.WriteString("export const FUNCTIONAL_ROLES = [\n")
	for _, role := range auth.AllFunctionalRoles {
		fmt.Fprintf(&b, "  %q,\n", role)
	}
	b.WriteString("] as const;\n\n")
	b.WriteString("export type FunctionalRole = (typeof FUNCTIONAL_ROLES)[number];\n\n")

	b.WriteString("export const ROLE_LABELS: Record<FunctionalRole, string> = {\n")
	for _, role := range auth.AllFunctionalRoles {
		fmt.Fprintf(&b, "  %s: %q,\n", role, auth.RoleLabels[role])
	}
	b.WriteString("};\n\n")

	b.WriteString("export interface PermissionDef {\n")
	b.WriteString("  id: string;\n")
	b.WriteString("  label: string;\n")
	b.WriteString("  module: string;\n")
	b.WriteString("  defaultRoles: Record<FunctionalRole, boolean>;\n")
	b.WriteString("}\n\n")

	b.WriteString("export const PERMISSION_CATALOG: PermissionDef[] = [\n")
	for _, p := range auth.PermissionCatalog {
		fmt.Fprintf(&b, "  { id: %q, label: %q, module: %q, defaultRoles: {", p.ID, p.Label, p.Module)
		for _, role := range auth.AllFunctionalRoles {
			fmt.Fprintf(&b, " %s: %t,", role, p.DefaultRoles[role])
		}
		b.WriteString(" } },\n")
	}
	b.WriteString("];\n\n")

	// Derivados — mesma forma que o frontend já consumia.
	b.WriteString("export const DEFAULT_MATRIX = PERMISSION_CATALOG.reduce(\n")
	b.WriteString("  (acc, p) => {\n")
	b.WriteString("    acc[p.id] = { ...p.defaultRoles };\n")
	b.WriteString("    return acc;\n")
	b.WriteString("  },\n")
	b.WriteString("  {} as Record<string, Record<FunctionalRole, boolean>>,\n")
	b.WriteString(");\n\n")

	b.WriteString("export const PERMISSION_LABELS_PT = PERMISSION_CATALOG.reduce(\n")
	b.WriteString("  (acc, p) => {\n")
	b.WriteString("    acc[p.id] = p.label;\n")
	b.WriteString("    return acc;\n")
	b.WriteString("  },\n")
	b.WriteString("  {} as Record<string, string>,\n")
	b.WriteString(");\n\n")

	b.WriteString("export const PERMISSION_IDS: string[] = PERMISSION_CATALOG.map((p) => p.id);\n\n")

	b.WriteString("export const PERMISSION_MODULES: string[] = [\n")
	for _, m := range modules() {
		fmt.Fprintf(&b, "  %q,\n", m)
	}
	b.WriteString("];\n")

	return []byte(b.String())
}

// modules devolve os módulos na ordem de primeira aparição no catálogo Go (a
// mesma ordem que a tela de Permissões usa para agrupar).
func modules() []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range auth.PermissionCatalog {
		if !seen[p.Module] {
			seen[p.Module] = true
			out = append(out, p.Module)
		}
	}
	return out
}
