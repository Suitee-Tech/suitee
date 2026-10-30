package handlers_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// P1/A7 — o router não pode exigir permissão que não existe no catálogo.
//
// A revisão de permissões (docs/reviews/2026-09-19-permissoes-acesso-review.md,
// achado A7/M3) registrou `transactions.DELETE("/:id", ... "finance:delete")`:
// `finance:delete` não existe em catálogo algum, então a rota era inalcançável
// para TODO papel funcional — só perfil root passava. O mesmo padrão (permissão
// citada na borda que o catálogo não conhece) é o que M15 chama de divergência
// de item.
//
// Este teste é a versão local do check do CI (`scripts/check-route-guards`,
// regra A7): lê o router e o catálogo do backend e cruza os dois. Também fixa os
// dois guards que a rodada mudou.
func TestRouterGuardsReferenceExistingCatalogPermissions(t *testing.T) {
	root := repoRoot(t)
	router := readFile(t, filepath.Join(root, "toca-financial", "internal", "handlers", "router.go"))
	catalogSrc := readFile(t, filepath.Join(root, "pkg", "auth", "permission_catalog.go"))

	catalog := map[string]bool{}
	for _, m := range regexp.MustCompile(`\{ID: "([^"]+)"`).FindAllStringSubmatch(catalogSrc, -1) {
		catalog[m[1]] = true
	}
	if len(catalog) == 0 {
		t.Fatal("catálogo não foi lido — o teste perderia o efeito")
	}

	callRe := regexp.MustCompile(`Require(?:Any)?Permission(?:OrInternalKey)?\([^)]*\)`)
	literalRe := regexp.MustCompile(`"([a-z_]+:[a-z_]+)"`)

	checked := 0
	for _, line := range strings.Split(router, "\n") {
		if !strings.Contains(line, "Permission(") {
			continue
		}
		for _, call := range callRe.FindAllString(line, -1) {
			for _, m := range literalRe.FindAllStringSubmatch(call, -1) {
				checked++
				if !catalog[m[1]] {
					t.Errorf("rota exige %q, que não existe no catálogo — rota inalcançável para todo papel funcional:\n  %s", m[1], strings.TrimSpace(line))
				}
			}
		}
	}
	if checked < 50 {
		t.Fatalf("só %d guards encontrados no router — a varredura perdeu o alvo", checked)
	}
}

// O `DELETE /transactions/:id` passa a usar `finance:write` (M3) e os relatórios
// passam a aceitar `reports:read` (A7), mantendo `finance:read` para as matrizes
// que concedem só ele.
func TestRouterGuards_A7Decisions(t *testing.T) {
	root := repoRoot(t)
	router := readFile(t, filepath.Join(root, "toca-financial", "internal", "handlers", "router.go"))

	if strings.Contains(router, `"finance:delete"`) {
		t.Error("`finance:delete` voltou ao router: a permissão não existe no catálogo (M3)")
	}
	if !strings.Contains(router, `transactions.DELETE("/:id", auth.RequirePermission(permissionProvider, "finance:write"), txHandler.Delete)`) {
		t.Error("DELETE /transactions/:id deve exigir `finance:write` (mesma capacidade de settle/cancel/reverse)")
	}

	reports := groupBlock(t, router, `reports := secured.Group\("/reports"\)`)
	if !strings.Contains(reports, `"reports:read"`) {
		t.Error("o grupo /reports deve aceitar `reports:read` — sem isso a permissão não tem efeito (A7)")
	}
	if !strings.Contains(reports, `"finance:read"`) {
		t.Error("o grupo /reports deve continuar aceitando `finance:read` (matrizes de tenant que só concedem essa)")
	}
}

// groupBlock devolve o texto do bloco `{ ... }` que segue a declaração do grupo.
func groupBlock(t *testing.T, src, declRe string) string {
	t.Helper()
	loc := regexp.MustCompile(declRe).FindStringIndex(src)
	if loc == nil {
		t.Fatalf("declaração do grupo não encontrada: %s", declRe)
	}
	open := strings.Index(src[loc[0]:], "{")
	if open < 0 {
		t.Fatal("bloco do grupo não encontrado")
	}
	start := loc[0] + open
	depth := 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start : i+1]
			}
		}
	}
	t.Fatal("bloco do grupo não fechou")
	return ""
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ler %s: %v", path, err)
	}
	return string(b)
}

// repoRoot sobe até o diretório que contém go.work (raiz do monorepo).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 12; i++ {
		// go.work é gerado localmente e não é versionado; o checkout público
		// tem .git como marcador estável da raiz do monorepo.
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("go.work não encontrado a partir de %s", dir)
	return ""
}
